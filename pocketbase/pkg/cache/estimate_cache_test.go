package cache

import (
	"testing"
	"time"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/cost"
)

// TestNewEstimateCache tests cache creation
func TestNewEstimateCache(t *testing.T) {
	cache := NewEstimateCache()

	if cache == nil {
		t.Fatal("Expected cache to be created, got nil")
	}

	if cache.ttl != 1*time.Hour {
		t.Errorf("Expected TTL to be 1 hour, got %v", cache.ttl)
	}

	if cache.maxEntries != 1000 {
		t.Errorf("Expected maxEntries to be 1000, got %d", cache.maxEntries)
	}

	if cache.cache == nil {
		t.Error("Expected cache map to be initialized")
	}
}

// TestNewEstimateCacheWithTTL tests cache creation with custom TTL
func TestNewEstimateCacheWithTTL(t *testing.T) {
	customTTL := 30 * time.Minute
	cache := NewEstimateCacheWithTTL(customTTL)

	if cache.ttl != customTTL {
		t.Errorf("Expected TTL to be %v, got %v", customTTL, cache.ttl)
	}
}

// TestGenerateCacheKey tests cache key generation
func TestGenerateCacheKey(t *testing.T) {
	tests := []struct {
		name      string
		blueprint string
		region    string
		variables map[string]interface{}
		wantSame  bool
	}{
		{
			name:      "Same inputs produce same key",
			blueprint: "static-website",
			region:    "us-east-1",
			variables: map[string]interface{}{"storage": 10},
			wantSame:  true,
		},
		{
			name:      "Different blueprint produces different key",
			blueprint: "web-application",
			region:    "us-east-1",
			variables: map[string]interface{}{"storage": 10},
			wantSame:  false,
		},
		{
			name:      "Different region produces different key",
			blueprint: "static-website",
			region:    "us-west-2",
			variables: map[string]interface{}{"storage": 10},
			wantSame:  false,
		},
		{
			name:      "Different variables produce different key",
			blueprint: "static-website",
			region:    "us-east-1",
			variables: map[string]interface{}{"storage": 20},
			wantSame:  false,
		},
	}

	baseKey := GenerateCacheKey("static-website", "us-east-1", map[string]interface{}{"storage": 10})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := GenerateCacheKey(tt.blueprint, tt.region, tt.variables)

			if key == "" {
				t.Error("Expected non-empty cache key")
			}

			if tt.wantSame && key != baseKey {
				t.Errorf("Expected same key as base, got different: %s vs %s", key, baseKey)
			}

			if !tt.wantSame && key == baseKey {
				t.Errorf("Expected different key from base, got same: %s", key)
			}
		})
	}
}

// TestGenerateCacheKeyDeterministic tests that cache key generation is deterministic
func TestGenerateCacheKeyDeterministic(t *testing.T) {
	blueprint := "static-website"
	region := "us-east-1"
	variables := map[string]interface{}{
		"storage":  10,
		"requests": 1000,
	}

	key1 := GenerateCacheKey(blueprint, region, variables)
	key2 := GenerateCacheKey(blueprint, region, variables)

	if key1 != key2 {
		t.Errorf("Expected deterministic key generation, got different keys: %s vs %s", key1, key2)
	}
}

// TestCacheSetAndGet tests basic cache set and get operations
func TestCacheSetAndGet(t *testing.T) {
	cache := NewEstimateCache()

	blueprint := "static-website"
	region := "us-east-1"
	variables := map[string]interface{}{"storage": 10}

	estimate := &cost.CostEstimateWithRange{
		Estimate: 56.65,
		RangeMin: 45.32,
		RangeMax: 79.31,
		Currency: "USD",
	}

	// Set estimate in cache
	err := cache.Set(blueprint, region, variables, estimate)
	if err != nil {
		t.Fatalf("Failed to set cache: %v", err)
	}

	// Get estimate from cache
	cached := cache.Get(blueprint, region, variables)
	if cached == nil {
		t.Fatal("Expected cached estimate, got nil")
	}

	if cached.Estimate != estimate.Estimate {
		t.Errorf("Expected estimate %f, got %f", estimate.Estimate, cached.Estimate)
	}

	if cached.RangeMin != estimate.RangeMin {
		t.Errorf("Expected RangeMin %f, got %f", estimate.RangeMin, cached.RangeMin)
	}

	if cached.RangeMax != estimate.RangeMax {
		t.Errorf("Expected RangeMax %f, got %f", estimate.RangeMax, cached.RangeMax)
	}
}

