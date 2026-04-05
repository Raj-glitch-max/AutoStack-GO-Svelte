package cost

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// MicroservicesBlueprintCalculator calculates costs for microservices blueprint
// Services: Multiple Fargate services, ALB, RDS, S3, CloudFront, API Gateway, SQS, ElastiCache
type MicroservicesBlueprintCalculator struct {
	app            core.App
	fargateCalc    *FargateCostCalculator
	albCalc        *ALBCostCalculator
	rdsCalc        *RDSCostCalculator
	s3Calc         *S3CostCalculator
	cloudfrontCalc *CloudFrontCostCalculator
}

// NewMicroservicesBlueprintCalculator creates a new microservices blueprint calculator
func NewMicroservicesBlueprintCalculator(app core.App) *MicroservicesBlueprintCalculator {
	return &MicroservicesBlueprintCalculator{
		app:            app,
		fargateCalc:    NewFargateCostCalculator(app),
		albCalc:        NewALBCostCalculator(app),
		rdsCalc:        NewRDSCostCalculator(app),
		s3Calc:         NewS3CostCalculator(app),
		cloudfrontCalc: NewCloudFrontCostCalculator(app),
	}
}

// MicroservicesConfig represents configuration for microservices
type MicroservicesConfig struct {
	Region            string  `json:"region"`
	// Number of microservices
	ServiceCount      int     `json:"serviceCount"`      // Default: 3
	// Per-service Fargate configuration
	ServiceVCPU       float64 `json:"serviceVcpu"`       // Default: 0.5
	ServiceMemoryGB   float64 `json:"serviceMemoryGb"`   // Default: 1.0
	ServiceTasks      int     `json:"serviceTasks"`      // Default: 2
	// RDS configuration
	DBInstanceClass   string  `json:"dbInstanceClass"`   // Default: db.t3.medium
	DBStorageGB       int     `json:"dbStorageGb"`       // Default: 100
	DBEngine          string  `json:"dbEngine"`          // Default: postgres
	// S3 configuration
	StorageGB         float64 `json:"storageGb"`         // Default: 50GB
	RequestsPerMonth  int64   `json:"requestsPerMonth"`  // Default: 100,000
	// CloudFront configuration
	DataTransferGB    float64 `json:"dataTransferGb"`    // Default: 300GB
	// ALB configuration
	LCUHours          float64 `json:"lcuHours"`          // Default: 2190 (3 LCUs * 730 hours)
	// Additional services
	NATDataGB         float64 `json:"natDataGb"`         // Default: 200GB
	CloudWatchLogsGB  float64 `json:"cloudwatchLogsGb"`  // Default: 20GB
	APIGatewayReqs    int64   `json:"apiGatewayReqs"`    // Default: 1,000,000
	SQSRequests       int64   `json:"sqsRequests"`       // Default: 1,000,000
	ElastiCacheNodes  int     `json:"elasticacheNodes"`  // Default: 1
}

// MicroservicesCostBreakdown represents cost breakdown for microservices
type MicroservicesCostBreakdown struct {
	Fargate         float64                `json:"fargate"`
	ALB             float64                `json:"alb"`
	RDS             float64                `json:"rds"`
	S3              float64                `json:"s3"`
	CloudFront      float64                `json:"cloudfront"`
	NATGateway      float64                `json:"natGateway"`
	CloudWatchLogs  float64                `json:"cloudwatchLogs"`
	APIGateway      float64                `json:"apiGateway"`
	SQS             float64                `json:"sqs"`
	ElastiCache     float64                `json:"elasticache"`
	ECR             float64                `json:"ecr"`
	TotalMonthly    float64                `json:"totalMonthly"`
	Currency        string                 `json:"currency"`
	Assumptions     map[string]string      `json:"assumptions"`
	ServiceDetails  map[string]interface{} `json:"serviceDetails"`
}

