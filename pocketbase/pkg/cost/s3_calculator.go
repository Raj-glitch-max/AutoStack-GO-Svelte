package cost

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
)

// S3CostCalculator calculates costs for AWS S3
type S3CostCalculator struct {
	app          core.App
	pricingCache *aws.PricingCache
}

// NewS3CostCalculator creates a new S3 cost calculator
func NewS3CostCalculator(app core.App) *S3CostCalculator {
	return &S3CostCalculator{
		app:          app,
		pricingCache: aws.NewPricingCache(app),
	}
}

// S3Config represents S3 configuration
type S3Config struct {
	StorageGB           float64 `json:"storageGb"`           // Storage in GB
	StorageClass        string  `json:"storageClass"`        // STANDARD, INTELLIGENT_TIERING, etc.
	PUTRequests         int64   `json:"putRequests"`         // PUT/COPY/POST/LIST requests per month
	GETRequests         int64   `json:"getRequests"`         // GET/SELECT requests per month
	DataTransferOutGB   float64 `json:"dataTransferOutGb"`   // Data transfer out to internet (GB)
	DataRetrievalGB     float64 `json:"dataRetrievalGb"`     // Data retrieval for Glacier/Deep Archive
	Region              string  `json:"region"`              // AWS region
}

// S3CostBreakdown represents the cost breakdown for S3
type S3CostBreakdown struct {
	StorageCost         float64 `json:"storageCost"`
	PUTRequestCost      float64 `json:"putRequestCost"`
	GETRequestCost      float64 `json:"getRequestCost"`
	DataTransferCost    float64 `json:"dataTransferCost"`
	DataRetrievalCost   float64 `json:"dataRetrievalCost"`
	TotalMonthly        float64 `json:"totalMonthly"`
	Currency            string  `json:"currency"`
}

// Calculate computes the monthly cost for S3
func (s3c *S3CostCalculator) Calculate(config S3Config) (*S3CostBreakdown, error) {
	// Validate configuration
	if err := s3c.validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid S3 configuration: %w", err)
	}

	// Set default storage class
	if config.StorageClass == "" {
		config.StorageClass = "STANDARD"
	}

	// Get pricing data
	storagePricing := s3c.getStoragePricing(config.Region, config.StorageClass)
	putRequestPricing := s3c.getPUTRequestPricing(config.Region)
	getRequestPricing := s3c.getGETRequestPricing(config.Region)
	dataTransferPricing := s3c.getDataTransferPricing(config.Region)
	dataRetrievalPricing := s3c.getDataRetrievalPricing(config.Region, config.StorageClass)

	// Calculate costs
	storageCost := config.StorageGB * storagePricing
	putRequestCost := float64(config.PUTRequests) / 1000.0 * putRequestPricing
	getRequestCost := float64(config.GETRequests) / 1000.0 * getRequestPricing
	dataTransferCost := config.DataTransferOutGB * dataTransferPricing
	dataRetrievalCost := config.DataRetrievalGB * dataRetrievalPricing

	totalMonthly := storageCost + putRequestCost + getRequestCost + dataTransferCost + dataRetrievalCost

	return &S3CostBreakdown{
		StorageCost:       roundToTwoDecimals(storageCost),
		PUTRequestCost:    roundToTwoDecimals(putRequestCost),
		GETRequestCost:    roundToTwoDecimals(getRequestCost),
		DataTransferCost:  roundToTwoDecimals(dataTransferCost),
		DataRetrievalCost: roundToTwoDecimals(dataRetrievalCost),
		TotalMonthly:      roundToTwoDecimals(totalMonthly),
		Currency:          "USD",
	}, nil
}

