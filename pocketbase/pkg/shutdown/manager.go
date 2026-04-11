package shutdown

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Manager handles graceful shutdown of the application
// WEEK 3 TASK 3.2: Graceful shutdown
type Manager struct {
	acceptingDeployments bool
	inFlightProcesses    map[string]*ProcessInfo
	mu                   sync.RWMutex
	shutdownTimeout      time.Duration
	shutdownChan         chan os.Signal
	ctx                  context.Context
	cancel               context.CancelFunc
}

// ProcessInfo tracks information about an in-flight process
type ProcessInfo struct {
	DeploymentID string
	StartTime    time.Time
	ProcessID    int
}

// NewManager creates a new shutdown manager
// WEEK 3 TASK 3.2: Initialize with 5-minute timeout for in-flight processes
func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Manager{
		acceptingDeployments: true,
		inFlightProcesses:    make(map[string]*ProcessInfo),
		shutdownTimeout:      5 * time.Minute,
		shutdownChan:         make(chan os.Signal, 1),
		ctx:                  ctx,
		cancel:               cancel,
	}
}

// Start starts listening for shutdown signals
// WEEK 3 TASK 3.2: Listen for SIGTERM and SIGINT
func (m *Manager) Start() {
	signal.Notify(m.shutdownChan, syscall.SIGTERM, syscall.SIGINT)
	
	go func() {
		sig := <-m.shutdownChan
		log.Printf("Received signal: %v - initiating graceful shutdown", sig)
		m.InitiateShutdown()
	}()
	
	log.Println("Shutdown manager started - listening for SIGTERM/SIGINT")
}

// InitiateShutdown begins the graceful shutdown process
// WEEK 3 TASK 3.2: Stop accepting new deployments, wait for in-flight processes
func (m *Manager) InitiateShutdown() {
	m.mu.Lock()
	m.acceptingDeployments = false
	inFlightCount := len(m.inFlightProcesses)
	m.mu.Unlock()
	
	log.Printf("Graceful shutdown initiated - %d in-flight processes", inFlightCount)
	log.Println("No longer accepting new deployments")
	
	if inFlightCount == 0 {
		log.Println("No in-flight processes - shutting down immediately")
		m.cancel()
		return
	}
	
	// Wait for in-flight processes with timeout
	log.Printf("Waiting up to %s for in-flight processes to complete...", m.shutdownTimeout)
	
	timeoutChan := time.After(m.shutdownTimeout)
	checkTicker := time.NewTicker(5 * time.Second)
	defer checkTicker.Stop()
	
	for {
		select {
		case <-timeoutChan:
			m.mu.RLock()
			remaining := len(m.inFlightProcesses)
			m.mu.RUnlock()
			
			if remaining > 0 {
				log.Printf("Shutdown timeout reached - %d processes still running", remaining)
				log.Println("Sending SIGTERM to remaining processes...")
				m.terminateRemainingProcesses()
			}
			
			m.cancel()
			return
			
		case <-checkTicker.C:
			m.mu.RLock()
			remaining := len(m.inFlightProcesses)
			m.mu.RUnlock()
			
			if remaining == 0 {
				log.Println("All in-flight processes completed - shutting down")
				m.cancel()
				return
			}
			
			log.Printf("Still waiting for %d in-flight processes...", remaining)
		}
	}
}

// IsAcceptingDeployments returns whether new deployments are being accepted
// WEEK 3 TASK 3.2: Check if server is accepting new deployments
func (m *Manager) IsAcceptingDeployments() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.acceptingDeployments
}

// RegisterProcess registers a new in-flight process
// WEEK 3 TASK 3.2: Track in-flight Terraform processes
func (m *Manager) RegisterProcess(deploymentID string, processID int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.inFlightProcesses[deploymentID] = &ProcessInfo{
		DeploymentID: deploymentID,
		StartTime:    time.Now(),
		ProcessID:    processID,
	}
	
	log.Printf("Registered in-flight process: deployment=%s pid=%d", deploymentID, processID)
}

// UnregisterProcess removes a process from tracking
// WEEK 3 TASK 3.2: Remove completed processes from tracking
func (m *Manager) UnregisterProcess(deploymentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if info, exists := m.inFlightProcesses[deploymentID]; exists {
		duration := time.Since(info.StartTime)
		delete(m.inFlightProcesses, deploymentID)
		log.Printf("Unregistered in-flight process: deployment=%s duration=%s", deploymentID, duration)
	}
}

// terminateRemainingProcesses sends SIGTERM to all remaining processes
// WEEK 3 TASK 3.2: After 5 minutes, send SIGTERM to child processes
func (m *Manager) terminateRemainingProcesses() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for deploymentID, info := range m.inFlightProcesses {
		log.Printf("Terminating process: deployment=%s pid=%d", deploymentID, info.ProcessID)
		
		// Find the process
		process, err := os.FindProcess(info.ProcessID)
		if err != nil {
			log.Printf("Failed to find process %d: %v", info.ProcessID, err)
			continue
		}
		
		// Send SIGTERM
		if err := process.Signal(syscall.SIGTERM); err != nil {
			log.Printf("Failed to send SIGTERM to process %d: %v", info.ProcessID, err)
		}
	}
}

// GetContext returns the shutdown context
// WEEK 3 TASK 3.2: Context is cancelled when shutdown is complete
func (m *Manager) GetContext() context.Context {
	return m.ctx
}

// GetInFlightCount returns the number of in-flight processes
func (m *Manager) GetInFlightCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.inFlightProcesses)
}

// WaitForShutdown blocks until shutdown is complete
func (m *Manager) WaitForShutdown() {
	<-m.ctx.Done()
	log.Println("Shutdown complete")
}

// Stop stops the shutdown manager (for testing)
func (m *Manager) Stop() {
	signal.Stop(m.shutdownChan)
	close(m.shutdownChan)
}
