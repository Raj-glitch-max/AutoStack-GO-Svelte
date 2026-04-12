package cost

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
)

// FargateCostCalculator calculates costs for AWS Fargate
type FargateCostCalculator struct {
	app          core.App
	pricingCache *aws.PricingCache
}

// NewFargateCostCalculator creates a new Fargate cost calculator
func NewFargateCostCalculator(app core.App) *FargateCostCalculator {
	return &FargateCostCalculator{
		app:          app,
		pricingCache: aws.NewPricingCache(app),
	}
}

// FargateConfig represents Fargate configuration
type FargateConfig struct {
	VCPU         float64 `json:"vcpu"`         // vCPU units (0.25, 0.5, 1, 2, 4, etc.)
	MemoryGB     float64 `json:"memoryGb"`     // Memory in GB
	TaskCount    int     `json:"taskCount"`    // Number of tasks
	HoursPerDay  float64 `json:"hoursPerDay"`  // Hours running per day (default 24)
	DaysPerMonth float64 `json:"daysPerMonth"` // Days per month (default 30)
	Region       string  `json:"region"`       // AWS region
}

// FargateCostBreakdown represents the cost breakdown for Fargate
type FargateCostBreakdown struct {
	VCPUCost      float64 `json:"vcpuCost"`
	MemoryCost    float64 `json:"memoryCost"`
	TotalHourly   float64 `json:"totalHourly"`
	TotalDaily    float64 `json:"totalDaily"`
	TotalMonthly  float64 `json:"totalMonthly"`
	VCPUHours     float64 `json:"vcpuHours"`
	MemoryGBHours float64 `json:"memoryGbHours"`
	Currency      string  `json:"currency"`
}

// Calculate computes the monthly cost for Fargate
func (fcc *FargateCostCalculator) Calculate(config FargateConfig) (*FargateCostBreakdown, error) {
	// Validate configuration
	if err := fcc.validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid Fargate configuration: %w", err)
	}

	// Set defaults
	if config.HoursPerDay == 0 {
		config.HoursPerDay = 24
	}
	if config.DaysPerMonth == 0 {
		config.DaysPerMonth = 30
	}

	// Get pricing data for vCPU
	vcpuPricing, err := fcc.getVCPUPricing(config.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to get vCPU pricing: %w", err)
	}

	// Get pricing data for memory
	memoryPricing, err := fcc.getMemoryPricing(config.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to get memory pricing: %w", err)
	}

	// Calculate total hours per month
	hoursPerMonth := config.HoursPerDay * config.DaysPerMonth

	// Calculate vCPU cost
	vcpuHours := config.VCPU * hoursPerMonth * float64(config.TaskCount)
	vcpuCost := vcpuHours * vcpuPricing

	// Calculate memory cost
	memoryGBHours := config.MemoryGB * hoursPerMonth * float64(config.TaskCount)
	memoryCost := memoryGBHours * memoryPricing

	// Calculate totals
	totalHourly := (config.VCPU*vcpuPricing + config.MemoryGB*memoryPricing) * float64(config.TaskCount)
	totalDaily := totalHourly * config.HoursPerDay
	totalMonthly := vcpuCost + memoryCost

	return &FargateCostBreakdown{
		VCPUCost:      roundToTwoDecimals(vcpuCost),
		MemoryCost:    roundToTwoDecimals(memoryCost),
		TotalHourly:   roundToTwoDecimals(totalHourly),
		TotalDaily:    roundToTwoDecimals(totalDaily),
		TotalMonthly:  roundToTwoDecimals(totalMonthly),
		VCPUHours:     roundToTwoDecimals(vcpuHours),
		MemoryGBHours: roundToTwoDecimals(memoryGBHours),
		Currency:      "USD",
	}, nil
}

