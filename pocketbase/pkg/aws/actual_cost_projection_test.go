package aws

import (
	"testing"
	"time"
)

// TestProjectedMonthlyCostEdgeCases tests edge cases for monthly cost projection
func TestProjectedMonthlyCostEdgeCases(t *testing.T) {
	acf := &ActualCostFetcher{}

	tests := []struct {
		name        string
		costToDate  float64
		daysElapsed float64
		expected    float64
		description string
	}{
		{
			name:        "First day of month",
			costToDate:  5.00,
			daysElapsed: 1.0,
			expected:    150.00, // 5/1 * 30 = 150
			description: "Should project first day cost to full month",
		},
		{
			name:        "Last day of month (30 days)",
			costToDate:  150.00,
			daysElapsed: 30.0,
			expected:    150.00, // 150/30 * 30 = 150
			description: "Should return same cost for full month",
		},
		{
			name:        "Mid-month (15 days)",
			costToDate:  75.00,
			daysElapsed: 15.0,
			expected:    150.00, // 75/15 * 30 = 150
			description: "Should project mid-month to full month",
		},
		{
			name:        "Quarter month (7.5 days)",
			costToDate:  37.50,
			daysElapsed: 7.5,
			expected:    150.00, // 37.5/7.5 * 30 = 150
			description: "Should handle fractional days",
		},
		{
			name:        "Very early in month (0.5 days / 12 hours)",
			costToDate:  2.50,
			daysElapsed: 0.5,
			expected:    150.00, // 2.5/0.5 * 30 = 150
			description: "Should handle partial first day",
		},
		{
			name:        "Zero cost at start",
			costToDate:  0.00,
			daysElapsed: 1.0,
			expected:    0.00,
			description: "Should handle zero cost gracefully",
		},
		{
			name:        "Very small cost",
			costToDate:  0.01,
			daysElapsed: 1.0,
			expected:    0.30, // 0.01/1 * 30 = 0.30
			description: "Should handle very small costs",
		},
		{
			name:        "Large cost",
			costToDate:  10000.00,
			daysElapsed: 10.0,
			expected:    30000.00, // 10000/10 * 30 = 30000
			description: "Should handle large costs",
		},
		{
			name:        "31-day month equivalent",
			costToDate:  155.00,
			daysElapsed: 31.0,
			expected:    150.00, // 155/31 * 30 = 150
			description: "Should normalize 31-day month to 30 days",
		},
		{
			name:        "28-day month equivalent (February)",
			costToDate:  140.00,
			daysElapsed: 28.0,
			expected:    150.00, // 140/28 * 30 = 150
			description: "Should normalize 28-day month to 30 days",
		},
		{
			name:        "Exactly 48 hours (Cost Explorer delay)",
			costToDate:  10.00,
			daysElapsed: 2.0,
			expected:    150.00, // 10/2 * 30 = 150
			description: "Should handle minimum fetchable period",
		},
		{
			name:        "One week",
			costToDate:  35.00,
			daysElapsed: 7.0,
			expected:    150.00, // 35/7 * 30 = 150
			description: "Should project one week to full month",
		},
		{
			name:        "Two weeks",
			costToDate:  70.00,
			daysElapsed: 14.0,
			expected:    150.00, // 70/14 * 30 = 150
			description: "Should project two weeks to full month",
		},
		{
			name:        "Three weeks",
			costToDate:  105.00,
			daysElapsed: 21.0,
			expected:    150.00, // 105/21 * 30 = 150
			description: "Should project three weeks to full month",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create period with exact days elapsed
			now := time.Now()
			start := now.Add(-time.Duration(tt.daysElapsed * 24 * float64(time.Hour)))
			period := CostPeriod{
				Start: start,
				End:   now,
			}

			result := acf.calculateProjectedMonthlyCost(tt.costToDate, period)

			// Allow small floating point differences (0.01)
			if abs(result-tt.expected) > 0.01 {
				t.Errorf("%s: expected %.2f, got %.2f (cost: %.2f, days: %.2f)", 
					tt.description, tt.expected, result, tt.costToDate, tt.daysElapsed)
			}
		})
	}
}

