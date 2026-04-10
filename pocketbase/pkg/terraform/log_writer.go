package terraform

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// RotatingLogWriter writes logs to disk with size-based rotation
// WEEK 3 TASK 3.5: Log rotation for terraform logs
type RotatingLogWriter struct {
	workingDir    string
	deploymentID  string
	maxSize       int64 // Maximum size in bytes (10MB)
	currentSize   int64
	file          *os.File
	mu            sync.Mutex
	ringBuffer    *RingBuffer // Keep last 1000 lines in memory
}

// RingBuffer stores the last N lines in memory
type RingBuffer struct {
	lines    []string
	maxLines int
	index    int
	full     bool
	mu       sync.RWMutex
}

// NewRingBuffer creates a new ring buffer
func NewRingBuffer(maxLines int) *RingBuffer {
	return &RingBuffer{
		lines:    make([]string, maxLines),
		maxLines: maxLines,
		index:    0,
		full:     false,
	}
}

// Add adds a line to the ring buffer
func (rb *RingBuffer) Add(line string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	
	rb.lines[rb.index] = line
	rb.index++
	
	if rb.index >= rb.maxLines {
		rb.index = 0
		rb.full = true
	}
}

// GetLines returns all lines in order (oldest to newest)
func (rb *RingBuffer) GetLines() []string {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	
	if !rb.full {
		// Buffer not full yet, return from start to index
		return append([]string{}, rb.lines[:rb.index]...)
	}
	
	// Buffer is full, return from index to end, then start to index
	result := make([]string, 0, rb.maxLines)
	result = append(result, rb.lines[rb.index:]...)
	result = append(result, rb.lines[:rb.index]...)
	return result
}

// NewRotatingLogWriter creates a new rotating log writer
// WEEK 3 TASK 3.5: Logs written to /terraform-workdir/logs/{deploymentID}.log with 10MB max size
func NewRotatingLogWriter(workingDir, deploymentID string) (*RotatingLogWriter, error) {
	logsDir := filepath.Join(workingDir, "logs")
	
	// Create logs directory if it doesn't exist
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}
	
	logPath := filepath.Join(logsDir, fmt.Sprintf("%s.log", deploymentID))
	
	// Open or create log file
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	
	// Get current file size
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat log file: %w", err)
	}
	
	writer := &RotatingLogWriter{
		workingDir:   workingDir,
		deploymentID: deploymentID,
		maxSize:      10 * 1024 * 1024, // 10MB
		currentSize:  info.Size(),
		file:         file,
		ringBuffer:   NewRingBuffer(1000), // Keep last 1000 lines
	}
	
	// Load existing lines into ring buffer
	if info.Size() > 0 {
		writer.loadExistingLines()
	}
	
	return writer, nil
}

// loadExistingLines loads existing log lines into the ring buffer
func (w *RotatingLogWriter) loadExistingLines() {
	// Seek to beginning
	w.file.Seek(0, 0)
	
	scanner := bufio.NewScanner(w.file)
	for scanner.Scan() {
		w.ringBuffer.Add(scanner.Text())
	}
	
	// Seek back to end for appending
	w.file.Seek(0, 2)
}

// Write writes data to the log file with rotation
func (w *RotatingLogWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	// Check if we need to rotate
	if w.currentSize+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	
	// Write to file
	n, err = w.file.Write(p)
	if err != nil {
		return n, err
	}
	
	w.currentSize += int64(n)
	
	// Add to ring buffer (line by line)
	line := string(p)
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	w.ringBuffer.Add(line)
	
	return n, nil
}

// WriteLine writes a complete line to the log
func (w *RotatingLogWriter) WriteLine(line string) error {
	_, err := w.Write([]byte(line + "\n"))
	return err
}

// rotate rotates the log file
func (w *RotatingLogWriter) rotate() error {
	// Close current file
	if err := w.file.Close(); err != nil {
		return err
	}
	
	// Rename current file to .old
	logsDir := filepath.Join(w.workingDir, "logs")
	currentPath := filepath.Join(logsDir, fmt.Sprintf("%s.log", w.deploymentID))
	oldPath := filepath.Join(logsDir, fmt.Sprintf("%s.log.old", w.deploymentID))
	
	// Remove old backup if exists
	os.Remove(oldPath)
	
	// Rename current to old
	if err := os.Rename(currentPath, oldPath); err != nil {
		return err
	}
	
	// Create new file
	file, err := os.OpenFile(currentPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	
	w.file = file
	w.currentSize = 0
	
	return nil
}

// GetRecentLines returns the last N lines from the ring buffer
// WEEK 3 TASK 3.5: When client connects after deployment complete, serve last 1000 lines
func (w *RotatingLogWriter) GetRecentLines(maxLines int) []string {
	allLines := w.ringBuffer.GetLines()
	
	if len(allLines) <= maxLines {
		return allLines
	}
	
	// Return last maxLines
	return allLines[len(allLines)-maxLines:]
}

// Close closes the log writer
func (w *RotatingLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// ReadLogFile reads the entire log file for a deployment
// WEEK 3 TASK 3.5: Serve logs from disk when client reconnects
func ReadLogFile(workingDir, deploymentID string, maxLines int) ([]string, error) {
	logsDir := filepath.Join(workingDir, "logs")
	logPath := filepath.Join(logsDir, fmt.Sprintf("%s.log", deploymentID))
	
	file, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer file.Close()
	
	// Read all lines
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	
	// Return last maxLines
	if len(lines) > maxLines {
		return lines[len(lines)-maxLines:], nil
	}
	
	return lines, nil
}

// TailLogFile tails a log file and sends new lines to a channel
func TailLogFile(workingDir, deploymentID string, output chan<- string, done <-chan struct{}) error {
	logsDir := filepath.Join(workingDir, "logs")
	logPath := filepath.Join(logsDir, fmt.Sprintf("%s.log", deploymentID))
	
	file, err := os.Open(logPath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	// Seek to end
	file.Seek(0, io.SeekEnd)
	
	reader := bufio.NewReader(file)
	
	for {
		select {
		case <-done:
			return nil
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					// Wait a bit and try again
					continue
				}
				return err
			}
			
			output <- line
		}
	}
}
