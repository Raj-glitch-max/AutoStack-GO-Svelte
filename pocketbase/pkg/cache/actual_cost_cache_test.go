package cache

import (
	"testing"
	"time"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
)

// TestNewActualCostCache verifies cache initialization
func TestNewActualCostCache(t *testing.T) {
	cache := NewActualCostCache()

	if cache == nil {
		t.Fatal("Expected cache to be initialized")
	}

	if cache.ttl != 6*time.Hour {
		t.Errorf("Expected TTL to be 6 hours, got %v", cache.ttl)
	}

	if cache.maxEntries != 500 {
		t.Errorf("Expected maxEntries to be 500, got %d", cache.maxEntries)
	}

	if len(cache.cache) != 0 {
		t.Errorf("Expected empty cache, got %d entries", len(cache.cache))
	}
}

// TestNewActualCostCacheWithTTL verifies custom TTL initialization
func TestNewActualCostCacheWithTTL(t *testing.T) {
	customTTL := 30 * time.Minute
	cache := NewActualCostCacheWithTTL(customTTL)

	if cache.ttl != customTTL {
		t.Errorf("Expected TTL to be %v, got %v", customTTL, cache.ttl)
	}
}

// TestGenerateActualCostCacheKey verifies cache key generation
func TestGenerateActualCostCacheKey(t *testing.T) {
	deploymentID := "deploy_123"
	key1 := GenerateActualCostCacheKey(deploymentID)
	key2 := GenerateActualCostCacheKey(deploymentID)

	// Same deployment ID should produce same key
	if key1 != key2 {
		t.Errorf("Expected same key for same deployment ID, got %s and %s", key1, key2)
	}

	// Different deployment IDs should produce different keys
	key3 := GenerateActualCostCacheKey("deploy_456")
	if key1 == key3 {
		t.Error("Expected different keys for different deployment IDs")
	}

	// Key should be a valid hex string (SHA256 = 64 hex chars)
	if len(key1) != 64 {
		t.Errorf("Expected key length to be 64, got %d", len(key1))
	}
}

// TestActualCostCacheSetAndGet verifies basic cache operations
func TestActualCostCacheSetAndGet(t *testing.T) {
	cache := NewActualCostCache()
	deploymentID := "deploy_123"

	actualCost := &aws.ActualCostData{
		CostToDate:       47.23,
		ProjectedMonthly: 62.18,
		Variance:         9.76,
		Breakdown: map[string]float64{
			"AmazonS3":         2.45,
			"AmazonCloudFront": 8.90,
		},
		Period: aws.CostPeriod{
			Start: time.Now().AddDate(0, 0, -10),
			End:   time.Now(),
		},
		FetchedAt: time.Now(),
	}

	// Set cache entry
	err := cache.Set(deploymentID, actualCost)
	if err != nil {
		t.Fatalf("Failed to set cache entry: %v", err)
	}

	// Get cache entry
	retrieved := cache.Get(deploymentID)
	if retrieved == nil {
		t.Fatal("Expected to retrieve cached entry")
	}

	// Verify data
	if retrieved.CostToDate != actualCost.CostToDate {
		t.Errorf("Expected CostToDate %f, got %f", actualCost.CostToDate, retrieved.CostToDate)
	}

	if retrieved.ProjectedMonthly != actualCost.ProjectedMonthly {
		t.Errorf("Expected ProjectedMonthly %f, got %f", actualCost.ProjectedMonthly, retrieved.ProjectedMonthly)
	}

	if retrieved.Variance != actualCost.Variance {
		t.Errorf("Expected Variance %f, got %f", actualCost.Variance, retrieved.Variance)
	}
}

// TestActualCostCacheGetMiss verifies cache miss behavior
func TestActualCostCacheGetMiss(t *testing.T) {
	cache := NewActualCostCache()

	// Try to get non-existent entry
	retrieved := cache.Get("nonexistent_deployment")
	if retrieved != nil {
		t.Error("Expected nil for cache miss")
	}
}

// TestActualCostCacheSetNil verifies nil value handling
func TestActualCostCacheSetNil(t *testing.T) {
	cache := NewActualCostCache()

	err := cache.Set("deploy_123", nil)
	if err == nil {
		t.Error("Expected error when setting nil value")
	}
}

