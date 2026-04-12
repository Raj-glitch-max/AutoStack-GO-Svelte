package cost

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// BlueprintMapper maps blueprint types to their cost calculators
type BlueprintMapper struct {
	app                      core.App
	staticWebsiteCalc        *StaticWebsiteBlueprintCalculator
	webAppCalc               *WebAppBlueprintCalculator
	fullStackCalc            *FullStackBlueprintCalculator
	microservicesCalc        *MicroservicesBlueprintCalculator
}

// NewBlueprintMapper creates a new blueprint mapper
func NewBlueprintMapper(app core.App) *BlueprintMapper {
	return &BlueprintMapper{
		app:               app,
		staticWebsiteCalc: NewStaticWebsiteBlueprintCalculator(app),
		webAppCalc:        NewWebAppBlueprintCalculator(app),
		fullStackCalc:     NewFullStackBlueprintCalculator(app),
		microservicesCalc: NewMicroservicesBlueprintCalculator(app),
	}
}

// BlueprintType represents supported blueprint types
type BlueprintType string

const (
	BlueprintStaticWebsite BlueprintType = "static-website"
	BlueprintWebApp        BlueprintType = "web-application"
	BlueprintFullStack     BlueprintType = "full-stack-app"
	BlueprintMicroservices BlueprintType = "microservices"
)

// BlueprintCalculator is the interface all blueprint calculators must implement
type BlueprintCalculator interface {
	EstimateFromBlueprint(blueprintConfig map[string]interface{}, region string) (*CostEstimateWithRange, error)
	GetDefaultAssumptions() map[string]string
	GetDisclaimer() string
}

// EstimateCost estimates cost for a given blueprint type
func (bm *BlueprintMapper) EstimateCost(blueprintType string, blueprintConfig map[string]interface{}, region string) (*CostEstimateWithRange, error) {
	calculator, err := bm.getCalculator(blueprintType)
	if err != nil {
		return nil, err
	}

	// Merge default assumptions with provided config
	config := bm.mergeWithDefaults(blueprintType, blueprintConfig)

	return calculator.EstimateFromBlueprint(config, region)
}

// GetAssumptions returns usage assumptions for a blueprint type
func (bm *BlueprintMapper) GetAssumptions(blueprintType string) (map[string]string, error) {
	calculator, err := bm.getCalculator(blueprintType)
	if err != nil {
		return nil, err
	}

	return calculator.GetDefaultAssumptions(), nil
}

// GetDisclaimer returns cost disclaimer for a blueprint type
func (bm *BlueprintMapper) GetDisclaimer(blueprintType string) (string, error) {
	calculator, err := bm.getCalculator(blueprintType)
	if err != nil {
		return "", err
	}

	return calculator.GetDisclaimer(), nil
}

// GetSupportedBlueprints returns list of supported blueprint types
func (bm *BlueprintMapper) GetSupportedBlueprints() []string {
	return []string{
		string(BlueprintStaticWebsite),
		string(BlueprintWebApp),
		string(BlueprintFullStack),
		string(BlueprintMicroservices),
	}
}

// IsBlueprintSupported checks if a blueprint type is supported
func (bm *BlueprintMapper) IsBlueprintSupported(blueprintType string) bool {
	supported := bm.GetSupportedBlueprints()
	for _, bt := range supported {
		if bt == blueprintType {
			return true
		}
	}
	return false
}

// getCalculator returns the appropriate calculator for a blueprint type
func (bm *BlueprintMapper) getCalculator(blueprintType string) (BlueprintCalculator, error) {
	switch BlueprintType(blueprintType) {
	case BlueprintStaticWebsite:
		return bm.staticWebsiteCalc, nil
	case BlueprintWebApp:
		return bm.webAppCalc, nil
	case BlueprintFullStack:
		return bm.fullStackCalc, nil
	case BlueprintMicroservices:
		return bm.microservicesCalc, nil
	default:
		return nil, fmt.Errorf("unsupported blueprint type: %s", blueprintType)
	}
}

// mergeWithDefaults merges provided config with default assumptions
func (bm *BlueprintMapper) mergeWithDefaults(blueprintType string, config map[string]interface{}) map[string]interface{} {
	defaults := bm.getDefaultConfig(blueprintType)
	
	// Create merged config
	merged := make(map[string]interface{})
	
	// Start with defaults
	for k, v := range defaults {
		merged[k] = v
	}
	
	// Override with provided config
	for k, v := range config {
		merged[k] = v
	}
	
	return merged
}

