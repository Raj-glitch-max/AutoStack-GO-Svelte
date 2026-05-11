package aws

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/smithy-go"
)

// CostExplorerError represents a categorized Cost Explorer API error
type CostExplorerError struct {
	Type         ErrorType
	OriginalErr  error
	Message      string
	Retryable    bool
	DeploymentID string
	Timestamp    time.Time
}

// ErrorType categorizes different types of Cost Explorer errors
type ErrorType string

const (
	ErrorTypeRateLimit       ErrorType = "RateLimit"
	ErrorTypeAuthentication  ErrorType = "Authentication"
	ErrorTypeAuthorization   ErrorType = "Authorization"
	ErrorTypeNetwork         ErrorType = "Network"
	ErrorTypeTimeout         ErrorType = "Timeout"
	ErrorTypeInvalidRequest  ErrorType = "InvalidRequest"
	ErrorTypeDataNotFound    ErrorType = "DataNotFound"
	ErrorTypeServiceError    ErrorType = "ServiceError"
	ErrorTypeUnknown         ErrorType = "Unknown"
)

// Error implements the error interface
func (e *CostExplorerError) Error() string {
	return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.OriginalErr)
}

// Unwrap returns the original error for error chain support
func (e *CostExplorerError) Unwrap() error {
	return e.OriginalErr
}

// IsRetryable returns whether the error should be retried
func (e *CostExplorerError) IsRetryable() bool {
	return e.Retryable
}

// ClassifyCostExplorerError categorizes a Cost Explorer API error
func ClassifyCostExplorerError(err error, deploymentID string) *CostExplorerError {
	if err == nil {
		return nil
	}

	ceErr := &CostExplorerError{
		OriginalErr:  err,
		DeploymentID: deploymentID,
		Timestamp:    time.Now(),
	}

	// Check for AWS SDK specific errors
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		ceErr.Type, ceErr.Message, ceErr.Retryable = classifyAPIError(apiErr)
		return ceErr
	}

	// Check for common error patterns in error string
	errStr := strings.ToLower(err.Error())

	switch {
	case strings.Contains(errStr, "throttl") || strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "too many requests"):
		ceErr.Type = ErrorTypeRateLimit
		ceErr.Message = "API rate limit exceeded"
		ceErr.Retryable = true

	case strings.Contains(errStr, "unauthorized") || strings.Contains(errStr, "authentication") || strings.Contains(errStr, "credentials"):
		ceErr.Type = ErrorTypeAuthentication
		ceErr.Message = "Authentication failed - check AWS credentials"
		ceErr.Retryable = false

	case strings.Contains(errStr, "accessdenied") || strings.Contains(errStr, "access denied") || strings.Contains(errStr, "forbidden") || strings.Contains(errStr, "permission") || strings.Contains(errStr, "not authorized"):
		ceErr.Type = ErrorTypeAuthorization
		ceErr.Message = "Authorization failed - insufficient permissions for Cost Explorer"
		ceErr.Retryable = false

	case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded"):
		ceErr.Type = ErrorTypeTimeout
		ceErr.Message = "Request timeout - Cost Explorer API did not respond in time"
		ceErr.Retryable = true

	case strings.Contains(errStr, "connection") || strings.Contains(errStr, "network") || strings.Contains(errStr, "dial") || strings.Contains(errStr, "dns"):
		ceErr.Type = ErrorTypeNetwork
		ceErr.Message = "Network error - unable to reach Cost Explorer API"
		ceErr.Retryable = true

	case strings.Contains(errStr, "invalid") || strings.Contains(errStr, "malformed") || strings.Contains(errStr, "bad request"):
		ceErr.Type = ErrorTypeInvalidRequest
		ceErr.Message = "Invalid request parameters"
		ceErr.Retryable = false

	case strings.Contains(errStr, "not found") || strings.Contains(errStr, "no data"):
		ceErr.Type = ErrorTypeDataNotFound
		ceErr.Message = "Cost data not found for deployment"
		ceErr.Retryable = false

	case strings.Contains(errStr, "internal") || strings.Contains(errStr, "service error") || strings.Contains(errStr, "unavailable"):
		ceErr.Type = ErrorTypeServiceError
		ceErr.Message = "AWS Cost Explorer service error"
		ceErr.Retryable = true

	default:
		ceErr.Type = ErrorTypeUnknown
		ceErr.Message = "Unknown error occurred"
		ceErr.Retryable = false
	}

	return ceErr
}