// TestActualCostCacheExpiration verifies TTL expiration
func TestActualCostCacheExpiration(t *testing.T) {
	// Create cache with short TTL for testing
	cache := NewActualCostCacheWithTTL(100 * time.Millisecond)
	deploymentID := "deploy_123"

	actualCost := &aws.ActualCostData{
		CostToDate:       47.23,
		ProjectedMonthly: 62.18,
		Variance:         9.76,
		Breakdown:        map[string]float64{},
		Period: aws.CostPeriod{
			Start: time.Now().AddDate(0, 0, -10),
			End:   time.Now(),
		},
		FetchedAt: time.Now(),
	}

	// Set cache entry
	err := cache.Set(deploymentID, actualCost)
	if err != nil {
		t.Fatalf("Failed to set cache entry: %v", err)
	}

	// Verify entry exists
	retrieved := cache.Get(deploymentID)
	if retrieved == nil {
		t.Fatal("Expected to retrieve cached entry")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Verify entry is expired
	retrieved = cache.Get(deploymentID)
	if retrieved != nil {
		t.Error("Expected nil for expired entry")
	}
}

// TestActualCostCacheInvalidate verifies cache invalidation
func TestActualCostCacheInvalidate(t *testing.T) {
	cache := NewActualCostCache()
	deploymentID := "deploy_123"

	actualCost := &aws.ActualCostData{
		CostToDate:       47.23,
		ProjectedMonthly: 62.18,
		Variance:         9.76,
		Breakdown:        map[string]float64{},
		Period: aws.CostPeriod{
			Start: time.Now().AddDate(0, 0, -10),
			End:   time.Now(),
		},
		FetchedAt: time.Now(),
	}

	// Set cache entry
	cache.Set(deploymentID, actualCost)

	// Verify entry exists
	if cache.Get(deploymentID) == nil {
		t.Fatal("Expected to retrieve cached entry")
	}

	// Invalidate entry
	cache.Invalidate(deploymentID)

	// Verify entry is gone
	if cache.Get(deploymentID) != nil {
		t.Error("Expected nil after invalidation")
	}
}

// TestActualCostCacheInvalidateAll verifies full cache invalidation
func TestActualCostCacheInvalidateAll(t *testing.T) {
	cache := NewActualCostCache()

	actualCost := &aws.ActualCostData{
		CostToDate:       47.23,
		ProjectedMonthly: 62.18,
		Variance:         9.76,
		Breakdown:        map[string]float64{},
		Period: aws.CostPeriod{
			Start: time.Now().AddDate(0, 0, -10),
			End:   time.Now(),
		},
		FetchedAt: time.Now(),
	}

	// Set multiple entries
	cache.Set("deploy_1", actualCost)
	cache.Set("deploy_2", actualCost)
	cache.Set("deploy_3", actualCost)

	// Verify entries exist
	stats := cache.GetStats()
	if stats.TotalEntries != 3 {
		t.Errorf("Expected 3 entries, got %d", stats.TotalEntries)
	}

	// Invalidate all
	cache.InvalidateAll()

	// Verify cache is empty
	stats = cache.GetStats()
	if stats.TotalEntries != 0 {
		t.Errorf("Expected 0 entries after InvalidateAll, got %d", stats.TotalEntries)
	}
}

// TestActualCostCacheCleanExpired verifies expired entry cleanup
func TestActualCostCacheCleanExpired(t *testing.T) {
	// Create cache with short TTL
	cache := NewActualCostCacheWithTTL(100 * time.Millisecond)

	actualCost := &aws.ActualCostData{
		CostToDate:       47.23,
		ProjectedMonthly: 62.18,
		Variance:         9.76,
		Breakdown:        map[string]float64{},
		Period: aws.CostPeriod{
			Start: time.Now().AddDate(0, 0, -10),
			End:   time.Now(),
		},
		FetchedAt: time.Now(),
	}

	// Set multiple entries
	cache.Set("deploy_1", actualCost)
	cache.Set("deploy_2", actualCost)
	cache.Set("deploy_3", actualCost)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Clean expired entries
	count := cache.CleanExpired()
	if count != 3 {
		t.Errorf("Expected to clean 3 expired entries, got %d", count)
	}

	// Verify cache is empty
	stats := cache.GetStats()
	if stats.TotalEntries != 0 {
		t.Errorf("Expected 0 entries after cleanup, got %d", stats.TotalEntries)
	}
}

// TestActualCostCacheMaxEntries verifies eviction when max entries reached
func TestActualCostCacheMaxEntries(t *testing.T) {
	cache := NewActualCostCache()
	cache.maxEntries = 3 // Set low limit for testing

	actualCost := &aws.ActualCostData{
		CostToDate:       47.23,
		ProjectedMonthly: 62.18,
		Variance:         9.76,
		Breakdown:        map[string]float64{},
		Period: aws.CostPeriod{
			Start: time.Now().AddDate(0, 0, -10),
			End:   time.Now(),
		},
		FetchedAt: time.Now(),
	}

	// Fill cache to max
	cache.Set("deploy_1", actualCost)
	time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	cache.Set("deploy_2", actualCost)
	time.Sleep(10 * time.Millisecond)
	cache.Set("deploy_3", actualCost)

	// Verify cache is full
	stats := cache.GetStats()
	if stats.TotalEntries != 3 {
		t.Errorf("Expected 3 entries, got %d", stats.TotalEntries)
	}

	// Add one more entry (should evict oldest)
	time.Sleep(10 * time.Millisecond)
	cache.Set("deploy_4", actualCost)

	// Verify cache still has max entries
	stats = cache.GetStats()
	if stats.TotalEntries != 3 {
		t.Errorf("Expected 3 entries after eviction, got %d", stats.TotalEntries)
	}

	// Verify oldest entry was evicted
	if cache.Get("deploy_1") != nil {
		t.Error("Expected oldest entry to be evicted")
	}

	// Verify newest entry exists
	if cache.Get("deploy_4") == nil {
		t.Error("Expected newest entry to exist")
	}
}

// TestActualCostCacheGetStats verifies statistics reporting
func TestActualCostCacheGetStats(t *testing.T) {
	cache := NewActualCostCacheWithTTL(100 * time.Millisecond)

	actualCost := &aws.ActualCostData{
		CostToDate:       47.23,
		ProjectedMonthly: 62.18,
		Variance:         9.76,
		Breakdown:        map[string]float64{},
		Period: aws.CostPeriod{
			Start: time.Now().AddDate(0, 0, -10),
			End:   time.Now(),
		},
		FetchedAt: time.Now(),
	}

	// Add some entries
	cache.Set("deploy_1", actualCost)
	cache.Set("deploy_2", actualCost)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Add fresh entry
	cache.Set("deploy_3", actualCost)

	// Get stats
	stats := cache.GetStats()

	if stats.TotalEntries != 3 {
		t.Errorf("Expected 3 total entries, got %d", stats.TotalEntries)
	}

	if stats.ActiveEntries != 1 {
		t.Errorf("Expected 1 active entry, got %d", stats.ActiveEntries)
	}

	if stats.ExpiredEntries != 2 {
		t.Errorf("Expected 2 expired entries, got %d", stats.ExpiredEntries)
	}

	if stats.MaxEntries != 500 {
		t.Errorf("Expected maxEntries to be 500, got %d", stats.MaxEntries)
	}

	if stats.TTL != 100*time.Millisecond {
		t.Errorf("Expected TTL to be 100ms, got %v", stats.TTL)
	}
}

// TestActualCostCacheGetEntry verifies full entry retrieval
func TestActualCostCacheGetEntry(t *testing.T) {
	cache := NewActualCostCache()
	deploymentID := "deploy_123"

	actualCost := &aws.ActualCostData{
		CostToDate:       47.23,
		ProjectedMonthly: 62.18,
		Variance:         9.76,
		Breakdown:        map[string]float64{},
		Period: aws.CostPeriod{
			Start: time.Now().AddDate(0, 0, -10),
			End:   time.Now(),
		},
		FetchedAt: time.Now(),
	}

	// Set cache entry
	cache.Set(deploymentID, actualCost)

	// Get full entry
	entry := cache.GetEntry(deploymentID)
	if entry == nil {
		t.Fatal("Expected to retrieve cache entry")
	}

	// Verify metadata
	if entry.DeploymentID != deploymentID {
		t.Errorf("Expected deployment ID %s, got %s", deploymentID, entry.DeploymentID)
	}

	if entry.ActualCost == nil {
		t.Error("Expected actual cost to be set")
	}

	if entry.CachedAt.IsZero() {
		t.Error("Expected CachedAt to be set")
	}

	if entry.ExpiresAt.IsZero() {
		t.Error("Expected ExpiresAt to be set")
	}

	// Verify expiration time is 6 hours in the future
	expectedExpiration := entry.CachedAt.Add(6 * time.Hour)
	if !entry.ExpiresAt.Equal(expectedExpiration) {
		t.Errorf("Expected expiration at %v, got %v", expectedExpiration, entry.ExpiresAt)
	}
}

// TestActualCostCacheConcurrency verifies thread-safe operations
func TestActualCostCacheConcurrency(t *testing.T) {
	cache := NewActualCostCache()

	actualCost := &aws.ActualCostData{
		CostToDate:       47.23,
		ProjectedMonthly: 62.18,
		Variance:         9.76,
		Breakdown:        map[string]float64{},
		Period: aws.CostPeriod{
			Start: time.Now().AddDate(0, 0, -10),
			End:   time.Now(),
		},
		FetchedAt: time.Now(),
	}

	// Concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			deploymentID := "deploy_" + string(rune('0'+id))
			cache.Set(deploymentID, actualCost)
			done <- true
		}(i)
	}

	// Wait for all writes
	for i := 0; i < 10; i++ {
		<-done
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func(id int) {
			deploymentID := "deploy_" + string(rune('0'+id))
			cache.Get(deploymentID)
			done <- true
		}(i)
	}

	// Wait for all reads
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify no panics occurred
	stats := cache.GetStats()
	if stats.TotalEntries == 0 {
		t.Error("Expected entries to be cached")
	}
}
