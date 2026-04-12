package cost

import (
	"testing"
)

// TestBreakdownIntegration_AllBlueprints tests that all blueprint types
// produce valid category breakdowns that sum to the total
// **Validates**: AC-1.2 (Estimate includes compute, networking, and storage costs)
// **Validates**: Cost Calculation Property 4 (Service breakdown sums to total estimate)
func TestBreakdownIntegration_AllBlueprints(t *testing.T) {
	bg := NewBreakdownGenerator()

	tests := []struct {
		name         string
		estimate     *CostEstimateWithRange
		hasCompute   bool
		hasNetwork   bool
		hasStorage   bool
		hasDatabase  bool
	}{
		{
			name: "Static Website Blueprint",
			estimate: &CostEstimateWithRange{
				Estimate: 12.30,
				RangeMin: 9.84,
				RangeMax: 17.22,
				Breakdown: &StaticWebsiteCostBreakdown{
					S3Storage:    2.30,
					S3Requests:   0.50,
					CloudFront:   9.00,
					Route53:      0.50,
					TotalMonthly: 12.30,
					Currency:     "USD",
				},
				Currency: "USD",
			},
			hasCompute:  false,
			hasNetwork:  true,
			hasStorage:  true,
			hasDatabase: false,
		},
		{
			name: "Web Application Blueprint",
			estimate: &CostEstimateWithRange{
				Estimate: 96.34,
				RangeMin: 77.07,
				RangeMax: 134.88,
				Breakdown: &WebAppCostBreakdown{
					Fargate:        29.15,
					ALB:            16.20,
					RDS:            13.14,
					NATGateway:     35.10,
					CloudWatchLogs: 2.65,
					ECR:            0.10,
					TotalMonthly:   96.34,
					Currency:       "USD",
				},
				Currency: "USD",
			},
			hasCompute:  true,
			hasNetwork:  true,
			hasStorage:  true,
			hasDatabase: true,
		},
		{
			name: "Full-Stack Application Blueprint",
			estimate: &CostEstimateWithRange{
				Estimate: 178.21,
				RangeMin: 142.57,
				RangeMax: 249.49,
				Breakdown: &FullStackCostBreakdown{
					FrontendFargate: 14.58,
					BackendFargate:  58.30,
					ALB:             16.20,
					RDS:             26.28,
					S3Assets:        2.00,
					CloudFront:      18.00,
					NATGateway:      37.35,
					CloudWatchLogs:  5.30,
					ECR:             0.20,
					TotalMonthly:    178.21,
					Currency:        "USD",
				},
				Currency: "USD",
			},
			hasCompute:  true,
			hasNetwork:  true,
			hasStorage:  true,
			hasDatabase: true,
		},
		{
			name: "Microservices Blueprint",
			estimate: &CostEstimateWithRange{
				Estimate: 344.32,
				RangeMin: 275.46,
				RangeMax: 482.05,
				Breakdown: &MicroservicesCostBreakdown{
					Fargate:        174.90,
					ALB:            16.20,
					RDS:            52.56,
					S3:             5.00,
					CloudFront:     27.00,
					NATGateway:     41.85,
					CloudWatchLogs: 10.60,
					APIGateway:     3.50,
					SQS:            0.00,
					ElastiCache:    12.41,
					ECR:            0.30,
					TotalMonthly:   344.32,
					Currency:       "USD",
				},
				Currency: "USD",
			},
			hasCompute:  true,
			hasNetwork:  true,
			hasStorage:  true,
			hasDatabase: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			breakdown, err := bg.GenerateBreakdown(tt.estimate)
			if err != nil {
				t.Fatalf("GenerateBreakdown failed: %v", err)
			}

			// Validate that breakdown includes required categories
			// **Validates**: AC-1.2 (Estimate includes compute, networking, and storage costs)
			if tt.hasCompute && breakdown.Compute == 0 {
				t.Error("Expected compute costs but got 0")
			}
			if tt.hasNetwork && breakdown.Networking == 0 {
				t.Error("Expected networking costs but got 0")
			}
			if tt.hasStorage && breakdown.Storage == 0 {
				t.Error("Expected storage costs but got 0")
			}
			if tt.hasDatabase && breakdown.Database == 0 {
				t.Error("Expected database costs but got 0")
			}

			// Validate that breakdown sums to total
			// **Validates**: Cost Calculation Property 4 (Service breakdown sums to total estimate)
			calculatedSum := breakdown.Compute + breakdown.Networking +
				breakdown.Storage + breakdown.Database + breakdown.Other

			expectedTotal := tt.estimate.Estimate
			tolerance := 0.01

			diff := calculatedSum - expectedTotal
			if diff < 0 {
				diff = -diff
			}

			if diff > tolerance {
				t.Errorf("Breakdown sum (%.2f) does not match expected total (%.2f), difference: %.2f",
					calculatedSum, expectedTotal, diff)
			}

			// Validate that details are populated
			if len(breakdown.Details) == 0 {
				t.Error("Expected details to be populated")
			}

			// Validate that all costs are non-negative
			if breakdown.Compute < 0 {
				t.Errorf("Compute cost cannot be negative: %.2f", breakdown.Compute)
			}
			if breakdown.Networking < 0 {
				t.Errorf("Networking cost cannot be negative: %.2f", breakdown.Networking)
			}
			if breakdown.Storage < 0 {
				t.Errorf("Storage cost cannot be negative: %.2f", breakdown.Storage)
			}
			if breakdown.Database < 0 {
				t.Errorf("Database cost cannot be negative: %.2f", breakdown.Database)
			}
			if breakdown.Other < 0 {
				t.Errorf("Other cost cannot be negative: %.2f", breakdown.Other)
			}
		})
	}
}

