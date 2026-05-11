package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
)

// ActualCostCacheEntry represents a cached actual cost with metadata
type ActualCostCacheEntry struct {
	ActualCost *aws.ActualCostData `json:"actualCost"`
	CachedAt   time.Time           `json:"cachedAt"`
	ExpiresAt  time.Time           `json:"expiresAt"`
	DeploymentID string            `json:"deploymentId"`
}

// ActualCostCache provides in-memory caching for actual cost API responses
// **Validates**: TR-3.1 (Cost estimate endpoint responds in <500ms) and Cost Explorer rate limits
type ActualCostCache struct {
	cache      map[string]*ActualCostCacheEntry
	mu         sync.RWMutex
	ttl        time.Duration
	maxEntries int
}

// NewActualCostCache creates a new actual cost cache with 6-hour TTL
// **Validates**: 6-hour TTL for actual cost responses (daily cost updates)
func NewActualCostCache() *ActualCostCache {
	return &ActualCostCache{
		cache:      make(map[string]*ActualCostCacheEntry),
		ttl:        6 * time.Hour, // 6-hour TTL per requirement
		maxEntries: 500,           // Prevent unbounded memory growth
	}
}

// NewActualCostCacheWithTTL creates a new actual cost cache with custom TTL
// Useful for testing with shorter TTLs
func NewActualCostCacheWithTTL(ttl time.Duration) *ActualCostCache {
	return &ActualCostCache{
		cache:      make(map[string]*ActualCostCacheEntry),
		ttl:        ttl,
		maxEntries: 500,
	}
}

// GenerateActualCostCacheKey generates a unique cache key from deployment ID
// Cache key is based on deployment ID as specified in requirements
// **Validates**: Cache key should be based on deployment ID
func GenerateActualCostCacheKey(deploymentID string) string {
	// Generate SHA256 hash for compact, unique key
	hash := sha256.Sum256([]byte(deploymentID))
	return hex.EncodeToString(hash[:])
}

// Get retrieves a cached actual cost if it exists and hasn't expired
// Returns nil if cache miss or expired entry
// **Validates**: Return cached response if available and not expired
func (acc *ActualCostCache) Get(deploymentID string) *aws.ActualCostData {
	acc.mu.RLock()
	defer acc.mu.RUnlock()

	key := GenerateActualCostCacheKey(deploymentID)
	entry, exists := acc.cache[key]

	if !exists {
		return nil
	}

	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		// Entry expired, will be cleaned up later
		return nil
	}

	return entry.ActualCost
}

// Set stores an actual cost in the cache with 6-hour TTL
// **Validates**: Cache actual costs for 6 hours (appropriate for daily cost updates)
func (acc *ActualCostCache) Set(deploymentID string, actualCost *aws.ActualCostData) error {
	if actualCost == nil {
		return fmt.Errorf("actualCost cannot be nil")
	}

	acc.mu.Lock()
	defer acc.mu.Unlock()

	// Check if we need to evict entries
	if len(acc.cache) >= acc.maxEntries {
		acc.evictOldestUnlocked()
	}

	key := GenerateActualCostCacheKey(deploymentID)
	now := time.Now()

	entry := &ActualCostCacheEntry{
		ActualCost:   actualCost,
		CachedAt:     now,
		ExpiresAt:    now.Add(acc.ttl),
		DeploymentID: deploymentID,
	}

	acc.cache[key] = entry
	return nil
}

// Invalidate removes a specific cache entry
// **Validates**: Invalidate cache when new cost data is fetched
func (acc *ActualCostCache) Invalidate(deploymentID string) {
	acc.mu.Lock()
	defer acc.mu.Unlock()

	key := GenerateActualCostCacheKey(deploymentID)
	delete(acc.cache, key)
}

// InvalidateAll clears the entire cache
// **Validates**: Implement cache invalidation mechanism
func (acc *ActualCostCache) InvalidateAll() {
	acc.mu.Lock()
	defer acc.mu.Unlock()

	acc.cache = make(map[string]*ActualCostCacheEntry)
}

// CleanExpired removes all expired entries from the cache
// Should be called periodically to prevent memory leaks
func (acc *ActualCostCache) CleanExpired() int {
	acc.mu.Lock()
	defer acc.mu.Unlock()

	now := time.Now()
	count := 0

	for key, entry := range acc.cache {
		if now.After(entry.ExpiresAt) {
			delete(acc.cache, key)
			count++
		}
	}

	return count
}

// evictOldestUnlocked removes the oldest cache entry
// Must be called with lock held
func (acc *ActualCostCache) evictOldestUnlocked() {
	var oldestKey string
	var oldestTime time.Time

	// Find oldest entry
	for key, entry := range acc.cache {
		if oldestKey == "" || entry.CachedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.CachedAt
		}
	}

	if oldestKey != "" {
		delete(acc.cache, oldestKey)
	}
}

// GetStats returns cache statistics
func (acc *ActualCostCache) GetStats() ActualCostCacheStats {
	acc.mu.RLock()
	defer acc.mu.RUnlock()

	now := time.Now()
	activeCount := 0
	expiredCount := 0

	for _, entry := range acc.cache {
		if now.After(entry.ExpiresAt) {
			expiredCount++
		} else {
			activeCount++
		}
	}

	return ActualCostCacheStats{
		TotalEntries:   len(acc.cache),
		ActiveEntries:  activeCount,
		ExpiredEntries: expiredCount,
		MaxEntries:     acc.maxEntries,
		TTL:            acc.ttl,
	}
}

// ActualCostCacheStats represents cache statistics
type ActualCostCacheStats struct {
	TotalEntries   int           `json:"totalEntries"`
	ActiveEntries  int           `json:"activeEntries"`
	ExpiredEntries int           `json:"expiredEntries"`
	MaxEntries     int           `json:"maxEntries"`
	TTL            time.Duration `json:"ttl"`
}

// GetEntry retrieves a full cache entry with metadata
// Useful for debugging and monitoring
func (acc *ActualCostCache) GetEntry(deploymentID string) *ActualCostCacheEntry {
	acc.mu.RLock()
	defer acc.mu.RUnlock()

	key := GenerateActualCostCacheKey(deploymentID)
	entry, exists := acc.cache[key]

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
func (acc *ActualCostCache) StartCleanupRoutine(interval time.Duration) chan struct{} {
	stopChan := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				count := acc.CleanExpired()
				if count > 0 {
					fmt.Printf("Actual cost cache cleanup: removed %d expired entries\n", count)
				}
			case <-stopChan:
				return
			}
		}
	}()

	return stopChan
}
