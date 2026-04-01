package aws

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/pricing/types"
)

// RetryConfig defines retry behavior configuration
type RetryConfig struct {
	MaxRetries      int           `json:"maxRetries"`
	BaseDelay       time.Duration `json:"baseDelay"`
	MaxDelay        time.Duration `json:"maxDelay"`
	BackoffFactor   float64       `json:"backoffFactor"`
	JitterEnabled   bool          `json:"jitterEnabled"`
	RetryableErrors []string      `json:"retryableErrors"`
}

// DefaultRetryConfig returns a sensible default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    5,
		BaseDelay:     1 * time.Second,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
		JitterEnabled: true,
		RetryableErrors: []string{
			"ThrottlingException",
			"RequestLimitExceeded",
			"ServiceUnavailable",
			"InternalServerError",
			"RequestTimeout",
		},
	}
}

// RetryableOperation represents an operation that can be retried
type RetryableOperation func() error

// RetryablePricingOperation represents a pricing API operation that can be retried
type RetryablePricingOperation func() (*pricing.GetProductsOutput, error)

// RetryManager handles retry logic with exponential backoff
type RetryManager struct {
	config RetryConfig
}

// NewRetryManager creates a new retry manager with the given configuration
func NewRetryManager(config RetryConfig) *RetryManager {
	return &RetryManager{
		config: config,
	}
}

// ExecuteWithRetry executes an operation with retry logic
func (rm *RetryManager) ExecuteWithRetry(ctx context.Context, operation RetryableOperation, operationName string) error {
	var lastErr error

	for attempt := 0; attempt <= rm.config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := rm.calculateDelay(attempt)
			log.Printf("Retrying %s (attempt %d/%d) after %v delay", operationName, attempt, rm.config.MaxRetries, delay)
			
			select {
			case <-ctx.Done():
				return fmt.Errorf("operation cancelled: %w", ctx.Err())
			case <-time.After(delay):
				// Continue with retry
			}
		}

		err := operation()
		if err == nil {
			if attempt > 0 {
				log.Printf("Operation %s succeeded after %d retries", operationName, attempt)
			}
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !rm.isRetryableError(err) {
			log.Printf("Non-retryable error for %s: %v", operationName, err)
			return err
		}

		if attempt == rm.config.MaxRetries {
			log.Printf("Max retries exceeded for %s: %v", operationName, err)
			break
		}

		log.Printf("Retryable error for %s (attempt %d): %v", operationName, attempt+1, err)
	}

	return fmt.Errorf("operation %s failed after %d retries: %w", operationName, rm.config.MaxRetries, lastErr)
}

// ExecutePricingWithRetry executes a pricing API operation with retry logic
func (rm *RetryManager) ExecutePricingWithRetry(ctx context.Context, operation RetryablePricingOperation, operationName string) (*pricing.GetProductsOutput, error) {
	var lastErr error
	var result *pricing.GetProductsOutput

	for attempt := 0; attempt <= rm.config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := rm.calculateDelay(attempt)
			log.Printf("Retrying %s (attempt %d/%d) after %v delay", operationName, attempt, rm.config.MaxRetries, delay)
			
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("operation cancelled: %w", ctx.Err())
			case <-time.After(delay):
				// Continue with retry
			}
		}

		output, err := operation()
		if err == nil {
			if attempt > 0 {
				log.Printf("Operation %s succeeded after %d retries", operationName, attempt)
			}
			return output, nil
		}

		lastErr = err
		result = output

		// Check if error is retryable
		if !rm.isRetryableError(err) {
			log.Printf("Non-retryable error for %s: %v", operationName, err)
			return result, err
		}

		if attempt == rm.config.MaxRetries {
			log.Printf("Max retries exceeded for %s: %v", operationName, err)
			break
		}

		log.Printf("Retryable error for %s (attempt %d): %v", operationName, attempt+1, err)
	}

	return result, fmt.Errorf("operation %s failed after %d retries: %w", operationName, rm.config.MaxRetries, lastErr)
}

// calculateDelay calculates the delay for the given attempt using exponential backoff
func (rm *RetryManager) calculateDelay(attempt int) time.Duration {
	// Calculate exponential backoff: baseDelay * (backoffFactor ^ (attempt - 1))
	delay := float64(rm.config.BaseDelay) * math.Pow(rm.config.BackoffFactor, float64(attempt-1))
	
	// Apply maximum delay limit
	if delay > float64(rm.config.MaxDelay) {
		delay = float64(rm.config.MaxDelay)
	}

	// Add jitter to prevent thundering herd
	if rm.config.JitterEnabled {
		jitter := rand.Float64() * 0.1 * delay // Up to 10% jitter
		delay += jitter
	}

	return time.Duration(delay)
}

// isRetryableError checks if an error is retryable based on configuration
func (rm *RetryManager) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errorStr := err.Error()

	// Check against configured retryable errors
	for _, retryableError := range rm.config.RetryableErrors {
		if contains(errorStr, retryableError) {
			return true
		}
	}

	// Additional AWS-specific retryable conditions
	if contains(errorStr, "timeout") ||
		contains(errorStr, "connection") ||
		contains(errorStr, "network") ||
		contains(errorStr, "temporary") {
		return true
	}

	return false
}

