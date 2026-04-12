package validation

import (
	"fmt"
	"strings"
)

// ValidateCostEstimateRequest validates a cost estimation request
// Validates: AC-1.4 (Estimate is region-specific)
func ValidateCostEstimateRequest(blueprint, region string, variables map[string]interface{}) error {
	// Validate blueprint type
	if err := ValidateBlueprint(blueprint); err != nil {
		return fmt.Errorf("invalid blueprint: %w", err)
	}

	// Validate region
	if err := ValidateRegion(region); err != nil {
		return fmt.Errorf("invalid region: %w", err)
	}

	// Validate configuration variables based on blueprint type
	if err := ValidateBlueprintConfig(blueprint, variables); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	return nil
}

// ValidateBlueprint validates that the blueprint type is supported
func ValidateBlueprint(blueprint string) error {
	if blueprint == "" {
		return fmt.Errorf("blueprint type is required")
	}

	// Supported blueprint types
	supportedBlueprints := map[string]bool{
		"static-website":  true,
		"web-application": true,
		"full-stack-app":  true,
		"microservices":   true,
	}

	if !supportedBlueprints[blueprint] {
		return fmt.Errorf("unsupported blueprint type: %s (supported: static-website, web-application, full-stack-app, microservices)", blueprint)
	}

	return nil
}

// ValidateRegion validates that the region is a valid AWS region
// Validates: AC-1.4 (Estimate is region-specific)
func ValidateRegion(region string) error {
	if region == "" {
		return fmt.Errorf("region is required")
	}

	// Use existing AWS region validation
	_, err := SanitizeAWSRegion(region)
	if err != nil {
		return err
	}

	return nil
}

// ValidateBlueprintConfig validates configuration variables based on blueprint type
func ValidateBlueprintConfig(blueprint string, variables map[string]interface{}) error {
	if variables == nil {
		// Variables are optional, so nil is acceptable
		return nil
	}

	switch blueprint {
	case "static-website":
		return validateStaticWebsiteConfig(variables)
	case "web-application":
		return validateWebApplicationConfig(variables)
	case "full-stack-app":
		return validateFullStackConfig(variables)
	case "microservices":
		return validateMicroservicesConfig(variables)
	default:
		return fmt.Errorf("unknown blueprint type: %s", blueprint)
	}
}

// validateStaticWebsiteConfig validates static website configuration
func validateStaticWebsiteConfig(variables map[string]interface{}) error {
	// Validate storage_gb if provided
	if storageGB, ok := variables["storage_gb"]; ok {
		if err := validatePositiveNumber(storageGB, "storage_gb"); err != nil {
			return err
		}
	}

	// Validate requests_per_month if provided
	if requests, ok := variables["requests_per_month"]; ok {
		if err := validatePositiveNumber(requests, "requests_per_month"); err != nil {
			return err
		}
	}

	// Validate data_transfer_gb if provided
	if dataTransfer, ok := variables["data_transfer_gb"]; ok {
		if err := validatePositiveNumber(dataTransfer, "data_transfer_gb"); err != nil {
			return err
		}
	}

	// Validate has_custom_domain if provided
	if customDomain, ok := variables["has_custom_domain"]; ok {
		if err := validateBoolean(customDomain, "has_custom_domain"); err != nil {
			return err
		}
	}

	return nil
}

