package cost

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/PranavMagar/autostack/pkg/aws"
)

// Route53CostCalculator calculates costs for AWS Route53
type Route53CostCalculator struct {
	app          core.App
	pricingCache *aws.PricingCache
}

// NewRoute53CostCalculator creates a new Route53 cost calculator
func NewRoute53CostCalculator(app core.App) *Route53CostCalculator {
	return &Route53CostCalculator{
		app:          app,
		pricingCache: aws.NewPricingCache(app),
	}
}

// Route53Config represents Route53 configuration
type Route53Config struct {
	HostedZones           int   `json:"hostedZones"`           // Number of hosted zones
	StandardQueries       int64 `json:"standardQueries"`       // Standard queries per month
	LatencyBasedQueries   int64 `json:"latencyBasedQueries"`   // Latency-based routing queries
	GeoQueries            int64 `json:"geoQueries"`            // Geo DNS queries
	HealthChecks          int   `json:"healthChecks"`          // Number of health checks
	HealthCheckMeasurements int64 `json:"healthCheckMeasurements"` // Health check measurements
	Region                string `json:"region"`               // Region (for reference)
}

// Route53CostBreakdown represents the cost breakdown for Route53
type Route53CostBreakdown struct {
	HostedZoneCost        float64 `json:"hostedZoneCost"`
	StandardQueryCost     float64 `json:"standardQueryCost"`
	LatencyQueryCost      float64 `json:"latencyQueryCost"`
	GeoQueryCost          float64 `json:"geoQueryCost"`
	HealthCheckCost       float64 `json:"healthCheckCost"`
	TotalMonthly          float64 `json:"totalMonthly"`
	Currency              string  `json:"currency"`
}

// Calculate computes the monthly cost for Route53
func (r53c *Route53CostCalculator) Calculate(config Route53Config) (*Route53CostBreakdown, error) {
	// Validate configuration
	if err := r53c.validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid Route53 configuration: %w", err)
	}

	// Get pricing data
	hostedZonePricing := r53c.getHostedZonePricing()
	standardQueryPricing := r53c.getStandardQueryPricing()
	latencyQueryPricing := r53c.getLatencyQueryPricing()
	geoQueryPricing := r53c.getGeoQueryPricing()
	healthCheckPricing := r53c.getHealthCheckPricing()

	// Calculate costs
	hostedZoneCost := float64(config.HostedZones) * hostedZonePricing
	
	// Standard queries: First 1 billion queries/month are $0.40 per million
	standardQueryCost := r53c.calculateQueryCost(config.StandardQueries, standardQueryPricing)
	
	// Latency-based routing queries
	latencyQueryCost := r53c.calculateQueryCost(config.LatencyBasedQueries, latencyQueryPricing)
	
	// Geo DNS queries
	geoQueryCost := r53c.calculateQueryCost(config.GeoQueries, geoQueryPricing)
	
	// Health checks: $0.50 per health check per month
	healthCheckCost := float64(config.HealthChecks) * healthCheckPricing

	totalMonthly := hostedZoneCost + standardQueryCost + latencyQueryCost + geoQueryCost + healthCheckCost

	return &Route53CostBreakdown{
		HostedZoneCost:    roundToTwoDecimals(hostedZoneCost),
		StandardQueryCost: roundToTwoDecimals(standardQueryCost),
		LatencyQueryCost:  roundToTwoDecimals(latencyQueryCost),
		GeoQueryCost:      roundToTwoDecimals(geoQueryCost),
		HealthCheckCost:   roundToTwoDecimals(healthCheckCost),
		TotalMonthly:      roundToTwoDecimals(totalMonthly),
		Currency:          "USD",
	}, nil
}

// CalculateWithRange calculates cost with min/max range
func (r53c *Route53CostCalculator) CalculateWithRange(config Route53Config) (*CostEstimateWithRange, error) {
	breakdown, err := r53c.Calculate(config)
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

// validateConfig validates the Route53 configuration
func (r53c *Route53CostCalculator) validateConfig(config Route53Config) error {
	if config.HostedZones < 0 {
		return fmt.Errorf("hosted zones cannot be negative")
	}
	if config.StandardQueries < 0 {
		return fmt.Errorf("standard queries cannot be negative")
	}
	if config.LatencyBasedQueries < 0 {
		return fmt.Errorf("latency-based queries cannot be negative")
	}
	if config.GeoQueries < 0 {
		return fmt.Errorf("geo queries cannot be negative")
	}
	if config.HealthChecks < 0 {
		return fmt.Errorf("health checks cannot be negative")
	}
	return nil
}

// getHostedZonePricing returns hosted zone pricing per zone per month
func (r53c *Route53CostCalculator) getHostedZonePricing() float64 {
	// Try to get from cache
	pricingData, err := r53c.pricingCache.GetSpecificPricing(
		"global",
		"AmazonRoute53",
		"DNS Zone",
		"hosted-zone",
	)
	if err == nil {
		return pricingData.PricePerMonth
	}

	// Fallback to default pricing
	return 0.50 // $0.50 per hosted zone per month
}

// getStandardQueryPricing returns standard query pricing per million queries
func (r53c *Route53CostCalculator) getStandardQueryPricing() float64 {
	return 0.40 // $0.40 per million queries (first 1 billion)
}

// getLatencyQueryPricing returns latency-based routing query pricing per million
func (r53c *Route53CostCalculator) getLatencyQueryPricing() float64 {
	return 0.60 // $0.60 per million queries
}

// getGeoQueryPricing returns geo DNS query pricing per million
func (r53c *Route53CostCalculator) getGeoQueryPricing() float64 {
	return 0.70 // $0.70 per million queries
}

// getHealthCheckPricing returns health check pricing per check per month
func (r53c *Route53CostCalculator) getHealthCheckPricing() float64 {
	return 0.50 // $0.50 per health check per month
}

// calculateQueryCost calculates query cost with tiered pricing
func (r53c *Route53CostCalculator) calculateQueryCost(queries int64, pricePerMillion float64) float64 {
	if queries == 0 {
		return 0.0
	}

	// Convert to millions
	millions := float64(queries) / 1000000.0
	
	// Apply tiered pricing (simplified)
	// First 1 billion queries: base price
	// Over 1 billion: reduced price (not implemented for simplicity)
	
	return millions * pricePerMillion
}

// EstimateFromBlueprint estimates Route53 costs from blueprint configuration
func (r53c *Route53CostCalculator) EstimateFromBlueprint(blueprintConfig map[string]interface{}, region string) (*CostEstimateWithRange, error) {
	config := Route53Config{
		HostedZones:             getIntOrDefault(blueprintConfig, "hostedZones", 1),
		StandardQueries:         int64(getIntOrDefault(blueprintConfig, "standardQueries", 1000000)),
		LatencyBasedQueries:     int64(getIntOrDefault(blueprintConfig, "latencyBasedQueries", 0)),
		GeoQueries:              int64(getIntOrDefault(blueprintConfig, "geoQueries", 0)),
		HealthChecks:            getIntOrDefault(blueprintConfig, "healthChecks", 0),
		HealthCheckMeasurements: int64(getIntOrDefault(blueprintConfig, "healthCheckMeasurements", 0)),
		Region:                  region,
	}

	return r53c.CalculateWithRange(config)
}