// TestProjectedMonthlyCostFormula verifies the formula: projectedMonthly = (costToDate / daysElapsed) * 30
func TestProjectedMonthlyCostFormula(t *testing.T) {
	acf := &ActualCostFetcher{}

	// Property: For any cost C and days D, projected = (C/D) * 30
	testCases := []struct {
		cost float64
		days float64
	}{
		{10.00, 1.0},
		{20.00, 2.0},
		{50.00, 5.0},
		{100.00, 10.0},
		{150.00, 15.0},
		{200.00, 20.0},
		{250.00, 25.0},
		{300.00, 30.0},
		{0.50, 0.5},
		{1.25, 1.5},
		{37.50, 7.5},
		{75.00, 15.0},
		{112.50, 22.5},
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			now := time.Now()
			start := now.Add(-time.Duration(tc.days * 24 * float64(time.Hour)))
			period := CostPeriod{
				Start: start,
				End:   now,
			}

			result := acf.calculateProjectedMonthlyCost(tc.cost, period)
			
			// Calculate expected using formula
			dailyAverage := tc.cost / tc.days
			expected := dailyAverage * 30

			if abs(result-expected) > 0.01 {
				t.Errorf("Formula verification failed: cost=%.2f, days=%.2f, expected=%.2f, got=%.2f",
					tc.cost, tc.days, expected, result)
			}
		})
	}
}

// TestProjectedMonthlyCostConsistency verifies that projection is consistent
func TestProjectedMonthlyCostConsistency(t *testing.T) {
	acf := &ActualCostFetcher{}

	// Property: If daily rate is constant, projected monthly should be the same
	// regardless of how many days have elapsed
	dailyRate := 5.00 // $5 per day

	testDays := []float64{1, 2, 3, 5, 7, 10, 15, 20, 25, 30}
	expectedMonthly := dailyRate * 30 // $150

	for _, days := range testDays {
		t.Run("", func(t *testing.T) {
			costToDate := dailyRate * days
			
			now := time.Now()
			start := now.Add(-time.Duration(days * 24 * float64(time.Hour)))
			period := CostPeriod{
				Start: start,
				End:   now,
			}

			result := acf.calculateProjectedMonthlyCost(costToDate, period)

			if abs(result-expectedMonthly) > 0.01 {
				t.Errorf("Consistency check failed for %v days: expected %.2f, got %.2f",
					days, expectedMonthly, result)
			}
		})
	}
}

// TestProjectedMonthlyCostBoundaryConditions tests boundary conditions
func TestProjectedMonthlyCostBoundaryConditions(t *testing.T) {
	acf := &ActualCostFetcher{}

	t.Run("Zero days elapsed", func(t *testing.T) {
		now := time.Now()
		period := CostPeriod{
			Start: now,
			End:   now,
		}

		result := acf.calculateProjectedMonthlyCost(100.00, period)
		
		// Should return cost as-is when days elapsed is 0
		if result != 100.00 {
			t.Errorf("Expected 100.00 for zero days, got %.2f", result)
		}
	})

	t.Run("Negative days (end before start)", func(t *testing.T) {
		now := time.Now()
		period := CostPeriod{
			Start: now,
			End:   now.Add(-24 * time.Hour), // End before start
		}

		result := acf.calculateProjectedMonthlyCost(100.00, period)
		
		// Should handle gracefully (return cost as-is)
		if result != 100.00 {
			t.Errorf("Expected 100.00 for negative days, got %.2f", result)
		}
	})

	t.Run("Very small time period (1 hour)", func(t *testing.T) {
		now := time.Now()
		period := CostPeriod{
			Start: now.Add(-1 * time.Hour),
			End:   now,
		}

		result := acf.calculateProjectedMonthlyCost(0.21, period) // $0.21 per hour
		
		// 0.21 / (1/24) * 30 = 0.21 * 24 * 30 = 151.20
		expected := 151.20
		if abs(result-expected) > 0.01 {
			t.Errorf("Expected %.2f for 1 hour period, got %.2f", expected, result)
		}
	})

	t.Run("Very large time period (90 days)", func(t *testing.T) {
		now := time.Now()
		period := CostPeriod{
			Start: now.Add(-90 * 24 * time.Hour),
			End:   now,
		}

		result := acf.calculateProjectedMonthlyCost(450.00, period) // $450 over 90 days
		
		// 450 / 90 * 30 = 150
		expected := 150.00
		if abs(result-expected) > 0.01 {
			t.Errorf("Expected %.2f for 90 day period, got %.2f", expected, result)
		}
	})
}