// validateWebApplicationConfig validates web application configuration
func validateWebApplicationConfig(variables map[string]interface{}) error {
	// Validate vcpu if provided
	if vcpu, ok := variables["vcpu"]; ok {
		if err := validateFargateVCPU(vcpu); err != nil {
			return err
		}
	}

	// Validate memory if provided
	if memory, ok := variables["memory"]; ok {
		if err := validatePositiveNumber(memory, "memory"); err != nil {
			return err
		}
	}

	// Validate task_count if provided
	if taskCount, ok := variables["task_count"]; ok {
		if err := validatePositiveInteger(taskCount, "task_count"); err != nil {
			return err
		}
	}

	// Validate db_instance_class if provided
	if dbClass, ok := variables["db_instance_class"]; ok {
		if err := validateRDSInstanceClass(dbClass); err != nil {
			return err
		}
	}

	// Validate db_storage_gb if provided
	if dbStorage, ok := variables["db_storage_gb"]; ok {
		if err := validatePositiveNumber(dbStorage, "db_storage_gb"); err != nil {
			return err
		}
	}

	// Validate db_engine if provided
	if dbEngine, ok := variables["db_engine"]; ok {
		if err := validateRDSEngine(dbEngine); err != nil {
			return err
		}
	}

	// Validate lcu_hours if provided
	if lcuHours, ok := variables["lcu_hours"]; ok {
		if err := validatePositiveNumber(lcuHours, "lcu_hours"); err != nil {
			return err
		}
	}

	// Validate nat_data_gb if provided
	if natData, ok := variables["nat_data_gb"]; ok {
		if err := validatePositiveNumber(natData, "nat_data_gb"); err != nil {
			return err
		}
	}

	// Validate cloudwatch_logs_gb if provided
	if cwLogs, ok := variables["cloudwatch_logs_gb"]; ok {
		if err := validatePositiveNumber(cwLogs, "cloudwatch_logs_gb"); err != nil {
			return err
		}
	}

	return nil
}

// validateFullStackConfig validates full-stack application configuration
func validateFullStackConfig(variables map[string]interface{}) error {
	// Validate frontend configuration
	if frontendStorage, ok := variables["frontend_storage_gb"]; ok {
		if err := validatePositiveNumber(frontendStorage, "frontend_storage_gb"); err != nil {
			return err
		}
	}

	if frontendRequests, ok := variables["frontend_requests"]; ok {
		if err := validatePositiveNumber(frontendRequests, "frontend_requests"); err != nil {
			return err
		}
	}

	if frontendTransfer, ok := variables["frontend_data_transfer"]; ok {
		if err := validatePositiveNumber(frontendTransfer, "frontend_data_transfer"); err != nil {
			return err
		}
	}

	// Validate backend configuration (reuse web app validation)
	backendVars := make(map[string]interface{})
	if backendVCPU, ok := variables["backend_vcpu"]; ok {
		backendVars["vcpu"] = backendVCPU
	}
	if backendMemory, ok := variables["backend_memory"]; ok {
		backendVars["memory"] = backendMemory
	}
	if backendTasks, ok := variables["backend_task_count"]; ok {
		backendVars["task_count"] = backendTasks
	}

	if len(backendVars) > 0 {
		if err := validateWebApplicationConfig(backendVars); err != nil {
			return fmt.Errorf("backend validation failed: %w", err)
		}
	}

	// Validate database configuration
	if dbClass, ok := variables["db_instance_class"]; ok {
		if err := validateRDSInstanceClass(dbClass); err != nil {
			return err
		}
	}

	if dbStorage, ok := variables["db_storage_gb"]; ok {
		if err := validatePositiveNumber(dbStorage, "db_storage_gb"); err != nil {
			return err
		}
	}

	if dbEngine, ok := variables["db_engine"]; ok {
		if err := validateRDSEngine(dbEngine); err != nil {
			return err
		}
	}

	// Validate other services
	if lcuHours, ok := variables["lcu_hours"]; ok {
		if err := validatePositiveNumber(lcuHours, "lcu_hours"); err != nil {
			return err
		}
	}

	if natData, ok := variables["nat_data_gb"]; ok {
		if err := validatePositiveNumber(natData, "nat_data_gb"); err != nil {
			return err
		}
	}

	if cwLogs, ok := variables["cloudwatch_logs_gb"]; ok {
		if err := validatePositiveNumber(cwLogs, "cloudwatch_logs_gb"); err != nil {
			return err
		}
	}

	if customDomain, ok := variables["has_custom_domain"]; ok {
		if err := validateBoolean(customDomain, "has_custom_domain"); err != nil {
			return err
		}
	}

	return nil
}

