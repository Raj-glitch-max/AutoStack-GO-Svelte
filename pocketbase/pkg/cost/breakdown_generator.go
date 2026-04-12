package cost

import (
	"fmt"
)

// ServiceCategory represents a logical grouping of AWS services
type ServiceCategory string

const (
	CategoryCompute    ServiceCategory = "compute"
	CategoryNetworking ServiceCategory = "networking"
	CategoryStorage    ServiceCategory = "storage"
	CategoryDatabase   ServiceCategory = "database"
	CategoryOther      ServiceCategory = "other"
)

// CostBreakdownByCategory represents costs grouped by service category
// **Validates**: AC-1.2 (Estimate includes compute, networking, and storage costs)
type CostBreakdownByCategory struct {
	Compute    float64            `json:"compute"`
	Networking float64            `json:"networking"`
	Storage    float64            `json:"storage"`
	Database   float64            `json:"database"`
	Other      float64            `json:"other"`
	Total      float64            `json:"total"`
	Details    map[string]float64 `json:"details"` // Service-level breakdown
}

// BreakdownGenerator generates cost breakdowns by service category
type BreakdownGenerator struct{}

// NewBreakdownGenerator creates a new breakdown generator
func NewBreakdownGenerator() *BreakdownGenerator {
	return &BreakdownGenerator{}
}

// GenerateBreakdown generates a cost breakdown by service category from an estimate
// **Validates**: AC-1.2 (Estimate includes compute, networking, and storage costs)
// **Validates**: Cost Calculation Property 4 (Service breakdown sums to total estimate)
func (bg *BreakdownGenerator) GenerateBreakdown(estimate *CostEstimateWithRange) (*CostBreakdownByCategory, error) {
	if estimate == nil {
		return nil, fmt.Errorf("estimate cannot be nil")
	}

	if estimate.Breakdown == nil {
		// If no breakdown available, return total as "other"
		return &CostBreakdownByCategory{
			Compute:    0,
			Networking: 0,
			Storage:    0,
			Database:   0,
			Other:      estimate.Estimate,
			Total:      estimate.Estimate,
			Details:    map[string]float64{"total": estimate.Estimate},
		}, nil
	}

	// Extract breakdown based on type
	switch bd := estimate.Breakdown.(type) {
	case *StaticWebsiteCostBreakdown:
		return bg.breakdownStaticWebsite(bd)
	case *WebAppCostBreakdown:
		return bg.breakdownWebApp(bd)
	case *FullStackCostBreakdown:
		return bg.breakdownFullStack(bd)
	case *MicroservicesCostBreakdown:
		return bg.breakdownMicroservices(bd)
	case map[string]float64:
		return bg.breakdownFromMap(bd)
	case map[string]interface{}:
		return bg.breakdownFromInterfaceMap(bd)
	default:
		return nil, fmt.Errorf("unsupported breakdown type: %T", bd)
	}
}

// breakdownStaticWebsite categorizes static website costs
func (bg *BreakdownGenerator) breakdownStaticWebsite(bd *StaticWebsiteCostBreakdown) (*CostBreakdownByCategory, error) {
	breakdown := &CostBreakdownByCategory{
		Compute:    0, // Static websites have no compute
		Networking: bd.CloudFront + bd.Route53,
		Storage:    bd.S3Storage,
		Database:   0, // Static websites have no database
		Other:      bd.S3Requests,
		Details: map[string]float64{
			"s3Storage":   bd.S3Storage,
			"s3Requests":  bd.S3Requests,
			"cloudfront":  bd.CloudFront,
			"route53":     bd.Route53,
		},
	}

	breakdown.Total = breakdown.Compute + breakdown.Networking + 
		breakdown.Storage + breakdown.Database + breakdown.Other

	// Validate breakdown sums to total
	if err := bg.validateBreakdownSum(breakdown, bd.TotalMonthly); err != nil {
		return nil, err
	}

	return breakdown, nil
}

