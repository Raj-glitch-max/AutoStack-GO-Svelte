package cost

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/PranavMagar/autostack/pkg/aws"
)

// RDSCostCalculator calculates costs for AWS RDS
type RDSCostCalculator struct {
	app          core.App
	pricingCache *aws.PricingCache
}

// NewRDSCostCalculator creates a new RDS cost calculator
func NewRDSCostCalculator(app core.App) *RDSCostCalculator {
	return &RDSCostCalculator{
		app:          app,
		pricingCache: aws.NewPricingCache(app),
	}
}

// RDSConfig represents RDS configuration
type RDSConfig struct {
	InstanceClass     string  `json:"instanceClass"`     // e.g., db.t3.micro, db.t3.small
	Engine            string  `json:"engine"`            // postgres, mysql, mariadb, etc.
	StorageGB         float64 `json:"storageGb"`         // Storage in GB
	StorageType       string  `json:"storageType"`       // gp2, gp3, io1
	IOPS              int     `json:"iops"`              // Provisioned IOPS (for io1)
	BackupStorageGB   float64 `json:"backupStorageGb"`   // Backup storage in GB
	MultiAZ           bool    `json:"multiAz"`           // Multi-AZ deployment
	HoursPerMonth     float64 `json:"hoursPerMonth"`     // Hours running per month (default 730)
	Region            string  `json:"region"`            // AWS region
}

// RDSCostBreakdown represents the cost breakdown for RDS
type RDSCostBreakdown struct {
	InstanceCost      float64 `json:"instanceCost"`
	StorageCost       float64 `json:"storageCost"`
	IOPSCost          float64 `json:"iopsCost"`
	BackupStorageCost float64 `json:"backupStorageCost"`
	TotalMonthly      float64 `json:"totalMonthly"`
	Currency          string  `json:"currency"`
}

// Calculate computes the monthly cost for RDS
func (rdsc *RDSCostCalculator) Calculate(config RDSConfig) (*RDSCostBreakdown, error) {
	// Validate configuration
	if err := rdsc.validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid RDS configuration: %w", err)
	}

	// Set defaults
	if config.HoursPerMonth == 0 {
		config.HoursPerMonth = 730
	}
	if config.StorageType == "" {
		config.StorageType = "gp3"
	}
	if config.Engine == "" {
		config.Engine = "postgres"
	}

	// Get pricing data
	instancePricing := rdsc.getInstancePricing(config.Region, config.InstanceClass, config.Engine, config.MultiAZ)
	storagePricing := rdsc.getStoragePricing(config.Region, config.StorageType)
	iopsPricing := rdsc.getIOPSPricing(config.Region, config.StorageType)
	backupStoragePricing := rdsc.getBackupStoragePricing(config.Region)

	// Calculate costs
	instanceCost := instancePricing * config.HoursPerMonth
	storageCost := storagePricing * config.StorageGB
	iopsCost := 0.0
	if config.StorageType == "io1" && config.IOPS > 0 {
		iopsCost = iopsPricing * float64(config.IOPS)
	}
	backupStorageCost := backupStoragePricing * config.BackupStorageGB

	totalMonthly := instanceCost + storageCost + iopsCost + backupStorageCost

	return &RDSCostBreakdown{
		InstanceCost:      roundToTwoDecimals(instanceCost),
		StorageCost:       roundToTwoDecimals(storageCost),
		IOPSCost:          roundToTwoDecimals(iopsCost),
		BackupStorageCost: roundToTwoDecimals(backupStorageCost),
		TotalMonthly:      roundToTwoDecimals(totalMonthly),
		Currency:          "USD",
	}, nil
}