// Calculate computes the monthly cost for microservices blueprint
func (msc *MicroservicesBlueprintCalculator) Calculate(config MicroservicesConfig) (*MicroservicesCostBreakdown, error) {
	// Apply defaults
	if config.ServiceCount == 0 {
		config.ServiceCount = 3
	}
	if config.ServiceVCPU == 0 {
		config.ServiceVCPU = 0.5
	}
	if config.ServiceMemoryGB == 0 {
		config.ServiceMemoryGB = 1.0
	}
	if config.ServiceTasks == 0 {
		config.ServiceTasks = 2
	}
	if config.DBInstanceClass == "" {
		config.DBInstanceClass = "db.t3.medium"
	}
	if config.DBStorageGB == 0 {
		config.DBStorageGB = 100
	}
	if config.DBEngine == "" {
		config.DBEngine = "postgres"
	}
	if config.StorageGB == 0 {
		config.StorageGB = 50
	}
	if config.RequestsPerMonth == 0 {
		config.RequestsPerMonth = 100000
	}
	if config.DataTransferGB == 0 {
		config.DataTransferGB = 300
	}
	if config.LCUHours == 0 {
		config.LCUHours = 2190 // 3 LCUs for full month
	}
	if config.NATDataGB == 0 {
		config.NATDataGB = 200
	}
	if config.CloudWatchLogsGB == 0 {
		config.CloudWatchLogsGB = 20
	}
	if config.APIGatewayReqs == 0 {
		config.APIGatewayReqs = 1000000
	}
	if config.SQSRequests == 0 {
		config.SQSRequests = 1000000
	}
	if config.ElastiCacheNodes == 0 {
		config.ElastiCacheNodes = 1
	}

	// Calculate Fargate costs for all microservices
	totalFargateTasks := config.ServiceCount * config.ServiceTasks
	fargateConfig := FargateConfig{
		VCPU:         config.ServiceVCPU,
		MemoryGB:     config.ServiceMemoryGB,
		TaskCount:    totalFargateTasks,
		HoursPerDay:  24,
		DaysPerMonth: 30,
		Region:       config.Region,
	}
	fargateBreakdown, err := msc.fargateCalc.Calculate(fargateConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate Fargate costs: %w", err)
	}

	// Calculate ALB costs
	albConfig := ALBConfig{
		LCUHours: config.LCUHours,
		Region:   config.Region,
	}
	albBreakdown, err := msc.albCalc.Calculate(albConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate ALB costs: %w", err)
	}

	// Calculate RDS costs
	rdsConfig := RDSConfig{
		InstanceClass:    config.DBInstanceClass,
		Engine:           config.DBEngine,
		StorageGB:        config.DBStorageGB,
		IOPS:             0,
		BackupStorageGB:  config.DBStorageGB,
		MultiAZ:          false,
		Region:           config.Region,
	}
	rdsBreakdown, err := msc.rdsCalc.Calculate(rdsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate RDS costs: %w", err)
	}

	// Calculate S3 costs
	s3Config := S3Config{
		StorageGB:        config.StorageGB,
		RequestsPerMonth: config.RequestsPerMonth,
		DataTransferGB:   0, // CloudFront handles data transfer
		Region:           config.Region,
	}
	s3Breakdown, err := msc.s3Calc.Calculate(s3Config)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate S3 costs: %w", err)
	}

	// Calculate CloudFront costs
	cloudfrontConfig := CloudFrontConfig{
		DataTransferGB:   config.DataTransferGB,
		RequestsPerMonth: config.RequestsPerMonth,
		Region:           config.Region,
	}
	cloudfrontBreakdown, err := msc.cloudfrontCalc.Calculate(cloudfrontConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate CloudFront costs: %w", err)
	}

	// Calculate additional service costs
	natCost := msc.calculateNATGatewayCost(config.NATDataGB, config.Region)
	cloudwatchCost := msc.calculateCloudWatchLogsCost(config.CloudWatchLogsGB)
	apiGatewayCost := msc.calculateAPIGatewayCost(config.APIGatewayReqs)
	sqsCost := msc.calculateSQSCost(config.SQSRequests)
	elasticacheCost := msc.calculateElastiCacheCost(config.ElastiCacheNodes, config.Region)
	ecrCost := msc.calculateECRCost(float64(config.ServiceCount))

	// Calculate total
	totalMonthly := fargateBreakdown.TotalMonthly + albBreakdown.TotalMonthly + 
		rdsBreakdown.TotalMonthly + s3Breakdown.TotalMonthly + 
		cloudfrontBreakdown.TotalMonthly + natCost + cloudwatchCost + 
		apiGatewayCost + sqsCost + elasticacheCost + ecrCost

	// Build assumptions
	assumptions := map[string]string{
		"microservices":   fmt.Sprintf("%d services", config.ServiceCount),
		"perService":      fmt.Sprintf("%.2f vCPU, %.2f GB memory, %d tasks", config.ServiceVCPU, config.ServiceMemoryGB, config.ServiceTasks),
		"database":        fmt.Sprintf("%s, %d GB storage", config.DBInstanceClass, config.DBStorageGB),
		"storage":         fmt.Sprintf("%.0f GB S3", config.StorageGB),
		"cdn":             fmt.Sprintf("%.0f GB data transfer", config.DataTransferGB),
		"loadBalancer":    fmt.Sprintf("%.0f LCU-hours", config.LCUHours),
		"apiGateway":      fmt.Sprintf("%d requests/month", config.APIGatewayReqs),
		"messaging":       fmt.Sprintf("%d SQS requests/month", config.SQSRequests),
		"cache":           fmt.Sprintf("%d ElastiCache nodes", config.ElastiCacheNodes),
		"availability":    "Single AZ",
		"backupRetention": "7 days",
	}

	// Build service details
	serviceDetails := map[string]interface{}{
		"fargate":    fargateBreakdown,
		"alb":        albBreakdown,
		"rds":        rdsBreakdown,
		"s3":         s3Breakdown,
		"cloudfront": cloudfrontBreakdown,
	}

	return &MicroservicesCostBreakdown{
		Fargate:        fargateBreakdown.TotalMonthly,
		ALB:            albBreakdown.TotalMonthly,
		RDS:            rdsBreakdown.TotalMonthly,
		S3:             s3Breakdown.TotalMonthly,
		CloudFront:     cloudfrontBreakdown.TotalMonthly,
		NATGateway:     natCost,
		CloudWatchLogs: cloudwatchCost,
		APIGateway:     apiGatewayCost,
		SQS:            sqsCost,
		ElastiCache:    elasticacheCost,
		ECR:            ecrCost,
		TotalMonthly:   roundToTwoDecimals(totalMonthly),
		Currency:       "USD",
		Assumptions:    assumptions,
		ServiceDetails: serviceDetails,
	}, nil
}

