package aws

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// CircuitState represents the state of the circuit breaker
type CircuitState string

const (
	StateClosed   CircuitState = "closed"   // Normal operation
	StateOpen     CircuitState = "open"     // Failing, rejecting requests
	StateHalfOpen CircuitState = "half_open" // Testing if service recovered
)

// CircuitBreakerConfig defines circuit breaker configuration
type CircuitBreakerConfig struct {
	// MaxFailures is the number of failures before opening the circuit
	MaxFailures int

	// Timeout is how long to wait before attempting to close the circuit
	Timeout time.Duration

	// HalfOpenMaxRequests is the max requests allowed in half-open state
	HalfOpenMaxRequests int

	// FailureThreshold is the percentage of failures that triggers opening (0.0-1.0)
	FailureThreshold float64

	// MinimumRequests is the minimum number of requests before calculating failure rate
	MinimumRequests int
}

// DefaultCircuitBreakerConfig returns default circuit breaker configuration
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxFailures:         5,
		Timeout:             60 * time.Second,
		HalfOpenMaxRequests: 3,
		FailureThreshold:    0.5, // 50% failure rate
		MinimumRequests:     10,
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config       CircuitBreakerConfig
	state        CircuitState
	failures     int
	successes    int
	requests     int
	lastFailTime time.Time
	halfOpenReqs int
	mu           sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

// Execute executes an operation with circuit breaker protection
func (cb *CircuitBreaker) Execute(ctx context.Context, operation func() error, operationName string) error {
	// Check if circuit breaker allows the request
	if err := cb.beforeRequest(operationName); err != nil {
		return err
	}

	// Execute the operation
	err := operation()

	// Record the result
	cb.afterRequest(err, operationName)

	return err
}

// beforeRequest checks if the request should be allowed
func (cb *CircuitBreaker) beforeRequest(operationName string) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateOpen:
		// Check if timeout has elapsed
		if time.Since(cb.lastFailTime) > cb.config.Timeout {
			log.Printf("Circuit breaker for %s transitioning to half-open state", operationName)
			cb.state = StateHalfOpen
			cb.halfOpenReqs = 0
			return nil
		}
		return fmt.Errorf("circuit breaker is open for %s (too many failures)", operationName)

	case StateHalfOpen:
		// Limit requests in half-open state
		if cb.halfOpenReqs >= cb.config.HalfOpenMaxRequests {
			return fmt.Errorf("circuit breaker is half-open for %s (max test requests reached)", operationName)
		}
		cb.halfOpenReqs++
		return nil

	case StateClosed:
		return nil

	default:
		return nil
	}
}

// afterRequest records the result of a request
func (cb *CircuitBreaker) afterRequest(err error, operationName string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.requests++

	if err != nil {
		cb.failures++
		cb.lastFailTime = time.Now()
	} else {
		cb.successes++
	}

	// Check circuit breaker state transitions based on current state
	switch cb.state {
	case StateClosed:
		// Check if we should open the circuit (on any request, not just failures)
		if err != nil && cb.shouldOpen() {
			log.Printf("Circuit breaker for %s opening (failures: %d/%d, rate: %.2f%%)",
				operationName, cb.failures, cb.requests, cb.getFailureRate()*100)
			cb.state = StateOpen
		}

	case StateHalfOpen:
		if err != nil {
			// Any failure in half-open state reopens the circuit
			log.Printf("Circuit breaker for %s reopening (failure in half-open state)", operationName)
			cb.state = StateOpen
			cb.resetCounters()
		} else {
			// Check if we should close the circuit after successful requests
			if cb.halfOpenReqs >= cb.config.HalfOpenMaxRequests {
				log.Printf("Circuit breaker for %s closing (successful recovery)", operationName)
				cb.state = StateClosed
				cb.resetCounters()
			}
		}
	}
}

// shouldOpen determines if the circuit should open
func (cb *CircuitBreaker) shouldOpen() bool {
	// Need minimum requests before opening
	if cb.requests < cb.config.MinimumRequests {
		return false
	}

	// Check if failure count exceeds max
	if cb.failures >= cb.config.MaxFailures {
		return true
	}

	// Check if failure rate exceeds threshold
	return cb.getFailureRate() >= cb.config.FailureThreshold
}

// getFailureRate calculates the current failure rate
func (cb *CircuitBreaker) getFailureRate() float64 {
	if cb.requests == 0 {
		return 0.0
	}
	return float64(cb.failures) / float64(cb.requests)
}

// resetCounters resets the internal counters
func (cb *CircuitBreaker) resetCounters() {
	cb.failures = 0
	cb.successes = 0
	cb.requests = 0
	cb.halfOpenReqs = 0
}

// GetState returns the current circuit breaker state
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStats returns circuit breaker statistics
func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:        cb.state,
		Failures:     cb.failures,
		Successes:    cb.successes,
		Requests:     cb.requests,
		FailureRate:  cb.getFailureRate(),
		LastFailTime: cb.lastFailTime,
	}
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	log.Printf("Circuit breaker manually reset to closed state")
	cb.state = StateClosed
	cb.resetCounters()
}

// CircuitBreakerStats contains circuit breaker statistics
type CircuitBreakerStats struct {
	State        CircuitState  `json:"state"`
	Failures     int           `json:"failures"`
	Successes    int           `json:"successes"`
	Requests     int           `json:"requests"`
	FailureRate  float64       `json:"failureRate"`
	LastFailTime time.Time     `json:"lastFailTime"`
}

// IsCircuitBreakerOpen checks if a circuit breaker error occurred
func IsCircuitBreakerOpen(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == "circuit breaker is open" ||
		   err.Error() == "circuit breaker is half-open"
}

// CostExplorerCircuitBreaker is a global circuit breaker for Cost Explorer API
var costExplorerCircuitBreaker = NewCircuitBreaker(DefaultCircuitBreakerConfig())

// GetCostExplorerCircuitBreaker returns the global Cost Explorer circuit breaker
func GetCostExplorerCircuitBreaker() *CircuitBreaker {
	return costExplorerCircuitBreaker
}

// ResetCostExplorerCircuitBreaker resets the global circuit breaker (useful for testing)
func ResetCostExplorerCircuitBreaker() {
	costExplorerCircuitBreaker.Reset()
}
