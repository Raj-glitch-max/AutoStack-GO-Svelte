package controller

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase/core"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/cache"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/cost"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/validation"
)

// CostEstimateController handles cost estimation endpoints
type CostEstimateController struct {
	app                core.App
	blueprintMapper    *cost.BlueprintMapper
	staleHandler       *aws.StaleDataHandler
	breakdownGenerator *cost.BreakdownGenerator
	estimateCache      *cache.EstimateCache
}

// NewCostEstimateController creates a new cost estimate controller
func NewCostEstimateController(app core.App) *CostEstimateController {
	estimateCache := cache.NewEstimateCache()
	
	// Start background cleanup routine (runs every 15 minutes)
	estimateCache.StartCleanupRoutine(15 * time.Minute)
	
	return &CostEstimateController{
		app:                app,
		blueprintMapper:    cost.NewBlueprintMapper(app),
		staleHandler:       aws.NewStaleDataHandler(app),
		breakdownGenerator: cost.NewBreakdownGenerator(),
		estimateCache:      estimateCache,
	}
}

// CostEstimateRequest represents the request body for cost estimation
type CostEstimateRequest struct {
	Blueprint string                 `json:"blueprint"` // Blueprint type (required)
	Region    string                 `json:"region"`    // AWS region (required)
	Variables map[string]interface{} `json:"variables"` // Optional configuration variables
}

// CostEstimateResponse represents the cost estimation response
type CostEstimateResponse struct {
	Estimate CostEstimateData `json:"estimate"`
}

// CostEstimateData contains the detailed cost estimate information
type CostEstimateData struct {
	Total            float64                `json:"total"`
	Range            CostRange              `json:"range"`
	Breakdown        map[string]float64     `json:"breakdown"`
	Assumptions      map[string]string      `json:"assumptions"`
	Disclaimer       string                 `json:"disclaimer"`
	PricingFetchedAt time.Time              `json:"pricingFetchedAt"`
	Region           string                 `json:"region"`
	Blueprint        string                 `json:"blueprint"`
	Currency         string                 `json:"currency"`
	ServiceDetails   map[string]interface{} `json:"serviceDetails,omitempty"`
}

// CostRange represents the min/max cost range
type CostRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// RegisterCostEstimateRoutes registers cost estimation routes
func (cec *CostEstimateController) RegisterCostEstimateRoutes(e *echo.Echo) {
	apiGroup := e.Group("/api/cost")
	
	apiGroup.POST("/estimate", cec.EstimateCost)
	apiGroup.GET("/blueprints", cec.GetSupportedBlueprints)
	apiGroup.GET("/blueprints/:type", cec.GetBlueprintDetails)
	
	// Admin routes for cache management
	adminGroup := e.Group("/api/admin/cost")
	adminGroup.POST("/cache/invalidate", cec.InvalidateCache)
	adminGroup.GET("/cache/stats", cec.GetCacheStats)
}

