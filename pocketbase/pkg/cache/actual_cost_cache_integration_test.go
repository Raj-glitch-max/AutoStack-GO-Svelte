package cache

import (
	"testing"
	"time"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
)

// TestActualCostCacheIntegration_CacheHitAndMiss tests cache hit and miss scenarios
func TestActualCostCacheIntegration_CacheHitAndMiss(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cache := NewActualCostCache()
	deploymentID := "deploy_integration_test"

	// Test cache miss
	result := cache.Get(deploymentID)
	if result != nil {
		t.Error("Expected cache miss for non-existent deployment")
	}

	// Create actual cost data
	actualCost := &aws.ActualCostData{
		DeploymentID:     deploymentID,
		CostToDate:       123.45,
		ProjectedMonthly: 180.00,
		Variance:         15.5,
		Breakdown: map[string]float64{
			"AmazonEC2":        50.00,
			"AmazonS3":         20.00,
			"AmazonRDS":        30.00,
			"AmazonCloudFront": 23.45,
		},
		Period: aws.CostPeriod{
			Start: time.Now().AddDate(0, 0, -20),
			End:   time.Now().AddDate(0, 0, -2),
		},
		FetchedAt: time.Now(),
	}

	// Set cache entry
	err := cache.Set(deploymentID, actualCost)
	if err != nil {
		t.Fatalf("Failed to set cache entry: %v", err)
	}

	// Test cache hit
	result = cache.Get(deploymentID)
	if result == nil {
		t.Fatal("Expected cache hit for existing deployment")
	}

	// Verify data integrity
	if result.DeploymentID != actualCost.DeploymentID {
		t.Errorf("Expected deployment ID %s, got %s", actualCost.DeploymentID, result.DeploymentID)
	}

	if result.CostToDate != actualCost.CostToDate {
		t.Errorf("Expected cost to date %f, got %f", actualCost.CostToDate, result.CostToDate)
	}

	if result.ProjectedMonthly != actualCost.ProjectedMonthly {
		t.Errorf("Expected projected monthly %f, got %f", actualCost.ProjectedMonthly, result.ProjectedMonthly)
	}

	if result.Variance != actualCost.Variance {
		t.Errorf("Expected variance %f, got %f", actualCost.Variance, result.Variance)
	}

	// Verify breakdown
	if len(result.Breakdown) != len(actualCost.Breakdown) {
		t.Errorf("Expected %d breakdown entries, got %d", len(actualCost.Breakdown), len(result.Breakdown))
	}

	for service, cost := range actualCost.Breakdown {
		if result.Breakdown[service] != cost {
			t.Errorf("Expected %s cost %f, got %f", service, cost, result.Breakdown[service])
		}
	}
}

// TestActualCostCacheIntegration_TTLExpiration tests 6-hour TTL expiration
func TestActualCostCacheIntegration_TTLExpiration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create cache with short TTL for testing
	cache := NewActualCostCacheWithTTL(200 * time.Millisecond)
	deploymentID := "deploy_ttl_test"

	actualCost := &aws.ActualCostData{
		DeploymentID:     deploymentID,
		CostToDate:       50.00,
		ProjectedMonthly: 75.00,
		Variance:         5.0,
		Breakdown:        map[string]float64{"AmazonS3": 50.00},
		Period: aws.CostPeriod{
			Start: time.Now().AddDate(0, 0, -10),
			End:   time.Now(),
		},
		FetchedAt: time.Now(),
	}

	// Set cache entry
	cache.Set(deploymentID, actualCost)

	// Verify entry exists
	result := cache.Get(deploymentID)
	if result == nil {
		t.Fatal("Expected cache hit immediately after set")
	}

	// Wait for expiration
	time.Sleep(250 * time.Millisecond)

	// Verify entry is expired
	result = cache.Get(deploymentID)
	if result != nil {
		t.Error("Expected cache miss after TTL expiration")
	}

	// Verify cleanup removes expired entry
	count := cache.CleanExpired()
	if count != 1 {
		t.Errorf("Expected to clean 1 expired entry, got %d", count)
	}

	stats := cache.GetStats()
	if stats.TotalEntries != 0 {
		t.Errorf("Expected 0 entries after cleanup, got %d", stats.TotalEntries)
	}
}