// TestBreakdownIntegration_CategoryCoverage ensures all major AWS service
// categories are properly represented in breakdowns
// **Validates**: AC-1.2 (Estimate includes compute, networking, and storage costs)
func TestBreakdownIntegration_CategoryCoverage(t *testing.T) {
	bg := NewBreakdownGenerator()

	// Test that a comprehensive blueprint includes all categories
	estimate := &CostEstimateWithRange{
		Estimate: 344.32,
		Breakdown: &MicroservicesCostBreakdown{
			Fargate:        174.90, // Compute
			ALB:            16.20,  // Networking
			RDS:            52.56,  // Database
			S3:             5.00,   // Storage
			CloudFront:     27.00,  // Networking
			NATGateway:     41.85,  // Networking
			CloudWatchLogs: 10.60,  // Other
			APIGateway:     3.50,   // Networking
			SQS:            0.00,   // Other
			ElastiCache:    12.41,  // Database
			ECR:            0.30,   // Storage
			TotalMonthly:   344.32,
			Currency:       "USD",
		},
		Currency: "USD",
	}

	breakdown, err := bg.GenerateBreakdown(estimate)
	if err != nil {
		t.Fatalf("GenerateBreakdown failed: %v", err)
	}

	// Verify all major categories are present
	categories := map[string]float64{
		"compute":    breakdown.Compute,
		"networking": breakdown.Networking,
		"storage":    breakdown.Storage,
		"database":   breakdown.Database,
	}

	for category, cost := range categories {
		if cost <= 0 {
			t.Errorf("Expected %s category to have positive cost, got %.2f", category, cost)
		}
	}

	// Verify the breakdown includes the three required categories from AC-1.2
	requiredCategories := []string{"compute", "networking", "storage"}
	for _, category := range requiredCategories {
		if categories[category] <= 0 {
			t.Errorf("AC-1.2 requires %s costs, but got %.2f", category, categories[category])
		}
	}
}