// EstimateCost handles POST /api/cost/estimate
// Validates: AC-1.6 (Estimate loads in <500ms from cache)
func (cec *CostEstimateController) EstimateCost(c echo.Context) error {
	startTime := time.Now()
	
	// Parse request body
	var request CostEstimateRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}
	
	// Validate request using comprehensive validation
	// Validates: AC-1.4 (Estimate is region-specific)
	if err := validation.ValidateCostEstimateRequest(request.Blueprint, request.Region, request.Variables); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
	}
	
	// Initialize variables if nil
	if request.Variables == nil {
		request.Variables = make(map[string]interface{})
	}
	
	// Add region to variables for validation
	request.Variables["region"] = request.Region
	
	// Check cache first for fast response
	// **Validates**: TR-3.1 (Cost estimate endpoint responds in <500ms)
	cachedEstimate := cec.estimateCache.Get(request.Blueprint, request.Region, request.Variables)
	if cachedEstimate != nil {
		// Cache hit - return cached estimate
		elapsed := time.Since(startTime)
		fmt.Printf("Cache HIT: Cost estimate returned in %v\n", elapsed)
		
		// Get assumptions and disclaimer
		assumptions, _ := cec.blueprintMapper.GetAssumptions(request.Blueprint)
		disclaimer, _ := cec.blueprintMapper.GetDisclaimer(request.Blueprint)
		
		// Get pricing data freshness
		pricingFetchedAt, _ := cec.getPricingDataFreshness(request.Region)
		
		// Extract breakdown from cached estimate
		breakdown := cec.extractBreakdown(cachedEstimate)
		
		// Build response from cache
		response := CostEstimateResponse{
			Estimate: CostEstimateData{
				Total: cachedEstimate.Estimate,
				Range: CostRange{
					Min: cachedEstimate.RangeMin,
					Max: cachedEstimate.RangeMax,
				},
				Breakdown:        breakdown,
				Assumptions:      assumptions,
				Disclaimer:       disclaimer,
				PricingFetchedAt: pricingFetchedAt,
				Region:           request.Region,
				Blueprint:        request.Blueprint,
				Currency:         cachedEstimate.Currency,
				ServiceDetails:   cec.extractServiceDetails(cachedEstimate),
			},
		}
		
		return c.JSON(http.StatusOK, response)
	}
	
	// Cache miss - calculate estimate
	fmt.Printf("Cache MISS: Calculating cost estimate for %s in %s\n", request.Blueprint, request.Region)
	
	// Validate configuration
	if err := cec.blueprintMapper.ValidateConfig(request.Blueprint, request.Variables); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("Invalid configuration: %v", err),
		})
	}
	
	// Calculate cost estimate
	estimate, err := cec.blueprintMapper.EstimateCost(request.Blueprint, request.Variables, request.Region)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to calculate cost estimate: %v", err),
		})
	}
	
	// Store in cache for future requests
	// **Validates**: Cache estimates for 1 hour to meet <500ms response time requirement
	if err := cec.estimateCache.Set(request.Blueprint, request.Region, request.Variables, estimate); err != nil {
		// Log error but don't fail the request
		fmt.Printf("WARNING: Failed to cache estimate: %v\n", err)
	}
	
	// Get assumptions and disclaimer
	assumptions, err := cec.blueprintMapper.GetAssumptions(request.Blueprint)
	if err != nil {
		assumptions = make(map[string]string)
	}
	
	disclaimer, err := cec.blueprintMapper.GetDisclaimer(request.Blueprint)
	if err != nil {
		disclaimer = "Cost estimates are approximate and may vary based on actual usage."
	}
	
	// Get pricing data freshness
	pricingFetchedAt, err := cec.getPricingDataFreshness(request.Region)
	if err != nil {
		// Log error but don't fail the request
		pricingFetchedAt = time.Now().Add(-24 * time.Hour) // Default to 24 hours ago
	}
	
	// Extract breakdown from estimate
	breakdown := cec.extractBreakdown(estimate)
	
	// Build response
	response := CostEstimateResponse{
		Estimate: CostEstimateData{
			Total: estimate.Estimate,
			Range: CostRange{
				Min: estimate.RangeMin,
				Max: estimate.RangeMax,
			},
			Breakdown:        breakdown,
			Assumptions:      assumptions,
			Disclaimer:       disclaimer,
			PricingFetchedAt: pricingFetchedAt,
			Region:           request.Region,
			Blueprint:        request.Blueprint,
			Currency:         estimate.Currency,
			ServiceDetails:   cec.extractServiceDetails(estimate),
		},
	}
	
	// Log performance
	elapsed := time.Since(startTime)
	if elapsed > 500*time.Millisecond {
		// Log warning if response time exceeds 500ms
		fmt.Printf("WARNING: Cost estimate took %v (exceeds 500ms target)\n", elapsed)
	} else {
		fmt.Printf("Cost estimate calculated in %v\n", elapsed)
	}
	
	return c.JSON(http.StatusOK, response)
}

// GetSupportedBlueprints returns list of supported blueprint types
func (cec *CostEstimateController) GetSupportedBlueprints(c echo.Context) error {
	blueprints := cec.blueprintMapper.GetSupportedBlueprints()
	
	// Build detailed response with descriptions
	blueprintDetails := make([]map[string]interface{}, 0, len(blueprints))
	for _, bp := range blueprints {
		description, _ := cec.blueprintMapper.GetBlueprintDescription(bp)
		services, _ := cec.blueprintMapper.GetServiceBreakdown(bp)
		assumptions, _ := cec.blueprintMapper.GetAssumptions(bp)
		
		blueprintDetails = append(blueprintDetails, map[string]interface{}{
			"type":        bp,
			"description": description,
			"services":    services,
			"assumptions": assumptions,
		})
	}
	
	return c.JSON(http.StatusOK, map[string]interface{}{
		"blueprints": blueprintDetails,
	})
}

// GetBlueprintDetails returns detailed information about a specific blueprint
func (cec *CostEstimateController) GetBlueprintDetails(c echo.Context) error {
	blueprintType := c.PathParam("type")
	
	if !cec.blueprintMapper.IsBlueprintSupported(blueprintType) {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": fmt.Sprintf("Blueprint type not found: %s", blueprintType),
		})
	}
	
	description, _ := cec.blueprintMapper.GetBlueprintDescription(blueprintType)
	services, _ := cec.blueprintMapper.GetServiceBreakdown(blueprintType)
	assumptions, _ := cec.blueprintMapper.GetAssumptions(blueprintType)
	disclaimer, _ := cec.blueprintMapper.GetDisclaimer(blueprintType)
	
	return c.JSON(http.StatusOK, map[string]interface{}{
		"type":        blueprintType,
		"description": description,
		"services":    services,
		"assumptions": assumptions,
		"disclaimer":  disclaimer,
	})
}

// Helper methods