// validateMicroservicesConfig validates microservices configuration
func validateMicroservicesConfig(variables map[string]interface{}) error {
	// Validate service_count if provided
	if serviceCount, ok := variables["service_count"]; ok {
		if err := validatePositiveInteger(serviceCount, "service_count"); err != nil {
			return err
		}
	}

	// Validate service_vcpu if provided
	if serviceVCPU, ok := variables["service_vcpu"]; ok {
		if err := validateFargateVCPU(serviceVCPU); err != nil {
			return err
		}
	}

	// Validate service_memory if provided
	if serviceMemory, ok := variables["service_memory"]; ok {
		if err := validatePositiveNumber(serviceMemory, "service_memory"); err != nil {
			return err
		}
	}

	// Validate service_tasks if provided
	if serviceTasks, ok := variables["service_tasks"]; ok {
		if err := validatePositiveInteger(serviceTasks, "service_tasks"); err != nil {
			return err
		}
	}

	// Validate database configuration
	if dbClass, ok := variables["db_instance_class"]; ok {
		if err := validateRDSInstanceClass(dbClass); err != nil {
			return err
		}
	}

	if dbStorage, ok := variables["db_storage_gb"]; ok {
		if err := validatePositiveNumber(dbStorage, "db_storage_gb"); err != nil {
			return err
		}
	}

	if dbEngine, ok := variables["db_engine"]; ok {
		if err := validateRDSEngine(dbEngine); err != nil {
			return err
		}
	}

	// Validate storage_gb if provided
	if storageGB, ok := variables["storage_gb"]; ok {
		if err := validatePositiveNumber(storageGB, "storage_gb"); err != nil {
			return err
		}
	}

	// Validate requests_per_month if provided
	if requests, ok := variables["requests_per_month"]; ok {
		if err := validatePositiveNumber(requests, "requests_per_month"); err != nil {
			return err
		}
	}

	// Validate data_transfer_gb if provided
	if dataTransfer, ok := variables["data_transfer_gb"]; ok {
		if err := validatePositiveNumber(dataTransfer, "data_transfer_gb"); err != nil {
			return err
		}
	}

	// Validate lcu_hours if provided
	if lcuHours, ok := variables["lcu_hours"]; ok {
		if err := validatePositiveNumber(lcuHours, "lcu_hours"); err != nil {
			return err
		}
	}

	// Validate nat_data_gb if provided
	if natData, ok := variables["nat_data_gb"]; ok {
		if err := validatePositiveNumber(natData, "nat_data_gb"); err != nil {
			return err
		}
	}

	// Validate cloudwatch_logs_gb if provided
	if cwLogs, ok := variables["cloudwatch_logs_gb"]; ok {
		if err := validatePositiveNumber(cwLogs, "cloudwatch_logs_gb"); err != nil {
			return err
		}
	}

	// Validate api_gateway_reqs if provided
	if apiGatewayReqs, ok := variables["api_gateway_reqs"]; ok {
		if err := validatePositiveNumber(apiGatewayReqs, "api_gateway_reqs"); err != nil {
			return err
		}
	}

	// Validate sqs_requests if provided
	if sqsRequests, ok := variables["sqs_requests"]; ok {
		if err := validatePositiveNumber(sqsRequests, "sqs_requests"); err != nil {
			return err
		}
	}

	// Validate elasticache_nodes if provided
	if elasticacheNodes, ok := variables["elasticache_nodes"]; ok {
		if err := validatePositiveInteger(elasticacheNodes, "elasticache_nodes"); err != nil {
			return err
		}
	}

	return nil
}

// Helper validation functions

// validatePositiveNumber validates that a value is a positive number
func validatePositiveNumber(value interface{}, fieldName string) error {
	var num float64

	switch v := value.(type) {
	case float64:
		num = v
	case float32:
		num = float64(v)
	case int:
		num = float64(v)
	case int64:
		num = float64(v)
	case int32:
		num = float64(v)
	default:
		return fmt.Errorf("%s must be a number", fieldName)
	}

	if num < 0 {
		return fmt.Errorf("%s cannot be negative", fieldName)
	}

	return nil
}

