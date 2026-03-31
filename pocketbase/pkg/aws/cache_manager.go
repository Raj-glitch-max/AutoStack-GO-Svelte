package aws

import (
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// CacheManager handles cache invalidation and refresh operations
type CacheManager struct {
	app           core.App
	pricingFetcher *PricingFetcher
	pricingCache  *PricingCache
}

// NewCacheManager creates a new cache manager instance
func NewCacheManager(app core.App) (*CacheManager, error) {
	pricingFetcher, err := NewPricingFetcher(app)
	if err != nil {
		return nil, fmt.Errorf("failed to create pricing fetcher: %w", err)
	}

	return &CacheManager{
		app:           app,
		pricingFetcher: pricingFetcher,
		pricingCache:  NewPricingCache(app),
	}, nil
}

// InvalidateAll invalidates all cached pricing data
func (cm *CacheManager) InvalidateAll() error {
	log.Println("Invalidating all pricing cache data...")

	collection, err := cm.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return fmt.Errorf("failed to find awsPricingCache collection: %w", err)
	}

	records, err := cm.app.Dao().FindRecordsByFilter(
		collection.Id,
		"",
		"",
		0,
		0,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to retrieve cache records: %w", err)
	}

	deletedCount := 0
	for _, record := range records {
		if err := cm.app.Dao().DeleteRecord(record); err != nil {
			log.Printf("Failed to delete record %s: %v", record.Id, err)
			continue
		}
		deletedCount++
	}

	log.Printf("Invalidated %d pricing cache records", deletedCount)
	return nil
}

// InvalidateByRegion invalidates cached pricing data for a specific region
func (cm *CacheManager) InvalidateByRegion(region string) error {
	log.Printf("Invalidating pricing cache for region: %s", region)

	collection, err := cm.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return fmt.Errorf("failed to find awsPricingCache collection: %w", err)
	}

	records, err := cm.app.Dao().FindRecordsByFilter(
		collection.Id,
		"region = {:region}",
		"",
		0,
		0,
		map[string]any{
			"region": region,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to retrieve cache records for region %s: %w", region, err)
	}

	deletedCount := 0
	for _, record := range records {
		if err := cm.app.Dao().DeleteRecord(record); err != nil {
			log.Printf("Failed to delete record %s: %v", record.Id, err)
			continue
		}
		deletedCount++
	}

	log.Printf("Invalidated %d pricing cache records for region %s", deletedCount, region)
	return nil
}

// InvalidateByService invalidates cached pricing data for a specific service
func (cm *CacheManager) InvalidateByService(service string) error {
	log.Printf("Invalidating pricing cache for service: %s", service)

	collection, err := cm.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return fmt.Errorf("failed to find awsPricingCache collection: %w", err)
	}

	records, err := cm.app.Dao().FindRecordsByFilter(
		collection.Id,
		"service = {:service}",
		"",
		0,
		0,
		map[string]any{
			"service": service,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to retrieve cache records for service %s: %w", service, err)
	}

	deletedCount := 0
	for _, record := range records {
		if err := cm.app.Dao().DeleteRecord(record); err != nil {
			log.Printf("Failed to delete record %s: %v", record.Id, err)
			continue
		}
		deletedCount++
	}

	log.Printf("Invalidated %d pricing cache records for service %s", deletedCount, service)
	return nil
}

