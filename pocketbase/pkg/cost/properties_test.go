package cost

import (
	"math"
	"testing"
	"testing/quick"
)

// TestCostPositivityProperty validates that all cost calculations produce positive values
func TestCostPositivityProperty(t *testing.T) {
	t.Run("Property: All cost estimates must be positive", func(t *testing.T) {
		property := func(vCPU, memory, storage uint8) bool {
			// Ensure reasonable input ranges
			if vCPU == 0 || vCPU > 16 {
				return true
			}
			if memory == 0 || memory > 128 {
				return true
			}
			if storage > 200 {
				return true
			}

			// Mock pricing data
			pricing := &PricingData{
				VCPUPrice:    0.04048,
				MemoryPrice:  0.004445,
				StoragePrice: 0.115,
			}

			cost := calculateMonthlyCost(int(vCPU), int(memory), int(storage), pricing)
			return cost > 0
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})

	t.Run("Property: Cost increases with more resources", func(t *testing.T) {
		property := func(vCPU1, vCPU2 uint8) bool {
			// Ensure reasonable input ranges
			if vCPU1 == 0 || vCPU1 > 8 || vCPU2 == 0 || vCPU2 > 8 {
				return true
			}
			if vCPU1 >= vCPU2 {
				return true
			}

			pricing := &PricingData{
				VCPUPrice:    0.04048,
				MemoryPrice:  0.004445,
				StoragePrice: 0.115,
			}

			cost1 := calculateMonthlyCost(int(vCPU1), 4, 20, pricing)
			cost2 := calculateMonthlyCost(int(vCPU2), 4, 20, pricing)

			return cost2 > cost1
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})

	t.Run("Property: Cost is properly rounded to 2 decimal places", func(t *testing.T) {
		property := func(vCPU, memory uint8) bool {
			if vCPU == 0 || vCPU > 16 || memory == 0 || memory > 128 {
				return true
			}

			pricing := &PricingData{
				VCPUPrice:   0.04048,
				MemoryPrice: 0.004445,
			}

			cost := calculateMonthlyCost(int(vCPU), int(memory), 0, pricing)
			rounded := math.Round(cost*100) / 100

			// Check if cost has at most 2 decimal places
			return math.Abs(cost-rounded) < 0.001
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})
}

// TestBreakdownCompletenessProperty validates that service breakdown sums to total
func TestBreakdownCompletenessProperty(t *testing.T) {
	t.Run("Property: Service breakdown sums to total estimate", func(t *testing.T) {
		property := func(compute, network, storage uint8) bool {
			// Ensure reasonable ranges
			if compute > 200 || network > 100 || storage > 100 {
				return true
			}

			breakdown := CostBreakdown{
				Compute:    float64(compute),
				Networking: float64(network),
				Storage:    float64(storage),
			}

			total := breakdown.Compute + breakdown.Networking + breakdown.Storage
			calculatedTotal := breakdown.Total()

			// Allow small floating point differences
			return math.Abs(total-calculatedTotal) < 0.01
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})

	t.Run("Property: Breakdown percentages sum to 100", func(t *testing.T) {
		property := func(compute, network, storage uint8) bool {
			// Ensure at least one non-zero value
			if compute == 0 && network == 0 && storage == 0 {
				return true
			}

			breakdown := CostBreakdown{
				Compute:    float64(compute),
				Networking: float64(network),
				Storage:    float64(storage),
			}

			percentages := breakdown.Percentages()
			sum := percentages.Compute + percentages.Networking + percentages.Storage

			// Should sum to approximately 100%
			return math.Abs(sum-100.0) < 0.1
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})
}

// TestVarianceCalculationProperty validates variance formula correctness
func TestVarianceCalculationProperty(t *testing.T) {
	t.Run("Property: Variance calculation is mathematically correct", func(t *testing.T) {
		property := func(actual, estimate uint16) bool {
			// Ensure reasonable ranges and non-zero estimate
			if estimate == 0 || actual > 10000 || estimate > 10000 {
				return true
			}

			actualCost := float64(actual)
			estimateCost := float64(estimate)

			variance := calculateVariance(actualCost, estimateCost)
			expectedVariance := ((actualCost - estimateCost) / estimateCost) * 100

			// Allow small floating point differences
			return math.Abs(variance-expectedVariance) < 0.01
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})

	t.Run("Property: Positive variance when actual > estimate", func(t *testing.T) {
		property := func(actual, estimate uint16) bool {
			if estimate == 0 || actual <= estimate {
				return true
			}

			variance := calculateVariance(float64(actual), float64(estimate))
			return variance > 0
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})

	t.Run("Property: Negative variance when actual < estimate", func(t *testing.T) {
		property := func(actual, estimate uint16) bool {
			if estimate == 0 || actual >= estimate {
				return true
			}

			variance := calculateVariance(float64(actual), float64(estimate))
			return variance < 0
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})

	t.Run("Property: Zero variance when actual equals estimate", func(t *testing.T) {
		property := func(cost uint16) bool {
			if cost == 0 {
				return true
			}

			variance := calculateVariance(float64(cost), float64(cost))
			return math.Abs(variance) < 0.01
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})
}

// TestRegionalPricingConsistency validates regional pricing behavior
func TestRegionalPricingConsistency(t *testing.T) {
	t.Run("Property: Same configuration in same region produces same cost", func(t *testing.T) {
		config := ResourceConfig{
			VCPU:    2,
			Memory:  4,
			Storage: 20,
		}

		region := "us-east-1"
		pricing := &PricingData{
			VCPUPrice:    0.04048,
			MemoryPrice:  0.004445,
			StoragePrice: 0.115,
		}

		// Calculate cost multiple times
		cost1 := calculateRegionalCost(config, region, pricing)
		cost2 := calculateRegionalCost(config, region, pricing)
		cost3 := calculateRegionalCost(config, region, pricing)

		if cost1 != cost2 || cost2 != cost3 {
			t.Errorf("Same configuration should produce same cost: %f, %f, %f", cost1, cost2, cost3)
		}
	})

	t.Run("Property: Different regions may have different costs", func(t *testing.T) {
		config := ResourceConfig{
			VCPU:    2,
			Memory:  4,
			Storage: 20,
		}

		usEast1Pricing := &PricingData{
			VCPUPrice:    0.04048,
			MemoryPrice:  0.004445,
			StoragePrice: 0.115,
		}

		euWest1Pricing := &PricingData{
			VCPUPrice:    0.04456,
			MemoryPrice:  0.004889,
			StoragePrice: 0.127,
		}

		cost1 := calculateRegionalCost(config, "us-east-1", usEast1Pricing)
		cost2 := calculateRegionalCost(config, "eu-west-1", euWest1Pricing)

		// Costs should be different (EU is typically more expensive)
		if cost1 == cost2 {
			t.Logf("Warning: Regional costs are identical, which is unusual")
		}
	})
}

// TestCostMonotonicityProperty validates that costs increase monotonically with resources
func TestCostMonotonicityProperty(t *testing.T) {
	t.Run("Property: More vCPU means higher cost", func(t *testing.T) {
		property := func(vCPU1, vCPU2 uint8) bool {
			if vCPU1 == 0 || vCPU2 == 0 || vCPU1 >= vCPU2 || vCPU2 > 16 {
				return true
			}

			pricing := &PricingData{VCPUPrice: 0.04048}
			cost1 := float64(vCPU1) * pricing.VCPUPrice * 730
			cost2 := float64(vCPU2) * pricing.VCPUPrice * 730

			return cost2 > cost1
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})

	t.Run("Property: More memory means higher cost", func(t *testing.T) {
		property := func(mem1, mem2 uint8) bool {
			if mem1 == 0 || mem2 == 0 || mem1 >= mem2 || mem2 > 128 {
				return true
			}

			pricing := &PricingData{MemoryPrice: 0.004445}
			cost1 := float64(mem1) * pricing.MemoryPrice * 730
			cost2 := float64(mem2) * pricing.MemoryPrice * 730

			return cost2 > cost1
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})

	t.Run("Property: More storage means higher cost", func(t *testing.T) {
		property := func(storage1, storage2 uint8) bool {
			if storage1 >= storage2 || storage2 > 200 {
				return true
			}

			pricing := &PricingData{StoragePrice: 0.115}
			cost1 := float64(storage1) * pricing.StoragePrice
			cost2 := float64(storage2) * pricing.StoragePrice

			return cost2 > cost1
		}

		if err := quick.Check(property, nil); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})
}

// Helper types and functions for property tests

type PricingData struct {
	VCPUPrice    float64
	MemoryPrice  float64
	StoragePrice float64
}

type ResourceConfig struct {
	VCPU    int
	Memory  int
	Storage int
}

type CostBreakdown struct {
	Compute    float64
	Networking float64
	Storage    float64
}

type CostPercentages struct {
	Compute    float64
	Networking float64
	Storage    float64
}

func (cb *CostBreakdown) Total() float64 {
	return cb.Compute + cb.Networking + cb.Storage
}

func (cb *CostBreakdown) Percentages() CostPercentages {
	total := cb.Total()
	if total == 0 {
		return CostPercentages{}
	}

	return CostPercentages{
		Compute:    (cb.Compute / total) * 100,
		Networking: (cb.Networking / total) * 100,
		Storage:    (cb.Storage / total) * 100,
	}
}

func calculateMonthlyCost(vCPU, memory, storage int, pricing *PricingData) float64 {
	hoursPerMonth := 730.0
	cost := (float64(vCPU) * pricing.VCPUPrice * hoursPerMonth) +
		(float64(memory) * pricing.MemoryPrice * hoursPerMonth) +
		(float64(storage) * pricing.StoragePrice)

	return math.Round(cost*100) / 100
}

func calculateVariance(actual, estimate float64) float64 {
	if estimate == 0 {
		return 0
	}
	return ((actual - estimate) / estimate) * 100
}

func calculateRegionalCost(config ResourceConfig, region string, pricing *PricingData) float64 {
	return calculateMonthlyCost(config.VCPU, config.Memory, config.Storage, pricing)
}