// TestActualCostCacheIntegration_MultipleDeployments tests caching multiple deployments
func TestActualCostCacheIntegration_MultipleDeployments(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cache := NewActualCostCache()

	// Create multiple deployment cost data
	deployments := []string{"deploy_1", "deploy_2", "deploy_3", "deploy_4", "deploy_5"}

	for i, deploymentID := range deployments {
		actualCost := &aws.ActualCostData{
			DeploymentID:     deploymentID,
			CostToDate:       float64(100 + i*10),
			ProjectedMonthly: float64(150 + i*15),
			Variance:         float64(5 + i),
			Breakdown: map[string]float64{
				"AmazonEC2": float64(50 + i*5),
				"AmazonS3":  float64(50 + i*5),
			},
			Period: aws.CostPeriod{
				Start: time.Now().AddDate(0, 0, -10),
				End:   time.Now(),
			},
			FetchedAt: time.Now(),
		}

		err := cache.Set(deploymentID, actualCost)
		if err != nil {
			t.Fatalf("Failed to set cache entry for %s: %v", deploymentID, err)
		}
	}

	// Verify all entries are cached
	stats := cache.GetStats()
	if stats.TotalEntries != len(deployments) {
		t.Errorf("Expected %d entries, got %d", len(deployments), stats.TotalEntries)
	}

	// Verify each entry can be retrieved
	for i, deploymentID := range deployments {
		result := cache.Get(deploymentID)
		if result == nil {
			t.Errorf("Expected cache hit for deployment %s", deploymentID)
			continue
		}

		expectedCost := float64(100 + i*10)
		if result.CostToDate != expectedCost {
			t.Errorf("Expected cost %f for %s, got %f", expectedCost, deploymentID, result.CostToDate)
		}
	}
}

// TestActualCostCacheIntegration_CacheInvalidation tests cache invalidation scenarios
func TestActualCostCacheIntegration_CacheInvalidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cache := NewActualCostCache()

	// Create multiple deployments
	deployments := []string{"deploy_1", "deploy_2", "deploy_3"}

	for _, deploymentID := range deployments {
		actualCost := &aws.ActualCostData{
			DeploymentID:     deploymentID,
			CostToDate:       100.00,
			ProjectedMonthly: 150.00,
			Variance:         10.0,
			Breakdown:        map[string]float64{"AmazonS3": 100.00},
			Period: aws.CostPeriod{
				Start: time.Now().AddDate(0, 0, -10),
				End:   time.Now(),
			},
			FetchedAt: time.Now(),
		}

		cache.Set(deploymentID, actualCost)
	}

	// Verify all entries exist
	stats := cache.GetStats()
	if stats.TotalEntries != 3 {
		t.Errorf("Expected 3 entries, got %d", stats.TotalEntries)
	}

	// Invalidate one deployment
	cache.Invalidate("deploy_2")

	// Verify deploy_2 is gone but others remain
	if cache.Get("deploy_1") == nil {
		t.Error("Expected deploy_1 to still be cached")
	}

	if cache.Get("deploy_2") != nil {
		t.Error("Expected deploy_2 to be invalidated")
	}

	if cache.Get("deploy_3") == nil {
		t.Error("Expected deploy_3 to still be cached")
	}

	// Invalidate all
	cache.InvalidateAll()

	// Verify all entries are gone
	stats = cache.GetStats()
	if stats.TotalEntries != 0 {
		t.Errorf("Expected 0 entries after InvalidateAll, got %d", stats.TotalEntries)
	}
}

