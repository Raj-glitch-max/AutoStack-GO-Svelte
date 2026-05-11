package controller

import (
	"net/http"
	"sync"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/models"
	"golang.org/x/time/rate"
)

// IPRateLimiter stores rate limiters for each identifier (IP or User ID)
type IPRateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	r        rate.Limit
	b        int
}

// NewIPRateLimiter creates a new rate limiter manager
func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	return &IPRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		r:        r,
		b:        b,
	}
}

// GetLimiter returns the rate limiter for a specific identifier
func (i *IPRateLimiter) GetLimiter(id string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	limiter, exists := i.limiters[id]
	if !exists {
		limiter = rate.NewLimiter(i.r, i.b)
		i.limiters[id] = limiter
	}

	return limiter
}

// RateLimitMiddleware returns an echo middleware that enforces rate limits.
// It uses the authenticated user ID if available, otherwise falls back to the client IP.
func RateLimitMiddleware(requestsPerSecond float64, burst int) echo.MiddlewareFunc {
	limiterManager := NewIPRateLimiter(rate.Limit(requestsPerSecond), burst)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Identify the caller: use User ID if authenticated, fallback to Real IP
			identifier := c.RealIP()
			user, _ := c.Get(apis.ContextAuthRecordKey).(*models.Record)
			if user != nil {
				identifier = user.Id
			}

			// Check if allowed
			if !limiterManager.GetLimiter(identifier).Allow() {
				return echo.NewHTTPError(http.StatusTooManyRequests, "Rate limit exceeded. Please try again later.")
			}

			return next(c)
		}
	}
}
