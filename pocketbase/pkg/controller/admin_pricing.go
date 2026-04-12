package controller

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase/core"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/jobs"
)

// AdminPricingController handles admin pricing management endpoints
type AdminPricingController struct {
	app        core.App
	jobManager *jobs.PricingJobManager
}

// NewAdminPricingController creates a new admin pricing controller
func NewAdminPricingController(app core.App, jobManager *jobs.PricingJobManager) *AdminPricingController {
	return &AdminPricingController{
		app:        app,
		jobManager: jobManager,
	}
}

// PricingStatusResponse represents the pricing system status
type PricingStatusResponse struct {
	SystemHealth      bool                       `json:"systemHealth"`
	Issues            []string                   `json:"issues"`
	DataFreshness     *aws.DataFreshness         `json:"dataFreshness"`
	JobStatus         []jobs.JobStatus           `json:"jobStatus"`
	RegionDataAge     map[string]time.Duration   `json:"regionDataAge"`
	LastRefresh       time.Time                  `json:"lastRefresh"`
	NextScheduledRun  time.Time                  `json:"nextScheduledRun"`
	TotalRecords      int64                      `json:"totalRecords"`
	CacheHitRate      float64                    `json:"cacheHitRate"`
}

// ManualRefreshRequest represents a manual refresh request
type ManualRefreshRequest struct {
	Services []string `json:"services"` // Optional: specific services to refresh
	Regions  []string `json:"regions"`  // Optional: specific regions to refresh
	Force    bool     `json:"force"`    // Force refresh even if data is fresh
}

// ManualRefreshResponse represents the response from a manual refresh
type ManualRefreshResponse struct {
	Success       bool      `json:"success"`
	Message       string    `json:"message"`
	StartedAt     time.Time `json:"startedAt"`
	JobID         string    `json:"jobId"`
	EstimatedTime string    `json:"estimatedTime"`
}

// RegisterAdminPricingRoutes registers admin pricing routes
func (apc *AdminPricingController) RegisterAdminPricingRoutes(e *echo.Echo) {
	adminGroup := e.Group("/api/admin/pricing")
	
	// Require admin authentication for all routes
	adminGroup.Use(apc.requireAdminAuth)
	
	adminGroup.GET("/status", apc.GetPricingStatus)
	adminGroup.POST("/refresh", apc.TriggerManualRefresh)
	adminGroup.GET("/jobs", apc.GetJobStatus)
	adminGroup.POST("/jobs/:jobName/trigger", apc.TriggerJob)
	adminGroup.DELETE("/cache", apc.ClearCache)
	adminGroup.GET("/regions", apc.GetRegionStatus)
	adminGroup.GET("/metrics", apc.GetPricingMetrics)
}

// requireAdminAuth middleware to ensure only admins can access these endpoints
func (apc *AdminPricingController) requireAdminAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		// TODO: Implement proper admin authentication
		// For now, this is a placeholder
		
		// In a real implementation, you would:
		// 1. Check for valid admin session/token
		// 2. Verify admin role in database
		// 3. Return 401/403 if not authorized
		
		return next(c)
	}
}