// breakdownWebApp categorizes web application costs
func (bg *BreakdownGenerator) breakdownWebApp(bd *WebAppCostBreakdown) (*CostBreakdownByCategory, error) {
	breakdown := &CostBreakdownByCategory{
		Compute:    bd.Fargate,
		Networking: bd.ALB + bd.NATGateway,
		Storage:    bd.ECR,
		Database:   bd.RDS,
		Other:      bd.CloudWatchLogs,
		Details: map[string]float64{
			"fargate":        bd.Fargate,
			"alb":            bd.ALB,
			"rds":            bd.RDS,
			"natGateway":     bd.NATGateway,
			"cloudwatchLogs": bd.CloudWatchLogs,
			"ecr":            bd.ECR,
		},
	}

	breakdown.Total = breakdown.Compute + breakdown.Networking + 
		breakdown.Storage + breakdown.Database + breakdown.Other

	// Validate breakdown sums to total
	if err := bg.validateBreakdownSum(breakdown, bd.TotalMonthly); err != nil {
		return nil, err
	}

	return breakdown, nil
}

// breakdownFullStack categorizes full-stack application costs
func (bg *BreakdownGenerator) breakdownFullStack(bd *FullStackCostBreakdown) (*CostBreakdownByCategory, error) {
	breakdown := &CostBreakdownByCategory{
		Compute:    bd.FrontendFargate + bd.BackendFargate,
		Networking: bd.ALB + bd.CloudFront + bd.NATGateway,
		Storage:    bd.S3Assets + bd.ECR,
		Database:   bd.RDS,
		Other:      bd.CloudWatchLogs,
		Details: map[string]float64{
			"frontendFargate": bd.FrontendFargate,
			"backendFargate":  bd.BackendFargate,
			"alb":             bd.ALB,
			"rds":             bd.RDS,
			"s3Assets":        bd.S3Assets,
			"cloudfront":      bd.CloudFront,
			"natGateway":      bd.NATGateway,
			"cloudwatchLogs":  bd.CloudWatchLogs,
			"ecr":             bd.ECR,
		},
	}

	breakdown.Total = breakdown.Compute + breakdown.Networking + 
		breakdown.Storage + breakdown.Database + breakdown.Other

	// Validate breakdown sums to total
	if err := bg.validateBreakdownSum(breakdown, bd.TotalMonthly); err != nil {
		return nil, err
	}

	return breakdown, nil
}

// breakdownMicroservices categorizes microservices architecture costs
func (bg *BreakdownGenerator) breakdownMicroservices(bd *MicroservicesCostBreakdown) (*CostBreakdownByCategory, error) {
	breakdown := &CostBreakdownByCategory{
		Compute:    bd.Fargate,
		Networking: bd.ALB + bd.CloudFront + bd.NATGateway + bd.APIGateway,
		Storage:    bd.S3 + bd.ECR,
		Database:   bd.RDS + bd.ElastiCache,
		Other:      bd.CloudWatchLogs + bd.SQS,
		Details: map[string]float64{
			"fargate":        bd.Fargate,
			"alb":            bd.ALB,
			"rds":            bd.RDS,
			"s3":             bd.S3,
			"cloudfront":     bd.CloudFront,
			"natGateway":     bd.NATGateway,
			"cloudwatchLogs": bd.CloudWatchLogs,
			"ecr":            bd.ECR,
			"apiGateway":     bd.APIGateway,
			"sqs":            bd.SQS,
			"elasticache":    bd.ElastiCache,
		},
	}

	breakdown.Total = breakdown.Compute + breakdown.Networking + 
		breakdown.Storage + breakdown.Database + breakdown.Other

	// Validate breakdown sums to total
	if err := bg.validateBreakdownSum(breakdown, bd.TotalMonthly); err != nil {
		return nil, err
	}

	return breakdown, nil
}

// breakdownFromMap categorizes costs from a simple map
func (bg *BreakdownGenerator) breakdownFromMap(bd map[string]float64) (*CostBreakdownByCategory, error) {
	breakdown := &CostBreakdownByCategory{
		Compute:    0,
		Networking: 0,
		Storage:    0,
		Database:   0,
		Other:      0,
		Details:    make(map[string]float64),
	}

	// Categorize each service
	for service, cost := range bd {
		breakdown.Details[service] = cost
		category := bg.categorizeService(service)
		
		switch category {
		case CategoryCompute:
			breakdown.Compute += cost
		case CategoryNetworking:
			breakdown.Networking += cost
		case CategoryStorage:
			breakdown.Storage += cost
		case CategoryDatabase:
			breakdown.Database += cost
		default:
			breakdown.Other += cost
		}
	}

	breakdown.Total = breakdown.Compute + breakdown.Networking + 
		breakdown.Storage + breakdown.Database + breakdown.Other

	return breakdown, nil
}

