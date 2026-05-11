package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/pricing/types"
)

// FargateRates represents Fargate pricing structure
type FargateRates struct {
	VCPUPricePerHour    float64 `json:"vcpuPricePerHour"`
	MemoryPricePerHour  float64 `json:"memoryPricePerHour"`
	VCPUPricePerMonth   float64 `json:"vcpuPricePerMonth"`
	MemoryPricePerMonth float64 `json:"memoryPricePerMonth"`
}

// fetchFargateRates fetches Fargate pricing data for all configured regions.
// Individual region failures are logged and skipped (partial failure is OK).
// Returns an error only if ALL regions fail — which indicates a systemic issue
// (e.g. throttling, invalid credentials) rather than a region-specific problem.
func (pf *PricingFetcher) fetchFargateRates() error {
	log.Println("Fetching Fargate pricing data...")

	successCount := 0
	for _, region := range pf.regions {
		log.Printf("Fetching Fargate rates for region: %s", region)

		// Fetch vCPU pricing
		vcpuRates, err := pf.fetchFargateVCPURates(region)
		if err != nil {
			log.Printf("Failed to fetch Fargate vCPU rates for %s: %v", region, err)
			continue
		}

		// Fetch memory pricing
		memoryRates, err := pf.fetchFargateMemoryRates(region)
		if err != nil {
			log.Printf("Failed to fetch Fargate memory rates for %s: %v", region, err)
			continue
		}

		// Save combined rates
		rates := FargateRates{
			VCPUPricePerHour:    vcpuRates,
			MemoryPricePerHour:  memoryRates,
			VCPUPricePerMonth:   vcpuRates * 24 * 30,
			MemoryPricePerMonth: memoryRates * 24 * 30,
		}

		if err := pf.saveFargateRates(region, rates); err != nil {
			log.Printf("Failed to save Fargate rates for %s: %v", region, err)
			continue
		}

		log.Printf("Successfully saved Fargate rates for %s", region)
		successCount++
	}

	// If every region failed, it is a systemic error (throttling, bad credentials, etc.)
	// and the caller should be informed rather than silently returning nil.
	if successCount == 0 && len(pf.regions) > 0 {
		return fmt.Errorf("failed to fetch Fargate rates for all %d regions", len(pf.regions))
	}

	return nil
}


// fetchFargateVCPURates fetches vCPU pricing for Fargate
func (pf *PricingFetcher) fetchFargateVCPURates(region string) (float64, error) {
	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonECS"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("location"),
				Value: aws.String(pf.getLocationName(region)),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("capacityStatus"),
				Value: aws.String("Used"),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("launchType"),
				Value: aws.String("Fargate"),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("usageType"),
				Value: aws.String(fmt.Sprintf("%s-Fargate-vCPU", pf.getUsageTypePrefix(region))),
			},
		},
		MaxResults: aws.Int32(10),
	}

	result, err := pf.client.GetProducts(context.TODO(), input)
	if err != nil {
		return 0, fmt.Errorf("failed to get Fargate vCPU products: %w", err)
	}

	if len(result.PriceList) == 0 {
		return 0, fmt.Errorf("no Fargate vCPU pricing found for region %s", region)
	}

	// Parse the first result
	return pf.parseFargatePricing(result.PriceList[0])
}

// fetchFargateMemoryRates fetches memory pricing for Fargate
func (pf *PricingFetcher) fetchFargateMemoryRates(region string) (float64, error) {
	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonECS"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("location"),
				Value: aws.String(pf.getLocationName(region)),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("capacityStatus"),
				Value: aws.String("Used"),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("launchType"),
				Value: aws.String("Fargate"),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("usageType"),
				Value: aws.String(fmt.Sprintf("%s-Fargate-GB", pf.getUsageTypePrefix(region))),
			},
		},
		MaxResults: aws.Int32(10),
	}

	result, err := pf.client.GetProducts(context.TODO(), input)
	if err != nil {
		return 0, fmt.Errorf("failed to get Fargate memory products: %w", err)
	}

	if len(result.PriceList) == 0 {
		return 0, fmt.Errorf("no Fargate memory pricing found for region %s", region)
	}

	// Parse the first result
	return pf.parseFargatePricing(result.PriceList[0])
}

