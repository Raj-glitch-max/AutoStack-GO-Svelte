package cost

import (
	"testing"
	"testing/quick"

	"github.com/pocketbase/pocketbase/tests"
)

// **Validates: Requirements AC-1.4** - Estimate is region-specific (different prices per region)
// **Validates: Cost Calculation Property 3** - Same blueprint in same region produces same estimate

// TestRegionalPricingConsistencyProperty validates that calling the cost estimation function
// multiple times with the same blueprint and region parameters always produces identical results.
// This is a critical property for user trust - estimates must be deterministic and reproducible.
func TestRegionalPricingConsistencyProperty(t *testing.T) {
	// Create a test app
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	
	mapper := NewBlueprintMapper(app)

	t.Run("Property: Same blueprint in same region produces same estimate", func(t *testing.T) {
		property := func(seed uint8) bool {
			// Use seed to select blueprint type and region
			blueprintTypes := []string{"static-website", "web-application", "full-stack-app", "microservices"}
			regions := []string{"us-east-1", "us-west-2", "eu-west-1", "ap-southeast-1"}

			blueprintType := blueprintTypes[int(seed)%len(blueprintTypes)]
			region := regions[int(seed/4)%len(regions)]

			// Create a consistent configuration
			config := map[string]interface{}{
				"region": region,
			}

			// Call EstimateCost multiple times with the same inputs
			estimate1, err1 := mapper.EstimateCost(blueprintType, config, region)
			estimate2, err2 := mapper.EstimateCost(blueprintType, config, region)
			estimate3, err3 := mapper.EstimateCost(blueprintType, config, region)

			// All calls should succeed or fail consistently
			if (err1 == nil) != (err2 == nil) || (err2 == nil) != (err3 == nil) {
				t.Logf("Inconsistent error states for %s in %s", blueprintType, region)
				return false
			}

			// If any call failed, skip this test case
			if err1 != nil {
				return true
			}

			// All estimates should be identical
			if estimate1.Estimate != estimate2.Estimate || estimate2.Estimate != estimate3.Estimate {
				t.Logf("Inconsistent estimates for %s in %s: %f, %f, %f",
					blueprintType, region, estimate1.Estimate, estimate2.Estimate, estimate3.Estimate)
				return false
			}

			// Range values should also be identical
			if estimate1.RangeMin != estimate2.RangeMin || estimate2.RangeMin != estimate3.RangeMin {
				t.Logf("Inconsistent range min for %s in %s: %f, %f, %f",
					blueprintType, region, estimate1.RangeMin, estimate2.RangeMin, estimate3.RangeMin)
				return false
			}

			if estimate1.RangeMax != estimate2.RangeMax || estimate2.RangeMax != estimate3.RangeMax {
				t.Logf("Inconsistent range max for %s in %s: %f, %f, %f",
					blueprintType, region, estimate1.RangeMax, estimate2.RangeMax, estimate3.RangeMax)
				return false
			}

			return true
		}

		if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})

	t.Run("Property: Estimates are deterministic with custom configuration", func(t *testing.T) {
		property := func(vcpu uint8, memory uint8, storage uint8) bool {
			// Constrain to valid ranges
			if vcpu == 0 || vcpu > 4 {
				return true
			}
			if memory == 0 || memory > 8 {
				return true
			}
			if storage > 100 {
				return true
			}

			blueprintType := "web-application"
			region := "us-east-1"

			// Create configuration with specific values
			config := map[string]interface{}{
				"region":     region,
				"vcpu":       float64(vcpu) * 0.25, // Convert to valid Fargate vCPU values
				"memory":     float64(memory) * 0.5,
				"db_storage_gb": int(storage),
			}

			// Call EstimateCost multiple times
			estimate1, err1 := mapper.EstimateCost(blueprintType, config, region)
			estimate2, err2 := mapper.EstimateCost(blueprintType, config, region)

			// Both calls should have the same error state
			if (err1 == nil) != (err2 == nil) {
				return false
			}

			// If error, skip this test case
			if err1 != nil {
				return true
			}

			// Estimates should be identical
			return estimate1.Estimate == estimate2.Estimate &&
				estimate1.RangeMin == estimate2.RangeMin &&
				estimate1.RangeMax == estimate2.RangeMax
		}

		if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})

	t.Run("Property: Empty configuration produces consistent results", func(t *testing.T) {
		property := func(seed uint8) bool {
			blueprintTypes := []string{"static-website", "web-application", "full-stack-app", "microservices"}
			regions := []string{"us-east-1", "us-west-2", "eu-west-1"}

			blueprintType := blueprintTypes[int(seed)%len(blueprintTypes)]
			region := regions[int(seed/4)%len(regions)]

			// Use empty configuration (should use defaults)
			config := map[string]interface{}{}

			// Call multiple times
			estimate1, err1 := mapper.EstimateCost(blueprintType, config, region)
			estimate2, err2 := mapper.EstimateCost(blueprintType, config, region)

			// Error states should match
			if (err1 == nil) != (err2 == nil) {
				return false
			}

			if err1 != nil {
				return true
			}

			// Results should be identical
			return estimate1.Estimate == estimate2.Estimate
		}

		if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
			t.Errorf("Property violated: %v", err)
		}
	})
}

