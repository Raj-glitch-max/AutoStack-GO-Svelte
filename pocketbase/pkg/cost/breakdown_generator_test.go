package cost

import (
	"testing"
)

func TestBreakdownGenerator_GenerateBreakdown_StaticWebsite(t *testing.T) {
	bg := NewBreakdownGenerator()

	// Create a static website breakdown
	staticBreakdown := &StaticWebsiteCostBreakdown{
		S3Storage:    2.30,
		S3Requests:   0.50,
		CloudFront:   9.00,
		Route53:      0.50,
		TotalMonthly: 12.30,
		Currency:     "USD",
	}

	estimate := &CostEstimateWithRange{
		Estimate:  12.30,
		RangeMin:  9.84,
		RangeMax:  17.22,
		Breakdown: staticBreakdown,
		Currency:  "USD",
	}

	breakdown, err := bg.GenerateBreakdown(estimate)
	if err != nil {
		t.Fatalf("GenerateBreakdown failed: %v", err)
	}

	// Validate categories
	if breakdown.Compute != 0 {
		t.Errorf("Expected compute to be 0, got %.2f", breakdown.Compute)
	}

	expectedStorage := 2.30
	if breakdown.Storage != expectedStorage {
		t.Errorf("Expected storage to be %.2f, got %.2f", expectedStorage, breakdown.Storage)
	}

	expectedNetworking := 9.50 // CloudFront + Route53
	if breakdown.Networking != expectedNetworking {
		t.Errorf("Expected networking to be %.2f, got %.2f", expectedNetworking, breakdown.Networking)
	}

	if breakdown.Database != 0 {
		t.Errorf("Expected database to be 0, got %.2f", breakdown.Database)
	}

	expectedOther := 0.50 // S3Requests
	if breakdown.Other != expectedOther {
		t.Errorf("Expected other to be %.2f, got %.2f", expectedOther, breakdown.Other)
	}

	// Validate total
	if breakdown.Total != staticBreakdown.TotalMonthly {
		t.Errorf("Expected total to be %.2f, got %.2f", staticBreakdown.TotalMonthly, breakdown.Total)
	}

	// Validate details
	if len(breakdown.Details) == 0 {
		t.Error("Expected details to be populated")
	}
}

func TestBreakdownGenerator_GenerateBreakdown_WebApp(t *testing.T) {
	bg := NewBreakdownGenerator()

	// Create a web app breakdown
	webAppBreakdown := &WebAppCostBreakdown{
		Fargate:        29.15,
		ALB:            16.20,
		RDS:            13.14,
		NATGateway:     35.10,
		CloudWatchLogs: 2.65,
		ECR:            0.10,
		TotalMonthly:   96.34,
		Currency:       "USD",
	}

	estimate := &CostEstimateWithRange{
		Estimate:  96.34,
		RangeMin:  77.07,
		RangeMax:  134.88,
		Breakdown: webAppBreakdown,
		Currency:  "USD",
	}

	breakdown, err := bg.GenerateBreakdown(estimate)
	if err != nil {
		t.Fatalf("GenerateBreakdown failed: %v", err)
	}

	// Validate categories
	expectedCompute := 29.15
	if breakdown.Compute != expectedCompute {
		t.Errorf("Expected compute to be %.2f, got %.2f", expectedCompute, breakdown.Compute)
	}

	expectedNetworking := 51.30 // ALB + NATGateway
	if breakdown.Networking != expectedNetworking {
		t.Errorf("Expected networking to be %.2f, got %.2f", expectedNetworking, breakdown.Networking)
	}

	expectedStorage := 0.10 // ECR
	if breakdown.Storage != expectedStorage {
		t.Errorf("Expected storage to be %.2f, got %.2f", expectedStorage, breakdown.Storage)
	}

	expectedDatabase := 13.14
	if breakdown.Database != expectedDatabase {
		t.Errorf("Expected database to be %.2f, got %.2f", expectedDatabase, breakdown.Database)
	}

	expectedOther := 2.65 // CloudWatchLogs
	if breakdown.Other != expectedOther {
		t.Errorf("Expected other to be %.2f, got %.2f", expectedOther, breakdown.Other)
	}

	// Validate total
	expectedTotal := roundToTwoDecimals(webAppBreakdown.TotalMonthly)
	actualTotal := roundToTwoDecimals(breakdown.Total)
	if actualTotal != expectedTotal {
		t.Errorf("Expected total to be %.2f, got %.2f", expectedTotal, actualTotal)
	}
}

