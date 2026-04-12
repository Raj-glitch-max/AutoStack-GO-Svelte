package cost

import (
	"fmt"
)

// CostRange represents a cost estimate with uncertainty bounds
type CostRange struct {
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	Estimate float64 `json:"estimate"`
}

// CalculateRange calculates the cost range (min, max, base) for a given base cost
// Min range: estimate * 0.8 (20% below estimate)
// Max range: estimate * 1.4 (40% above estimate)
// All values are rounded to 2 decimal places
//
// **Validates**: AC-1.1 (System shows cost range not single point estimate)
// **Validates**: TR-2.2 (Calculate range with 20% margin)
// **Validates**: TR-2.4 (Round to 2 decimal places for display)
func CalculateRange(baseCost float64) (min, max, base float64) {
	// Reject negative costs
	if baseCost < 0 {
		return 0, 0, 0
	}

	// Handle zero cost
	if baseCost == 0 {
		return 0, 0, 0
	}

	// Round base cost first
	base = roundToTwoDecimals(baseCost)
	
	// Handle very small costs that round to zero
	if base == 0 {
		return 0, 0, 0
	}

	// Calculate range with specified margins
	min = roundToTwoDecimals(base * 0.8)  // 20% below estimate
	max = roundToTwoDecimals(base * 1.4)  // 40% above estimate
	
	// Handle edge case where min rounds to zero but base doesn't
	if min == 0 && base > 0 {
		min = 0.01 // Minimum representable cost
	}

	return min, max, base
}

// CalculateCostRange calculates a CostRange struct from a base cost estimate
// This is a convenience wrapper around CalculateRange that returns a struct
//
// **Validates**: AC-1.1 (System shows cost range not single point estimate)
// **Validates**: TR-2.2 (Calculate range with 20% margin)
// **Validates**: TR-2.4 (Round to 2 decimal places for display)
func CalculateCostRange(baseCost float64) (*CostRange, error) {
	// Validate input
	if baseCost < 0 {
		return nil, fmt.Errorf("base cost cannot be negative: %f", baseCost)
	}

	min, max, estimate := CalculateRange(baseCost)

	return &CostRange{
		Min:      min,
		Max:      max,
		Estimate: estimate,
	}, nil
}

// AddRangeToCostEstimate adds min/max range to an existing CostEstimateWithRange
// This is useful for calculators that need to add range information to their estimates
//
// **Validates**: AC-1.1 (System shows cost range not single point estimate)
// **Validates**: TR-2.2 (Calculate range with 20% margin)
func AddRangeToCostEstimate(estimate *CostEstimateWithRange) error {
	if estimate == nil {
		return fmt.Errorf("estimate cannot be nil")
	}

	if estimate.Estimate < 0 {
		return fmt.Errorf("estimate value cannot be negative: %f", estimate.Estimate)
	}

	min, max, base := CalculateRange(estimate.Estimate)
	estimate.RangeMin = min
	estimate.RangeMax = max
	estimate.Estimate = base

	return nil
}

// ValidateRange checks if a cost range satisfies the required properties
// Returns an error if the range is invalid
//
// **Validates**: Cost Calculation Property 2 (Cost ranges must satisfy min ≤ estimate ≤ max)
func ValidateRange(costRange *CostRange) error {
	if costRange == nil {
		return fmt.Errorf("cost range cannot be nil")
	}

	// Check for negative values
	if costRange.Min < 0 {
		return fmt.Errorf("min cost cannot be negative: %f", costRange.Min)
	}
	if costRange.Max < 0 {
		return fmt.Errorf("max cost cannot be negative: %f", costRange.Max)
	}
	if costRange.Estimate < 0 {
		return fmt.Errorf("estimate cannot be negative: %f", costRange.Estimate)
	}

	// Check range ordering: min ≤ estimate ≤ max
	if costRange.Min > costRange.Estimate {
		return fmt.Errorf("min (%f) cannot be greater than estimate (%f)", costRange.Min, costRange.Estimate)
	}
	if costRange.Estimate > costRange.Max {
		return fmt.Errorf("estimate (%f) cannot be greater than max (%f)", costRange.Estimate, costRange.Max)
	}

	return nil
}

// CalculateRangeForBreakdown calculates cost ranges for a breakdown of services
// This ensures all service costs have consistent range calculations
//
// **Validates**: TR-2.2 (Calculate range with 20% margin)
// **Validates**: TR-2.4 (Round to 2 decimal places for display)
func CalculateRangeForBreakdown(breakdown map[string]float64) (map[string]*CostRange, error) {
	if breakdown == nil {
		return nil, fmt.Errorf("breakdown cannot be nil")
	}

	result := make(map[string]*CostRange)

	for service, cost := range breakdown {
		if cost < 0 {
			return nil, fmt.Errorf("cost for service %s cannot be negative: %f", service, cost)
		}

		costRange, err := CalculateCostRange(cost)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate range for service %s: %w", service, err)
		}

		result[service] = costRange
	}

	return result, nil
}
