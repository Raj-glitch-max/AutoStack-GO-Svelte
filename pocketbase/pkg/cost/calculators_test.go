package cost

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestFargateCostCalculator(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	calc := NewFargateCostCalculator(app)

	t.Run("ValidConfiguration", func(t *testing.T) {
		config := FargateConfig{
			VCPU:         0.25,
			MemoryGB:     0.5,
			TaskCount:    1,
			HoursPerDay:  24,
			DaysPerMonth: 30,
			Region:       "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		if breakdown.TotalMonthly <= 0 {
			t.Error("Expected positive total monthly cost")
		}
		if breakdown.VCPUCost <= 0 {
			t.Error("Expected positive vCPU cost")
		}
		if breakdown.MemoryCost <= 0 {
			t.Error("Expected positive memory cost")
		}
		if breakdown.Currency != "USD" {
			t.Errorf("Expected USD currency, got %s", breakdown.Currency)
		}
	})

	t.Run("WithRange", func(t *testing.T) {
		config := FargateConfig{
			VCPU:     0.5,
			MemoryGB: 1,
			TaskCount: 2,
			Region:   "us-east-1",
		}

		estimate, err := calc.CalculateWithRange(config)
		if err != nil {
			t.Fatalf("CalculateWithRange failed: %v", err)
		}

		if estimate.RangeMin >= estimate.Estimate {
			t.Error("RangeMin should be less than Estimate")
		}
		if estimate.RangeMax <= estimate.Estimate {
			t.Error("RangeMax should be greater than Estimate")
		}
		
		// Verify 20% margin on min and 40% on max
		expectedMin := estimate.Estimate * 0.8
		expectedMax := estimate.Estimate * 1.4
		if estimate.RangeMin != roundToTwoDecimals(expectedMin) {
			t.Errorf("Expected RangeMin %.2f, got %.2f", expectedMin, estimate.RangeMin)
		}
		if estimate.RangeMax != roundToTwoDecimals(expectedMax) {
			t.Errorf("Expected RangeMax %.2f, got %.2f", expectedMax, estimate.RangeMax)
		}
	})

	t.Run("InvalidVCPU", func(t *testing.T) {
		config := FargateConfig{
			VCPU:     0.3, // Invalid vCPU value
			MemoryGB: 0.5,
			TaskCount: 1,
			Region:   "us-east-1",
		}

		_, err := calc.Calculate(config)
		if err == nil {
			t.Error("Expected error for invalid vCPU")
		}
	})

	t.Run("InvalidMemoryRatio", func(t *testing.T) {
		config := FargateConfig{
			VCPU:     0.25,
			MemoryGB: 5, // Too much memory for 0.25 vCPU
			TaskCount: 1,
			Region:   "us-east-1",
		}

		_, err := calc.Calculate(config)
		if err == nil {
			t.Error("Expected error for invalid memory/vCPU ratio")
		}
	})
}

func TestS3CostCalculator(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	calc := NewS3CostCalculator(app)

	t.Run("ValidConfiguration", func(t *testing.T) {
		config := S3Config{
			StorageGB:         100,
			StorageClass:      "STANDARD",
			PUTRequests:       10000,
			GETRequests:       100000,
			DataTransferOutGB: 50,
			Region:            "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		if breakdown.TotalMonthly <= 0 {
			t.Error("Expected positive total monthly cost")
		}
		if breakdown.StorageCost <= 0 {
			t.Error("Expected positive storage cost")
		}
	})

	t.Run("ZeroUsage", func(t *testing.T) {
		config := S3Config{
			StorageGB: 0,
			Region:    "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		if breakdown.TotalMonthly != 0 {
			t.Error("Expected zero cost for zero usage")
		}
	})

	t.Run("NegativeValues", func(t *testing.T) {
		config := S3Config{
			StorageGB: -10,
			Region:    "us-east-1",
		}

		_, err := calc.Calculate(config)
		if err == nil {
			t.Error("Expected error for negative storage")
		}
	})
}