// CalculateWithRange calculates cost with min/max range (±20% and +40%)
func (fcc *FargateCostCalculator) CalculateWithRange(config FargateConfig) (*CostEstimateWithRange, error) {
	breakdown, err := fcc.Calculate(config)
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

// validateConfig validates the Fargate configuration
func (fcc *FargateCostCalculator) validateConfig(config FargateConfig) error {
	// Validate vCPU
	validVCPU := []float64{0.25, 0.5, 1, 2, 4, 8, 16}
	isValidVCPU := false
	for _, v := range validVCPU {
		if config.VCPU == v {
			isValidVCPU = true
			break
		}
	}
	if !isValidVCPU {
		return fmt.Errorf("invalid vCPU value: %f (must be one of: 0.25, 0.5, 1, 2, 4, 8, 16)", config.VCPU)
	}

	// Validate memory (must be between 0.5GB and 120GB)
	if config.MemoryGB < 0.5 || config.MemoryGB > 120 {
		return fmt.Errorf("invalid memory: %f GB (must be between 0.5 and 120)", config.MemoryGB)
	}

	// Validate memory/vCPU ratio
	if err := fcc.validateMemoryVCPURatio(config.VCPU, config.MemoryGB); err != nil {
		return err
	}

	// Validate task count
	if config.TaskCount < 1 {
		return fmt.Errorf("task count must be at least 1")
	}

	// Validate region
	if config.Region == "" {
		return fmt.Errorf("region is required")
	}

	return nil
}

// validateMemoryVCPURatio validates the memory to vCPU ratio
func (fcc *FargateCostCalculator) validateMemoryVCPURatio(vcpu, memory float64) error {
	// AWS Fargate memory/vCPU ratio constraints
	ratioConstraints := map[float64]struct{ min, max float64 }{
		0.25: {0.5, 2},
		0.5:  {1, 4},
		1:    {2, 8},
		2:    {4, 16},
		4:    {8, 30},
		8:    {16, 60},
		16:   {32, 120},
	}

	constraint, exists := ratioConstraints[vcpu]
	if !exists {
		return fmt.Errorf("no memory constraints found for vCPU: %f", vcpu)
	}

	if memory < constraint.min || memory > constraint.max {
		return fmt.Errorf("memory %f GB is outside valid range for %f vCPU (must be between %f and %f GB)",
			memory, vcpu, constraint.min, constraint.max)
	}

	return nil
}

// getVCPUPricing retrieves vCPU pricing for the region
func (fcc *FargateCostCalculator) getVCPUPricing(region string) (float64, error) {
	pricingData, err := fcc.pricingCache.GetSpecificPricing(
		region,
		"AmazonECS",
		"Compute",
		"fargate-vcpu",
	)
	if err != nil {
		// Fallback to default pricing if not found
		return fcc.getDefaultVCPUPricing(region), nil
	}

	return pricingData.PricePerHour, nil
}

// getMemoryPricing retrieves memory pricing for the region
func (fcc *FargateCostCalculator) getMemoryPricing(region string) (float64, error) {
	pricingData, err := fcc.pricingCache.GetSpecificPricing(
		region,
		"AmazonECS",
		"Compute",
		"fargate-memory",
	)
	if err != nil {
		// Fallback to default pricing if not found
		return fcc.getDefaultMemoryPricing(region), nil
	}

	return pricingData.PricePerHour, nil
}

// getDefaultVCPUPricing returns default vCPU pricing by region
func (fcc *FargateCostCalculator) getDefaultVCPUPricing(region string) float64 {
	// Default pricing per vCPU per hour (as of 2024)
	defaultPricing := map[string]float64{
		"us-east-1":      0.04048,
		"us-east-2":      0.04048,
		"us-west-1":      0.04445,
		"us-west-2":      0.04048,
		"eu-west-1":      0.04456,
		"eu-west-2":      0.04663,
		"eu-central-1":   0.04856,
		"ap-southeast-1": 0.04865,
		"ap-southeast-2": 0.05056,
		"ap-northeast-1": 0.05252,
	}

	if price, exists := defaultPricing[region]; exists {
		return price
	}

	// Default to us-east-1 pricing if region not found
	return 0.04048
}

// getDefaultMemoryPricing returns default memory pricing by region
func (fcc *FargateCostCalculator) getDefaultMemoryPricing(region string) float64 {
	// Default pricing per GB per hour (as of 2024)
	defaultPricing := map[string]float64{
		"us-east-1":      0.004445,
		"us-east-2":      0.004445,
		"us-west-1":      0.004879,
		"us-west-2":      0.004445,
		"eu-west-1":      0.004890,
		"eu-west-2":      0.005117,
		"eu-central-1":   0.005330,
		"ap-southeast-1": 0.005341,
		"ap-southeast-2": 0.005549,
		"ap-northeast-1": 0.005767,
	}

	if price, exists := defaultPricing[region]; exists {
		return price
	}

	// Default to us-east-1 pricing if region not found
	return 0.004445
}

// EstimateFromBlueprint estimates Fargate costs from blueprint configuration
func (fcc *FargateCostCalculator) EstimateFromBlueprint(blueprintConfig map[string]interface{}, region string) (*CostEstimateWithRange, error) {
	// Extract Fargate configuration from blueprint
	config := FargateConfig{
		VCPU:         getFloat64OrDefault(blueprintConfig, "vcpu", 0.25),
		MemoryGB:     getFloat64OrDefault(blueprintConfig, "memory", 0.5),
		TaskCount:    getIntOrDefault(blueprintConfig, "taskCount", 1),
		HoursPerDay:  getFloat64OrDefault(blueprintConfig, "hoursPerDay", 24),
		DaysPerMonth: getFloat64OrDefault(blueprintConfig, "daysPerMonth", 30),
		Region:       region,
	}

	return fcc.CalculateWithRange(config)
}
