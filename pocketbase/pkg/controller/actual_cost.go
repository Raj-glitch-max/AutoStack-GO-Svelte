package controller

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/cache"
)

// ActualCostController handles actual cost endpoints
type ActualCostController struct {
	app         core.App
	costFetcher *aws.ActualCostFetcher
	cache       *cache.ActualCostCache
}

// NewActualCostController creates a new actual cost controller
func NewActualCostController(app core.App) (*ActualCostController, error) {
	costFetcher, err := aws.NewActualCostFetcher(app)
	if err != nil {
		return nil, fmt.Errorf("failed to create cost fetcher: %w", err)
	}

	// Create cache with 6-hour TTL
	actualCostCache := cache.NewActualCostCache()
	
	// Start background cleanup routine (runs every hour)
	actualCostCache.StartCleanupRoutine(1 * time.Hour)

	return &ActualCostController{
		app:         app,
		costFetcher: costFetcher,
		cache:       actualCostCache,
	}, nil
}

// ActualCostResponse represents the actual cost API response
type ActualCostResponse struct {
	Actual   ActualCostData   `json:"actual"`
	Estimate EstimateData     `json:"estimate"`
}

// ActualCostData contains actual cost information
// Validates: AC-3.3 (Cost-to-date and projected monthly cost)
// Validates: AC-3.4 (Variance comparison)
// Validates: AC-3.5 (Service-level cost breakdown)
type ActualCostData struct {
	CostToDate       float64            `json:"costToDate"`       // Total cost accumulated to date
	ProjectedMonthly float64            `json:"projectedMonthly"` // Projected monthly cost based on current usage
	Variance         float64            `json:"variance"`         // Percentage variance from estimate
	Breakdown        map[string]float64 `json:"breakdown"`        // Cost breakdown by AWS service (EC2, S3, RDS, CloudFront, etc.)
	Period           PeriodData         `json:"period"`           // Time period for cost data
	FetchedAt        time.Time          `json:"fetchedAt"`        // When the cost data was fetched
}

// PeriodData represents the cost period
type PeriodData struct {
	Start string `json:"start"` // YYYY-MM-DD format
	End   string `json:"end"`   // YYYY-MM-DD format
}

// EstimateData contains estimate information for comparison
type EstimateData struct {
	Total     float64   `json:"total"`
	CreatedAt time.Time `json:"createdAt"`
}

// RegisterActualCostRoutes registers actual cost routes
func (acc *ActualCostController) RegisterActualCostRoutes(e *echo.Echo) {
	apiGroup := e.Group("/api/cost")
	
	// GET /api/cost/actual/{deploymentId}
	// Validates: AC-3.1 (Cost Explorer integration), AC-3.3 (Cost projection), AC-3.4 (Variance comparison)
	apiGroup.GET("/actual/:deploymentId", acc.GetActualCost)
	
	// Admin routes for monitoring
	adminGroup := e.Group("/api/admin/cost")
	adminGroup.GET("/actual/health", acc.GetHealthStatus)
	adminGroup.POST("/actual/refresh/:deploymentId", acc.RefreshActualCost)
}