// getPricingDataFreshness gets the timestamp of when pricing data was last fetched
func (cec *CostEstimateController) getPricingDataFreshness(region string) (time.Time, error) {
	collection, err := cec.app.Dao().FindCollectionByNameOrId("awsPricingCache")
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to find pricing cache collection: %w", err)
	}
	
	// Get most recent pricing record for this region
	records, err := cec.app.Dao().FindRecordsByFilter(
		collection.Id,
		fmt.Sprintf("region = '%s'", region),
		"-fetchedAt",
		1,
		0,
		nil,
	)
	
	if err != nil || len(records) == 0 {
		return time.Time{}, fmt.Errorf("no pricing data found for region %s", region)
	}
	
	return records[0].GetDateTime("fetchedAt").Time(), nil
}

// extractBreakdown extracts cost breakdown by category from estimate
func (cec *CostEstimateController) extractBreakdown(estimate *cost.CostEstimateWithRange) map[string]float64 {
	// Use the breakdown generator to categorize costs
	categoryBreakdown, err := cec.breakdownGenerator.GenerateBreakdown(estimate)
	if err != nil {
		// Log error and return simple breakdown
		fmt.Printf("WARNING: Failed to generate category breakdown: %v\n", err)
		return map[string]float64{
			"total": estimate.Estimate,
		}
	}

	// Return the categorized breakdown
	// **Validates**: AC-1.2 (Estimate includes compute, networking, and storage costs)
	return map[string]float64{
		"compute":    categoryBreakdown.Compute,
		"networking": categoryBreakdown.Networking,
		"storage":    categoryBreakdown.Storage,
		"database":   categoryBreakdown.Database,
		"other":      categoryBreakdown.Other,
	}
}

// extractServiceDetails extracts detailed service information from estimate
func (cec *CostEstimateController) extractServiceDetails(estimate *cost.CostEstimateWithRange) map[string]interface{} {
	if estimate.Breakdown == nil {
		return nil
	}
	
	// Try to extract service details
	switch bd := estimate.Breakdown.(type) {
	case map[string]interface{}:
		if details, ok := bd["serviceDetails"].(map[string]interface{}); ok {
			return details
		}
	}
	
	return nil
}

// InvalidateCacheRequest represents cache invalidation request
type InvalidateCacheRequest struct {
	Region    string `json:"region,omitempty"`    // Optional: invalidate specific region
	Blueprint string `json:"blueprint,omitempty"` // Optional: invalidate specific blueprint
	All       bool   `json:"all,omitempty"`       // Optional: invalidate all cache
}

// InvalidateCache handles POST /api/admin/cost/cache/invalidate
// Invalidates cache when pricing data is refreshed
// **Validates**: Cache invalidation when pricing data refreshes
func (cec *CostEstimateController) InvalidateCache(c echo.Context) error {
	var request InvalidateCacheRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	var count int
	var message string

	if request.All {
		// Invalidate entire cache
		cec.estimateCache.InvalidateAll()
		message = "All cache entries invalidated"
	} else if request.Region != "" && request.Blueprint != "" {
		// Invalidate specific blueprint+region combination
		cec.estimateCache.Invalidate(request.Blueprint, request.Region, nil)
		count = 1
		message = fmt.Sprintf("Invalidated cache for blueprint '%s' in region '%s'", request.Blueprint, request.Region)
	} else if request.Region != "" {
		// Invalidate all entries for a region
		count = cec.estimateCache.InvalidateRegion(request.Region)
		message = fmt.Sprintf("Invalidated %d cache entries for region '%s'", count, request.Region)
	} else if request.Blueprint != "" {
		// Invalidate all entries for a blueprint
		count = cec.estimateCache.InvalidateBlueprint(request.Blueprint)
		message = fmt.Sprintf("Invalidated %d cache entries for blueprint '%s'", count, request.Blueprint)
	} else {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Must specify region, blueprint, or all=true",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":          message,
		"entriesInvalidated": count,
	})
}

// GetCacheStats handles GET /api/admin/cost/cache/stats
// Returns cache statistics for monitoring
func (cec *CostEstimateController) GetCacheStats(c echo.Context) error {
	stats := cec.estimateCache.GetStats()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"stats": stats,
	})
}

// InvalidateCacheForRegion is a helper method to invalidate cache when pricing data is refreshed
// This should be called by the pricing fetcher after successfully updating pricing data
// **Validates**: Cache invalidation when pricing data refreshes
func (cec *CostEstimateController) InvalidateCacheForRegion(region string) int {
	count := cec.estimateCache.InvalidateRegion(region)
	fmt.Printf("Invalidated %d cache entries for region %s after pricing refresh\n", count, region)
	return count
}

// GetEstimateCache returns the estimate cache instance
// This allows other components (like pricing fetcher) to invalidate cache when needed
func (cec *CostEstimateController) GetEstimateCache() *cache.EstimateCache {
	return cec.estimateCache
}
