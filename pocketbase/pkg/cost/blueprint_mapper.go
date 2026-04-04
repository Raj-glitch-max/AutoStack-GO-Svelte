package cost

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// BlueprintMapper maps blueprint types to their cost calculators
type BlueprintMapper struct {
	app                  core.App
	staticWebsiteCalc    *StaticWebsiteBlueprintCalculator
	webAppCalc           *WebAppBlueprintCalculator
	fullStackCalc        *FullStackBlueprintCalculator
	microservicesCalc    *MicroservicesBlueprintCalculator
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

// EstimateCost estimates cost for a given blueprint type
func (bm *BlueprintMapper) EstimateCost(blueprintType string, config map[string]interface{}, region string) (*CostEstimateWithRange, error) {
	switch BlueprintType(blueprintType) {
	case BlueprintStaticWebsite:
		return bm.staticWebsiteCalc.EstimateFromBlueprint(config, region)
	case BlueprintWebApp:
		return bm.webAppCalc.EstimateFromBlueprint(config, region)
	case BlueprintFullStack:
		return bm.fullStackCalc.EstimateFromBlueprint(config, region)
	case BlueprintMicroservices:
		return bm.microservicesCalc.EstimateFromBlueprint(config, region)
	default:
		return nil, fmt.Errorf("unsupported blueprint type: %s", blueprintType)
	}
}

// GetDefaultAssumptions returns default assumptions for a blueprint type
func (bm *BlueprintMapper) GetDefaultAssumptions(blueprintType string) (map[string]string, error) {
	switch BlueprintType(blueprintType) {
	case BlueprintStaticWebsite:
		return bm.staticWebsiteCalc.GetDefaultAssumptions(), nil
	case BlueprintWebApp:
		return bm.webAppCalc.GetDefaultAssumptions(), nil
	case BlueprintFullStack:
		return bm.fullStackCalc.GetDefaultAssumptions(), nil
	case BlueprintMicroservices:
		return bm.microservicesCalc.GetDefaultAssumptions(), nil
	default:
		return nil, fmt.Errorf("unsupported blueprint type: %s", blueprintType)
	}
}

// GetDisclaimer returns cost disclaimer for a blueprint type
func (bm *BlueprintMapper) GetDisclaimer(blueprintType string) (string, error) {
	switch BlueprintType(blueprintType) {
	case BlueprintStaticWebsite:
		return bm.staticWebsiteCalc.GetDisclaimer(), nil
	case BlueprintWebApp:
		return bm.webAppCalc.GetDisclaimer(), nil
	case BlueprintFullStack:
		return bm.fullStackCalc.GetDisclaimer(), nil
	case BlueprintMicroservices:
		return bm.microservicesCalc.GetDisclaimer(), nil
	default:
		return "", fmt.Errorf("unsupported blueprint type: %s", blueprintType)
	}
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

// ValidateBlueprintType checks if a blueprint type is supported
func (bm *BlueprintMapper) ValidateBlueprintType(blueprintType string) bool {
	supported := bm.GetSupportedBlueprints()
	for _, bt := range supported {
		if bt == blueprintType {
			return true
		}
	}
	return false
}

// GetBlueprintServices returns list of AWS services used by a blueprint
func (bm *BlueprintMapper) GetBlueprintServices(blueprintType string) ([]string, error) {
	switch BlueprintType(blueprintType) {
	case BlueprintStaticWebsite:
		return []string{"S3", "CloudFront", "Route53"}, nil
	case BlueprintWebApp:
		return []string{"Fargate", "ALB", "RDS", "NAT Gateway", "CloudWatch Logs", "ECR"}, nil
	case BlueprintFullStack:
		return []string{"Fargate", "ALB", "RDS", "S3", "CloudFront", "NAT Gateway", "CloudWatch Logs", "ECR"}, nil
	case BlueprintMicroservices:
		return []string{"Fargate", "ALB", "RDS", "S3", "CloudFront", "NAT Gateway", "CloudWatch Logs", "API Gateway", "SQS", "ElastiCache", "ECR"}, nil
	default:
		return nil, fmt.Errorf("unsupported blueprint type: %s", blueprintType)
	}
}

// BlueprintCostEstimate represents a complete cost estimate with metadata
type BlueprintCostEstimate struct {
	BlueprintType string                 `json:"blueprintType"`
	Region        string                 `json:"region"`
	Estimate      *CostEstimateWithRange `json:"estimate"`
	Assumptions   map[string]string      `json:"assumptions"`
	Disclaimer    string                 `json:"disclaimer"`
	Services      []string               `json:"services"`
	CreatedAt     string                 `json:"createdAt"`
}

// GetCompleteEstimate returns a complete cost estimate with all metadata
func (bm *BlueprintMapper) GetCompleteEstimate(blueprintType string, config map[string]interface{}, region string) (*BlueprintCostEstimate, error) {
	// Validate blueprint type
	if !bm.ValidateBlueprintType(blueprintType) {
		return nil, fmt.Errorf("unsupported blueprint type: %s", blueprintType)
	}

	// Get cost estimate
	estimate, err := bm.EstimateCost(blueprintType, config, region)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate cost: %w", err)
	}

	// Get assumptions
	assumptions, err := bm.GetDefaultAssumptions(blueprintType)
	if err != nil {
		return nil, fmt.Errorf("failed to get assumptions: %w", err)
	}

	// Get disclaimer
	disclaimer, err := bm.GetDisclaimer(blueprintType)
	if err != nil {
		return nil, fmt.Errorf("failed to get disclaimer: %w", err)
	}

	// Get services
	services, err := bm.GetBlueprintServices(blueprintType)
	if err != nil {
		return nil, fmt.Errorf("failed to get services: %w", err)
	}

	return &BlueprintCostEstimate{
		BlueprintType: blueprintType,
		Region:        region,
		Estimate:      estimate,
		Assumptions:   assumptions,
		Disclaimer:    disclaimer,
		Services:      services,
		CreatedAt:     "", // Will be set by caller
	}, nil
}
