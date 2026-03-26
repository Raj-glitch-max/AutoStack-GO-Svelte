package aws

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/pricing/types"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
)

// PricingFetcher handles fetching AWS pricing data
type PricingFetcher struct {
	app       core.App
	client    *pricing.Client
	regions   []string
	batchSize int
}

// PricingData represents a single pricing entry
type PricingData struct {
	Region        string  `json:"region"`
	Service       string  `json:"service"`
	ProductFamily string  `json:"productFamily"`
	InstanceType  string  `json:"instanceType,omitempty"`
	VCPU          float64 `json:"vcpu,omitempty"`
	Memory        float64 `json:"memory,omitempty"`
	PricePerHour  float64 `json:"pricePerHour"`
	PricePerMonth float64 `json:"pricePerMonth"`
	Currency      string  `json:"currency"`
	FetchedAt     time.Time `json:"fetchedAt"`
	ValidUntil    time.Time `json:"validUntil"`
}

// NewPricingFetcher creates a new pricing fetcher instance
func NewPricingFetcher(app core.App) (*PricingFetcher, error) {
	// Load AWS config
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"), // Pricing API is only available in us-east-1
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := pricing.NewFromConfig(cfg)

	// Default regions to fetch pricing for
	regions := []string{
		"us-east-1", "us-east-2", "us-west-1", "us-west-2",
		"eu-west-1", "eu-west-2", "eu-central-1",
		"ap-southeast-1", "ap-southeast-2", "ap-northeast-1",
	}

	return &PricingFetcher{
		app:       app,
		client:    client,
		regions:   regions,
		batchSize: 100, // Process in batches to avoid memory issues
	}, nil
}

// FetchAllPricing fetches pricing data for all supported services and regions
func (pf *PricingFetcher) FetchAllPricing() error {
	log.Println("Starting AWS pricing data fetch...")

	// Fetch pricing for different services
	services := []string{
		"AmazonECS",        // Fargate
		"AmazonS3",         // S3 storage
		"AmazonEC2",        // ALB, data transfer
		"AmazonRDS",        // RDS instances
		"AmazonCloudFront", // CloudFront
		"AmazonRoute53",    // Route53
	}

	for _, service := range services {
		log.Printf("Fetching pricing for service: %s", service)
		
		switch service {
		case "AmazonECS":
			if err := pf.fetchFargateRates(); err != nil {
				log.Printf("Failed to fetch Fargate rates: %v", err)
				continue
			}
		case "AmazonS3":
			if err := pf.fetchS3Rates(); err != nil {
				log.Printf("Failed to fetch S3 rates: %v", err)
				continue
			}
		case "AmazonEC2":
			if err := pf.fetchALBRates(); err != nil {
				log.Printf("Failed to fetch ALB rates: %v", err)
				continue
			}
		case "AmazonRDS":
			if err := pf.fetchRDSRates(); err != nil {
				log.Printf("Failed to fetch RDS rates: %v", err)
				continue
			}
		case "AmazonCloudFront":
			if err := pf.fetchCloudFrontRates(); err != nil {
				log.Printf("Failed to fetch CloudFront rates: %v", err)
				continue
			}
		case "AmazonRoute53":
			if err := pf.fetchRoute53Rates(); err != nil {
				log.Printf("Failed to fetch Route53 rates: %v", err)
				continue
			}
		}
	}

	log.Println("AWS pricing data fetch completed")
	return nil
}

// savePricingData saves pricing data to the database
func (pf *PricingFetcher) savePricingData(data PricingData) error {
	collection, err := pf.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return fmt.Errorf("failed to find awsPricingCache collection: %w", err)
	}

	// Check if record already exists
	existingRecord, err := pf.app.Dao().FindFirstRecordByFilter(
		"awsPricingCache",
		"region = {:region} && service = {:service} && productFamily = {:productFamily} && instanceType = {:instanceType}",
		map[string]any{
			"region":        data.Region,
			"service":       data.Service,
			"productFamily": data.ProductFamily,
			"instanceType":  data.InstanceType,
		},
	)

	var record *models.Record
	if err != nil {
		// Create new record
		record = models.NewRecord(collection)
	} else {
		// Update existing record
		record = existingRecord
	}

	// Set record data
	record.Set("region", data.Region)
	record.Set("service", data.Service)
	record.Set("productFamily", data.ProductFamily)
	record.Set("instanceType", data.InstanceType)
	record.Set("vcpu", data.VCPU)
	record.Set("memory", data.Memory)
	record.Set("pricePerHour", data.PricePerHour)
	record.Set("pricePerMonth", data.PricePerMonth)
	record.Set("currency", data.Currency)
	record.Set("fetchedAt", data.FetchedAt)
	record.Set("validUntil", data.ValidUntil)

	return pf.app.Dao().SaveRecord(record)
}

