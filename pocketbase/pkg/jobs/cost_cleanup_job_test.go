package jobs

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tests"
)

func TestCostCleanupJob_Execution(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	scheduler := NewPricingScheduler(app)

	// Create the cost cleanup job function
	jobFunc := scheduler.createCostCleanupJob()

	// Execute the job
	err := jobFunc()
	
	// Job should not fail even if there are no destroyed deployments
	if err != nil {
		t.Fatalf("Job execution failed: %v", err)
	}
}

func TestCostCleanupJob_WithDestroyedDeployment(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// Create test collections if they don't exist
	ensureCostCollections(t, app)

	// Create a destroyed deployment
	deploymentsCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentsCollection)
	deployment.Set("name", "test-deployment")
	deployment.Set("status", "destroyed")
	deployment.Set("user", "test-user-id")
	deployment.Set("blueprint", "static-website")
	deployment.Set("region", "us-east-1")
	
	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to create test deployment: %v", err)
	}

	deploymentID := deployment.Id

	// Create cost data for this deployment
	createTestCostData(t, app, deploymentID)

	// Verify cost data exists
	actualCost, err := app.Dao().FindFirstRecordByFilter(
		"actualCosts",
		"deployment = {:deploymentId}",
		map[string]any{"deploymentId": deploymentID},
	)
	if err != nil || actualCost == nil {
		t.Fatal("Test cost data was not created properly")
	}

	// Run cleanup job
	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createCostCleanupJob()
	
	err = jobFunc()
	if err != nil {
		t.Fatalf("Cleanup job failed: %v", err)
	}

	// Verify cost data was deleted
	actualCost, err = app.Dao().FindFirstRecordByFilter(
		"actualCosts",
		"deployment = {:deploymentId}",
		map[string]any{"deploymentId": deploymentID},
	)
	if err == nil && actualCost != nil {
		t.Error("Cost data was not deleted for destroyed deployment")
	}
}

func TestCostCleanupJob_MultipleDeployments(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollections(t, app)

	deploymentsCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	// Create multiple destroyed deployments
	deploymentIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		deployment := models.NewRecord(deploymentsCollection)
		deployment.Set("name", "test-deployment-"+string(rune(i)))
		deployment.Set("status", "destroyed")
		deployment.Set("user", "test-user-id")
		deployment.Set("blueprint", "static-website")
		deployment.Set("region", "us-east-1")
		
		if err := app.Dao().SaveRecord(deployment); err != nil {
			t.Fatalf("Failed to create test deployment %d: %v", i, err)
		}
		
		deploymentIDs[i] = deployment.Id
		createTestCostData(t, app, deployment.Id)
	}

	// Run cleanup job
	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createCostCleanupJob()
	
	err = jobFunc()
	if err != nil {
		t.Fatalf("Cleanup job failed: %v", err)
	}

	// Verify all cost data was deleted
	for i, deploymentID := range deploymentIDs {
		actualCost, err := app.Dao().FindFirstRecordByFilter(
			"actualCosts",
			"deployment = {:deploymentId}",
			map[string]any{"deploymentId": deploymentID},
		)
		if err == nil && actualCost != nil {
			t.Errorf("Cost data was not deleted for deployment %d", i)
		}
	}
}

func TestCostCleanupJob_ActiveDeploymentNotAffected(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollections(t, app)

	deploymentsCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	// Create an active deployment
	activeDeployment := models.NewRecord(deploymentsCollection)
	activeDeployment.Set("name", "active-deployment")
	activeDeployment.Set("status", "active")
	activeDeployment.Set("user", "test-user-id")
	activeDeployment.Set("blueprint", "static-website")
	activeDeployment.Set("region", "us-east-1")
	
	if err := app.Dao().SaveRecord(activeDeployment); err != nil {
		t.Fatalf("Failed to create active deployment: %v", err)
	}

	activeDeploymentID := activeDeployment.Id
	createTestCostData(t, app, activeDeploymentID)

	// Create a destroyed deployment
	destroyedDeployment := models.NewRecord(deploymentsCollection)
	destroyedDeployment.Set("name", "destroyed-deployment")
	destroyedDeployment.Set("status", "destroyed")
	destroyedDeployment.Set("user", "test-user-id")
	destroyedDeployment.Set("blueprint", "static-website")
	destroyedDeployment.Set("region", "us-east-1")
	
	if err := app.Dao().SaveRecord(destroyedDeployment); err != nil {
		t.Fatalf("Failed to create destroyed deployment: %v", err)
	}

	destroyedDeploymentID := destroyedDeployment.Id
	createTestCostData(t, app, destroyedDeploymentID)

	// Run cleanup job
	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createCostCleanupJob()
	
	err = jobFunc()
	if err != nil {
		t.Fatalf("Cleanup job failed: %v", err)
	}

	// Verify active deployment cost data still exists
	activeCost, err := app.Dao().FindFirstRecordByFilter(
		"actualCosts",
		"deployment = {:deploymentId}",
		map[string]any{"deploymentId": activeDeploymentID},
	)
	if err != nil || activeCost == nil {
		t.Error("Active deployment cost data was incorrectly deleted")
	}

	// Verify destroyed deployment cost data was deleted
	destroyedCost, err := app.Dao().FindFirstRecordByFilter(
		"actualCosts",
		"deployment = {:deploymentId}",
		map[string]any{"deploymentId": destroyedDeploymentID},
	)
	if err == nil && destroyedCost != nil {
		t.Error("Destroyed deployment cost data was not deleted")
	}
}

