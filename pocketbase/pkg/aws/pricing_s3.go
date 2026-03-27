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

// S3Rates represents S3 pricing structure
type S3Rates struct {
	StoragePricePerGB    float64 `json:"storagePricePerGB"`
	RequestPricePer1000  float64 `json:"requestPricePer1000"`
	StorageClass         string  `json:"storageClass"`
	RequestType          string  `json:"requestType"`
}

// fetchS3Rates fetches S3 pricing data for all regions
func (pf *PricingFetcher) fetchS3Rates() error {
	log.Println("Fetching S3 pricing data...")

	for _, region := range pf.regions {
		log.Printf("Fetching S3 rates for region: %s", region)

		// Fetch storage pricing (Standard storage class)
		storageRates, err := pf.fetchS3StorageRates(region)
		if err != nil {
			log.Printf("Failed to fetch S3 storage rates for %s: %v", region, err)
			continue
		}

		// Fetch request pricing (PUT/POST requests)
		putRequestRates, err := pf.fetchS3RequestRates(region, "PUT")
		if err != nil {
			log.Printf("Failed to fetch S3 PUT request rates for %s: %v", region, err)
			continue
		}

		// Fetch GET request pricing
		getRequestRates, err := pf.fetchS3RequestRates(region, "GET")
		if err != nil {
			log.Printf("Failed to fetch S3 GET request rates for %s: %v", region, err)
			continue
		}

		// Save all S3 rates
		if err := pf.saveS3StorageRates(region, storageRates); err != nil {
			log.Printf("Failed to save S3 storage rates for %s: %v", region, err)
			continue
		}

		if err := pf.saveS3RequestRates(region, "PUT", putRequestRates); err != nil {
			log.Printf("Failed to save S3 PUT request rates for %s: %v", region, err)
			continue
		}

		if err := pf.saveS3RequestRates(region, "GET", getRequestRates); err != nil {
			log.Printf("Failed to save S3 GET request rates for %s: %v", region, err)
			continue
		}

		log.Printf("Successfully saved S3 rates for %s", region)
	}

	return nil
}

// fetchS3StorageRates fetches S3 storage pricing
func (pf *PricingFetcher) fetchS3StorageRates(region string) (float64, error) {
	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonS3"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("location"),
				Value: aws.String(pf.getLocationName(region)),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("storageClass"),
				Value: aws.String("General Purpose"),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("volumeType"),
				Value: aws.String("Standard"),
			},
		},
		MaxResults: aws.Int32(10),
	}

	result, err := pf.client.GetProducts(context.TODO(), input)
	if err != nil {
		return 0, fmt.Errorf("failed to get S3 storage products: %w", err)
	}

	if len(result.PriceList) == 0 {
		return 0, fmt.Errorf("no S3 storage pricing found for region %s", region)
	}

	// Parse the first result
	return pf.parseS3Pricing(result.PriceList[0])
}

// fetchS3RequestRates fetches S3 request pricing
func (pf *PricingFetcher) fetchS3RequestRates(region, requestType string) (float64, error) {
	var groupFilter string
	switch requestType {
	case "PUT":
		groupFilter = "S3-API-Class1"
	case "GET":
		groupFilter = "S3-API-Class2"
	default:
		return 0, fmt.Errorf("unsupported request type: %s", requestType)
	}

	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonS3"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("location"),
				Value: aws.String(pf.getLocationName(region)),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("group"),
				Value: aws.String(groupFilter),
			},
		},
		MaxResults: aws.Int32(10),
	}

	result, err := pf.client.GetProducts(context.TODO(), input)
	if err != nil {
		return 0, fmt.Errorf("failed to get S3 request products: %w", err)
	}

	if len(result.PriceList) == 0 {
		return 0, fmt.Errorf("no S3 %s request pricing found for region %s", requestType, region)
	}

	// Parse the first result
	return pf.parseS3Pricing(result.PriceList[0])
}

// parseS3Pricing parses S3 pricing from AWS pricing API response
func (pf *PricingFetcher) parseS3Pricing(productJSON string) (float64, error) {
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

// saveS3StorageRates saves S3 storage pricing to database
func (pf *PricingFetcher) saveS3StorageRates(region string, pricePerGB float64) error {
	now := time.Now()
	validUntil := now.Add(24 * time.Hour)

	data := PricingData{
		Region:        region,
		Service:       "AmazonS3",
		ProductFamily: "Storage",
		InstanceType:  "standard-storage",
		VCPU:          0,
		Memory:        0,
		PricePerHour:  pricePerGB / (24 * 30), // Convert monthly to hourly
		PricePerMonth: pricePerGB * 30,        // Assume 30 days per month
		Currency:      "USD",
		FetchedAt:     now,
		ValidUntil:    validUntil,
	}

	return pf.savePricingData(data)
}

// saveS3RequestRates saves S3 request pricing to database
func (pf *PricingFetcher) saveS3RequestRates(region, requestType string, pricePer1000 float64) error {
	now := time.Now()
	validUntil := now.Add(24 * time.Hour)

	instanceType := fmt.Sprintf("requests-%s", requestType)

	data := PricingData{
		Region:        region,
		Service:       "AmazonS3",
		ProductFamily: "API Request",
		InstanceType:  instanceType,
		VCPU:          0,
		Memory:        0,
		PricePerHour:  pricePer1000 / 1000,                    // Price per single request
		PricePerMonth: pricePer1000 * 10,                      // Assume 10K requests per month
		Currency:      "USD",
		FetchedAt:     now,
		ValidUntil:    validUntil,
	}

	return pf.savePricingData(data)
}

// GetS3StorageRates retrieves cached S3 storage pricing
func (pf *PricingFetcher) GetS3StorageRates(region string) (float64, error) {
	data, err := pf.GetCachedPricing(region, "AmazonS3", "Storage", "standard-storage")
	if err != nil {
		return 0, fmt.Errorf("failed to get S3 storage rates: %w", err)
	}

	return data.PricePerMonth / 30, nil // Return price per GB per month
}

// GetS3RequestRates retrieves cached S3 request pricing
func (pf *PricingFetcher) GetS3RequestRates(region, requestType string) (float64, error) {
	instanceType := fmt.Sprintf("requests-%s", requestType)
	data, err := pf.GetCachedPricing(region, "AmazonS3", "API Request", instanceType)
	if err != nil {
		return 0, fmt.Errorf("failed to get S3 %s request rates: %w", requestType, err)
	}

	return data.PricePerHour * 1000, nil // Return price per 1000 requests
}

// CalculateS3Cost calculates S3 cost for given storage and requests
func (pf *PricingFetcher) CalculateS3Cost(region string, storageGB float64, putRequests, getRequests int64) (float64, error) {
	// Get storage rates
	storageRate, err := pf.GetS3StorageRates(region)
	if err != nil {
		return 0, fmt.Errorf("failed to get S3 storage rates: %w", err)
	}

	// Get request rates
	putRate, err := pf.GetS3RequestRates(region, "PUT")
	if err != nil {
		return 0, fmt.Errorf("failed to get S3 PUT request rates: %w", err)
	}

	getRate, err := pf.GetS3RequestRates(region, "GET")
	if err != nil {
		return 0, fmt.Errorf("failed to get S3 GET request rates: %w", err)
	}

	// Calculate costs
	storageCost := storageGB * storageRate
	putCost := float64(putRequests) / 1000 * putRate
	getCost := float64(getRequests) / 1000 * getRate

	return storageCost + putCost + getCost, nil
}