// CalculateWithRange calculates cost with min/max range
func (s3c *S3CostCalculator) CalculateWithRange(config S3Config) (*CostEstimateWithRange, error) {
	breakdown, err := s3c.Calculate(config)
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

// validateConfig validates the S3 configuration
func (s3c *S3CostCalculator) validateConfig(config S3Config) error {
	if config.StorageGB < 0 {
		return fmt.Errorf("storage GB cannot be negative")
	}
	if config.PUTRequests < 0 {
		return fmt.Errorf("PUT requests cannot be negative")
	}
	if config.GETRequests < 0 {
		return fmt.Errorf("GET requests cannot be negative")
	}
	if config.DataTransferOutGB < 0 {
		return fmt.Errorf("data transfer out cannot be negative")
	}
	if config.Region == "" {
		return fmt.Errorf("region is required")
	}
	return nil
}

// getStoragePricing returns storage pricing per GB per month
func (s3c *S3CostCalculator) getStoragePricing(region, storageClass string) float64 {
	// Try to get from cache
	pricingData, err := s3c.pricingCache.GetSpecificPricing(
		region,
		"AmazonS3",
		"Storage",
		storageClass,
	)
	if err == nil {
		return pricingData.PricePerMonth
	}

	// Fallback to default pricing
	defaultPricing := map[string]map[string]float64{
		"STANDARD": {
			"us-east-1":      0.023,
			"us-east-2":      0.023,
			"us-west-1":      0.026,
			"us-west-2":      0.023,
			"eu-west-1":      0.024,
			"eu-west-2":      0.024,
			"eu-central-1":   0.025,
			"ap-southeast-1": 0.025,
			"ap-southeast-2": 0.026,
			"ap-northeast-1": 0.025,
		},
		"INTELLIGENT_TIERING": {
			"us-east-1": 0.023,
			"us-east-2": 0.023,
		},
	}

	if classPricing, exists := defaultPricing[storageClass]; exists {
		if price, exists := classPricing[region]; exists {
			return price
		}
	}

	// Default to STANDARD us-east-1 pricing
	return 0.023
}

// getPUTRequestPricing returns PUT request pricing per 1000 requests
func (s3c *S3CostCalculator) getPUTRequestPricing(region string) float64 {
	// Default pricing per 1000 PUT requests
	defaultPricing := map[string]float64{
		"us-east-1":      0.005,
		"us-east-2":      0.005,
		"us-west-1":      0.0055,
		"us-west-2":      0.005,
		"eu-west-1":      0.0055,
		"eu-west-2":      0.0055,
		"eu-central-1":   0.0055,
		"ap-southeast-1": 0.0055,
		"ap-southeast-2": 0.0055,
		"ap-northeast-1": 0.0055,
	}

	if price, exists := defaultPricing[region]; exists {
		return price
	}
	return 0.005
}

// getGETRequestPricing returns GET request pricing per 1000 requests
func (s3c *S3CostCalculator) getGETRequestPricing(region string) float64 {
	// Default pricing per 1000 GET requests
	defaultPricing := map[string]float64{
		"us-east-1":      0.0004,
		"us-east-2":      0.0004,
		"us-west-1":      0.00044,
		"us-west-2":      0.0004,
		"eu-west-1":      0.00044,
		"eu-west-2":      0.00044,
		"eu-central-1":   0.00044,
		"ap-southeast-1": 0.00044,
		"ap-southeast-2": 0.00044,
		"ap-northeast-1": 0.00044,
	}

	if price, exists := defaultPricing[region]; exists {
		return price
	}
	return 0.0004
}

// getDataTransferPricing returns data transfer out pricing per GB
func (s3c *S3CostCalculator) getDataTransferPricing(region string) float64 {
	// Default pricing per GB for data transfer out to internet
	// First 1 GB/month is free, then tiered pricing
	// Using average pricing for simplicity
	defaultPricing := map[string]float64{
		"us-east-1":      0.09,
		"us-east-2":      0.09,
		"us-west-1":      0.09,
		"us-west-2":      0.09,
		"eu-west-1":      0.09,
		"eu-west-2":      0.09,
		"eu-central-1":   0.09,
		"ap-southeast-1": 0.12,
		"ap-southeast-2": 0.14,
		"ap-northeast-1": 0.14,
	}

	if price, exists := defaultPricing[region]; exists {
		return price
	}
	return 0.09
}

// getDataRetrievalPricing returns data retrieval pricing per GB
func (s3c *S3CostCalculator) getDataRetrievalPricing(region, storageClass string) float64 {
	// Only applicable for Glacier and Deep Archive
	if storageClass != "GLACIER" && storageClass != "DEEP_ARCHIVE" {
		return 0.0
	}

	// Default retrieval pricing (standard retrieval)
	if storageClass == "GLACIER" {
		return 0.01 // $0.01 per GB
	}
	if storageClass == "DEEP_ARCHIVE" {
		return 0.02 // $0.02 per GB
	}

	return 0.0
}

// EstimateFromBlueprint estimates S3 costs from blueprint configuration
func (s3c *S3CostCalculator) EstimateFromBlueprint(blueprintConfig map[string]interface{}, region string) (*CostEstimateWithRange, error) {
	config := S3Config{
		StorageGB:         getFloat64OrDefault(blueprintConfig, "storageGb", 10),
		StorageClass:      getStringOrDefault(blueprintConfig, "storageClass", "STANDARD"),
		PUTRequests:       int64(getIntOrDefault(blueprintConfig, "putRequests", 1000)),
		GETRequests:       int64(getIntOrDefault(blueprintConfig, "getRequests", 10000)),
		DataTransferOutGB: getFloat64OrDefault(blueprintConfig, "dataTransferOutGb", 10),
		DataRetrievalGB:   getFloat64OrDefault(blueprintConfig, "dataRetrievalGb", 0),
		Region:            region,
	}

	return s3c.CalculateWithRange(config)
}

