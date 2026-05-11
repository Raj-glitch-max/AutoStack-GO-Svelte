package jobs

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tests"
)

func TestCostFreshnessMonitor(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// Ensure collections exist
	ensureCostCollections(t, app)

	// Create scheduler
	scheduler := NewPricingScheduler(app)

	// Create test deployment
	deploymentCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentCollection)
	deployment.Set("name", "test-deployment")
	deployment.Set("status", "active")
	deployment.Set("region", "us-east-1")
	deployment.Set("blueprint", "web-application")
	deployment.Set("user", "test-user-id")
	
	// Set created time to 72 hours ago (old enough for cost data)
	createdTime := time.Now().Add(-72 * time.Hour)
	deployment.Set("created", createdTime)

	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to save deployment: %v", err)
	}

	// Create the cost freshness monitor job function
	jobFunc := scheduler.createCostFreshnessMonitorJob()

	// Execute the job - should detect deployment without cost data
	if err := jobFunc(); err != nil {
		t.Fatalf("Cost freshness monitor job failed: %v", err)
	}

	// Verify metrics were calculated
	metrics, err := scheduler.GetCostDataFreshnessStatus()
	if err != nil {
		t.Fatalf("Failed to get cost freshness status: %v", err)
	}

	if metrics.TotalActiveDeployments != 1 {
		t.Errorf("Expected 1 active deployment, got %d", metrics.TotalActiveDeployments)
	}

	if metrics.DeploymentsWithoutData != 1 {
		t.Errorf("Expected 1 deployment without data, got %d", metrics.DeploymentsWithoutData)
	}

	// Verify alert was created
	alerts, err := app.Dao().FindRecordsByFilter(
		"costAlerts",
		"deployment = {:deploymentId} && type = 'cost_data_stale'",
		"",
		0,
		0,
		map[string]any{
			"deploymentId": deployment.Id,
		},
	)
	if err != nil {
		t.Fatalf("Failed to find alerts: %v", err)
	}

	if len(alerts) != 1 {
		t.Errorf("Expected 1 alert, got %d", len(alerts))
	}
}

func TestCostFreshnessMonitor_WithFreshData(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// Ensure collections exist
	ensureCostCollections(t, app)

	// Create scheduler
	scheduler := NewPricingScheduler(app)

	// Create test deployment
	deploymentCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentCollection)
	deployment.Set("name", "test-deployment-fresh")
	deployment.Set("status", "active")
	deployment.Set("region", "us-east-1")
	deployment.Set("blueprint", "web-application")
	deployment.Set("user", "test-user-id")
	
	// Set created time to 72 hours ago
	createdTime := time.Now().Add(-72 * time.Hour)
	deployment.Set("created", createdTime)

	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to save deployment: %v", err)
	}

	// Create fresh actual cost data
	actualCostCollection, err := app.Dao().FindCollectionByNameOrId("actualCosts")
	if err != nil {
		t.Fatalf("Failed to find actualCosts collection: %v", err)
	}

	actualCost := models.NewRecord(actualCostCollection)
	actualCost.Set("deployment", deployment.Id)
	actualCost.Set("costToDate", 50.0)
	actualCost.Set("projectedMonthly", 60.0)
	actualCost.Set("variance", 5.0)
	actualCost.Set("breakdown", map[string]float64{"EC2": 30.0, "S3": 20.0})
	actualCost.Set("periodStart", time.Now().Add(-24*time.Hour))
	actualCost.Set("periodEnd", time.Now())
	actualCost.Set("fetchedAt", time.Now().Add(-12*time.Hour)) // Fresh data (12 hours old)

	if err := app.Dao().SaveRecord(actualCost); err != nil {
		t.Fatalf("Failed to save actual cost: %v", err)
	}

	// Execute the job
	jobFunc := scheduler.createCostFreshnessMonitorJob()
	if err := jobFunc(); err != nil {
		t.Fatalf("Cost freshness monitor job failed: %v", err)
	}

	// Verify metrics
	metrics, err := scheduler.GetCostDataFreshnessStatus()
	if err != nil {
		t.Fatalf("Failed to get cost freshness status: %v", err)
	}

	if metrics.TotalActiveDeployments != 1 {
		t.Errorf("Expected 1 active deployment, got %d", metrics.TotalActiveDeployments)
	}

	if metrics.DeploymentsWithFreshData != 1 {
		t.Errorf("Expected 1 deployment with fresh data, got %d", metrics.DeploymentsWithFreshData)
	}

	if metrics.DeploymentsWithStaleData != 0 {
		t.Errorf("Expected 0 deployments with stale data, got %d", metrics.DeploymentsWithStaleData)
	}

	// Verify no alert was created
	alerts, err := app.Dao().FindRecordsByFilter(
		"costAlerts",
		"deployment = {:deploymentId} && type = 'cost_data_stale'",
		"",
		0,
		0,
		map[string]any{
			"deploymentId": deployment.Id,
		},
	)
	if err != nil {
		t.Fatalf("Failed to find alerts: %v", err)
	}

	if len(alerts) != 0 {
		t.Errorf("Expected 0 alerts for fresh data, got %d", len(alerts))
	}
}

