package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/pricing/types"
)

type CostEstimatorService struct {
	pricingClient *pricing.Client
	cache         map[string]*CachedPrice
}

type CachedPrice struct {
	Price     float64
	Timestamp time.Time
}

type CostEstimate struct {
	TotalMonthlyCost float64            `json:"totalMonthlyCost"`
	Breakdown        map[string]float64 `json:"breakdown"`
	Currency         string             `json:"currency"`
	Region           string             `json:"region"`
	Disclaimer       string             `json:"disclaimer"`
}

type DeploymentConfiguration struct {
	Blueprint     string                 `json:"blueprint"`
	Region        string                 `json:"region"`
	InstanceType  string                 `json:"instanceType"`
	Configuration map[string]interface{} `json:"configuration"`
}

func NewCostEstimatorService(region string) (*CostEstimatorService, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion("us-east-1")) // Pricing API is only in us-east-1
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &CostEstimatorService{
		pricingClient: pricing.NewFromConfig(cfg),
		cache:         make(map[string]*CachedPrice),
	}, nil
}

// EstimateCost calculates the estimated monthly cost for a deployment
func (e *CostEstimatorService) EstimateCost(config DeploymentConfiguration) (*CostEstimate, error) {
	estimate := &CostEstimate{
		Breakdown:  make(map[string]float64),
		Currency:   "USD",
		Region:     config.Region,
		Disclaimer: "Estimates are approximate and actual costs may vary based on usage patterns, data transfer, and other factors.",
	}

	switch config.Blueprint {
	case "ecs-web-app":
		return e.estimateECSWebAppCost(config, estimate)
	case "full-stack":
		return e.estimateFullStackCost(config, estimate)
	case "static-site":
		return e.estimateStaticSiteCost(config, estimate)
	default:
		return e.estimateBasicCost(config, estimate)
	}
}

// estimateECSWebAppCost calculates cost for ECS web application
func (e *CostEstimatorService) estimateECSWebAppCost(config DeploymentConfiguration, estimate *CostEstimate) (*CostEstimate, error) {
	// ECS Fargate compute cost
	instanceType := config.InstanceType
	if instanceType == "" {
		instanceType = "256"
	}

	fargatePrice, err := e.getFargatePrice(config.Region, instanceType)
	if err != nil {
		// Fallback to estimated pricing
		fargatePrice = e.getFallbackFargatePrice(instanceType)
	}
	estimate.Breakdown["ECS Fargate Compute"] = fargatePrice

	// Application Load Balancer cost
	albPrice, err := e.getALBPrice(config.Region)
	if err != nil {
		albPrice = 16.20 // Fallback: ~$16.20/month for ALB
	}
	estimate.Breakdown["Application Load Balancer"] = albPrice

	// NAT Gateway cost
	natPrice, err := e.getNATGatewayPrice(config.Region)
	if err != nil {
		natPrice = 32.40 // Fallback: ~$32.40/month for NAT Gateway
	}
	estimate.Breakdown["NAT Gateway"] = natPrice

	// CloudWatch Logs cost (estimated)
	estimate.Breakdown["CloudWatch Logs"] = 2.00

	// Data transfer cost (estimated)
	estimate.Breakdown["Data Transfer"] = 5.00

	// Calculate total
	for _, cost := range estimate.Breakdown {
		estimate.TotalMonthlyCost += cost
	}

	return estimate, nil
}

// estimateFullStackCost calculates cost for full stack application with RDS
func (e *CostEstimatorService) estimateFullStackCost(config DeploymentConfiguration, estimate *CostEstimate) (*CostEstimate, error) {
	// Start with ECS web app costs
	estimate, err := e.estimateECSWebAppCost(config, estimate)
	if err != nil {
		return nil, err
	}

	// Add RDS cost
	dbInstanceClass := "db.t3.micro"
	if dbConfig, ok := config.Configuration["db_instance_class"].(string); ok {
		dbInstanceClass = dbConfig
	}

	rdsPrice, err := e.getRDSPrice(config.Region, dbInstanceClass)
	if err != nil {
		rdsPrice = e.getFallbackRDSPrice(dbInstanceClass)
	}
	estimate.Breakdown["RDS Database"] = rdsPrice

	// RDS storage cost
	storageGB := 20.0
	if storage, ok := config.Configuration["db_allocated_storage"].(float64); ok {
		storageGB = storage
	}
	storagePrice := storageGB * 0.115 // ~$0.115 per GB/month for gp2
	estimate.Breakdown["RDS Storage"] = storagePrice

	// Recalculate total
	estimate.TotalMonthlyCost = 0
	for _, cost := range estimate.Breakdown {
		estimate.TotalMonthlyCost += cost
	}

	return estimate, nil
}

