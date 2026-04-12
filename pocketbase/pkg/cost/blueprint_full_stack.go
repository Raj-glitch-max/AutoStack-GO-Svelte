package cost

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// FullStackBlueprintCalculator calculates costs for full-stack application blueprint
type FullStackBlueprintCalculator struct {
	app            core.App
	fargateCalc    *FargateCostCalculator
	albCalc        *ALBCostCalculator
	rdsCalc        *RDSCostCalculator
	s3Calc         *S3CostCalculator
	cloudfrontCalc *CloudFrontCostCalculator
}

// NewFullStackBlueprintCalculator creates a new full-stack blueprint calculator
func NewFullStackBlueprintCalculator(app core.App) *FullStackBlueprintCalculator {
	return &FullStackBlueprintCalculator{
		app:            app,
		fargateCalc:    NewFargateCostCalculator(app),
		albCalc:        NewALBCostCalculator(app),
		rdsCalc:        NewRDSCostCalculator(app),
		s3Calc:         NewS3CostCalculator(app),
		cloudfrontCalc: NewCloudFrontCostCalculator(app),
	}
}

// FullStackConfig represents configuration for full-stack application
type FullStackConfig struct {
	Region            string
	FrontendVCPU      float64
	FrontendMemoryGB  float64
	FrontendTasks     int
	BackendVCPU       float64
	BackendMemoryGB   float64
	BackendTasks      int
	DBInstanceClass   string
	DBStorageGB       int
	DBEngine          string
	AssetStorageGB    float64
	AssetRequestsPM   int64
	CDNDataTransferGB float64
	LCUHours          float64
	NATDataGB         float64
	CloudWatchLogsGB  float64
}

// FullStackCostBreakdown represents cost breakdown for full-stack application
type FullStackCostBreakdown struct {
	FrontendFargate float64
	BackendFargate  float64
	ALB             float64
	RDS             float64
	S3Assets        float64
	CloudFront      float64
	NATGateway      float64
	CloudWatchLogs  float64
	ECR             float64
	TotalMonthly    float64
	Currency        string
	Assumptions     map[string]string
	ServiceDetails  map[string]interface{}
}