// GetActualCost handles GET /api/cost/actual/{deploymentId}
// Returns actual AWS costs with service-level breakdown
// Validates: AC-3.1 (Cost Explorer integration)
// Validates: AC-3.3 (Cost projection)
// Validates: AC-3.4 (Variance comparison)
// Validates: AC-3.5 (Service-level cost breakdown by EC2, S3, RDS, CloudFront, etc.)
// Validates: TR-3.1 (Cost estimate endpoint responds in <500ms) via caching
func (acc *ActualCostController) GetActualCost(c echo.Context) error {
	deploymentID := c.PathParam("deploymentId")
	
	if deploymentID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "deploymentId is required",
		})
	}
	
	// Verify deployment exists and user has access
	deployment, err := acc.app.Dao().FindRecordById("deployments", deploymentID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Deployment not found",
		})
	}
	
	// Implement user authorization check
	// Validates: TR-5.4 (Cost data isolated per user)
	authRecord, _ := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if authRecord == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
		})
	}
	
	// Verify deployment ownership
	// Validates: User can only access their own deployment costs
	if deployment.GetString("user") != authRecord.Id {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Access denied: You can only access costs for your own deployments",
		})
	}
	
	// Try to get cached actual cost data first (6-hour TTL)
	// Validates: Return cached response if available and not expired
	actualCost := acc.cache.Get(deploymentID)
	
	if actualCost == nil {
		// Cache miss - try to get from database cache
		actualCost, err = acc.costFetcher.GetCachedActualCost(deploymentID)
		if err != nil {
			// No cached data - check if deployment is old enough
			createdAt := deployment.GetDateTime("created")
			age := time.Since(createdAt.Time())
			
			if age < aws.CostExplorerDelay {
				// Deployment too new for Cost Explorer data
				// Validates: AC-3.2 (48-hour delay handling)
				remaining := aws.CostExplorerDelay - age
				return c.JSON(http.StatusAccepted, map[string]interface{}{
					"message": "Cost data not yet available (AWS requires 48-hour delay)",
					"availableIn": remaining.Round(time.Hour).String(),
					"deploymentAge": age.Round(time.Hour).String(),
				})
			}
			
			// Deployment is old enough but no data - try to fetch
			// Validates: Respect AWS Cost Explorer API rate limits (5 requests/second)
			actualCost, err = acc.costFetcher.FetchActualCosts(deploymentID)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{
					"error": fmt.Sprintf("Failed to fetch actual costs: %v", err),
				})
			}
		}
		
		// Store in cache for future requests (6-hour TTL)
		// Validates: Cache key should be based on deployment ID
		if err := acc.cache.Set(deploymentID, actualCost); err != nil {
			// Log warning but don't fail the request
			fmt.Printf("Warning: Failed to cache actual cost for deployment %s: %v\n", deploymentID, err)
		}
	}
	
	// Get estimate for comparison
	// Validates: AC-3.4 (Variance comparison)
	estimate, err := acc.getEstimate(deploymentID)
	if err != nil {
		// Log warning but don't fail the request
		fmt.Printf("Warning: Could not get estimate for deployment %s: %v\n", deploymentID, err)
		estimate = &EstimateData{
			Total:     0,
			CreatedAt: time.Time{},
		}
	}
	
	// Build response
	// Validates: AC-3.3 (Cost projection), AC-3.5 (Service breakdown)
	response := ActualCostResponse{
		Actual: ActualCostData{
			CostToDate:       actualCost.CostToDate,
			ProjectedMonthly: actualCost.ProjectedMonthly,
			Variance:         actualCost.Variance,
			Breakdown:        actualCost.Breakdown,
			Period: PeriodData{
				Start: actualCost.Period.Start.Format("2006-01-02"),
				End:   actualCost.Period.End.Format("2006-01-02"),
			},
			FetchedAt: actualCost.FetchedAt,
		},
		Estimate: *estimate,
	}
	
	return c.JSON(http.StatusOK, response)
}

// GetHealthStatus handles GET /api/admin/cost/actual/health
// Returns health status of the actual cost fetcher
func (acc *ActualCostController) GetHealthStatus(c echo.Context) error {
	healthStatus := acc.costFetcher.GetHealthStatus()
	
	statusCode := http.StatusOK
	if !healthStatus["healthy"].(bool) {
		statusCode = http.StatusServiceUnavailable
	}
	
	return c.JSON(statusCode, healthStatus)
}

// RefreshActualCost handles POST /api/admin/cost/actual/refresh/{deploymentId}
// Manually triggers a cost data refresh for a deployment
func (acc *ActualCostController) RefreshActualCost(c echo.Context) error {
	deploymentID := c.PathParam("deploymentId")
	
	if deploymentID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "deploymentId is required",
		})
	}
	
	// Verify deployment exists
	deployment, err := acc.app.Dao().FindRecordById("deployments", deploymentID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Deployment not found",
		})
	}
	
	// Implement user authorization check for admin endpoint
	// Validates: TR-5.4 (Cost data isolated per user)
	authRecord, _ := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if authRecord == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
		})
	}
	
	// Verify deployment ownership (even for admin endpoint)
	if deployment.GetString("user") != authRecord.Id {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Access denied: You can only refresh costs for your own deployments",
		})
	}
	
	// Invalidate cache before fetching new data
	// Validates: Invalidate cache when new cost data is fetched
	acc.cache.Invalidate(deploymentID)
	
	// Fetch actual costs
	actualCost, err := acc.costFetcher.FetchActualCosts(deploymentID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to refresh actual costs: %v", err),
		})
	}
	
	// Store in cache for future requests
	if err := acc.cache.Set(deploymentID, actualCost); err != nil {
		// Log warning but don't fail the request
		fmt.Printf("Warning: Failed to cache actual cost for deployment %s: %v\n", deploymentID, err)
	}
	
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":           "Actual costs refreshed successfully",
		"costToDate":        actualCost.CostToDate,
		"projectedMonthly":  actualCost.ProjectedMonthly,
		"variance":          actualCost.Variance,
		"fetchedAt":         actualCost.FetchedAt,
	})
}

// Helper methods

// getEstimate retrieves cost estimate for deployment
func (acc *ActualCostController) getEstimate(deploymentID string) (*EstimateData, error) {
	record, err := acc.app.Dao().FindFirstRecordByFilter(
		"costEstimates",
		"deployment = {:deploymentId}",
		map[string]any{
			"deploymentId": deploymentID,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("estimate not found: %w", err)
	}

	return &EstimateData{
		Total:     record.GetFloat("totalEstimate"),
		CreatedAt: record.GetDateTime("created").Time(),
	}, nil
}