// TestBreakdownIntegration_ServiceMapping verifies that specific AWS services
// are correctly mapped to their categories
func TestBreakdownIntegration_ServiceMapping(t *testing.T) {
	bg := NewBreakdownGenerator()

	tests := []struct {
		serviceName      string
		expectedCategory ServiceCategory
	}{
		// Compute services
		{"fargate", CategoryCompute},
		{"ec2", CategoryCompute},
		{"lambda", CategoryCompute},
		{"ecs", CategoryCompute},

		// Networking services
		{"alb", CategoryNetworking},
		{"cloudfront", CategoryNetworking},
		{"route53", CategoryNetworking},
		{"natGateway", CategoryNetworking},
		{"apiGateway", CategoryNetworking},

		// Storage services
		{"s3Storage", CategoryStorage},
		{"s3", CategoryStorage},
		{"ebs", CategoryStorage},
		{"ecr", CategoryStorage},

		// Database services
		{"rds", CategoryDatabase},
		{"dynamodb", CategoryDatabase},
		{"elasticache", CategoryDatabase},

		// Other services
		{"cloudwatchLogs", CategoryOther},
		{"sqs", CategoryOther},
	}

	for _, tt := range tests {
		t.Run(tt.serviceName, func(t *testing.T) {
			category := bg.categorizeService(tt.serviceName)
			if category != tt.expectedCategory {
				t.Errorf("Expected service %s to be in category %s, got %s",
					tt.serviceName, tt.expectedCategory, category)
			}
		})
	}
}

// TestBreakdownIntegration_ZeroCosts verifies that blueprints with zero costs
// in certain categories are handled correctly
func TestBreakdownIntegration_ZeroCosts(t *testing.T) {
	bg := NewBreakdownGenerator()

	// Static website has no compute or database costs
	estimate := &CostEstimateWithRange{
		Estimate: 12.30,
		Breakdown: &StaticWebsiteCostBreakdown{
			S3Storage:    2.30,
			S3Requests:   0.50,
			CloudFront:   9.00,
			Route53:      0.50,
			TotalMonthly: 12.30,
			Currency:     "USD",
		},
		Currency: "USD",
	}

	breakdown, err := bg.GenerateBreakdown(estimate)
	if err != nil {
		t.Fatalf("GenerateBreakdown failed: %v", err)
	}

	// Verify zero costs are handled correctly
	if breakdown.Compute != 0 {
		t.Errorf("Expected compute to be 0 for static website, got %.2f", breakdown.Compute)
	}

	if breakdown.Database != 0 {
		t.Errorf("Expected database to be 0 for static website, got %.2f", breakdown.Database)
	}

	// Verify non-zero costs are present
	if breakdown.Storage == 0 {
		t.Error("Expected storage costs for static website")
	}

	if breakdown.Networking == 0 {
		t.Error("Expected networking costs for static website")
	}
}

// TestBreakdownIntegration_FormattedOutput verifies that formatted output
// is user-friendly and includes all categories
func TestBreakdownIntegration_FormattedOutput(t *testing.T) {
	bg := NewBreakdownGenerator()

	breakdown := &CostBreakdownByCategory{
		Compute:    29.15,
		Networking: 16.20,
		Storage:    2.30,
		Database:   13.14,
		Other:      0.50,
		Total:      61.29,
		Details: map[string]float64{
			"fargate": 29.15,
			"alb":     16.20,
			"rds":     13.14,
			"s3":      2.30,
			"logs":    0.50,
		},
	}

	formatted := bg.FormatBreakdown(breakdown)

	// Verify all categories are present in formatted output
	requiredKeys := []string{"compute", "networking", "storage", "database", "other", "total"}
	for _, key := range requiredKeys {
		if _, exists := formatted[key]; !exists {
			t.Errorf("Expected formatted output to include %s", key)
		}
	}

	// Verify format includes dollar sign
	for key, value := range formatted {
		if len(value) == 0 || value[0] != '$' {
			t.Errorf("Expected formatted value for %s to start with $, got %s", key, value)
		}
	}
}
