package jobs

import (
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/pocketbase/models"
)

// createCostCleanupJob creates the daily cost data cleanup job for destroyed deployments
func (ps *PricingScheduler) createCostCleanupJob() func() error {
	return func() error {
		log.Println("Starting cost data cleanup job for destroyed deployments...")
		
		startTime := time.Now()
		
		// Find all deployments with status "destroyed" or "failed"
		deployments, err := ps.app.Dao().FindRecordsByFilter(
			"deployments",
			"status = 'destroyed' || status = 'failed'",
			"-updated",
			0,
			0,
			nil,
		)
		if err != nil {
			// If no records found, that's okay - just log and return
			log.Println("No destroyed deployments found, cleanup job completed")
			return nil
		}
		
		if len(deployments) == 0 {
			log.Println("No destroyed deployments found, cleanup job completed")
			return nil
		}
		
		log.Printf("Found %d destroyed/failed deployments to clean up", len(deployments))
		
		totalDeleted := 0
		errorCount := 0
		
		for _, deployment := range deployments {
			deploymentID := deployment.Id
			
			// Clean up cost data for this deployment
			deleted, err := ps.cleanupDeploymentCostData(deploymentID)
			if err != nil {
				log.Printf("Failed to cleanup cost data for deployment %s: %v", deploymentID, err)
				errorCount++
				continue
			}
			
			totalDeleted += deleted
			log.Printf("Cleaned up %d cost records for deployment %s", deleted, deploymentID)
		}
		
		duration := time.Since(startTime)
		log.Printf("Cost cleanup job completed: deleted %d records from %d deployments in %v (errors: %d)", 
			totalDeleted, len(deployments), duration, errorCount)
		
		// Record cleanup metrics
		if err := ps.recordCostCleanupMetrics(totalDeleted, len(deployments), errorCount, duration); err != nil {
			log.Printf("Failed to record cost cleanup metrics: %v", err)
		}
		
		return nil
	}
}

// cleanupDeploymentCostData removes all cost-related data for a specific deployment
// Returns the number of records deleted
func (ps *PricingScheduler) cleanupDeploymentCostData(deploymentID string) (int, error) {
	deletedCount := 0
	
	// 1. Clean up actualCosts collection
	actualCostsDeleted, err := ps.deleteRecordsByDeployment("actualCosts", deploymentID)
	if err != nil {
		return deletedCount, fmt.Errorf("failed to delete actualCosts: %w", err)
	}
	deletedCount += actualCostsDeleted
	
	// 2. Clean up costEstimates collection
	estimatesDeleted, err := ps.deleteRecordsByDeployment("costEstimates", deploymentID)
	if err != nil {
		return deletedCount, fmt.Errorf("failed to delete costEstimates: %w", err)
	}
	deletedCount += estimatesDeleted
	
	// 3. Clean up costAlerts collection
	alertsDeleted, err := ps.deleteRecordsByDeployment("costAlerts", deploymentID)
	if err != nil {
		return deletedCount, fmt.Errorf("failed to delete costAlerts: %w", err)
	}
	deletedCount += alertsDeleted
	
	return deletedCount, nil
}

// deleteRecordsByDeployment deletes all records in a collection for a specific deployment
// Returns the number of records deleted
func (ps *PricingScheduler) deleteRecordsByDeployment(collectionName, deploymentID string) (int, error) {
	// Find collection
	collection, err := ps.app.Dao().FindCollectionByNameOrId(collectionName)
	if err != nil {
		// If collection doesn't exist, that's okay - just skip it
		log.Printf("Collection %s not found, skipping: %v", collectionName, err)
		return 0, nil
	}
	
	// Find all records for this deployment
	records, err := ps.app.Dao().FindRecordsByFilter(
		collection.Id,
		"deployment = {:deploymentId}",
		"-created",
		0,
		0,
		map[string]any{
			"deploymentId": deploymentID,
		},
	)
	if err != nil {
		// If no records found, that's okay
		log.Printf("No records found in %s for deployment %s", collectionName, deploymentID)
		return 0, nil
	}
	
	deletedCount := 0
	for _, record := range records {
		if err := ps.app.Dao().DeleteRecord(record); err != nil {
			log.Printf("Failed to delete record %s from %s: %v", record.Id, collectionName, err)
			continue
		}
		deletedCount++
	}
	
	return deletedCount, nil
}

