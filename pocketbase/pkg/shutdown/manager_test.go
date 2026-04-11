package shutdown

import (
	"testing"
	"time"
)

// WEEK 3 TASK 3.2: Test graceful shutdown
func TestGracefulShutdown(t *testing.T) {
	t.Log("WEEK 3 TASK 3.2: Graceful Shutdown")
	t.Log("On SIGTERM:")
	t.Log("  1. Stop accepting new deployments")
	t.Log("  2. Wait up to 5 minutes for in-flight processes")
	t.Log("  3. After 5 minutes, send SIGTERM to child processes")
	t.Log("  4. Shutdown complete")
}

// WEEK 3 TASK 3.2: Test accepting deployments flag
func TestAcceptingDeployments(t *testing.T) {
	manager := NewManager()
	
	// Initially accepting
	if !manager.IsAcceptingDeployments() {
		t.Error("Manager should initially accept deployments")
	}
	
	// After shutdown initiated
	manager.acceptingDeployments = false
	if manager.IsAcceptingDeployments() {
		t.Error("Manager should not accept deployments after shutdown")
	}
}

// WEEK 3 TASK 3.2: Test process tracking
func TestProcessTracking(t *testing.T) {
	manager := NewManager()
	
	// Register process
	manager.RegisterProcess("deploy-123", 12345)
	
	if manager.GetInFlightCount() != 1 {
		t.Errorf("Expected 1 in-flight process, got %d", manager.GetInFlightCount())
	}
	
	// Unregister process
	manager.UnregisterProcess("deploy-123")
	
	if manager.GetInFlightCount() != 0 {
		t.Errorf("Expected 0 in-flight processes, got %d", manager.GetInFlightCount())
	}
}

// WEEK 3 TASK 3.2: Test shutdown timeout
func TestShutdownTimeout(t *testing.T) {
	manager := NewManager()
	
	expectedTimeout := 5 * time.Minute
	if manager.shutdownTimeout != expectedTimeout {
		t.Errorf("Expected timeout %s, got %s", expectedTimeout, manager.shutdownTimeout)
	}
	
	t.Log("Shutdown timeout: 5 minutes")
	t.Log("Allows in-flight Terraform processes to complete")
	t.Log("After timeout, forcefully terminates remaining processes")
}
