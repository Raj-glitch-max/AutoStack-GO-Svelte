package cost

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// TestGetDeploymentThreshold_CustomThreshold tests getting custom threshold
func TestGetDeploymentThreshold_CustomThreshold(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	monitor, err := NewCostMonitor(app)
	if err != nil {
		t.Fatalf("Failed to create cost monitor: %v", err)
	}

	// Create test deployment with custom threshold
	deployment, err := createTestDeploymentWithThreshold(app, "test-deployment", 30.0)
	if err != nil {
		t.Fatalf("Failed to create test deployment: %v", err)
	}

	// Get threshold
	threshold := monitor.getDeploymentThreshold(deployment.Id)

	// Should return custom threshold
	if threshold != 30.0 {
		t.Errorf("Expected custom threshold 30.0, got %.2f", threshold)
	}
}

// TestGetDeploymentThreshold_DefaultThreshold tests getting default threshold
func TestGetDeploymentThreshold_DefaultThreshold(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	monitor, err := NewCostMonitor(app)
	if err != nil {
		t.Fatalf("Failed to create cost monitor: %v", err)
	}

	// Create test deployment without custom threshold
	deployment, err := createTestDeploymentWithThreshold(app, "test-deployment", 0)
	if err != nil {
		t.Fatalf("Failed to create test deployment: %v", err)
	}

	// Get threshold
	threshold := monitor.getDeploymentThreshold(deployment.Id)

	// Should return default threshold (20%)
	if threshold != 20.0 {
		t.Errorf("Expected default threshold 20.0, got %.2f", threshold)
	}
}

// TestGetDeploymentThreshold_NonExistentDeployment tests handling of non-existent deployment
func TestGetDeploymentThreshold_NonExistentDeployment(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	monitor, err := NewCostMonitor(app)
	if err != nil {
		t.Fatalf("Failed to create cost monitor: %v", err)
	}

	// Get threshold for non-existent deployment
	threshold := monitor.getDeploymentThreshold("non-existent-id")

	// Should return default threshold
	if threshold != 20.0 {
		t.Errorf("Expected default threshold 20.0 for non-existent deployment, got %.2f", threshold)
	}
}

// TestGetDeploymentThreshold_ZeroThreshold tests handling of zero threshold
func TestGetDeploymentThreshold_ZeroThreshold(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	monitor, err := NewCostMonitor(app)
	if err != nil {
		t.Fatalf("Failed to create cost monitor: %v", err)
	}

	// Create test deployment with zero threshold (should use default)
	deployment, err := createTestDeploymentWithThreshold(app, "test-deployment", 0)
	if err != nil {
		t.Fatalf("Failed to create test deployment: %v", err)
	}

	// Get threshold
	threshold := monitor.getDeploymentThreshold(deployment.Id)

	// Should return default threshold when custom is 0
	if threshold != 20.0 {
		t.Errorf("Expected default threshold 20.0 when custom is 0, got %.2f", threshold)
	}
}

// TestGetDeploymentThreshold_VariousThresholds tests various threshold values
func TestGetDeploymentThreshold_VariousThresholds(t *testing.T) {
	testCases := []struct {
		name              string
		customThreshold   float64
		expectedThreshold float64
	}{
		{
			name:              "5% threshold",
			customThreshold:   5.0,
			expectedThreshold: 5.0,
		},
		{
			name:              "10% threshold",
			customThreshold:   10.0,
			expectedThreshold: 10.0,
		},
		{
			name:              "50% threshold",
			customThreshold:   50.0,
			expectedThreshold: 50.0,
		},
		{
			name:              "100% threshold",
			customThreshold:   100.0,
			expectedThreshold: 100.0,
		},
		{
			name:              "No custom threshold",
			customThreshold:   0,
			expectedThreshold: 20.0,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := tests.NewTestApp()
			defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

			monitor, err := NewCostMonitor(app)
			if err != nil {
				t.Fatalf("Failed to create cost monitor: %v", err)
			}

			// Create test deployment
			deployment, err := createTestDeploymentWithThreshold(app, "test-deployment", tt.customThreshold)
			if err != nil {
				t.Fatalf("Failed to create test deployment: %v", err)
			}

			// Get threshold
			threshold := monitor.getDeploymentThreshold(deployment.Id)

			if threshold != tt.expectedThreshold {
				t.Errorf("Expected threshold %.2f, got %.2f", tt.expectedThreshold, threshold)
			}
		})
	}
}

// TestCostMonitor_ThresholdValidation tests that thresholds are properly validated
// Validates: AC-4.4 (User can set custom alert thresholds per deployment)
func TestCostMonitor_ThresholdValidation(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	monitor, err := NewCostMonitor(app)
	if err != nil {
		t.Fatalf("Failed to create cost monitor: %v", err)
	}

	// Test various threshold scenarios
	testCases := []struct {
		name              string
		customThreshold   float64
		actualCost        float64
		estimatedCost     float64
		shouldTrigger     bool
	}{
		{
			name:              "Custom 30% threshold - should not trigger at 25% variance",
			customThreshold:   30.0,
			actualCost:        125.0,
			estimatedCost:     100.0,
			shouldTrigger:     false,
		},
		{
			name:              "Custom 30% threshold - should trigger at 35% variance",
			customThreshold:   30.0,
			actualCost:        135.0,
			estimatedCost:     100.0,
			shouldTrigger:     true,
		},
		{
			name:              "Custom 10% threshold - should trigger at 15% variance",
			customThreshold:   10.0,
			actualCost:        115.0,
			estimatedCost:     100.0,
			shouldTrigger:     true,
		},
		{
			name:              "Default 20% threshold - should trigger at 25% variance",
			customThreshold:   0, // Use default
			actualCost:        125.0,
			estimatedCost:     100.0,
			shouldTrigger:     true,
		},
		{
			name:              "Default 20% threshold - should not trigger at 15% variance",
			customThreshold:   0, // Use default
			actualCost:        115.0,
			estimatedCost:     100.0,
			shouldTrigger:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create deployment with custom threshold
			deployment, err := createTestDeploymentWithThreshold(app, "test-deployment", tc.customThreshold)
			if err != nil {
				t.Fatalf("Failed to create test deployment: %v", err)
			}

			// Get threshold for deployment
			threshold := monitor.getDeploymentThreshold(deployment.Id)

			// Calculate variance
			variance := monitor.calculateVariance(tc.actualCost, tc.estimatedCost)

			// Check if alert should trigger
			shouldTrigger := variance > threshold

			if shouldTrigger != tc.shouldTrigger {
				t.Errorf("Expected shouldTrigger=%v, got %v (variance=%.2f%%, threshold=%.2f%%)",
					tc.shouldTrigger, shouldTrigger, variance, threshold)
			}
		})
	}
}

// Helper function to create a test deployment with custom threshold
func createTestDeploymentWithThreshold(app core.App, name string, threshold float64) (*models.Record, error) {
	collection, err := app.Dao().FindCollectionByNameOrId("awsDeployments")
	if err != nil {
		return nil, err
	}

	deployment := models.NewRecord(collection)
	deployment.Set("name", name)
	deployment.Set("status", "active")
	deployment.Set("region", "us-east-1")

	if threshold > 0 {
		deployment.Set("costAlertThreshold", threshold)
	}

	if err := app.Dao().SaveRecord(deployment); err != nil {
		return nil, err
	}

	return deployment, nil
}
