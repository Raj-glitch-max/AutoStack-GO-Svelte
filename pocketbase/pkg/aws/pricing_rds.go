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

// RDSRates represents RDS pricing structure
type RDSRates struct {
	InstanceHourlyRate float64 `json:"instanceHourlyRate"`
	StorageMonthlyRate float64 `json:"storageMonthlyRate"`
	InstanceClass      string  `json:"instanceClass"`
	Engine             string  `json:"engine"`
	DeploymentOption   string  `json:"deploymentOption"`
}

// fetchRDSRates fetches RDS pricing data for all regions
func (pf *PricingFetcher) fetchRDSRates() error {
	log.Println("Fetching RDS pricing data...")

	// Common RDS instance classes to fetch pricing for
	instanceClasses := []string{"db.t3.micro", "db.t3.small", "db.t3.medium"}
	engines := []string{"MySQL", "PostgreSQL"}

	for _, region := range pf.regions {
		log.Printf("Fetching RDS rates for region: %s", region)

		for _, instanceClass := range instanceClasses {
			for _, engine := range engines {
				// Fetch instance pricing
				instanceRate, err := pf.fetchRDSInstanceRates(region, instanceClass, engine)
				if err != nil {
					log.Printf("Failed to fetch RDS instance rates for %s/%s in %s: %v", instanceClass, engine, region, err)
					continue
				}

				// Fetch storage pricing
				storageRate, err := pf.fetchRDSStorageRates(region, engine)
				if err != nil {
					log.Printf("Failed to fetch RDS storage rates for %s in %s: %v", engine, region, err)
					continue
				}

				// Save RDS rates
				rates := RDSRates{
					InstanceHourlyRate: instanceRate,
					StorageMonthlyRate: storageRate,
					InstanceClass:      instanceClass,
					Engine:             engine,
					DeploymentOption:   "Single-AZ",
				}

				if err := pf.saveRDSRates(region, rates); err != nil {
					log.Printf("Failed to save RDS rates for %s/%s in %s: %v", instanceClass, engine, region, err)
					continue
				}
			}
		}

		log.Printf("Successfully saved RDS rates for %s", region)
	}

	return nil
}

// fetchRDSInstanceRates fetches RDS instance pricing
func (pf *PricingFetcher) fetchRDSInstanceRates(region, instanceClass, engine string) (float64, error) {
	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonRDS"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("location"),
				Value: aws.String(pf.getLocationName(region)),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("instanceType"),
				Value: aws.String(instanceClass),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("databaseEngine"),
				Value: aws.String(engine),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("deploymentOption"),
				Value: aws.String("Single-AZ"),
			},
		},
		MaxResults: aws.Int32(10),
	}

	result, err := pf.client.GetProducts(context.TODO(), input)
	if err != nil {
		return 0, fmt.Errorf("failed to get RDS instance products: %w", err)
	}

	if len(result.PriceList) == 0 {
		return 0, fmt.Errorf("no RDS instance pricing found for %s/%s in region %s", instanceClass, engine, region)
	}

	return pf.parseRDSPricing(result.PriceList[0])
}

// fetchRDSStorageRates fetches RDS storage pricing
func (pf *PricingFetcher) fetchRDSStorageRates(region, engine string) (float64, error) {
	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonRDS"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("location"),
				Value: aws.String(pf.getLocationName(region)),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("databaseEngine"),
				Value: aws.String(engine),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("productFamily"),
				Value: aws.String("Database Storage"),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("volumeType"),
				Value: aws.String("General Purpose"),
			},
		},
		MaxResults: aws.Int32(10),
	}

	result, err := pf.client.GetProducts(context.TODO(), input)
	if err != nil {
		return 0, fmt.Errorf("failed to get RDS storage products: %w", err)
	}

	if len(result.PriceList) == 0 {
		return 0, fmt.Errorf("no RDS storage pricing found for %s in region %s", engine, region)
	}

	return pf.parseRDSPricing(result.PriceList[0])
}

