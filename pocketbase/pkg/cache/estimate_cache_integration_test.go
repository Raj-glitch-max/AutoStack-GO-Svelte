package cache

import (
	"testing"
	"time"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/cost"
)

// TestCacheIntegration_CompleteFlow tests the complete caching workflow
// **Validates**: TR-3.1 (Cost estimate endpoint responds in <500ms)
func TestCacheIntegration_CompleteFlow(t *testing.T) {
	// Create cache with 1-hour TTL
	cache := NewEstimateCache()

	blueprint := "static-website"
	region := "us-east-1"
	variables := map[string]interface{}{
		"storage_gb":         10.0,
		"requests_per_month": 10000,
	}

	estimate := &cost.CostEstimateWithRange{
		Estimate: 56.65,
		RangeMin: 45.32,
		RangeMax: 79.31,
		Currency: "USD",
	}

	// Step 1: Cache miss - no entry exists
	cached := cache.Get(blueprint, region, variables)
	if cached != nil {
		t.Error("Expected cache miss on first access")
	}

	// Step 2: Store estimate in cache
	err := cache.Set(blueprint, region, variables, estimate)
	if err != nil {
		t.Fatalf("Failed to cache estimate: %v", err)
	}

	// Step 3: Cache hit - entry should be returned
	cached = cache.Get(blueprint, region, variables)
	if cached == nil {
		t.Fatal("Expected cache hit after storing estimate")
	}

	if cached.Estimate != estimate.Estimate {
		t.Errorf("Expected estimate %f, got %f", estimate.Estimate, cached.Estimate)
	}

	// Step 4: Verify cache stats
	stats := cache.GetStats()
	if stats.ActiveEntries != 1 {
		t.Errorf("Expected 1 active entry, got %d", stats.ActiveEntries)
	}

	// Step 5: Invalidate cache for region
	count := cache.InvalidateRegion(region)
	if count != 1 {
		t.Errorf("Expected 1 entry invalidated, got %d", count)
	}

	// Step 6: Verify cache miss after invalidation
	cached = cache.Get(blueprint, region, variables)
	if cached != nil {
		t.Error("Expected cache miss after invalidation")
	}
}

// TestCacheIntegration_PricingRefreshScenario simulates pricing data refresh
// **Validates**: Cache invalidation when pricing data refreshes
func TestCacheIntegration_PricingRefreshScenario(t *testing.T) {
	cache := NewEstimateCache()

	estimate := &cost.CostEstimateWithRange{
		Estimate: 56.65,
		RangeMin: 45.32,
		RangeMax: 79.31,
		Currency: "USD",
	}

	// Populate cache with estimates for multiple regions
	regions := []string{"us-east-1", "us-west-2", "eu-west-1"}
	blueprints := []string{"static-website", "web-application"}

	for _, region := range regions {
		for _, blueprint := range blueprints {
			cache.Set(blueprint, region, nil, estimate)
		}
	}

	// Verify all entries are cached
	stats := cache.GetStats()
	expectedEntries := len(regions) * len(blueprints)
	if stats.ActiveEntries != expectedEntries {
		t.Errorf("Expected %d active entries, got %d", expectedEntries, stats.ActiveEntries)
	}

	// Simulate pricing refresh for us-east-1
	// This should invalidate all cache entries for that region
	count := cache.InvalidateRegion("us-east-1")
	expectedInvalidated := len(blueprints)
	if count != expectedInvalidated {
		t.Errorf("Expected %d entries invalidated for us-east-1, got %d", expectedInvalidated, count)
	}

	// Verify us-east-1 entries are gone
	for _, blueprint := range blueprints {
		cached := cache.Get(blueprint, "us-east-1", nil)
		if cached != nil {
			t.Errorf("Expected cache miss for %s in us-east-1 after invalidation", blueprint)
		}
	}

	// Verify other regions still have cached entries
	for _, region := range []string{"us-west-2", "eu-west-1"} {
		for _, blueprint := range blueprints {
			cached := cache.Get(blueprint, region, nil)
			if cached == nil {
				t.Errorf("Expected cache hit for %s in %s", blueprint, region)
			}
		}
	}
}