// GetPricingStatus returns the current status of the pricing system
func (apc *AdminPricingController) GetPricingStatus(c echo.Context) error {
	// Check system health
	isHealthy, issues := apc.jobManager.IsHealthy()
	
	// Get data freshness
	staleHandler := aws.NewStaleDataHandler(apc.app)
	dataFreshness, err := staleHandler.GetSystemDataFreshness()
	if err != nil {
		issues = append(issues, fmt.Sprintf("Failed to check data freshness: %v", err))
		isHealthy = false
	}
	
	// Get job status
	jobStatus := apc.jobManager.GetJobStatus()
	
	// Get region data age
	regionDataAge, err := staleHandler.GetDataAgeByRegion()
	if err != nil {
		issues = append(issues, fmt.Sprintf("Failed to get region data age: %v", err))
	}
	
	// Get cache statistics
	totalRecords, err := apc.getCacheRecordCount()
	if err != nil {
		issues = append(issues, fmt.Sprintf("Failed to get cache record count: %v", err))
	}
	
	// Get last refresh time
	lastRefresh, err := apc.getLastRefreshTime()
	if err != nil {
		issues = append(issues, fmt.Sprintf("Failed to get last refresh time: %v", err))
	}
	
	// Calculate next scheduled run (daily at 2 AM UTC)
	now := time.Now().UTC()
	nextRun := time.Date(now.Year(), now.Month(), now.Day()+1, 2, 0, 0, 0, time.UTC)
	if now.Hour() < 2 {
		nextRun = time.Date(now.Year(), now.Month(), now.Day(), 2, 0, 0, 0, time.UTC)
	}
	
	response := PricingStatusResponse{
		SystemHealth:     isHealthy,
		Issues:           issues,
		DataFreshness:    dataFreshness,
		JobStatus:        jobStatus,
		RegionDataAge:    regionDataAge,
		LastRefresh:      lastRefresh,
		NextScheduledRun: nextRun,
		TotalRecords:     totalRecords,
		CacheHitRate:     0.95, // TODO: Calculate actual cache hit rate
	}
	
	return c.JSON(http.StatusOK, response)
}

// TriggerManualRefresh triggers a manual pricing data refresh
func (apc *AdminPricingController) TriggerManualRefresh(c echo.Context) error {
	var request ManualRefreshRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}
	
	// Check if a refresh is already in progress
	// TODO: Implement job status checking to prevent concurrent refreshes
	
	// Trigger the pricing fetch job
	err := apc.jobManager.TriggerPricingFetch()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to trigger pricing refresh: %v", err),
		})
	}
	
	response := ManualRefreshResponse{
		Success:       true,
		Message:       "Pricing refresh started successfully",
		StartedAt:     time.Now(),
		JobID:         "manual-refresh-" + fmt.Sprintf("%d", time.Now().Unix()),
		EstimatedTime: "5-10 minutes",
	}
	
	return c.JSON(http.StatusOK, response)
}

// GetJobStatus returns the status of all pricing jobs
func (apc *AdminPricingController) GetJobStatus(c echo.Context) error {
	jobStatus := apc.jobManager.GetJobStatus()
	
	return c.JSON(http.StatusOK, map[string]interface{}{
		"jobs": jobStatus,
	})
}

// TriggerJob triggers a specific job manually
func (apc *AdminPricingController) TriggerJob(c echo.Context) error {
	jobName := c.PathParam("jobName")
	
	if jobName == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Job name is required",
		})
	}
	
	// Validate job name
	validJobs := []string{
		"daily-pricing-fetch",
		"pricing-cache-health-check",
		"pricing-data-cleanup",
	}
	
	isValid := false
	for _, validJob := range validJobs {
		if jobName == validJob {
			isValid = true
			break
		}
	}
	
	if !isValid {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("Invalid job name: %s", jobName),
		})
	}
	
	// Trigger the job
	err := apc.jobManager.TriggerPricingFetch() // TODO: Make this job-specific
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to trigger job %s: %v", jobName, err),
		})
	}
	
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Job %s triggered successfully", jobName),
		"jobName": jobName,
		"triggeredAt": time.Now(),
	})
}

// ClearCache clears the pricing cache
func (apc *AdminPricingController) ClearCache(c echo.Context) error {
	collection, err := apc.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to find pricing cache collection",
		})
	}
	
	// Get all records
	records, err := apc.app.Dao().FindRecordsByFilter(
		collection.Id,
		"",
		"",
		0,
		0,
		nil,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to retrieve cache records",
		})
	}
	
	// Delete all records
	deletedCount := 0
	for _, record := range records {
		if err := apc.app.Dao().DeleteRecord(record); err != nil {
			continue // Skip failed deletions
		}
		deletedCount++
	}
	
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Cleared %d records from pricing cache", deletedCount),
		"deletedRecords": deletedCount,
		"clearedAt": time.Now(),
	})
}

