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
           app,
   NewFargateCostCalculator(app),
       NewALBCostCalculator(app),
       NewRDSCostCalculator(app),
        NewS3CostCalculator(app),
tCalc: NewCloudFrontCostCalculator(app),
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
fig.FrontendVCPU = 0.25
}
if config.FrontendMemoryGB == 0 {
fig.FrontendMemoryGB = 0.5
}
if config.FrontendTasks == 0 {
fig.FrontendTasks = 1
}
if config.BackendVCPU == 0 {
fig.BackendVCPU = 0.5
}
if config.BackendMemoryGB == 0 {
fig.BackendMemoryGB = 1.0
}
if config.BackendTasks == 0 {
fig.BackendTasks = 2
}
if config.DBInstanceClass == "" {
fig.DBInstanceClass = "db.t3.small"
}
if config.DBStorageGB == 0 {
fig.DBStorageGB = 50
}
if config.DBEngine == "" {
fig.DBEngine = "postgres"
}
if config.AssetStorageGB == 0 {
fig.AssetStorageGB = 20
}
if config.AssetRequestsPM == 0 {
fig.AssetRequestsPM = 50000
}
if config.CDNDataTransferGB == 0 {
fig.CDNDataTransferGB = 200
}
if config.LCUHours == 0 {
fig.LCUHours = 1460
}
if config.NATDataGB == 0 {
fig.NATDataGB = 100
}
if config.CloudWatchLogsGB == 0 {
fig.CloudWatchLogsGB = 10
}

frontendFargateConfig := FargateConfig{
        config.FrontendVCPU,
GB:     config.FrontendMemoryGB,
t:    config.FrontendTasks,
:  24,
sPerMonth: 30,
:       config.Region,
}
frontendBreakdown, err := fsc.fargateCalc.Calculate(frontendFargateConfig)
if err != nil {
 nil, fmt.Errorf("failed to calculate frontend Fargate costs: %w", err)
}

backendFargateConfig := FargateConfig{
        config.BackendVCPU,
GB:     config.BackendMemoryGB,
t:    config.BackendTasks,
:  24,
sPerMonth: 30,
:       config.Region,
}
backendBreakdown, err := fsc.fargateCalc.Calculate(backendFargateConfig)
if err != nil {
 nil, fmt.Errorf("failed to calculate backend Fargate costs: %w", err)
}

albConfig := ALBConfig{
config.LCUHours,
:   config.Region,
}
albBreakdown, err := fsc.albCalc.Calculate(albConfig)
if err != nil {
 nil, fmt.Errorf("failed to calculate ALB costs: %w", err)
}

rdsConfig := RDSConfig{
stanceClass:   config.DBInstanceClass,
gine:          config.DBEngine,
      config.DBStorageGB,
           0,
config.DBStorageGB,
        false,
:          config.Region,
}
rdsBreakdown, err := fsc.rdsCalc.Calculate(rdsConfig)
if err != nil {
 nil, fmt.Errorf("failed to calculate RDS costs: %w", err)
}

s3Config := S3Config{
       config.AssetStorageGB,
uestsPerMonth: config.AssetRequestsPM,
sferGB:   0,
:           config.Region,
}
s3Breakdown, err := fsc.s3Calc.Calculate(s3Config)
if err != nil {
 nil, fmt.Errorf("failed to calculate S3 costs: %w", err)
}

cloudfrontConfig := CloudFrontConfig{
sferGB:   config.CDNDataTransferGB,
uestsPerMonth: config.AssetRequestsPM,
:           config.Region,
}
cloudfrontBreakdown, err := fsc.cloudfrontCalc.Calculate(cloudfrontConfig)
if err != nil {
 nil, fmt.Errorf("failed to calculate CloudFront costs: %w", err)
}

natCost := fsc.calculateNATGatewayCost(config.NATDataGB, config.Region)
cloudwatchCost := fsc.calculateCloudWatchLogsCost(config.CloudWatchLogsGB)
ecrCost := fsc.calculateECRCost(2.0)

totalMonthly := frontendBreakdown.TotalMonthly + backendBreakdown.TotalMonthly +
.TotalMonthly + rdsBreakdown.TotalMonthly +
.TotalMonthly + cloudfrontBreakdown.TotalMonthly +
atCost + cloudwatchCost + ecrCost

assumptions := map[string]string{
tend":        fmt.Sprintf("%.2f vCPU, %.2f GB memory, %d tasks", config.FrontendVCPU, config.FrontendMemoryGB, config.FrontendTasks),
d":         fmt.Sprintf("%.2f vCPU, %.2f GB memory, %d tasks", config.BackendVCPU, config.BackendMemoryGB, config.BackendTasks),
       fmt.Sprintf("%s, %d GB storage", config.DBInstanceClass, config.DBStorageGB),
         fmt.Sprintf("%.0f GB S3 storage", config.AssetStorageGB),
":             fmt.Sprintf("%.0f GB data transfer", config.CDNDataTransferGB),
cer":    fmt.Sprintf("%.0f LCU-hours", config.LCUHours),
atGateway":      fmt.Sprintf("%.0f GB data processed", config.NATDataGB),
 fmt.Sprintf("%.0f GB logs", config.CloudWatchLogsGB),
":    "Single AZ",
tion": "7 days",
}

