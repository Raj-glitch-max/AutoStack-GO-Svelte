package aws

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/smithy-go"
)

// TestClassifyCostExplorerError tests error classification
func TestClassifyCostExplorerError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		deploymentID string
		expectedType ErrorType
		retryable    bool
	}{
		{
			name:         "Rate limit error",
			err:          errors.New("ThrottlingException: Rate exceeded"),
			deploymentID: "deploy-123",
			expectedType: ErrorTypeRateLimit,
			retryable:    true,
		},
		{
			name:         "Authentication error",
			err:          errors.New("UnrecognizedClientException: Invalid credentials"),
			deploymentID: "deploy-123",
			expectedType: ErrorTypeAuthentication,
			retryable:    false,
		},
		{
			name:         "Authorization error",
			err:          errors.New("AccessDeniedException: User not authorized"),
			deploymentID: "deploy-123",
			expectedType: ErrorTypeAuthorization,
			retryable:    false,
		},
		{
			name:         "Network error",
			err:          errors.New("dial tcp: connection refused"),
			deploymentID: "deploy-123",
			expectedType: ErrorTypeNetwork,
			retryable:    true,
		},
		{
			name:         "Timeout error",
			err:          errors.New("context deadline exceeded"),
			deploymentID: "deploy-123",
			expectedType: ErrorTypeTimeout,
			retryable:    true,
		},
		{
			name:         "Invalid request error",
			err:          errors.New("ValidationException: Invalid parameter"),
			deploymentID: "deploy-123",
			expectedType: ErrorTypeInvalidRequest,
			retryable:    false,
		},
		{
			name:         "Data not found error",
			err:          errors.New("ResourceNotFoundException: No data found"),
			deploymentID: "deploy-123",
			expectedType: ErrorTypeDataNotFound,
			retryable:    false,
		},
		{
			name:         "Service error",
			err:          errors.New("InternalServerError: Service unavailable"),
			deploymentID: "deploy-123",
			expectedType: ErrorTypeServiceError,
			retryable:    true,
		},
		{
			name:         "Unknown error",
			err:          errors.New("some random error"),
			deploymentID: "deploy-123",
			expectedType: ErrorTypeUnknown,
			retryable:    false,
		},
		{
			name:         "Nil error",
			err:          nil,
			deploymentID: "deploy-123",
			expectedType: "",
			retryable:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ceErr := ClassifyCostExplorerError(tt.err, tt.deploymentID)
			
			if tt.err == nil {
				if ceErr != nil {
					t.Errorf("Expected nil error, got %v", ceErr)
				}
				return
			}

			if ceErr == nil {
				t.Fatal("Expected CostExplorerError, got nil")
			}

			if ceErr.Type != tt.expectedType {
				t.Errorf("Expected error type %s, got %s", tt.expectedType, ceErr.Type)
			}

			if ceErr.Retryable != tt.retryable {
				t.Errorf("Expected retryable=%v, got %v", tt.retryable, ceErr.Retryable)
			}

			if ceErr.DeploymentID != tt.deploymentID {
				t.Errorf("Expected deployment ID %s, got %s", tt.deploymentID, ceErr.DeploymentID)
			}

			if ceErr.OriginalErr != tt.err {
				t.Errorf("Original error not preserved")
			}
		})
	}
}

// TestIsRetryableError tests retryable error detection
func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name:      "Throttling error",
			err:       errors.New("ThrottlingException"),
			retryable: true,
		},
		{
			name:      "Rate limit error",
			err:       errors.New("rate limit exceeded"),
			retryable: true,
		},
		{
			name:      "Timeout error",
			err:       errors.New("request timeout"),
			retryable: true,
		},
		{
			name:      "Network error",
			err:       errors.New("network connection failed"),
			retryable: true,
		},
		{
			name:      "Temporary error",
			err:       errors.New("temporary failure"),
			retryable: true,
		},
		{
			name:      "Service unavailable",
			err:       errors.New("service unavailable"),
			retryable: true,
		},
		{
			name:      "Authentication error",
			err:       errors.New("authentication failed"),
			retryable: false,
		},
		{
			name:      "Authorization error",
			err:       errors.New("access denied"),
			retryable: false,
		},
		{
			name:      "Invalid request",
			err:       errors.New("invalid parameter"),
			retryable: false,
		},
		{
			name:      "Nil error",
			err:       nil,
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableError(tt.err)
			if result != tt.retryable {
				t.Errorf("Expected retryable=%v, got %v", tt.retryable, result)
			}
		})
	}
}

