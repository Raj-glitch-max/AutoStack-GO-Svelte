package aws

import (
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// StaleDataHandler manages handling of stale pricing data
type StaleDataHandler struct {
	app                core.App
	maxStaleAge        time.Duration
	warningAge         time.Duration
	fallbackRegion     string
	enableFallback     bool
}

// StaleDataConfig configures stale data handling behavior
type StaleDataConfig struct {
	MaxStaleAge        time.Duration `json:"maxStaleAge"`        // Maximum age before data is considered unusable
	WarningAge         time.Duration `json:"warningAge"`         // Age at which to show warnings
	FallbackRegion     string        `json:"fallbackRegion"`     // Region to use as fallback
	EnableFallback     bool          `json:"enableFallback"`     // Whether to use fallback data
	ShowWarnings       bool          `json:"showWarnings"`       // Whether to show stale data warnings
}

// DataFreshness represents the freshness status of pricing data
type DataFreshness struct {
	IsFresh            bool          `json:"isFresh"`
	IsStale            bool          `json:"isStale"`
	IsCriticallyStale  bool          `json:"isCriticallyStale"`
	Age                time.Duration `json:"age"`
	LastFetched        time.Time     `json:"lastFetched"`
	WarningMessage     string        `json:"warningMessage"`
	ShouldShowWarning  bool          `json:"shouldShowWarning"`
}

// NewStaleDataHandler creates a new stale data handler
func NewStaleDataHandler(app core.App) *StaleDataHandler {
	return &StaleDataHandler{
		app:            app,
		maxStaleAge:    48 * time.Hour,  // Data unusable after 48 hours
		warningAge:     24 * time.Hour,  // Show warnings after 24 hours
		fallbackRegion: "us-east-1",     // Default fallback region
		enableFallback: true,            // Enable fallback by default
	}
}

// NewStaleDataHandlerWithConfig creates a handler with custom configuration
func NewStaleDataHandlerWithConfig(app core.App, config StaleDataConfig) *StaleDataHandler {
	return &StaleDataHandler{
		app:            app,
		maxStaleAge:    config.MaxStaleAge,
		warningAge:     config.WarningAge,
		fallbackRegion: config.FallbackRegion,
		enableFallback: config.EnableFallback,
	}
}

// CheckDataFreshness checks the freshness of pricing data for a region/service
func (sdh *StaleDataHandler) CheckDataFreshness(region, service, productFamily, instanceType string) (*DataFreshness, error) {
	// Get the most recent pricing data for the specified criteria
	records, err := sdh.app.Dao().FindRecordsByFilter(
		"awsPricingCache",
		"region = {:region} && service = {:service} && productFamily = {:productFamily} && instanceType = {:instanceType}",
		"-fetchedAt", // Order by fetchedAt descending
		1,
		0,
		map[string]any{
			"region":        region,
			"service":       service,
			"productFamily": productFamily,
			"instanceType":  instanceType,
		},
	)
	
	if err != nil || len(records) == 0 {
		return &DataFreshness{
			IsFresh:           false,
			IsStale:           true,
			IsCriticallyStale: true,
			Age:               time.Duration(0),
			LastFetched:       time.Time{},
			WarningMessage:    fmt.Sprintf("No pricing data found for %s/%s", region, service),
			ShouldShowWarning: true,
		}, nil
	}
	
	record := records[0]
	fetchedAt := record.GetDateTime("fetchedAt").Time()
	age := time.Since(fetchedAt)
	
	freshness := &DataFreshness{
		Age:         age,
		LastFetched: fetchedAt,
	}
	
	// Determine freshness status
	if age <= sdh.warningAge {
		freshness.IsFresh = true
		freshness.IsStale = false
		freshness.IsCriticallyStale = false
		freshness.ShouldShowWarning = false
	} else if age <= sdh.maxStaleAge {
		freshness.IsFresh = false
		freshness.IsStale = true
		freshness.IsCriticallyStale = false
		freshness.ShouldShowWarning = true
		freshness.WarningMessage = fmt.Sprintf(
			"Pricing data is %v old (last updated: %s). Estimates may be slightly outdated.",
			formatDuration(age), fetchedAt.Format("2006-01-02 15:04"),
		)
	} else {
		freshness.IsFresh = false
		freshness.IsStale = true
		freshness.IsCriticallyStale = true
		freshness.ShouldShowWarning = true
		freshness.WarningMessage = fmt.Sprintf(
			"Pricing data is critically stale (%v old). Estimates may be significantly inaccurate.",
			formatDuration(age),
		)
	}
	
	return freshness, nil
}

// GetPricingWithFallback attempts to get pricing data with fallback handling
func (sdh *StaleDataHandler) GetPricingWithFallback(region, service, productFamily, instanceType string) (*PricingData, *DataFreshness, error) {
	// First, try to get data for the requested region
	freshness, err := sdh.CheckDataFreshness(region, service, productFamily, instanceType)
	if err != nil {
		return nil, freshness, err
	}
	
	// If data is available and not critically stale, use it
	if !freshness.IsCriticallyStale {
		pricingData, err := sdh.getPricingData(region, service, productFamily, instanceType)
		if err == nil {
			return pricingData, freshness, nil
		}
	}
	
	// If fallback is disabled, return error
	if !sdh.enableFallback {
		return nil, freshness, fmt.Errorf("pricing data for %s is critically stale and fallback is disabled", region)
	}
	
	// Try fallback region if different from requested region
	if region != sdh.fallbackRegion {
		log.Printf("Attempting fallback to region %s for %s/%s", sdh.fallbackRegion, service, productFamily)
		
		fallbackFreshness, err := sdh.CheckDataFreshness(sdh.fallbackRegion, service, productFamily, instanceType)
		if err == nil && !fallbackFreshness.IsCriticallyStale {
			pricingData, err := sdh.getPricingData(sdh.fallbackRegion, service, productFamily, instanceType)
			if err == nil {
				// Update freshness message to indicate fallback usage
				fallbackFreshness.WarningMessage = fmt.Sprintf(
					"Using %s pricing data as fallback (requested region: %s). %s",
					sdh.fallbackRegion, region, fallbackFreshness.WarningMessage,
				)
				fallbackFreshness.ShouldShowWarning = true
				
				return pricingData, fallbackFreshness, nil
			}
		}
	}
	
	// No usable data found
	return nil, freshness, fmt.Errorf("no usable pricing data found for %s/%s (including fallback)", region, service)
}

// getPricingData retrieves pricing data from the database
func (sdh *StaleDataHandler) getPricingData(region, service, productFamily, instanceType string) (*PricingData, error) {
	records, err := sdh.app.Dao().FindRecordsByFilter(
		"awsPricingCache",
		"region = {:region} && service = {:service} && productFamily = {:productFamily} && instanceType = {:instanceType}",
		"-fetchedAt",
		1,
		0,
		map[string]any{
			"region":        region,
			"service":       service,
			"productFamily": productFamily,
			"instanceType":  instanceType,
		},
	)
	if err != nil || len(records) == 0 {
		return nil, fmt.Errorf("pricing data not found: %w", err)
	}
	
	record := records[0]
	
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

// GetSystemDataFreshness returns overall system data freshness status
func (sdh *StaleDataHandler) GetSystemDataFreshness() (*DataFreshness, error) {
	// Get the most recent pricing data across all regions/services
	records, err := sdh.app.Dao().FindRecordsByFilter(
		"awsPricingCache",
		"",
		"-fetchedAt",
		1,
		0,
		nil,
	)
	
	if err != nil || len(records) == 0 {
		return &DataFreshness{
			IsFresh:           false,
			IsStale:           true,
			IsCriticallyStale: true,
			Age:               time.Duration(0),
			LastFetched:       time.Time{},
			WarningMessage:    "No pricing data found in system",
			ShouldShowWarning: true,
		}, nil
	}
	
	record := records[0]
	fetchedAt := record.GetDateTime("fetchedAt").Time()
	age := time.Since(fetchedAt)
	
	freshness := &DataFreshness{
		Age:         age,
		LastFetched: fetchedAt,
	}
	
	if age <= sdh.warningAge {
		freshness.IsFresh = true
		freshness.ShouldShowWarning = false
	} else if age <= sdh.maxStaleAge {
		freshness.IsStale = true
		freshness.ShouldShowWarning = true
		freshness.WarningMessage = fmt.Sprintf(
			"System pricing data is %v old. Some estimates may be outdated.",
			formatDuration(age),
		)
	} else {
		freshness.IsStale = true
		freshness.IsCriticallyStale = true
		freshness.ShouldShowWarning = true
		freshness.WarningMessage = fmt.Sprintf(
			"System pricing data is critically stale (%v old). Estimates may be significantly inaccurate.",
			formatDuration(age),
		)
	}
	
	return freshness, nil
}

// CleanupStaleData removes pricing data older than the specified age
func (sdh *StaleDataHandler) CleanupStaleData(maxAge time.Duration) (int, error) {
	cutoffTime := time.Now().Add(-maxAge)
	
	collection, err := sdh.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return 0, fmt.Errorf("failed to find awsPricingCache collection: %w", err)
	}
	
	// Find records older than cutoff
	records, err := sdh.app.Dao().FindRecordsByFilter(
		collection.Id,
		"fetchedAt < {:cutoff}",
		"-fetchedAt",
		0,
		0,
		map[string]any{"cutoff": cutoffTime},
	)
	if err != nil {
		return 0, fmt.Errorf("failed to find stale records: %w", err)
	}
	
	deletedCount := 0
	for _, record := range records {
		if err := sdh.app.Dao().DeleteRecord(record); err != nil {
			log.Printf("Failed to delete stale pricing record %s: %v", record.Id, err)
			continue
		}
		deletedCount++
	}
	
	log.Printf("Cleaned up %d stale pricing records older than %v", deletedCount, maxAge)
	return deletedCount, nil
}

// GetDataAgeByRegion returns data age statistics by region
func (sdh *StaleDataHandler) GetDataAgeByRegion() (map[string]time.Duration, error) {
	regions := []string{
		"us-east-1", "us-east-2", "us-west-1", "us-west-2",
		"eu-west-1", "eu-west-2", "eu-central-1",
		"ap-southeast-1", "ap-southeast-2", "ap-northeast-1",
	}
	
	ageByRegion := make(map[string]time.Duration)
	
	for _, region := range regions {
		records, err := sdh.app.Dao().FindRecordsByFilter(
			"awsPricingCache",
			"region = {:region}",
			"-fetchedAt",
			1,
			0,
			map[string]any{"region": region},
		)
		
		if err != nil || len(records) == 0 {
			ageByRegion[region] = time.Duration(0) // No data
			continue
		}
		
		record := records[0]
		fetchedAt := record.GetDateTime("fetchedAt").Time()
		ageByRegion[region] = time.Since(fetchedAt)
	}
	
	return ageByRegion, nil
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0f seconds", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.0f minutes", d.Minutes())
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%.1f hours", d.Hours())
	} else {
		days := d.Hours() / 24
		return fmt.Sprintf("%.1f days", days)
	}
}

// IsDataUsable returns whether pricing data is usable for cost estimation
func (sdh *StaleDataHandler) IsDataUsable(region, service, productFamily, instanceType string) (bool, string) {
	freshness, err := sdh.CheckDataFreshness(region, service, productFamily, instanceType)
	if err != nil {
		return false, "No pricing data available"
	}
	
	if freshness.IsCriticallyStale {
		return false, freshness.WarningMessage
	}
	
	return true, freshness.WarningMessage
}

// GetWarningMessage returns an appropriate warning message for stale data
func (sdh *StaleDataHandler) GetWarningMessage(region, service string) string {
	freshness, err := sdh.CheckDataFreshness(region, service, "", "")
	if err != nil {
		return "Pricing data unavailable"
	}
	
	if freshness.ShouldShowWarning {
		return freshness.WarningMessage
	}
	
	return ""
}