serviceDetails := map[string]interface{}{
tendFargate": frontendBreakdown,
dFargate":  backendBreakdown,
            albBreakdown,
            rdsBreakdown,
             s3Breakdown,
t":      cloudfrontBreakdown,
}

return &FullStackCostBreakdown{
tendFargate: frontendBreakdown.TotalMonthly,
dFargate:  backendBreakdown.TotalMonthly,
            albBreakdown.TotalMonthly,
            rdsBreakdown.TotalMonthly,
       s3Breakdown.TotalMonthly,
t:      cloudfrontBreakdown.TotalMonthly,
ATGateway:      natCost,
 cloudwatchCost,
            ecrCost,
thly:    roundToTwoDecimals(totalMonthly),
cy:        "USD",
s:     assumptions,
 serviceDetails,
}, nil
}

func (fsc *FullStackBlueprintCalculator) CalculateWithRange(config FullStackConfig) (*CostEstimateWithRange, error) {
breakdown, err := fsc.Calculate(config)
if err != nil {
 nil, err
}
return &CostEstimateWithRange{
 breakdown.TotalMonthly,
geMin:  roundToTwoDecimals(breakdown.TotalMonthly * 0.8),
geMax:  roundToTwoDecimals(breakdown.TotalMonthly * 1.4),
: breakdown,
cy:  "USD",
}, nil
}

func (fsc *FullStackBlueprintCalculator) EstimateFromBlueprint(blueprintConfig map[string]interface{}, region string) (*CostEstimateWithRange, error) {
config := FullStackConfig{
:            region,
tendVCPU:      getFloat64OrDefault(blueprintConfig, "frontend_vcpu", 0.25),
tendMemoryGB:  getFloat64OrDefault(blueprintConfig, "frontend_memory", 0.5),
tendTasks:     getIntOrDefault(blueprintConfig, "frontend_tasks", 1),
dVCPU:       getFloat64OrDefault(blueprintConfig, "backend_vcpu", 0.5),
dMemoryGB:   getFloat64OrDefault(blueprintConfig, "backend_memory", 1.0),
dTasks:      getIntOrDefault(blueprintConfig, "backend_tasks", 2),
stanceClass:   getStringOrDefault(blueprintConfig, "db_instance_class", "db.t3.small"),
      getIntOrDefault(blueprintConfig, "db_storage_gb", 50),
gine:          getStringOrDefault(blueprintConfig, "db_engine", "postgres"),
   getFloat64OrDefault(blueprintConfig, "asset_storage_gb", 20),
uestsPM:   int64(getIntOrDefault(blueprintConfig, "asset_requests_pm", 50000)),
DataTransferGB: getFloat64OrDefault(blueprintConfig, "cdn_data_transfer_gb", 200),
         getFloat64OrDefault(blueprintConfig, "lcu_hours", 1460),
ATDataGB:         getFloat64OrDefault(blueprintConfig, "nat_data_gb", 100),
 getFloat64OrDefault(blueprintConfig, "cloudwatch_logs_gb", 10),
}
return fsc.CalculateWithRange(config)
}

func (fsc *FullStackBlueprintCalculator) calculateNATGatewayCost(dataGB float64, region string) float64 {
hourlyRate := 0.045
dataRate := 0.045
regionalMultiplier := map[string]float64{
1.0, "us-east-2": 1.0, "us-west-1": 1.0, "us-west-2": 1.0,
1.0, "eu-west-2": 1.0, "eu-central-1": 1.0,
1.0, "ap-southeast-2": 1.0, "ap-northeast-1": 1.0,
}
multiplier := 1.0
if m, exists := regionalMultiplier[region]; exists {
= m
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
tend":        "0.25 vCPU, 0.5 GB memory, 1 task",
d":         "0.5 vCPU, 1.0 GB memory, 2 tasks",
       "db.t3.small, 50 GB storage",
         "20 GB S3 storage",
":             "200 GB data transfer",
cer":    "1460 LCU-hours (2 LCUs constant)",
atGateway":      "100 GB data processed",
 "10 GB logs",
":    "Single AZ",
tion": "7 days",
}
}

func (fsc *FullStackBlueprintCalculator) GetDisclaimer() string {
return "Excludes: Data transfer out to internet beyond AWS free tier, " +
snapshot storage beyond backup retention, CloudWatch detailed monitoring, " +
Secrets Manager, VPC endpoints, ElastiCache if added, " +
S/SNS messaging services, and any third-party integrations."
}