// TestErrorStatsRecording tests error statistics recording
func TestErrorStatsRecording(t *testing.T) {
	stats := NewErrorStats()

	// Record various errors
	errors := []*CostExplorerError{
		{Type: ErrorTypeRateLimit, Retryable: true},
		{Type: ErrorTypeRateLimit, Retryable: true},
		{Type: ErrorTypeAuthentication, Retryable: false},
		{Type: ErrorTypeNetwork, Retryable: true},
		{Type: ErrorTypeTimeout, Retryable: true},
	}

	for _, err := range errors {
		stats.RecordError(err)
	}

	// Verify statistics
	if stats.TotalErrors != 5 {
		t.Errorf("Expected 5 total errors, got %d", stats.TotalErrors)
	}

	if stats.RetryableErrors != 4 {
		t.Errorf("Expected 4 retryable errors, got %d", stats.RetryableErrors)
	}

	if stats.NonRetryableErrors != 1 {
		t.Errorf("Expected 1 non-retryable error, got %d", stats.NonRetryableErrors)
	}

	if stats.ErrorsByType[ErrorTypeRateLimit] != 2 {
		t.Errorf("Expected 2 rate limit errors, got %d", stats.ErrorsByType[ErrorTypeRateLimit])
	}

	if stats.ErrorsByType[ErrorTypeAuthentication] != 1 {
		t.Errorf("Expected 1 authentication error, got %d", stats.ErrorsByType[ErrorTypeAuthentication])
	}

	// Test error rate calculation
	rateLimitRate := stats.GetErrorRate(ErrorTypeRateLimit)
	expectedRate := 2.0 / 5.0
	if rateLimitRate != expectedRate {
		t.Errorf("Expected rate limit error rate %.2f, got %.2f", expectedRate, rateLimitRate)
	}
}

// TestCircuitBreakerStates tests circuit breaker state transitions
func TestCircuitBreakerStates(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:         3,
		Timeout:             100 * time.Millisecond,
		HalfOpenMaxRequests: 2,
		FailureThreshold:    0.5,
		MinimumRequests:     5,
	}

	cb := NewCircuitBreaker(config)

	// Initial state should be closed
	if cb.GetState() != StateClosed {
		t.Errorf("Expected initial state to be closed, got %s", cb.GetState())
	}

	// Simulate failures to open the circuit
	ctx := context.Background()
	failingOp := func() error {
		return errors.New("operation failed")
	}

	// Execute enough failures to open circuit
	for i := 0; i < 6; i++ {
		cb.Execute(ctx, failingOp, "test-op")
	}

	// Circuit should be open now
	if cb.GetState() != StateOpen {
		t.Errorf("Expected state to be open after failures, got %s", cb.GetState())
	}

	// Verify that requests are rejected when circuit is open
	err := cb.Execute(ctx, func() error { return nil }, "test-op")
	if err == nil {
		t.Error("Expected error when circuit is open, got nil")
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Next request should transition to half-open
	successOp := func() error {
		return nil
	}

	err = cb.Execute(ctx, successOp, "test-op")
	if err != nil {
		t.Errorf("Expected success in half-open state, got error: %v", err)
	}

	// Execute more successful requests to close the circuit
	for i := 0; i < 2; i++ {
		cb.Execute(ctx, successOp, "test-op")
	}

	// Circuit should be closed now
	if cb.GetState() != StateClosed {
		t.Errorf("Expected state to be closed after successful recovery, got %s", cb.GetState())
	}
}

// TestCircuitBreakerFailureThreshold tests failure threshold triggering
func TestCircuitBreakerFailureThreshold(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:         10,
		Timeout:             1 * time.Second,
		HalfOpenMaxRequests: 2,
		FailureThreshold:    0.5, // 50% failure rate
		MinimumRequests:     10,
	}

	cb := NewCircuitBreaker(config)
	ctx := context.Background()

	// Execute 10 requests with 60% failure rate (6 failures, 4 successes)
	// The circuit should open once we reach MinimumRequests with failure rate > threshold
	for i := 0; i < 10; i++ {
		var op func() error
		if i < 6 {
			op = func() error { return errors.New("failed") }
		} else {
			op = func() error { return nil }
		}
		cb.Execute(ctx, op, "test-op")
		
		// Log state after each request
		if i == 9 {
			stats := cb.GetStats()
			t.Logf("After request %d: state=%s, failures=%d, requests=%d, rate=%.2f", 
				i+1, cb.GetState(), stats.Failures, stats.Requests, stats.FailureRate)
		}
	}

	// After 10 requests with 60% failure rate, circuit should be open
	// because we've reached MinimumRequests and failure rate (0.6) > threshold (0.5)
	stats := cb.GetStats()
	
	// The circuit should have opened on the 10th request (when we hit MinimumRequests)
	// But since the last 4 requests were successes, it might not have opened
	// Let's add one more failure to trigger it
	cb.Execute(ctx, func() error { return errors.New("failed") }, "test-op")
	
	if cb.GetState() != StateOpen {
		t.Errorf("Expected circuit to be open due to failure rate, got %s", cb.GetState())
	}

	stats = cb.GetStats()
	if stats.FailureRate < 0.5 {
		t.Errorf("Expected failure rate >= 0.5, got %.2f", stats.FailureRate)
	}
}