// estimateStaticSiteCost calculates cost for static website
func (e *CostEstimatorService) estimateStaticSiteCost(config DeploymentConfiguration, estimate *CostEstimate) (*CostEstimate, error) {
	// S3 storage cost (very minimal for static sites)
	estimate.Breakdown["S3 Storage"] = 0.50

	// CloudFront cost
	cloudFrontPrice, err := e.getCloudFrontPrice(config.Region)
	if err != nil {
		cloudFrontPrice = 1.00 // Very low for basic usage
	}
	estimate.Breakdown["CloudFront CDN"] = cloudFrontPrice

	// Route 53 (if custom domain)
	if domain, ok := config.Configuration["domain_name"].(string); ok && domain != "" {
		estimate.Breakdown["Route 53 Hosted Zone"] = 0.50
	}

	// Calculate total
	for _, cost := range estimate.Breakdown {
		estimate.TotalMonthlyCost += cost
	}

	return estimate, nil
}

// estimateBasicCost provides a basic cost estimate for unknown blueprints
func (e *CostEstimatorService) estimateBasicCost(config DeploymentConfiguration, estimate *CostEstimate) (*CostEstimate, error) {
	estimate.Breakdown["Estimated Base Cost"] = 25.00
	estimate.TotalMonthlyCost = 25.00
	return estimate, nil
}

// getFargatePrice gets the current Fargate pricing from AWS Pricing API
func (e *CostEstimatorService) getFargatePrice(region, instanceType string) (float64, error) {
	cacheKey := fmt.Sprintf("fargate-%s-%s", region, instanceType)
	
	// Check cache first
	if cached, exists := e.cache[cacheKey]; exists {
		if time.Since(cached.Timestamp) < 24*time.Hour {
			return cached.Price, nil
		}
	}

	// Convert instance type to vCPU and memory
	vCPU, memory := e.parseInstanceType(instanceType)
	
	// Get vCPU price
	vCPUPrice, err := e.getPriceFromAPI("AmazonECS", region, map[string]string{
		"servicecode":   "AmazonECS",
		"productFamily": "Compute",
		"usagetype":     fmt.Sprintf("%s-Fargate-vCPU", e.getRegionCode(region)),
	})
	if err != nil {
		return 0, err
	}

	// Get memory price
	memoryPrice, err := e.getPriceFromAPI("AmazonECS", region, map[string]string{
		"servicecode":   "AmazonECS",
		"productFamily": "Compute",
		"usagetype":     fmt.Sprintf("%s-Fargate-GB", e.getRegionCode(region)),
	})
	if err != nil {
		return 0, err
	}

	// Calculate monthly cost (assuming 24/7 usage)
	hoursPerMonth := 730.0
	totalPrice := (vCPUPrice*vCPU + memoryPrice*memory) * hoursPerMonth

	// Cache the result
	e.cache[cacheKey] = &CachedPrice{
		Price:     totalPrice,
		Timestamp: time.Now(),
	}

	return totalPrice, nil
}

// getFallbackFargatePrice provides fallback pricing when API is unavailable
func (e *CostEstimatorService) getFallbackFargatePrice(instanceType string) float64 {
	switch instanceType {
	case "256":
		return 15.00 // ~$15/month for 0.25 vCPU, 0.5 GB
	case "512":
		return 25.00 // ~$25/month for 0.5 vCPU, 1 GB
	case "1024":
		return 45.00 // ~$45/month for 1 vCPU, 2 GB
	default:
		return 25.00
	}
}

// getFallbackRDSPrice provides fallback RDS pricing
func (e *CostEstimatorService) getFallbackRDSPrice(instanceClass string) float64 {
	switch instanceClass {
	case "db.t3.micro":
		return 15.00
	case "db.t3.small":
		return 30.00
	case "db.t3.medium":
		return 60.00
	default:
		return 30.00
	}
}

// parseInstanceType converts instance type string to vCPU and memory values
func (e *CostEstimatorService) parseInstanceType(instanceType string) (float64, float64) {
	switch instanceType {
	case "256":
		return 0.25, 0.5
	case "512":
		return 0.5, 1.0
	case "1024":
		return 1.0, 2.0
	default:
		return 0.5, 1.0
	}
}

// getRegionCode converts region name to pricing API region code
func (e *CostEstimatorService) getRegionCode(region string) string {
	regionCodes := map[string]string{
		"us-east-1":      "USE1",
		"us-east-2":      "USE2",
		"us-west-1":      "USW1",
		"us-west-2":      "USW2",
		"eu-west-1":      "EUW1",
		"eu-west-2":      "EUW2",
		"eu-central-1":   "EUC1",
		"ap-southeast-1": "APS1",
		"ap-southeast-2": "APS2",
		"ap-northeast-1": "APN1",
	}
	
	if code, exists := regionCodes[region]; exists {
		return code
	}
	return "USE1" // Default to us-east-1
}