func TestCleanupDeploymentCostData(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollections(t, app)

	deploymentsCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentsCollection)
	deployment.Set("name", "test-deployment")
	deployment.Set("status", "destroyed")
	deployment.Set("user", "test-user-id")
	deployment.Set("blueprint", "static-website")
	deployment.Set("region", "us-east-1")
	
	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to create test deployment: %v", err)
	}

	deploymentID := deployment.Id
	createTestCostData(t, app, deploymentID)

	// Run cleanup for specific deployment
	scheduler := NewPricingScheduler(app)
	deleted, err := scheduler.cleanupDeploymentCostData(deploymentID)
	
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	if deleted == 0 {
		t.Error("Expected some records to be deleted, got 0")
	}

	t.Logf("Deleted %d cost records", deleted)
}

func TestDeleteRecordsByDeployment(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollections(t, app)

	deploymentsCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentsCollection)
	deployment.Set("name", "test-deployment")
	deployment.Set("status", "destroyed")
	deployment.Set("user", "test-user-id")
	deployment.Set("blueprint", "static-website")
	deployment.Set("region", "us-east-1")
	
	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to create test deployment: %v", err)
	}

	deploymentID := deployment.Id

	// Create test data in actualCosts
	actualCostsCollection, err := app.Dao().FindCollectionByNameOrId("actualCosts")
	if err != nil {
		t.Fatalf("Failed to find actualCosts collection: %v", err)
	}

	actualCost := models.NewRecord(actualCostsCollection)
	actualCost.Set("deployment", deploymentID)
	actualCost.Set("costToDate", 50.0)
	actualCost.Set("projectedMonthly", 60.0)
	actualCost.Set("variance", 10.0)
	
	if err := app.Dao().SaveRecord(actualCost); err != nil {
		t.Fatalf("Failed to create test actual cost: %v", err)
	}

	// Delete records
	scheduler := NewPricingScheduler(app)
	deleted, err := scheduler.deleteRecordsByDeployment("actualCosts", deploymentID)
	
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if deleted != 1 {
		t.Errorf("Expected 1 record deleted, got %d", deleted)
	}

	// Verify deletion
	record, err := app.Dao().FindFirstRecordByFilter(
		"actualCosts",
		"deployment = {:deploymentId}",
		map[string]any{"deploymentId": deploymentID},
	)
	if err == nil && record != nil {
		t.Error("Record was not deleted")
	}
}

func TestCleanupOldCostData(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollections(t, app)

	deploymentsCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	// Create an old destroyed deployment
	oldDeployment := models.NewRecord(deploymentsCollection)
	oldDeployment.Set("name", "old-deployment")
	oldDeployment.Set("status", "destroyed")
	oldDeployment.Set("user", "test-user-id")
	oldDeployment.Set("blueprint", "static-website")
	oldDeployment.Set("region", "us-east-1")
	
	if err := app.Dao().SaveRecord(oldDeployment); err != nil {
		t.Fatalf("Failed to create old deployment: %v", err)
	}

	// Manually set updated time to be old (this is a test workaround)
	// In real scenario, the deployment would naturally be old
	oldDeploymentID := oldDeployment.Id
	createTestCostData(t, app, oldDeploymentID)

	// Run cleanup with retention period
	scheduler := NewPricingScheduler(app)
	err = scheduler.CleanupOldCostData(90) // 90 days retention
	
	if err != nil {
		t.Fatalf("Cleanup old cost data failed: %v", err)
	}

	// Note: In this test, the deployment won't actually be old enough
	// This test mainly verifies the function doesn't crash
	t.Log("Cleanup old cost data completed successfully")
}

