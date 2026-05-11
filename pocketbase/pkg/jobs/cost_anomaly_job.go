package jobs

import (
	"fmt"
	"log"
	"time"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/cost"
)

// createCostAnomalyDetectionJob creates the daily cost anomaly detection job
// Validates: AC-4.1 (Alert triggered when actual cost exceeds estimate by 20%)
// Validates: AC-4.2 (Alert sent via email and in-app notification)
// Validates: AC-4.3 (Alert includes breakdown of which services exceeded budget)
func (ps *PricingScheduler) createCostAnomalyDetectionJob() func() error {
	return func() error {
		log.Println("Starting cost anomaly detection job...")
		
		startTime := time.Now()
		
		// Create cost monitor
		monitor, err := cost.NewCostMonitor(ps.app)
		if err != nil {
			return fmt.Errorf("failed to create cost monitor: %w", err)
		}
		
		// Check for cost anomalies across all active deployments
		err = monitor.CheckCostAnomalies()
		if err != nil {
			log.Printf("Cost anomaly detection completed with errors: %v", err)
			// Don't fail completely - partial detection is better than none
		}
		
		duration := time.Since(startTime)
		log.Printf("Cost anomaly detection job completed in %v", duration)
		
		// Update job completion metrics
		if err := ps.recordJobCompletion("daily-cost-anomaly-detection", duration, err); err != nil {
			log.Printf("Failed to record job completion: %v", err)
		}
		
		return nil
	}
}