// InvalidateStale invalidates expired pricing data
func (cm *CacheManager) InvalidateStale() error {
	log.Println("Invalidating stale pricing cache data...")

	collection, err := cm.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return fmt.Errorf("failed to find awsPricingCache collection: %w", err)
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	
	records, err := cm.app.Dao().FindRecordsByFilter(
		collection.Id,
		"validUntil < {:now}",
		"",
		0,
		0,
		map[string]any{
			"now": now,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to retrieve stale cache records: %w", err)
	}

	deletedCount := 0
	for _, record := range records {
		if err := cm.app.Dao().DeleteRecord(record); err != nil {
			log.Printf("Failed to delete record %s: %v", record.Id, err)
			continue
		}
		deletedCount++
	}

	log.Printf("Invalidated %d stale pricing cache records", deletedCount)
	return nil
}

// RefreshAll refreshes all pricing data
func (cm *CacheManager) RefreshAll() error {
	log.Println("Refreshing all pricing data...")

	// Invalidate existing data
	if err := cm.InvalidateAll(); err != nil {
		log.Printf("Warning: Failed to invalidate cache before refresh: %v", err)
		// Continue with refresh anyway
	}

	// Fetch new pricing data
	if err := cm.pricingFetcher.FetchAllPricing(); err != nil {
		return fmt.Errorf("failed to fetch pricing data: %w", err)
	}

	log.Println("Successfully refreshed all pricing data")
	return nil
}

// RefreshByRegion refreshes pricing data for a specific region
func (cm *CacheManager) RefreshByRegion(region string) error {
	log.Printf("Refreshing pricing data for region: %s", region)

	// Invalidate existing data for this region
	if err := cm.InvalidateByRegion(region); err != nil {
		log.Printf("Warning: Failed to invalidate cache for region %s: %v", region, err)
	}

	// Fetch new pricing data for this region
	// Note: This would require modifying PricingFetcher to support region-specific fetching
	// For now, we'll fetch all and filter
	if err := cm.pricingFetcher.FetchAllPricing(); err != nil {
		return fmt.Errorf("failed to fetch pricing data for region %s: %w", region, err)
	}

	log.Printf("Successfully refreshed pricing data for region %s", region)
	return nil
}

// RefreshByService refreshes pricing data for a specific service
func (cm *CacheManager) RefreshByService(service string) error {
	log.Printf("Refreshing pricing data for service: %s", service)

	// Invalidate existing data for this service
	if err := cm.InvalidateByService(service); err != nil {
		log.Printf("Warning: Failed to invalidate cache for service %s: %v", service, err)
	}

	// Fetch new pricing data for this service
	// Note: This would require modifying PricingFetcher to support service-specific fetching
	if err := cm.pricingFetcher.FetchAllPricing(); err != nil {
		return fmt.Errorf("failed to fetch pricing data for service %s: %w", service, err)
	}

	log.Printf("Successfully refreshed pricing data for service %s", service)
	return nil
}

// CleanupOldData removes pricing data older than the specified duration
func (cm *CacheManager) CleanupOldData(maxAge time.Duration) error {
	log.Printf("Cleaning up pricing data older than %v", maxAge)

	collection, err := cm.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return fmt.Errorf("failed to find awsPricingCache collection: %w", err)
	}

	cutoffTime := time.Now().Add(-maxAge).Format("2006-01-02 15:04:05")
	
	records, err := cm.app.Dao().FindRecordsByFilter(
		collection.Id,
		"fetchedAt < {:cutoff}",
		"",
		0,
		0,
		map[string]any{
			"cutoff": cutoffTime,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to retrieve old cache records: %w", err)
	}

	deletedCount := 0
	for _, record := range records {
		if err := cm.app.Dao().DeleteRecord(record); err != nil {
			log.Printf("Failed to delete record %s: %v", record.Id, err)
			continue
		}
		deletedCount++
	}

	log.Printf("Cleaned up %d old pricing cache records", deletedCount)
	return nil
}

// ValidateCache checks the integrity of the pricing cache
func (cm *CacheManager) ValidateCache() (*CacheValidationResult, error) {
	log.Println("Validating pricing cache...")

	stats, err := cm.pricingCache.GetCacheStatistics()
	if err != nil {
		return nil, fmt.Errorf("failed to get cache statistics: %w", err)
	}

	result := &CacheValidationResult{
		IsValid:        true,
		Issues:         []string{},
		TotalRecords:   stats.TotalRecords,
		FreshRecords:   stats.FreshRecords,
		StaleRecords:   stats.StaleRecords,
		ExpiredRecords: stats.ExpiredRecords,
		ValidatedAt:    time.Now(),
	}

	// Check for expired records
	if stats.ExpiredRecords > 0 {
		result.Issues = append(result.Issues, fmt.Sprintf("%d expired records found", stats.ExpiredRecords))
		result.IsValid = false
	}

	// Check for stale records (>24 hours old)
	if stats.StaleRecords > stats.TotalRecords/2 {
		result.Issues = append(result.Issues, fmt.Sprintf("More than 50%% of records are stale (%d/%d)", stats.StaleRecords, stats.TotalRecords))
		result.IsValid = false
	}

	// Check if cache is empty
	if stats.TotalRecords == 0 {
		result.Issues = append(result.Issues, "Cache is empty")
		result.IsValid = false
	}

	// Check for missing regions
	expectedRegions := []string{
		"us-east-1", "us-east-2", "us-west-1", "us-west-2",
		"eu-west-1", "eu-west-2", "eu-central-1",
		"ap-southeast-1", "ap-southeast-2", "ap-northeast-1",
	}
	
	for _, region := range expectedRegions {
		if count, exists := stats.ByRegion[region]; !exists || count == 0 {
			result.Issues = append(result.Issues, fmt.Sprintf("No pricing data for region: %s", region))
			result.IsValid = false
		}
	}

	// Check for missing services
	expectedServices := []string{
		"AmazonECS", "AmazonS3", "AmazonEC2", "AmazonRDS",
		"AmazonCloudFront", "AmazonRoute53",
	}
	
	for _, service := range expectedServices {
		if count, exists := stats.ByService[service]; !exists || count == 0 {
			result.Issues = append(result.Issues, fmt.Sprintf("No pricing data for service: %s", service))
			result.IsValid = false
		}
	}

	if result.IsValid {
		log.Println("Cache validation passed")
	} else {
		log.Printf("Cache validation failed with %d issues", len(result.Issues))
	}

	return result, nil
}

// CacheValidationResult represents the result of cache validation
type CacheValidationResult struct {
	IsValid        bool      `json:"isValid"`
	Issues         []string  `json:"issues"`
	TotalRecords   int       `json:"totalRecords"`
	FreshRecords   int       `json:"freshRecords"`
	StaleRecords   int       `json:"staleRecords"`
	ExpiredRecords int       `json:"expiredRecords"`
	ValidatedAt    time.Time `json:"validatedAt"`
}

// GetCacheHealth returns the overall health status of the cache
func (cm *CacheManager) GetCacheHealth() (*CacheHealthStatus, error) {
	validation, err := cm.ValidateCache()
	if err != nil {
		return nil, fmt.Errorf("failed to validate cache: %w", err)
	}

	stats, err := cm.pricingCache.GetCacheStatistics()
	if err != nil {
		return nil, fmt.Errorf("failed to get cache statistics: %w", err)
	}

	health := &CacheHealthStatus{
		Status:         "healthy",
		LastValidation: validation.ValidatedAt,
		TotalRecords:   stats.TotalRecords,
		FreshRecords:   stats.FreshRecords,
		StaleRecords:   stats.StaleRecords,
		ExpiredRecords: stats.ExpiredRecords,
		Issues:         validation.Issues,
		OldestRecord:   stats.OldestRecord,
		NewestRecord:   stats.NewestRecord,
	}

	// Determine overall health status
	if !validation.IsValid {
		health.Status = "unhealthy"
	} else if stats.StaleRecords > stats.TotalRecords/4 {
		health.Status = "degraded"
	}

	return health, nil
}

// CacheHealthStatus represents the health status of the cache
type CacheHealthStatus struct {
	Status         string    `json:"status"` // healthy, degraded, unhealthy
	LastValidation time.Time `json:"lastValidation"`
	TotalRecords   int       `json:"totalRecords"`
	FreshRecords   int       `json:"freshRecords"`
	StaleRecords   int       `json:"staleRecords"`
	ExpiredRecords int       `json:"expiredRecords"`
	Issues         []string  `json:"issues"`
	OldestRecord   time.Time `json:"oldestRecord"`
	NewestRecord   time.Time `json:"newestRecord"`
}