func TestBreakdownGenerator_GenerateBreakdown_FullStack(t *testing.T) {
	bg := NewBreakdownGenerator()

	// Create a full-stack breakdown
	fullStackBreakdown := &FullStackCostBreakdown{
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
	}

	estimate := &CostEstimateWithRange{
		Estimate:  178.21,
		RangeMin:  142.57,
		RangeMax:  249.49,
		Breakdown: fullStackBreakdown,
		Currency:  "USD",
	}

	breakdown, err := bg.GenerateBreakdown(estimate)
	if err != nil {
		t.Fatalf("GenerateBreakdown failed: %v", err)
	}

	// Validate categories
	expectedCompute := 72.88 // FrontendFargate + BackendFargate
	if breakdown.Compute != expectedCompute {
		t.Errorf("Expected compute to be %.2f, got %.2f", expectedCompute, breakdown.Compute)
	}

	expectedNetworking := roundToTwoDecimals(71.55) // ALB + CloudFront + NATGateway
	actualNetworking := roundToTwoDecimals(breakdown.Networking)
	if actualNetworking != expectedNetworking {
		t.Errorf("Expected networking to be %.2f, got %.2f", expectedNetworking, actualNetworking)
	}

	expectedStorage := 2.20 // S3Assets + ECR
	if breakdown.Storage != expectedStorage {
		t.Errorf("Expected storage to be %.2f, got %.2f", expectedStorage, breakdown.Storage)
	}

	expectedDatabase := 26.28
	if breakdown.Database != expectedDatabase {
		t.Errorf("Expected database to be %.2f, got %.2f", expectedDatabase, breakdown.Database)
	}

	expectedOther := 5.30 // CloudWatchLogs
	if breakdown.Other != expectedOther {
		t.Errorf("Expected other to be %.2f, got %.2f", expectedOther, breakdown.Other)
	}

	// Validate total
	expectedTotal := roundToTwoDecimals(fullStackBreakdown.TotalMonthly)
	actualTotal := roundToTwoDecimals(breakdown.Total)
	if actualTotal != expectedTotal {
		t.Errorf("Expected total to be %.2f, got %.2f", expectedTotal, actualTotal)
	}
}

func TestBreakdownGenerator_GenerateBreakdown_Microservices(t *testing.T) {
	bg := NewBreakdownGenerator()

	// Create a microservices breakdown
	microservicesBreakdown := &MicroservicesCostBreakdown{
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
	}

	estimate := &CostEstimateWithRange{
		Estimate:  344.32,
		RangeMin:  275.46,
		RangeMax:  482.05,
		Breakdown: microservicesBreakdown,
		Currency:  "USD",
	}

	breakdown, err := bg.GenerateBreakdown(estimate)
	if err != nil {
		t.Fatalf("GenerateBreakdown failed: %v", err)
	}

	// Validate categories
	expectedCompute := 174.90
	if breakdown.Compute != expectedCompute {
		t.Errorf("Expected compute to be %.2f, got %.2f", expectedCompute, breakdown.Compute)
	}

	expectedNetworking := roundToTwoDecimals(88.55) // ALB + CloudFront + NATGateway + APIGateway
	actualNetworking := roundToTwoDecimals(breakdown.Networking)
	if actualNetworking != expectedNetworking {
		t.Errorf("Expected networking to be %.2f, got %.2f", expectedNetworking, actualNetworking)
	}

	expectedStorage := 5.30 // S3 + ECR
	if breakdown.Storage != expectedStorage {
		t.Errorf("Expected storage to be %.2f, got %.2f", expectedStorage, breakdown.Storage)
	}

	expectedDatabase := 64.97 // RDS + ElastiCache
	if breakdown.Database != expectedDatabase {
		t.Errorf("Expected database to be %.2f, got %.2f", expectedDatabase, breakdown.Database)
	}

	expectedOther := 10.60 // CloudWatchLogs + SQS
	if breakdown.Other != expectedOther {
		t.Errorf("Expected other to be %.2f, got %.2f", expectedOther, breakdown.Other)
	}

	// Validate total
	expectedTotal := roundToTwoDecimals(microservicesBreakdown.TotalMonthly)
	actualTotal := roundToTwoDecimals(breakdown.Total)
	if actualTotal != expectedTotal {
		t.Errorf("Expected total to be %.2f, got %.2f", expectedTotal, actualTotal)
	}
}

