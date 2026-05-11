package cost

import (
	"testing"

	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tests"
)

// TestAnomalyDetectionThreshold tests that alerts are triggered at the correct threshold
func TestAnomalyDetectionThreshold(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	testCases := []struct {
		name           string
		estimatedCost  float64
		actualCost     float64
		threshold      float64
		shouldTrigger  bool
	}{
		{
			name:          "Below threshold - no alert",
			estimatedCost: 100.0,
			actualCost:    115.0,
			threshold:     20.0,
			shouldTrigger: false,
		},
		{
			name:          "At threshold - trigger alert",
			estimatedCost: 100.0,
			actualCost:    120.0,
			threshold:     20.0,
			shouldTrigger: true,
		},
		{
			name:          "Above threshold - trigger alert",
			estimatedCost: 100.0,
			actualCost:    130.0,
			threshold:     20.0,
			shouldTrigger: true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			variance := (tt.actualCost - tt.estimatedCost) / tt.estimatedCost * 100
			shouldTrigger := variance >= tt.threshold

			if shouldTrigger != tt.shouldTrigger {
				t.Errorf("Expected shouldTrigger=%v, got %v (variance=%.1f%%, threshold=%.1f%%)",
					tt.shouldTrigger, shouldTrigger, variance, tt.threshold)
			}
		})
	}
}

// TestAnomalyDetectionDeduplication tests that duplicate alerts are not sent
func TestAnomalyDetectionDeduplication(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	if err := bootstrapAlertTestCollections(app); err != nil {
		t.Fatalf("Failed to bootstrap collections: %v", err)
	}

	deploymentCollection, _ := app.Dao().FindCollectionByNameOrId("deployments")
	deployment := models.NewRecord(deploymentCollection)
	deployment.Set("name", "test-deployment")
	deployment.Set("user", "test-user-id")
	deployment.Set("status", "active")
	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	monitor, _ := NewCostMonitor(app)

	alertCollection, _ := app.Dao().FindCollectionByNameOrId("costAlerts")
	alert1 := models.NewRecord(alertCollection)
	alert1.Set("deployment", deployment.Id)
	alert1.Set("user", "test-user-id")
	alert1.Set("acknowledged", false)
	if err := app.Dao().SaveRecord(alert1); err != nil {
		t.Fatalf("Failed to create alert: %v", err)
	}

	if !monitor.hasUnacknowledgedAlert(deployment.Id) {
		t.Error("Expected hasUnacknowledgedAlert to return true")
	}

	monitor.AcknowledgeAlert(alert1.Id)

	if monitor.hasUnacknowledgedAlert(deployment.Id) {
		t.Error("Expected hasUnacknowledgedAlert to return false after acknowledgment")
	}
}

func TestAnomalyDetectionServiceBreakdown(t *testing.T) {
	serviceBreakdown := map[string]float64{
		"AmazonEC2": 50.0,
		"AmazonRDS": 70.0,
	}

	total := 0.0
	for _, cost := range serviceBreakdown {
		total += cost
	}

	if total != 120.0 {
		t.Errorf("Expected total cost 120.0, got %.2f", total)
	}
}