func TestALBCostCalculator(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	calc := NewALBCostCalculator(app)

	t.Run("ValidConfiguration", func(t *testing.T) {
		config := ALBConfig{
			HoursPerMonth:        730,
			NewConnectionsPerSec: 25,
			ActiveConnectionsMin: 3000,
			ProcessedBytesGB:     100,
			RuleEvaluations:      1000000,
			Region:               "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		if breakdown.TotalMonthly <= 0 {
			t.Error("Expected positive total monthly cost")
		}
		if breakdown.FixedHourlyCost <= 0 {
			t.Error("Expected positive fixed hourly cost")
		}
		if breakdown.AverageLCUsPerHour < 0 {
			t.Error("LCUs cannot be negative")
		}
	})

	t.Run("MinimalUsage", func(t *testing.T) {
		config := ALBConfig{
			HoursPerMonth: 730,
			Region:        "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		// Should have at least fixed cost
		if breakdown.FixedHourlyCost <= 0 {
			t.Error("Expected positive fixed cost even with minimal usage")
		}
	})
}

func TestRDSCostCalculator(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	calc := NewRDSCostCalculator(app)

	t.Run("ValidConfiguration", func(t *testing.T) {
		config := RDSConfig{
			InstanceClass:   "db.t3.micro",
			Engine:          "postgres",
			StorageGB:       20,
			StorageType:     "gp3",
			BackupStorageGB: 10,
			MultiAZ:         false,
			Region:          "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		if breakdown.TotalMonthly <= 0 {
			t.Error("Expected positive total monthly cost")
		}
		if breakdown.InstanceCost <= 0 {
			t.Error("Expected positive instance cost")
		}
		if breakdown.StorageCost <= 0 {
			t.Error("Expected positive storage cost")
		}
	})

	t.Run("MultiAZ", func(t *testing.T) {
		configSingleAZ := RDSConfig{
			InstanceClass: "db.t3.micro",
			StorageGB:     20,
			MultiAZ:       false,
			Region:        "us-east-1",
		}

		configMultiAZ := RDSConfig{
			InstanceClass: "db.t3.micro",
			StorageGB:     20,
			MultiAZ:       true,
			Region:        "us-east-1",
		}

		breakdownSingle, _ := calc.Calculate(configSingleAZ)
		breakdownMulti, _ := calc.Calculate(configMultiAZ)

		// Multi-AZ should cost more (roughly double instance cost)
		if breakdownMulti.InstanceCost <= breakdownSingle.InstanceCost {
			t.Error("Multi-AZ instance cost should be higher than Single-AZ")
		}
	})

	t.Run("InvalidStorage", func(t *testing.T) {
		config := RDSConfig{
			InstanceClass: "db.t3.micro",
			StorageGB:     10, // Below minimum of 20 GB
			Region:        "us-east-1",
		}

		_, err := calc.Calculate(config)
		if err == nil {
			t.Error("Expected error for storage below minimum")
		}
	})
}

func TestCloudFrontCostCalculator(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	calc := NewCloudFrontCostCalculator(app)

	t.Run("ValidConfiguration", func(t *testing.T) {
		config := CloudFrontConfig{
			DataTransferOutGB:      100,
			DataTransferToOriginGB: 10,
			HTTPRequests:           0,
			HTTPSRequests:          1000000,
			InvalidationRequests:   100,
			Region:                 "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		if breakdown.TotalMonthly <= 0 {
			t.Error("Expected positive total monthly cost")
		}
		if breakdown.DataTransferCost <= 0 {
			t.Error("Expected positive data transfer cost")
		}
	})

	t.Run("FreeInvalidations", func(t *testing.T) {
		config := CloudFrontConfig{
			DataTransferOutGB:    10,
			InvalidationRequests: 500, // Under 1000 free limit
			Region:               "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		if breakdown.InvalidationCost != 0 {
			t.Error("Expected zero invalidation cost for requests under 1000")
		}
	})
}

func TestRoute53CostCalculator(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	calc := NewRoute53CostCalculator(app)

	t.Run("ValidConfiguration", func(t *testing.T) {
		config := Route53Config{
			HostedZones:     1,
			StandardQueries: 1000000,
			HealthChecks:    2,
			Region:          "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		if breakdown.TotalMonthly <= 0 {
			t.Error("Expected positive total monthly cost")
		}
		if breakdown.HostedZoneCost <= 0 {
			t.Error("Expected positive hosted zone cost")
		}
	})

	t.Run("MultipleHostedZones", func(t *testing.T) {
		config := Route53Config{
			HostedZones: 5,
			Region:      "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		expectedCost := 5 * 0.50 // 5 zones * $0.50
		if breakdown.HostedZoneCost != expectedCost {
			t.Errorf("Expected hosted zone cost %.2f, got %.2f", expectedCost, breakdown.HostedZoneCost)
		}
	})
}

func TestCostPositivity(t *testing.T) {
	// Property: All cost estimates must be positive numbers
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	t.Run("FargatePositivity", func(t *testing.T) {
		calc := NewFargateCostCalculator(app)
		config := FargateConfig{
			VCPU: 0.25, MemoryGB: 0.5, TaskCount: 1, Region: "us-east-1",
		}
		breakdown, _ := calc.Calculate(config)
		if breakdown.TotalMonthly <= 0 {
			t.Error("Fargate cost must be positive")
		}
	})

	t.Run("S3Positivity", func(t *testing.T) {
		calc := NewS3CostCalculator(app)
		config := S3Config{
			StorageGB: 10, Region: "us-east-1",
		}
		breakdown, _ := calc.Calculate(config)
		if breakdown.TotalMonthly < 0 {
			t.Error("S3 cost cannot be negative")
		}
	})
}

func TestRangeConsistency(t *testing.T) {
	// Property: Cost ranges must satisfy min ≤ estimate ≤ max
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	calc := NewFargateCostCalculator(app)
	config := FargateConfig{
		VCPU: 1, MemoryGB: 2, TaskCount: 1, Region: "us-east-1",
	}

	estimate, err := calc.CalculateWithRange(config)
	if err != nil {
		t.Fatalf("CalculateWithRange failed: %v", err)
	}

	if estimate.RangeMin > estimate.Estimate {
		t.Error("RangeMin must be ≤ Estimate")
	}
	if estimate.Estimate > estimate.RangeMax {
		t.Error("Estimate must be ≤ RangeMax")
	}
	if estimate.RangeMin > estimate.RangeMax {
		t.Error("RangeMin must be ≤ RangeMax")
	}
}
