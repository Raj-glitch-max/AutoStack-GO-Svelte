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

	t.Run("MicroservicesRange", func(t *testing.T) {
		calc := NewMicroservicesBlueprintCalculator(app)
		config := MicroservicesConfig{Region: "us-east-1"}
		estimate, _ := calc.CalculateWithRange(config)
		
		if estimate.RangeMin > estimate.Estimate {
			t.Error("RangeMin must be ≤ Estimate")
		}
		if estimate.Estimate > estimate.RangeMax {
			t.Error("Estimate must be ≤ RangeMax")
		}
	})
}

func TestMicroservicesBlueprintCalculator(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	calc := NewMicroservicesBlueprintCalculator(app)

	t.Run("ValidConfiguration", func(t *testing.T) {
		config := MicroservicesConfig{
			Region:           "us-east-1",
			ServiceCount:     3,
			ServiceVCPU:      0.5,
			ServiceMemoryGB:  1.0,
			ServiceTasks:     2,
			DBInstanceClass:  "db.t3.medium",
			DBStorageGB:      100,
			DBEngine:         "postgres",
			StorageGB:        50,
			RequestsPerMonth: 100000,
			DataTransferGB:   300,
			LCUHours:         2190,
			NATDataGB:        200,
			CloudWatchLogsGB: 20,
			APIGatewayReqs:   1000000,
			SQSRequests:      1000000,
			ElastiCacheNodes: 1,
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
		if breakdown.S3 <= 0 {
			t.Error("Expected positive S3 cost")
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
		if breakdown.APIGateway <= 0 {
			t.Error("Expected positive API Gateway cost")
		}
		if breakdown.ElastiCache <= 0 {
			t.Error("Expected positive ElastiCache cost")
		}
		if breakdown.ECR <= 0 {
			t.Error("Expected positive ECR cost")
		}
		if breakdown.Currency != "USD" {
			t.Errorf("Expected USD currency, got %s", breakdown.Currency)
		}
	})

	t.Run("WithRange", func(t *testing.T) {
		config := MicroservicesConfig{
			Region:          "us-east-1",
			ServiceCount:    5,
			ServiceVCPU:     1.0,
			ServiceMemoryGB: 2.0,
			ServiceTasks:    3,
			DBInstanceClass: "db.t3.large",
			DBStorageGB:     200,
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
		config := MicroservicesConfig{
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
		if breakdown.Fargate <= 0 {
			t.Error("Expected default Fargate cost")
		}
		if breakdown.RDS <= 0 {
			t.Error("Expected default RDS cost")
		}
	})

	t.Run("CostBreakdownSum", func(t *testing.T) {
		config := MicroservicesConfig{
			Region: "us-east-1",
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		// Verify breakdown sums to total
		sum := breakdown.Fargate + breakdown.ALB + breakdown.RDS + 
			breakdown.S3 + breakdown.CloudFront + breakdown.NATGateway + 
			breakdown.CloudWatchLogs + breakdown.APIGateway + breakdown.SQS + 
			breakdown.ElastiCache + breakdown.ECR
		
		if roundToTwoDecimals(sum) != breakdown.TotalMonthly {
			t.Errorf("Breakdown sum %.2f does not match total %.2f", sum, breakdown.TotalMonthly)
		}
	})

	t.Run("ScalingWithServiceCount", func(t *testing.T) {
		config3Services := MicroservicesConfig{
			Region:       "us-east-1",
			ServiceCount: 3,
		}

		config5Services := MicroservicesConfig{
			Region:       "us-east-1",
			ServiceCount: 5,
		}

		breakdown3, _ := calc.Calculate(config3Services)
		breakdown5, _ := calc.Calculate(config5Services)

		if breakdown5.TotalMonthly <= breakdown3.TotalMonthly {
			t.Error("Cost should increase with more services")
		}
		if breakdown5.Fargate <= breakdown3.Fargate {
			t.Error("Fargate cost should increase with more services")
		}
	})

	t.Run("EstimateFromBlueprint", func(t *testing.T) {
		blueprintConfig := map[string]interface{}{
			"service_count":      4,
			"service_vcpu":       0.5,
			"service_memory":     1.0,
			"service_tasks":      2,
			"db_instance_class":  "db.t3.medium",
			"db_storage_gb":      100,
			"storage_gb":         50,
			"data_transfer_gb":   300,
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
		config := MicroservicesConfig{
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
		requiredAssumptions := []string{"microservices", "perService", "database", "storage", "cdn"}
		for _, key := range requiredAssumptions {
			if _, exists := breakdown.Assumptions[key]; !exists {
				t.Errorf("Expected assumption for %s", key)
			}
		}
	})

	t.Run("ServiceDetailsPresent", func(t *testing.T) {
		config := MicroservicesConfig{
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
		requiredServices := []string{"fargate", "alb", "rds", "s3", "cloudfront"}
		for _, key := range requiredServices {
			if _, exists := breakdown.ServiceDetails[key]; !exists {
				t.Errorf("Expected service detail for %s", key)
			}
		}
	})

	t.Run("MostExpensiveBlueprint", func(t *testing.T) {
		// Microservices should be the most expensive blueprint
		staticCalc := NewStaticWebsiteBlueprintCalculator(app)
		webAppCalc := NewWebAppBlueprintCalculator(app)
		fullStackCalc := NewFullStackBlueprintCalculator(app)

		staticConfig := StaticWebsiteConfig{Region: "us-east-1"}
		webAppConfig := WebAppConfig{Region: "us-east-1"}
		fullStackConfig := FullStackConfig{Region: "us-east-1"}
		microservicesConfig := MicroservicesConfig{Region: "us-east-1"}

		staticBreakdown, _ := staticCalc.Calculate(staticConfig)
		webAppBreakdown, _ := webAppCalc.Calculate(webAppConfig)
		fullStackBreakdown, _ := fullStackCalc.Calculate(fullStackConfig)
		microservicesBreakdown, _ := calc.Calculate(microservicesConfig)

		if microservicesBreakdown.TotalMonthly <= staticBreakdown.TotalMonthly {
			t.Error("Microservices should cost more than static website")
		}
		if microservicesBreakdown.TotalMonthly <= webAppBreakdown.TotalMonthly {
			t.Error("Microservices should cost more than web app")
		}
		if microservicesBreakdown.TotalMonthly <= fullStackBreakdown.TotalMonthly {
			t.Error("Microservices should cost more than full-stack")
		}
	})
}

func TestBlueprintRangeMargins(t *testing.T) {
	// **Validates: Requirements TR-2.2** (Calculate range with 20% margin)
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	t.Run("StaticWebsiteRangeMargins", func(t *testing.T) {
		calc := NewStaticWebsiteBlueprintCalculator(app)
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

		// Verify exact margin calculations
		expectedMin := roundToTwoDecimals(estimate.Estimate * 0.8)
		expectedMax := roundToTwoDecimals(estimate.Estimate * 1.4)

		if estimate.RangeMin != expectedMin {
			t.Errorf("Expected RangeMin %.2f (estimate * 0.8), got %.2f", expectedMin, estimate.RangeMin)
		}
		if estimate.RangeMax != expectedMax {
			t.Errorf("Expected RangeMax %.2f (estimate * 1.4), got %.2f", expectedMax, estimate.RangeMax)
		}
	})

	t.Run("WebAppRangeMargins", func(t *testing.T) {
		calc := NewWebAppBlueprintCalculator(app)
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

		expectedMin := roundToTwoDecimals(estimate.Estimate * 0.8)
		expectedMax := roundToTwoDecimals(estimate.Estimate * 1.4)

		if estimate.RangeMin != expectedMin {
			t.Errorf("Expected RangeMin %.2f (estimate * 0.8), got %.2f", expectedMin, estimate.RangeMin)
		}
		if estimate.RangeMax != expectedMax {
			t.Errorf("Expected RangeMax %.2f (estimate * 1.4), got %.2f", expectedMax, estimate.RangeMax)
		}
	})

	t.Run("FullStackRangeMargins", func(t *testing.T) {
		calc := NewFullStackBlueprintCalculator(app)
		config := FullStackConfig{
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

		estimate, err := calc.CalculateWithRange(config)
		if err != nil {
			t.Fatalf("CalculateWithRange failed: %v", err)
		}

		expectedMin := roundToTwoDecimals(estimate.Estimate * 0.8)
		expectedMax := roundToTwoDecimals(estimate.Estimate * 1.4)

		if estimate.RangeMin != expectedMin {
			t.Errorf("Expected RangeMin %.2f (estimate * 0.8), got %.2f", expectedMin, estimate.RangeMin)
		}
		if estimate.RangeMax != expectedMax {
			t.Errorf("Expected RangeMax %.2f (estimate * 1.4), got %.2f", expectedMax, estimate.RangeMax)
		}
	})

	t.Run("MicroservicesRangeMargins", func(t *testing.T) {
		calc := NewMicroservicesBlueprintCalculator(app)
		config := MicroservicesConfig{
			Region:          "us-east-1",
			ServiceCount:    3,
			ServiceVCPU:     0.5,
			ServiceMemoryGB: 1.0,
			ServiceTasks:    2,
			DBInstanceClass: "db.t3.medium",
			DBStorageGB:     100,
		}

		estimate, err := calc.CalculateWithRange(config)
		if err != nil {
			t.Fatalf("CalculateWithRange failed: %v", err)
		}

		expectedMin := roundToTwoDecimals(estimate.Estimate * 0.8)
		expectedMax := roundToTwoDecimals(estimate.Estimate * 1.4)

		if estimate.RangeMin != expectedMin {
			t.Errorf("Expected RangeMin %.2f (estimate * 0.8), got %.2f", expectedMin, estimate.RangeMin)
		}
		if estimate.RangeMax != expectedMax {
			t.Errorf("Expected RangeMax %.2f (estimate * 1.4), got %.2f", expectedMax, estimate.RangeMax)
		}
	})
}

func TestBlueprintBreakdownCompleteness(t *testing.T) {
	// **Validates: Cost Calculation Property 4** (Service breakdown sums to total estimate)
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	t.Run("StaticWebsiteBreakdownSum", func(t *testing.T) {
		calc := NewStaticWebsiteBlueprintCalculator(app)
		config := StaticWebsiteConfig{
			Region:           "us-east-1",
			StorageGB:        10,
			RequestsPerMonth: 10000,
			DataTransferGB:   100,
			HasCustomDomain:  true,
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		sum := breakdown.S3Storage + breakdown.S3Requests + 
			breakdown.CloudFront + breakdown.Route53
		
		if roundToTwoDecimals(sum) != breakdown.TotalMonthly {
			t.Errorf("Breakdown sum %.2f does not match total %.2f", sum, breakdown.TotalMonthly)
		}
	})

	t.Run("WebAppBreakdownSum", func(t *testing.T) {
		calc := NewWebAppBlueprintCalculator(app)
		config := WebAppConfig{
			Region:          "us-east-1",
			VCPU:            0.5,
			MemoryGB:        1.0,
			TaskCount:       2,
			DBInstanceClass: "db.t3.small",
			DBStorageGB:     50,
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		sum := breakdown.Fargate + breakdown.ALB + breakdown.RDS + 
			breakdown.NATGateway + breakdown.CloudWatchLogs + breakdown.ECR
		
		if roundToTwoDecimals(sum) != breakdown.TotalMonthly {
			t.Errorf("Breakdown sum %.2f does not match total %.2f", sum, breakdown.TotalMonthly)
		}
	})

	t.Run("FullStackBreakdownSum", func(t *testing.T) {
		calc := NewFullStackBlueprintCalculator(app)
		config := FullStackConfig{
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

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		sum := breakdown.FrontendFargate + breakdown.BackendFargate + 
			breakdown.ALB + breakdown.RDS + breakdown.S3Assets + 
			breakdown.CloudFront + breakdown.NATGateway + 
			breakdown.CloudWatchLogs + breakdown.ECR
		
		if roundToTwoDecimals(sum) != breakdown.TotalMonthly {
			t.Errorf("Breakdown sum %.2f does not match total %.2f", sum, breakdown.TotalMonthly)
		}
	})

	t.Run("MicroservicesBreakdownSum", func(t *testing.T) {
		calc := NewMicroservicesBlueprintCalculator(app)
		config := MicroservicesConfig{
			Region:          "us-east-1",
			ServiceCount:    3,
			ServiceVCPU:     0.5,
			ServiceMemoryGB: 1.0,
			ServiceTasks:    2,
			DBInstanceClass: "db.t3.medium",
			DBStorageGB:     100,
		}

		breakdown, err := calc.Calculate(config)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		sum := breakdown.Fargate + breakdown.ALB + breakdown.RDS + 
			breakdown.S3 + breakdown.CloudFront + breakdown.NATGateway + 
			breakdown.CloudWatchLogs + breakdown.APIGateway + breakdown.SQS + 
			breakdown.ElastiCache + breakdown.ECR
		
		if roundToTwoDecimals(sum) != breakdown.TotalMonthly {
			t.Errorf("Breakdown sum %.2f does not match total %.2f", sum, breakdown.TotalMonthly)
		}
	})
}

func TestBlueprintDecimalRounding(t *testing.T) {
	// **Validates: Requirements TR-2.4** (Round to 2 decimal places for display)
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	t.Run("StaticWebsiteRounding", func(t *testing.T) {
		calc := NewStaticWebsiteBlueprintCalculator(app)
		config := StaticWebsiteConfig{Region: "us-east-1"}

		estimate, err := calc.CalculateWithRange(config)
		if err != nil {
			t.Fatalf("CalculateWithRange failed: %v", err)
		}

		// Check that all values are rounded to 2 decimal places
		if estimate.Estimate != roundToTwoDecimals(estimate.Estimate) {
			t.Error("Estimate should be rounded to 2 decimal places")
		}
		if estimate.RangeMin != roundToTwoDecimals(estimate.RangeMin) {
			t.Error("RangeMin should be rounded to 2 decimal places")
		}
		if estimate.RangeMax != roundToTwoDecimals(estimate.RangeMax) {
			t.Error("RangeMax should be rounded to 2 decimal places")
		}
	})

	t.Run("AllBlueprintsRounding", func(t *testing.T) {
		blueprints := []struct {
			name     string
			estimate *CostEstimateWithRange
		}{
			{
				name: "StaticWebsite",
				estimate: func() *CostEstimateWithRange {
					calc := NewStaticWebsiteBlueprintCalculator(app)
					est, _ := calc.CalculateWithRange(StaticWebsiteConfig{Region: "us-east-1"})
					return est
				}(),
			},
			{
				name: "WebApp",
				estimate: func() *CostEstimateWithRange {
					calc := NewWebAppBlueprintCalculator(app)
					est, _ := calc.CalculateWithRange(WebAppConfig{Region: "us-east-1"})
					return est
				}(),
			},
			{
				name: "FullStack",
				estimate: func() *CostEstimateWithRange {
					calc := NewFullStackBlueprintCalculator(app)
					est, _ := calc.CalculateWithRange(FullStackConfig{Region: "us-east-1"})
					return est
				}(),
			},
			{
				name: "Microservices",
				estimate: func() *CostEstimateWithRange {
					calc := NewMicroservicesBlueprintCalculator(app)
					est, _ := calc.CalculateWithRange(MicroservicesConfig{Region: "us-east-1"})
					return est
				}(),
			},
		}

		for _, bp := range blueprints {
			t.Run(bp.name, func(t *testing.T) {
				if bp.estimate.Estimate != roundToTwoDecimals(bp.estimate.Estimate) {
					t.Errorf("%s: Estimate should be rounded to 2 decimal places", bp.name)
				}
				if bp.estimate.RangeMin != roundToTwoDecimals(bp.estimate.RangeMin) {
					t.Errorf("%s: RangeMin should be rounded to 2 decimal places", bp.name)
				}
				if bp.estimate.RangeMax != roundToTwoDecimals(bp.estimate.RangeMax) {
					t.Errorf("%s: RangeMax should be rounded to 2 decimal places", bp.name)
				}
			})
		}
	})
}