// contains checks if a string contains a substring (case-insensitive)
func contains(str, substr string) bool {
	return len(str) >= len(substr) && 
		   (str == substr || 
		    len(str) > len(substr) && 
		    (str[:len(substr)] == substr || 
		     str[len(str)-len(substr):] == substr ||
		     containsSubstring(str, substr)))
}

// containsSubstring performs a simple substring search
func containsSubstring(str, substr string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// WithRetry is a convenience method to wrap pricing fetcher operations with retry logic
func (pf *PricingFetcher) WithRetry(ctx context.Context, operation RetryablePricingOperation, operationName string) (*pricing.GetProductsOutput, error) {
	retryManager := NewRetryManager(DefaultRetryConfig())
	return retryManager.ExecutePricingWithRetry(ctx, operation, operationName)
}

// Enhanced pricing fetcher methods with retry logic

// fetchFargateVCPURatesWithRetry fetches Fargate vCPU rates with retry logic
func (pf *PricingFetcher) fetchFargateVCPURatesWithRetry(region string) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	operation := func() (*pricing.GetProductsOutput, error) {
		input := &pricing.GetProductsInput{
			ServiceCode: aws.String("AmazonECS"),
			Filters: []types.Filter{
				{
					Type:  types.FilterTypeTermMatch,
					Field: aws.String("location"),
					Value: aws.String(pf.getLocationName(region)),
				},
				{
					Type:  types.FilterTypeTermMatch,
					Field: aws.String("capacityStatus"),
					Value: aws.String("Used"),
				},
				{
					Type:  types.FilterTypeTermMatch,
					Field: aws.String("launchType"),
					Value: aws.String("Fargate"),
				},
				{
					Type:  types.FilterTypeTermMatch,
					Field: aws.String("usageType"),
					Value: aws.String(fmt.Sprintf("%s-Fargate-vCPU", pf.getUsageTypePrefix(region))),
				},
			},
			MaxResults: aws.Int32(10),
		}

		return pf.client.GetProducts(ctx, input)
	}

	result, err := pf.WithRetry(ctx, operation, fmt.Sprintf("fetchFargateVCPU-%s", region))
	if err != nil {
		return 0, fmt.Errorf("failed to get Fargate vCPU products with retry: %w", err)
	}

	if len(result.PriceList) == 0 {
		return 0, fmt.Errorf("no Fargate vCPU pricing found for region %s", region)
	}

	return pf.parseFargatePricing(result.PriceList[0])
}

// fetchS3StorageRatesWithRetry fetches S3 storage rates with retry logic
func (pf *PricingFetcher) fetchS3StorageRatesWithRetry(region string) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	operation := func() (*pricing.GetProductsOutput, error) {
		input := &pricing.GetProductsInput{
			ServiceCode: aws.String("AmazonS3"),
			Filters: []types.Filter{
				{
					Type:  types.FilterTypeTermMatch,
					Field: aws.String("location"),
					Value: aws.String(pf.getLocationName(region)),
				},
				{
					Type:  types.FilterTypeTermMatch,
					Field: aws.String("storageClass"),
					Value: aws.String("General Purpose"),
				},
				{
					Type:  types.FilterTypeTermMatch,
					Field: aws.String("volumeType"),
					Value: aws.String("Standard"),
				},
			},
			MaxResults: aws.Int32(10),
		}

		return pf.client.GetProducts(ctx, input)
	}

	result, err := pf.WithRetry(ctx, operation, fmt.Sprintf("fetchS3Storage-%s", region))
	if err != nil {
		return 0, fmt.Errorf("failed to get S3 storage products with retry: %w", err)
	}

	if len(result.PriceList) == 0 {
		return 0, fmt.Errorf("no S3 storage pricing found for region %s", region)
	}

	return pf.parseS3Pricing(result.PriceList[0])
}

// RetryStats tracks retry statistics for monitoring
type RetryStats struct {
	TotalOperations    int64         `json:"totalOperations"`
	SuccessfulRetries  int64         `json:"successfulRetries"`
	FailedOperations   int64         `json:"failedOperations"`
	AverageRetries     float64       `json:"averageRetries"`
	TotalRetryTime     time.Duration `json:"totalRetryTime"`
	LastUpdated        time.Time     `json:"lastUpdated"`
}

// Global retry statistics
var globalRetryStats = &RetryStats{
	LastUpdated: time.Now(),
}

// UpdateRetryStats updates global retry statistics
func UpdateRetryStats(successful bool, retryCount int, retryTime time.Duration) {
	globalRetryStats.TotalOperations++
	
	if successful && retryCount > 0 {
		globalRetryStats.SuccessfulRetries++
	}
	
	if !successful {
		globalRetryStats.FailedOperations++
	}
	
	globalRetryStats.TotalRetryTime += retryTime
	globalRetryStats.AverageRetries = float64(globalRetryStats.SuccessfulRetries) / float64(globalRetryStats.TotalOperations)
	globalRetryStats.LastUpdated = time.Now()
}

// GetRetryStats returns current retry statistics
func GetRetryStats() RetryStats {
	return *globalRetryStats
}