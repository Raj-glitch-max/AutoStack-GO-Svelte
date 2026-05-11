package jobs

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tests"
)

// TestCostFreshnessMonitor_Integration tests the full freshness monitoring workflow
func TestCostFreshnessMonitor_Integration(t *testing.T) {
	// Skip if running in CI without database setup
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// Verify collections exist
	collections := []string{"deployments", "actualCosts", "costAlerts"}
	for _, collectionName := range collections {
		_, err := app.Dao().FindCollectionByNameOrId(collectionName)
		if err != nil {
			t.Skipf("Skipping integration test: collection %s not found", collectionName)
		}
	}

	// Create scheduler
	scheduler := NewPricingScheduler(app)

	// Create test deployment with stale cost data
	deploymentCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentCollection)
	deployment.Set("name", "integration-test-stale-cost")
	deployment.Set("status", "active")
	deployment.Set("region", "us-east-1")
	deployment.Set("blueprint", "web-application")
	deployment.Set("user", "test-user-id")
	deployment.Set("created", time.Now().Add(-5*24*time.Hour))

	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to save deployment: %v", err)
	}

	deploymentID := deployment.Id
	t.Logf("Created test deployment: %s", deploymentID)

	// Create stale actual cost data
	actualCostCollection, err := app.Dao().FindCollectionByNameOrId("actualCosts")
	if err != nil {
		t.Fatalf("Failed to find actualCosts collection: %v", err)
	}

	actualCost := models.NewRecord(actualCostCollection)
	actualCost.Set("deployment", deploymentID)
	actualCost.Set("costToDate", 50.0)
	actualCost.Set("projectedMonthly", 60.0)
	actualCost.Set("variance", 5.0)
	actualCost.Set("breakdown", map[string]interface{}{"EC2": 30.0, "S3": 20.0})
	actualCost.Set("periodStart", time.Now().Add(-72*time.Hour))
	actualCost.Set("periodEnd", time.Now().Add(-48*time.Hour))
	actualCost.Set("fetchedAt", time.Now().Add(-60*time.Hour)) // Stale: 60 hours old

	if err := app.Dao().SaveRecord(actualCost); err != nil {
		t.Fatalf("Failed to save actual cost: %v", err)
	}

	t.Log("Created stale cost data (60 hours old)")

	// Execute the freshness monitoring job
	jobFunc := scheduler.createCostFreshnessMonitorJob()
	if err := jobFunc(); err != nil {
		t.Fatalf("Cost freshness monitor job failed: %v", err)
	}

	t.Log("Freshness monitoring job completed successfully")

	// Verify metrics
	metrics, err := scheduler.GetCostDataFreshnessStatus()
	if err != nil {
		t.Fatalf("Failed to get freshness status: %v", err)
	}

	t.Logf("Freshness metrics: %+v", metrics)

	if metrics.DeploymentsWithStaleData < 1 {
		t.Errorf("Expected at least 1 deployment with stale data, got %d", metrics.DeploymentsWithStaleData)
	}

	// Verify alert was created
	alerts, err := app.Dao().FindRecordsByFilter(
		"costAlerts",
		"deployment = {:deploymentId} && type = 'cost_data_stale'",
		"",
		0,
		0,
		map[string]any{
			"deploymentId": deploymentID,
		},
	)
	if err != nil {
		t.Fatalf("Failed to find alerts: %v", err)
	}

	if len(alerts) < 1 {
		t.Errorf("Expected at least 1 alert for stale data, got %d", len(alerts))
	} else {
		alert := alerts[0]
		t.Logf("Alert created: %s", alert.GetString("message"))
		
		if alert.GetString("type") != "cost_data_stale" {
			t.Errorf("Expected alert type 'cost_data_stale', got '%s'", alert.GetString("type"))
		}
		
		if !alert.GetBool("triggered") {
			t.Error("Expected alert to be triggered")
		}
	}

	// Clean up
	if err := app.Dao().DeleteRecord(deployment); err != nil {
		t.Logf("Warning: Failed to clean up deployment: %v", err)
	}
	if err := app.Dao().DeleteRecord(actualCost); err != nil {
		t.Logf("Warning: Failed to clean up actual cost: %v", err)
	}
	for _, alert := range alerts {
		if err := app.Dao().DeleteRecord(alert); err != nil {
			t.Logf("Warning: Failed to clean up alert: %v", err)
		}
	}
}