// parseFargatePricing parses Fargate pricing from AWS pricing API response
func (pf *PricingFetcher) parseFargatePricing(productJSON string) (float64, error) {
	var product map[string]interface{}
	if err := json.Unmarshal([]byte(productJSON), &product); err != nil {
		return 0, fmt.Errorf("failed to unmarshal product JSON: %w", err)
	}

	// Extract pricing information
	terms, ok := product["terms"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("invalid terms structure")
	}

	onDemand, ok := terms["OnDemand"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("no OnDemand pricing found")
	}

	// Get the first (and usually only) pricing term
	for _, termData := range onDemand {
		termMap, ok := termData.(map[string]interface{})
		if !ok {
			continue
		}

		priceDimensions, ok := termMap["priceDimensions"].(map[string]interface{})
		if !ok {
			continue
		}

		for _, dimensionData := range priceDimensions {
			dimensionMap, ok := dimensionData.(map[string]interface{})
			if !ok {
				continue
			}

			pricePerUnit, ok := dimensionMap["pricePerUnit"].(map[string]interface{})
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

	return 0, fmt.Errorf("no valid pricing found")
}

// saveFargateRates saves Fargate pricing rates to database
func (pf *PricingFetcher) saveFargateRates(region string, rates FargateRates) error {
	now := time.Now()
	validUntil := now.Add(24 * time.Hour)

	// Save vCPU pricing
	vcpuData := PricingData{
		Region:        region,
		Service:       "AmazonECS",
		ProductFamily: "Compute",
		InstanceType:  "fargate-vcpu",
		VCPU:          1.0, // Per vCPU
		Memory:        0,
		PricePerHour:  rates.VCPUPricePerHour,
		PricePerMonth: rates.VCPUPricePerMonth,
		Currency:      "USD",
		FetchedAt:     now,
		ValidUntil:    validUntil,
	}

	if err := pf.savePricingData(vcpuData); err != nil {
		return fmt.Errorf("failed to save vCPU pricing: %w", err)
	}

	// Save memory pricing
	memoryData := PricingData{
		Region:        region,
		Service:       "AmazonECS",
		ProductFamily: "Compute",
		InstanceType:  "fargate-memory",
		VCPU:          0,
		Memory:        1.0, // Per GB
		PricePerHour:  rates.MemoryPricePerHour,
		PricePerMonth: rates.MemoryPricePerMonth,
		Currency:      "USD",
		FetchedAt:     now,
		ValidUntil:    validUntil,
	}

	return pf.savePricingData(memoryData)
}

// getUsageTypePrefix returns the usage type prefix for a region
func (pf *PricingFetcher) getUsageTypePrefix(region string) string {
	// Convert region to usage type prefix
	// e.g., us-east-1 -> USE1, us-west-2 -> USW2
	parts := strings.Split(region, "-")
	if len(parts) != 3 {
		return strings.ToUpper(region) // Fallback
	}

	regionMap := map[string]string{
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

	if prefix, exists := regionMap[region]; exists {
		return prefix
	}

	// Generate prefix from region parts
	return strings.ToUpper(parts[0][:2] + parts[1][:1] + parts[2])
}

// GetFargateRates retrieves cached Fargate pricing rates
func (pf *PricingFetcher) GetFargateRates(region string) (*FargateRates, error) {
	// Get vCPU rates
	vcpuData, err := pf.GetCachedPricing(region, "AmazonECS", "Compute", "fargate-vcpu")
	if err != nil {
		return nil, fmt.Errorf("failed to get Fargate vCPU rates: %w", err)
	}

	// Get memory rates
	memoryData, err := pf.GetCachedPricing(region, "AmazonECS", "Compute", "fargate-memory")
	if err != nil {
		return nil, fmt.Errorf("failed to get Fargate memory rates: %w", err)
	}

	return &FargateRates{
		VCPUPricePerHour:    vcpuData.PricePerHour,
		MemoryPricePerHour:  memoryData.PricePerHour,
		VCPUPricePerMonth:   vcpuData.PricePerMonth,
		MemoryPricePerMonth: memoryData.PricePerMonth,
	}, nil
}

// CalculateFargateCost calculates Fargate cost for given vCPU and memory
func (pf *PricingFetcher) CalculateFargateCost(region string, vcpu, memory float64) (float64, error) {
	rates, err := pf.GetFargateRates(region)
	if err != nil {
		return 0, fmt.Errorf("failed to get Fargate rates: %w", err)
	}

	// Calculate monthly cost
	vcpuCost := vcpu * rates.VCPUPricePerMonth
	memoryCost := memory * rates.MemoryPricePerMonth

	return vcpuCost + memoryCost, nil
}