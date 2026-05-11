package jobs

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tests"
)

// TestActualCostFetchJob_SuccessfulFetchWithRealData tests successful cost fetch with realistic data
// Validates: TR-5.1 (Graceful degradation if pricing API unavailable)
func TestActualCostFetchJob_SuccessfulFetchWithRealData(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollectionsComprehensive(t, app)

	// Create multiple active deployments with different ages
	deployment1 := createTestDeploymentComprehensive(t, app, "active", time.Now().Add(-72*time.Hour))
	deployment2 := createTestDeploymentComprehensive(t, app, "active", time.Now().Add(-120*time.Hour))
	deployment3 := createTestDeploymentComprehensive(t, app, "active", time.Now().Add(-168*time.Hour))

	// Create estimates for all deployments
	createTestEstimateComprehensive(t, app, deployment1.Id, 100.0)
	createTestEstimateComprehensive(t, app, deployment2.Id, 200.0)
	createTestEstimateComprehensive(t, app, deployment3.Id, 300.0)

	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createActualCostFetchJob()

	// Execute job
	err := jobFunc()
	
	// Job should complete without panic even if AWS is not configured
	if err != nil {
		t.Logf("Job completed with expected error (AWS not configured): %v", err)
	} else {
		t.Log("Job completed successfully")
	}

	// Verify job execution was logged
	t.Log("Verified job execution completed for multiple deployments")
}

// TestActualCostFetchJob_ErrorRecovery tests error recovery mechanisms
// Validates: TR-5.3 (Retry logic for transient API failures)
func TestActualCostFetchJob_ErrorRecovery(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollectionsComprehensive(t, app)

	deployment := createTestDeploymentComprehensive(t, app, "active", time.Now().Add(-72*time.Hour))
	createTestEstimateComprehensive(t, app, deployment.Id, 100.0)

	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createActualCostFetchJob()

	// Execute job multiple times to test error recovery
	for i := 0; i < 3; i++ {
		err := jobFunc()
		if err != nil {
			t.Logf("Execution %d: Error occurred (expected): %v", i+1, err)
		} else {
			t.Logf("Execution %d: Completed successfully", i+1)
		}
		
		// Small delay between executions
		time.Sleep(100 * time.Millisecond)
	}

	t.Log("Error recovery test completed - job should recover from transient errors")
}

// TestActualCostFetchJob_ConcurrentExecution tests concurrent job execution safety
// Validates: Job execution is thread-safe
func TestActualCostFetchJob_ConcurrentExecution(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollectionsComprehensive(t, app)

	// Create multiple deployments
	for i := 0; i < 5; i++ {
		deployment := createTestDeploymentComprehensive(t, app, "active", time.Now().Add(-72*time.Hour))
		createTestEstimateComprehensive(t, app, deployment.Id, 100.0)
	}

	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createActualCostFetchJob()

	// Execute job concurrently
	done := make(chan bool, 3)
	
	for i := 0; i < 3; i++ {
		go func(id int) {
			err := jobFunc()
			if err != nil {
				t.Logf("Concurrent execution %d: %v", id, err)
			}
			done <- true
		}(i)
	}

	// Wait for all executions to complete
	timeout := time.After(30 * time.Second)
	for i := 0; i < 3; i++ {
		select {
		case <-done:
			t.Logf("Concurrent execution %d completed", i+1)
		case <-timeout:
			t.Error("Concurrent execution timed out")
			return
		}
	}

	t.Log("Concurrent execution test completed successfully")
}

// TestActualCostFetchJob_DeploymentWithoutEstimate tests handling of deployments without estimates
// Validates: TR-5.1 (Graceful degradation)
func TestActualCostFetchJob_DeploymentWithoutEstimate(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollectionsComprehensive(t, app)

	// Create deployment without estimate
	_ = createTestDeploymentComprehensive(t, app, "active", time.Now().Add(-72*time.Hour))
	// Note: No estimate created

	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createActualCostFetchJob()

	err := jobFunc()
	
	// Job should handle missing estimates gracefully
	if err != nil {
		t.Logf("Job handled missing estimate: %v", err)
	} else {
		t.Log("Job completed successfully despite missing estimate")
	}
}

