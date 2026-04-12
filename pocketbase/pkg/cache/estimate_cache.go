package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/cost"
)

// CacheEntry represents a cached cost estimate with metadata
type CacheEntry struct {
	Estimate  *cost.CostEstimateWithRange `json:"estimate"`
	CachedAt  time.Time                   `json:"cachedAt"`
	ExpiresAt time.Time                   `json:"expiresAt"`
	Blueprint string                      `json:"blueprint"`
	Region    string                      `json:"region"`
	Variables map[string]interface{}      `json:"variables"`
}

// EstimateCache provides in-memory caching for cost estimates
// **Validates**: TR-3.1 (Cost estimate endpoint responds in <500ms)
type EstimateCache struct {
	cache      map[string]*CacheEntry
	mu         sync.RWMutex
	ttl        time.Duration
	maxEntries int
}

// NewEstimateCache creates a new estimate cache with 1-hour TTL
// **Validates**: TR-3.1 (Cost estimate endpoint responds in <500ms)
func NewEstimateCache() *EstimateCache {
	return &EstimateCache{
		cache:      make(map[string]*CacheEntry),
		ttl:        1 * time.Hour, // 1-hour TTL per requirement
		maxEntries: 1000,          // Prevent unbounded memory growth
	}
}

// NewEstimateCacheWithTTL creates a new estimate cache with custom TTL
// Useful for testing with shorter TTLs
func NewEstimateCacheWithTTL(ttl time.Duration) *EstimateCache {
	return &EstimateCache{
		cache:      make(map[string]*CacheEntry),
		ttl:        ttl,
		maxEntries: 1000,
	}
}

// GenerateCacheKey generates a unique cache key from blueprint, region, and variables
// Cache key is based on blueprint + region combination as specified in requirements
// **Validates**: Cache key should be based on blueprint + region combination
func GenerateCacheKey(blueprint, region string, variables map[string]interface{}) string {
	// Create a deterministic representation of the request
	keyData := map[string]interface{}{
		"blueprint": blueprint,
		"region":    region,
		"variables": variables,
	}

	// Serialize to JSON for consistent hashing
	jsonData, err := json.Marshal(keyData)
	if err != nil {
		// Fallback to simple concatenation if JSON fails
		return fmt.Sprintf("%s:%s", blueprint, region)
	}

	// Generate SHA256 hash for compact, unique key
	hash := sha256.Sum256(jsonData)
	return hex.EncodeToString(hash[:])
}

// Get retrieves a cached estimate if it exists and hasn't expired
// Returns nil if cache miss or expired entry
// **Validates**: TR-3.1 (Cost estimate endpoint responds in <500ms)
func (ec *EstimateCache) Get(blueprint, region string, variables map[string]interface{}) *cost.CostEstimateWithRange {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	key := GenerateCacheKey(blueprint, region, variables)
	entry, exists := ec.cache[key]

	if !exists {
		return nil
	}

	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		// Entry expired, will be cleaned up later
		return nil
	}

	return entry.Estimate
}

// Set stores a cost estimate in the cache with 1-hour TTL
// **Validates**: Cache estimates for 1 hour to meet <500ms response time requirement
func (ec *EstimateCache) Set(blueprint, region string, variables map[string]interface{}, estimate *cost.CostEstimateWithRange) error {
	if estimate == nil {
		return fmt.Errorf("estimate cannot be nil")
	}

	ec.mu.Lock()
	defer ec.mu.Unlock()

	// Check if we need to evict entries
	if len(ec.cache) >= ec.maxEntries {
		ec.evictOldestUnlocked()
	}

	key := GenerateCacheKey(blueprint, region, variables)
	now := time.Now()

	entry := &CacheEntry{
		Estimate:  estimate,
		CachedAt:  now,
		ExpiresAt: now.Add(ec.ttl),
		Blueprint: blueprint,
		Region:    region,
		Variables: variables,
	}

	ec.cache[key] = entry
	return nil
}

// Invalidate removes a specific cache entry
// **Validates**: Implement cache invalidation mechanism
func (ec *EstimateCache) Invalidate(blueprint, region string, variables map[string]interface{}) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	key := GenerateCacheKey(blueprint, region, variables)
	delete(ec.cache, key)
}

// InvalidateRegion removes all cache entries for a specific region
// Useful when pricing data is refreshed for a region
// **Validates**: Implement cache invalidation when pricing data is refreshed
func (ec *EstimateCache) InvalidateRegion(region string) int {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	count := 0
	for key, entry := range ec.cache {
		if entry.Region == region {
			delete(ec.cache, key)
			count++
		}
	}

	return count
}

// InvalidateBlueprint removes all cache entries for a specific blueprint type
// Useful when blueprint calculation logic changes
func (ec *EstimateCache) InvalidateBlueprint(blueprint string) int {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	count := 0
	for key, entry := range ec.cache {
		if entry.Blueprint == blueprint {
			delete(ec.cache, key)
			count++
		}
	}

	return count
}

// InvalidateAll clears the entire cache
// **Validates**: Implement cache invalidation when pricing data is refreshed
func (ec *EstimateCache) InvalidateAll() {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.cache = make(map[string]*CacheEntry)
}

// CleanExpired removes all expired entries from the cache
// Should be called periodically to prevent memory leaks
func (ec *EstimateCache) CleanExpired() int {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	now := time.Now()
	count := 0

	for key, entry := range ec.cache {
		if now.After(entry.ExpiresAt) {
			delete(ec.cache, key)
			count++
		}
	}

	return count
}

// evictOldestUnlocked removes the oldest cache entry
// Must be called with lock held
func (ec *EstimateCache) evictOldestUnlocked() {
	var oldestKey string
	var oldestTime time.Time

	// Find oldest entry
	for key, entry := range ec.cache {
		if oldestKey == "" || entry.CachedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.CachedAt
		}
	}

	if oldestKey != "" {
		delete(ec.cache, oldestKey)
	}
}

// GetStats returns cache statistics
func (ec *EstimateCache) GetStats() CacheStats {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	now := time.Now()
	activeCount := 0
	expiredCount := 0

	for _, entry := range ec.cache {
		if now.After(entry.ExpiresAt) {
			expiredCount++
		} else {
			activeCount++
		}
	}

	return CacheStats{
		TotalEntries:   len(ec.cache),
		ActiveEntries:  activeCount,
		ExpiredEntries: expiredCount,
		MaxEntries:     ec.maxEntries,
		TTL:            ec.ttl,
	}
}

// CacheStats represents cache statistics
type CacheStats struct {
	TotalEntries   int           `json:"totalEntries"`
	ActiveEntries  int           `json:"activeEntries"`
	ExpiredEntries int           `json:"expiredEntries"`
	MaxEntries     int           `json:"maxEntries"`
	TTL            time.Duration `json:"ttl"`
}

// GetEntry retrieves a full cache entry with metadata
// Useful for debugging and monitoring
func (ec *EstimateCache) GetEntry(blueprint, region string, variables map[string]interface{}) *CacheEntry {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	key := GenerateCacheKey(blueprint, region, variables)
	entry, exists := ec.cache[key]

	if !exists {
		return nil
	}

	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		return nil
	}

	return entry
}

// StartCleanupRoutine starts a background goroutine that periodically cleans expired entries
// Returns a channel that can be closed to stop the cleanup routine
func (ec *EstimateCache) StartCleanupRoutine(interval time.Duration) chan struct{} {
	stopChan := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				count := ec.CleanExpired()
				if count > 0 {
					fmt.Printf("Cache cleanup: removed %d expired entries\n", count)
				}
			case <-stopChan:
				return
			}
		}
	}()

	return stopChan
}