// getDefaultConfig returns default configuration for a blueprint type
func (bm *BlueprintMapper) getDefaultConfig(blueprintType string) map[string]interface{} {
	switch BlueprintType(blueprintType) {
	case BlueprintStaticWebsite:
		return map[string]interface{}{
			"storage_gb":          10.0,
			"requests_per_month":  10000,
			"data_transfer_gb":    100.0,
			"has_custom_domain":   false,
		}
	case BlueprintWebApp:
		return map[string]interface{}{
			"vcpu":                0.25,
			"memory":              0.5,
			"task_count":          1,
			"db_instance_class":   "db.t3.micro",
			"db_storage_gb":       20,
			"db_engine":           "postgres",
			"lcu_hours":           730.0,
			"nat_data_gb":         50.0,
			"cloudwatch_logs_gb":  5.0,
		}
	case BlueprintFullStack:
		return map[string]interface{}{
			// Frontend (S3 + CloudFront)
			"frontend_storage_gb":      5.0,
			"frontend_requests":        50000,
			"frontend_data_transfer":   200.0,
			// Backend (Fargate)
			"backend_vcpu":             0.5,
			"backend_memory":           1.0,
			"backend_task_count":       2,
			// Database (RDS)
			"db_instance_class":        "db.t3.small",
			"db_storage_gb":            50,
			"db_engine":                "postgres",
			// Load Balancer
			"lcu_hours":                1460.0, // 2 LCUs
			// Additional services
			"nat_data_gb":              100.0,
			"cloudwatch_logs_gb":       10.0,
			"has_custom_domain":        true,
		}
	case BlueprintMicroservices:
		return map[string]interface{}{
			"service_count":        3,
			"service_vcpu":         0.5,
			"service_memory":       1.0,
			"service_tasks":        2,
			"db_instance_class":    "db.t3.medium",
			"db_storage_gb":        100,
			"db_engine":            "postgres",
			"storage_gb":           50.0,
			"requests_per_month":   100000,
			"data_transfer_gb":     300.0,
			"lcu_hours":            2190.0, // 3 LCUs
			"nat_data_gb":          200.0,
			"cloudwatch_logs_gb":   20.0,
			"api_gateway_reqs":     1000000,
			"sqs_requests":         1000000,
			"elasticache_nodes":    1,
		}
	default:
		return make(map[string]interface{})
	}
}

// GetBlueprintDescription returns a description of what services are included
func (bm *BlueprintMapper) GetBlueprintDescription(blueprintType string) (string, error) {
	switch BlueprintType(blueprintType) {
	case BlueprintStaticWebsite:
		return "Static website hosted on S3 with CloudFront CDN. " +
			"Includes: S3 storage, S3 requests, CloudFront data transfer, and optional Route53 hosted zone.", nil
	case BlueprintWebApp:
		return "Web application running on Fargate with RDS database. " +
			"Includes: Fargate compute (vCPU + memory), Application Load Balancer, RDS instance and storage, " +
			"NAT Gateway, CloudWatch Logs, and ECR container registry.", nil
	case BlueprintFullStack:
		return "Full-stack application with separate frontend and backend. " +
			"Includes: S3 + CloudFront for frontend, Fargate for backend, RDS database, " +
			"Application Load Balancer, Route53, NAT Gateway, CloudWatch Logs, and ECR.", nil
	case BlueprintMicroservices:
		return "Microservices architecture with multiple services. " +
			"Includes: Multiple Fargate services, Application Load Balancer, RDS database, " +
			"S3 storage, CloudFront CDN, API Gateway, SQS messaging, ElastiCache, " +
			"NAT Gateway, CloudWatch Logs, and ECR.", nil
	default:
		return "", fmt.Errorf("unsupported blueprint type: %s", blueprintType)
	}
}

