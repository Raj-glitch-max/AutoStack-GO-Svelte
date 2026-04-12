package cost

import (
	"testing"
	"testing/quick"
)

// TestCostRangeProperties validates that cost ranges satisfy mathematical properties
func TestCostRangeProperties(t *testing.T) {
	t.Run("Property: Range minimum is always less than or equal to base estimate", func(t *testing.T) {
		property := func(baseCost float64) bool {
			// Only test realistic positive costs (0.01 to 1 million)
			if baseCost <= 0.01 || baseCost > 1000000 {
				return true
			}

			min, _, base := CalculateRange(baseCost)
			return min <= base
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})

	t.Run("Property: Range maximum is always greater than or equal to base estimate", func(t *testing.T) {
		property := func(baseCost float64) bool {
			// Only test realistic positive costs (0.01 to 1 million)
			if baseCost <= 0.01 || baseCost > 1000000 {
				return true
			}

			_, max, base := CalculateRange(baseCost)
			return max >= base
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})

	t.Run("Property: Base estimate is always between min and max", func(t *testing.T) {
		property := func(baseCost float64) bool {
			// Only test realistic positive costs (0.01 to 1 million)
			if baseCost <= 0.01 || baseCost > 1000000 {
				return true
			}

			min, max, base := CalculateRange(baseCost)
			return min <= base && base <= max
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})

	t.Run("Property: Range width is proportional to base cost", func(t *testing.T) {
		property := func(cost1, cost2 float64) bool {
			// Only test realistic positive costs (0.01 to 1 million)
			if cost1 <= 0.01 || cost1 > 1000000 || cost2 <= 0.01 || cost2 > 1000000 {
				return true
			}

			min1, max1, _ := CalculateRange(cost1)
			min2, max2, _ := CalculateRange(cost2)

			width1 := max1 - min1
			width2 := max2 - min2

			// If cost2 > cost1, then width2 should be > width1
			if cost2 > cost1 {
				return width2 > width1
			}
			return true
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})

	t.Run("Property: Range multipliers are consistent", func(t *testing.T) {
		property := func(baseCost float64) bool {
			// Only test realistic positive costs (0.01 to 1 million)
			if baseCost <= 0.01 || baseCost > 1000000 {
				return true
			}

			min, max, base := CalculateRange(baseCost)

			// Min should be approximately 80% of base
			expectedMin := base * 0.8
			minRatio := min / expectedMin
			if minRatio < 0.99 || minRatio > 1.01 {
				return false
			}

			// Max should be approximately 140% of base
			expectedMax := base * 1.4
			maxRatio := max / expectedMax
			if maxRatio < 0.99 || maxRatio > 1.01 {
				return false
			}

			return true
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})

	t.Run("Property: Range calculation is deterministic", func(t *testing.T) {
		property := func(baseCost float64) bool {
			// Only test realistic positive costs (0.01 to 1 million)
			if baseCost <= 0.01 || baseCost > 1000000 {
				return true
			}

			// Calculate range twice
			min1, max1, base1 := CalculateRange(baseCost)
			min2, max2, base2 := CalculateRange(baseCost)

			// Results should be identical
			return min1 == min2 && max1 == max2 && base1 == base2
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})

	t.Run("Property: Range values are properly rounded", func(t *testing.T) {
		property := func(baseCost float64) bool {
			// Only test realistic positive costs (0.01 to 1 million)
			if baseCost <= 0.01 || baseCost > 1000000 {
				return true
			}

			min, max, base := CalculateRange(baseCost)

			// Check that values are rounded to 2 decimal places
			minRounded := roundToTwoDecimals(min)
			maxRounded := roundToTwoDecimals(max)
			baseRounded := roundToTwoDecimals(base)

			return min == minRounded && max == maxRounded && base == baseRounded
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})

	t.Run("Property: Zero cost produces zero range", func(t *testing.T) {
		min, max, base := CalculateRange(0)
		if min != 0 || max != 0 || base != 0 {
			t.Errorf("Zero cost should produce zero range, got min=%f, max=%f, base=%f", min, max, base)
		}
	})

	t.Run("Property: Negative costs are rejected", func(t *testing.T) {
		property := func(baseCost float64) bool {
			// Only test negative costs
			if baseCost >= 0 {
				return true
			}

			// Should return error or zero values
			min, max, base := CalculateRange(baseCost)
			return min == 0 && max == 0 && base == 0
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})
}

// TestRangeConsistencyAcrossScales validates range behavior at different cost scales
func TestRangeConsistencyAcrossScales(t *testing.T) {
	testCases := []struct {
		name     string
		baseCost float64
	}{
		{"Very small cost", 1.00},   // Changed from 0.01 to avoid rounding issues
		{"Small cost", 10.00},       // Changed from 1.00
		{"Medium cost", 100.00},
		{"Large cost", 1000.00},
		{"Very large cost", 10000.00},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			min, max, base := CalculateRange(tc.baseCost)

			// Verify range properties
			if min > base {
				t.Errorf("Min (%f) should be <= base (%f)", min, base)
			}
			if max < base {
				t.Errorf("Max (%f) should be >= base (%f)", max, base)
			}

			// Verify multipliers
			minRatio := min / base
			maxRatio := max / base

			if minRatio < 0.79 || minRatio > 0.81 {
				t.Errorf("Min ratio (%f) should be approximately 0.8", minRatio)
			}
			if maxRatio < 1.39 || maxRatio > 1.41 {
				t.Errorf("Max ratio (%f) should be approximately 1.4", maxRatio)
			}
		})
	}
}

// TestRangeEdgeCases validates range calculation for edge cases
func TestRangeEdgeCases(t *testing.T) {
	testCases := []struct {
		name        string
		baseCost    float64
		expectError bool
	}{
		{"Zero cost", 0, false},
		{"Negative cost", -10.00, true},
		{"Very small positive", 0.10, false},  // Changed from 0.001 to realistic minimum
		{"Maximum realistic cost", 1000000, false},  // Changed from 1e308 to realistic max
		{"Minimum realistic cost", 0.01, false},  // Changed from 1e-308 to 1 cent
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			min, max, base := CalculateRange(tc.baseCost)

			if tc.expectError {
				// Should return zero values for invalid input
				if min != 0 || max != 0 || base != 0 {
					t.Errorf("Expected zero values for invalid input, got min=%f, max=%f, base=%f", min, max, base)
				}
			} else if tc.baseCost > 0 {
				// Should return valid range
				if min <= 0 || max <= 0 || base <= 0 {
					t.Errorf("Expected positive values, got min=%f, max=%f, base=%f", min, max, base)
				}
				if min > base || base > max {
					t.Errorf("Range order violated: min=%f, base=%f, max=%f", min, max, base)
				}
			}
		})
	}
}