func TestGetCostDataStats(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollections(t, app)

	scheduler := NewPricingScheduler(app)
	stats, err := scheduler.GetCostDataStats()
	
	if err != nil {
		t.Fatalf("Failed to get cost data stats: %v", err)
	}

	// Verify stats structure
	if _, ok := stats["actualCosts"]; !ok {
		t.Error("Stats missing actualCosts count")
	}
	if _, ok := stats["costEstimates"]; !ok {
		t.Error("Stats missing costEstimates count")
	}
	if _, ok := stats["costAlerts"]; !ok {
		t.Error("Stats missing costAlerts count")
	}
	if _, ok := stats["destroyedDeploymentsWithCostData"]; !ok {
		t.Error("Stats missing destroyedDeploymentsWithCostData count")
	}

	t.Logf("Cost data stats: %+v", stats)
}

func TestDeploymentHasCostData(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollections(t, app)

	deploymentsCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	// Create deployment with cost data
	deploymentWithCost := models.NewRecord(deploymentsCollection)
	deploymentWithCost.Set("name", "deployment-with-cost")
	deploymentWithCost.Set("status", "active")
	deploymentWithCost.Set("user", "test-user-id")
	deploymentWithCost.Set("blueprint", "static-website")
	deploymentWithCost.Set("region", "us-east-1")
	
	if err := app.Dao().SaveRecord(deploymentWithCost); err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	deploymentWithCostID := deploymentWithCost.Id
	createTestCostData(t, app, deploymentWithCostID)

	// Create deployment without cost data
	deploymentWithoutCost := models.NewRecord(deploymentsCollection)
	deploymentWithoutCost.Set("name", "deployment-without-cost")
	deploymentWithoutCost.Set("status", "active")
	deploymentWithoutCost.Set("user", "test-user-id")
	deploymentWithoutCost.Set("blueprint", "static-website")
	deploymentWithoutCost.Set("region", "us-east-1")
	
	if err := app.Dao().SaveRecord(deploymentWithoutCost); err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	deploymentWithoutCostID := deploymentWithoutCost.Id

	scheduler := NewPricingScheduler(app)

	// Test deployment with cost data
	hasCost, err := scheduler.deploymentHasCostData(deploymentWithCostID)
	if err != nil {
		t.Fatalf("Failed to check cost data: %v", err)
	}
	if !hasCost {
		t.Error("Expected deployment to have cost data")
	}

	// Test deployment without cost data
	hasCost, err = scheduler.deploymentHasCostData(deploymentWithoutCostID)
	if err != nil {
		t.Fatalf("Failed to check cost data: %v", err)
	}
	if hasCost {
		t.Error("Expected deployment to not have cost data")
	}
}

func TestCostCleanupJob_Scheduling(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	scheduler := NewPricingScheduler(app)

	// Start the scheduler
	err := scheduler.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}
	defer scheduler.Stop()

	// Check if the cost cleanup job was scheduled
	jobs := scheduler.GetScheduledJobs()
	found := false
	for _, jobName := range jobs {
		if jobName == "daily-cost-cleanup" {
			found = true
			break
		}
	}

	if !found {
		t.Error("daily-cost-cleanup job was not scheduled")
	}
}

func TestCostCleanupJob_FailedDeployments(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollections(t, app)

	deploymentsCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	// Create a failed deployment
	failedDeployment := models.NewRecord(deploymentsCollection)
	failedDeployment.Set("name", "failed-deployment")
	failedDeployment.Set("status", "failed")
	failedDeployment.Set("user", "test-user-id")
	failedDeployment.Set("blueprint", "static-website")
	failedDeployment.Set("region", "us-east-1")
	
	if err := app.Dao().SaveRecord(failedDeployment); err != nil {
		t.Fatalf("Failed to create failed deployment: %v", err)
	}

	failedDeploymentID := failedDeployment.Id
	createTestCostData(t, app, failedDeploymentID)

	// Run cleanup job
	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createCostCleanupJob()
	
	err = jobFunc()
	if err != nil {
		t.Fatalf("Cleanup job failed: %v", err)
	}

	// Verify cost data was deleted for failed deployment
	actualCost, err := app.Dao().FindFirstRecordByFilter(
		"actualCosts",
		"deployment = {:deploymentId}",
		map[string]any{"deploymentId": failedDeploymentID},
	)
	if err == nil && actualCost != nil {
		t.Error("Cost data was not deleted for failed deployment")
	}
}