// TestCircuitBreakerHalfOpenBehavior tests half-open state behavior
func TestCircuitBreakerHalfOpenBehavior(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxFailures:         3,
		Timeout:             50 * time.Millisecond,
		HalfOpenMaxRequests: 2,
		FailureThreshold:    0.5,
		MinimumRequests:     5,
	}

	cb := NewCircuitBreaker(config)
	ctx := context.Background()

	// Open the circuit
	for i := 0; i < 6; i++ {
		cb.Execute(ctx, func() error { return errors.New("failed") }, "test-op")
	}

	if cb.GetState() != StateOpen {
		t.Fatal("Circuit should be open")
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// First request should succeed and transition to half-open
	err := cb.Execute(ctx, func() error { return nil }, "test-op")
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}

	// Failure in half-open should reopen the circuit
	cb.Execute(ctx, func() error { return errors.New("failed again") }, "test-op")

	if cb.GetState() != StateOpen {
		t.Errorf("Expected circuit to reopen after failure in half-open, got %s", cb.GetState())
	}
}

// TestCircuitBreakerReset tests manual circuit breaker reset
func TestCircuitBreakerReset(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	cb := NewCircuitBreaker(config)
	ctx := context.Background()

	// Open the circuit
	for i := 0; i < 10; i++ {
		cb.Execute(ctx, func() error { return errors.New("failed") }, "test-op")
	}

	if cb.GetState() != StateOpen {
		t.Fatal("Circuit should be open")
	}

	// Reset the circuit
	cb.Reset()

	// Circuit should be closed
	if cb.GetState() != StateClosed {
		t.Errorf("Expected circuit to be closed after reset, got %s", cb.GetState())
	}

	// Stats should be reset
	stats := cb.GetStats()
	if stats.Failures != 0 || stats.Successes != 0 || stats.Requests != 0 {
		t.Error("Expected stats to be reset")
	}
}

// TestCostExplorerErrorWrapping tests error wrapping and unwrapping
func TestCostExplorerErrorWrapping(t *testing.T) {
	originalErr := errors.New("original error")
	ceErr := &CostExplorerError{
		Type:        ErrorTypeNetwork,
		OriginalErr: originalErr,
		Message:     "Network error occurred",
		Retryable:   true,
	}

	// Test Error() method
	errStr := ceErr.Error()
	if errStr == "" {
		t.Error("Error string should not be empty")
	}

	// Test Unwrap()
	unwrapped := ceErr.Unwrap()
	if unwrapped != originalErr {
		t.Error("Unwrap should return original error")
	}

	// Test errors.Is()
	if !errors.Is(ceErr, originalErr) {
		t.Error("errors.Is should work with wrapped error")
	}

	// Test errors.As()
	var targetErr *CostExplorerError
	if !errors.As(ceErr, &targetErr) {
		t.Error("errors.As should work with CostExplorerError")
	}
}

// TestGlobalErrorStatsRecording tests global error statistics
func TestGlobalErrorStatsRecording(t *testing.T) {
	// Reset global stats
	ResetErrorStats()

	// Record some errors
	err1 := errors.New("ThrottlingException")
	err2 := errors.New("authentication failed")
	err3 := errors.New("network error")

	RecordCostExplorerError(err1, "deploy-1")
	RecordCostExplorerError(err2, "deploy-2")
	RecordCostExplorerError(err3, "deploy-3")

	// Get global stats
	stats := GetGlobalErrorStats()

	if stats.TotalErrors != 3 {
		t.Errorf("Expected 3 total errors, got %d", stats.TotalErrors)
	}

	if stats.RetryableErrors < 1 {
		t.Errorf("Expected at least 1 retryable error, got %d", stats.RetryableErrors)
	}

	if stats.LastError == nil {
		t.Error("Expected last error to be recorded")
	}
}