// recordCostCleanupMetrics records cost cleanup job metrics
func (ps *PricingScheduler) recordCostCleanupMetrics(deletedRecords, deploymentCount, errorCount int, duration time.Duration) error {
	log.Printf("Cost cleanup metrics - Deleted: %d records, Deployments: %d, Errors: %d, Duration: %v", 
		deletedRecords, deploymentCount, errorCount, duration)
	
	// TODO: Store cleanup metrics for monitoring dashboard
	// This could be implemented as a separate collection: jobExecutions
	/*
	collection, err := ps.app.Dao().FindCollectionByNameOrId("jobExecutions")
	if err != nil {
		return fmt.Errorf("failed to find jobExecutions collection: %w", err)
	}
	
	record := models.NewRecord(collection)
	record.Set("jobName", "daily-cost-cleanup")
	record.Set("status", "success")
	record.Set("duration", duration.Seconds())
	record.Set("executedAt", time.Now())
	record.Set("metadata", map[string]any{
		"deletedRecords": deletedRecords,
		"deploymentCount": deploymentCount,
		"errorCount": errorCount,
	})
	
	return ps.app.Dao().SaveRecord(record)
	*/
	
	return nil
}

// CleanupOldCostData removes cost data for deployments that have been destroyed for more than X days
// This is useful for compliance and data retention policies
func (ps *PricingScheduler) CleanupOldCostData(retentionDays int) error {
	log.Printf("Starting cleanup of cost data older than %d days...", retentionDays)
	
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)
	
	// Find deployments that were destroyed before the cutoff date
	deployments, err := ps.app.Dao().FindRecordsByFilter(
		"deployments",
		"(status = 'destroyed' || status = 'failed') && updated < {:cutoff}",
		"-updated",
		0,
		0,
		map[string]any{
			"cutoff": cutoffDate,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to find old destroyed deployments: %w", err)
	}
	
	if len(deployments) == 0 {
		log.Printf("No old destroyed deployments found (older than %d days)", retentionDays)
		return nil
	}
	
	log.Printf("Found %d old destroyed deployments to clean up", len(deployments))
	
	totalDeleted := 0
	for _, deployment := range deployments {
		deleted, err := ps.cleanupDeploymentCostData(deployment.Id)
		if err != nil {
			log.Printf("Failed to cleanup cost data for deployment %s: %v", deployment.Id, err)
			continue
		}
		totalDeleted += deleted
	}
	
	log.Printf("Cleaned up %d cost records from %d old deployments", totalDeleted, len(deployments))
	return nil
}

// GetCostDataStats returns statistics about cost data storage
func (ps *PricingScheduler) GetCostDataStats() (map[string]int, error) {
	stats := make(map[string]int)
	
	// Count records in each collection
	collections := []string{"actualCosts", "costEstimates", "costAlerts"}
	
	for _, collectionName := range collections {
		collection, err := ps.app.Dao().FindCollectionByNameOrId(collectionName)
		if err != nil {
			log.Printf("Collection %s not found: %v", collectionName, err)
			stats[collectionName] = 0
			continue
		}
		
		records, err := ps.app.Dao().FindRecordsByFilter(
			collection.Id,
			"",
			"",
			0,
			0,
			nil,
		)
		if err != nil {
			log.Printf("Failed to count records in %s: %v", collectionName, err)
			stats[collectionName] = 0
			continue
		}
		
		stats[collectionName] = len(records)
	}
	
	// Count destroyed deployments with cost data
	destroyedWithCosts := 0
	deployments, err := ps.app.Dao().FindRecordsByFilter(
		"deployments",
		"status = 'destroyed' || status = 'failed'",
		"",
		0,
		0,
		nil,
	)
	if err == nil {
		for _, deployment := range deployments {
			// Check if this deployment has any cost data
			hasCostData, err := ps.deploymentHasCostData(deployment.Id)
			if err == nil && hasCostData {
				destroyedWithCosts++
			}
		}
	}
	
	stats["destroyedDeploymentsWithCostData"] = destroyedWithCosts
	
	return stats, nil
}

// deploymentHasCostData checks if a deployment has any cost-related data
func (ps *PricingScheduler) deploymentHasCostData(deploymentID string) (bool, error) {
	// Check actualCosts
	actualCost, err := ps.app.Dao().FindFirstRecordByFilter(
		"actualCosts",
		"deployment = {:deploymentId}",
		map[string]any{
			"deploymentId": deploymentID,
		},
	)
	if err == nil && actualCost != nil {
		return true, nil
	}
	
	// Check costEstimates
	estimate, err := ps.app.Dao().FindFirstRecordByFilter(
		"costEstimates",
		"deployment = {:deploymentId}",
		map[string]any{
			"deploymentId": deploymentID,
		},
	)
	if err == nil && estimate != nil {
		return true, nil
	}
	
	// Check costAlerts
	alert, err := ps.app.Dao().FindFirstRecordByFilter(
		"costAlerts",
		"deployment = {:deploymentId}",
		map[string]any{
			"deploymentId": deploymentID,
		},
	)
	if err == nil && alert != nil {
		return true, nil
	}
	
	return false, nil
}

// Helper function to safely get deployment status
func getDeploymentStatus(deployment *models.Record) string {
	if deployment == nil {
		return ""
	}
	return deployment.GetString("status")
}