// CalculateWithRange calculates cost with min/max range
func (msc *MicroservicesBlueprintCalculator) CalculateWithRange(config MicroservicesConfig) (*CostEstimateWithRange, error) {
	breakdown, err := msc.Calculate(config)
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
func (msc *MicroservicesBlueprintCalculator) EstimateFromBlueprint(blueprintConfig map[string]interface{}, region string) (*CostEstimateWithRange, error) {
	config := MicroservicesConfig{
		Region:            region,
		ServiceCount:      getIntOrDefault(blueprintConfig, "service_count", 3),
		ServiceVCPU:       getFloat64OrDefault(blueprintConfig, "service_vcpu", 0.5),
		ServiceMemoryGB:   getFloat64OrDefault(blueprintConfig, "service_memory", 1.0),
		ServiceTasks:      getIntOrDefault(blueprintConfig, "service_tasks", 2),
		DBInstanceClass:   getStringOrDefault(blueprintConfig, "db_instance_class", "db.t3.medium"),
		DBStorageGB:       getIntOrDefault(blueprintConfig, "db_storage_gb", 100),
		DBEngine:          getStringOrDefault(blueprintConfig, "db_engine", "postgres"),
		StorageGB:         getFloat64OrDefault(blueprintConfig, "storage_gb", 50),
		RequestsPerMonth:  int64(getIntOrDefault(blueprintConfig, "requests_per_month", 100000)),
		DataTransferGB:    getFloat64OrDefault(blueprintConfig, "data_transfer_gb", 300),
		LCUHours:          getFloat64OrDefault(blueprintConfig, "lcu_hours", 2190),
		NATDataGB:         getFloat64OrDefault(blueprintConfig, "nat_data_gb", 200),
		CloudWatchLogsGB:  getFloat64OrDefault(blueprintConfig, "cloudwatch_logs_gb", 20),
		APIGatewayReqs:    int64(getIntOrDefault(blueprintConfig, "api_gateway_reqs", 1000000)),
		SQSRequests:       int64(getIntOrDefault(blueprintConfig, "sqs_requests", 1000000)),
		ElastiCacheNodes:  getIntOrDefault(blueprintConfig, "elasticache_nodes", 1),
	}

	return msc.CalculateWithRange(config)
}

// calculateNATGatewayCost calculates NAT Gateway costs
func (msc *MicroservicesBlueprintCalculator) calculateNATGatewayCost(dataGB float64, region string) float64 {
	hourlyRate := 0.045
	dataRate := 0.045
	hoursPerMonth := 730.0
	
	hourlyCost := hourlyRate * hoursPerMonth
	dataCost := dataRate * dataGB
	
	return roundToTwoDecimals(hourlyCost + dataCost)
}

// calculateCloudWatchLogsCost calculates CloudWatch Logs costs
func (msc *MicroservicesBlueprintCalculator) calculateCloudWatchLogsCost(logsGB float64) float64 {
	ingestionRate := 0.50
	storageRate := 0.03
	
	ingestionCost := ingestionRate * logsGB
	storageCost := storageRate * logsGB
	
	return roundToTwoDecimals(ingestionCost + storageCost)
}

// calculateAPIGatewayCost calculates API Gateway costs
func (msc *MicroservicesBlueprintCalculator) calculateAPIGatewayCost(requests int64) float64 {
	// API Gateway pricing (as of 2024)
	// First 333 million requests: $3.50 per million
	// Next 667 million requests: $2.80 per million
	// Over 1 billion requests: $2.38 per million
	
	requestsInMillions := float64(requests) / 1000000.0
	cost := 0.0
	
	if requestsInMillions <= 333 {
		cost = requestsInMillions * 3.50
	} else if requestsInMillions <= 1000 {
		cost = 333*3.50 + (requestsInMillions-333)*2.80
	} else {
		cost = 333*3.50 + 667*2.80 + (requestsInMillions-1000)*2.38
	}
	
	return roundToTwoDecimals(cost)
}

// calculateSQSCost calculates SQS costs
func (msc *MicroservicesBlueprintCalculator) calculateSQSCost(requests int64) float64 {
	// SQS pricing (as of 2024)
	// First 1 million requests per month: Free
	// After that: $0.40 per million requests
	
	if requests <= 1000000 {
		return 0.0
	}
	
	billableRequests := float64(requests-1000000) / 1000000.0
	cost := billableRequests * 0.40
	
	return roundToTwoDecimals(cost)
}

// calculateElastiCacheCost calculates ElastiCache costs
func (msc *MicroservicesBlueprintCalculator) calculateElastiCacheCost(nodes int, region string) float64 {
	// ElastiCache pricing for cache.t3.micro (as of 2024)
	hourlyRate := 0.017 // $0.017 per hour per node
	hoursPerMonth := 730.0
	
	monthlyCost := hourlyRate * hoursPerMonth * float64(nodes)
	
	return roundToTwoDecimals(monthlyCost)
}

// calculateECRCost calculates ECR costs
func (msc *MicroservicesBlueprintCalculator) calculateECRCost(storageGB float64) float64 {
	storageRate := 0.10
	return roundToTwoDecimals(storageRate * storageGB)
}

// GetDefaultAssumptions returns default usage assumptions for microservices
func (msc *MicroservicesBlueprintCalculator) GetDefaultAssumptions() map[string]string {
	return map[string]string{
		"microservices":   "3 services",
		"perService":      "0.5 vCPU, 1.0 GB memory, 2 tasks",
		"database":        "db.t3.medium, 100 GB storage",
		"storage":         "50 GB S3",
		"cdn":             "300 GB data transfer",
		"loadBalancer":    "2190 LCU-hours (3 LCUs constant)",
		"apiGateway":      "1,000,000 requests/month",
		"messaging":       "1,000,000 SQS requests/month",
		"cache":           "1 ElastiCache node (cache.t3.micro)",
		"availability":    "Single AZ",
		"backupRetention": "7 days",
	}
}

// GetDisclaimer returns cost disclaimer for microservices
func (msc *MicroservicesBlueprintCalculator) GetDisclaimer() string {
	return "Excludes: Data transfer out to internet beyond AWS free tier, " +
		"RDS snapshot storage beyond backup retention, CloudWatch detailed monitoring, " +
		"AWS Secrets Manager, VPC endpoints, additional ElastiCache nodes for HA, " +
		"Step Functions, EventBridge, Lambda functions, DynamoDB tables, " +
		"and any third-party integrations or monitoring tools."
}
