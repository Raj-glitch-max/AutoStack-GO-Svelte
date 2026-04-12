package cost

import (
	"testing"
)

// TestRangeCalculatorIntegration tests the range calculator integration with cost estimators
func TestRangeCalculatorIntegration(t *testing.T) {
	t.Run("Range calculator produces consistent results across all calculators", func(t *testing.T) {
		testCases := []struct {
			name     string
			baseCost float64
		}{
			{"Small cost", 10.00},
			{"Medium cost", 100.00},
			{"Large cost", 1000.00},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Calculate range
				min, max, estimate := CalculateRange(tc.baseCost)

				// Verify range properties
				if min > estimate {
					t.Errorf("Min (%f) should be <= estimate (%f)", min, estimate)
				}
				if max < estimate {
					t.Errorf("Max (%f) should be >= estimate (%f)", max, estimate)
				}

				// Verify margins
				expectedMin := estimate * 0.8
				expectedMax := estimate * 1.4

				// Allow small rounding differences
				if min < expectedMin-0.01 || min > expectedMin+0.01 {
					t.Errorf("Min (%f) should be approximately %f (80%% of estimate)", min, expectedMin)
				}
				if max < expectedMax-0.01 || max > expectedMax+0.01 {
					t.Errorf("Max (%f) should be approximately %f (140%% of estimate)", max, expectedMax)
				}
			})
		}
	})

	t.Run("CalculateCostRange returns valid CostRange struct", func(t *testing.T) {
		baseCost := 56.65

		costRange, err := CalculateCostRange(baseCost)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Verify the struct is properly populated
		if costRange.Estimate != 56.65 {
			t.Errorf("Expected estimate=56.65, got %f", costRange.Estimate)
		}
		if costRange.Min != 45.32 {
			t.Errorf("Expected min=45.32, got %f", costRange.Min)
		}
		if costRange.Max != 79.31 {
			t.Errorf("Expected max=79.31, got %f", costRange.Max)
		}

		// Validate the range
		if err := ValidateRange(costRange); err != nil {
			t.Errorf("Range validation failed: %v", err)
		}
	})

	t.Run("AddRangeToCostEstimate works with CostEstimateWithRange", func(t *testing.T) {
		estimate := &CostEstimateWithRange{
			Estimate: 100.00,
			Currency: "USD",
		}

		err := AddRangeToCostEstimate(estimate)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Verify range was added
		if estimate.RangeMin != 80.00 {
			t.Errorf("Expected RangeMin=80.00, got %f", estimate.RangeMin)
		}
		if estimate.RangeMax != 140.00 {
			t.Errorf("Expected RangeMax=140.00, got %f", estimate.RangeMax)
		}
	})

	t.Run("CalculateRangeForBreakdown works with service breakdown", func(t *testing.T) {
		breakdown := map[string]float64{
			"compute":    29.15,
			"networking": 16.20,
			"storage":    2.30,
			"transfer":   9.00,
		}

		ranges, err := CalculateRangeForBreakdown(breakdown)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Verify all services have ranges
		if len(ranges) != len(breakdown) {
			t.Errorf("Expected %d ranges, got %d", len(breakdown), len(ranges))
		}

		// Verify each range is valid
		for service, costRange := range ranges {
			if err := ValidateRange(costRange); err != nil {
				t.Errorf("Range validation failed for service %s: %v", service, err)
			}

			// Verify the estimate matches the original cost
			originalCost := breakdown[service]
			if costRange.Estimate != originalCost {
				t.Errorf("Service %s: expected estimate=%f, got %f", service, originalCost, costRange.Estimate)
			}
		}
	})

	t.Run("Range calculator handles edge cases gracefully", func(t *testing.T) {
		testCases := []struct {
			name        string
			baseCost    float64
			expectError bool
		}{
			{"Zero cost", 0, false},
			{"Negative cost", -10.00, false}, // Returns zeros, no error
			{"Very small cost", 0.01, false},
			{"Very large cost", 1000000, false},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				min, max, estimate := CalculateRange(tc.baseCost)

				// For invalid inputs, should return zeros
				if tc.baseCost <= 0 {
					if min != 0 || max != 0 || estimate != 0 {
						t.Errorf("Expected zeros for invalid input, got min=%f, max=%f, estimate=%f",
							min, max, estimate)
					}
				} else {
					// For valid inputs, verify range properties
					if min > estimate || estimate > max {
						t.Errorf("Range order violated: min=%f, estimate=%f, max=%f", min, estimate, max)
					}
				}
			})
		}
	})
}

// TestRangeCalculatorWithRealWorldScenarios tests the range calculator with realistic cost scenarios
func TestRangeCalculatorWithRealWorldScenarios(t *testing.T) {
	scenarios := []struct {
		name        string
		description string
		baseCost    float64
		expectedMin float64
		expectedMax float64
	}{
		{
			name:        "Static website",
			description: "Small static website with S3 and CloudFront",
			baseCost:    15.50,
			expectedMin: 12.40,
			expectedMax: 21.70,
		},
		{
			name:        "Web application",
			description: "Web app with Fargate, ALB, and RDS",
			baseCost:    125.75,
			expectedMin: 100.60,
			expectedMax: 176.05,
		},
		{
			name:        "Full-stack application",
			description: "Full-stack app with multiple services",
			baseCost:    450.00,
			expectedMin: 360.00,
			expectedMax: 630.00,
		},
		{
			name:        "Microservices architecture",
			description: "Complex microservices with multiple containers",
			baseCost:    1250.00,
			expectedMin: 1000.00,
			expectedMax: 1750.00,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			min, max, estimate := CalculateRange(scenario.baseCost)

			// Verify the calculated values match expectations
			if min != scenario.expectedMin {
				t.Errorf("%s: Expected min=%f, got %f", scenario.description, scenario.expectedMin, min)
			}
			if max != scenario.expectedMax {
				t.Errorf("%s: Expected max=%f, got %f", scenario.description, scenario.expectedMax, max)
			}
			if estimate != scenario.baseCost {
				t.Errorf("%s: Expected estimate=%f, got %f", scenario.description, scenario.baseCost, estimate)
			}

			// Verify range properties
			if min > estimate || estimate > max {
				t.Errorf("%s: Range order violated: min=%f, estimate=%f, max=%f",
					scenario.description, min, estimate, max)
			}

			// Verify margins are correct
			minMargin := (estimate - min) / estimate
			maxMargin := (max - estimate) / estimate

			if minMargin < 0.19 || minMargin > 0.21 {
				t.Errorf("%s: Min margin should be ~20%%, got %.2f%%", scenario.description, minMargin*100)
			}
			if maxMargin < 0.39 || maxMargin > 0.41 {
				t.Errorf("%s: Max margin should be ~40%%, got %.2f%%", scenario.description, maxMargin*100)
			}
		})
	}
}