func TestCostFreshnessMonitor_WithStaleData(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// Ensure collections exist
	ensureCostCollections(t, app)

	// Create scheduler
	scheduler := NewPricingScheduler(app)

	// Create test deployment
	deploymentCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentCollection)
	deployment.Set("name", "test-deployment-stale")
	deployment.Set("status", "active")
	deployment.Set("region", "us-east-1")
	deployment.Set("blueprint", "web-application")
	deployment.Set("user", "test-user-id")
	
	// Set created time to 5 days ago
	createdTime := time.Now().Add(-5 * 24 * time.Hour)
	deployment.Set("created", createdTime)

	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to save deployment: %v", err)
	}

	// Create stale actual cost data
	actualCostCollection, err := app.Dao().FindCollectionByNameOrId("actualCosts")
	if err != nil {
		t.Fatalf("Failed to find actualCosts collection: %v", err)
	}

	actualCost := models.NewRecord(actualCostCollection)
	actualCost.Set("deployment", deployment.Id)
	actualCost.Set("costToDate", 50.0)
	actualCost.Set("projectedMonthly", 60.0)
	actualCost.Set("variance", 5.0)
	actualCost.Set("breakdown", map[string]interface{}{"EC2": 30.0, "S3": 20.0})
	actualCost.Set("periodStart", time.Now().Add(-72*time.Hour))
	actualCost.Set("periodEnd", time.Now().Add(-48*time.Hour))
	actualCost.Set("fetchedAt", time.Now().Add(-60*time.Hour)) // Stale data (60 hours old)

	if err := app.Dao().SaveRecord(actualCost); err != nil {
		t.Fatalf("Failed to save actual cost: %v", err)
	}

	// Execute the job
	jobFunc := scheduler.createCostFreshnessMonitorJob()
	if err := jobFunc(); err != nil {
		t.Fatalf("Cost freshness monitor job failed: %v", err)
	}

	// Verify metrics
	metrics, err := scheduler.GetCostDataFreshnessStatus()
	if err != nil {
		t.Fatalf("Failed to get cost freshness status: %v", err)
	}

	if metrics.TotalActiveDeployments != 1 {
		t.Errorf("Expected 1 active deployment, got %d", metrics.TotalActiveDeployments)
	}

	if metrics.DeploymentsWithStaleData != 1 {
		t.Errorf("Expected 1 deployment with stale data, got %d", metrics.DeploymentsWithStaleData)
	}

	if len(metrics.StaleDeployments) != 1 {
		t.Errorf("Expected 1 stale deployment in list, got %d", len(metrics.StaleDeployments))
	}

	// Verify alert was created
	alerts, err := app.Dao().FindRecordsByFilter(
		"costAlerts",
		"deployment = {:deploymentId} && type = 'cost_data_stale'",
		"",
		0,
		0,
		map[string]any{
			"deploymentId": deployment.Id,
		},
	)
	if err != nil {
		t.Fatalf("Failed to find alerts: %v", err)
	}

	if len(alerts) != 1 {
		t.Errorf("Expected 1 alert for stale data, got %d", len(alerts))
	}

	// Verify alert details
	alert := alerts[0]
	if alert.GetString("type") != "cost_data_stale" {
		t.Errorf("Expected alert type 'cost_data_stale', got '%s'", alert.GetString("type"))
	}

	if !alert.GetBool("triggered") {
		t.Error("Expected alert to be triggered")
	}

	if alert.GetBool("acknowledged") {
		t.Error("Expected alert to not be acknowledged")
	}
}