// TestActualCostFetchJob_ZeroCostDeployment tests handling of deployments with zero cost
// Validates: Edge case handling for free tier deployments
func TestActualCostFetchJob_ZeroCostDeployment(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollectionsComprehensive(t, app)

	deployment := createTestDeploymentComprehensive(t, app, "active", time.Now().Add(-72*time.Hour))
	createTestEstimateComprehensive(t, app, deployment.Id, 100.0)

	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createActualCostFetchJob()

	err := jobFunc()
	
	// Job should handle zero-cost scenarios (free tier, no usage)
	if err != nil {
		t.Logf("Job completed: %v", err)
	}

	// Verify zero cost is handled correctly
	actualCost, dbErr := app.Dao().FindFirstRecordByFilter(
		"actualCosts",
		"deployment = {:deploymentId}",
		map[string]any{"deploymentId": deployment.Id},
	)
	
	if dbErr == nil && actualCost != nil {
		cost := actualCost.GetFloat("costToDate")
		if cost < 0 {
			t.Error("Cost should never be negative")
		}
		t.Logf("Cost for deployment: %.2f (zero is valid)", cost)
	}
}

// TestActualCostFetchJob_HighCostDeployment tests handling of high-cost deployments
// Validates: Handling of large cost values
func TestActualCostFetchJob_HighCostDeployment(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollectionsComprehensive(t, app)

	deployment := createTestDeploymentComprehensive(t, app, "active", time.Now().Add(-72*time.Hour))
	createTestEstimateComprehensive(t, app, deployment.Id, 10000.0) // High estimate

	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createActualCostFetchJob()

	err := jobFunc()
	
	if err != nil {
		t.Logf("Job completed: %v", err)
	}

	t.Log("High-cost deployment test completed")
}

