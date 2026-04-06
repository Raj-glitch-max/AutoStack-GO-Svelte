package watcher

import (
	"log"
	"net/http"

	"github.com/PranavMagar/autostack/pkg/terraform"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"
)

var (
	awsLogStreamer *terraform.WebSocketLogStreamer
	upgrader       = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow connections from any origin in development
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

// WsAWSTerraformLogsHandler handles WebSocket connections for AWS Terraform logs
func WsAWSTerraformLogsHandler(c echo.Context) error {
	deploymentID := c.QueryParam("deploymentId")
	if deploymentID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "deploymentId parameter is required")
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(c.Response().Writer, c.Request(), nil)
	if err != nil {
		log.Printf("Failed to upgrade WebSocket connection: %v", err)
		return err
	}
	defer conn.Close()

	// Add connection to the log streamer
	awsLogStreamer.AddConnection(deploymentID, conn)
	defer awsLogStreamer.RemoveConnection(deploymentID, conn)

	log.Printf("WebSocket connection established for AWS deployment: %s", deploymentID)

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
			log.Printf("Received message from client: %s", string(message))
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

	log.Printf("WebSocket connection closed for AWS deployment: %s", deploymentID)
	return nil
}

// WsAWSDeploymentStatusHandler handles WebSocket connections for AWS deployment status updates
func WsAWSDeploymentStatusHandler(c echo.Context) error {
	deploymentID := c.QueryParam("deploymentId")
	if deploymentID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "deploymentId parameter is required")
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(c.Response().Writer, c.Request(), nil)
	if err != nil {
		log.Printf("Failed to upgrade WebSocket connection: %v", err)
		return err
	}
	defer conn.Close()

	log.Printf("WebSocket connection established for AWS deployment status: %s", deploymentID)

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
			log.Printf("Received status message from client: %s", string(message))
		case websocket.PingMessage:
			err = conn.WriteMessage(websocket.PongMessage, nil)
			if err != nil {
				log.Printf("Failed to send pong: %v", err)
				break
			}
		}
	}

	log.Printf("WebSocket connection closed for AWS deployment status: %s", deploymentID)
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