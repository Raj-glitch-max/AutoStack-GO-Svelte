package watcher

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/middleware"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/terraform"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase/models"
)

// StatusMessage is the shape pushed to clients on each deployment state transition.
// The frontend interprets "state" to drive the progress indicator.
type StatusMessage struct {
	Type         string `json:"type"`         // always "status"
	DeploymentID string `json:"deploymentId"`
	State        string `json:"state"`        // queued | planning | applying | success | failed
	Message      string `json:"message"`
	Timestamp    string `json:"timestamp"`
}

var (
	awsLogStreamer *terraform.WebSocketLogStreamer
	upgrader       = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow connections from any origin in development
			// TODO: In production, implement proper CORS policy based on allowed origins.
		},
	}

	// statusConnections holds WebSocket connections subscribed to deployment status updates.
	// Keyed by deploymentID. Protected by statusMu.
	statusConnections = make(map[string][]*websocket.Conn)
	statusMu          sync.RWMutex
)

func init() {
	awsLogStreamer = terraform.NewWebSocketLogStreamer()
}

// GetAWSLogStreamer returns the global AWS log streamer instance
func GetAWSLogStreamer() *terraform.WebSocketLogStreamer {
	return awsLogStreamer
}

// validateWebSocketAuth validates JWT token and returns userID
// SECURITY: This MUST be called before upgrading the WebSocket connection
func validateWebSocketAuth(c echo.Context) (string, error) {
	// Try to get token from query parameter (for WebSocket connections)
	token := c.QueryParam("token")
	if token == "" {
		// Try to get from Authorization header as fallback
		authHeader := c.Request().Header.Get("Authorization")
		if authHeader != "" && len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}
	}

	if token == "" {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "authentication token required")
	}

	// Get the authenticated record from PocketBase
	// PocketBase's auth middleware should have already validated the token
	authRecord := c.Get("authRecord")
	if authRecord == nil {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
	}

	// Type assert to *models.Record
	record, ok := authRecord.(*models.Record)
	if !ok {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "invalid auth record")
	}

	return record.Id, nil
}

// WsAWSTerraformLogsHandler handles WebSocket connections for AWS Terraform logs
// SECURITY: Validates JWT before upgrading connection (Master Plan §3)
func WsAWSTerraformLogsHandler(c echo.Context) error {
	deploymentID := c.QueryParam("deploymentId")
	if deploymentID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "deploymentId parameter is required")
	}

	// SECURITY: Validate authentication BEFORE upgrading connection
	// This is CRITICAL - never upgrade before auth validation (Master Plan C6)
	userID, err := validateWebSocketAuth(c)
	if err != nil {
		log.Printf("WebSocket auth failed for deployment %s: %v", deploymentID, err)
		return err
	}

	// SECURITY: Verify user owns the deployment before allowing connection
	// Load deployment record from database
	app := c.Get("app")
	if app == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "application context not available")
	}

	// Get PocketBase app instance
	pbApp, ok := app.(interface {
		Dao() interface {
			FindRecordById(string, string, ...func(interface{}) error) (*models.Record, error)
		}
	})
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "invalid application context")
	}

	// Load deployment record
	deployment, err := pbApp.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		log.Printf("Deployment not found: %s", deploymentID)
		return echo.NewHTTPError(http.StatusNotFound, "deployment not found")
	}

	// SECURITY: Assert ownership - user must own the deployment
	if err := middleware.AssertOwnership(deployment, userID); err != nil {
		log.Printf("Ownership check failed for deployment %s, user %s: %v", deploymentID, userID, err)
		// Return 403, not 404 (don't leak existence)
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	// Only after authentication and authorization: upgrade connection
	conn, err := upgrader.Upgrade(c.Response().Writer, c.Request(), nil)
	if err != nil {
		log.Printf("Failed to upgrade WebSocket connection: %v", err)
		return err
	}
	defer conn.Close()

	// Add connection to the log streamer with user context
	awsLogStreamer.AddConnection(deploymentID, conn)
	defer awsLogStreamer.RemoveConnection(deploymentID, conn)

	log.Printf("WebSocket connection established for AWS deployment: %s (user: %s)", deploymentID, userID)

	// Send initial connection confirmation
	err = conn.WriteJSON(map[string]interface{}{
		"type":         "connection",
		"status":       "connected",
		"deploymentId": deploymentID,
		"message":      "Connected to AWS Terraform logs",
	})
	if err != nil {
		log.Printf("Failed to send connection confirmation: %v", err)
		return err
	}

	// Keep connection alive and handle client messages
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle different message types
		switch messageType {
		case websocket.TextMessage:
			log.Printf("Received message from client (user %s): %s", userID, string(message))
			// Echo back a pong message
			err = conn.WriteJSON(map[string]interface{}{
				"type":    "pong",
				"message": "Message received",
			})
			if err != nil {
				log.Printf("Failed to send pong message: %v", err)
				break
			}
		case websocket.PingMessage:
			err = conn.WriteMessage(websocket.PongMessage, nil)
			if err != nil {
				log.Printf("Failed to send pong: %v", err)
				break
			}
		}
	}

	log.Printf("WebSocket connection closed for AWS deployment: %s (user: %s)", deploymentID, userID)
	return nil
}