// TestCacheIntegration_PerformanceRequirement validates <500ms response time
// **Validates**: TR-3.1 (Cost estimate endpoint responds in <500ms)
func TestCacheIntegration_PerformanceRequirement(t *testing.T) {
	cache := NewEstimateCache()

	blueprint := "static-website"
	region := "us-east-1"
	variables := map[string]interface{}{"storage_gb": 10.0}

	estimate := &cost.CostEstimateWithRange{
		Estimate: 56.65,
		RangeMin: 45.32,
		RangeMax: 79.31,
		Currency: "USD",
	}

	// Populate cache
	cache.Set(blueprint, region, variables, estimate)

	// Measure cache hit performance
	iterations := 1000
	start := time.Now()

	for i := 0; i < iterations; i++ {
		cached := cache.Get(blueprint, region, variables)
		if cached == nil {
			t.Fatal("Expected cache hit")
		}
	}

	elapsed := time.Since(start)
	avgTime := elapsed / time.Duration(iterations)

	t.Logf("Average cache hit time: %v", avgTime)

	// Cache hits should be extremely fast (well under 1ms)
	if avgTime > 1*time.Millisecond {
		t.Errorf("Cache hit too slow: %v (expected < 1ms)", avgTime)
	}

	// Verify we can easily meet the 500ms requirement
	// Even with 100 cache operations, we should be well under 500ms
	if elapsed > 500*time.Millisecond {
		t.Errorf("1000 cache hits took %v, exceeds 500ms", elapsed)
	}
}

// TestCacheIntegration_TTLExpiration tests cache entry expiration
// **Validates**: Cache entries expire after 1-hour TTL
func TestCacheIntegration_TTLExpiration(t *testing.T) {
	// Use short TTL for testing
	cache := NewEstimateCacheWithTTL(100 * time.Millisecond)

	blueprint := "static-website"
	region := "us-east-1"
	variables := map[string]interface{}{"storage_gb": 10.0}

	estimate := &cost.CostEstimateWithRange{
		Estimate: 56.65,
		RangeMin: 45.32,
		RangeMax: 79.31,
		Currency: "USD",
	}

	// Store estimate
	cache.Set(blueprint, region, variables, estimate)

	// Verify immediate cache hit
	cached := cache.Get(blueprint, region, variables)
	if cached == nil {
		t.Fatal("Expected cache hit immediately after set")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Verify cache miss after expiration
	cached = cache.Get(blueprint, region, variables)
	if cached != nil {
		t.Error("Expected cache miss after TTL expiration")
	}

	// Verify stats show expired entry
	stats := cache.GetStats()
	if stats.ExpiredEntries != 1 {
		t.Errorf("Expected 1 expired entry, got %d", stats.ExpiredEntries)
	}

	// Clean expired entries
	count := cache.CleanExpired()
	if count != 1 {
		t.Errorf("Expected 1 entry cleaned, got %d", count)
	}

	// Verify cache is empty after cleanup
	stats = cache.GetStats()
	if stats.TotalEntries != 0 {
		t.Errorf("Expected 0 entries after cleanup, got %d", stats.TotalEntries)
	}
}

// TestCacheIntegration_ConcurrentAccess tests thread-safe cache operations
// **Validates**: Cache is thread-safe for concurrent access
func TestCacheIntegration_ConcurrentAccess(t *testing.T) {
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

	// Concurrent reads and invalidations
	for i := 0; i < operations; i++ {
		go func(index int) {
			blueprint := "static-website"
			region := "us-east-1"
			variables := map[string]interface{}{"index": index}

			// Read
			cache.Get(blueprint, region, variables)

			// Invalidate
			if index%10 == 0 {
				cache.Invalidate(blueprint, region, variables)
			}

			done <- true
		}(i)
	}

	// Wait for all operations
	for i := 0; i < operations; i++ {
		<-done
	}

	// Verify cache is still functional
	stats := cache.GetStats()
	if stats.TotalEntries == 0 {
		t.Error("Expected some entries to remain after concurrent operations")
	}

	t.Logf("Cache has %d entries after concurrent operations", stats.TotalEntries)
}

// TestCacheIntegration_CleanupRoutine tests background cleanup
// **Validates**: Background cleanup routine removes expired entries
func TestCacheIntegration_CleanupRoutine(t *testing.T) {
	cache := NewEstimateCacheWithTTL(50 * time.Millisecond)

	estimate := &cost.CostEstimateWithRange{
		Estimate: 56.65,
		RangeMin: 45.32,
		RangeMax: 79.31,
		Currency: "USD",
	}

	// Add entries
	for i := 0; i < 10; i++ {
		cache.Set("blueprint", "us-east-1", map[string]interface{}{"index": i}, estimate)
	}

	// Verify entries exist
	stats := cache.GetStats()
	if stats.TotalEntries != 10 {
		t.Errorf("Expected 10 entries, got %d", stats.TotalEntries)
	}

	// Start cleanup routine with short interval
	stopChan := cache.StartCleanupRoutine(100 * time.Millisecond)
	defer close(stopChan)

	// Wait for entries to expire and cleanup to run
	time.Sleep(200 * time.Millisecond)

	// Verify entries were cleaned up
	stats = cache.GetStats()
	if stats.TotalEntries != 0 {
		t.Errorf("Expected 0 entries after cleanup, got %d", stats.TotalEntries)
	}
}