// Calculate computes the monthly cost for full-stack application blueprint
func (fsc *FullStackBlueprintCalculator) Calculate(config FullStackConfig) (*FullStackCostBreakdown, error) {
	if config.FrontendVCPU == 0 {
		config.FrontendVCPU = 0.25
	}
	if config.FrontendMemoryGB == 0 {
		config.FrontendMemoryGB = 0.5
	}
	if config.FrontendTasks == 0 {
		config.FrontendTasks = 1
	}
	if config.BackendVCPU == 0 {
		config.BackendVCPU = 0.5
	}
	if config.BackendMemoryGB == 0 {
		config.BackendMemoryGB = 1.0
	}
	if config.BackendTasks == 0 {
		config.BackendTasks = 2
	}
	if config.DBInstanceClass == "" {
		config.DBInstanceClass = "db.t3.small"
	}
	if config.DBStorageGB == 0 {
		config.DBStorageGB = 50
	}
	if config.DBEngine == "" {
		config.DBEngine = "postgres"
	}
	if config.AssetStorageGB == 0 {
		config.AssetStorageGB = 20
	}
	if config.AssetRequestsPM == 0 {
		config.AssetRequestsPM = 50000
	}
	if config.CDNDataTransferGB == 0 {
		config.CDNDataTransferGB = 200
	}
	if config.LCUHours == 0 {
		config.LCUHours = 1460
	}
	if config.NATDataGB == 0 {
		config.NATDataGB = 100
	}
	if config.CloudWatchLogsGB == 0 {
		config.CloudWatchLogsGB = 10
	}

	frontendFargateConfig := FargateConfig{
		VCPU:         config.FrontendVCPU,
		MemoryGB:     config.FrontendMemoryGB,
		TaskCount:    config.FrontendTasks,
		HoursPerDay:  24,
		DaysPerMonth: 30,
		Region:       config.Region,
	}
	frontendBreakdown, err := fsc.fargateCalc.Calculate(frontendFargateConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate frontend Fargate costs: %w", err)
	}

	backendFargateConfig := FargateConfig{
		VCPU:         config.BackendVCPU,
		MemoryGB:     config.BackendMemoryGB,
		TaskCount:    config.BackendTasks,
		HoursPerDay:  24,
		DaysPerMonth: 30,
		Region:       config.Region,
	}
	backendBreakdown, err := fsc.fargateCalc.Calculate(backendFargateConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate backend Fargate costs: %w", err)
	}

	albConfig := ALBConfig{
		HoursPerMonth: 730,
		Region:        config.Region,
	}
	albBreakdown, err := fsc.albCalc.Calculate(albConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate ALB costs: %w", err)
	}

	rdsConfig := RDSConfig{
		InstanceClass:   config.DBInstanceClass,
		Engine:          config.DBEngine,
		StorageGB:       float64(config.DBStorageGB),
		IOPS:            0,
		BackupStorageGB: float64(config.DBStorageGB),
		MultiAZ:         false,
		Region:          config.Region,
	}
	rdsBreakdown, err := fsc.rdsCalc.Calculate(rdsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate RDS costs: %w", err)
	}

	s3Config := S3Config{
		StorageGB:         config.AssetStorageGB,
		PUTRequests:       config.AssetRequestsPM / 10,
		GETRequests:       config.AssetRequestsPM,
		DataTransferOutGB: 0,
		Region:            config.Region,
	}
	s3Breakdown, err := fsc.s3Calc.Calculate(s3Config)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate S3 costs: %w", err)
	}

	cloudfrontConfig := CloudFrontConfig{
		DataTransferOutGB: config.CDNDataTransferGB,
		HTTPSRequests:     config.AssetRequestsPM,
		Region:            config.Region,
	}
	cloudfrontBreakdown, err := fsc.cloudfrontCalc.Calculate(cloudfrontConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate CloudFront costs: %w", err)
	}

	natCost := fsc.calculateNATGatewayCost(config.NATDataGB, config.Region)
	cloudwatchCost := fsc.calculateCloudWatchLogsCost(config.CloudWatchLogsGB)
	ecrCost := fsc.calculateECRCost(2.0)

	totalMonthly := frontendBreakdown.TotalMonthly + backendBreakdown.TotalMonthly +
		albBreakdown.TotalMonthly + rdsBreakdown.TotalMonthly +
		s3Breakdown.TotalMonthly + cloudfrontBreakdown.TotalMonthly +
		natCost + cloudwatchCost + ecrCost

	assumptions := map[string]string{
		"frontend":        fmt.Sprintf("%.2f vCPU, %.2f GB memory, %d tasks", config.FrontendVCPU, config.FrontendMemoryGB, config.FrontendTasks),
		"backend":         fmt.Sprintf("%.2f vCPU, %.2f GB memory, %d tasks", config.BackendVCPU, config.BackendMemoryGB, config.BackendTasks),
		"database":        fmt.Sprintf("%s, %d GB storage", config.DBInstanceClass, config.DBStorageGB),
		"assets":          fmt.Sprintf("%.0f GB S3 storage", config.AssetStorageGB),
		"cdn":             fmt.Sprintf("%.0f GB data transfer", config.CDNDataTransferGB),
		"loadBalancer":    fmt.Sprintf("%.0f LCU-hours", config.LCUHours),
		"natGateway":      fmt.Sprintf("%.0f GB data processed", config.NATDataGB),
		"cloudwatchLogs":  fmt.Sprintf("%.0f GB logs", config.CloudWatchLogsGB),
		"availability":    "Single AZ",
		"backupRetention": "7 days",
	}

	serviceDetails := map[string]interface{}{
		"frontendFargate": frontendBreakdown,
		"backendFargate":  backendBreakdown,
		"alb":             albBreakdown,
		"rds":             rdsBreakdown,
		"s3":              s3Breakdown,
		"cloudfront":      cloudfrontBreakdown,
	}

	return &FullStackCostBreakdown{
		FrontendFargate: frontendBreakdown.TotalMonthly,
		BackendFargate:  backendBreakdown.TotalMonthly,
		ALB:             albBreakdown.TotalMonthly,
		RDS:             rdsBreakdown.TotalMonthly,
		S3Assets:        s3Breakdown.TotalMonthly,
		CloudFront:      cloudfrontBreakdown.TotalMonthly,
		NATGateway:      natCost,
		CloudWatchLogs:  cloudwatchCost,
		ECR:             ecrCost,
		TotalMonthly:    roundToTwoDecimals(totalMonthly),
		Currency:        "USD",
		Assumptions:     assumptions,
		ServiceDetails:  serviceDetails,
	}, nil
}