// TestCostFreshnessMonitor_FreshData_Integration tests with fresh cost data
func TestCostFreshnessMonitor_FreshData_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// Verify collections exist
	collections := []string{"deployments", "actualCosts", "costAlerts"}
	for _, collectionName := range collections {
		_, err := app.Dao().FindCollectionByNameOrId(collectionName)
		if err != nil {
			t.Skipf("Skipping integration test: collection %s not found", collectionName)
		}
	}

	scheduler := NewPricingScheduler(app)

	// Create test deployment with fresh cost data
	deploymentCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentCollection)
	deployment.Set("name", "integration-test-fresh-cost")
	deployment.Set("status", "active")
	deployment.Set("region", "us-east-1")
	deployment.Set("blueprint", "web-application")
	deployment.Set("user", "test-user-id")
	deployment.Set("created", time.Now().Add(-72*time.Hour))

	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to save deployment: %v", err)
	}

	deploymentID := deployment.Id
	t.Logf("Created test deployment: %s", deploymentID)

	// Create fresh actual cost data
	actualCostCollection, err := app.Dao().FindCollectionByNameOrId("actualCosts")
	if err != nil {
		t.Fatalf("Failed to find actualCosts collection: %v", err)
	}

	actualCost := models.NewRecord(actualCostCollection)
	actualCost.Set("deployment", deploymentID)
	actualCost.Set("costToDate", 50.0)
	actualCost.Set("projectedMonthly", 60.0)
	actualCost.Set("variance", 5.0)
	actualCost.Set("breakdown", map[string]interface{}{"EC2": 30.0, "S3": 20.0})
	actualCost.Set("periodStart", time.Now().Add(-24*time.Hour))
	actualCost.Set("periodEnd", time.Now())
	actualCost.Set("fetchedAt", time.Now().Add(-12*time.Hour)) // Fresh: 12 hours old

	if err := app.Dao().SaveRecord(actualCost); err != nil {
		t.Fatalf("Failed to save actual cost: %v", err)
	}

	t.Log("Created fresh cost data (12 hours old)")

	// Execute the freshness monitoring job
	jobFunc := scheduler.createCostFreshnessMonitorJob()
	if err := jobFunc(); err != nil {
		t.Fatalf("Cost freshness monitor job failed: %v", err)
	}

	t.Log("Freshness monitoring job completed successfully")

	// Verify metrics
	metrics, err := scheduler.GetCostDataFreshnessStatus()
	if err != nil {
		t.Fatalf("Failed to get freshness status: %v", err)
	}

	t.Logf("Freshness metrics: %+v", metrics)

	if metrics.DeploymentsWithFreshData < 1 {
		t.Errorf("Expected at least 1 deployment with fresh data, got %d", metrics.DeploymentsWithFreshData)
	}

	// Verify no alert was created for fresh data
	alerts, err := app.Dao().FindRecordsByFilter(
		"costAlerts",
		"deployment = {:deploymentId} && type = 'cost_data_stale'",
		"",
		0,
		0,
		map[string]any{
			"deploymentId": deploymentID,
		},
	)
	if err != nil {
		t.Fatalf("Failed to find alerts: %v", err)
	}

	if len(alerts) > 0 {
		t.Errorf("Expected 0 alerts for fresh data, got %d", len(alerts))
	}

	// Clean up
	if err := app.Dao().DeleteRecord(deployment); err != nil {
		t.Logf("Warning: Failed to clean up deployment: %v", err)
	}
	if err := app.Dao().DeleteRecord(actualCost); err != nil {
		t.Logf("Warning: Failed to clean up actual cost: %v", err)
	}
}

// TestIsCostDataFresh_Integration tests the IsCostDataFresh method
func TestIsCostDataFresh_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// Verify collections exist
	collections := []string{"deployments", "actualCosts"}
	for _, collectionName := range collections {
		_, err := app.Dao().FindCollectionByNameOrId(collectionName)
		if err != nil {
			t.Skipf("Skipping integration test: collection %s not found", collectionName)
		}
	}

	scheduler := NewPricingScheduler(app)

	// Create test deployment
	deploymentCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentCollection)
	deployment.Set("name", "integration-test-freshness-check")
	deployment.Set("status", "active")
	deployment.Set("region", "us-east-1")
	deployment.Set("blueprint", "web-application")
	deployment.Set("user", "test-user-id")

	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to save deployment: %v", err)
	}

	deploymentID := deployment.Id
	t.Logf("Created test deployment: %s", deploymentID)

	// Test 1: No cost data
	isFresh, dataAge, err := scheduler.IsCostDataFresh(deploymentID)
	if err == nil {
		t.Error("Expected error for deployment without cost data")
	}
	t.Logf("No cost data test: isFresh=%v, dataAge=%v, err=%v", isFresh, dataAge, err)

	// Create fresh cost data
	actualCostCollection, err := app.Dao().FindCollectionByNameOrId("actualCosts")
	if err != nil {
		t.Fatalf("Failed to find actualCosts collection: %v", err)
	}

	actualCost := models.NewRecord(actualCostCollection)
	actualCost.Set("deployment", deploymentID)
	actualCost.Set("costToDate", 50.0)
	actualCost.Set("projectedMonthly", 60.0)
	actualCost.Set("variance", 5.0)
	actualCost.Set("breakdown", map[string]interface{}{"EC2": 30.0})
	actualCost.Set("periodStart", time.Now().Add(-24*time.Hour))
	actualCost.Set("periodEnd", time.Now())
	actualCost.Set("fetchedAt", time.Now().Add(-12*time.Hour))

	if err := app.Dao().SaveRecord(actualCost); err != nil {
		t.Fatalf("Failed to save actual cost: %v", err)
	}

	// Test 2: Fresh data
	isFresh, dataAge, err = scheduler.IsCostDataFresh(deploymentID)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !isFresh {
		t.Error("Expected isFresh to be true for 12-hour-old data")
	}
	t.Logf("Fresh data test: isFresh=%v, dataAge=%v", isFresh, dataAge)

	// Update to stale data
	actualCost.Set("fetchedAt", time.Now().Add(-60*time.Hour))
	if err := app.Dao().SaveRecord(actualCost); err != nil {
		t.Fatalf("Failed to update actual cost: %v", err)
	}

	// Test 3: Stale data
	isFresh, dataAge, err = scheduler.IsCostDataFresh(deploymentID)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if isFresh {
		t.Error("Expected isFresh to be false for 60-hour-old data")
	}
	t.Logf("Stale data test: isFresh=%v, dataAge=%v", isFresh, dataAge)

	// Clean up
	if err := app.Dao().DeleteRecord(deployment); err != nil {
		t.Logf("Warning: Failed to clean up deployment: %v", err)
	}
	if err := app.Dao().DeleteRecord(actualCost); err != nil {
		t.Logf("Warning: Failed to clean up actual cost: %v", err)
	}
}
