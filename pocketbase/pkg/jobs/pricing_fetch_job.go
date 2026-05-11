package jobs

import (
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
)

// createPricingFetchJob creates the daily pricing fetch job function
func (ps *PricingScheduler) createPricingFetchJob() func() error {
	return func() error {
		log.Println("Starting AWS pricing data fetch job...")
		
		// Create pricing fetcher
		fetcher, err := aws.NewPricingFetcher(ps.app)
		if err != nil {
			return fmt.Errorf("failed to create pricing fetcher: %w", err)
		}
		
		// Record job start time
		startTime := time.Now()
		
		// Fetch all pricing data
		err = fetcher.FetchAllPricing()
		if err != nil {
			// Log error but don't fail completely - partial data is better than none
			log.Printf("Pricing fetch completed with errors: %v", err)
			
			// Check if we have any recent pricing data
			isStale, checkErr := fetcher.IsDataStale(48 * time.Hour)
			if checkErr != nil || isStale {
				// No recent data and fetch failed - this is a critical error
				return fmt.Errorf("pricing fetch failed and no recent data available: %w", err)
			}
			
			// We have recent data, so partial failure is acceptable
			log.Println("Pricing fetch had errors but recent data is available")
		}
		
		duration := time.Since(startTime)
		log.Printf("Pricing fetch job completed in %v", duration)
		
		// Update job completion metrics
		if err := ps.recordJobCompletion("daily-pricing-fetch", duration, err); err != nil {
			log.Printf("Failed to record job completion: %v", err)
		}
		
		return nil
	}
}

// createActualCostFetchJob creates the daily actual cost fetch job
func (ps *PricingScheduler) createActualCostFetchJob() func() error {
	return func() error {
		log.Println("Starting actual cost fetch job for active deployments...")
		
		// Create actual cost fetcher
		fetcher, err := aws.NewActualCostFetcher(ps.app)
		if err != nil {
			return fmt.Errorf("failed to create actual cost fetcher: %w", err)
		}
		
		// Record job start time
		startTime := time.Now()
		
		// Fetch actual costs for all active deployments
		err = fetcher.FetchActualCostsForActiveDeployments()
		if err != nil {
			// Log error but don't fail completely - partial data is better than none
			log.Printf("Actual cost fetch completed with errors: %v", err)
		}
		
		duration := time.Since(startTime)
		log.Printf("Actual cost fetch job completed in %v", duration)
		
		// Update job completion metrics
		if err := ps.recordJobCompletion("daily-actual-cost-fetch", duration, err); err != nil {
			log.Printf("Failed to record job completion: %v", err)
		}
		
		return nil
	}
}

// createHealthCheckJob creates the pricing cache health check job
func (ps *PricingScheduler) createHealthCheckJob() func() error {
	return func() error {
		log.Println("Starting pricing cache health check...")
		
		// Create pricing fetcher for health checks
		fetcher, err := aws.NewPricingFetcher(ps.app)
		if err != nil {
			return fmt.Errorf("failed to create pricing fetcher: %w", err)
		}
		
		// Check if pricing data is stale
		isStale, err := fetcher.IsDataStale(24 * time.Hour)
		if err != nil {
			log.Printf("Health check warning: %v", err)
			return nil // Don't fail on health check errors
		}
		
		if isStale {
			log.Println("Health check warning: Pricing data is stale (>24 hours old)")
			
			// Check if data is critically stale (>48 hours)
			isCriticallyStale, err := fetcher.IsDataStale(48 * time.Hour)
			if err == nil && isCriticallyStale {
				log.Println("Health check critical: Pricing data is critically stale (>48 hours old)")
				
				// TODO: Send alert to administrators
				if err := ps.sendStaleDataAlert(); err != nil {
					log.Printf("Failed to send stale data alert: %v", err)
				}
			}
		} else {
			log.Println("Health check passed: Pricing data is fresh")
		}
		
		// Check database connectivity and basic queries
		if err := ps.checkDatabaseHealth(); err != nil {
			log.Printf("Health check warning: Database issues detected: %v", err)
		}
		
		return nil
	}
}

// createCleanupJob creates the weekly pricing data cleanup job
func (ps *PricingScheduler) createCleanupJob() func() error {
	return func() error {
		log.Println("Starting pricing data cleanup job...")
		
		startTime := time.Now()
		
		// Clean up pricing data older than 90 days
		cutoffDate := time.Now().AddDate(0, 0, -90)
		
		collection, err := ps.app.Dao().FindCollectionByNameOrId("awsPricingCache")
		if err != nil {
			return fmt.Errorf("failed to find awsPricingCache collection: %w", err)
		}
		
		// Find old records
		records, err := ps.app.Dao().FindRecordsByFilter(
			collection.Id,
			"fetchedAt < {:cutoff}",
			"-fetchedAt",
			0,
			0,
			map[string]any{"cutoff": cutoffDate},
		)
		if err != nil {
			return fmt.Errorf("failed to find old pricing records: %w", err)
		}
		
		deletedCount := 0
		for _, record := range records {
			if err := ps.app.Dao().DeleteRecord(record); err != nil {
				log.Printf("Failed to delete pricing record %s: %v", record.Id, err)
				continue
			}
			deletedCount++
		}
		
		duration := time.Since(startTime)
		log.Printf("Cleanup job completed: deleted %d old pricing records in %v", deletedCount, duration)
		
		// Record cleanup metrics
		if err := ps.recordCleanupMetrics(deletedCount, duration); err != nil {
			log.Printf("Failed to record cleanup metrics: %v", err)
		}
		
		return nil
	}
}