// classifyAPIError classifies AWS SDK API errors
func classifyAPIError(apiErr smithy.APIError) (ErrorType, string, bool) {
	code := apiErr.ErrorCode()
	message := apiErr.ErrorMessage()

	switch code {
	// Rate limiting errors
	case "ThrottlingException", "RequestLimitExceeded", "TooManyRequestsException":
		return ErrorTypeRateLimit, fmt.Sprintf("Rate limit exceeded: %s", message), true

	// Authentication errors
	case "UnrecognizedClientException", "InvalidClientTokenId", "SignatureDoesNotMatch":
		return ErrorTypeAuthentication, fmt.Sprintf("Authentication failed: %s", message), false

	// Authorization errors
	case "AccessDeniedException", "UnauthorizedException", "InsufficientPermissionsException":
		return ErrorTypeAuthorization, fmt.Sprintf("Authorization failed: %s", message), false

	// Invalid request errors
	case "ValidationException", "InvalidParameterException", "InvalidParameterValueException":
		return ErrorTypeInvalidRequest, fmt.Sprintf("Invalid request: %s", message), false

	// Data not found errors
	case "DataUnavailableException", "ResourceNotFoundException":
		return ErrorTypeDataNotFound, fmt.Sprintf("Data not found: %s", message), false

	// Service errors (retryable)
	case "InternalServerError", "ServiceUnavailableException", "InternalFailureException":
		return ErrorTypeServiceError, fmt.Sprintf("Service error: %s", message), true

	// Timeout errors
	case "RequestTimeout", "RequestTimeoutException":
		return ErrorTypeTimeout, fmt.Sprintf("Request timeout: %s", message), true

	default:
		return ErrorTypeUnknown, fmt.Sprintf("Unknown error (%s): %s", code, message), false
	}
}

// IsCostExplorerError checks if an error is a CostExplorerError
func IsCostExplorerError(err error) bool {
	var ceErr *CostExplorerError
	return errors.As(err, &ceErr)
}

// GetErrorType extracts the error type from an error
func GetErrorType(err error) ErrorType {
	var ceErr *CostExplorerError
	if errors.As(err, &ceErr) {
		return ceErr.Type
	}
	return ErrorTypeUnknown
}

// IsRetryableError checks if an error should be retried
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var ceErr *CostExplorerError
	if errors.As(err, &ceErr) {
		return ceErr.Retryable
	}

	// Fallback to string matching for non-classified errors
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "throttl") ||
		strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "network") ||
		strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "temporary") ||
		strings.Contains(errStr, "unavailable")
}

// ErrorStats tracks error statistics for monitoring
type ErrorStats struct {
	TotalErrors        int64                `json:"totalErrors"`
	ErrorsByType       map[ErrorType]int64  `json:"errorsByType"`
	RetryableErrors    int64                `json:"retryableErrors"`
	NonRetryableErrors int64                `json:"nonRetryableErrors"`
	LastError          *CostExplorerError   `json:"lastError,omitempty"`
	LastUpdated        time.Time            `json:"lastUpdated"`
}

// NewErrorStats creates a new ErrorStats instance
func NewErrorStats() *ErrorStats {
	return &ErrorStats{
		ErrorsByType: make(map[ErrorType]int64),
		LastUpdated:  time.Now(),
	}
}

// RecordError records an error in the statistics
func (es *ErrorStats) RecordError(err *CostExplorerError) {
	if err == nil {
		return
	}

	es.TotalErrors++
	es.ErrorsByType[err.Type]++
	
	if err.Retryable {
		es.RetryableErrors++
	} else {
		es.NonRetryableErrors++
	}
	
	es.LastError = err
	es.LastUpdated = time.Now()
}

// GetErrorRate returns the error rate for a specific error type
func (es *ErrorStats) GetErrorRate(errorType ErrorType) float64 {
	if es.TotalErrors == 0 {
		return 0.0
	}
	return float64(es.ErrorsByType[errorType]) / float64(es.TotalErrors)
}

// ShouldTriggerCircuitBreaker determines if circuit breaker should be triggered
func (es *ErrorStats) ShouldTriggerCircuitBreaker(threshold float64, window time.Duration) bool {
	// Check if we have recent errors
	if time.Since(es.LastUpdated) > window {
		return false
	}

	// Check if error rate exceeds threshold
	if es.TotalErrors == 0 {
		return false
	}

	errorRate := float64(es.NonRetryableErrors) / float64(es.TotalErrors)
	return errorRate >= threshold
}

// Global error statistics
var globalErrorStats = NewErrorStats()

// RecordCostExplorerError records a Cost Explorer error globally
func RecordCostExplorerError(err error, deploymentID string) *CostExplorerError {
	ceErr := ClassifyCostExplorerError(err, deploymentID)
	if ceErr != nil {
		globalErrorStats.RecordError(ceErr)
	}
	return ceErr
}

// GetGlobalErrorStats returns global error statistics
func GetGlobalErrorStats() *ErrorStats {
	return globalErrorStats
}

// ResetErrorStats resets global error statistics (useful for testing)
func ResetErrorStats() {
	globalErrorStats = NewErrorStats()
}