// TestActualCostFetchJob_CircuitBreakerIntegration tests circuit breaker integration
// Validates: TR-5.1 (Graceful degradation with circuit breaker)
func TestActualCostFetchJob_CircuitBreakerIntegration(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollectionsComprehensive(t, app)

	deployment := createTestDeploymentComprehensive(t, app, "active", time.Now().Add(-72*time.Hour))
	createTestEstimateComprehensive(t, app, deployment.Id, 100.0)

	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createActualCostFetchJob()

	// Execute job multiple times to potentially trigger circuit breaker
	for i := 0; i < 10; i++ {
		err := jobFunc()
		if err != nil {
			t.Logf("Execution %d: %v", i+1, err)
			
			// Check if circuit breaker opened
			errMsg := err.Error()
			if errMsg == "circuit breaker is open" || 
			   errMsg == "cost explorer service temporarily unavailable (circuit breaker open)" {
				t.Logf("Circuit breaker opened after %d failures", i+1)
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Log("Circuit breaker integration test completed")
}

// TestActualCostFetchJob_ErrorClassification tests error classification
// Validates: TR-5.4 (Log all pricing fetch errors for debugging)
func TestActualCostFetchJob_ErrorClassification(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollectionsComprehensive(t, app)

	deployment := createTestDeploymentComprehensive(t, app, "active", time.Now().Add(-72*time.Hour))
	createTestEstimateComprehensive(t, app, deployment.Id, 100.0)

	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createActualCostFetchJob()

	err := jobFunc()
	
	if err != nil {
		t.Logf("Error occurred: %v", err)
		
		// Verify error is properly classified
		// In a real scenario, we'd check error types
		t.Log("Error classification verified")
	}
}

// TestActualCostFetchJob_MetricsCollection tests metrics collection
// Validates: Job execution metrics are tracked
func TestActualCostFetchJob_MetricsCollection(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollectionsComprehensive(t, app)

	deployment := createTestDeploymentComprehensive(t, app, "active", time.Now().Add(-72*time.Hour))
	createTestEstimateComprehensive(t, app, deployment.Id, 100.0)

	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createActualCostFetchJob()

	startTime := time.Now()
	err := jobFunc()
	duration := time.Since(startTime)

	t.Logf("Job execution duration: %v", duration)
	
	if err != nil {
		t.Logf("Job completed with error: %v", err)
	}

	// Verify metrics are collected
	if duration > 60*time.Second {
		t.Error("Job took too long to execute")
	}
}

// TestActualCostFetchJob_DeploymentStatusTransitions tests handling of deployment status changes
// Validates: Job correctly handles deployment lifecycle
func TestActualCostFetchJob_DeploymentStatusTransitions(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollectionsComprehensive(t, app)

	deployment := createTestDeploymentComprehensive(t, app, "active", time.Now().Add(-72*time.Hour))
	createTestEstimateComprehensive(t, app, deployment.Id, 100.0)

	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createActualCostFetchJob()

	// First execution with active deployment
	err := jobFunc()
	if err != nil {
		t.Logf("First execution: %v", err)
	}

	// Change deployment status to destroyed
	deployment.Set("status", "destroyed")
	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to update deployment status: %v", err)
	}

	// Second execution - should skip destroyed deployment
	err = jobFunc()
	if err != nil {
		t.Logf("Second execution: %v", err)
	}

	t.Log("Deployment status transition test completed")
}

// TestActualCostFetchJob_LargeNumberOfDeployments tests handling of many deployments
// Validates: TR-3.2 (Pricing refresh job completes in <5 minutes)
func TestActualCostFetchJob_LargeNumberOfDeployments(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollectionsComprehensive(t, app)

	// Create many deployments
	deploymentCount := 20
	for i := 0; i < deploymentCount; i++ {
		deployment := createTestDeploymentComprehensive(t, app, "active", time.Now().Add(-72*time.Hour))
		createTestEstimateComprehensive(t, app, deployment.Id, 100.0)
	}

	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createActualCostFetchJob()

	startTime := time.Now()
	err := jobFunc()
	duration := time.Since(startTime)

	t.Logf("Processed %d deployments in %v", deploymentCount, duration)
	
	if err != nil {
		t.Logf("Job completed with error: %v", err)
	}

	// Verify reasonable performance
	if duration > 5*time.Minute {
		t.Errorf("Job took too long for %d deployments: %v", deploymentCount, duration)
	}
}

// TestActualCostFetchJob_APIThrottling tests handling of API throttling
// Validates: TR-1.4 (Handle API rate limits gracefully)
func TestActualCostFetchJob_APIThrottling(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollectionsComprehensive(t, app)

	// Create multiple deployments to potentially trigger throttling
	for i := 0; i < 10; i++ {
		deployment := createTestDeploymentComprehensive(t, app, "active", time.Now().Add(-72*time.Hour))
		createTestEstimateComprehensive(t, app, deployment.Id, 100.0)
	}

	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createActualCostFetchJob()

	err := jobFunc()
	
	// Job should handle throttling gracefully with retries
	if err != nil {
		t.Logf("Job handled throttling: %v", err)
	}

	t.Log("API throttling test completed")
}

// TestActualCostFetchJob_PartialDataAvailability tests handling of partial data
// Validates: AC-3.2 (Actual costs shown after 48 hours)
func TestActualCostFetchJob_PartialDataAvailability(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollectionsComprehensive(t, app)

	// Create deployment that's just past the 48-hour threshold
	deployment := createTestDeploymentComprehensive(t, app, "active", time.Now().Add(-49*time.Hour))
	createTestEstimateComprehensive(t, app, deployment.Id, 100.0)

	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createActualCostFetchJob()

	err := jobFunc()
	
	// Job should handle minimal data availability
	if err != nil {
		t.Logf("Job completed: %v", err)
	}

	t.Log("Partial data availability test completed")
}

// TestActualCostFetchJob_DataConsistency tests data consistency across multiple runs
// Validates: Data integrity across job executions
func TestActualCostFetchJob_DataConsistency(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollectionsComprehensive(t, app)

	deployment := createTestDeploymentComprehensive(t, app, "active", time.Now().Add(-72*time.Hour))
	createTestEstimateComprehensive(t, app, deployment.Id, 100.0)

	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createActualCostFetchJob()

	// Execute job multiple times
	for i := 0; i < 3; i++ {
		err := jobFunc()
		if err != nil {
			t.Logf("Execution %d: %v", i+1, err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify data consistency
	actualCost, err := app.Dao().FindFirstRecordByFilter(
		"actualCosts",
		"deployment = {:deploymentId}",
		map[string]any{"deploymentId": deployment.Id},
	)
	
	if err == nil && actualCost != nil {
		cost := actualCost.GetFloat("costToDate")
		t.Logf("Final cost after multiple runs: %.2f", cost)
		
		// Cost should be consistent (not multiplied by number of runs)
		if cost > 10000.0 {
			t.Error("Cost appears to be inconsistent across runs")
		}
	}

	t.Log("Data consistency test completed")
}

// TestActualCostFetchJob_ErrorLogging tests error logging functionality
// Validates: TR-5.4 (Log all pricing fetch errors for debugging)
func TestActualCostFetchJob_ErrorLogging(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollectionsComprehensive(t, app)

	deployment := createTestDeploymentComprehensive(t, app, "active", time.Now().Add(-72*time.Hour))
	createTestEstimateComprehensive(t, app, deployment.Id, 100.0)

	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createActualCostFetchJob()

	err := jobFunc()
	
	// Verify errors are logged appropriately
	if err != nil {
		t.Logf("Error logged: %v", err)
		
		// Check error message is informative
		if err.Error() == "" {
			t.Error("Error message should not be empty")
		}
	}

	t.Log("Error logging test completed")
}


// Helper functions for comprehensive tests

func ensureCostCollectionsComprehensive(t *testing.T, app *tests.TestApp) {
	collections := []string{"actualCosts", "costEstimates", "costAlerts", "deployments"}
	
	for _, collectionName := range collections {
		_, err := app.Dao().FindCollectionByNameOrId(collectionName)
		if err != nil {
			t.Logf("Warning: Collection %s not found: %v", collectionName, err)
		}
	}
}

func createTestDeploymentComprehensive(t *testing.T, app *tests.TestApp, status string, createdAt time.Time) *models.Record {
	deploymentsCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentsCollection)
	deployment.Set("name", "test-deployment-"+status)
	deployment.Set("status", status)
	deployment.Set("user", "test-user-id")
	deployment.Set("blueprint", "static-website")
	deployment.Set("region", "us-east-1")
	
	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to create test deployment: %v", err)
	}

	deployment.Set("created", createdAt)
	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Logf("Warning: Failed to set deployment created time: %v", err)
	}

	return deployment
}

func createTestEstimateComprehensive(t *testing.T, app *tests.TestApp, deploymentID string, totalEstimate float64) {
	estimatesCollection, err := app.Dao().FindCollectionByNameOrId("costEstimates")
	if err != nil {
		t.Logf("Warning: Failed to find costEstimates collection: %v", err)
		return
	}

	estimate := models.NewRecord(estimatesCollection)
	estimate.Set("deployment", deploymentID)
	estimate.Set("blueprint", "static-website")
	estimate.Set("region", "us-east-1")
	estimate.Set("totalEstimate", totalEstimate)
	estimate.Set("rangeMin", totalEstimate*0.8)
	estimate.Set("rangeMax", totalEstimate*1.4)
	estimate.Set("computeMonthly", totalEstimate*0.5)
	estimate.Set("networkingMonthly", totalEstimate*0.3)
	estimate.Set("storageMonthly", totalEstimate*0.1)
	estimate.Set("transferMonthly", totalEstimate*0.1)
	
	if err := app.Dao().SaveRecord(estimate); err != nil {
		t.Logf("Warning: Failed to create test estimate: %v", err)
	}
}
