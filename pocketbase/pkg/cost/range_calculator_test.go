package cost

import (
	"math"
	"testing"
)

// TestCalculateRange tests the basic CalculateRange function
func TestCalculateRange(t *testing.T) {
	testCases := []struct {
		name        string
		baseCost    float64
		expectedMin float64
		expectedMax float64
		expectedEst float64
	}{
		{
			name:        "Standard cost",
			baseCost:    100.00,
			expectedMin: 80.00,
			expectedMax: 140.00,
			expectedEst: 100.00,
		},
		{
			name:        "Small cost",
			baseCost:    10.00,
			expectedMin: 8.00,
			expectedMax: 14.00,
			expectedEst: 10.00,
		},
		{
			name:        "Large cost",
			baseCost:    1000.00,
			expectedMin: 800.00,
			expectedMax: 1400.00,
			expectedEst: 1000.00,
		},
		{
			name:        "Fractional cost",
			baseCost:    56.65,
			expectedMin: 45.32,
			expectedMax: 79.31,
			expectedEst: 56.65,
		},
		{
			name:        "Very small cost",
			baseCost:    0.01,
			expectedMin: 0.01,
			expectedMax: 0.01,
			expectedEst: 0.01,
		},
		{
			name:        "Zero cost",
			baseCost:    0,
			expectedMin: 0,
			expectedMax: 0,
			expectedEst: 0,
		},
		{
			name:        "Negative cost",
			baseCost:    -10.00,
			expectedMin: 0,
			expectedMax: 0,
			expectedEst: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			min, max, est := CalculateRange(tc.baseCost)

			if min != tc.expectedMin {
				t.Errorf("Expected min=%f, got %f", tc.expectedMin, min)
			}
			if max != tc.expectedMax {
				t.Errorf("Expected max=%f, got %f", tc.expectedMax, max)
			}
			if est != tc.expectedEst {
				t.Errorf("Expected estimate=%f, got %f", tc.expectedEst, est)
			}
		})
	}
}

// TestCalculateCostRange tests the struct-based CalculateCostRange function
func TestCalculateCostRange(t *testing.T) {
	t.Run("Valid positive cost", func(t *testing.T) {
		costRange, err := CalculateCostRange(100.00)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if costRange.Min != 80.00 {
			t.Errorf("Expected min=80.00, got %f", costRange.Min)
		}
		if costRange.Max != 140.00 {
			t.Errorf("Expected max=140.00, got %f", costRange.Max)
		}
		if costRange.Estimate != 100.00 {
			t.Errorf("Expected estimate=100.00, got %f", costRange.Estimate)
		}
	})

	t.Run("Zero cost", func(t *testing.T) {
		costRange, err := CalculateCostRange(0)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if costRange.Min != 0 || costRange.Max != 0 || costRange.Estimate != 0 {
			t.Errorf("Expected all zeros, got min=%f, max=%f, estimate=%f",
				costRange.Min, costRange.Max, costRange.Estimate)
		}
	})

	t.Run("Negative cost returns error", func(t *testing.T) {
		_, err := CalculateCostRange(-10.00)
		if err == nil {
			t.Error("Expected error for negative cost, got nil")
		}
	})
}

// TestAddRangeToCostEstimate tests adding range to existing estimates
func TestAddRangeToCostEstimate(t *testing.T) {
	t.Run("Valid estimate", func(t *testing.T) {
		estimate := &CostEstimateWithRange{
			Estimate: 100.00,
			Currency: "USD",
		}

		err := AddRangeToCostEstimate(estimate)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if estimate.RangeMin != 80.00 {
			t.Errorf("Expected RangeMin=80.00, got %f", estimate.RangeMin)
		}
		if estimate.RangeMax != 140.00 {
			t.Errorf("Expected RangeMax=140.00, got %f", estimate.RangeMax)
		}
		if estimate.Estimate != 100.00 {
			t.Errorf("Expected Estimate=100.00, got %f", estimate.Estimate)
		}
	})

	t.Run("Nil estimate returns error", func(t *testing.T) {
		err := AddRangeToCostEstimate(nil)
		if err == nil {
			t.Error("Expected error for nil estimate, got nil")
		}
	})

	t.Run("Negative estimate returns error", func(t *testing.T) {
		estimate := &CostEstimateWithRange{
			Estimate: -10.00,
		}

		err := AddRangeToCostEstimate(estimate)
		if err == nil {
			t.Error("Expected error for negative estimate, got nil")
		}
	})
}