func (fsc *FullStackBlueprintCalculator) CalculateWithRange(config FullStackConfig) (*CostEstimateWithRange, error) {
	breakdown, err := fsc.Calculate(config)
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

func (fsc *FullStackBlueprintCalculator) EstimateFromBlueprint(blueprintConfig map[string]interface{}, region string) (*CostEstimateWithRange, error) {
	config := FullStackConfig{
		Region:            region,
		FrontendVCPU:      getFloat64OrDefault(blueprintConfig, "frontend_vcpu", 0.25),
		FrontendMemoryGB:  getFloat64OrDefault(blueprintConfig, "frontend_memory", 0.5),
		FrontendTasks:     getIntOrDefault(blueprintConfig, "frontend_tasks", 1),
		BackendVCPU:       getFloat64OrDefault(blueprintConfig, "backend_vcpu", 0.5),
		BackendMemoryGB:   getFloat64OrDefault(blueprintConfig, "backend_memory", 1.0),
		BackendTasks:      getIntOrDefault(blueprintConfig, "backend_tasks", 2),
		DBInstanceClass:   getStringOrDefault(blueprintConfig, "db_instance_class", "db.t3.small"),
		DBStorageGB:       getIntOrDefault(blueprintConfig, "db_storage_gb", 50),
		DBEngine:          getStringOrDefault(blueprintConfig, "db_engine", "postgres"),
		AssetStorageGB:    getFloat64OrDefault(blueprintConfig, "asset_storage_gb", 20),
		AssetRequestsPM:   int64(getIntOrDefault(blueprintConfig, "asset_requests_pm", 50000)),
		CDNDataTransferGB: getFloat64OrDefault(blueprintConfig, "cdn_data_transfer_gb", 200),
		LCUHours:          getFloat64OrDefault(blueprintConfig, "lcu_hours", 1460),
		NATDataGB:         getFloat64OrDefault(blueprintConfig, "nat_data_gb", 100),
		CloudWatchLogsGB:  getFloat64OrDefault(blueprintConfig, "cloudwatch_logs_gb", 10),
	}
	return fsc.CalculateWithRange(config)
}

func (fsc *FullStackBlueprintCalculator) calculateNATGatewayCost(dataGB float64, region string) float64 {
	hourlyRate := 0.045
	dataRate := 0.045
	regionalMultiplier := map[string]float64{
		"us-east-1": 1.0, "us-east-2": 1.0, "us-west-1": 1.0, "us-west-2": 1.0,
		"eu-west-1": 1.0, "eu-west-2": 1.0, "eu-central-1": 1.0,
		"ap-southeast-1": 1.0, "ap-southeast-2": 1.0, "ap-northeast-1": 1.0,
	}
	multiplier := 1.0
	if m, exists := regionalMultiplier[region]; exists {
		multiplier = m
	}
	hoursPerMonth := 730.0
	hourlyCost := hourlyRate * hoursPerMonth * multiplier
	dataCost := dataRate * dataGB * multiplier
	return roundToTwoDecimals(hourlyCost + dataCost)
}

func (fsc *FullStackBlueprintCalculator) calculateCloudWatchLogsCost(logsGB float64) float64 {
	ingestionRate := 0.50
	storageRate := 0.03
	ingestionCost := ingestionRate * logsGB
	storageCost := storageRate * logsGB
	return roundToTwoDecimals(ingestionCost + storageCost)
}

func (fsc *FullStackBlueprintCalculator) calculateECRCost(storageGB float64) float64 {
	storageRate := 0.10
	return roundToTwoDecimals(storageRate * storageGB)
}

func (fsc *FullStackBlueprintCalculator) GetDefaultAssumptions() map[string]string {
	return map[string]string{
		"frontend":        "0.25 vCPU, 0.5 GB memory, 1 task",
		"backend":         "0.5 vCPU, 1.0 GB memory, 2 tasks",
		"database":        "db.t3.small, 50 GB storage",
		"assets":          "20 GB S3 storage",
		"cdn":             "200 GB data transfer",
		"loadBalancer":    "1460 LCU-hours (2 LCUs constant)",
		"natGateway":      "100 GB data processed",
		"cloudwatchLogs":  "10 GB logs",
		"availability":    "Single AZ",
		"backupRetention": "7 days",
	}
}

func (fsc *FullStackBlueprintCalculator) GetDisclaimer() string {
	return "Excludes: Data transfer out to internet beyond AWS free tier, " +
		"RDS snapshot storage beyond backup retention, CloudWatch detailed monitoring, " +
		"AWS Secrets Manager, VPC endpoints, ElastiCache if added, " +
		"SQS/SNS messaging services, and any third-party integrations."
}