// TestActualCostCacheIntegration_CacheUpdate tests updating cached entries
func TestActualCostCacheIntegration_CacheUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cache := NewActualCostCache()
	deploymentID := "deploy_update_test"

	// Initial cost data
	initialCost := &aws.ActualCostData{
		DeploymentID:     deploymentID,
		CostToDate:       100.00,
		ProjectedMonthly: 150.00,
		Variance:         10.0,
		Breakdown:        map[string]float64{"AmazonS3": 100.00},
		Period: aws.CostPeriod{
			Start: time.Now().AddDate(0, 0, -10),
			End:   time.Now().AddDate(0, 0, -5),
		},
		FetchedAt: time.Now().Add(-5 * 24 * time.Hour),
	}

	cache.Set(deploymentID, initialCost)

	// Verify initial data
	result := cache.Get(deploymentID)
	if result == nil {
		t.Fatal("Expected cache hit for initial data")
	}

	if result.CostToDate != 100.00 {
		t.Errorf("Expected initial cost 100.00, got %f", result.CostToDate)
	}

	// Update with new cost data (simulating incremental update)
	updatedCost := &aws.ActualCostData{
		DeploymentID:     deploymentID,
		CostToDate:       150.00, // Increased cost
		ProjectedMonthly: 200.00,
		Variance:         15.0,
		Breakdown: map[string]float64{
			"AmazonS3":  100.00,
			"AmazonEC2": 50.00, // New service
		},
		Period: aws.CostPeriod{
			Start: time.Now().AddDate(0, 0, -10),
			End:   time.Now(),
		},
		FetchedAt: time.Now(),
	}

	cache.Set(deploymentID, updatedCost)

	// Verify updated data
	result = cache.Get(deploymentID)
	if result == nil {
		t.Fatal("Expected cache hit for updated data")
	}

	if result.CostToDate != 150.00 {
		t.Errorf("Expected updated cost 150.00, got %f", result.CostToDate)
	}

	if result.ProjectedMonthly != 200.00 {
		t.Errorf("Expected updated projected monthly 200.00, got %f", result.ProjectedMonthly)
	}

	if len(result.Breakdown) != 2 {
		t.Errorf("Expected 2 services in breakdown, got %d", len(result.Breakdown))
	}
}

// TestActualCostCacheIntegration_CleanupRoutine tests background cleanup routine
func TestActualCostCacheIntegration_CleanupRoutine(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create cache with short TTL
	cache := NewActualCostCacheWithTTL(100 * time.Millisecond)

	// Start cleanup routine with short interval
	stopChan := cache.StartCleanupRoutine(150 * time.Millisecond)
	defer close(stopChan)

	// Add entries
	for i := 0; i < 5; i++ {
		deploymentID := "deploy_" + string(rune('0'+i))
		actualCost := &aws.ActualCostData{
			DeploymentID:     deploymentID,
			CostToDate:       100.00,
			ProjectedMonthly: 150.00,
			Variance:         10.0,
			Breakdown:        map[string]float64{"AmazonS3": 100.00},
			Period: aws.CostPeriod{
				Start: time.Now().AddDate(0, 0, -10),
				End:   time.Now(),
			},
			FetchedAt: time.Now(),
		}
		cache.Set(deploymentID, actualCost)
	}

	// Verify entries exist
	stats := cache.GetStats()
	if stats.TotalEntries != 5 {
		t.Errorf("Expected 5 entries, got %d", stats.TotalEntries)
	}

	// Wait for entries to expire and cleanup to run
	time.Sleep(400 * time.Millisecond)

	// Verify entries were cleaned up
	stats = cache.GetStats()
	if stats.TotalEntries != 0 {
		t.Errorf("Expected 0 entries after cleanup, got %d", stats.TotalEntries)
	}
}

// TestActualCostCacheIntegration_CacheKeyConsistency tests cache key generation consistency
func TestActualCostCacheIntegration_CacheKeyConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cache := NewActualCostCache()
	deploymentID := "deploy_key_test"

	actualCost := &aws.ActualCostData{
		DeploymentID:     deploymentID,
		CostToDate:       100.00,
		ProjectedMonthly: 150.00,
		Variance:         10.0,
		Breakdown:        map[string]float64{"AmazonS3": 100.00},
		Period: aws.CostPeriod{
			Start: time.Now().AddDate(0, 0, -10),
			End:   time.Now(),
		},
		FetchedAt: time.Now(),
	}

	// Set entry
	cache.Set(deploymentID, actualCost)

	// Generate keys multiple times
	key1 := GenerateActualCostCacheKey(deploymentID)
	key2 := GenerateActualCostCacheKey(deploymentID)
	key3 := GenerateActualCostCacheKey(deploymentID)

	// Verify keys are consistent
	if key1 != key2 || key2 != key3 {
		t.Error("Expected consistent cache keys for same deployment ID")
	}

	// Verify we can retrieve using the same deployment ID
	result := cache.Get(deploymentID)
	if result == nil {
		t.Error("Expected cache hit with consistent key generation")
	}
}
