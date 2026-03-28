package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/pricing/types"
)

// ALBRates represents ALB pricing structure
type ALBRates struct {
	HourlyRate float64 `json:"hourlyRate"`
	LCURate    float64 `json:"lcuRate"`
}

// DataTransferRates represents data transfer pricing
type DataTransferRates struct {
	OutboundRate float64 `json:"outboundRate"`
	InboundRate  float64 `json:"inboundRate"`
}

// fetchALBRates fetches ALB pricing data for all regions
func (pf *PricingFetcher) fetchALBRates() error {
	log.Println("Fetching ALB pricing data...")

	for _, region := range pf.regions {
		log.Printf("Fetching ALB rates for region: %s", region)

		// Fetch ALB hourly pricing
		hourlyRate, err := pf.fetchALBHourlyRates(region)
		if err != nil {
			log.Printf("Failed to fetch ALB hourly rates for %s: %v", region, err)
			continue
		}

		// Fetch ALB LCU pricing
		lcuRate, err := pf.fetchALBLCURates(region)
		if err != nil {
			log.Printf("Failed to fetch ALB LCU rates for %s: %v", region, err)
			continue
		}

		// Fetch data transfer pricing
		dataTransferRates, err := pf.fetchDataTransferRates(region)
		if err != nil {
			log.Printf("Failed to fetch data transfer rates for %s: %v", region, err)
			continue
		}

		// Save ALB rates
		albRates := ALBRates{
			HourlyRate: hourlyRate,
			LCURate:    lcuRate,
		}

		if err := pf.saveALBRates(region, albRates); err != nil {
			log.Printf("Failed to save ALB rates for %s: %v", region, err)
			continue
		}

		if err := pf.saveDataTransferRates(region, *dataTransferRates); err != nil {
			log.Printf("Failed to save data transfer rates for %s: %v", region, err)
			continue
		}

		log.Printf("Successfully saved ALB rates for %s", region)
	}

	return nil
}

// fetchALBHourlyRates fetches ALB hourly pricing
func (pf *PricingFetcher) fetchALBHourlyRates(region string) (float64, error) {
	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AWSELB"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("location"),
				Value: aws.String(pf.getLocationName(region)),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("productFamily"),
				Value: aws.String("Load Balancer-Application"),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("usageType"),
				Value: aws.String(fmt.Sprintf("%s-LoadBalancerUsage", pf.getUsageTypePrefix(region))),
			},
		},
		MaxResults: aws.Int32(10),
	}

	result, err := pf.client.GetProducts(context.TODO(), input)
	if err != nil {
		return 0, fmt.Errorf("failed to get ALB hourly products: %w", err)
	}

	if len(result.PriceList) == 0 {
		return 0, fmt.Errorf("no ALB hourly pricing found for region %s", region)
	}

	return pf.parseALBPricing(result.PriceList[0])
}

// fetchALBLCURates fetches ALB LCU pricing
func (pf *PricingFetcher) fetchALBLCURates(region string) (float64, error) {
	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AWSELB"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("location"),
				Value: aws.String(pf.getLocationName(region)),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("productFamily"),
				Value: aws.String("Load Balancer-Application"),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("usageType"),
				Value: aws.String(fmt.Sprintf("%s-LCUUsage", pf.getUsageTypePrefix(region))),
			},
		},
		MaxResults: aws.Int32(10),
	}

	result, err := pf.client.GetProducts(context.TODO(), input)
	if err != nil {
		return 0, fmt.Errorf("failed to get ALB LCU products: %w", err)
	}

	if len(result.PriceList) == 0 {
		return 0, fmt.Errorf("no ALB LCU pricing found for region %s", region)
	}

	return pf.parseALBPricing(result.PriceList[0])
}

// fetchDataTransferRates fetches data transfer pricing
func (pf *PricingFetcher) fetchDataTransferRates(region string) (*DataTransferRates, error) {
	// Fetch outbound data transfer rates
	outboundRate, err := pf.fetchOutboundDataTransferRates(region)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch outbound data transfer rates: %w", err)
	}

	// Inbound data transfer is typically free
	inboundRate := 0.0

	return &DataTransferRates{
		OutboundRate: outboundRate,
		InboundRate:  inboundRate,
	}, nil
}

// fetchOutboundDataTransferRates fetches outbound data transfer pricing
func (pf *PricingFetcher) fetchOutboundDataTransferRates(region string) (float64, error) {
	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonEC2"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("location"),
				Value: aws.String(pf.getLocationName(region)),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("productFamily"),
				Value: aws.String("Data Transfer"),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("transferType"),
				Value: aws.String("AWS Outbound"),
			},
		},
		MaxResults: aws.Int32(10),
	}

	result, err := pf.client.GetProducts(context.TODO(), input)
	if err != nil {
		return 0, fmt.Errorf("failed to get data transfer products: %w", err)
	}

	if len(result.PriceList) == 0 {
		return 0, fmt.Errorf("no data transfer pricing found for region %s", region)
	}

	return pf.parseALBPricing(result.PriceList[0])
}

