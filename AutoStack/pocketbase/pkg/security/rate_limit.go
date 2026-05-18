package security

import (
	"fmt"
	"sync"
	"time"
)

// RateLimitPolicy defines the maximum number of requests per window for a key.
type RateLimitPolicy struct {
	MaxRequests int
	Window      time.Duration
}

// DefaultAuthRateLimit is the production default for auth endpoints.
var DefaultAuthRateLimit = RateLimitPolicy{
	MaxRequests: 20,
	Window:      time.Minute,
}

// DefaultAPIRateLimit is the production default for API endpoints.
var DefaultAPIRateLimit = RateLimitPolicy{
	MaxRequests: 600,
	Window:      time.Minute,
}

// RateLimitDecision is the outcome of a rate limit check.
type RateLimitDecision struct {
	Allowed     bool
	Remaining   int
	ResetAt     time.Time
	Reason      string
}

// rateLimitBucket tracks request counts for a single key.
type rateLimitBucket struct {
	count     int
	windowEnd time.Time
}

// RateLimiter is a thread-safe, in-process token-bucket rate limiter.
// It is scoped to a single process — not shared across replicas.
// For distributed rate limiting, wire this to a shared store (Redis, PocketBase).
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateLimitBucket
	policy  RateLimitPolicy
}

// NewRateLimiter creates a RateLimiter with the given policy.
func NewRateLimiter(policy RateLimitPolicy) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*rateLimitBucket),
		policy:  policy,
	}
}

// Check tests and records a request for key. key is typically tenantID or IP.
// Returns RateLimitDecision — never blocks.
func (r *RateLimiter) Check(key string) RateLimitDecision {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	b, ok := r.buckets[key]
	if !ok || now.After(b.windowEnd) {
		b = &rateLimitBucket{
			count:     0,
			windowEnd: now.Add(r.policy.Window),
		}
		r.buckets[key] = b
	}

	b.count++
	remaining := r.policy.MaxRequests - b.count
	if remaining < 0 {
		remaining = 0
	}

	if b.count > r.policy.MaxRequests {
		return RateLimitDecision{
			Allowed:   false,
			Remaining: 0,
			ResetAt:   b.windowEnd,
			Reason:    fmt.Sprintf("rate limit exceeded: %d/%d requests in %s window", b.count, r.policy.MaxRequests, r.policy.Window),
		}
	}

	return RateLimitDecision{
		Allowed:   true,
		Remaining: remaining,
		ResetAt:   b.windowEnd,
	}
}

// Reset clears the rate limit bucket for a key. Useful in tests.
func (r *RateLimiter) Reset(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.buckets, key)
}

// Trim removes expired buckets to prevent unbounded memory growth.
// Call periodically (e.g., every minute) in production.
func (r *RateLimiter) Trim() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for k, b := range r.buckets {
		if now.After(b.windowEnd) {
			delete(r.buckets, k)
		}
	}
}