// getPriceFromAPI queries the AWS Pricing API
func (e *CostEstimatorService) getPriceFromAPI(serviceCode, region string, filters map[string]string) (float64, error) {
	var filterList []types.Filter
	
	for key, value := range filters {
		filterList = append(filterList, types.Filter{
			Field: aws.String(key),
			Value: aws.String(value),
			Type:  types.FilterTypeTermMatch,
		})
	}

	input := &pricing.GetProductsInput{
		ServiceCode: aws.String(serviceCode),
		Filters:     filterList,
	}

	result, err := e.pricingClient.GetProducts(context.Background(), input)
	if err != nil {
		return 0, fmt.Errorf("failed to get pricing: %w", err)
	}

	if len(result.PriceList) == 0 {
		return 0, fmt.Errorf("no pricing data found")
	}

	// Parse the first result
	var priceData map[string]interface{}
	if err := json.Unmarshal([]byte(result.PriceList[0]), &priceData); err != nil {
		return 0, fmt.Errorf("failed to parse pricing data: %w", err)
	}

	// Extract price from the complex pricing structure
	price, err := e.extractPriceFromData(priceData)
	if err != nil {
		return 0, fmt.Errorf("failed to extract price: %w", err)
	}

	return price, nil
}

// extractPriceFromData extracts the actual price from AWS pricing API response
func (e *CostEstimatorService) extractPriceFromData(data map[string]interface{}) (float64, error) {
	terms, ok := data["terms"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("no terms found in pricing data")
	}

	onDemand, ok := terms["OnDemand"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("no OnDemand pricing found")
	}

	// Get the first (and usually only) pricing term
	for _, termData := range onDemand {
		term, ok := termData.(map[string]interface{})
		if !ok {
			continue
		}

		priceDimensions, ok := term["priceDimensions"].(map[string]interface{})
		if !ok {
			continue
		}

		// Get the first price dimension
		for _, dimensionData := range priceDimensions {
			dimension, ok := dimensionData.(map[string]interface{})
			if !ok {
				continue
			}

			pricePerUnit, ok := dimension["pricePerUnit"].(map[string]interface{})
			if !ok {
				continue
			}

			usdPrice, ok := pricePerUnit["USD"].(string)
			if !ok {
				continue
			}

			price, err := strconv.ParseFloat(usdPrice, 64)
			if err != nil {
				continue
			}

			return price, nil
		}
	}

	return 0, fmt.Errorf("no valid price found in pricing data")
}

// getALBPrice gets Application Load Balancer pricing
func (e *CostEstimatorService) getALBPrice(region string) (float64, error) {
	// ALB has fixed hourly cost + LCU cost
	// Simplified calculation for basic usage
	hoursPerMonth := 730.0
	albHourlyRate := 0.0225 // ~$0.0225/hour
	return albHourlyRate * hoursPerMonth, nil
}

// getNATGatewayPrice gets NAT Gateway pricing
func (e *CostEstimatorService) getNATGatewayPrice(region string) (float64, error) {
	hoursPerMonth := 730.0
	natHourlyRate := 0.045 // ~$0.045/hour
	return natHourlyRate * hoursPerMonth, nil
}

// getCloudFrontPrice gets CloudFront pricing (simplified)
func (e *CostEstimatorService) getCloudFrontPrice(region string) (float64, error) {
	// CloudFront pricing is complex, this is a simplified estimate for low usage
	return 1.00, nil
}

// getRDSPrice gets RDS pricing for the specified instance class
func (e *CostEstimatorService) getRDSPrice(region, instanceClass string) (float64, error) {
	cacheKey := fmt.Sprintf("rds-%s-%s", region, instanceClass)
	
	// Check cache first
	if cached, exists := e.cache[cacheKey]; exists {
		if time.Since(cached.Timestamp) < 24*time.Hour {
			return cached.Price, nil
		}
	}

	// Try to get from pricing API
	price, err := e.getPriceFromAPI("AmazonRDS", region, map[string]string{
		"servicecode":    "AmazonRDS",
		"productFamily":  "Database Instance",
		"instanceType":   instanceClass,
		"databaseEngine": "PostgreSQL",
	})
	
	if err != nil {
		return 0, err
	}

	// Calculate monthly cost
	hoursPerMonth := 730.0
	totalPrice := price * hoursPerMonth

	// Cache the result
	e.cache[cacheKey] = &CachedPrice{
		Price:     totalPrice,
		Timestamp: time.Now(),
	}

	return totalPrice, nil
}