// parseALBPricing parses ALB pricing from AWS pricing API response
func (pf *PricingFetcher) parseALBPricing(productJSON string) (float64, error) {
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

// saveALBRates saves ALB pricing to database
func (pf *PricingFetcher) saveALBRates(region string, rates ALBRates) error {
	now := time.Now()
	validUntil := now.Add(24 * time.Hour)

	// Save hourly rate
	hourlyData := PricingData{
		Region:        region,
		Service:       "AWSELB",
		ProductFamily: "Load Balancer",
		InstanceType:  "application-hourly",
		VCPU:          0,
		Memory:        0,
		PricePerHour:  rates.HourlyRate,
		PricePerMonth: rates.HourlyRate * 24 * 30,
		Currency:      "USD",
		FetchedAt:     now,
		ValidUntil:    validUntil,
	}

	if err := pf.savePricingData(hourlyData); err != nil {
		return fmt.Errorf("failed to save ALB hourly pricing: %w", err)
	}

	// Save LCU rate
	lcuData := PricingData{
		Region:        region,
		Service:       "AWSELB",
		ProductFamily: "Load Balancer",
		InstanceType:  "application-lcu",
		VCPU:          0,
		Memory:        0,
		PricePerHour:  rates.LCURate,
		PricePerMonth: rates.LCURate * 24 * 30,
		Currency:      "USD",
		FetchedAt:     now,
		ValidUntil:    validUntil,
	}

	return pf.savePricingData(lcuData)
}

// saveDataTransferRates saves data transfer pricing to database
func (pf *PricingFetcher) saveDataTransferRates(region string, rates DataTransferRates) error {
	now := time.Now()
	validUntil := now.Add(24 * time.Hour)

	// Save outbound rate
	outboundData := PricingData{
		Region:        region,
		Service:       "AmazonEC2",
		ProductFamily: "Data Transfer",
		InstanceType:  "outbound",
		VCPU:          0,
		Memory:        0,
		PricePerHour:  rates.OutboundRate / (24 * 30), // Convert to hourly
		PricePerMonth: rates.OutboundRate * 100,        // Assume 100GB per month
		Currency:      "USD",
		FetchedAt:     now,
		ValidUntil:    validUntil,
	}

	if err := pf.savePricingData(outboundData); err != nil {
		return fmt.Errorf("failed to save outbound data transfer pricing: %w", err)
	}

	// Save inbound rate (typically free)
	inboundData := PricingData{
		Region:        region,
		Service:       "AmazonEC2",
		ProductFamily: "Data Transfer",
		InstanceType:  "inbound",
		VCPU:          0,
		Memory:        0,
		PricePerHour:  rates.InboundRate,
		PricePerMonth: rates.InboundRate,
		Currency:      "USD",
		FetchedAt:     now,
		ValidUntil:    validUntil,
	}

	return pf.savePricingData(inboundData)
}

// GetALBRates retrieves cached ALB pricing
func (pf *PricingFetcher) GetALBRates(region string) (*ALBRates, error) {
	// Get hourly rate
	hourlyData, err := pf.GetCachedPricing(region, "AWSELB", "Load Balancer", "application-hourly")
	if err != nil {
		return nil, fmt.Errorf("failed to get ALB hourly rates: %w", err)
	}

	// Get LCU rate
	lcuData, err := pf.GetCachedPricing(region, "AWSELB", "Load Balancer", "application-lcu")
	if err != nil {
		return nil, fmt.Errorf("failed to get ALB LCU rates: %w", err)
	}

	return &ALBRates{
		HourlyRate: hourlyData.PricePerHour,
		LCURate:    lcuData.PricePerHour,
	}, nil
}

// GetDataTransferRates retrieves cached data transfer pricing
func (pf *PricingFetcher) GetDataTransferRates(region string) (*DataTransferRates, error) {
	// Get outbound rate
	outboundData, err := pf.GetCachedPricing(region, "AmazonEC2", "Data Transfer", "outbound")
	if err != nil {
		return nil, fmt.Errorf("failed to get outbound data transfer rates: %w", err)
	}

	// Get inbound rate
	inboundData, err := pf.GetCachedPricing(region, "AmazonEC2", "Data Transfer", "inbound")
	if err != nil {
		return nil, fmt.Errorf("failed to get inbound data transfer rates: %w", err)
	}

	return &DataTransferRates{
		OutboundRate: outboundData.PricePerHour * 24 * 30, // Convert back to per GB
		InboundRate:  inboundData.PricePerHour,
	}, nil
}

// CalculateALBCost calculates ALB cost for given usage
func (pf *PricingFetcher) CalculateALBCost(region string, hoursPerMonth float64, lcuHours float64) (float64, error) {
	rates, err := pf.GetALBRates(region)
	if err != nil {
		return 0, fmt.Errorf("failed to get ALB rates: %w", err)
	}

	// Calculate costs
	hourlyCost := hoursPerMonth * rates.HourlyRate
	lcuCost := lcuHours * rates.LCURate

	return hourlyCost + lcuCost, nil
}

// CalculateDataTransferCost calculates data transfer cost
func (pf *PricingFetcher) CalculateDataTransferCost(region string, outboundGB, inboundGB float64) (float64, error) {
	rates, err := pf.GetDataTransferRates(region)
	if err != nil {
		return 0, fmt.Errorf("failed to get data transfer rates: %w", err)
	}

	// Calculate costs
	outboundCost := outboundGB * rates.OutboundRate
	inboundCost := inboundGB * rates.InboundRate

	return outboundCost + inboundCost, nil
}