// Helper functions

func ensureCostCollections(t *testing.T, app *tests.TestApp) {
	// These collections should already exist from migrations
	// This function just verifies they exist
	collections := []string{"actualCosts", "costEstimates", "costAlerts", "deployments"}
	
	for _, collectionName := range collections {
		_, err := app.Dao().FindCollectionByNameOrId(collectionName)
		if err != nil {
			t.Logf("Warning: Collection %s not found: %v", collectionName, err)
		}
	}
}

func createTestCostData(t *testing.T, app *tests.TestApp, deploymentID string) {
	// Create actualCosts record
	actualCostsCollection, err := app.Dao().FindCollectionByNameOrId("actualCosts")
	if err == nil {
		actualCost := models.NewRecord(actualCostsCollection)
		actualCost.Set("deployment", deploymentID)
		actualCost.Set("costToDate", 50.0)
		actualCost.Set("projectedMonthly", 60.0)
		actualCost.Set("variance", 10.0)
		actualCost.Set("breakdown", map[string]interface{}{
			"AmazonS3": 10.0,
			"AmazonEC2": 40.0,
		})
		actualCost.Set("periodStart", time.Now().AddDate(0, 0, -7))
		actualCost.Set("periodEnd", time.Now())
		actualCost.Set("fetchedAt", time.Now())
		
		if err := app.Dao().SaveRecord(actualCost); err != nil {
			t.Logf("Warning: Failed to create test actual cost: %v", err)
		}
	}

	// Create costEstimates record
	estimatesCollection, err := app.Dao().FindCollectionByNameOrId("costEstimates")
	if err == nil {
		estimate := models.NewRecord(estimatesCollection)
		estimate.Set("deployment", deploymentID)
		estimate.Set("blueprint", "static-website")
		estimate.Set("region", "us-east-1")
		estimate.Set("totalEstimate", 55.0)
		estimate.Set("rangeMin", 44.0)
		estimate.Set("rangeMax", 77.0)
		estimate.Set("computeMonthly", 30.0)
		estimate.Set("networkingMonthly", 15.0)
		estimate.Set("storageMonthly", 5.0)
		estimate.Set("transferMonthly", 5.0)
		
		if err := app.Dao().SaveRecord(estimate); err != nil {
			t.Logf("Warning: Failed to create test estimate: %v", err)
		}
	}

	// Create costAlerts record
	alertsCollection, err := app.Dao().FindCollectionByNameOrId("costAlerts")
	if err == nil {
		alert := models.NewRecord(alertsCollection)
		alert.Set("deployment", deploymentID)
		alert.Set("user", "test-user-id")
		alert.Set("type", "cost_overrun")
		alert.Set("threshold", 20.0)
		alert.Set("triggered", true)
		alert.Set("actualCost", 62.0)
		alert.Set("estimatedCost", 55.0)
		alert.Set("variance", 12.7)
		alert.Set("message", "Test alert")
		alert.Set("acknowledged", false)
		
		if err := app.Dao().SaveRecord(alert); err != nil {
			t.Logf("Warning: Failed to create test alert: %v", err)
		}
	}
}

// Benchmark tests

func BenchmarkCostCleanupJob_Creation(b *testing.B) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	scheduler := NewPricingScheduler(app)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = scheduler.createCostCleanupJob()
	}
}

func BenchmarkCleanupDeploymentCostData(b *testing.B) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ensureCostCollections(nil, app)

	deploymentsCollection, _ := app.Dao().FindCollectionByNameOrId("deployments")
	deployment := models.NewRecord(deploymentsCollection)
	deployment.Set("name", "bench-deployment")
	deployment.Set("status", "destroyed")
	deployment.Set("user", "test-user-id")
	deployment.Set("blueprint", "static-website")
	deployment.Set("region", "us-east-1")
	app.Dao().SaveRecord(deployment)

	deploymentID := deployment.Id
	createTestCostData(nil, app, deploymentID)

	scheduler := NewPricingScheduler(app)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scheduler.cleanupDeploymentCostData(deploymentID)
		// Recreate data for next iteration
		createTestCostData(nil, app, deploymentID)
	}
}
