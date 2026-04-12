package cost

import (
	"fmt"
)

// UsageAssumption represents a single usage assumption with its value and description
type UsageAssumption struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Unit        string `json:"unit,omitempty"`
}

// UsageAssumptions represents a collection of usage assumptions for a blueprint
type UsageAssumptions struct {
	BlueprintType string              `json:"blueprintType"`
	Assumptions   []UsageAssumption   `json:"assumptions"`
	AsMap         map[string]string   `json:"asMap"`
}

// UsageAssumptionsManager manages usage assumptions for all blueprint types
type UsageAssumptionsManager struct {
	assumptions map[string]*UsageAssumptions
}

// NewUsageAssumptionsManager creates a new usage assumptions manager
func NewUsageAssumptionsManager() *UsageAssumptionsManager {
	manager := &UsageAssumptionsManager{
		assumptions: make(map[string]*UsageAssumptions),
	}
	
	// Initialize default assumptions for all blueprint types
	manager.initializeDefaults()
	
	return manager
}

// initializeDefaults sets up default assumptions for all blueprint types
func (uam *UsageAssumptionsManager) initializeDefaults() {
	// Static Website assumptions
	uam.assumptions[string(BlueprintStaticWebsite)] = &UsageAssumptions{
		BlueprintType: string(BlueprintStaticWebsite),
		Assumptions: []UsageAssumption{
			{
				Key:         "storage",
				Value:       "10 GB",
				Description: "S3 storage for static assets",
				Unit:        "GB",
			},
			{
				Key:         "requests",
				Value:       "10,000 per month",
				Description: "HTTP/HTTPS requests to website",
				Unit:        "requests/month",
			},
			{
				Key:         "dataTransfer",
				Value:       "100 GB per month",
				Description: "Data transfer out via CloudFront",
				Unit:        "GB/month",
			},
			{
				Key:         "customDomain",
				Value:       "false",
				Description: "Route53 hosted zone for custom domain",
				Unit:        "",
			},
			{
				Key:         "cacheHitRatio",
				Value:       "80%",
				Description: "CloudFront cache hit ratio",
				Unit:        "%",
			},
			{
				Key:         "compressionRate",
				Value:       "60%",
				Description: "Content compression rate",
				Unit:        "%",
			},
		},
		AsMap: map[string]string{
			"storage":         "10 GB",
			"requests":        "10,000 per month",
			"dataTransfer":    "100 GB per month",
			"customDomain":    "false",
			"cacheHitRatio":   "80%",
			"compressionRate": "60%",
		},
	}

	// Web Application assumptions
	uam.assumptions[string(BlueprintWebApp)] = &UsageAssumptions{
		BlueprintType: string(BlueprintWebApp),
		Assumptions: []UsageAssumption{
			{
				Key:         "compute",
				Value:       "0.25 vCPU, 0.5 GB memory",
				Description: "Fargate task compute resources",
				Unit:        "",
			},
			{
				Key:         "taskCount",
				Value:       "1 task",
				Description: "Number of Fargate tasks running",
				Unit:        "tasks",
			},
			{
				Key:         "database",
				Value:       "db.t3.micro, 20 GB storage",
				Description: "RDS instance type and storage",
				Unit:        "",
			},
			{
				Key:         "loadBalancer",
				Value:       "730 LCU-hours",
				Description: "Application Load Balancer capacity units",
				Unit:        "LCU-hours",
			},
			{
				Key:         "natGateway",
				Value:       "50 GB data processed",
				Description: "NAT Gateway data processing",
				Unit:        "GB",
			},
			{
				Key:         "cloudwatchLogs",
				Value:       "5 GB logs",
				Description: "CloudWatch Logs ingestion and storage",
				Unit:        "GB",
			},
			{
				Key:         "availability",
				Value:       "Single AZ",
				Description: "Availability zone configuration",
				Unit:        "",
			},
			{
				Key:         "uptime",
				Value:       "24/7",
				Description: "Service uptime assumption",
				Unit:        "",
			},
		},
		AsMap: map[string]string{
			"compute":        "0.25 vCPU, 0.5 GB memory",
			"taskCount":      "1 task",
			"database":       "db.t3.micro, 20 GB storage",
			"loadBalancer":   "730 LCU-hours",
			"natGateway":     "50 GB data processed",
			"cloudwatchLogs": "5 GB logs",
			"availability":   "Single AZ",
			"uptime":         "24/7",
		},
	}

	// Full-Stack Application assumptions
	uam.assumptions[string(BlueprintFullStack)] = &UsageAssumptions{
		BlueprintType: string(BlueprintFullStack),
		Assumptions: []UsageAssumption{
			{
				Key:         "frontend",
				Value:       "0.25 vCPU, 0.5 GB memory, 1 task",
				Description: "Frontend Fargate service resources",
				Unit:        "",
			},
			{
				Key:         "backend",
				Value:       "0.5 vCPU, 1.0 GB memory, 2 tasks",
				Description: "Backend Fargate service resources",
				Unit:        "",
			},
			{
				Key:         "database",
				Value:       "db.t3.small, 50 GB storage",
				Description: "RDS instance type and storage",
				Unit:        "",
			},
			{
				Key:         "assets",
				Value:       "20 GB S3 storage",
				Description: "S3 storage for static assets",
				Unit:        "GB",
			},
			{
				Key:         "cdn",
				Value:       "200 GB data transfer",
				Description: "CloudFront data transfer out",
				Unit:        "GB/month",
			},
			{
				Key:         "loadBalancer",
				Value:       "1460 LCU-hours (2 LCUs constant)",
				Description: "Application Load Balancer capacity",
				Unit:        "LCU-hours",
			},
			{
				Key:         "natGateway",
				Value:       "100 GB data processed",
				Description: "NAT Gateway data processing",
				Unit:        "GB",
			},
			{
				Key:         "cloudwatchLogs",
				Value:       "10 GB logs",
				Description: "CloudWatch Logs ingestion and storage",
				Unit:        "GB",
			},
			{
				Key:         "availability",
				Value:       "Single AZ",
				Description: "Availability zone configuration",
				Unit:        "",
			},
			{
				Key:         "backupRetention",
				Value:       "7 days",
				Description: "Database backup retention period",
				Unit:        "days",
			},
		},
		AsMap: map[string]string{
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
		},
	}

	// Microservices assumptions
	uam.assumptions[string(BlueprintMicroservices)] = &UsageAssumptions{
		BlueprintType: string(BlueprintMicroservices),
		Assumptions: []UsageAssumption{
			{
				Key:         "services",
				Value:       "3 services, 0.5 vCPU, 1.0 GB memory each",
				Description: "Number and size of microservices",
				Unit:        "",
			},
			{
				Key:         "tasksPerService",
				Value:       "2 tasks per service",
				Description: "Fargate tasks per microservice",
				Unit:        "tasks",
			},
			{
				Key:         "database",
				Value:       "db.t3.medium, 100 GB storage",
				Description: "RDS instance type and storage",
				Unit:        "",
			},
			{
				Key:         "storage",
				Value:       "50 GB S3 storage",
				Description: "S3 storage for assets and data",
				Unit:        "GB",
			},
			{
				Key:         "cdn",
				Value:       "300 GB data transfer",
				Description: "CloudFront data transfer out",
				Unit:        "GB/month",
			},
			{
				Key:         "loadBalancer",
				Value:       "2190 LCU-hours (3 LCUs constant)",
				Description: "Application Load Balancer capacity",
				Unit:        "LCU-hours",
			},
			{
				Key:         "apiGateway",
				Value:       "1,000,000 requests",
				Description: "API Gateway requests per month",
				Unit:        "requests/month",
			},
			{
				Key:         "messaging",
				Value:       "1,000,000 SQS requests",
				Description: "SQS message queue requests",
				Unit:        "requests/month",
			},
			{
				Key:         "cache",
				Value:       "1 ElastiCache node (cache.t3.micro)",
				Description: "ElastiCache Redis/Memcached nodes",
				Unit:        "nodes",
			},
			{
				Key:         "natGateway",
				Value:       "200 GB data processed",
				Description: "NAT Gateway data processing",
				Unit:        "GB",
			},
			{
				Key:         "cloudwatchLogs",
				Value:       "20 GB logs",
				Description: "CloudWatch Logs ingestion and storage",
				Unit:        "GB",
			},
			{
				Key:         "availability",
				Value:       "Single AZ",
				Description: "Availability zone configuration",
				Unit:        "",
			},
		},
		AsMap: map[string]string{
			"services":        "3 services, 0.5 vCPU, 1.0 GB memory each",
			"tasksPerService": "2 tasks per service",
			"database":        "db.t3.medium, 100 GB storage",
			"storage":         "50 GB S3 storage",
			"cdn":             "300 GB data transfer",
			"loadBalancer":    "2190 LCU-hours (3 LCUs constant)",
			"apiGateway":      "1,000,000 requests",
			"messaging":       "1,000,000 SQS requests",
			"cache":           "1 ElastiCache node (cache.t3.micro)",
			"natGateway":      "200 GB data processed",
			"cloudwatchLogs":  "20 GB logs",
			"availability":    "Single AZ",
		},
	}
}

