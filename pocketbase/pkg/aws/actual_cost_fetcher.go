package aws

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
)

const (
	// AWS Cost Explorer requires 48-hour delay for data availability
	CostExplorerDelay = 48 * time.Hour
	
	// Default granularity for cost data
	DefaultGranularity = types.GranularityDaily
	
	// Deployment tag key used for cost tracking
	DeploymentTagKey = "autostack:deployment"
)

// CostExplorerClientInterface defines the interface for AWS Cost Explorer client
type CostExplorerClientInterface interface {
	GetCostAndUsage(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error)
}

// ActualCostFetcher handles fetching actual AWS costs from Cost Explorer API
type ActualCostFetcher struct {
	app            core.App
	client         CostExplorerClientInterface
	retry          *RetryManager
	circuitBreaker *CircuitBreaker
	errorStats     *ErrorStats
}

// ActualCostData represents actual cost data from AWS Cost Explorer
type ActualCostData struct {
	DeploymentID     string             `json:"deploymentId"`
	CostToDate       float64            `json:"costToDate"`
	ProjectedMonthly float64            `json:"projectedMonthly"`
	Variance         float64            `json:"variance"`
	Breakdown        map[string]float64 `json:"breakdown"`
	Period           CostPeriod         `json:"period"`
	FetchedAt        time.Time          `json:"fetchedAt"`
}

// CostPeriod represents the time period for cost data
type CostPeriod struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// NewActualCostFetcher creates a new actual cost fetcher instance
func NewActualCostFetcher(app core.App) (*ActualCostFetcher, error) {
	// Load AWS config
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := costexplorer.NewFromConfig(cfg)
	retryManager := NewRetryManager(DefaultRetryConfig())
	circuitBreaker := NewCircuitBreaker(DefaultCircuitBreakerConfig())
	errorStats := NewErrorStats()

	return &ActualCostFetcher{
		app:            app,
		client:         client,
		retry:          retryManager,
		circuitBreaker: circuitBreaker,
		errorStats:     errorStats,
	}, nil
}

// FetchActualCosts fetches actual costs for a specific deployment
func (acf *ActualCostFetcher) FetchActualCosts(deploymentID string) (*ActualCostData, error) {
	// Get deployment information
	deployment, err := acf.getDeployment(deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	// Check if deployment is old enough for Cost Explorer data
	if err := acf.validateDeploymentAge(deployment); err != nil {
		return nil, err
	}

	// Calculate time period for cost data (incremental update)
	period, isIncremental := acf.calculateIncrementalCostPeriod(deploymentID, deployment)

	// Fetch cost data from AWS Cost Explorer
	newCostData, err := acf.fetchCostExplorerData(deploymentID, period)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cost explorer data: %w", err)
	}

	// If this is an incremental update, merge with existing data
	var totalCostToDate float64
	var combinedBreakdown map[string]float64
	var fullPeriod CostPeriod

	if isIncremental {
		existingCost, err := acf.GetCachedActualCost(deploymentID)
		if err == nil {
			// Merge costs
			totalCostToDate = existingCost.CostToDate + newCostData.CostToDate
			combinedBreakdown = acf.mergeBreakdowns(existingCost.Breakdown, newCostData.Breakdown)
			fullPeriod = CostPeriod{
				Start: existingCost.Period.Start,
				End:   period.End,
			}
			log.Printf("Incremental update for deployment %s: added %.2f to existing %.2f", 
				deploymentID, newCostData.CostToDate, existingCost.CostToDate)
		} else {
			// Fallback to new data only if we can't get existing data
			log.Printf("Warning: Could not get existing cost data for incremental update: %v", err)
			totalCostToDate = newCostData.CostToDate
			combinedBreakdown = newCostData.Breakdown
			fullPeriod = period
		}
	} else {
		// Full fetch from deployment start
		totalCostToDate = newCostData.CostToDate
		combinedBreakdown = newCostData.Breakdown
		fullPeriod = period
	}

	// Get estimate for variance calculation
	estimate, err := acf.getEstimate(deploymentID)
	if err != nil {
		log.Printf("Warning: Could not get estimate for deployment %s: %v", deploymentID, err)
		estimate = 0
	}

	// Calculate projected monthly cost
	projectedMonthly := acf.calculateProjectedMonthlyCost(totalCostToDate, fullPeriod)

	// Calculate variance
	variance := acf.calculateVariance(projectedMonthly, estimate)

	actualCost := &ActualCostData{
		DeploymentID:     deploymentID,
		CostToDate:       totalCostToDate,
		ProjectedMonthly: projectedMonthly,
		Variance:         variance,
		Breakdown:        combinedBreakdown,
		Period:           fullPeriod,
		FetchedAt:        time.Now(),
	}

	// Save actual cost data to database
	if err := acf.saveActualCost(actualCost); err != nil {
		return nil, fmt.Errorf("failed to save actual cost: %w", err)
	}

	return actualCost, nil
}