func TestCostFreshnessMonitor_NewDeployment(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// Create scheduler
	scheduler := NewPricingScheduler(app)

	// Create test deployment that's too new for cost data
	deploymentCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentCollection)
	deployment.Set("name", "test-deployment-new")
	deployment.Set("status", "active")
	deployment.Set("region", "us-east-1")
	deployment.Set("blueprint", "web-application")
	deployment.Set("user", "test-user-id")
	
	// Set created time to 24 hours ago (too new for cost data)
	createdTime := time.Now().Add(-24 * time.Hour)
	deployment.Set("created", createdTime)

	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to save deployment: %v", err)
	}

	// Execute the job
	jobFunc := scheduler.createCostFreshnessMonitorJob()
	if err := jobFunc(); err != nil {
		t.Fatalf("Cost freshness monitor job failed: %v", err)
	}

	// Verify metrics - new deployment should not be counted
	metrics, err := scheduler.GetCostDataFreshnessStatus()
	if err != nil {
		t.Fatalf("Failed to get cost freshness status: %v", err)
	}

	// The deployment is active but too new, so it shouldn't be counted in any category
	if metrics.DeploymentsWithoutData != 0 {
		t.Errorf("Expected 0 deployments without data (too new), got %d", metrics.DeploymentsWithoutData)
	}

	// Verify no alert was created for new deployment
	alerts, err := app.Dao().FindRecordsByFilter(
		"costAlerts",
		"deployment = {:deploymentId} && type = 'cost_data_stale'",
		"",
		0,
		0,
		map[string]any{
			"deploymentId": deployment.Id,
		},
	)
	if err != nil {
		t.Fatalf("Failed to find alerts: %v", err)
	}

	if len(alerts) != 0 {
		t.Errorf("Expected 0 alerts for new deployment, got %d", len(alerts))
	}
}

func TestIsCostDataFresh(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// Create scheduler
	scheduler := NewPricingScheduler(app)

	// Create test deployment
	deploymentCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentCollection)
	deployment.Set("name", "test-deployment")
	deployment.Set("status", "active")
	deployment.Set("region", "us-east-1")
	deployment.Set("blueprint", "web-application")
	deployment.Set("user", "test-user-id")

	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to save deployment: %v", err)
	}

	// Test 1: No cost data
	isFresh, dataAge, err := scheduler.IsCostDataFresh(deployment.Id)
	if err == nil {
		t.Error("Expected error for deployment without cost data")
	}
	if isFresh {
		t.Error("Expected isFresh to be false when no data exists")
	}

	// Create fresh cost data
	actualCostCollection, err := app.Dao().FindCollectionByNameOrId("actualCosts")
	if err != nil {
		t.Fatalf("Failed to find actualCosts collection: %v", err)
	}

	actualCost := models.NewRecord(actualCostCollection)
	actualCost.Set("deployment", deployment.Id)
	actualCost.Set("costToDate", 50.0)
	actualCost.Set("projectedMonthly", 60.0)
	actualCost.Set("variance", 5.0)
	actualCost.Set("breakdown", map[string]float64{"EC2": 30.0})
	actualCost.Set("periodStart", time.Now().Add(-24*time.Hour))
	actualCost.Set("periodEnd", time.Now())
	actualCost.Set("fetchedAt", time.Now().Add(-12*time.Hour)) // 12 hours old

	if err := app.Dao().SaveRecord(actualCost); err != nil {
		t.Fatalf("Failed to save actual cost: %v", err)
	}

	// Test 2: Fresh data
	isFresh, dataAge, err = scheduler.IsCostDataFresh(deployment.Id)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !isFresh {
		t.Error("Expected isFresh to be true for 12-hour-old data")
	}
	if dataAge < 11*time.Hour || dataAge > 13*time.Hour {
		t.Errorf("Expected data age around 12 hours, got %v", dataAge)
	}

	// Update to stale data
	actualCost.Set("fetchedAt", time.Now().Add(-60*time.Hour)) // 60 hours old
	if err := app.Dao().SaveRecord(actualCost); err != nil {
		t.Fatalf("Failed to update actual cost: %v", err)
	}

	// Test 3: Stale data
	isFresh, dataAge, err = scheduler.IsCostDataFresh(deployment.Id)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if isFresh {
		t.Error("Expected isFresh to be false for 60-hour-old data")
	}
	if dataAge < 59*time.Hour || dataAge > 61*time.Hour {
		t.Errorf("Expected data age around 60 hours, got %v", dataAge)
	}
}

