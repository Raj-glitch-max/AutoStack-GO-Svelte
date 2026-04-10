package terraform

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/dbx"
)

// CleanupManager handles cleanup of old working directories
// WEEK 3 TASK 3.4: Working directory management
type CleanupManager struct {
	workingDir string
	app        *pocketbase.PocketBase
}

// NewCleanupManager creates a new cleanup manager
func NewCleanupManager(workingDir string, app *pocketbase.PocketBase) *CleanupManager {
	return &CleanupManager{
		workingDir: workingDir,
		app:        app,
	}
}

// CleanupOldWorkingDirectories scans and removes old working directories
// WEEK 3 TASK 3.4: On server start, delete directories for completed deployments older than 24 hours
func (cm *CleanupManager) CleanupOldWorkingDirectories(ctx context.Context) error {
	deploymentsDir := filepath.Join(cm.workingDir, "deployments")
	
	// Check if deployments directory exists
	if _, err := os.Stat(deploymentsDir); os.IsNotExist(err) {
		log.Printf("Deployments directory does not exist: %s", deploymentsDir)
		return nil
	}

	// Read all deployment directories
	entries, err := os.ReadDir(deploymentsDir)
	if err != nil {
		return fmt.Errorf("failed to read deployments directory: %w", err)
	}

	cleanedCount := 0
	errorCount := 0
	cutoffTime := time.Now().Add(-24 * time.Hour)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		deploymentID := entry.Name()
		dirPath := filepath.Join(deploymentsDir, deploymentID)

		// Get directory info
		info, err := os.Stat(dirPath)
		if err != nil {
			log.Printf("Failed to stat directory %s: %v", dirPath, err)
			errorCount++
			continue
		}

		// Check if directory is older than 24 hours
		if info.ModTime().After(cutoffTime) {
			continue
		}

		// Check deployment status in database
		shouldCleanup, err := cm.shouldCleanupDeployment(deploymentID)
		if err != nil {
			log.Printf("Failed to check deployment status for %s: %v", deploymentID, err)
			errorCount++
			continue
		}

		if shouldCleanup {
			log.Printf("Cleaning up old working directory: %s (modified: %s)", deploymentID, info.ModTime())
			if err := os.RemoveAll(dirPath); err != nil {
				log.Printf("Failed to remove directory %s: %v", dirPath, err)
				errorCount++
			} else {
				cleanedCount++
			}
		}
	}

	log.Printf("Working directory cleanup complete: %d cleaned, %d errors", cleanedCount, errorCount)
	return nil
}

// shouldCleanupDeployment checks if a deployment's working directory should be cleaned up
// WEEK 3 TASK 3.4: Only cleanup if NOT in active states (planning/planned/applying/destroying)
func (cm *CleanupManager) shouldCleanupDeployment(deploymentID string) (bool, error) {
	// Try to find the deployment
	deployment, err := cm.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		// If deployment not found, it's safe to cleanup
		return true, nil
	}

	status := deployment.GetString("status")
	
	// Active states that should NOT be cleaned up
	activeStates := map[string]bool{
		"planning":   true,
		"planned":    true,
		"applying":   true,
		"destroying": true,
	}

	// If in active state, do NOT cleanup
	if activeStates[status] {
		return false, nil
	}

	// Safe to cleanup: pending, active, failed, destroyed
	return true, nil
}

// CleanupLogsDirectory cleans up old log files
// WEEK 3 TASK 3.5: Log rotation - remove logs older than 7 days
func (cm *CleanupManager) CleanupLogsDirectory(ctx context.Context) error {
	logsDir := filepath.Join(cm.workingDir, "logs")
	
	// Check if logs directory exists
	if _, err := os.Stat(logsDir); os.IsNotExist(err) {
		log.Printf("Logs directory does not exist: %s", logsDir)
		return nil
	}

	// Read all log files
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return fmt.Errorf("failed to read logs directory: %w", err)
	}

	cleanedCount := 0
	errorCount := 0
	cutoffTime := time.Now().Add(-7 * 24 * time.Hour) // 7 days

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filePath := filepath.Join(logsDir, entry.Name())

		// Get file info
		info, err := os.Stat(filePath)
		if err != nil {
			log.Printf("Failed to stat log file %s: %v", filePath, err)
			errorCount++
			continue
		}

		// Check if file is older than 7 days
		if info.ModTime().After(cutoffTime) {
			continue
		}

		log.Printf("Cleaning up old log file: %s (modified: %s)", entry.Name(), info.ModTime())
		if err := os.Remove(filePath); err != nil {
			log.Printf("Failed to remove log file %s: %v", filePath, err)
			errorCount++
		} else {
			cleanedCount++
		}
	}

	log.Printf("Log cleanup complete: %d cleaned, %d errors", cleanedCount, errorCount)
	return nil
}

// StartPeriodicCleanup starts a background goroutine that periodically cleans up old files
// WEEK 3 TASK 3.4: Periodic cleanup to prevent disk exhaustion
func (cm *CleanupManager) StartPeriodicCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	
	go func() {
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				log.Println("Stopping periodic cleanup")
				return
			case <-ticker.C:
				log.Println("Starting periodic cleanup...")
				
				if err := cm.CleanupOldWorkingDirectories(ctx); err != nil {
					log.Printf("Error during working directory cleanup: %v", err)
				}
				
				if err := cm.CleanupLogsDirectory(ctx); err != nil {
					log.Printf("Error during logs cleanup: %v", err)
				}
			}
		}
	}()
	
	log.Printf("Periodic cleanup started (interval: %s)", interval)
}

// GetDeploymentsByStatus returns deployments with specific statuses
func (cm *CleanupManager) GetDeploymentsByStatus(statuses []string) ([]*string, error) {
	var deploymentIDs []*string
	
	for _, status := range statuses {
		deployments, err := cm.app.Dao().FindRecordsByExpr("awsDeployments",
			dbx.NewExp("status = {:status}", dbx.Params{"status": status}))
		if err != nil {
			return nil, err
		}
		
		for _, deployment := range deployments {
			id := deployment.Id
			deploymentIDs = append(deploymentIDs, &id)
		}
	}
	
	return deploymentIDs, nil
}
