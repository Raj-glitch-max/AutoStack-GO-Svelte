package terraform

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketLogStreamer implements LogStreamer using WebSocket connections
type WebSocketLogStreamer struct {
	connections map[string][]*websocket.Conn
	mutex       sync.RWMutex
}

// LogMessage represents a log message sent over WebSocket
type LogMessage struct {
	Type         string    `json:"type"` // always "log"
	DeploymentID string    `json:"deploymentId"`
	RolloutID    string    `json:"rolloutId"`
	Operation    string    `json:"operation"`
	Level        string    `json:"level"`
	Message      string    `json:"message"`
	Timestamp    time.Time `json:"timestamp"`
}

// NewWebSocketLogStreamer creates a new WebSocket log streamer
func NewWebSocketLogStreamer() *WebSocketLogStreamer {
	return &WebSocketLogStreamer{
		connections: make(map[string][]*websocket.Conn),
	}
}

// StreamLog sends a log message to all connected WebSocket clients for the deployment
func (w *WebSocketLogStreamer) StreamLog(deploymentID, rolloutID, operation, level, message string) error {
	logMsg := LogMessage{
		Type:         "log",
		DeploymentID: deploymentID,
		RolloutID:    rolloutID,
		Operation:    operation,
		Level:        level,
		Message:      message,
		Timestamp:    time.Now(),
	}

	data, err := json.Marshal(logMsg)
	if err != nil {
		return err
	}

	w.mutex.RLock()
	connections := w.connections[deploymentID]
	w.mutex.RUnlock()

	// Send to all connected clients for this deployment
	for i, conn := range connections {
		err := conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			// Remove broken connection
			w.mutex.Lock()
			w.connections[deploymentID] = append(connections[:i], connections[i+1:]...)
			w.mutex.Unlock()
			conn.Close()
		}
	}

	return nil
}

// AddConnection adds a WebSocket connection for a specific deployment
func (w *WebSocketLogStreamer) AddConnection(deploymentID string, conn *websocket.Conn) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	
	if w.connections[deploymentID] == nil {
		w.connections[deploymentID] = make([]*websocket.Conn, 0)
	}
	
	w.connections[deploymentID] = append(w.connections[deploymentID], conn)
}

// RemoveConnection removes a WebSocket connection
func (w *WebSocketLogStreamer) RemoveConnection(deploymentID string, conn *websocket.Conn) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	
	connections := w.connections[deploymentID]
	for i, c := range connections {
		if c == conn {
			w.connections[deploymentID] = append(connections[:i], connections[i+1:]...)
			break
		}
	}
	
	if len(w.connections[deploymentID]) == 0 {
		delete(w.connections, deploymentID)
	}
}

// DatabaseLogStreamer implements LogStreamer by storing logs in the database
type DatabaseLogStreamer struct {
	// This would typically have a database connection
	// For now, we'll just log to console and could extend to store in PocketBase
}

// NewDatabaseLogStreamer creates a new database log streamer
func NewDatabaseLogStreamer() *DatabaseLogStreamer {
	return &DatabaseLogStreamer{}
}

// StreamLog stores the log message in the database and also logs to console
func (d *DatabaseLogStreamer) StreamLog(deploymentID, rolloutID, operation, level, message string) error {
	// Log to console for now
	log.Printf("[%s] %s/%s %s: %s", level, deploymentID, rolloutID, operation, message)
	
	// TODO: Store in PocketBase terraformExecutions collection
	// This would involve:
	// 1. Finding or creating a terraformExecution record
	// 2. Appending the log message to the output field
	// 3. Updating the record in the database
	
	return nil
}

// CompositeLogStreamer combines multiple log streamers
type CompositeLogStreamer struct {
	streamers []LogStreamer
}

// NewCompositeLogStreamer creates a new composite log streamer
func NewCompositeLogStreamer(streamers ...LogStreamer) *CompositeLogStreamer {
	return &CompositeLogStreamer{
		streamers: streamers,
	}
}

// StreamLog sends the log message to all configured streamers
func (c *CompositeLogStreamer) StreamLog(deploymentID, rolloutID, operation, level, message string) error {
	for _, streamer := range c.streamers {
		if err := streamer.StreamLog(deploymentID, rolloutID, operation, level, message); err != nil {
			log.Printf("Error streaming log: %v", err)
		}
	}
	return nil
}

// BufferedLogStreamer buffers log messages and sends them in batches
type BufferedLogStreamer struct {
	underlying LogStreamer
	buffer     []LogMessage
	mutex      sync.Mutex
	batchSize  int
	flushTimer *time.Timer
}

// NewBufferedLogStreamer creates a new buffered log streamer
func NewBufferedLogStreamer(underlying LogStreamer, batchSize int, flushInterval time.Duration) *BufferedLogStreamer {
	b := &BufferedLogStreamer{
		underlying: underlying,
		buffer:     make([]LogMessage, 0, batchSize),
		batchSize:  batchSize,
	}
	
	b.flushTimer = time.AfterFunc(flushInterval, b.flush)
	return b
}

// StreamLog adds a log message to the buffer
func (b *BufferedLogStreamer) StreamLog(deploymentID, rolloutID, operation, level, message string) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	
	logMsg := LogMessage{
		DeploymentID: deploymentID,
		RolloutID:    rolloutID,
		Operation:    operation,
		Level:        level,
		Message:      message,
		Timestamp:    time.Now(),
	}
	
	b.buffer = append(b.buffer, logMsg)
	
	if len(b.buffer) >= b.batchSize {
		b.flushLocked()
	}
	
	return nil
}

// flush sends all buffered messages to the underlying streamer
func (b *BufferedLogStreamer) flush() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.flushLocked()
}

// flushLocked flushes the buffer (must be called with mutex held)
func (b *BufferedLogStreamer) flushLocked() {
	if len(b.buffer) == 0 {
		return
	}
	
	for _, msg := range b.buffer {
		b.underlying.StreamLog(msg.DeploymentID, msg.RolloutID, msg.Operation, msg.Level, msg.Message)
	}
	
	b.buffer = b.buffer[:0] // Clear buffer
}

// Close flushes any remaining messages and stops the timer
func (b *BufferedLogStreamer) Close() {
	b.flushTimer.Stop()
	b.flush()
}