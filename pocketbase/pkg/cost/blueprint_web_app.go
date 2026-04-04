package cost

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// WebAppBlueprintCalculator calculates costs for web application blueprint
// Services: Fargate (vCPU, memory), ALB, RDS, NAT gateway, CloudWatch logs, ECR
type WebAppBlueprintCalculator struct {
	app            core.App
	fargateCalc    *FargateCostCalculator
	albCalc        *ALBCostCalculator
	rdsCalc        *RDSCostCalculator
}

// NewWebAppBlueprintCalculator creates a new web app blueprint calculator
func NewWebAppBlueprintCalculator(app core.App) *WebAppBlueprintCalculator {
	return &WebAppBlueprintCalculator{
		app:         app,
		fargateCalc: NewFargateCostCalculator(app),
		albCalc:     NewALBCostCalculator(app),
		rdsCalc:     NewRDSCostCalculator(app),
	}
}

// WebAppConfig represents configuration for web application
type WebAppConfig struct {
	Region           string  `json:"region"`
	// Fargate configuration
	VCPU             float64 `json:"vcpu"`             // Default: 0.25
	MemoryGB         float64 `json:"memoryGb"`         // Default: 0.5
	TaskCount        int     `json:"taskCount"`        // Default: 1
	// RDS configuration
	DBInstanceClass  string  `json:"dbInstanceClass"`  // Default: db.t3.micro
	DBStorageGB      int     `json:"dbStorageGb"`      // Default: 20
	DBEngine         string  `json:"dbEngine"`         // Default: postgres
	// ALB configuration
	LCUHours         float64 `json:"lcuHours"`         // Default: 730 (1 LCU * 730 hours)
	// Additional services
	NATDataGB        float64 `json:"natDataGb"`        // Default: 50GB
	CloudWatchLogsGB float64 `json:"cloudwatchLogsGb"` // Default: 5GB
}

// WebAppCostBreakdown represents cost breakdown for web application
type WebAppCostBreakdown struct {
	Fargate         float64                `json:"fargate"`
	ALB             float64                `json:"alb"`
	RDS             float64                `json:"rds"`
	NATGateway      float64                `json:"natGateway"`
	CloudWatchLogs  float64                `json:"cloudwatchLogs"`
	ECR             float64                `json:"ecr"`
	TotalMonthly    float64                `json:"totalMonthly"`
	Currency        string                 `json:"currency"`
	Assumptions     map[string]string      `json:"assumptions"`
	ServiceDetails  map[string]interface{} `json:"serviceDetails"`
}

// Calculate computes the monthly cost for web application blueprint
func (wac *WebAppBlueprintCalculator) Calculate(config WebAppConfig) (*WebAppCostBreakdown, error) {
	// Apply defaults
	if config.VCPU == 0 {
		config.VCPU = 0.25
	}
	if config.MemoryGB == 0 {
		config.MemoryGB = 0.5
	}
	if config.TaskCount == 0 {
		config.TaskCount = 1
	}
	if config.DBInstanceClass == "" {
		config.DBInstanceClass = "db.t3.micro"
	}
	if config.DBStorageGB == 0 {
		config.DBStorageGB = 20
	}
	if config.DBEngine == "" {
		config.DBEngine = "postgres"
	}
	if config.LCUHours == 0 {
		config.LCUHours = 730 // 1 LCU for full month
	}
	if config.NATDataGB == 0 {
		config.NATDataGB = 50
	}
	if config.CloudWatchLogsGB == 0 {
		config.CloudWatchLogsGB = 5
	}

	// Calculate Fargate costs
	fargateConfig := FargateConfig{
		VCPU:         config.VCPU,
		MemoryGB:     config.MemoryGB,
		TaskCount:    config.TaskCount,
		HoursPerDay:  24,
		DaysPerMonth: 30,
		Region:       config.Region,
	}
	fargateBreakdown, err := wac.fargateCalc.Calculate(fargateConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate Fargate costs: %w", err)
	}

	// Calculate ALB costs
	albConfig := ALBConfig{
		LCUHours: config.LCUHours,
		Region:   config.Region,
	}
	albBreakdown, err := wac.albCalc.Calculate(albConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate ALB costs: %w", err)
	}

	// Calculate RDS costs
	rdsConfig := RDSConfig{
		InstanceClass:    config.DBInstanceClass,
		Engine:           config.DBEngine,
		StorageGB:        config.DBStorageGB,
		IOPS:             0, // General Purpose SSD
		BackupStorageGB:  config.DBStorageGB, // Assume same as storage
		MultiAZ:          false,
		Region:           config.Region,
	}
	rdsBreakdown, err := wac.rdsCalc.Calculate(rdsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate RDS costs: %w", err)
	}

	// Calculate NAT Gateway costs
	natCost := wac.calculateNATGatewayCost(config.NATDataGB, config.Region)

	// Calculate CloudWatch Logs costs
	cloudwatchCost := wac.calculateCloudWatchLogsCost(config.CloudWatchLogsGB)

	// Calculate ECR costs (minimal for small images)
	ecrCost := wac.calculateECRCost()

	// Calculate total
	totalMonthly := fargateBreakdown.TotalMonthly + albBreakdown.TotalMonthly + 
		rdsBreakdown.TotalMonthly + natCost + cloudwatchCost + ecrCost

	// Build assumptions
	assumptions := map[string]string{
		"fargate":         fmt.Sprintf("%.2f vCPU, %.2f GB memory, %d tasks", config.VCPU, config.MemoryGB, config.TaskCount),
		"database":        fmt.Sprintf("%s, %d GB storage", config.DBInstanceClass, config.DBStorageGB),
		"loadBalancer":    fmt.Sprintf("%.0f LCU-hours", config.LCUHours),
		"natGateway":      fmt.Sprintf("%.0f GB data processed", config.NATDataGB),
		"cloudwatchLogs":  fmt.Sprintf("%.0f GB logs", config.CloudWatchLogsGB),
		"availability":    "Single AZ",
		"backupRetention": "7 days",
	}

	// Build service details
	serviceDetails := map[string]interface{}{
		"fargate":   fargateBreakdown,
		"alb":       albBreakdown,
		"rds":       rdsBreakdown,
	}

	return &WebAppCostBreakdown{
		Fargate:        fargateBreakdown.TotalMonthly,
		ALB:            albBreakdown.TotalMonthly,
		RDS:            rdsBreakdown.TotalMonthly,
		NATGateway:     natCost,
		CloudWatchLogs: cloudwatchCost,
		ECR:            ecrCost,
		TotalMonthly:   roundToTwoDecimals(totalMonthly),
		Currency:       "USD",
		Assumptions:    assumptions,
		ServiceDetails: serviceDetails,
	}, nil
}

