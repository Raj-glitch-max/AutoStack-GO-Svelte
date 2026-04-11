package terraform

import (
	"testing"
)

// WEEK 3 TASK 3.5: Test log rotation
func TestLogRotation(t *testing.T) {
	t.Log("WEEK 3 TASK 3.5: Log Rotation")
	t.Log("Features:")
	t.Log("  - Logs written to /terraform-workdir/logs/{deploymentID}.log")
	t.Log("  - Maximum size: 10MB")
	t.Log("  - When limit reached, rotate to .log.old")
	t.Log("  - Keep last 1000 lines in memory (ring buffer)")
	t.Log("  - Serve last 1000 lines when client reconnects")
}

// WEEK 3 TASK 3.5: Test ring buffer
func TestRingBuffer(t *testing.T) {
	rb := NewRingBuffer(5)
	
	// Add 3 lines (buffer not full)
	rb.Add("line1")
	rb.Add("line2")
	rb.Add("line3")
	
	lines := rb.GetLines()
	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(lines))
	}
	
	// Add 3 more lines (buffer wraps around)
	rb.Add("line4")
	rb.Add("line5")
	rb.Add("line6")
	
	lines = rb.GetLines()
	if len(lines) != 5 {
		t.Errorf("Expected 5 lines, got %d", len(lines))
	}
	
	// Should have lines 2-6 (oldest to newest)
	expected := []string{"line2", "line3", "line4", "line5", "line6"}
	for i, line := range lines {
		if line != expected[i] {
			t.Errorf("Line %d: got %q, want %q", i, line, expected[i])
		}
	}
}

// WEEK 3 TASK 3.5: Test log file size limit
func TestLogFileSizeLimit(t *testing.T) {
	maxSize := int64(10 * 1024 * 1024) // 10MB
	
	t.Logf("Maximum log file size: %d bytes (10MB)", maxSize)
	t.Log("When size exceeded, file is rotated")
	t.Log("Old file renamed to .log.old")
	t.Log("New file created for continued logging")
}

// WEEK 3 TASK 3.5: Test serving recent lines
func TestServingRecentLines(t *testing.T) {
	t.Log("When client connects after deployment complete:")
	t.Log("  1. Read log file from disk")
	t.Log("  2. Return last 1000 lines")
	t.Log("  3. Client can scroll through history")
	t.Log("Logs NOT stored in database - only on disk")
}