// TestCacheSetNilEstimate tests that setting nil estimate returns error
func TestCacheSetNilEstimate(t *testing.T) {
	cache := NewEstimateCache()

	err := cache.Set("static-website", "us-east-1", nil, nil)
	if err == nil {
		t.Error("Expected error when setting nil estimate, got nil")
	}
}

// TestCacheMiss tests cache miss scenario
func TestCacheMiss(t *testing.T) {
	cache := NewEstimateCache()

	// Try to get non-existent entry
	cached := cache.Get("static-website", "us-east-1", map[string]interface{}{"storage": 10})
	if cached != nil {
		t.Errorf("Expected nil for cache miss, got %v", cached)
	}
}

// TestCacheExpiration tests that cache entries expire after TTL
func TestCacheExpiration(t *testing.T) {
	// Use short TTL for testing
	cache := NewEstimateCacheWithTTL(100 * time.Millisecond)

	blueprint := "static-website"
	region := "us-east-1"
	variables := map[string]interface{}{"storage": 10}

	estimate := &cost.CostEstimateWithRange{
		Estimate: 56.65,
		RangeMin: 45.32,
		RangeMax: 79.31,
		Currency: "USD",
	}

	// Set estimate in cache
	err := cache.Set(blueprint, region, variables, estimate)
	if err != nil {
		t.Fatalf("Failed to set cache: %v", err)
	}

	// Immediately get - should succeed
	cached := cache.Get(blueprint, region, variables)
	if cached == nil {
		t.Error("Expected cached estimate immediately after set")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Get after expiration - should return nil
	cached = cache.Get(blueprint, region, variables)
	if cached != nil {
		t.Error("Expected nil after cache expiration, got cached estimate")
	}
}

// TestCacheInvalidate tests cache invalidation
func TestCacheInvalidate(t *testing.T) {
	cache := NewEstimateCache()

	blueprint := "static-website"
	region := "us-east-1"
	variables := map[string]interface{}{"storage": 10}

	estimate := &cost.CostEstimateWithRange{
		Estimate: 56.65,
		RangeMin: 45.32,
		RangeMax: 79.31,
		Currency: "USD",
	}

	// Set estimate in cache
	err := cache.Set(blueprint, region, variables, estimate)
	if err != nil {
		t.Fatalf("Failed to set cache: %v", err)
	}

	// Verify it's cached
	cached := cache.Get(blueprint, region, variables)
	if cached == nil {
		t.Fatal("Expected cached estimate")
	}

	// Invalidate
	cache.Invalidate(blueprint, region, variables)

	// Verify it's gone
	cached = cache.Get(blueprint, region, variables)
	if cached != nil {
		t.Error("Expected nil after invalidation, got cached estimate")
	}
}

// TestCacheInvalidateRegion tests region-based cache invalidation
func TestCacheInvalidateRegion(t *testing.T) {
	cache := NewEstimateCache()

	estimate := &cost.CostEstimateWithRange{
		Estimate: 56.65,
		RangeMin: 45.32,
		RangeMax: 79.31,
		Currency: "USD",
	}

	// Set multiple estimates for different regions
	cache.Set("static-website", "us-east-1", nil, estimate)
	cache.Set("static-website", "us-west-2", nil, estimate)
	cache.Set("web-application", "us-east-1", nil, estimate)

	// Invalidate us-east-1 region
	count := cache.InvalidateRegion("us-east-1")

	if count != 2 {
		t.Errorf("Expected 2 entries invalidated, got %d", count)
	}

	// Verify us-east-1 entries are gone
	if cache.Get("static-website", "us-east-1", nil) != nil {
		t.Error("Expected us-east-1 static-website to be invalidated")
	}
	if cache.Get("web-application", "us-east-1", nil) != nil {
		t.Error("Expected us-east-1 web-application to be invalidated")
	}

	// Verify us-west-2 entry still exists
	if cache.Get("static-website", "us-west-2", nil) == nil {
		t.Error("Expected us-west-2 entry to still exist")
	}
}

// TestCacheInvalidateBlueprint tests blueprint-based cache invalidation
func TestCacheInvalidateBlueprint(t *testing.T) {
	cache := NewEstimateCache()

	estimate := &cost.CostEstimateWithRange{
		Estimate: 56.65,
		RangeMin: 45.32,
		RangeMax: 79.31,
		Currency: "USD",
	}

	// Set multiple estimates for different blueprints
	cache.Set("static-website", "us-east-1", nil, estimate)
	cache.Set("static-website", "us-west-2", nil, estimate)
	cache.Set("web-application", "us-east-1", nil, estimate)

	// Invalidate static-website blueprint
	count := cache.InvalidateBlueprint("static-website")

	if count != 2 {
		t.Errorf("Expected 2 entries invalidated, got %d", count)
	}

	// Verify static-website entries are gone
	if cache.Get("static-website", "us-east-1", nil) != nil {
		t.Error("Expected static-website us-east-1 to be invalidated")
	}
	if cache.Get("static-website", "us-west-2", nil) != nil {
		t.Error("Expected static-website us-west-2 to be invalidated")
	}

	// Verify web-application entry still exists
	if cache.Get("web-application", "us-east-1", nil) == nil {
		t.Error("Expected web-application entry to still exist")
	}
}

// TestCacheInvalidateAll tests clearing entire cache
func TestCacheInvalidateAll(t *testing.T) {
	cache := NewEstimateCache()

	estimate := &cost.CostEstimateWithRange{
		Estimate: 56.65,
		RangeMin: 45.32,
		RangeMax: 79.31,
		Currency: "USD",
	}

	// Set multiple estimates
	cache.Set("static-website", "us-east-1", nil, estimate)
	cache.Set("static-website", "us-west-2", nil, estimate)
	cache.Set("web-application", "us-east-1", nil, estimate)

	// Invalidate all
	cache.InvalidateAll()

	// Verify all entries are gone
	if cache.Get("static-website", "us-east-1", nil) != nil {
		t.Error("Expected all entries to be invalidated")
	}
	if cache.Get("static-website", "us-west-2", nil) != nil {
		t.Error("Expected all entries to be invalidated")
	}
	if cache.Get("web-application", "us-east-1", nil) != nil {
		t.Error("Expected all entries to be invalidated")
	}

	stats := cache.GetStats()
	if stats.TotalEntries != 0 {
		t.Errorf("Expected 0 total entries after InvalidateAll, got %d", stats.TotalEntries)
	}
}

// TestCacheCleanExpired tests expired entry cleanup
func TestCacheCleanExpired(t *testing.T) {
	// Use short TTL for testing
	cache := NewEstimateCacheWithTTL(50 * time.Millisecond)

	estimate := &cost.CostEstimateWithRange{
		Estimate: 56.65,
		RangeMin: 45.32,
		RangeMax: 79.31,
		Currency: "USD",
	}

	// Set multiple estimates
	cache.Set("static-website", "us-east-1", nil, estimate)
	cache.Set("static-website", "us-west-2", nil, estimate)

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Clean expired entries
	count := cache.CleanExpired()

	if count != 2 {
		t.Errorf("Expected 2 expired entries cleaned, got %d", count)
	}

	stats := cache.GetStats()
	if stats.TotalEntries != 0 {
		t.Errorf("Expected 0 total entries after cleanup, got %d", stats.TotalEntries)
	}
}

// TestCacheMaxEntries tests cache eviction when max entries reached
func TestCacheMaxEntries(t *testing.T) {
	cache := NewEstimateCache()
	cache.maxEntries = 3 // Set low limit for testing

	estimate := &cost.CostEstimateWithRange{
		Estimate: 56.65,
		RangeMin: 45.32,
		RangeMax: 79.31,
		Currency: "USD",
	}

	// Add entries up to max
	cache.Set("blueprint1", "us-east-1", nil, estimate)
	cache.Set("blueprint2", "us-east-1", nil, estimate)
	cache.Set("blueprint3", "us-east-1", nil, estimate)

	stats := cache.GetStats()
	if stats.TotalEntries != 3 {
		t.Errorf("Expected 3 entries, got %d", stats.TotalEntries)
	}

	// Add one more - should trigger eviction
	cache.Set("blueprint4", "us-east-1", nil, estimate)

	stats = cache.GetStats()
	if stats.TotalEntries != 3 {
		t.Errorf("Expected 3 entries after eviction, got %d", stats.TotalEntries)
	}

	// Verify newest entry exists
	if cache.Get("blueprint4", "us-east-1", nil) == nil {
		t.Error("Expected newest entry to exist after eviction")
	}
}

// TestCacheGetStats tests cache statistics
func TestCacheGetStats(t *testing.T) {
	cache := NewEstimateCacheWithTTL(100 * time.Millisecond)

	estimate := &cost.CostEstimateWithRange{
		Estimate: 56.65,
		RangeMin: 45.32,
		RangeMax: 79.31,
		Currency: "USD",
	}

	// Add some entries
	cache.Set("blueprint1", "us-east-1", nil, estimate)
	cache.Set("blueprint2", "us-east-1", nil, estimate)

	// Get stats immediately
	stats := cache.GetStats()

	if stats.TotalEntries != 2 {
		t.Errorf("Expected 2 total entries, got %d", stats.TotalEntries)
	}

	if stats.ActiveEntries != 2 {
		t.Errorf("Expected 2 active entries, got %d", stats.ActiveEntries)
	}

	if stats.ExpiredEntries != 0 {
		t.Errorf("Expected 0 expired entries, got %d", stats.ExpiredEntries)
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Get stats after expiration
	stats = cache.GetStats()

	if stats.TotalEntries != 2 {
		t.Errorf("Expected 2 total entries (not cleaned yet), got %d", stats.TotalEntries)
	}

	if stats.ActiveEntries != 0 {
		t.Errorf("Expected 0 active entries, got %d", stats.ActiveEntries)
	}

	if stats.ExpiredEntries != 2 {
		t.Errorf("Expected 2 expired entries, got %d", stats.ExpiredEntries)
	}
}

// TestCacheGetEntry tests getting full cache entry with metadata
func TestCacheGetEntry(t *testing.T) {
	cache := NewEstimateCache()

	blueprint := "static-website"
	region := "us-east-1"
	variables := map[string]interface{}{"storage": 10}

	estimate := &cost.CostEstimateWithRange{
		Estimate: 56.65,
		RangeMin: 45.32,
		RangeMax: 79.31,
		Currency: "USD",
	}

	// Set estimate in cache
	err := cache.Set(blueprint, region, variables, estimate)
	if err != nil {
		t.Fatalf("Failed to set cache: %v", err)
	}

	// Get full entry
	entry := cache.GetEntry(blueprint, region, variables)
	if entry == nil {
		t.Fatal("Expected cache entry, got nil")
	}

	if entry.Blueprint != blueprint {
		t.Errorf("Expected blueprint %s, got %s", blueprint, entry.Blueprint)
	}

	if entry.Region != region {
		t.Errorf("Expected region %s, got %s", region, entry.Region)
	}

	if entry.Estimate.Estimate != estimate.Estimate {
		t.Errorf("Expected estimate %f, got %f", estimate.Estimate, entry.Estimate.Estimate)
	}

	if entry.CachedAt.IsZero() {
		t.Error("Expected CachedAt to be set")
	}

	if entry.ExpiresAt.IsZero() {
		t.Error("Expected ExpiresAt to be set")
	}

	// Verify TTL is approximately 1 hour
	ttl := entry.ExpiresAt.Sub(entry.CachedAt)
	if ttl < 59*time.Minute || ttl > 61*time.Minute {
		t.Errorf("Expected TTL around 1 hour, got %v", ttl)
	}
}

// TestCacheConcurrency tests concurrent cache operations
func TestCacheConcurrency(t *testing.T) {
	cache := NewEstimateCache()

	estimate := &cost.CostEstimateWithRange{
		Estimate: 56.65,
		RangeMin: 45.32,
		RangeMax: 79.31,
		Currency: "USD",
	}

	// Run concurrent operations
	done := make(chan bool)
	operations := 100

	// Concurrent writes
	for i := 0; i < operations; i++ {
		go func(index int) {
			blueprint := "static-website"
			region := "us-east-1"
			variables := map[string]interface{}{"index": index}
			cache.Set(blueprint, region, variables, estimate)
			done <- true
		}(i)
	}

	// Wait for all writes
	for i := 0; i < operations; i++ {
		<-done
	}

	// Concurrent reads
	for i := 0; i < operations; i++ {
		go func(index int) {
			blueprint := "static-website"
			region := "us-east-1"
			variables := map[string]interface{}{"index": index}
			cache.Get(blueprint, region, variables)
			done <- true
		}(i)
	}

	// Wait for all reads
	for i := 0; i < operations; i++ {
		<-done
	}

	// Verify cache is still functional
	stats := cache.GetStats()
	if stats.TotalEntries == 0 {
		t.Error("Expected cache to have entries after concurrent operations")
	}
}

// TestCacheCleanupRoutine tests background cleanup routine
func TestCacheCleanupRoutine(t *testing.T) {
	cache := NewEstimateCacheWithTTL(50 * time.Millisecond)

	estimate := &cost.CostEstimateWithRange{
		Estimate: 56.65,
		RangeMin: 45.32,
		RangeMax: 79.31,
		Currency: "USD",
	}

	// Add entries
	cache.Set("blueprint1", "us-east-1", nil, estimate)
	cache.Set("blueprint2", "us-east-1", nil, estimate)

	// Start cleanup routine with short interval
	stopChan := cache.StartCleanupRoutine(100 * time.Millisecond)
	defer close(stopChan)

	// Wait for entries to expire and cleanup to run
	time.Sleep(200 * time.Millisecond)

	// Verify entries were cleaned up
	stats := cache.GetStats()
	if stats.TotalEntries != 0 {
		t.Errorf("Expected 0 entries after cleanup routine, got %d", stats.TotalEntries)
	}
}