func TestBreakdownGenerator_GenerateBreakdown_NilEstimate(t *testing.T) {
	bg := NewBreakdownGenerator()

	_, err := bg.GenerateBreakdown(nil)
	if err == nil {
		t.Error("Expected error for nil estimate, got nil")
	}
}

func TestBreakdownGenerator_GenerateBreakdown_NilBreakdown(t *testing.T) {
	bg := NewBreakdownGenerator()

	estimate := &CostEstimateWithRange{
		Estimate:  50.00,
		RangeMin:  40.00,
		RangeMax:  70.00,
		Breakdown: nil,
		Currency:  "USD",
	}

	breakdown, err := bg.GenerateBreakdown(estimate)
	if err != nil {
		t.Fatalf("GenerateBreakdown failed: %v", err)
	}

	// Should return total as "other"
	if breakdown.Other != 50.00 {
		t.Errorf("Expected other to be 50.00, got %.2f", breakdown.Other)
	}

	if breakdown.Total != 50.00 {
		t.Errorf("Expected total to be 50.00, got %.2f", breakdown.Total)
	}
}

func TestBreakdownGenerator_GenerateBreakdown_MapBreakdown(t *testing.T) {
	bg := NewBreakdownGenerator()

	mapBreakdown := map[string]float64{
		"fargate":   29.15,
		"alb":       16.20,
		"rds":       13.14,
		"s3Storage": 2.30,
	}

	estimate := &CostEstimateWithRange{
		Estimate:  60.79,
		RangeMin:  48.63,
		RangeMax:  85.11,
		Breakdown: mapBreakdown,
		Currency:  "USD",
	}

	breakdown, err := bg.GenerateBreakdown(estimate)
	if err != nil {
		t.Fatalf("GenerateBreakdown failed: %v", err)
	}

	// Validate categorization
	if breakdown.Compute != 29.15 {
		t.Errorf("Expected compute to be 29.15, got %.2f", breakdown.Compute)
	}

	if breakdown.Networking != 16.20 {
		t.Errorf("Expected networking to be 16.20, got %.2f", breakdown.Networking)
	}

	if breakdown.Storage != 2.30 {
		t.Errorf("Expected storage to be 2.30, got %.2f", breakdown.Storage)
	}

	if breakdown.Database != 13.14 {
		t.Errorf("Expected database to be 13.14, got %.2f", breakdown.Database)
	}
}

func TestBreakdownGenerator_CategorizeService(t *testing.T) {
	bg := NewBreakdownGenerator()

	tests := []struct {
		service  string
		expected ServiceCategory
	}{
		{"fargate", CategoryCompute},
		{"ec2", CategoryCompute},
		{"lambda", CategoryCompute},
		{"alb", CategoryNetworking},
		{"cloudfront", CategoryNetworking},
		{"route53", CategoryNetworking},
		{"natGateway", CategoryNetworking},
		{"s3Storage", CategoryStorage},
		{"s3", CategoryStorage},
		{"ecr", CategoryStorage},
		{"rds", CategoryDatabase},
		{"dynamodb", CategoryDatabase},
		{"elasticache", CategoryDatabase},
		{"cloudwatchLogs", CategoryOther},
		{"sqs", CategoryOther},
		{"unknown", CategoryOther},
	}

	for _, tt := range tests {
		t.Run(tt.service, func(t *testing.T) {
			category := bg.categorizeService(tt.service)
			if category != tt.expected {
				t.Errorf("Expected category %s for service %s, got %s", tt.expected, tt.service, category)
			}
		})
	}
}

func TestBreakdownGenerator_ValidateBreakdownSum(t *testing.T) {
	bg := NewBreakdownGenerator()

	tests := []struct {
		name          string
		breakdown     *CostBreakdownByCategory
		expectedTotal float64
		shouldError   bool
	}{
		{
			name: "Valid breakdown",
			breakdown: &CostBreakdownByCategory{
				Compute:    10.00,
				Networking: 20.00,
				Storage:    5.00,
				Database:   15.00,
				Other:      0.00,
				Total:      50.00,
			},
			expectedTotal: 50.00,
			shouldError:   false,
		},
		{
			name: "Valid breakdown with rounding",
			breakdown: &CostBreakdownByCategory{
				Compute:    10.01,
				Networking: 20.02,
				Storage:    5.03,
				Database:   14.94,
				Other:      0.00,
				Total:      50.00,
			},
			expectedTotal: 50.00,
			shouldError:   false,
		},
		{
			name: "Invalid breakdown - sum mismatch",
			breakdown: &CostBreakdownByCategory{
				Compute:    10.00,
				Networking: 20.00,
				Storage:    5.00,
				Database:   15.00,
				Other:      0.00,
				Total:      50.00,
			},
			expectedTotal: 60.00,
			shouldError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := bg.validateBreakdownSum(tt.breakdown, tt.expectedTotal)
			if tt.shouldError && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
		})
	}
}