// validatePositiveInteger validates that a value is a positive integer
func validatePositiveInteger(value interface{}, fieldName string) error {
	var num int

	switch v := value.(type) {
	case int:
		num = v
	case int64:
		num = int(v)
	case int32:
		num = int(v)
	case float64:
		// Allow float64 if it's a whole number
		if v != float64(int(v)) {
			return fmt.Errorf("%s must be an integer", fieldName)
		}
		num = int(v)
	case float32:
		// Allow float32 if it's a whole number
		if v != float32(int(v)) {
			return fmt.Errorf("%s must be an integer", fieldName)
		}
		num = int(v)
	default:
		return fmt.Errorf("%s must be an integer", fieldName)
	}

	if num < 1 {
		return fmt.Errorf("%s must be at least 1", fieldName)
	}

	return nil
}

// validateBoolean validates that a value is a boolean
func validateBoolean(value interface{}, fieldName string) error {
	switch value.(type) {
	case bool:
		return nil
	default:
		return fmt.Errorf("%s must be a boolean", fieldName)
	}
}

// validateFargateVCPU validates Fargate vCPU values
func validateFargateVCPU(value interface{}) error {
	var vcpu float64

	switch v := value.(type) {
	case float64:
		vcpu = v
	case float32:
		vcpu = float64(v)
	case int:
		vcpu = float64(v)
	case int64:
		vcpu = float64(v)
	case int32:
		vcpu = float64(v)
	default:
		return fmt.Errorf("vcpu must be a number")
	}

	// Valid Fargate vCPU values
	validVCPU := []float64{0.25, 0.5, 1, 2, 4, 8, 16}
	for _, valid := range validVCPU {
		if vcpu == valid {
			return nil
		}
	}

	return fmt.Errorf("invalid vcpu value: %v (must be one of: 0.25, 0.5, 1, 2, 4, 8, 16)", vcpu)
}

// validateRDSInstanceClass validates RDS instance class names
func validateRDSInstanceClass(value interface{}) error {
	instanceClass, ok := value.(string)
	if !ok {
		return fmt.Errorf("db_instance_class must be a string")
	}

	if instanceClass == "" {
		return fmt.Errorf("db_instance_class cannot be empty")
	}

	// Validate format: db.<instance-family>.<instance-size>
	// Examples: db.t3.micro, db.m5.large, db.r5.xlarge
	if !strings.HasPrefix(instanceClass, "db.") {
		return fmt.Errorf("db_instance_class must start with 'db.'")
	}

	parts := strings.Split(instanceClass, ".")
	if len(parts) != 3 {
		return fmt.Errorf("db_instance_class must be in format: db.<family>.<size>")
	}

	// Validate instance family (common families)
	validFamilies := map[string]bool{
		"t2": true, "t3": true, "t4g": true,
		"m5": true, "m6g": true, "m6i": true,
		"r5": true, "r6g": true, "r6i": true,
		"x2g": true, "x2iedn": true,
	}

	if !validFamilies[parts[1]] {
		return fmt.Errorf("unknown db instance family: %s (common families: t3, m5, r5)", parts[1])
	}

	// Validate instance size (common sizes)
	validSizes := map[string]bool{
		"micro": true, "small": true, "medium": true, "large": true,
		"xlarge": true, "2xlarge": true, "4xlarge": true, "8xlarge": true,
		"12xlarge": true, "16xlarge": true, "24xlarge": true,
	}

	if !validSizes[parts[2]] {
		return fmt.Errorf("unknown db instance size: %s", parts[2])
	}

	return nil
}

// validateRDSEngine validates RDS database engine names
func validateRDSEngine(value interface{}) error {
	engine, ok := value.(string)
	if !ok {
		return fmt.Errorf("db_engine must be a string")
	}

	if engine == "" {
		return fmt.Errorf("db_engine cannot be empty")
	}

	// Valid RDS engines
	validEngines := map[string]bool{
		"postgres":   true,
		"mysql":      true,
		"mariadb":    true,
		"oracle-se2": true,
		"oracle-ee":  true,
		"sqlserver-ex": true,
		"sqlserver-web": true,
		"sqlserver-se": true,
		"sqlserver-ee": true,
		"aurora-mysql": true,
		"aurora-postgresql": true,
	}

	if !validEngines[engine] {
		return fmt.Errorf("unsupported db_engine: %s (supported: postgres, mysql, mariadb, oracle-se2, sqlserver-se, aurora-postgresql, etc.)", engine)
	}

	return nil
}
