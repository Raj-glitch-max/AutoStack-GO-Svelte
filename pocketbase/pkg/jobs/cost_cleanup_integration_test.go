package jobs

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tests"
)

// TestCostCleanupJob_Integration tests the full cleanup workflow with real database
func TestCostCleanupJob_Integration(t *testing.T) {
	// Skip if running in CI without database setup
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// Verify collections exist
	collections := []string{"deployments", "actualCosts", "costEstimates", "costAlerts"}
	for _, collectionName := range collections {
		_, err := app.Dao().FindCollectionByNameOrId(collectionName)
		if err != nil {
			t.Skipf("Skipping integration test: collection %s not found", collectionName)
		}
	}

	// Create a destroyed deployment
	deploymentsCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentsCollection)
	deployment.Set("name", "integration-test-deployment")
	deployment.Set("status", "destroyed")
	deployment.Set("user", "test-user-id")
	deployment.Set("blueprint", "static-website")
	deployment.Set("region", "us-east-1")
	
	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to create test deployment: %v", err)
	}

	deploymentID := deployment.Id
	t.Logf("Created test deployment: %s", deploymentID)

	// Create cost data
	actualCostsCollection, err := app.Dao().FindCollectionByNameOrId("actualCosts")
	if err != nil {
		t.Fatalf("Failed to find actualCosts collection: %v", err)
	}

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
		t.Fatalf("Failed to create test actual cost: %v", err)
	}

	t.Log("Created test cost data")

	// Verify cost data exists before cleanup
	record, err := app.Dao().FindFirstRecordByFilter(
		"actualCosts",
		"deployment = {:deploymentId}",
		map[string]any{"deploymentId": deploymentID},
	)
	if err != nil || record == nil {
		t.Fatal("Test cost data was not created properly")
	}

	// Run cleanup job
	scheduler := NewPricingScheduler(app)
	jobFunc := scheduler.createCostCleanupJob()
	
	err = jobFunc()
	if err != nil {
		t.Fatalf("Cleanup job failed: %v", err)
	}

	t.Log("Cleanup job completed")

	// Verify cost data was deleted
	record, err = app.Dao().FindFirstRecordByFilter(
		"actualCosts",
		"deployment = {:deploymentId}",
		map[string]any{"deploymentId": deploymentID},
	)
	if err == nil && record != nil {
		t.Error("Cost data was not deleted for destroyed deployment")
	}

	t.Log("Integration test completed successfully")
}

// TestCostCleanupJob_ManualTrigger tests manual triggering of the cleanup job
func TestCostCleanupJob_ManualTrigger(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	manager := NewPricingJobManager(app)

	// Start the job manager
	err := manager.Start()
	if err != nil {
		t.Fatalf("Failed to start job manager: %v", err)
	}
	defer manager.Stop()

	// Manually trigger the cost cleanup job
	err = manager.TriggerCostCleanup()
	if err != nil {
		t.Fatalf("Failed to trigger cost cleanup: %v", err)
	}

	// Wait a bit for job to start executing
	time.Sleep(100 * time.Millisecond)

	// Job should have been triggered (we can't verify completion in test environment)
	t.Log("Cost cleanup job triggered successfully")
}

// TestCostCleanupJob_Stats tests the cost data statistics function
func TestCostCleanupJob_Stats(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	scheduler := NewPricingScheduler(app)
	
	stats, err := scheduler.GetCostDataStats()
	if err != nil {
		t.Fatalf("Failed to get cost data stats: %v", err)
	}

	// Verify stats structure
	expectedKeys := []string{"actualCosts", "costEstimates", "costAlerts", "destroyedDeploymentsWithCostData"}
	for _, key := range expectedKeys {
		if _, ok := stats[key]; !ok {
			t.Errorf("Stats missing key: %s", key)
		}
	}

	t.Logf("Cost data stats: %+v", stats)
}