// GetServiceBreakdown returns a list of services included in a blueprint
func (bm *BlueprintMapper) GetServiceBreakdown(blueprintType string) ([]string, error) {
	switch BlueprintType(blueprintType) {
	case BlueprintStaticWebsite:
		return []string{
			"Amazon S3 (storage)",
			"Amazon S3 (requests)",
			"Amazon CloudFront (data transfer)",
			"Amazon Route53 (optional)",
		}, nil
	case BlueprintWebApp:
		return []string{
			"AWS Fargate (vCPU)",
			"AWS Fargate (memory)",
			"Application Load Balancer",
			"Amazon RDS (instance)",
			"Amazon RDS (storage)",
			"NAT Gateway",
			"CloudWatch Logs",
			"Amazon ECR",
		}, nil
	case BlueprintFullStack:
		return []string{
			"Amazon S3 (frontend storage)",
			"Amazon CloudFront (frontend CDN)",
			"AWS Fargate (backend compute)",
			"Application Load Balancer",
			"Amazon RDS (database)",
			"Amazon Route53",
			"NAT Gateway",
			"CloudWatch Logs",
			"Amazon ECR",
		}, nil
	case BlueprintMicroservices:
		return []string{
			"AWS Fargate (multiple services)",
			"Application Load Balancer",
			"Amazon RDS (database)",
			"Amazon S3 (storage)",
			"Amazon CloudFront (CDN)",
			"API Gateway",
			"Amazon SQS (messaging)",
			"Amazon ElastiCache",
			"NAT Gateway",
			"CloudWatch Logs",
			"Amazon ECR",
		}, nil
	default:
		return nil, fmt.Errorf("unsupported blueprint type: %s", blueprintType)
	}
}

// ValidateConfig validates blueprint configuration
func (bm *BlueprintMapper) ValidateConfig(blueprintType string, config map[string]interface{}) error {
	if !bm.IsBlueprintSupported(blueprintType) {
		return fmt.Errorf("unsupported blueprint type: %s", blueprintType)
	}

	// Validate region is provided
	if _, ok := config["region"]; !ok {
		return fmt.Errorf("region is required in configuration")
	}

	// Blueprint-specific validation
	switch BlueprintType(blueprintType) {
	case BlueprintStaticWebsite:
		return bm.validateStaticWebsiteConfig(config)
	case BlueprintWebApp:
		return bm.validateWebAppConfig(config)
	case BlueprintFullStack:
		return bm.validateFullStackConfig(config)
	case BlueprintMicroservices:
		return bm.validateMicroservicesConfig(config)
	}

	return nil
}

// validateStaticWebsiteConfig validates static website configuration
func (bm *BlueprintMapper) validateStaticWebsiteConfig(config map[string]interface{}) error {
	if storageGB := getFloat64OrDefault(config, "storage_gb", 0); storageGB < 0 {
		return fmt.Errorf("storage_gb cannot be negative")
	}
	if requests := getIntOrDefault(config, "requests_per_month", 0); requests < 0 {
		return fmt.Errorf("requests_per_month cannot be negative")
	}
	return nil
}

// validateWebAppConfig validates web application configuration
func (bm *BlueprintMapper) validateWebAppConfig(config map[string]interface{}) error {
	vcpu := getFloat64OrDefault(config, "vcpu", 0.25)
	validVCPU := []float64{0.25, 0.5, 1, 2, 4, 8, 16}
	isValid := false
	for _, v := range validVCPU {
		if vcpu == v {
			isValid = true
			break
		}
	}
	if !isValid {
		return fmt.Errorf("invalid vcpu value: %f (must be one of: 0.25, 0.5, 1, 2, 4, 8, 16)", vcpu)
	}

	if taskCount := getIntOrDefault(config, "task_count", 1); taskCount < 1 {
		return fmt.Errorf("task_count must be at least 1")
	}

	return nil
}

// validateFullStackConfig validates full-stack configuration
func (bm *BlueprintMapper) validateFullStackConfig(config map[string]interface{}) error {
	// Validate backend configuration
	if err := bm.validateWebAppConfig(config); err != nil {
		return fmt.Errorf("backend validation failed: %w", err)
	}

	// Validate frontend configuration
	if storageGB := getFloat64OrDefault(config, "frontend_storage_gb", 0); storageGB < 0 {
		return fmt.Errorf("frontend_storage_gb cannot be negative")
	}

	return nil
}

// validateMicroservicesConfig validates microservices configuration
func (bm *BlueprintMapper) validateMicroservicesConfig(config map[string]interface{}) error {
	if serviceCount := getIntOrDefault(config, "service_count", 1); serviceCount < 1 {
		return fmt.Errorf("service_count must be at least 1")
	}

	if serviceTasks := getIntOrDefault(config, "service_tasks", 1); serviceTasks < 1 {
		return fmt.Errorf("service_tasks must be at least 1")
	}

	return nil
}