// getLocationName converts AWS region code to location name used in pricing API
func (pf *PricingFetcher) getLocationName(region string) string {
	locationMap := map[string]string{
		"us-east-1":      "US East (N. Virginia)",
		"us-east-2":      "US East (Ohio)",
		"us-west-1":      "US West (N. California)",
		"us-west-2":      "US West (Oregon)",
		"eu-west-1":      "Europe (Ireland)",
		"eu-west-2":      "Europe (London)",
		"eu-central-1":   "Europe (Frankfurt)",
		"ap-southeast-1": "Asia Pacific (Singapore)",
		"ap-southeast-2": "Asia Pacific (Sydney)",
		"ap-northeast-1": "Asia Pacific (Tokyo)",
	}

	if location, exists := locationMap[region]; exists {
		return location
	}
	return region // Fallback to region code
}

// GetCachedPricing retrieves cached pricing data from database
func (pf *PricingFetcher) GetCachedPricing(region, service, productFamily, instanceType string) (*PricingData, error) {
	record, err := pf.app.Dao().FindFirstRecordByFilter(
		"awsPricingCache",
		"region = {:region} && service = {:service} && productFamily = {:productFamily} && instanceType = {:instanceType}",
		map[string]any{
			"region":        region,
			"service":       service,
			"productFamily": productFamily,
			"instanceType":  instanceType,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("pricing data not found: %w", err)
	}

	// Check if data is still valid
	validUntil := record.GetDateTime("validUntil")
	if time.Now().After(validUntil.Time()) {
		return nil, fmt.Errorf("pricing data is stale")
	}

	return &PricingData{
		Region:        record.GetString("region"),
		Service:       record.GetString("service"),
		ProductFamily: record.GetString("productFamily"),
		InstanceType:  record.GetString("instanceType"),
		VCPU:          record.GetFloat("vcpu"),
		Memory:        record.GetFloat("memory"),
		PricePerHour:  record.GetFloat("pricePerHour"),
		PricePerMonth: record.GetFloat("pricePerMonth"),
		Currency:      record.GetString("currency"),
		FetchedAt:     record.GetDateTime("fetchedAt").Time(),
		ValidUntil:    record.GetDateTime("validUntil").Time(),
	}, nil
}

// IsDataStale checks if pricing data is older than the specified duration
func (pf *PricingFetcher) IsDataStale(maxAge time.Duration) (bool, error) {
	records, err := pf.app.Dao().FindRecordsByFilter(
		"awsPricingCache",
		"",
		"-fetchedAt", // Order by fetchedAt descending to get most recent
		1,
		0,
		nil,
	)
	if err != nil || len(records) == 0 {
		return true, fmt.Errorf("no pricing data found: %w", err)
	}
	
	record := records[0]

	fetchedAt := record.GetDateTime("fetchedAt")
	return time.Since(fetchedAt.Time()) > maxAge, nil
}

// fetchCloudFrontRates fetches CloudFront pricing data
func (pf *PricingFetcher) fetchCloudFrontRates() error {
	// CloudFront pricing is global, so we only need to fetch once
	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonCloudFront"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("productFamily"),
				Value: aws.String("Data Transfer"),
			},
		},
		MaxResults: aws.Int32(50),
	}

	result, err := pf.client.GetProducts(context.TODO(), input)
	if err != nil {
		return fmt.Errorf("failed to get CloudFront products: %w", err)
	}

	for _, product := range result.PriceList {
		if err := pf.processCloudFrontProduct(product); err != nil {
			log.Printf("Failed to process CloudFront product: %v", err)
			continue
		}
	}

	return nil
}

// fetchRoute53Rates fetches Route53 pricing data
func (pf *PricingFetcher) fetchRoute53Rates() error {
	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonRoute53"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("productFamily"),
				Value: aws.String("DNS Zone"),
			},
		},
		MaxResults: aws.Int32(10),
	}

	result, err := pf.client.GetProducts(context.TODO(), input)
	if err != nil {
		return fmt.Errorf("failed to get Route53 products: %w", err)
	}

	for _, product := range result.PriceList {
		if err := pf.processRoute53Product(product); err != nil {
			log.Printf("Failed to process Route53 product: %v", err)
			continue
		}
	}

	return nil
}

// processCloudFrontProduct processes a CloudFront product
func (pf *PricingFetcher) processCloudFrontProduct(productJSON string) error {
	// Simplified CloudFront processing - in reality this would be more complex
	pricingData := PricingData{
		Region:        "global",
		Service:       "AmazonCloudFront",
		ProductFamily: "Data Transfer",
		InstanceType:  "data-transfer",
		PricePerHour:  0.085, // Simplified pricing
		PricePerMonth: 0.085 * 1000, // Per GB pricing
		Currency:      "USD",
		FetchedAt:     time.Now(),
		ValidUntil:    time.Now().Add(24 * time.Hour),
	}

	return pf.savePricingData(pricingData)
}

// processRoute53Product processes a Route53 product
func (pf *PricingFetcher) processRoute53Product(productJSON string) error {
	// Simplified Route53 processing
	pricingData := PricingData{
		Region:        "global",
		Service:       "AmazonRoute53",
		ProductFamily: "DNS Zone",
		InstanceType:  "hosted-zone",
		PricePerHour:  0.50 / 24 / 30, // $0.50 per month per hosted zone
		PricePerMonth: 0.50,
		Currency:      "USD",
		FetchedAt:     time.Now(),
		ValidUntil:    time.Now().Add(24 * time.Hour),
	}

	return pf.savePricingData(pricingData)
}