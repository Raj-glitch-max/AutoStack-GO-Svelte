package aws

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
)

// PricingCache provides methods to retrieve cached pricing data
type PricingCache struct {
	app core.App
}

// NewPricingCache creates a new pricing cache instance
func NewPricingCache(app core.App) *PricingCache {
	return &PricingCache{
		app: app,
	}
}

// GetPricingByRegion retrieves all pricing data for a specific region
func (pc *PricingCache) GetPricingByRegion(region string) ([]*PricingData, error) {
	collection, err := pc.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return nil, fmt.Errorf("failed to find awsPricingCache collection: %w", err)
	}

	records, err := pc.app.Dao().FindRecordsByFilter(
		collection.Id,
		"region = {:region}",
		"-fetchedAt",
		0,
		0,
		map[string]any{
			"region": region,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve pricing data for region %s: %w", region, err)
	}

	return pc.recordsToPricingData(records), nil
}

// GetPricingByService retrieves all pricing data for a specific service
func (pc *PricingCache) GetPricingByService(service string) ([]*PricingData, error) {
	collection, err := pc.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return nil, fmt.Errorf("failed to find awsPricingCache collection: %w", err)
	}

	records, err := pc.app.Dao().FindRecordsByFilter(
		collection.Id,
		"service = {:service}",
		"-fetchedAt",
		0,
		0,
		map[string]any{
			"service": service,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve pricing data for service %s: %w", service, err)
	}

	return pc.recordsToPricingData(records), nil
}

// GetPricingByRegionAndService retrieves pricing data for a specific region and service
func (pc *PricingCache) GetPricingByRegionAndService(region, service string) ([]*PricingData, error) {
	collection, err := pc.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return nil, fmt.Errorf("failed to find awsPricingCache collection: %w", err)
	}

	records, err := pc.app.Dao().FindRecordsByFilter(
		collection.Id,
		"region = {:region} && service = {:service}",
		"-fetchedAt",
		0,
		0,
		map[string]any{
			"region":  region,
			"service": service,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve pricing data for region %s and service %s: %w", region, service, err)
	}

	return pc.recordsToPricingData(records), nil
}

// GetSpecificPricing retrieves a specific pricing entry
func (pc *PricingCache) GetSpecificPricing(region, service, productFamily, instanceType string) (*PricingData, error) {
	record, err := pc.app.Dao().FindFirstRecordByFilter(
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
		return nil, fmt.Errorf("pricing data is stale (expired at %v)", validUntil.Time())
	}

	return pc.recordToPricingData(record), nil
}

// GetFreshPricing retrieves only fresh pricing data (not expired)
func (pc *PricingCache) GetFreshPricing(region, service string) ([]*PricingData, error) {
	collection, err := pc.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return nil, fmt.Errorf("failed to find awsPricingCache collection: %w", err)
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	
	records, err := pc.app.Dao().FindRecordsByFilter(
		collection.Id,
		"region = {:region} && service = {:service} && validUntil > {:now}",
		"-fetchedAt",
		0,
		0,
		map[string]any{
			"region":  region,
			"service": service,
			"now":     now,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve fresh pricing data: %w", err)
	}

	return pc.recordsToPricingData(records), nil
}

// GetAllRegions returns a list of all regions with cached pricing data
func (pc *PricingCache) GetAllRegions() ([]string, error) {
	collection, err := pc.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return nil, fmt.Errorf("failed to find awsPricingCache collection: %w", err)
	}

	records, err := pc.app.Dao().FindRecordsByFilter(
		collection.Id,
		"",
		"region",
		0,
		0,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve regions: %w", err)
	}

	// Extract unique regions
	regionMap := make(map[string]bool)
	for _, record := range records {
		region := record.GetString("region")
		if region != "" {
			regionMap[region] = true
		}
	}

	regions := make([]string, 0, len(regionMap))
	for region := range regionMap {
		regions = append(regions, region)
	}

	return regions, nil
}

// GetAllServices returns a list of all services with cached pricing data
func (pc *PricingCache) GetAllServices() ([]string, error) {
	collection, err := pc.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return nil, fmt.Errorf("failed to find awsPricingCache collection: %w", err)
	}

	records, err := pc.app.Dao().FindRecordsByFilter(
		collection.Id,
		"",
		"service",
		0,
		0,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve services: %w", err)
	}

	// Extract unique services
	serviceMap := make(map[string]bool)
	for _, record := range records {
		service := record.GetString("service")
		if service != "" {
			serviceMap[service] = true
		}
	}

	services := make([]string, 0, len(serviceMap))
	for service := range serviceMap {
		services = append(services, service)
	}

	return services, nil
}

// GetCacheStatistics returns statistics about the pricing cache
func (pc *PricingCache) GetCacheStatistics() (*CacheStatistics, error) {
	collection, err := pc.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return nil, fmt.Errorf("failed to find awsPricingCache collection: %w", err)
	}

	records, err := pc.app.Dao().FindRecordsByFilter(
		collection.Id,
		"",
		"",
		0,
		0,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve cache records: %w", err)
	}

	stats := &CacheStatistics{
		TotalRecords:  len(records),
		FreshRecords:  0,
		StaleRecords:  0,
		ExpiredRecords: 0,
		ByRegion:      make(map[string]int),
		ByService:     make(map[string]int),
	}

	now := time.Now()
	for _, record := range records {
		validUntil := record.GetDateTime("validUntil").Time()
		fetchedAt := record.GetDateTime("fetchedAt").Time()
		
		// Categorize by freshness
		if now.After(validUntil) {
			stats.ExpiredRecords++
		} else if now.Sub(fetchedAt) > 24*time.Hour {
			stats.StaleRecords++
		} else {
			stats.FreshRecords++
		}

		// Count by region
		region := record.GetString("region")
		stats.ByRegion[region]++

		// Count by service
		service := record.GetString("service")
		stats.ByService[service]++
	}

	// Get oldest and newest records
	if len(records) > 0 {
		oldestRecord := records[0]
		newestRecord := records[0]
		
		for _, record := range records {
			fetchedAt := record.GetDateTime("fetchedAt").Time()
			
			if fetchedAt.Before(oldestRecord.GetDateTime("fetchedAt").Time()) {
				oldestRecord = record
			}
			if fetchedAt.After(newestRecord.GetDateTime("fetchedAt").Time()) {
				newestRecord = record
			}
		}
		
		stats.OldestRecord = oldestRecord.GetDateTime("fetchedAt").Time()
		stats.NewestRecord = newestRecord.GetDateTime("fetchedAt").Time()
	}

	return stats, nil
}

// CacheStatistics represents statistics about the pricing cache
type CacheStatistics struct {
	TotalRecords   int            `json:"totalRecords"`
	FreshRecords   int            `json:"freshRecords"`
	StaleRecords   int            `json:"staleRecords"`
	ExpiredRecords int            `json:"expiredRecords"`
	ByRegion       map[string]int `json:"byRegion"`
	ByService      map[string]int `json:"byService"`
	OldestRecord   time.Time      `json:"oldestRecord"`
	NewestRecord   time.Time      `json:"newestRecord"`
}

// Helper methods

func (pc *PricingCache) recordToPricingData(record *models.Record) *PricingData {
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
	}
}

func (pc *PricingCache) recordsToPricingData(records []*models.Record) []*PricingData {
	result := make([]*PricingData, 0, len(records))
	for _, record := range records {
		result = append(result, pc.recordToPricingData(record))
	}
	return result
}