func TestGetStaleDeployments(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// Create scheduler
	scheduler := NewPricingScheduler(app)

	// Create multiple test deployments with different data states
	deploymentCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	actualCostCollection, err := app.Dao().FindCollectionByNameOrId("actualCosts")
	if err != nil {
		t.Fatalf("Failed to find actualCosts collection: %v", err)
	}

	// Deployment 1: Fresh data
	deployment1 := models.NewRecord(deploymentCollection)
	deployment1.Set("name", "deployment-fresh")
	deployment1.Set("status", "active")
	deployment1.Set("region", "us-east-1")
	deployment1.Set("blueprint", "web-application")
	deployment1.Set("created", time.Now().Add(-72*time.Hour))
	if err := app.Dao().SaveRecord(deployment1); err != nil {
		t.Fatalf("Failed to save deployment1: %v", err)
	}

	actualCost1 := models.NewRecord(actualCostCollection)
	actualCost1.Set("deployment", deployment1.Id)
	actualCost1.Set("costToDate", 50.0)
	actualCost1.Set("projectedMonthly", 60.0)
	actualCost1.Set("variance", 5.0)
	actualCost1.Set("breakdown", map[string]float64{"EC2": 30.0})
	actualCost1.Set("periodStart", time.Now().Add(-24*time.Hour))
	actualCost1.Set("periodEnd", time.Now())
	actualCost1.Set("fetchedAt", time.Now().Add(-12*time.Hour))
	if err := app.Dao().SaveRecord(actualCost1); err != nil {
		t.Fatalf("Failed to save actualCost1: %v", err)
	}

	// Deployment 2: Stale data
	deployment2 := models.NewRecord(deploymentCollection)
	deployment2.Set("name", "deployment-stale")
	deployment2.Set("status", "active")
	deployment2.Set("region", "us-east-1")
	deployment2.Set("blueprint", "web-application")
	deployment2.Set("created", time.Now().Add(-5*24*time.Hour))
	if err := app.Dao().SaveRecord(deployment2); err != nil {
		t.Fatalf("Failed to save deployment2: %v", err)
	}

	actualCost2 := models.NewRecord(actualCostCollection)
	actualCost2.Set("deployment", deployment2.Id)
	actualCost2.Set("costToDate", 50.0)
	actualCost2.Set("projectedMonthly", 60.0)
	actualCost2.Set("variance", 5.0)
	actualCost2.Set("breakdown", map[string]float64{"EC2": 30.0})
	actualCost2.Set("periodStart", time.Now().Add(-72*time.Hour))
	actualCost2.Set("periodEnd", time.Now().Add(-48*time.Hour))
	actualCost2.Set("fetchedAt", time.Now().Add(-60*time.Hour))
	if err := app.Dao().SaveRecord(actualCost2); err != nil {
		t.Fatalf("Failed to save actualCost2: %v", err)
	}

	// Deployment 3: No data
	deployment3 := models.NewRecord(deploymentCollection)
	deployment3.Set("name", "deployment-no-data")
	deployment3.Set("status", "active")
	deployment3.Set("region", "us-east-1")
	deployment3.Set("blueprint", "web-application")
	deployment3.Set("created", time.Now().Add(-72*time.Hour))
	if err := app.Dao().SaveRecord(deployment3); err != nil {
		t.Fatalf("Failed to save deployment3: %v", err)
	}

	// Get stale deployments
	staleDeployments, err := scheduler.GetStaleDeployments()
	if err != nil {
		t.Fatalf("Failed to get stale deployments: %v", err)
	}

	// Should have 2 stale deployments (deployment2 and deployment3)
	if len(staleDeployments) != 2 {
		t.Errorf("Expected 2 stale deployments, got %d", len(staleDeployments))
	}

	// Verify the stale deployments are correct
	foundStale := false
	foundNoData := false
	for _, stale := range staleDeployments {
		if stale.DeploymentID == deployment2.Id && stale.Status == "stale" {
			foundStale = true
		}
		if stale.DeploymentID == deployment3.Id && stale.Status == "no_data" {
			foundNoData = true
		}
	}

	if !foundStale {
		t.Error("Expected to find deployment2 in stale deployments list")
	}
	if !foundNoData {
		t.Error("Expected to find deployment3 in stale deployments list")
	}
}
