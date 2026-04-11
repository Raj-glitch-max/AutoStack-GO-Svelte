package cost

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/Raj-glitch-max/autostack/pkg/aws"
)

// CloudFrontCostCalculator calculates costs for AWS CloudFront
type CloudFrontCostCalculator struct {
	app          core.App
	pricingCache *aws.PricingCache
}

// NewCloudFrontCostCalculator creates a new CloudFront cost calculator
func NewCloudFrontCostCalculator(app core.App) *CloudFrontCostCalculator {
	return &CloudFrontCostCalculator{
		app:          app,
		pricingCache: aws.NewPricingCache(app),
	}
}

// CloudFrontConfig represents CloudFront configuration
type CloudFrontConfig struct {
	DataTransferOutGB     float64 `json:"dataTransferOutGb"`     // Data transfer out to internet (GB)
	DataTransferToOriginGB float64 `json:"dataTransferToOriginGb"` // Data transfer to origin (GB)
	HTTPRequests          int64   `json:"httpRequests"`          // HTTP requests per month
	HTTPSRequests         int64   `json:"httpsRequests"`         // HTTPS requests per month
	InvalidationRequests  int     `json:"invalidationRequests"`  // Invalidation requests per month
	PriceClass            string  `json:"priceClass"`            // PriceClass_All, PriceClass_200, PriceClass_100
	Region                string  `json:"region"`                // Origin region (for reference)
}

// CloudFrontCostBreakdown represents the cost breakdown for CloudFront
type CloudFrontCostBreakdown struct {
	DataTransferCost      float64 `json:"dataTransferCost"`
	OriginTransferCost    float64 `json:"originTransferCost"`
	HTTPRequestCost       float64 `json:"httpRequestCost"`
	HTTPSRequestCost      float64 `json:"httpsRequestCost"`
	InvalidationCost      float64 `json:"invalidationCost"`
	TotalMonthly          float64 `json:"totalMonthly"`
	Currency              string  `json:"currency"`
}

// Calculate computes the monthly cost for CloudFront
func (cfc *CloudFrontCostCalculator) Calculate(config CloudFrontConfig) (*CloudFrontCostBreakdown, error) {
	// Validate configuration
	if err := cfc.validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid CloudFront configuration: %w", err)
	}

	// Set defaults
	if config.PriceClass == "" {
		config.PriceClass = "PriceClass_All"
	}

	// Get pricing data
	dataTransferPricing := cfc.getDataTransferPricing(config.DataTransferOutGB)
	originTransferPricing := cfc.getOriginTransferPricing()
	httpRequestPricing := cfc.getHTTPRequestPricing()
	httpsRequestPricing := cfc.getHTTPSRequestPricing()
	invalidationPricing := cfc.getInvalidationPricing()

	// Calculate costs
	dataTransferCost := config.DataTransferOutGB * dataTransferPricing
	originTransferCost := config.DataTransferToOriginGB * originTransferPricing
	httpRequestCost := float64(config.HTTPRequests) / 10000.0 * httpRequestPricing
	httpsRequestCost := float64(config.HTTPSRequests) / 10000.0 * httpsRequestPricing
	
	// First 1000 invalidation paths per month are free
	invalidationCost := 0.0
	if config.InvalidationRequests > 1000 {
		invalidationCost = float64(config.InvalidationRequests-1000) * invalidationPricing
	}

	totalMonthly := dataTransferCost + originTransferCost + httpRequestCost + httpsRequestCost + invalidationCost

	return &CloudFrontCostBreakdown{
		DataTransferCost:   roundToTwoDecimals(dataTransferCost),
		OriginTransferCost: roundToTwoDecimals(originTransferCost),
		HTTPRequestCost:    roundToTwoDecimals(httpRequestCost),
		HTTPSRequestCost:   roundToTwoDecimals(httpsRequestCost),
		InvalidationCost:   roundToTwoDecimals(invalidationCost),
		TotalMonthly:       roundToTwoDecimals(totalMonthly),
		Currency:           "USD",
	}, nil
}

// CalculateWithRange calculates cost with min/max range
func (cfc *CloudFrontCostCalculator) CalculateWithRange(config CloudFrontConfig) (*CostEstimateWithRange, error) {
	breakdown, err := cfc.Calculate(config)
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

// validateConfig validates the CloudFront configuration
func (cfc *CloudFrontCostCalculator) validateConfig(config CloudFrontConfig) error {
	if config.DataTransferOutGB < 0 {
		return fmt.Errorf("data transfer out cannot be negative")
	}
	if config.DataTransferToOriginGB < 0 {
		return fmt.Errorf("data transfer to origin cannot be negative")
	}
	if config.HTTPRequests < 0 {
		return fmt.Errorf("HTTP requests cannot be negative")
	}
	if config.HTTPSRequests < 0 {
		return fmt.Errorf("HTTPS requests cannot be negative")
	}
	if config.InvalidationRequests < 0 {
		return fmt.Errorf("invalidation requests cannot be negative")
	}
	return nil
}

// getDataTransferPricing returns tiered data transfer pricing per GB
func (cfc *CloudFrontCostCalculator) getDataTransferPricing(totalGB float64) float64 {
	// CloudFront uses tiered pricing (US, Europe, Israel)
	// Simplified average pricing
	
	if totalGB <= 10240 { // First 10 TB
		return 0.085
	} else if totalGB <= 51200 { // Next 40 TB
		return 0.080
	} else if totalGB <= 153600 { // Next 100 TB
		return 0.060
	} else if totalGB <= 512000 { // Next 350 TB
		return 0.040
	} else { // Over 500 TB
		return 0.030
	}
}

// getOriginTransferPricing returns data transfer to origin pricing per GB
func (cfc *CloudFrontCostCalculator) getOriginTransferPricing() float64 {
	// Data transfer from CloudFront to origin
	return 0.020 // $0.020 per GB
}

// getHTTPRequestPricing returns HTTP request pricing per 10,000 requests
func (cfc *CloudFrontCostCalculator) getHTTPRequestPricing() float64 {
	return 0.0075 // $0.0075 per 10,000 HTTP requests
}

// getHTTPSRequestPricing returns HTTPS request pricing per 10,000 requests
func (cfc *CloudFrontCostCalculator) getHTTPSRequestPricing() float64 {
	return 0.010 // $0.010 per 10,000 HTTPS requests
}

// getInvalidationPricing returns invalidation pricing per path
func (cfc *CloudFrontCostCalculator) getInvalidationPricing() float64 {
	return 0.005 // $0.005 per path after first 1000
}

// EstimateFromBlueprint estimates CloudFront costs from blueprint configuration
func (cfc *CloudFrontCostCalculator) EstimateFromBlueprint(blueprintConfig map[string]interface{}, region string) (*CostEstimateWithRange, error) {
	config := CloudFrontConfig{
		DataTransferOutGB:      getFloat64OrDefault(blueprintConfig, "dataTransferOutGb", 100),
		DataTransferToOriginGB: getFloat64OrDefault(blueprintConfig, "dataTransferToOriginGb", 10),
		HTTPRequests:           int64(getIntOrDefault(blueprintConfig, "httpRequests", 0)),
		HTTPSRequests:          int64(getIntOrDefault(blueprintConfig, "httpsRequests", 1000000)),
		InvalidationRequests:   getIntOrDefault(blueprintConfig, "invalidationRequests", 100),
		PriceClass:             getStringOrDefault(blueprintConfig, "priceClass", "PriceClass_All"),
		Region:                 region,
	}

	return cfc.CalculateWithRange(config)
}
