package jobs

import (
	"log"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/intelligence"
)

// createAIOptimizerJob creates the weekly AI cost optimization job.
// Runs every Monday at 6 AM UTC, analyzes 30 days of cost data per user,
// and stores structured recommendations in the DB.
func (ps *PricingScheduler) createAIOptimizerJob() func() error {
	return func() error {
		log.Println("[AIOptimizer] Starting weekly AI cost optimization job...")

		optimizer := intelligence.NewCostOptimizer(ps.app)
		if err := optimizer.RunOptimizationJob(); err != nil {
			log.Printf("[AIOptimizer] Job failed: %v", err)
			return err
		}

		log.Println("[AIOptimizer] Weekly AI cost optimization job completed")
		return nil
	}
}