// TestValidateRange tests the range validation function
func TestValidateRange(t *testing.T) {
	t.Run("Valid range", func(t *testing.T) {
		costRange := &CostRange{
			Min:      80.00,
			Max:      140.00,
			Estimate: 100.00,
		}

		err := ValidateRange(costRange)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("Nil range returns error", func(t *testing.T) {
		err := ValidateRange(nil)
		if err == nil {
			t.Error("Expected error for nil range, got nil")
		}
	})

	t.Run("Negative min returns error", func(t *testing.T) {
		costRange := &CostRange{
			Min:      -10.00,
			Max:      140.00,
			Estimate: 100.00,
		}

		err := ValidateRange(costRange)
		if err == nil {
			t.Error("Expected error for negative min, got nil")
		}
	})

	t.Run("Negative max returns error", func(t *testing.T) {
		costRange := &CostRange{
			Min:      80.00,
			Max:      -140.00,
			Estimate: 100.00,
		}

		err := ValidateRange(costRange)
		if err == nil {
			t.Error("Expected error for negative max, got nil")
		}
	})

	t.Run("Negative estimate returns error", func(t *testing.T) {
		costRange := &CostRange{
			Min:      80.00,
			Max:      140.00,
			Estimate: -100.00,
		}

		err := ValidateRange(costRange)
		if err == nil {
			t.Error("Expected error for negative estimate, got nil")
		}
	})

	t.Run("Min greater than estimate returns error", func(t *testing.T) {
		costRange := &CostRange{
			Min:      150.00,
			Max:      200.00,
			Estimate: 100.00,
		}

		err := ValidateRange(costRange)
		if err == nil {
			t.Error("Expected error for min > estimate, got nil")
		}
	})

	t.Run("Estimate greater than max returns error", func(t *testing.T) {
		costRange := &CostRange{
			Min:      80.00,
			Max:      90.00,
			Estimate: 100.00,
		}

		err := ValidateRange(costRange)
		if err == nil {
			t.Error("Expected error for estimate > max, got nil")
		}
	})

	t.Run("Zero range is valid", func(t *testing.T) {
		costRange := &CostRange{
			Min:      0,
			Max:      0,
			Estimate: 0,
		}

		err := ValidateRange(costRange)
		if err != nil {
			t.Errorf("Expected no error for zero range, got: %v", err)
		}
	})
}

// TestCalculateRangeForBreakdown tests range calculation for service breakdowns
func TestCalculateRangeForBreakdown(t *testing.T) {
	t.Run("Valid breakdown", func(t *testing.T) {
		breakdown := map[string]float64{
			"compute":    29.15,
			"networking": 16.20,
			"storage":    2.30,
			"transfer":   9.00,
		}

		result, err := CalculateRangeForBreakdown(breakdown)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(result) != len(breakdown) {
			t.Errorf("Expected %d services, got %d", len(breakdown), len(result))
		}

		// Check compute service
		if result["compute"].Estimate != 29.15 {
			t.Errorf("Expected compute estimate=29.15, got %f", result["compute"].Estimate)
		}
		if result["compute"].Min != 23.32 {
			t.Errorf("Expected compute min=23.32, got %f", result["compute"].Min)
		}
		if result["compute"].Max != 40.81 {
			t.Errorf("Expected compute max=40.81, got %f", result["compute"].Max)
		}
	})

	t.Run("Nil breakdown returns error", func(t *testing.T) {
		_, err := CalculateRangeForBreakdown(nil)
		if err == nil {
			t.Error("Expected error for nil breakdown, got nil")
		}
	})

	t.Run("Negative cost in breakdown returns error", func(t *testing.T) {
		breakdown := map[string]float64{
			"compute": 29.15,
			"storage": -2.30,
		}

		_, err := CalculateRangeForBreakdown(breakdown)
		if err == nil {
			t.Error("Expected error for negative cost, got nil")
		}
	})

	t.Run("Empty breakdown is valid", func(t *testing.T) {
		breakdown := map[string]float64{}

		result, err := CalculateRangeForBreakdown(breakdown)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(result) != 0 {
			t.Errorf("Expected empty result, got %d services", len(result))
		}
	})
}

// TestRangeRounding tests that all values are properly rounded to 2 decimal places
func TestRangeRounding(t *testing.T) {
	testCases := []struct {
		name     string
		baseCost float64
	}{
		{"Cost with many decimals", 123.456789},
		{"Cost requiring rounding up", 100.005},
		{"Cost requiring rounding down", 100.004},
		{"Very small cost", 0.001},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			min, max, est := CalculateRange(tc.baseCost)

			// Check that values have at most 2 decimal places
			if !hasAtMostTwoDecimals(min) {
				t.Errorf("Min %f has more than 2 decimal places", min)
			}
			if !hasAtMostTwoDecimals(max) {
				t.Errorf("Max %f has more than 2 decimal places", max)
			}
			if !hasAtMostTwoDecimals(est) {
				t.Errorf("Estimate %f has more than 2 decimal places", est)
			}
		})
	}
}