// WsAWSDeploymentStatusHandler handles WebSocket connections for AWS deployment status updates.
// It sends a real-time status message whenever the deployment state changes.
// SECURITY: Validates JWT before upgrading connection (Master Plan §3)
func WsAWSDeploymentStatusHandler(c echo.Context) error {
	deploymentID := c.QueryParam("deploymentId")
	if deploymentID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "deploymentId parameter is required")
	}

	// SECURITY: Validate authentication BEFORE upgrading connection
	userID, err := validateWebSocketAuth(c)
	if err != nil {
		log.Printf("WebSocket auth failed for deployment status %s: %v", deploymentID, err)
		return err
	}

	// SECURITY: Verify user owns the deployment
	app := c.Get("app")
	if app == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "application context not available")
	}

	pbApp, ok := app.(interface {
		Dao() interface {
			FindRecordById(string, string, ...func(interface{}) error) (*models.Record, error)
		}
	})
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "invalid application context")
	}

	deployment, err := pbApp.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		log.Printf("Deployment not found: %s", deploymentID)
		return echo.NewHTTPError(http.StatusNotFound, "deployment not found")
	}

	if err := middleware.AssertOwnership(deployment, userID); err != nil {
		log.Printf("Ownership check failed for deployment %s, user %s: %v", deploymentID, userID, err)
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	// Only after authentication and authorization: upgrade connection
	conn, err := upgrader.Upgrade(c.Response().Writer, c.Request(), nil)
	if err != nil {
		log.Printf("Failed to upgrade WebSocket connection: %v", err)
		return err
	}
	defer conn.Close()

	log.Printf("Status WebSocket connected: deployment=%s user=%s", deploymentID, userID)

	// Register this connection so BroadcastDeploymentStatus can push to it.
	registerStatusConn(deploymentID, conn)
	defer unregisterStatusConn(deploymentID, conn)

	// Send the current deployment status immediately on connect so the client
	// doesn't have to wait for the next state transition.
	currentState := deployment.GetString("status")
	initialMsg := StatusMessage{
		Type:         "status",
		DeploymentID: deploymentID,
		State:        currentState,
		Message:      "Connected — current state: " + currentState,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}
	if err := conn.WriteJSON(initialMsg); err != nil {
		log.Printf("Failed to send initial status for %s: %v", deploymentID, err)
		return nil // connection broken; nothing to do
	}

	// Keep connection alive, handling pings and client-sent messages.
	for {
		messageType, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Status WebSocket error for %s: %v", deploymentID, err)
			}
			break
		}
		if messageType == websocket.PingMessage {
			if err := conn.WriteMessage(websocket.PongMessage, nil); err != nil {
				break
			}
		}
	}

	log.Printf("Status WebSocket closed: deployment=%s user=%s", deploymentID, userID)
	return nil
}

// BroadcastDeploymentStatus pushes a deployment state change to every WebSocket client
// currently subscribed to this deployment's status channel.
//
// Callers: the Terraform executor and awsDeployments controller call this after every
// status transition (queued → planning → applying → success | failed).
func BroadcastDeploymentStatus(deploymentID, state, message string) {
	msg := StatusMessage{
		Type:         "status",
		DeploymentID: deploymentID,
		State:        state,
		Message:      message,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("BroadcastDeploymentStatus: failed to marshal message: %v", err)
		return
	}

	statusMu.Lock()
	conns := make([]*websocket.Conn, len(statusConnections[deploymentID]))
	copy(conns, statusConnections[deploymentID])
	statusMu.Unlock()

	var deadConns []*websocket.Conn
	for _, conn := range conns {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("BroadcastDeploymentStatus: write failed for %s: %v", deploymentID, err)
			deadConns = append(deadConns, conn)
		}
	}

	// Clean up dead connections outside the lock-copy section.
	for _, dead := range deadConns {
		unregisterStatusConn(deploymentID, dead)
		dead.Close()
	}

	log.Printf("BroadcastDeploymentStatus: %s → %s (%d clients)", deploymentID, state, len(conns)-len(deadConns))
}

// registerStatusConn adds a WebSocket connection to the status broadcast pool.
func registerStatusConn(deploymentID string, conn *websocket.Conn) {
	statusMu.Lock()
	defer statusMu.Unlock()
	statusConnections[deploymentID] = append(statusConnections[deploymentID], conn)
}

// unregisterStatusConn removes a WebSocket connection from the status broadcast pool.
func unregisterStatusConn(deploymentID string, conn *websocket.Conn) {
	statusMu.Lock()
	defer statusMu.Unlock()
	conns := statusConnections[deploymentID]
	for i, c := range conns {
		if c == conn {
			statusConnections[deploymentID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}
	if len(statusConnections[deploymentID]) == 0 {
		delete(statusConnections, deploymentID)
	}
}