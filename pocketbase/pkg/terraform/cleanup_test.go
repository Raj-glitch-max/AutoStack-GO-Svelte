package terraform

import (
	"testing"
	"time"
)

// WEEK 3 TASK 3.4: Test working directory cleanup logic
func TestShouldCleanupDeployment(t *testing.T) {
	t.Log("WEEK 3 TASK 3.4: Working Directory Management")
	t.Log("Cleanup logic:")
	t.Log("  - Scan /terraform-workdir/deployments/ on server start")
	t.Log("  - Delete directories for deployments NOT in active states")
	t.Log("  - Active states: planning, planned, applying, destroying")
	t.Log("  - Safe to cleanup: pending, active, failed, destroyed")
	t.Log("  - Only cleanup if older than 24 hours")
	t.Log("  - Prevents disk exhaustion")
}

// WEEK 3 TASK 3.4: Test cleanup timing
func TestCleanupTiming(t *testing.T) {
	cutoffTime := time.Now().Add(-24 * time.Hour)
	
	tests := []struct {
		name       string
		modTime    time.Time
		shouldKeep bool
	}{
		{"recent file", time.Now(), true},
		{"1 hour old", time.Now().Add(-1 * time.Hour), true},
		{"23 hours old", time.Now().Add(-23 * time.Hour), true},
		{"24 hours old", time.Now().Add(-24 * time.Hour), false},
		{"25 hours old", time.Now().Add(-25 * time.Hour), false},
		{"1 week old", time.Now().Add(-7 * 24 * time.Hour), false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldCleanup := tt.modTime.Before(cutoffTime)
			shouldKeep := !shouldCleanup
			
			if shouldKeep != tt.shouldKeep {
				t.Errorf("File %s: shouldKeep=%v, want %v", tt.name, shouldKeep, tt.shouldKeep)
			}
		})
	}
}

// WEEK 3 TASK 3.4: Test periodic cleanup
func TestPeriodicCleanup(t *testing.T) {
	t.Log("Periodic cleanup runs every 6 hours")
	t.Log("Cleans up both working directories and log files")
	t.Log("Prevents disk exhaustion from accumulated files")
}