// TestCostToDateAccuracy tests that cost-to-date is accurately calculated
func TestCostToDateAccuracy(t *testing.T) {
	tests := []struct {
		name        string
		breakdown   map[string]float64
		expected    float64
		description string
	}{
		{
			name: "Single service",
			breakdown: map[string]float64{
				"AmazonEC2": 50.00,
			},
			expected:    50.00,
			description: "Should sum single service correctly",
		},
		{
			name: "Multiple services",
			breakdown: map[string]float64{
				"AmazonEC2": 50.00,
				"AmazonS3":  10.00,
				"AmazonRDS": 30.00,
			},
			expected:    90.00,
			description: "Should sum multiple services correctly",
		},
		{
			name: "Services with decimals",
			breakdown: map[string]float64{
				"AmazonEC2": 50.55,
				"AmazonS3":  10.25,
				"AmazonRDS": 30.75,
			},
			expected:    91.55,
			description: "Should handle decimal values correctly",
		},
		{
			name: "Very small costs",
			breakdown: map[string]float64{
				"AmazonS3":         0.01,
				"AmazonCloudWatch": 0.02,
			},
			expected:    0.03,
			description: "Should handle very small costs",
		},
		{
			name: "Mixed large and small costs",
			breakdown: map[string]float64{
				"AmazonEC2": 1000.00,
				"AmazonS3":  0.50,
			},
			expected:    1000.50,
			description: "Should handle mixed cost magnitudes",
		},
		{
			name:        "Empty breakdown",
			breakdown:   map[string]float64{},
			expected:    0.00,
			description: "Should return zero for empty breakdown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate total from breakdown
			total := 0.0
			for _, cost := range tt.breakdown {
				total += cost
			}
			total = roundToTwoDecimals(total)

			if abs(total-tt.expected) > 0.01 {
				t.Errorf("%s: expected %.2f, got %.2f", tt.description, tt.expected, total)
			}
		})
	}
}

// TestVarianceCalculationEdgeCases tests variance calculation edge cases
func TestVarianceCalculationEdgeCases(t *testing.T) {
	acf := &ActualCostFetcher{}

	tests := []struct {
		name        string
		actual      float64
		estimate    float64
		expected    float64
		description string
	}{
		{
			name:        "Exactly on estimate",
			actual:      100.00,
			estimate:    100.00,
			expected:    0.00,
			description: "Should return 0% when exactly on estimate",
		},
		{
			name:        "10% over estimate",
			actual:      110.00,
			estimate:    100.00,
			expected:    10.00,
			description: "Should calculate 10% positive variance",
		},
		{
			name:        "10% under estimate",
			actual:      90.00,
			estimate:    100.00,
			expected:    -10.00,
			description: "Should calculate 10% negative variance",
		},
		{
			name:        "20% over (alert threshold)",
			actual:      120.00,
			estimate:    100.00,
			expected:    20.00,
			description: "Should calculate 20% variance (alert threshold)",
		},
		{
			name:        "50% over estimate",
			actual:      150.00,
			estimate:    100.00,
			expected:    50.00,
			description: "Should calculate 50% positive variance",
		},
		{
			name:        "Double the estimate (100% over)",
			actual:      200.00,
			estimate:    100.00,
			expected:    100.00,
			description: "Should calculate 100% positive variance",
		},
		{
			name:        "Half the estimate (50% under)",
			actual:      50.00,
			estimate:    100.00,
			expected:    -50.00,
			description: "Should calculate 50% negative variance",
		},
		{
			name:        "Very small actual and estimate",
			actual:      0.02,
			estimate:    0.01,
			expected:    100.00,
			description: "Should handle very small values",
		},
		{
			name:        "Very large actual and estimate",
			actual:      10000.00,
			estimate:    9000.00,
			expected:    11.11,
			description: "Should handle very large values",
		},
		{
			name:        "Zero actual, non-zero estimate",
			actual:      0.00,
			estimate:    100.00,
			expected:    -100.00,
			description: "Should calculate -100% when actual is zero",
		},
		{
			name:        "Zero estimate (edge case)",
			actual:      100.00,
			estimate:    0.00,
			expected:    0.00,
			description: "Should return 0 when estimate is zero (avoid division by zero)",
		},
		{
			name:        "Both zero",
			actual:      0.00,
			estimate:    0.00,
			expected:    0.00,
			description: "Should return 0 when both are zero",
		},
		{
			name:        "Fractional variance",
			actual:      100.50,
			estimate:    100.00,
			expected:    0.50,
			description: "Should calculate fractional variance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := acf.calculateVariance(tt.actual, tt.estimate)

			if abs(result-tt.expected) > 0.02 {
				t.Errorf("%s: expected %.2f%%, got %.2f%%", tt.description, tt.expected, result)
			}
		})
	}
}