// fetchCostExplorerData fetches cost data from AWS Cost Explorer API
func (acf *ActualCostFetcher) fetchCostExplorerData(deploymentID string, period CostPeriod) (*ActualCostData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Format dates for Cost Explorer API (YYYY-MM-DD)
	startDate := period.Start.Format("2006-01-02")
	endDate := period.End.Format("2006-01-02")

	log.Printf("Fetching Cost Explorer data for deployment %s (period: %s to %s)", deploymentID, startDate, endDate)

	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod: &types.DateInterval{
			Start: aws.String(startDate),
			End:   aws.String(endDate),
		},
		Granularity: DefaultGranularity,
		Metrics:     []string{"BlendedCost"},
		GroupBy: []types.GroupDefinition{
			{
				Type: types.GroupDefinitionTypeDimension,
				Key:  aws.String("SERVICE"),
			},
		},
		Filter: &types.Expression{
			Tags: &types.TagValues{
				Key:    aws.String(DeploymentTagKey),
				Values: []string{deploymentID},
			},
		},
	}

	// Execute with circuit breaker and retry logic
	var result *costexplorer.GetCostAndUsageOutput
	var lastErr error

	// Wrap the operation with circuit breaker
	circuitBreakerErr := acf.circuitBreaker.Execute(ctx, func() error {
		// Execute with retry logic
		operation := func() error {
			var err error
			result, err = acf.client.GetCostAndUsage(ctx, input)
			
			// Classify and record the error
			if err != nil {
				ceErr := ClassifyCostExplorerError(err, deploymentID)
				acf.errorStats.RecordError(ceErr)
				
				log.Printf("Cost Explorer API error for deployment %s: [%s] %s", 
					deploymentID, ceErr.Type, ceErr.Message)
				
				// Only retry if error is retryable
				if !ceErr.Retryable {
					log.Printf("Non-retryable error for deployment %s, aborting: %v", deploymentID, err)
					return err
				}
				
				lastErr = err
				return err
			}
			
			return nil
		}

		return acf.retry.ExecuteWithRetry(ctx, operation, fmt.Sprintf("fetchCostExplorer-%s", deploymentID))
	}, fmt.Sprintf("CostExplorer-%s", deploymentID))

	// Handle circuit breaker errors
	if circuitBreakerErr != nil {
		if IsCircuitBreakerOpen(circuitBreakerErr) {
			log.Printf("Circuit breaker is open for deployment %s, skipping Cost Explorer API call", deploymentID)
			return nil, fmt.Errorf("cost explorer service temporarily unavailable (circuit breaker open): %w", circuitBreakerErr)
		}
		
		// Return the last error from retry attempts
		if lastErr != nil {
			ceErr := ClassifyCostExplorerError(lastErr, deploymentID)
			return nil, fmt.Errorf("failed to fetch cost data after retries: [%s] %s: %w", ceErr.Type, ceErr.Message, lastErr)
		}
		
		return nil, fmt.Errorf("failed to get cost and usage: %w", circuitBreakerErr)
	}

	log.Printf("Successfully fetched Cost Explorer data for deployment %s", deploymentID)

	// Process results
	return acf.processCostExplorerResults(result)
}