// GetAssumptions returns usage assumptions for a blueprint type
func (uam *UsageAssumptionsManager) GetAssumptions(blueprintType string) (*UsageAssumptions, error) {
	assumptions, exists := uam.assumptions[blueprintType]
	if !exists {
		return nil, fmt.Errorf("no assumptions found for blueprint type: %s", blueprintType)
	}
	return assumptions, nil
}

// GetAssumptionsAsMap returns usage assumptions as a simple map for a blueprint type
func (uam *UsageAssumptionsManager) GetAssumptionsAsMap(blueprintType string) (map[string]string, error) {
	assumptions, err := uam.GetAssumptions(blueprintType)
	if err != nil {
		return nil, err
	}
	return assumptions.AsMap, nil
}

// GetAssumptionsList returns usage assumptions as a list for a blueprint type
func (uam *UsageAssumptionsManager) GetAssumptionsList(blueprintType string) ([]UsageAssumption, error) {
	assumptions, err := uam.GetAssumptions(blueprintType)
	if err != nil {
		return nil, err
	}
	return assumptions.Assumptions, nil
}

// UpdateAssumption updates a specific assumption for a blueprint type
func (uam *UsageAssumptionsManager) UpdateAssumption(blueprintType, key, value string) error {
	assumptions, exists := uam.assumptions[blueprintType]
	if !exists {
		return fmt.Errorf("no assumptions found for blueprint type: %s", blueprintType)
	}

	// Update in the list
	found := false
	for i, assumption := range assumptions.Assumptions {
		if assumption.Key == key {
			assumptions.Assumptions[i].Value = value
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("assumption key '%s' not found for blueprint type: %s", key, blueprintType)
	}

	// Update in the map
	assumptions.AsMap[key] = value

	return nil
}

// SetCustomAssumptions sets custom assumptions for a blueprint type
func (uam *UsageAssumptionsManager) SetCustomAssumptions(blueprintType string, customAssumptions map[string]string) error {
	assumptions, exists := uam.assumptions[blueprintType]
	if !exists {
		return fmt.Errorf("no assumptions found for blueprint type: %s", blueprintType)
	}

	// Update each custom assumption
	for key, value := range customAssumptions {
		// Update in the list
		for i, assumption := range assumptions.Assumptions {
			if assumption.Key == key {
				assumptions.Assumptions[i].Value = value
				break
			}
		}

		// Update in the map
		assumptions.AsMap[key] = value
	}

	return nil
}

// GetSupportedBlueprints returns list of blueprint types with assumptions
func (uam *UsageAssumptionsManager) GetSupportedBlueprints() []string {
	blueprints := make([]string, 0, len(uam.assumptions))
	for blueprintType := range uam.assumptions {
		blueprints = append(blueprints, blueprintType)
	}
	return blueprints
}

// FormatAssumptionsForDisplay formats assumptions for user-friendly display
func (uam *UsageAssumptionsManager) FormatAssumptionsForDisplay(blueprintType string) (string, error) {
	assumptions, err := uam.GetAssumptions(blueprintType)
	if err != nil {
		return "", err
	}

	result := fmt.Sprintf("Usage Assumptions for %s:\n", blueprintType)
	for _, assumption := range assumptions.Assumptions {
		result += fmt.Sprintf("  • %s: %s", assumption.Description, assumption.Value)
		if assumption.Unit != "" {
			result += fmt.Sprintf(" (%s)", assumption.Unit)
		}
		result += "\n"
	}

	return result, nil
}

// ValidateAssumptionValue validates that an assumption value is reasonable
func (uam *UsageAssumptionsManager) ValidateAssumptionValue(blueprintType, key, value string) error {
	assumptions, exists := uam.assumptions[blueprintType]
	if !exists {
		return fmt.Errorf("no assumptions found for blueprint type: %s", blueprintType)
	}

	// Check if the key exists
	found := false
	for _, assumption := range assumptions.Assumptions {
		if assumption.Key == key {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("assumption key '%s' not found for blueprint type: %s", key, blueprintType)
	}

	// Basic validation - value should not be empty
	if value == "" {
		return fmt.Errorf("assumption value cannot be empty")
	}

	return nil
}
