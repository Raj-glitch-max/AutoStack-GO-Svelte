package cost

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// TestNewCostMonitor tests creating a new cost monitor
func TestNewCostMonitor(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	monitor, err := NewCostMonitor(app)
	if err != nil {
		t.Fatalf("Failed to create cost monitor: %v", err)
	}

	if monitor == nil {
		t.Fatal("Expected cost monitor to be created")
	}

	if monitor.alertThreshold != 20.0 {
		t.Errorf("Expected default threshold 20.0, got %.1f", monitor.alertThreshold)
	}
}

// TestCheckCostAnomalies_NoDeployments tests anomaly detection with no deployments
func TestCheckCostAnomalies_NoDeployments(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	monitor, err := NewCostMonitor(app)
	if err != nil {
		t.Fatalf("Failed to create cost monitor: %v", err)
	}

	// This test will fail if deployments collection doesn't exist
	// which is expected in a minimal test environment
	err = monitor.CheckCostAnomalies()
	
	// We expect either no error (if collections exist and are empty)
	// or an error about missing collections
	if err != nil && err.Error() != "failed to get active deployments: failed to find deployments collection: sql: no rows in result set" {
		t.Errorf("Unexpected error: %v", err)
	}
}

// TestCalculateVariance tests variance calculation
func TestCalculateVariance(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	monitor, err := NewCostMonitor(app)
	if err != nil {
		t.Fatalf("Failed to create cost monitor: %v", err)
	}

	testCases := []struct {
		name     string
		actual   float64
		estimate float64
		expected float64
	}{
		{
			name:     "25% over budget",
			actual:   125.0,
			estimate: 100.0,
			expected: 25.0,
		},
		{
			name:     "10% under budget",
			actual:   90.0,
			estimate: 100.0,
			expected: -10.0,
		},
		{
			name:     "Exactly on budget",
			actual:   100.0,
			estimate: 100.0,
			expected: 0.0,
		},
		{
			name:     "Zero estimate",
			actual:   100.0,
			estimate: 0.0,
			expected: 0.0,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			variance := monitor.calculateVariance(tt.actual, tt.estimate)
			if variance != tt.expected {
				t.Errorf("Expected variance %.1f, got %.1f", tt.expected, variance)
			}
		})
	}
}

// TestSetAlertThreshold tests setting custom alert threshold
func TestSetAlertThreshold(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	monitor, err := NewCostMonitor(app)
	if err != nil {
		t.Fatalf("Failed to create cost monitor: %v", err)
	}

	// Default threshold
	if monitor.GetAlertThreshold() != 20.0 {
		t.Errorf("Expected default threshold 20.0, got %.1f", monitor.GetAlertThreshold())
	}

	// Set custom threshold
	monitor.SetAlertThreshold(30.0)

	if monitor.GetAlertThreshold() != 30.0 {
		t.Errorf("Expected threshold 30.0, got %.1f", monitor.GetAlertThreshold())
	}
}

// TestGetAlertThreshold tests getting the alert threshold
func TestGetAlertThreshold(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	monitor, err := NewCostMonitor(app)
	if err != nil {
		t.Fatalf("Failed to create cost monitor: %v", err)
	}

	threshold := monitor.GetAlertThreshold()
	if threshold != 20.0 {
		t.Errorf("Expected default threshold 20.0, got %.1f", threshold)
	}
}

// TestCostMonitor_DefaultThreshold tests that default threshold is 20%
func TestCostMonitor_DefaultThreshold(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	monitor, err := NewCostMonitor(app)
	if err != nil {
		t.Fatalf("Failed to create cost monitor: %v", err)
	}

	// Validates: AC-4.1 (Alert triggered when actual cost exceeds estimate by 20%)
	if monitor.alertThreshold != 20.0 {
		t.Errorf("Expected default threshold 20.0%%, got %.1f%%", monitor.alertThreshold)
	}
}

// TestCostMonitor_VarianceCalculation tests variance calculation accuracy
func TestCostMonitor_VarianceCalculation(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	monitor, err := NewCostMonitor(app)
	if err != nil {
		t.Fatalf("Failed to create cost monitor: %v", err)
	}

	// Test case: 25% over budget
	actual := 125.0
	estimate := 100.0
	expectedVariance := 25.0

	variance := monitor.calculateVariance(actual, estimate)
	if variance != expectedVariance {
		t.Errorf("Expected variance %.1f%%, got %.1f%%", expectedVariance, variance)
	}

	// Test case: 10% under budget
	actual = 90.0
	estimate = 100.0
	expectedVariance = -10.0

	variance = monitor.calculateVariance(actual, estimate)
	if variance != expectedVariance {
		t.Errorf("Expected variance %.1f%%, got %.1f%%", expectedVariance, variance)
	}

	// Test case: Exactly on budget
	actual = 100.0
	estimate = 100.0
	expectedVariance = 0.0

	variance = monitor.calculateVariance(actual, estimate)
	if variance != expectedVariance {
		t.Errorf("Expected variance %.1f%%, got %.1f%%", expectedVariance, variance)
	}
}

// TestCostMonitor_ZeroEstimateHandling tests handling of zero estimate
func TestCostMonitor_ZeroEstimateHandling(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	monitor, err := NewCostMonitor(app)
	if err != nil {
		t.Fatalf("Failed to create cost monitor: %v", err)
	}

	// When estimate is zero, variance should be zero (avoid division by zero)
	actual := 100.0
	estimate := 0.0
	expectedVariance := 0.0

	variance := monitor.calculateVariance(actual, estimate)
	if variance != expectedVariance {
		t.Errorf("Expected variance %.1f%% for zero estimate, got %.1f%%", expectedVariance, variance)
	}
}