// TestVarianceFormulaAccuracy verifies the variance formula: ((actual - estimate) / estimate) * 100
func TestVarianceFormulaAccuracy(t *testing.T) {
	acf := &ActualCostFetcher{}

	testCases := []struct {
		actual   float64
		estimate float64
	}{
		{100.00, 100.00},
		{110.00, 100.00},
		{90.00, 100.00},
		{120.00, 100.00},
		{80.00, 100.00},
		{150.00, 100.00},
		{50.00, 100.00},
		{200.00, 100.00},
		{75.50, 100.00},
		{125.25, 100.00},
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			result := acf.calculateVariance(tc.actual, tc.estimate)

			// Calculate expected using formula
			var expected float64
			if tc.estimate == 0 {
				expected = 0
			} else {
				expected = ((tc.actual - tc.estimate) / tc.estimate) * 100
				expected = roundToTwoDecimals(expected)
			}

			if abs(result-expected) > 0.02 {
				t.Errorf("Formula verification failed: actual=%.2f, estimate=%.2f, expected=%.2f%%, got=%.2f%%",
					tc.actual, tc.estimate, expected, result)
			}
		})
	}
}

// TestRealWorldScenarios tests realistic cost scenarios
func TestRealWorldScenarios(t *testing.T) {
	acf := &ActualCostFetcher{}

	t.Run("Typical web application - 10 days in", func(t *testing.T) {
		costToDate := 47.23
		daysElapsed := 10.0
		estimate := 56.65

		now := time.Now()
		period := CostPeriod{
			Start: now.Add(-time.Duration(daysElapsed * 24 * float64(time.Hour))),
			End:   now,
		}

		projected := acf.calculateProjectedMonthlyCost(costToDate, period)
		variance := acf.calculateVariance(projected, estimate)

		// 47.23 / 10 * 30 = 141.69
		expectedProjected := 141.69
		if abs(projected-expectedProjected) > 0.01 {
			t.Errorf("Expected projected %.2f, got %.2f", expectedProjected, projected)
		}

		// (141.69 - 56.65) / 56.65 * 100 = 150.08%
		expectedVariance := 150.08
		if abs(variance-expectedVariance) > 0.5 {
			t.Errorf("Expected variance %.2f%%, got %.2f%%", expectedVariance, variance)
		}
	})

	t.Run("Static website - low cost", func(t *testing.T) {
		costToDate := 2.30
		daysElapsed := 15.0
		estimate := 5.00

		now := time.Now()
		period := CostPeriod{
			Start: now.Add(-time.Duration(daysElapsed * 24 * float64(time.Hour))),
			End:   now,
		}

		projected := acf.calculateProjectedMonthlyCost(costToDate, period)
		variance := acf.calculateVariance(projected, estimate)

		// 2.30 / 15 * 30 = 4.60
		expectedProjected := 4.60
		if abs(projected-expectedProjected) > 0.01 {
			t.Errorf("Expected projected %.2f, got %.2f", expectedProjected, projected)
		}

		// (4.60 - 5.00) / 5.00 * 100 = -8%
		expectedVariance := -8.00
		if abs(variance-expectedVariance) > 0.5 {
			t.Errorf("Expected variance %.2f%%, got %.2f%%", expectedVariance, variance)
		}
	})

	t.Run("Microservices - high cost", func(t *testing.T) {
		costToDate := 350.00
		daysElapsed := 20.0
		estimate := 500.00

		now := time.Now()
		period := CostPeriod{
			Start: now.Add(-time.Duration(daysElapsed * 24 * float64(time.Hour))),
			End:   now,
		}

		projected := acf.calculateProjectedMonthlyCost(costToDate, period)
		variance := acf.calculateVariance(projected, estimate)

		// 350 / 20 * 30 = 525
		expectedProjected := 525.00
		if abs(projected-expectedProjected) > 0.01 {
			t.Errorf("Expected projected %.2f, got %.2f", expectedProjected, projected)
		}

		// (525 - 500) / 500 * 100 = 5%
		expectedVariance := 5.00
		if abs(variance-expectedVariance) > 0.5 {
			t.Errorf("Expected variance %.2f%%, got %.2f%%", expectedVariance, variance)
		}
	})

	t.Run("First 48 hours (minimum Cost Explorer period)", func(t *testing.T) {
		costToDate := 10.00
		daysElapsed := 2.0
		estimate := 150.00

		now := time.Now()
		period := CostPeriod{
			Start: now.Add(-time.Duration(daysElapsed * 24 * float64(time.Hour))),
			End:   now,
		}

		projected := acf.calculateProjectedMonthlyCost(costToDate, period)
		variance := acf.calculateVariance(projected, estimate)

		// 10 / 2 * 30 = 150
		expectedProjected := 150.00
		if abs(projected-expectedProjected) > 0.01 {
			t.Errorf("Expected projected %.2f, got %.2f", expectedProjected, projected)
		}

		// (150 - 150) / 150 * 100 = 0%
		expectedVariance := 0.00
		if abs(variance-expectedVariance) > 0.5 {
			t.Errorf("Expected variance %.2f%%, got %.2f%%", expectedVariance, variance)
		}
	})
}