// breakdownFromInterfaceMap categorizes costs from an interface map
func (bg *BreakdownGenerator) breakdownFromInterfaceMap(bd map[string]interface{}) (*CostBreakdownByCategory, error) {
	// Convert interface map to float64 map
	floatMap := make(map[string]float64)
	for k, v := range bd {
		if floatVal, ok := v.(float64); ok {
			floatMap[k] = floatVal
		}
	}

	return bg.breakdownFromMap(floatMap)
}

// categorizeService determines the category for a service name
func (bg *BreakdownGenerator) categorizeService(serviceName string) ServiceCategory {
	// Compute services
	computeServices := map[string]bool{
		"fargate":   true,
		"ec2":       true,
		"lambda":    true,
		"ecs":       true,
		"compute":   true,
	}

	// Networking services
	networkingServices := map[string]bool{
		"alb":            true,
		"cloudfront":     true,
		"route53":        true,
		"natGateway":     true,
		"apiGateway":     true,
		"networking":     true,
		"dataTransfer":   true,
		"transfer":       true,
	}

	// Storage services
	storageServices := map[string]bool{
		"s3Storage":  true,
		"s3":         true,
		"ebs":        true,
		"ecr":        true,
		"storage":    true,
	}

	// Database services
	databaseServices := map[string]bool{
		"rds":          true,
		"dynamodb":     true,
		"elasticache":  true,
		"database":     true,
		"db":           true,
	}

	// Check each category
	if computeServices[serviceName] {
		return CategoryCompute
	}
	if networkingServices[serviceName] {
		return CategoryNetworking
	}
	if storageServices[serviceName] {
		return CategoryStorage
	}
	if databaseServices[serviceName] {
		return CategoryDatabase
	}

	return CategoryOther
}

// validateBreakdownSum validates that the breakdown sums to the expected total
// **Validates**: Cost Calculation Property 4 (Service breakdown sums to total estimate)
func (bg *BreakdownGenerator) validateBreakdownSum(breakdown *CostBreakdownByCategory, expectedTotal float64) error {
	// Round both values to 2 decimal places for comparison
	calculatedTotal := roundToTwoDecimals(breakdown.Total)
	expectedRounded := roundToTwoDecimals(expectedTotal)

	// Allow small floating point differences (0.01)
	tolerance := 0.01
	diff := calculatedTotal - expectedRounded
	if diff < 0 {
		diff = -diff
	}

	if diff > tolerance {
		return fmt.Errorf("breakdown sum (%.2f) does not match expected total (%.2f), difference: %.2f", 
			calculatedTotal, expectedRounded, diff)
	}

	return nil
}

// GetCategoryDescription returns a human-readable description of a service category
func (bg *BreakdownGenerator) GetCategoryDescription(category ServiceCategory) string {
	descriptions := map[ServiceCategory]string{
		CategoryCompute:    "Compute resources (Fargate, EC2, Lambda)",
		CategoryNetworking: "Networking services (ALB, CloudFront, NAT Gateway, Route53)",
		CategoryStorage:    "Storage services (S3, EBS, ECR)",
		CategoryDatabase:   "Database services (RDS, DynamoDB, ElastiCache)",
		CategoryOther:      "Other services (CloudWatch, SQS, API requests)",
	}

	if desc, exists := descriptions[category]; exists {
		return desc
	}

	return "Unknown category"
}

// FormatBreakdown formats a breakdown for display
func (bg *BreakdownGenerator) FormatBreakdown(breakdown *CostBreakdownByCategory) map[string]string {
	return map[string]string{
		"compute":    fmt.Sprintf("$%.2f", breakdown.Compute),
		"networking": fmt.Sprintf("$%.2f", breakdown.Networking),
		"storage":    fmt.Sprintf("$%.2f", breakdown.Storage),
		"database":   fmt.Sprintf("$%.2f", breakdown.Database),
		"other":      fmt.Sprintf("$%.2f", breakdown.Other),
		"total":      fmt.Sprintf("$%.2f", breakdown.Total),
	}
}