// GetRegionStatus returns pricing data status by region
func (apc *AdminPricingController) GetRegionStatus(c echo.Context) error {
	staleHandler := aws.NewStaleDataHandler(apc.app)
	regionDataAge, err := staleHandler.GetDataAgeByRegion()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to get region status: %v", err),
		})
	}
	
	// Convert to more detailed status
	regionStatus := make(map[string]interface{})
	for region, age := range regionDataAge {
		status := "fresh"
		if age > 24*time.Hour {
			status = "stale"
		}
		if age > 48*time.Hour {
			status = "critically_stale"
		}
		if age == 0 {
			status = "no_data"
		}
		
		regionStatus[region] = map[string]interface{}{
			"status": status,
			"age": age.String(),
			"ageHours": age.Hours(),
			"lastUpdated": time.Now().Add(-age).Format("2006-01-02 15:04:05"),
		}
	}
	
	return c.JSON(http.StatusOK, map[string]interface{}{
		"regions": regionStatus,
		"checkedAt": time.Now(),
	})
}

// GetPricingMetrics returns detailed pricing system metrics
func (apc *AdminPricingController) GetPricingMetrics(c echo.Context) error {
	// Get cache statistics
	totalRecords, err := apc.getCacheRecordCount()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to get cache metrics: %v", err),
		})
	}
	
	// Get records by service
	serviceBreakdown, err := apc.getRecordsByService()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to get service breakdown: %v", err),
		})
	}
	
	// Get records by region
	regionBreakdown, err := apc.getRecordsByRegion()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to get region breakdown: %v", err),
		})
	}
	
	metrics := map[string]interface{}{
		"cache": map[string]interface{}{
			"totalRecords": totalRecords,
			"byService": serviceBreakdown,
			"byRegion": regionBreakdown,
		},
		"system": map[string]interface{}{
			"uptime": time.Since(time.Now().Add(-24 * time.Hour)), // TODO: Track actual uptime
			"version": "1.0.0", // TODO: Get actual version
		},
		"generatedAt": time.Now(),
	}
	
	return c.JSON(http.StatusOK, metrics)
}

// Helper methods

func (apc *AdminPricingController) getCacheRecordCount() (int64, error) {
	collection, err := apc.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return 0, err
	}
	
	records, err := apc.app.Dao().FindRecordsByFilter(
		collection.Id,
		"",
		"",
		0,
		0,
		nil,
	)
	if err != nil {
		return 0, err
	}
	
	return int64(len(records)), nil
}

func (apc *AdminPricingController) getLastRefreshTime() (time.Time, error) {
	collection, err := apc.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return time.Time{}, err
	}
	
	records, err := apc.app.Dao().FindRecordsByFilter(
		collection.Id,
		"",
		"-fetchedAt",
		1,
		0,
		nil,
	)
	if err != nil || len(records) == 0 {
		return time.Time{}, err
	}
	
	return records[0].GetDateTime("fetchedAt").Time(), nil
}

func (apc *AdminPricingController) getRecordsByService() (map[string]int, error) {
	collection, err := apc.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return nil, err
	}
	
	records, err := apc.app.Dao().FindRecordsByFilter(
		collection.Id,
		"",
		"",
		0,
		0,
		nil,
	)
	if err != nil {
		return nil, err
	}
	
	breakdown := make(map[string]int)
	for _, record := range records {
		service := record.GetString("service")
		breakdown[service]++
	}
	
	return breakdown, nil
}

func (apc *AdminPricingController) getRecordsByRegion() (map[string]int, error) {
	collection, err := apc.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return nil, err
	}
	
	records, err := apc.app.Dao().FindRecordsByFilter(
		collection.Id,
		"",
		"",
		0,
		0,
		nil,
	)
	if err != nil {
		return nil, err
	}
	
	breakdown := make(map[string]int)
	for _, record := range records {
		region := record.GetString("region")
		breakdown[region]++
	}
	
	return breakdown, nil
}