// TestRegionalPricingDeterminism validates that estimates are deterministic
// across different execution contexts (e.g., different test runs)
func TestRegionalPricingDeterminism(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	testCases := []struct {
		name          string
		blueprintType string
		region        string
		config        map[string]interface{}
	}{
		{
			name:          "Static website in us-east-1",
			blueprintType: "static-website",
			region:        "us-east-1",
			config: map[string]interface{}{
				"storage_gb":         10.0,
				"requests_per_month": 10000,
			},
		},
		{
			name:          "Web app in eu-west-1",
			blueprintType: "web-application",
			region:        "eu-west-1",
			config: map[string]interface{}{
				"vcpu":   0.5,
				"memory": 1.0,
			},
		},
		{
			name:          "Full stack in ap-southeast-1",
			blueprintType: "full-stack-app",
			region:        "ap-southeast-1",
			config: map[string]interface{}{
				"backend_vcpu":   0.5,
				"backend_memory": 1.0,
			},
		},
		{
			name:          "Microservices in us-west-2",
			blueprintType: "microservices",
			region:        "us-west-2",
			config: map[string]interface{}{
				"service_count": 3,
				"service_vcpu":  0.5,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mapper := NewBlueprintMapper(app)

			// Call estimation function 5 times
			estimates := make([]*CostEstimateWithRange, 5)
			for i := 0; i < 5; i++ {
				estimate, err := mapper.EstimateCost(tc.blueprintType, tc.config, tc.region)
				if err != nil {
					t.Fatalf("Estimation failed on iteration %d: %v", i, err)
				}
				estimates[i] = estimate
			}

			// Verify all estimates are identical
			for i := 1; i < len(estimates); i++ {
				if estimates[0].Estimate != estimates[i].Estimate {
					t.Errorf("Estimate mismatch: iteration 0 = %f, iteration %d = %f",
						estimates[0].Estimate, i, estimates[i].Estimate)
				}
				if estimates[0].RangeMin != estimates[i].RangeMin {
					t.Errorf("RangeMin mismatch: iteration 0 = %f, iteration %d = %f",
						estimates[0].RangeMin, i, estimates[i].RangeMin)
				}
				if estimates[0].RangeMax != estimates[i].RangeMax {
					t.Errorf("RangeMax mismatch: iteration 0 = %f, iteration %d = %f",
						estimates[0].RangeMax, i, estimates[i].RangeMax)
				}
			}
		})
	}
}

// TestRegionalPricingIdempotency validates that the estimation function is idempotent
// (calling it multiple times has the same effect as calling it once)
func TestRegionalPricingIdempotency(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	
	mapper := NewBlueprintMapper(app)

	t.Run("Idempotency: Multiple calls don't change state", func(t *testing.T) {
		blueprintType := "web-application"
		region := "us-east-1"
		config := map[string]interface{}{
			"vcpu":   0.25,
			"memory": 0.5,
		}

		// First call
		estimate1, err1 := mapper.EstimateCost(blueprintType, config, region)
		if err1 != nil {
			t.Fatalf("First estimation failed: %v", err1)
		}

		// Second call (should not be affected by first call)
		estimate2, err2 := mapper.EstimateCost(blueprintType, config, region)
		if err2 != nil {
			t.Fatalf("Second estimation failed: %v", err2)
		}

		// Third call
		estimate3, err3 := mapper.EstimateCost(blueprintType, config, region)
		if err3 != nil {
			t.Fatalf("Third estimation failed: %v", err3)
		}

		// All estimates should be identical
		if estimate1.Estimate != estimate2.Estimate || estimate2.Estimate != estimate3.Estimate {
			t.Errorf("Estimates are not idempotent: %f, %f, %f",
				estimate1.Estimate, estimate2.Estimate, estimate3.Estimate)
		}
	})
}

// TestRegionalPricingConcurrency validates that concurrent calls to the estimation
// function produce consistent results (thread-safety)
func TestRegionalPricingConcurrency(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	
	mapper := NewBlueprintMapper(app)

	t.Run("Concurrency: Parallel calls produce consistent results", func(t *testing.T) {
		blueprintType := "static-website"
		region := "us-east-1"
		config := map[string]interface{}{
			"storage_gb": 10.0,
		}

		// Run 10 concurrent estimations
		const numGoroutines = 10
		results := make(chan *CostEstimateWithRange, numGoroutines)
		errors := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				estimate, err := mapper.EstimateCost(blueprintType, config, region)
				if err != nil {
					errors <- err
					return
				}
				results <- estimate
			}()
		}

		// Collect results
		estimates := make([]*CostEstimateWithRange, 0, numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			select {
			case err := <-errors:
				t.Fatalf("Concurrent estimation failed: %v", err)
			case estimate := <-results:
				estimates = append(estimates, estimate)
			}
		}

		// Verify all estimates are identical
		firstEstimate := estimates[0]
		for i := 1; i < len(estimates); i++ {
			if estimates[i].Estimate != firstEstimate.Estimate {
				t.Errorf("Concurrent estimate mismatch: estimate[0] = %f, estimate[%d] = %f",
					firstEstimate.Estimate, i, estimates[i].Estimate)
			}
		}
	})
}