// CalculateWithRange calculates cost with min/max range
func (wac *WebAppBlueprintCalculator) CalculateWithRange(config WebAppConfig) (*CostEstimateWithRange, error) {
	breakdown, err := wac.Calculate(config)
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
func (wac *WebAppBlueprintCalculator) EstimateFromBlueprint(blueprintConfig map[string]interface{}, region string) (*CostEstimateWithRange, error) {
	config := WebAppConfig{
		Region:           region,
		VCPU:             getFloat64OrDefault(blueprintConfig, "vcpu", 0.25),
		MemoryGB:         getFloat64OrDefault(blueprintConfig, "memory", 0.5),
		TaskCount:        getIntOrDefault(blueprintConfig, "task_count", 1),
		DBInstanceClass:  getStringOrDefault(blueprintConfig, "db_instance_class", "db.t3.micro"),
		DBStorageGB:      getIntOrDefault(blueprintConfig, "db_storage_gb", 20),
		DBEngine:         getStringOrDefault(blueprintConfig, "db_engine", "postgres"),
		LCUHours:         getFloat64OrDefault(blueprintConfig, "lcu_hours", 730),
		NATDataGB:        getFloat64OrDefault(blueprintConfig, "nat_data_gb", 50),
		CloudWatchLogsGB: getFloat64OrDefault(blueprintConfig, "cloudwatch_logs_gb", 5),
	}

	return wac.CalculateWithRange(config)
}

// calculateNATGatewayCost calculates NAT Gateway costs
func (wac *WebAppBlueprintCalculator) calculateNATGatewayCost(dataGB float64, region string) float64 {
	// NAT Gateway pricing (as of 2024)
	hourlyRate := 0.045 // $0.045 per hour
	dataRate := 0.045   // $0.045 per GB processed

	// Regional variations
	regionalMultiplier := map[string]float64{
		"us-east-1":      1.0,
		"us-east-2":      1.0,
		"us-west-1":      1.0,
		"us-west-2":      1.0,
		"eu-west-1":      1.0,
		"eu-west-2":      1.0,
		"eu-central-1":   1.0,
		"ap-southeast-1": 1.0,
		"ap-southeast-2": 1.0,
		"ap-northeast-1": 1.0,
	}

	multiplier := 1.0
	if m, exists := regionalMultiplier[region]; exists {
		multiplier = m
	}

	// Calculate monthly cost
	hoursPerMonth := 730.0
	hourlyCost := hourlyRate * hoursPerMonth * multiplier
	dataCost := dataRate * dataGB * multiplier

	return roundToTwoDecimals(hourlyCost + dataCost)
}

// calculateCloudWatchLogsCost calculates CloudWatch Logs costs
func (wac *WebAppBlueprintCalculator) calculateCloudWatchLogsCost(logsGB float64) float64 {
	// CloudWatch Logs pricing (as of 2024)
	ingestionRate := 0.50 // $0.50 per GB ingested
	storageRate := 0.03   // $0.03 per GB per month

	ingestionCost := ingestionRate * logsGB
	storageCost := storageRate * logsGB

	return roundToTwoDecimals(ingestionCost + storageCost)
}

// calculateECRCost calculates ECR (Elastic Container Registry) costs
func (wac *WebAppBlueprintCalculator) calculateECRCost() float64 {
	// ECR pricing (as of 2024)
	// Assume 1GB of container images stored
	storageRate := 0.10 // $0.10 per GB per month
	storageGB := 1.0

	return roundToTwoDecimals(storageRate * storageGB)
}

// GetDefaultAssumptions returns default usage assumptions for web application
func (wac *WebAppBlueprintCalculator) GetDefaultAssumptions() map[string]string {
	return map[string]string{
		"fargate":         "0.25 vCPU, 0.5 GB memory, 1 task",
		"database":        "db.t3.micro, 20 GB storage",
		"loadBalancer":    "730 LCU-hours (1 LCU constant)",
		"natGateway":      "50 GB data processed",
		"cloudwatchLogs":  "5 GB logs",
		"availability":    "Single AZ",
		"backupRetention": "7 days",
	}
}

// GetDisclaimer returns cost disclaimer for web application
func (wac *WebAppBlueprintCalculator) GetDisclaimer() string {
	return "Excludes: Data transfer out to internet beyond AWS free tier, " +
		"RDS snapshot storage beyond backup retention, CloudWatch detailed monitoring, " +
		"AWS Secrets Manager for credentials, VPC endpoints, and any third-party integrations."
}

// Helper function to get string from map
func getStringOrDefault(m map[string]interface{}, key string, defaultValue string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}