// recordJobCompletion records job completion metrics
func (ps *PricingScheduler) recordJobCompletion(jobName string, duration time.Duration, jobErr error) error {
	// This would typically store metrics in a database or monitoring system
	// For now, we'll just log the metrics
	
	status := "success"
	if jobErr != nil {
		status = "error"
	}
	
	log.Printf("Job completion recorded - Name: %s, Status: %s, Duration: %v", 
		jobName, status, duration)
	
	// TODO: Store in jobExecutions collection for monitoring dashboard
	/*
	collection, err := ps.app.Dao().FindCollectionByNameOrId("jobExecutions")
	if err != nil {
		return fmt.Errorf("failed to find jobExecutions collection: %w", err)
	}
	
	record := models.NewRecord(collection)
	record.Set("jobName", jobName)
	record.Set("status", status)
	record.Set("duration", duration.Seconds())
	record.Set("executedAt", time.Now())
	if jobErr != nil {
		record.Set("errorMessage", jobErr.Error())
	}
	
	return ps.app.Dao().SaveRecord(record)
	*/
	
	return nil
}

// recordCleanupMetrics records cleanup job metrics
func (ps *PricingScheduler) recordCleanupMetrics(deletedCount int, duration time.Duration) error {
	log.Printf("Cleanup metrics - Deleted: %d records, Duration: %v", deletedCount, duration)
	
	// TODO: Store cleanup metrics for monitoring
	return nil
}

// sendStaleDataAlert sends an alert when pricing data becomes critically stale
func (ps *PricingScheduler) sendStaleDataAlert() error {
	log.Println("Sending stale pricing data alert...")
	
	// TODO: Integrate with notification system
	// This could send emails to administrators or post to Slack
	
	alertMessage := fmt.Sprintf(
		"ALERT: AWS pricing data is critically stale (>48 hours old). " +
		"Cost estimates may be inaccurate. Please check the pricing fetch job.",
	)
	
	log.Printf("Stale data alert: %s", alertMessage)
	
	return nil
}

// checkDatabaseHealth performs basic database health checks
func (ps *PricingScheduler) checkDatabaseHealth() error {
	// Test basic database connectivity
	collection, err := ps.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return fmt.Errorf("cannot access awsPricingCache collection: %w", err)
	}
	
	// Test a simple query
	_, err = ps.app.Dao().FindRecordsByFilter(
		collection.Id,
		"",
		"-fetchedAt",
		1,
		0,
		nil,
	)
	if err != nil {
		return fmt.Errorf("database query failed: %w", err)
	}
	
	return nil
}

// PricingJobManager provides high-level job management functions
type PricingJobManager struct {
	scheduler *PricingScheduler
	app       core.App
}

// NewPricingJobManager creates a new pricing job manager
func NewPricingJobManager(app core.App) *PricingJobManager {
	return &PricingJobManager{
		scheduler: NewPricingScheduler(app),
		app:       app,
	}
}

// Start starts the pricing job manager
func (pjm *PricingJobManager) Start() error {
	return pjm.scheduler.Start()
}

// Stop stops the pricing job manager
func (pjm *PricingJobManager) Stop() error {
	return pjm.scheduler.Stop()
}

// TriggerPricingFetch manually triggers a pricing fetch
func (pjm *PricingJobManager) TriggerPricingFetch() error {
	return pjm.scheduler.TriggerJob("daily-pricing-fetch")
}

// TriggerActualCostFetch manually triggers an actual cost fetch
func (pjm *PricingJobManager) TriggerActualCostFetch() error {
	return pjm.scheduler.TriggerJob("daily-actual-cost-fetch")
}

// TriggerCostCleanup manually triggers a cost data cleanup
func (pjm *PricingJobManager) TriggerCostCleanup() error {
	return pjm.scheduler.TriggerJob("daily-cost-cleanup")
}

// GetJobStatus returns the status of all pricing jobs
func (pjm *PricingJobManager) GetJobStatus() []JobStatus {
	return pjm.scheduler.GetJobStatus()
}

// IsHealthy returns whether the pricing system is healthy
func (pjm *PricingJobManager) IsHealthy() (bool, []string) {
	var issues []string
	
	// Check if scheduler is running
	if !pjm.scheduler.IsRunning() {
		issues = append(issues, "Pricing scheduler is not running")
	}
	
	// Check pricing data freshness
	fetcher, err := aws.NewPricingFetcher(pjm.app)
	if err != nil {
		issues = append(issues, fmt.Sprintf("Cannot create pricing fetcher: %v", err))
		return false, issues
	}
	
	isStale, err := fetcher.IsDataStale(48 * time.Hour)
	if err != nil {
		issues = append(issues, fmt.Sprintf("Cannot check data freshness: %v", err))
	} else if isStale {
		issues = append(issues, "Pricing data is stale (>48 hours old)")
	}
	
	return len(issues) == 0, issues
}

// GetCostDataFreshnessStatus returns the current cost data freshness status
func (pjm *PricingJobManager) GetCostDataFreshnessStatus() (*CostFreshnessMetrics, error) {
	return pjm.scheduler.GetCostDataFreshnessStatus()
}

// GetStaleDeployments returns a list of deployments with stale cost data
func (pjm *PricingJobManager) GetStaleDeployments() ([]StaleDeploymentInfo, error) {
	return pjm.scheduler.GetStaleDeployments()
}

// IsCostDataFresh checks if cost data is fresh for a specific deployment
func (pjm *PricingJobManager) IsCostDataFresh(deploymentID string) (bool, time.Duration, error) {
	return pjm.scheduler.IsCostDataFresh(deploymentID)
}

// TriggerCostFreshnessMonitor manually triggers the cost freshness monitoring job
func (pjm *PricingJobManager) TriggerCostFreshnessMonitor() error {
	return pjm.scheduler.TriggerJob("cost-freshness-monitor")
}
