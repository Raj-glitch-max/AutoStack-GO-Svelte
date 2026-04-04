package cost

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// StaticWebsiteBlueprintCalculator calculates costs for static website blueprint
// Services: S3 storage, S3 requests, CloudFront data transfer, Route53
type StaticWebsiteBlueprintCalculator struct {
	app              core.App
	s3Calculator     *S3CostCalculator
	cloudfrontCalc   *CloudFrontCostCalculator
	route53Calc      *Route53CostCalculator
}

// NewStaticWebsiteBlueprintCalculator creates a new static website blueprint calculator
func NewStaticWebsiteBlueprintCalculator(app core.App) *StaticWebsiteBlueprintCalculator {
	return &StaticWebsiteBlueprintCalculator{
		app:            app,
		s3Calculator:   NewS3CostCalculator(app),
		cloudfrontCalc: NewCloudFrontCostCalculator(app),
		route53Calc:    NewRoute53CostCalculator(app),
	}
}

// StaticWebsiteConfig represents configuration for static website
type StaticWebsiteConfig struct {
	Region           string  `json:"region"`
	StorageGB        float64 `json:"storageGb"`        // Default: 10GB
	RequestsPerMonth int64   `json:"requestsPerMonth"` // Default: 10,000
	DataTransferGB   float64 `json:"dataTransferGb"`   // Default: 100GB
	HasCustomDomain  bool    `json:"hasCustomDomain"`  // Default: false
}

// StaticWebsiteCostBreakdown represents cost breakdown for static website
type StaticWebsiteCostBreakdown struct {
	S3Storage       float64                   `json:"s3Storage"`
	S3Requests      float64                   `json:"s3Requests"`
	CloudFront      float64                   `json:"cloudfront"`
	Route53         float64                   `json:"route53"`
	TotalMonthly    float64                   `json:"totalMonthly"`
	Currency        string                    `json:"currency"`
	Assumptions     map[string]string         `json:"assumptions"`
	ServiceDetails  map[string]interface{}    `json:"serviceDetails"`
}

// Calculate computes the monthly cost for static website blueprint
func (swc *StaticWebsiteBlueprintCalculator) Calculate(config StaticWebsiteConfig) (*StaticWebsiteCostBreakdown, error) {
	// Apply defaults
	if config.StorageGB == 0 {
		config.StorageGB = 10
	}
	if config.RequestsPerMonth == 0 {
		config.RequestsPerMonth = 10000
	}
	if config.DataTransferGB == 0 {
		config.DataTransferGB = 100
	}

	// Calculate S3 costs
	s3Config := S3Config{
		StorageGB:        config.StorageGB,
		RequestsPerMonth: config.RequestsPerMonth,
		DataTransferGB:   0, // CloudFront handles data transfer
		Region:           config.Region,
	}
	s3Breakdown, err := swc.s3Calculator.Calculate(s3Config)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate S3 costs: %w", err)
	}

	// Calculate CloudFront costs
	cloudfrontConfig := CloudFrontConfig{
		DataTransferGB:   config.DataTransferGB,
		RequestsPerMonth: config.RequestsPerMonth,
		Region:           config.Region,
	}
	cloudfrontBreakdown, err := swc.cloudfrontCalc.Calculate(cloudfrontConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate CloudFront costs: %w", err)
	}

	// Calculate Route53 costs (only if custom domain)
	route53Cost := 0.0
	if config.HasCustomDomain {
		route53Config := Route53Config{
			HostedZones:      1,
			QueriesPerMonth:  config.RequestsPerMonth,
			HealthChecks:     0,
			Region:           config.Region,
		}
		route53Breakdown, err := swc.route53Calc.Calculate(route53Config)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate Route53 costs: %w", err)
		}
		route53Cost = route53Breakdown.TotalMonthly
	}

	// Calculate total
	totalMonthly := s3Breakdown.TotalMonthly + cloudfrontBreakdown.TotalMonthly + route53Cost

	// Build assumptions
	assumptions := map[string]string{
		"storage":         fmt.Sprintf("%.0f GB", config.StorageGB),
		"requests":        fmt.Sprintf("%d per month", config.RequestsPerMonth),
		"dataTransfer":    fmt.Sprintf("%.0f GB per month", config.DataTransferGB),
		"customDomain":    fmt.Sprintf("%t", config.HasCustomDomain),
		"cacheHitRatio":   "80%",
		"compressionRate": "60%",
	}

	// Build service details
	serviceDetails := map[string]interface{}{
		"s3":         s3Breakdown,
		"cloudfront": cloudfrontBreakdown,
	}

	return &StaticWebsiteCostBreakdown{
		S3Storage:      s3Breakdown.StorageCost,
		S3Requests:     s3Breakdown.RequestCost,
		CloudFront:     cloudfrontBreakdown.TotalMonthly,
		Route53:        route53Cost,
		TotalMonthly:   roundToTwoDecimals(totalMonthly),
		Currency:       "USD",
		Assumptions:    assumptions,
		ServiceDetails: serviceDetails,
	}, nil
}

// CalculateWithRange calculates cost with min/max range
func (swc *StaticWebsiteBlueprintCalculator) CalculateWithRange(config StaticWebsiteConfig) (*CostEstimateWithRange, error) {
	breakdown, err := swc.Calculate(config)
	if err != nil {
		return nil, err
	}

	return &CostEstimateWithRange{
		Estimate:  breakdown.TotalMonthly,
		RangeMin:  roundToTwoDecimals(breakdown.TotalMonthly * 0.8),
		RangeMax:  roundToTwoDecimals(breakdown.TotalMonthly * 1.4),
		Breakdown: breakdown,
		Currency:  "USD",
	}, nil
}

// EstimateFromBlueprint estimates costs from blueprint configuration
func (swc *StaticWebsiteBlueprintCalculator) EstimateFromBlueprint(blueprintConfig map[string]interface{}, region string) (*CostEstimateWithRange, error) {
	config := StaticWebsiteConfig{
		Region:           region,
		StorageGB:        getFloat64OrDefault(blueprintConfig, "storage_gb", 10),
		RequestsPerMonth: int64(getIntOrDefault(blueprintConfig, "requests_per_month", 10000)),
		DataTransferGB:   getFloat64OrDefault(blueprintConfig, "data_transfer_gb", 100),
		HasCustomDomain:  getBoolOrDefault(blueprintConfig, "has_custom_domain", false),
	}

	return swc.CalculateWithRange(config)
}

// GetDefaultAssumptions returns default usage assumptions for static website
func (swc *StaticWebsiteBlueprintCalculator) GetDefaultAssumptions() map[string]string {
	return map[string]string{
		"storage":         "10 GB",
		"requests":        "10,000 per month",
		"dataTransfer":    "100 GB per month",
		"customDomain":    "false",
		"cacheHitRatio":   "80%",
		"compressionRate": "60%",
	}
}

// GetDisclaimer returns cost disclaimer for static website
func (swc *StaticWebsiteBlueprintCalculator) GetDisclaimer() string {
	return "Excludes: Data transfer overages beyond 100GB, CloudWatch detailed monitoring, " +
		"AWS WAF if enabled, SSL certificate costs (if using custom domain), " +
		"Lambda@Edge functions, and CloudFront invalidation requests."
}

// Helper function to get bool from map
func getBoolOrDefault(m map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case bool:
			return v
		case string:
			return v == "true" || v == "yes" || v == "1"
		}
	}
	return defaultValue
}