// parseRDSPricing parses RDS pricing from AWS pricing API response
func (pf *PricingFetcher) parseRDSPricing(productJSON string) (float64, error) {
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

// saveRDSRates saves RDS pricing to database
func (pf *PricingFetcher) saveRDSRates(region string, rates RDSRates) error {
	now := time.Now()
	validUntil := now.Add(24 * time.Hour)

	// Save instance pricing
	instanceType := fmt.Sprintf("%s-%s-%s", rates.InstanceClass, rates.Engine, rates.DeploymentOption)
	instanceData := PricingData{
		Region:        region,
		Service:       "AmazonRDS",
		ProductFamily: "Database Instance",
		InstanceType:  instanceType,
		VCPU:          0, // Could be extracted from instance class if needed
		Memory:        0, // Could be extracted from instance class if needed
		PricePerHour:  rates.InstanceHourlyRate,
		PricePerMonth: rates.InstanceHourlyRate * 24 * 30,
		Currency:      "USD",
		FetchedAt:     now,
		ValidUntil:    validUntil,
	}

	if err := pf.savePricingData(instanceData); err != nil {
		return fmt.Errorf("failed to save RDS instance pricing: %w", err)
	}

	// Save storage pricing
	storageType := fmt.Sprintf("storage-%s", rates.Engine)
	storageData := PricingData{
		Region:        region,
		Service:       "AmazonRDS",
		ProductFamily: "Database Storage",
		InstanceType:  storageType,
		VCPU:          0,
		Memory:        0,
		PricePerHour:  rates.StorageMonthlyRate / (24 * 30), // Convert monthly to hourly
		PricePerMonth: rates.StorageMonthlyRate * 20,        // Assume 20GB storage
		Currency:      "USD",
		FetchedAt:     now,
		ValidUntil:    validUntil,
	}

	return pf.savePricingData(storageData)
}

// GetRDSInstanceRates retrieves cached RDS instance pricing
func (pf *PricingFetcher) GetRDSInstanceRates(region, instanceClass, engine string) (float64, error) {
	instanceType := fmt.Sprintf("%s-%s-Single-AZ", instanceClass, engine)
	data, err := pf.GetCachedPricing(region, "AmazonRDS", "Database Instance", instanceType)
	if err != nil {
		return 0, fmt.Errorf("failed to get RDS instance rates: %w", err)
	}

	return data.PricePerHour, nil
}

// GetRDSStorageRates retrieves cached RDS storage pricing
func (pf *PricingFetcher) GetRDSStorageRates(region, engine string) (float64, error) {
	storageType := fmt.Sprintf("storage-%s", engine)
	data, err := pf.GetCachedPricing(region, "AmazonRDS", "Database Storage", storageType)
	if err != nil {
		return 0, fmt.Errorf("failed to get RDS storage rates: %w", err)
	}

	return data.PricePerHour * 24 * 30 / 20, nil // Convert back to per GB per month
}

// CalculateRDSCost calculates RDS cost for given instance and storage
func (pf *PricingFetcher) CalculateRDSCost(region, instanceClass, engine string, storageGB float64) (float64, error) {
	// Get instance rates
	instanceRate, err := pf.GetRDSInstanceRates(region, instanceClass, engine)
	if err != nil {
		return 0, fmt.Errorf("failed to get RDS instance rates: %w", err)
	}

	// Get storage rates
	storageRate, err := pf.GetRDSStorageRates(region, engine)
	if err != nil {
		return 0, fmt.Errorf("failed to get RDS storage rates: %w", err)
	}

	// Calculate monthly costs
	instanceCost := instanceRate * 24 * 30 // Monthly instance cost
	storageCost := storageGB * storageRate  // Monthly storage cost

	return instanceCost + storageCost, nil
}

// GetRDSRates retrieves complete RDS pricing information
func (pf *PricingFetcher) GetRDSRates(region, instanceClass, engine string) (*RDSRates, error) {
	instanceRate, err := pf.GetRDSInstanceRates(region, instanceClass, engine)
	if err != nil {
		return nil, fmt.Errorf("failed to get RDS instance rates: %w", err)
	}

	storageRate, err := pf.GetRDSStorageRates(region, engine)
	if err != nil {
		return nil, fmt.Errorf("failed to get RDS storage rates: %w", err)
	}

	return &RDSRates{
		InstanceHourlyRate: instanceRate,
		StorageMonthlyRate: storageRate,
		InstanceClass:      instanceClass,
		Engine:             engine,
		DeploymentOption:   "Single-AZ",
	}, nil
}