// processCostExplorerResults processes Cost Explorer API results
func (acf *ActualCostFetcher) processCostExplorerResults(result *costexplorer.GetCostAndUsageOutput) (*ActualCostData, error) {
	if result == nil || len(result.ResultsByTime) == 0 {
		return &ActualCostData{
			CostToDate: 0,
			Breakdown:  make(map[string]float64),
		}, nil
	}

	totalCost := 0.0
	breakdown := make(map[string]float64)

	// Aggregate costs across all time periods
	for _, timeResult := range result.ResultsByTime {
		if timeResult.Groups == nil {
			continue
		}

		for _, group := range timeResult.Groups {
			if len(group.Keys) == 0 || group.Metrics == nil {
				continue
			}

			serviceName := group.Keys[0]
			
			// Extract cost amount
			if blendedCost, ok := group.Metrics["BlendedCost"]; ok && blendedCost.Amount != nil {
				cost := parseFloat(*blendedCost.Amount)
				breakdown[serviceName] += cost
				totalCost += cost
			}
		}
	}

	return &ActualCostData{
		CostToDate: roundToTwoDecimals(totalCost),
		Breakdown:  roundBreakdownToTwoDecimals(breakdown),
	}, nil
}

// calculateProjectedMonthlyCost projects monthly cost from partial data
func (acf *ActualCostFetcher) calculateProjectedMonthlyCost(costToDate float64, period CostPeriod) float64 {
	// Calculate days elapsed
	daysElapsed := period.End.Sub(period.Start).Hours() / 24
	
	if daysElapsed <= 0 {
		return costToDate
	}

	// Project to 30 days
	dailyAverage := costToDate / daysElapsed
	projectedMonthly := dailyAverage * 30

	return roundToTwoDecimals(projectedMonthly)
}

// calculateVariance calculates variance between actual and estimated costs
func (acf *ActualCostFetcher) calculateVariance(actual, estimate float64) float64 {
	if estimate == 0 {
		return 0
	}

	variance := ((actual - estimate) / estimate) * 100
	return roundToTwoDecimals(variance)
}

// validateDeploymentAge checks if deployment is old enough for Cost Explorer data
func (acf *ActualCostFetcher) validateDeploymentAge(deployment *models.Record) error {
	createdAt := deployment.GetDateTime("created")
	age := time.Since(createdAt.Time())

	if age < CostExplorerDelay {
		remaining := CostExplorerDelay - age
		return fmt.Errorf("cost data not yet available (AWS requires 48-hour delay, %v remaining)", remaining.Round(time.Hour))
	}

	return nil
}

// calculateCostPeriod calculates the time period for cost data
func (acf *ActualCostFetcher) calculateCostPeriod(deployment *models.Record) CostPeriod {
	createdAt := deployment.GetDateTime("created").Time()
	
	// Start from deployment creation date
	start := createdAt
	
	// End at 48 hours ago (Cost Explorer delay)
	end := time.Now().Add(-CostExplorerDelay)
	
	// Ensure end is not before start
	if end.Before(start) {
		end = start.Add(24 * time.Hour)
	}

	return CostPeriod{
		Start: start,
		End:   end,
	}
}