// CalculateWithRange calculates cost with min/max range
func (rdsc *RDSCostCalculator) CalculateWithRange(config RDSConfig) (*CostEstimateWithRange, error) {
	breakdown, err := rdsc.Calculate(config)
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

// validateConfig validates the RDS configuration
func (rdsc *RDSCostCalculator) validateConfig(config RDSConfig) error {
	if config.InstanceClass == "" {
		return fmt.Errorf("instance class is required")
	}
	if config.StorageGB < 20 {
		return fmt.Errorf("storage must be at least 20 GB")
	}
	if config.StorageGB > 65536 {
		return fmt.Errorf("storage cannot exceed 65536 GB")
	}
	if config.BackupStorageGB < 0 {
		return fmt.Errorf("backup storage cannot be negative")
	}
	if config.Region == "" {
		return fmt.Errorf("region is required")
	}
	return nil
}

// getInstancePricing returns instance pricing per hour
func (rdsc *RDSCostCalculator) getInstancePricing(region, instanceClass, engine string, multiAZ bool) float64 {
	// Try to get from cache
	pricingData, err := rdsc.pricingCache.GetSpecificPricing(
		region,
		"AmazonRDS",
		"Database Instance",
		instanceClass,
	)
	if err == nil {
		price := pricingData.PricePerHour
		if multiAZ {
			price *= 2 // Multi-AZ doubles the instance cost
		}
		return price
	}

	// Fallback to default pricing (PostgreSQL, Single-AZ)
	defaultPricing := map[string]map[string]float64{
		"us-east-1": {
			"db.t3.micro":  0.017,
			"db.t3.small":  0.034,
			"db.t3.medium": 0.068,
			"db.t3.large":  0.136,
			"db.m5.large":  0.192,
			"db.m5.xlarge": 0.384,
		},
		"us-west-2": {
			"db.t3.micro":  0.017,
			"db.t3.small":  0.034,
			"db.t3.medium": 0.068,
			"db.t3.large":  0.136,
			"db.m5.large":  0.192,
			"db.m5.xlarge": 0.384,
		},
		"eu-west-1": {
			"db.t3.micro":  0.018,
			"db.t3.small":  0.037,
			"db.t3.medium": 0.074,
			"db.t3.large":  0.148,
			"db.m5.large":  0.209,
			"db.m5.xlarge": 0.418,
		},
	}

	price := 0.017 // Default to db.t3.micro in us-east-1
	if regionPricing, exists := defaultPricing[region]; exists {
		if instancePrice, exists := regionPricing[instanceClass]; exists {
			price = instancePrice
		}
	}

	if multiAZ {
		price *= 2
	}

	return price
}

// getStoragePricing returns storage pricing per GB per month
func (rdsc *RDSCostCalculator) getStoragePricing(region, storageType string) float64 {
	// Default pricing per GB per month
	defaultPricing := map[string]map[string]float64{
		"gp2": {
			"us-east-1": 0.115,
			"us-west-2": 0.115,
			"eu-west-1": 0.127,
		},
		"gp3": {
			"us-east-1": 0.08,
			"us-west-2": 0.08,
			"eu-west-1": 0.088,
		},
		"io1": {
			"us-east-1": 0.125,
			"us-west-2": 0.125,
			"eu-west-1": 0.138,
		},
	}

	if typePricing, exists := defaultPricing[storageType]; exists {
		if price, exists := typePricing[region]; exists {
			return price
		}
	}

	// Default to gp3 pricing in us-east-1
	return 0.08
}

// getIOPSPricing returns IOPS pricing per IOPS per month
func (rdsc *RDSCostCalculator) getIOPSPricing(region, storageType string) float64 {
	if storageType != "io1" {
		return 0.0
	}

	// Default pricing per IOPS per month
	defaultPricing := map[string]float64{
		"us-east-1": 0.10,
		"us-west-2": 0.10,
		"eu-west-1": 0.11,
	}

	if price, exists := defaultPricing[region]; exists {
		return price
	}
	return 0.10
}

// getBackupStoragePricing returns backup storage pricing per GB per month
func (rdsc *RDSCostCalculator) getBackupStoragePricing(region string) float64 {
	// Backup storage beyond allocated storage is charged
	// Default pricing per GB per month
	defaultPricing := map[string]float64{
		"us-east-1": 0.095,
		"us-west-2": 0.095,
		"eu-west-1": 0.095,
	}

	if price, exists := defaultPricing[region]; exists {
		return price
	}
	return 0.095
}

// EstimateFromBlueprint estimates RDS costs from blueprint configuration
func (rdsc *RDSCostCalculator) EstimateFromBlueprint(blueprintConfig map[string]interface{}, region string) (*CostEstimateWithRange, error) {
	config := RDSConfig{
		InstanceClass:   getStringOrDefault(blueprintConfig, "instanceClass", "db.t3.micro"),
		Engine:          getStringOrDefault(blueprintConfig, "engine", "postgres"),
		StorageGB:       getFloat64OrDefault(blueprintConfig, "storageGb", 20),
		StorageType:     getStringOrDefault(blueprintConfig, "storageType", "gp3"),
		IOPS:            getIntOrDefault(blueprintConfig, "iops", 0),
		BackupStorageGB: getFloat64OrDefault(blueprintConfig, "backupStorageGb", 0),
		MultiAZ:         getBoolOrDefault(blueprintConfig, "multiAz", false),
		HoursPerMonth:   getFloat64OrDefault(blueprintConfig, "hoursPerMonth", 730),
		Region:          region,
	}

	return rdsc.CalculateWithRange(config)
}

// Helper function to get bool from map
func getBoolOrDefault(m map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultValue
}