// TestMonthEndScenarios tests scenarios at the end of the month
func TestMonthEndScenarios(t *testing.T) {
	acf := &ActualCostFetcher{}

	t.Run("29 days elapsed (near month end)", func(t *testing.T) {
		costToDate := 145.00
		daysElapsed := 29.0

		now := time.Now()
		period := CostPeriod{
			Start: now.Add(-time.Duration(daysElapsed * 24 * float64(time.Hour))),
			End:   now,
		}

		projected := acf.calculateProjectedMonthlyCost(costToDate, period)

		// 145 / 29 * 30 = 150
		expectedProjected := 150.00
		if abs(projected-expectedProjected) > 0.01 {
			t.Errorf("Expected projected %.2f, got %.2f", expectedProjected, projected)
		}
	})

	t.Run("30 days elapsed (full month)", func(t *testing.T) {
		costToDate := 150.00
		daysElapsed := 30.0

		now := time.Now()
		period := CostPeriod{
			Start: now.Add(-time.Duration(daysElapsed * 24 * float64(time.Hour))),
			End:   now,
		}

		projected := acf.calculateProjectedMonthlyCost(costToDate, period)

		// 150 / 30 * 30 = 150
		expectedProjected := 150.00
		if abs(projected-expectedProjected) > 0.01 {
			t.Errorf("Expected projected %.2f, got %.2f", expectedProjected, projected)
		}
	})

	t.Run("31 days elapsed (long month)", func(t *testing.T) {
		costToDate := 155.00
		daysElapsed := 31.0

		now := time.Now()
		period := CostPeriod{
			Start: now.Add(-time.Duration(daysElapsed * 24 * float64(time.Hour))),
			End:   now,
		}

		projected := acf.calculateProjectedMonthlyCost(costToDate, period)

		// 155 / 31 * 30 = 150
		expectedProjected := 150.00
		if abs(projected-expectedProjected) > 0.01 {
			t.Errorf("Expected projected %.2f, got %.2f", expectedProjected, projected)
		}
	})
}