// calculateIncrementalCostPeriod calculates the time period for incremental cost updates
// Returns the period and a boolean indicating if this is an incremental update
func (acf *ActualCostFetcher) calculateIncrementalCostPeriod(deploymentID string, deployment *models.Record) (CostPeriod, bool) {
	// Try to get existing cost data
	existingCost, err := acf.GetCachedActualCost(deploymentID)
	if err != nil {
		// No existing data, fetch from deployment start
		log.Printf("No existing cost data for deployment %s, performing full fetch", deploymentID)
		return acf.calculateCostPeriod(deployment), false
	}

	// Calculate the new end time (48 hours ago due to Cost Explorer delay)
	newEnd := time.Now().Add(-CostExplorerDelay)
	
	// Start from where we left off (previous periodEnd)
	newStart := existingCost.Period.End
	
	// Check if there's new data to fetch
	if !newEnd.After(newStart) {
		log.Printf("No new cost data available for deployment %s (last fetch: %v)", 
			deploymentID, existingCost.FetchedAt)
		// Return existing period with no new data
		return CostPeriod{
			Start: newStart,
			End:   newStart,
		}, true
	}

	// Ensure we don't fetch more than necessary
	// AWS Cost Explorer works best with reasonable time ranges
	if newEnd.Sub(newStart) > 90*24*time.Hour {
		log.Printf("Warning: Large time gap detected for deployment %s, limiting fetch range", deploymentID)
		newStart = newEnd.Add(-90 * 24 * time.Hour)
	}

	log.Printf("Incremental fetch for deployment %s: %v to %v", 
		deploymentID, newStart.Format("2006-01-02"), newEnd.Format("2006-01-02"))

	return CostPeriod{
		Start: newStart,
		End:   newEnd,
	}, true
}

// mergeBreakdowns merges two cost breakdowns by service
func (acf *ActualCostFetcher) mergeBreakdowns(existing, new map[string]float64) map[string]float64 {
	merged := make(map[string]float64)
	
	// Copy existing breakdown
	for service, cost := range existing {
		merged[service] = cost
	}
	
	// Add new costs
	for service, cost := range new {
		merged[service] += cost
	}
	
	// Round all values to two decimals
	return roundBreakdownToTwoDecimals(merged)
}

// getDeployment retrieves deployment record from database
func (acf *ActualCostFetcher) getDeployment(deploymentID string) (*models.Record, error) {
	deployment, err := acf.app.Dao().FindRecordById("deployments", deploymentID)
	if err != nil {
		return nil, fmt.Errorf("deployment not found: %w", err)
	}
	return deployment, nil
}

// getEstimate retrieves cost estimate for deployment
func (acf *ActualCostFetcher) getEstimate(deploymentID string) (float64, error) {
	record, err := acf.app.Dao().FindFirstRecordByFilter(
		"costEstimates",
		"deployment = {:deploymentId}",
		map[string]any{
			"deploymentId": deploymentID,
		},
	)
	if err != nil {
		return 0, fmt.Errorf("estimate not found: %w", err)
	}

	return record.GetFloat("totalEstimate"), nil
}

// saveActualCost saves actual cost data to database
func (acf *ActualCostFetcher) saveActualCost(data *ActualCostData) error {
	collection, err := acf.app.Dao().FindCollectionByNameOrId("actualCosts")
	if err != nil {
		return fmt.Errorf("failed to find actualCosts collection: %w", err)
	}

	// Check if record already exists
	existingRecord, err := acf.app.Dao().FindFirstRecordByFilter(
		"actualCosts",
		"deployment = {:deploymentId}",
		map[string]any{
			"deploymentId": data.DeploymentID,
		},
	)

	var record *models.Record
	if err != nil {
		// Create new record
		record = models.NewRecord(collection)
	} else {
		// Update existing record
		record = existingRecord
	}

	// Set record data
	record.Set("deployment", data.DeploymentID)
	record.Set("costToDate", data.CostToDate)
	record.Set("projectedMonthly", data.ProjectedMonthly)
	record.Set("variance", data.Variance)
	record.Set("breakdown", data.Breakdown)
	record.Set("periodStart", data.Period.Start)
	record.Set("periodEnd", data.Period.End)
	record.Set("fetchedAt", data.FetchedAt)

	return acf.app.Dao().SaveRecord(record)
}