// TestRangeMargins tests that the margins are exactly 20% and 40%
func TestRangeMargins(t *testing.T) {
	testCases := []float64{10.00, 50.00, 100.00, 500.00, 1000.00}

	for _, baseCost := range testCases {
		t.Run("", func(t *testing.T) {
			min, max, est := CalculateRange(baseCost)

			// Calculate actual margins (allowing for rounding)
			minMargin := (est - min) / est
			maxMargin := (max - est) / est

			// Check min margin is approximately 20% (0.2)
			if math.Abs(minMargin-0.2) > 0.01 {
				t.Errorf("Min margin should be ~20%%, got %.2f%% for cost %f", minMargin*100, baseCost)
			}

			// Check max margin is approximately 40% (0.4)
			if math.Abs(maxMargin-0.4) > 0.01 {
				t.Errorf("Max margin should be ~40%%, got %.2f%% for cost %f", maxMargin*100, baseCost)
			}
		})
	}
}

// TestRangeCalculatorConsistency tests that the same input always produces the same output
func TestRangeCalculatorConsistency(t *testing.T) {
	baseCost := 123.45

	// Calculate range multiple times
	min1, max1, est1 := CalculateRange(baseCost)
	min2, max2, est2 := CalculateRange(baseCost)
	min3, max3, est3 := CalculateRange(baseCost)

	// All results should be identical
	if min1 != min2 || min1 != min3 {
		t.Errorf("Min values are inconsistent: %f, %f, %f", min1, min2, min3)
	}
	if max1 != max2 || max1 != max3 {
		t.Errorf("Max values are inconsistent: %f, %f, %f", max1, max2, max3)
	}
	if est1 != est2 || est1 != est3 {
		t.Errorf("Estimate values are inconsistent: %f, %f, %f", est1, est2, est3)
	}
}

// TestRangeOrderInvariant tests that min ≤ estimate ≤ max always holds
func TestRangeOrderInvariant(t *testing.T) {
	testCases := []float64{0.01, 1.00, 10.00, 100.00, 1000.00, 10000.00}

	for _, baseCost := range testCases {
		t.Run("", func(t *testing.T) {
			min, max, est := CalculateRange(baseCost)

			if min > est {
				t.Errorf("Min (%f) should be <= estimate (%f) for cost %f", min, est, baseCost)
			}
			if est > max {
				t.Errorf("Estimate (%f) should be <= max (%f) for cost %f", est, max, baseCost)
			}
		})
	}
}

// Helper function to check if a float has at most 2 decimal places
func hasAtMostTwoDecimals(value float64) bool {
	// Multiply by 100 and check if it's close to an integer
	scaled := value * 100
	rounded := math.Round(scaled)
	return math.Abs(scaled-rounded) < 0.0001
}
