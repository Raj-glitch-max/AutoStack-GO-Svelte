package cost

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestStaticWebsiteBlueprintCalculator(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	calc := NewStaticWebsiteBlueprintCalculator(app)

	t.Run("ValidConfiguration", func(t *testing.T) {
		config := StaticWebsiteConfig{
			Region:           "us-east-1",
			StorageGB:        10,
			RequestsPerMonth: 10000,
			DataTransferGB:   100,
			HasCustomDomain:  false,
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		if breakdown.TotalMonthly <= 0 {
			t.Error("Expected positive total monthly cost")
		}
		if breakdown.S3Storage < 0 {
			t.Error("S3 storage cost cannot be negative")
		}
		if breakdown.CloudFront < 0 {
			t.Error("CloudFront cost cannot be negative")
		}
		if breakdown.Currency != "USD" {
			t.Errorf("Expected USD currency, got %s", breakdown.Currency)
		}
	})

	t.Run("WithCustomDomain", func(t *testing.T) {
		configWithoutDomain := StaticWebsiteConfig{
			Region:           "us-east-1",
			StorageGB:        10,
			RequestsPerMonth: 10000,
			DataTransferGB:   100,
			HasCustomDomain:  false,
		}

		configWithDomain := StaticWebsiteConfig{
			Region:           "us-east-1",
			StorageGB:        10,
			RequestsPerMonth: 10000,
			DataTransferGB:   100,
			HasCustomDomain:  true,
		}

		breakdownWithout, _ := calc.Calculate(configWithoutDomain)
		breakdownWith, _ := calc.Calculate(configWithDomain)

		if breakdownWith.Route53 <= 0 {
			t.Error("Expected positive Route53 cost with custom domain")
		}
		if breakdownWithout.Route53 != 0 {
			t.Error("Expected zero Route53 cost without custom domain")
		}
		if breakdownWith.TotalMonthly <= breakdownWithout.TotalMonthly {
			t.Error("Cost with custom domain should be higher")
		}
	})

	t.Run("WithRange", func(t *testing.T) {
		config := StaticWebsiteConfig{
			Region:           "us-east-1",
			StorageGB:        10,
			RequestsPerMonth: 10000,
			DataTransferGB:   100,
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

	t.Run("DefaultValues", func(t *testing.T) {
		config := StaticWebsiteConfig{
			Region: "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate with defaults failed: %v", err)
		}

		if breakdown.TotalMonthly <= 0 {
			t.Error("Expected positive cost with default values")
		}
	})
}

func TestWebAppBlueprintCalculator(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	calc := NewWebAppBlueprintCalculator(app)

	t.Run("ValidConfiguration", func(t *testing.T) {
		config := WebAppConfig{
			Region:           "us-east-1",
			VCPU:             0.25,
			MemoryGB:         0.5,
			TaskCount:        1,
			DBInstanceClass:  "db.t3.micro",
			DBStorageGB:      20,
			DBEngine:         "postgres",
			LCUHours:         730,
			NATDataGB:        50,
			CloudWatchLogsGB: 5,
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		if breakdown.TotalMonthly <= 0 {
			t.Error("Expected positive total monthly cost")
		}
		if breakdown.Fargate <= 0 {
			t.Error("Expected positive Fargate cost")
		}
		if breakdown.ALB <= 0 {
			t.Error("Expected positive ALB cost")
		}
		if breakdown.RDS <= 0 {
			t.Error("Expected positive RDS cost")
		}
		if breakdown.NATGateway <= 0 {
			t.Error("Expected positive NAT Gateway cost")
		}
		if breakdown.CloudWatchLogs <= 0 {
			t.Error("Expected positive CloudWatch Logs cost")
		}
		if breakdown.ECR <= 0 {
			t.Error("Expected positive ECR cost")
		}
		if breakdown.Currency != "USD" {
			t.Errorf("Expected USD currency, got %s", breakdown.Currency)
		}
	})

	t.Run("WithRange", func(t *testing.T) {
		config := WebAppConfig{
			Region:          "us-east-1",
			VCPU:            0.5,
			MemoryGB:        1.0,
			TaskCount:       2,
			DBInstanceClass: "db.t3.small",
			DBStorageGB:     50,
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
	})

	t.Run("DefaultValues", func(t *testing.T) {
		config := WebAppConfig{
			Region: "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate with defaults failed: %v", err)
		}

		if breakdown.TotalMonthly <= 0 {
			t.Error("Expected positive cost with default values")
		}
	})

	t.Run("CostBreakdownSum", func(t *testing.T) {
		config := WebAppConfig{
			Region: "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		// Verify breakdown sums to total
		sum := breakdown.Fargate + breakdown.ALB + breakdown.RDS + 
			breakdown.NATGateway + breakdown.CloudWatchLogs + breakdown.ECR
		
		if roundToTwoDecimals(sum) != breakdown.TotalMonthly {
			t.Errorf("Breakdown sum %.2f does not match total %.2f", sum, breakdown.TotalMonthly)
		}
	})
}

func TestFullStackBlueprintCalculator(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	calc := NewFullStackBlueprintCalculator(app)

	t.Run("ValidConfiguration", func(t *testing.T) {
		config := FullStackConfig{
			Region:            "us-east-1",
			FrontendVCPU:      0.25,
			FrontendMemoryGB:  0.5,
			FrontendTasks:     1,
			BackendVCPU:       0.5,
			BackendMemoryGB:   1.0,
			BackendTasks:      2,
			DBInstanceClass:   "db.t3.small",
			DBStorageGB:       50,
			DBEngine:          "postgres",
			AssetStorageGB:    20,
			AssetRequestsPM:   50000,
			CDNDataTransferGB: 200,
			LCUHours:          1460,
			NATDataGB:         100,
			CloudWatchLogsGB:  10,
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		if breakdown.TotalMonthly <= 0 {
			t.Error("Expected positive total monthly cost")
		}
		if breakdown.FrontendFargate <= 0 {
			t.Error("Expected positive Frontend Fargate cost")
		}
		if breakdown.BackendFargate <= 0 {
			t.Error("Expected positive Backend Fargate cost")
		}
		if breakdown.ALB <= 0 {
			t.Error("Expected positive ALB cost")
		}
		if breakdown.RDS <= 0 {
			t.Error("Expected positive RDS cost")
		}
		if breakdown.S3Assets <= 0 {
			t.Error("Expected positive S3 Assets cost")
		}
		if breakdown.CloudFront <= 0 {
			t.Error("Expected positive CloudFront cost")
		}
		if breakdown.NATGateway <= 0 {
			t.Error("Expected positive NAT Gateway cost")
		}
		if breakdown.CloudWatchLogs <= 0 {
			t.Error("Expected positive CloudWatch Logs cost")
		}
		if breakdown.ECR <= 0 {
			t.Error("Expected positive ECR cost")
		}
		if breakdown.Currency != "USD" {
			t.Errorf("Expected USD currency, got %s", breakdown.Currency)
		}
	})

	t.Run("WithRange", func(t *testing.T) {
		config := FullStackConfig{
			Region:           "us-east-1",
			FrontendVCPU:     0.25,
			FrontendMemoryGB: 0.5,
			FrontendTasks:    2,
			BackendVCPU:      1.0,
			BackendMemoryGB:  2.0,
			BackendTasks:     3,
			DBInstanceClass:  "db.t3.medium",
			DBStorageGB:      100,
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

	t.Run("DefaultValues", func(t *testing.T) {
		config := FullStackConfig{
			Region: "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate with defaults failed: %v", err)
		}

		if breakdown.TotalMonthly <= 0 {
			t.Error("Expected positive cost with default values")
		}

		// Verify defaults were applied
		if breakdown.FrontendFargate <= 0 {
			t.Error("Expected default frontend Fargate cost")
		}
		if breakdown.BackendFargate <= 0 {
			t.Error("Expected default backend Fargate cost")
		}
	})

	t.Run("CostBreakdownSum", func(t *testing.T) {
		config := FullStackConfig{
			Region: "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		// Verify breakdown sums to total
		sum := breakdown.FrontendFargate + breakdown.BackendFargate + 
			breakdown.ALB + breakdown.RDS + breakdown.S3Assets + 
			breakdown.CloudFront + breakdown.NATGateway + 
			breakdown.CloudWatchLogs + breakdown.ECR
		
		if roundToTwoDecimals(sum) != breakdown.TotalMonthly {
			t.Errorf("Breakdown sum %.2f does not match total %.2f", sum, breakdown.TotalMonthly)
		}
	})

	t.Run("MoreExpensiveThanWebApp", func(t *testing.T) {
		// Full-stack should cost more than web app with similar config
		webAppCalc := NewWebAppBlueprintCalculator(app)
		
		webAppConfig := WebAppConfig{
			Region:          "us-east-1",
			VCPU:            0.5,
			MemoryGB:        1.0,
			TaskCount:       2,
			DBInstanceClass: "db.t3.small",
			DBStorageGB:     50,
		}

		fullStackConfig := FullStackConfig{
			Region:           "us-east-1",
			FrontendVCPU:     0.25,
			FrontendMemoryGB: 0.5,
			FrontendTasks:    1,
			BackendVCPU:      0.5,
			BackendMemoryGB:  1.0,
			BackendTasks:     2,
			DBInstanceClass:  "db.t3.small",
			DBStorageGB:      50,
		}

		webAppBreakdown, _ := webAppCalc.Calculate(webAppConfig)
		fullStackBreakdown, _ := calc.Calculate(fullStackConfig)

		if fullStackBreakdown.TotalMonthly <= webAppBreakdown.TotalMonthly {
			t.Error("Full-stack should cost more than web app due to additional services")
		}
	})

	t.Run("EstimateFromBlueprint", func(t *testing.T) {
		blueprintConfig := map[string]interface{}{
			"frontend_vcpu":        0.25,
			"frontend_memory":      0.5,
			"frontend_tasks":       1,
			"backend_vcpu":         0.5,
			"backend_memory":       1.0,
			"backend_tasks":        2,
			"db_instance_class":    "db.t3.small",
			"db_storage_gb":        50,
			"asset_storage_gb":     20,
			"cdn_data_transfer_gb": 200,
		}

		estimate, err := calc.EstimateFromBlueprint(blueprintConfig, "us-east-1")
		if err != nil {
			t.Fatalf("EstimateFromBlueprint failed: %v", err)
		}

		if estimate.Estimate <= 0 {
			t.Error("Expected positive estimate")
		}
		if estimate.RangeMin >= estimate.Estimate {
			t.Error("RangeMin should be less than Estimate")
		}
		if estimate.RangeMax <= estimate.Estimate {
			t.Error("RangeMax should be greater than Estimate")
		}
	})

	t.Run("AssumptionsPresent", func(t *testing.T) {
		config := FullStackConfig{
			Region: "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		if len(breakdown.Assumptions) == 0 {
			t.Error("Expected assumptions to be present")
		}

		// Check for key assumptions
		requiredAssumptions := []string{"frontend", "backend", "database", "assets", "cdn"}
		for _, key := range requiredAssumptions {
			if _, exists := breakdown.Assumptions[key]; !exists {
				t.Errorf("Expected assumption for %s", key)
			}
		}
	})

	t.Run("ServiceDetailsPresent", func(t *testing.T) {
		config := FullStackConfig{
			Region: "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		if len(breakdown.ServiceDetails) == 0 {
			t.Error("Expected service details to be present")
		}

		// Check for key service details
		requiredServices := []string{"frontendFargate", "backendFargate", "alb", "rds", "s3", "cloudfront"}
		for _, key := range requiredServices {
			if _, exists := breakdown.ServiceDetails[key]; !exists {
				t.Errorf("Expected service detail for %s", key)
			}
		}
	})
}

func TestBlueprintCostPositivity(t *testing.T) {
	// Property: All blueprint cost estimates must be positive numbers
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	t.Run("StaticWebsitePositivity", func(t *testing.T) {
		calc := NewStaticWebsiteBlueprintCalculator(app)
		config := StaticWebsiteConfig{Region: "us-east-1"}
		breakdown, _ := calc.Calculate(config)
		if breakdown.TotalMonthly <= 0 {
			t.Error("Static website cost must be positive")
		}
	})

	t.Run("WebAppPositivity", func(t *testing.T) {
		calc := NewWebAppBlueprintCalculator(app)
		config := WebAppConfig{Region: "us-east-1"}
		breakdown, _ := calc.Calculate(config)
		if breakdown.TotalMonthly <= 0 {
			t.Error("Web app cost must be positive")
		}
	})

	t.Run("FullStackPositivity", func(t *testing.T) {
		calc := NewFullStackBlueprintCalculator(app)
		config := FullStackConfig{Region: "us-east-1"}
		breakdown, _ := calc.Calculate(config)
		if breakdown.TotalMonthly <= 0 {
			t.Error("Full-stack cost must be positive")
		}
	})
}

func TestBlueprintRangeConsistency(t *testing.T) {
	// Property: Cost ranges must satisfy min ≤ estimate ≤ max
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	t.Run("StaticWebsiteRange", func(t *testing.T) {
		calc := NewStaticWebsiteBlueprintCalculator(app)
		config := StaticWebsiteConfig{Region: "us-east-1"}
		estimate, _ := calc.CalculateWithRange(config)
		
		if estimate.RangeMin > estimate.Estimate {
			t.Error("RangeMin must be ≤ Estimate")
		}
		if estimate.Estimate > estimate.RangeMax {
			t.Error("Estimate must be ≤ RangeMax")
		}
	})

	t.Run("WebAppRange", func(t *testing.T) {
		calc := NewWebAppBlueprintCalculator(app)
		config := WebAppConfig{Region: "us-east-1"}
		estimate, _ := calc.CalculateWithRange(config)
		
		if estimate.RangeMin > estimate.Estimate {
			t.Error("RangeMin must be ≤ Estimate")
		}
		if estimate.Estimate > estimate.RangeMax {
			t.Error("Estimate must be ≤ RangeMax")
		}
	})

	t.Run("FullStackRange", func(t *testing.T) {
		calc := NewFullStackBlueprintCalculator(app)
		config := FullStackConfig{Region: "us-east-1"}
		estimate, _ := calc.CalculateWithRange(config)
		
		if estimate.RangeMin > estimate.Estimate {
			t.Error("RangeMin must be ≤ Estimate")
		}
		if estimate.Estimate > estimate.RangeMax {
			t.Error("Estimate must be ≤ RangeMax")
		}
	})
}