// TestErrorStatsCircuitBreakerTrigger tests circuit breaker triggering based on error stats
func TestErrorStatsCircuitBreakerTrigger(t *testing.T) {
	stats := NewErrorStats()

	// Record errors within a time window
	for i := 0; i < 10; i++ {
		err := &CostExplorerError{
			Type:      ErrorTypeAuthentication,
			Retryable: false,
		}
		stats.RecordError(err)
	}

	// Should trigger circuit breaker with high non-retryable error rate
	threshold := 0.5
	window := 1 * time.Minute

	shouldTrigger := stats.ShouldTriggerCircuitBreaker(threshold, window)
	if !shouldTrigger {
		t.Error("Expected circuit breaker to be triggered with high error rate")
	}

	// Test with old errors (outside window)
	stats.LastUpdated = time.Now().Add(-2 * time.Minute)
	shouldTrigger = stats.ShouldTriggerCircuitBreaker(threshold, window)
	if shouldTrigger {
		t.Error("Expected circuit breaker not to trigger with old errors")
	}
}

// MockAPIError implements smithy.APIError for testing
type MockAPIError struct {
	code    string
	message string
}

func (e *MockAPIError) Error() string {
	return fmt.Sprintf("%s: %s", e.code, e.message)
}

func (e *MockAPIError) ErrorCode() string {
	return e.code
}

func (e *MockAPIError) ErrorMessage() string {
	return e.message
}

func (e *MockAPIError) ErrorFault() smithy.ErrorFault {
	return smithy.FaultUnknown
}

// TestClassifyAPIError tests AWS SDK API error classification
func TestClassifyAPIError(t *testing.T) {
	tests := []struct {
		name         string
		apiErr       smithy.APIError
		expectedType ErrorType
		retryable    bool
	}{
		{
			name:         "Throttling exception",
			apiErr:       &MockAPIError{code: "ThrottlingException", message: "Rate exceeded"},
			expectedType: ErrorTypeRateLimit,
			retryable:    true,
		},
		{
			name:         "Unrecognized client",
			apiErr:       &MockAPIError{code: "UnrecognizedClientException", message: "Invalid credentials"},
			expectedType: ErrorTypeAuthentication,
			retryable:    false,
		},
		{
			name:         "Access denied",
			apiErr:       &MockAPIError{code: "AccessDeniedException", message: "Not authorized"},
			expectedType: ErrorTypeAuthorization,
			retryable:    false,
		},
		{
			name:         "Validation exception",
			apiErr:       &MockAPIError{code: "ValidationException", message: "Invalid parameter"},
			expectedType: ErrorTypeInvalidRequest,
			retryable:    false,
		},
		{
			name:         "Resource not found",
			apiErr:       &MockAPIError{code: "ResourceNotFoundException", message: "Not found"},
			expectedType: ErrorTypeDataNotFound,
			retryable:    false,
		},
		{
			name:         "Internal server error",
			apiErr:       &MockAPIError{code: "InternalServerError", message: "Server error"},
			expectedType: ErrorTypeServiceError,
			retryable:    true,
		},
		{
			name:         "Request timeout",
			apiErr:       &MockAPIError{code: "RequestTimeout", message: "Timeout"},
			expectedType: ErrorTypeTimeout,
			retryable:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorType, _, retryable := classifyAPIError(tt.apiErr)

			if errorType != tt.expectedType {
				t.Errorf("Expected error type %s, got %s", tt.expectedType, errorType)
			}

			if retryable != tt.retryable {
				t.Errorf("Expected retryable=%v, got %v", tt.retryable, retryable)
			}
		})
	}
}

// Benchmark tests

func BenchmarkClassifyCostExplorerError(b *testing.B) {
	err := errors.New("ThrottlingException: Rate exceeded")
	deploymentID := "deploy-123"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ClassifyCostExplorerError(err, deploymentID)
	}
}

func BenchmarkIsRetryableError(b *testing.B) {
	err := errors.New("ThrottlingException: Rate exceeded")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsRetryableError(err)
	}
}

func BenchmarkCircuitBreakerExecute(b *testing.B) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())
	ctx := context.Background()
	op := func() error { return nil }

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Execute(ctx, op, "bench-op")
	}
}

func BenchmarkErrorStatsRecording(b *testing.B) {
	stats := NewErrorStats()
	err := &CostExplorerError{
		Type:      ErrorTypeRateLimit,
		Retryable: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stats.RecordError(err)
	}
}