// GetCachedActualCost retrieves cached actual cost data from database
func (acf *ActualCostFetcher) GetCachedActualCost(deploymentID string) (*ActualCostData, error) {
	record, err := acf.app.Dao().FindFirstRecordByFilter(
		"actualCosts",
		"deployment = {:deploymentId}",
		map[string]any{
			"deploymentId": deploymentID,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("actual cost not found: %w", err)
	}

	return &ActualCostData{
		DeploymentID:     record.GetString("deployment"),
		CostToDate:       record.GetFloat("costToDate"),
		ProjectedMonthly: record.GetFloat("projectedMonthly"),
		Variance:         record.GetFloat("variance"),
		Breakdown:        record.Get("breakdown").(map[string]float64),
		Period: CostPeriod{
			Start: record.GetDateTime("periodStart").Time(),
			End:   record.GetDateTime("periodEnd").Time(),
		},
		FetchedAt: record.GetDateTime("fetchedAt").Time(),
	}, nil
}

// FetchActualCostsForActiveDeployments fetches costs for all active deployments
func (acf *ActualCostFetcher) FetchActualCostsForActiveDeployments() error {
	// Get all active deployments
	deployments, err := acf.app.Dao().FindRecordsByFilter(
		"deployments",
		"status = 'active'",
		"-created",
		0,
		0,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to get active deployments: %w", err)
	}

	log.Printf("Fetching actual costs for %d active deployments", len(deployments))

	successCount := 0
	errorCount := 0

	for _, deployment := range deployments {
		deploymentID := deployment.Id
		
		// Check if deployment is old enough
		if err := acf.validateDeploymentAge(deployment); err != nil {
			log.Printf("Skipping deployment %s: %v", deploymentID, err)
			continue
		}

		// Fetch actual costs
		_, err := acf.FetchActualCosts(deploymentID)
		if err != nil {
			log.Printf("Failed to fetch costs for deployment %s: %v", deploymentID, err)
			errorCount++
			continue
		}

		successCount++
		log.Printf("Successfully fetched costs for deployment %s", deploymentID)
	}

	log.Printf("Actual cost fetch completed: %d successful, %d errors", successCount, errorCount)
	return nil
}

// Helper functions

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

func roundToTwoDecimals(value float64) float64 {
	return float64(int(value*100+0.5)) / 100
}

func roundBreakdownToTwoDecimals(breakdown map[string]float64) map[string]float64 {
	rounded := make(map[string]float64)
	for key, value := range breakdown {
		rounded[key] = roundToTwoDecimals(value)
	}
	return rounded
}

// GetErrorStats returns error statistics for this fetcher
func (acf *ActualCostFetcher) GetErrorStats() *ErrorStats {
	return acf.errorStats
}

// GetCircuitBreakerStats returns circuit breaker statistics
func (acf *ActualCostFetcher) GetCircuitBreakerStats() CircuitBreakerStats {
	return acf.circuitBreaker.GetStats()
}

// ResetCircuitBreaker manually resets the circuit breaker
func (acf *ActualCostFetcher) ResetCircuitBreaker() {
	log.Printf("Manually resetting Cost Explorer circuit breaker")
	acf.circuitBreaker.Reset()
}

// IsHealthy checks if the fetcher is healthy (circuit breaker closed, low error rate)
func (acf *ActualCostFetcher) IsHealthy() bool {
	cbStats := acf.circuitBreaker.GetStats()
	
	// Circuit breaker must be closed
	if cbStats.State != StateClosed {
		return false
	}
	
	// Error rate must be below threshold (e.g., 20%)
	if cbStats.FailureRate > 0.2 {
		return false
	}
	
	return true
}

// GetHealthStatus returns a detailed health status
func (acf *ActualCostFetcher) GetHealthStatus() map[string]interface{} {
	cbStats := acf.circuitBreaker.GetStats()
	
	return map[string]interface{}{
		"healthy":              acf.IsHealthy(),
		"circuitBreakerState":  string(cbStats.State),
		"failureRate":          cbStats.FailureRate,
		"totalRequests":        cbStats.Requests,
		"failures":             cbStats.Failures,
		"successes":            cbStats.Successes,
		"lastFailTime":         cbStats.LastFailTime,
		"errorStats":           acf.errorStats,
	}
}
