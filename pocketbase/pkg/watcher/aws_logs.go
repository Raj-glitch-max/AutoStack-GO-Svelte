package watcher

import (
	"log"
	"net/http"

	"github.com/Raj-glitch-max/autostack/pkg/middleware"
	"github.com/Raj-glitch-max/autostack/pkg/terraform"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase/models"
)

var (
	awsLogStreamer *terraform.WebSocketLogStreamer
	upgrader       = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow connections from any origin in development
			// TODO: In production, implement proper CORS policy
		},
	}
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

// WsAWSDeploymentStatusHandler handles WebSocket connections for AWS deployment status updates
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

	log.Printf("WebSocket connection established for AWS deployment status: %s (user: %s)", deploymentID, userID)

	// Send initial status
	err = conn.WriteJSON(map[string]interface{}{
		"type":         "status",
		"deploymentId": deploymentID,
		"status":       "connected",
		"message":      "Connected to deployment status updates",
	})
	if err != nil {
		log.Printf("Failed to send initial status: %v", err)
		return err
	}

	// TODO: Implement real-time status updates
	// This would involve:
	// 1. Subscribing to deployment status changes in the database
	// 2. Sending status updates when deployment state changes
	// 3. Handling client disconnections gracefully

	// Keep connection alive
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		switch messageType {
		case websocket.TextMessage:
			log.Printf("Received status message from client (user %s): %s", userID, string(message))
		case websocket.PingMessage:
			err = conn.WriteMessage(websocket.PongMessage, nil)
			if err != nil {
				log.Printf("Failed to send pong: %v", err)
				break
			}
		}
	}

	log.Printf("WebSocket connection closed for AWS deployment status: %s (user: %s)", deploymentID, userID)
	return nil
}

// BroadcastDeploymentStatus broadcasts a status update to all connected clients for a deployment
func BroadcastDeploymentStatus(deploymentID, status, message string) {
	// This function would be called when deployment status changes
	// For now, it's a placeholder for future implementation
	log.Printf("Broadcasting status update for deployment %s: %s - %s", deploymentID, status, message)
	
	// TODO: Implement actual broadcasting to WebSocket connections
	// This would involve maintaining a separate connection pool for status updates
}