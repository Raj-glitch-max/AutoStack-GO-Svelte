package cost

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
)

// ALBCostCalculator calculates costs for AWS Application Load Balancer
type ALBCostCalculator struct {
	app          core.App
	pricingCache *aws.PricingCache
}

// NewALBCostCalculator creates a new ALB cost calculator
func NewALBCostCalculator(app core.App) *ALBCostCalculator {
	return &ALBCostCalculator{
		app:          app,
		pricingCache: aws.NewPricingCache(app),
	}
}

// ALBConfig represents ALB configuration
type ALBConfig struct {
	HoursPerMonth         float64 `json:"hoursPerMonth"`         // Hours running per month (default 730)
	NewConnectionsPerSec  float64 `json:"newConnectionsPerSec"`  // New connections per second
	ActiveConnectionsMin  float64 `json:"activeConnectionsMin"`  // Active connections per minute
	ProcessedBytesGB      float64 `json:"processedBytesGb"`      // Processed bytes in GB per month
	RuleEvaluations       int64   `json:"ruleEvaluations"`       // Rule evaluations per month
	Region                string  `json:"region"`                // AWS region
}

// ALBCostBreakdown represents the cost breakdown for ALB
type ALBCostBreakdown struct {
	FixedHourlyCost       float64 `json:"fixedHourlyCost"`
	LCUCost               float64 `json:"lcuCost"`
	TotalMonthly          float64 `json:"totalMonthly"`
	AverageLCUsPerHour    float64 `json:"averageLcusPerHour"`
	Currency              string  `json:"currency"`
}

// Calculate computes the monthly cost for ALB
func (albc *ALBCostCalculator) Calculate(config ALBConfig) (*ALBCostBreakdown, error) {
	// Validate configuration
	if err := albc.validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid ALB configuration: %w", err)
	}

	// Set defaults
	if config.HoursPerMonth == 0 {
		config.HoursPerMonth = 730 // ~30.4 days
	}

	// Get pricing data
	hourlyRate := albc.getHourlyRate(config.Region)
	lcuRate := albc.getLCURate(config.Region)

	// Calculate LCUs (Load Balancer Capacity Units)
	// LCU is the maximum of:
	// - New connections per second / 25
	// - Active connections per minute / 3000
	// - Processed bytes (GB) per hour / 1
	// - Rule evaluations per second / 1000

	newConnectionLCU := config.NewConnectionsPerSec / 25.0
	activeConnectionLCU := config.ActiveConnectionsMin / 3000.0
	processedBytesLCU := (config.ProcessedBytesGB / config.HoursPerMonth) // GB per hour
	ruleEvaluationLCU := (float64(config.RuleEvaluations) / config.HoursPerMonth / 3600.0) / 1000.0 // per second

	// Take the maximum LCU dimension
	maxLCU := max(newConnectionLCU, activeConnectionLCU, processedBytesLCU, ruleEvaluationLCU)

	// Calculate costs
	fixedCost := hourlyRate * config.HoursPerMonth
	lcuCost := lcuRate * maxLCU * config.HoursPerMonth
	totalMonthly := fixedCost + lcuCost

	return &ALBCostBreakdown{
		FixedHourlyCost:    roundToTwoDecimals(fixedCost),
		LCUCost:            roundToTwoDecimals(lcuCost),
		TotalMonthly:       roundToTwoDecimals(totalMonthly),
		AverageLCUsPerHour: roundToTwoDecimals(maxLCU),
		Currency:           "USD",
	}, nil
}

// CalculateWithRange calculates cost with min/max range
func (albc *ALBCostCalculator) CalculateWithRange(config ALBConfig) (*CostEstimateWithRange, error) {
	breakdown, err := albc.Calculate(config)
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

// validateConfig validates the ALB configuration
func (albc *ALBCostCalculator) validateConfig(config ALBConfig) error {
	if config.NewConnectionsPerSec < 0 {
		return fmt.Errorf("new connections per second cannot be negative")
	}
	if config.ActiveConnectionsMin < 0 {
		return fmt.Errorf("active connections per minute cannot be negative")
	}
	if config.ProcessedBytesGB < 0 {
		return fmt.Errorf("processed bytes cannot be negative")
	}
	if config.RuleEvaluations < 0 {
		return fmt.Errorf("rule evaluations cannot be negative")
	}
	if config.Region == "" {
		return fmt.Errorf("region is required")
	}
	return nil
}

// getHourlyRate returns the fixed hourly rate for ALB
func (albc *ALBCostCalculator) getHourlyRate(region string) float64 {
	// Try to get from cache
	pricingData, err := albc.pricingCache.GetSpecificPricing(
		region,
		"AmazonEC2",
		"Load Balancer",
		"application",
	)
	if err == nil {
		return pricingData.PricePerHour
	}

	// Fallback to default pricing
	defaultPricing := map[string]float64{
		"us-east-1":      0.0225,
		"us-east-2":      0.0225,
		"us-west-1":      0.025,
		"us-west-2":      0.0225,
		"eu-west-1":      0.0243,
		"eu-west-2":      0.0243,
		"eu-central-1":   0.0243,
		"ap-southeast-1": 0.025,
		"ap-southeast-2": 0.025,
		"ap-northeast-1": 0.025,
	}

	if price, exists := defaultPricing[region]; exists {
		return price
	}
	return 0.0225 // Default to us-east-1
}

// getLCURate returns the LCU rate per hour
func (albc *ALBCostCalculator) getLCURate(region string) float64 {
	// Try to get from cache
	pricingData, err := albc.pricingCache.GetSpecificPricing(
		region,
		"AmazonEC2",
		"Load Balancer-LCU",
		"application",
	)
	if err == nil {
		return pricingData.PricePerHour
	}

	// Fallback to default pricing
	defaultPricing := map[string]float64{
		"us-east-1":      0.008,
		"us-east-2":      0.008,
		"us-west-1":      0.009,
		"us-west-2":      0.008,
		"eu-west-1":      0.0086,
		"eu-west-2":      0.0086,
		"eu-central-1":   0.0086,
		"ap-southeast-1": 0.009,
		"ap-southeast-2": 0.009,
		"ap-northeast-1": 0.009,
	}

	if price, exists := defaultPricing[region]; exists {
		return price
	}
	return 0.008 // Default to us-east-1
}

// EstimateFromBlueprint estimates ALB costs from blueprint configuration
func (albc *ALBCostCalculator) EstimateFromBlueprint(blueprintConfig map[string]interface{}, region string) (*CostEstimateWithRange, error) {
	config := ALBConfig{
		HoursPerMonth:        getFloat64OrDefault(blueprintConfig, "hoursPerMonth", 730),
		NewConnectionsPerSec: getFloat64OrDefault(blueprintConfig, "newConnectionsPerSec", 25),
		ActiveConnectionsMin: getFloat64OrDefault(blueprintConfig, "activeConnectionsMin", 3000),
		ProcessedBytesGB:     getFloat64OrDefault(blueprintConfig, "processedBytesGb", 100),
		RuleEvaluations:      int64(getIntOrDefault(blueprintConfig, "ruleEvaluations", 1000000)),
		Region:               region,
	}

	return albc.CalculateWithRange(config)
}

// Helper function to get max of multiple float64 values
func max(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}
	maxVal := values[0]
	for _, v := range values[1:] {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}