func TestBreakdownGenerator_GetCategoryDescription(t *testing.T) {
	bg := NewBreakdownGenerator()

	tests := []struct {
		category ServiceCategory
		expected string
	}{
		{CategoryCompute, "Compute resources (Fargate, EC2, Lambda)"},
		{CategoryNetworking, "Networking services (ALB, CloudFront, NAT Gateway, Route53)"},
		{CategoryStorage, "Storage services (S3, EBS, ECR)"},
		{CategoryDatabase, "Database services (RDS, DynamoDB, ElastiCache)"},
		{CategoryOther, "Other services (CloudWatch, SQS, API requests)"},
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			description := bg.GetCategoryDescription(tt.category)
			if description != tt.expected {
				t.Errorf("Expected description '%s', got '%s'", tt.expected, description)
			}
		})
	}
}

func TestBreakdownGenerator_FormatBreakdown(t *testing.T) {
	bg := NewBreakdownGenerator()

	breakdown := &CostBreakdownByCategory{
		Compute:    29.15,
		Networking: 16.20,
		Storage:    2.30,
		Database:   13.14,
		Other:      0.50,
		Total:      61.29,
	}

	formatted := bg.FormatBreakdown(breakdown)

	if formatted["compute"] != "$29.15" {
		t.Errorf("Expected compute to be '$29.15', got '%s'", formatted["compute"])
	}

	if formatted["networking"] != "$16.20" {
		t.Errorf("Expected networking to be '$16.20', got '%s'", formatted["networking"])
	}

	if formatted["storage"] != "$2.30" {
		t.Errorf("Expected storage to be '$2.30', got '%s'", formatted["storage"])
	}

	if formatted["database"] != "$13.14" {
		t.Errorf("Expected database to be '$13.14', got '%s'", formatted["database"])
	}

	if formatted["other"] != "$0.50" {
		t.Errorf("Expected other to be '$0.50', got '%s'", formatted["other"])
	}

	if formatted["total"] != "$61.29" {
		t.Errorf("Expected total to be '$61.29', got '%s'", formatted["total"])
	}
}

// Test that breakdown sums match total for all blueprint types
func TestBreakdownGenerator_BreakdownSumsToTotal(t *testing.T) {
	bg := NewBreakdownGenerator()

	tests := []struct {
		name      string
		breakdown interface{}
		total     float64
	}{
		{
			name: "Static Website",
			breakdown: &StaticWebsiteCostBreakdown{
				S3Storage:    2.30,
				S3Requests:   0.50,
				CloudFront:   9.00,
				Route53:      0.50,
				TotalMonthly: 12.30,
			},
			total: 12.30,
		},
		{
			name: "Web App",
			breakdown: &WebAppCostBreakdown{
				Fargate:        29.15,
				ALB:            16.20,
				RDS:            13.14,
				NATGateway:     35.10,
				CloudWatchLogs: 2.65,
				ECR:            0.10,
				TotalMonthly:   96.34,
			},
			total: 96.34,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			estimate := &CostEstimateWithRange{
				Estimate:  tt.total,
				Breakdown: tt.breakdown,
			}

			categoryBreakdown, err := bg.GenerateBreakdown(estimate)
			if err != nil {
				t.Fatalf("GenerateBreakdown failed: %v", err)
			}

			// Validate that the sum of categories equals the total
			calculatedSum := categoryBreakdown.Compute + categoryBreakdown.Networking +
				categoryBreakdown.Storage + categoryBreakdown.Database + categoryBreakdown.Other

			if roundToTwoDecimals(calculatedSum) != roundToTwoDecimals(tt.total) {
				t.Errorf("Breakdown sum (%.2f) does not match expected total (%.2f)",
					calculatedSum, tt.total)
			}
		})
	}
}
