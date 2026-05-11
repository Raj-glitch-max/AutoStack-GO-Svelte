package controller

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
)

// AlertController handles cost alert management endpoints
type AlertController struct {
	app core.App
}

// NewAlertController creates a new alert controller
func NewAlertController(app core.App) *AlertController {
	return &AlertController{
		app: app,
	}
}

// ListAlerts returns a list of cost alerts for the authenticated user
// Supports filtering and pagination via query parameters:
// - status: "acknowledged" or "unacknowledged"
// - type: "SPIKE", "DROP", or "budget_exceeded"
// - page: page number (default 1)
// - limit: items per page (default 20, max 100)
// - sort: field to sort by (default "created")
// - order: "asc" or "desc" (default "desc")
func (ac *AlertController) ListAlerts(c echo.Context) error {
	user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	// Parse query parameters
	status := c.QueryParam("status")           // "acknowledged", "unacknowledged", or empty for all
	alertType := c.QueryParam("type")          // "SPIKE", "DROP", "budget_exceeded", or empty for all
	page := c.QueryParamDefault("page", "1")
	limit := c.QueryParamDefault("limit", "20")
	sortField := c.QueryParamDefault("sort", "created")
	sortOrder := c.QueryParamDefault("order", "desc")

	// Parse pagination
	pageNum := 1
	if p, err := parseIntParam(page); err == nil && p > 0 {
		pageNum = p
	}

	limitNum := 20
	if l, err := parseIntParam(limit); err == nil && l > 0 && l <= 100 {
		limitNum = l
	}

	// Build filter
	filter := "user = {:userId}"
	params := map[string]any{"userId": user.Id}

	if status == "acknowledged" {
		filter += " && acknowledged = true"
	} else if status == "unacknowledged" {
		filter += " && acknowledged = false"
	}

	if alertType != "" {
		filter += " && type = {:alertType}"
		params["alertType"] = alertType
	}

	// Build sort string
	sortStr := sortField
	if sortOrder == "desc" {
		sortStr = "-" + sortField
	}

	// Calculate offset
	offset := (pageNum - 1) * limitNum

	// Fetch alerts
	alerts, err := ac.app.Dao().FindRecordsByFilter(
		"costAlerts",
		filter,
		sortStr,
		limitNum,
		offset,
		params,
	)
	if err != nil {
		return apis.NewBadRequestError("Failed to fetch alerts", err)
	}

	// Get total count for pagination metadata
	allAlerts, _ := ac.app.Dao().FindRecordsByFilter(
		"costAlerts",
		filter,
		"",
		0,
		0,
		params,
	)
	totalCount := len(allAlerts)
	totalPages := (totalCount + limitNum - 1) / limitNum

	// Return paginated response
	return c.JSON(http.StatusOK, map[string]any{
		"items":       alerts,
		"page":        pageNum,
		"limit":       limitNum,
		"totalItems":  totalCount,
		"totalPages":  totalPages,
		"hasNext":     pageNum < totalPages,
		"hasPrevious": pageNum > 1,
	})
}

// parseIntParam safely parses an integer parameter
func parseIntParam(s string) (int, error) {
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	return i, err
}

// GetUnreadCount returns the count of unacknowledged alerts for the user
func (ac *AlertController) GetUnreadCount(c echo.Context) error {
	user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	// Using FindRecordsByFilter to count unacknowledged alerts
	alerts, err := ac.app.Dao().FindRecordsByFilter(
		"costAlerts",
		"user = {:userId} && acknowledged = false",
		"",
		0,
		0,
		map[string]any{"userId": user.Id},
	)
	
	count := 0
	if err == nil {
		count = len(alerts)
	}

	return c.JSON(http.StatusOK, map[string]int{"count": count})
}

// AcknowledgeAlert marks a specific alert as read/acknowledged
func (ac *AlertController) AcknowledgeAlert(c echo.Context) error {
	alertID := c.PathParam("alertId")
	user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	
	if user == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	alert, err := ac.app.Dao().FindRecordById("costAlerts", alertID)
	if err != nil {
		return apis.NewNotFoundError("Alert not found", err)
	}

	// Verify ownership
	if alert.GetString("user") != user.Id {
		return apis.NewForbiddenError("You do not have permission to acknowledge this alert", nil)
	}

	alert.Set("acknowledged", true)
	alert.Set("acknowledgedAt", time.Now())

	if err := ac.app.Dao().SaveRecord(alert); err != nil {
		return apis.NewBadRequestError("Failed to acknowledge alert", err)
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "acknowledged"})
}

// AcknowledgeAllAlerts marks all alerts for the user as read
func (ac *AlertController) AcknowledgeAllAlerts(c echo.Context) error {
	user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	alerts, err := ac.app.Dao().FindRecordsByFilter(
		"costAlerts",
		"user = {:userId} && acknowledged = false",
		"",
		0,
		0,
		map[string]any{"userId": user.Id},
	)
	if err != nil {
		return apis.NewBadRequestError("Failed to fetch alerts", err)
	}

	for _, alert := range alerts {
		alert.Set("acknowledged", true)
		alert.Set("acknowledgedAt", time.Now())
		if err := ac.app.Dao().SaveRecord(alert); err != nil {
			return apis.NewBadRequestError("Failed to acknowledge all alerts", err)
		}
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "all_acknowledged"})
}

// STANDALONE FUNCTIONS FOR COMPATIBILITY WITH TESTS

// GetDeploymentAlerts returns a list of cost alerts for a specific deployment
func GetDeploymentAlerts(c echo.Context, app core.App) error {
	deploymentID := c.PathParam("deploymentId")
	if deploymentID == "" {
		return apis.NewBadRequestError("deploymentId is required", nil)
	}

	user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	// Verify ownership
	deploymentRecord, err := app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err == nil && deploymentRecord.GetString("user") != user.Id {
		return apis.NewForbiddenError("You do not have permission to view these alerts", nil)
	}
	// Fallback to check normal deployments collection if awsDeployments not found
	if err != nil {
		deploymentRecord, err = app.Dao().FindRecordById("deployments", deploymentID)
		if err == nil && deploymentRecord.GetString("user") != user.Id {
			return apis.NewForbiddenError("You do not have permission to view these alerts", nil)
		}
	}

	alerts, err := app.Dao().FindRecordsByFilter(
		"costAlerts",
		"deployment = {:deploymentId}",
		"-created",
		0,
		0,
		map[string]any{"deploymentId": deploymentID},
	)
	if err != nil {
		return apis.NewBadRequestError("Failed to fetch alerts", err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"alerts": alerts,
	})
}

// GetUserAlerts returns a list of cost alerts for the authenticated user
func GetUserAlerts(c echo.Context, app core.App) error {
	user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	alerts, err := app.Dao().FindRecordsByFilter(
		"costAlerts",
		"user = {:userId}",
		"-created",
		0,
		0,
		map[string]any{"userId": user.Id},
	)
	if err != nil {
		return apis.NewBadRequestError("Failed to fetch alerts", err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"alerts": alerts,
	})
}

// AcknowledgeAlert marks a specific alert as read
func AcknowledgeAlert(c echo.Context, app core.App) error {
	alertID := c.PathParam("alertId")
	user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	alert, err := app.Dao().FindRecordById("costAlerts", alertID)
	if err != nil {
		return apis.NewNotFoundError("Alert not found", err)
	}

	if alert.GetString("user") != user.Id {
		return apis.NewForbiddenError("Permission denied", nil)
	}

	alert.Set("acknowledged", true)
	alert.Set("acknowledgedAt", time.Now())

	if err := app.Dao().SaveRecord(alert); err != nil {
		return apis.NewBadRequestError("Failed to update alert", err)
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// AcknowledgeAllAlerts marks all alerts as read
func AcknowledgeAllAlerts(c echo.Context, app core.App) error {
	user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	alerts, err := app.Dao().FindRecordsByFilter(
		"costAlerts",
		"user = {:userId} && acknowledged = false",
		"",
		0,
		0,
		map[string]any{"userId": user.Id},
	)
	if err != nil {
		return apis.NewBadRequestError("Failed to fetch alerts", err)
	}

	for _, alert := range alerts {
		alert.Set("acknowledged", true)
		alert.Set("acknowledgedAt", time.Now())
		app.Dao().SaveRecord(alert)
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// GetAlertStatistics returns aggregate statistics about user's cost alerts
func (ac *AlertController) GetAlertStatistics(c echo.Context) error {
	user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	// Fetch all alerts for the user
	alerts, err := ac.app.Dao().FindRecordsByFilter(
		"costAlerts",
		"user = {:userId}",
		"",
		0,
		0,
		map[string]any{"userId": user.Id},
	)
	if err != nil {
		return apis.NewBadRequestError("Failed to fetch alerts", err)
	}

	// Calculate statistics
	totalCount := len(alerts)
	unacknowledgedCount := 0
	spikeCount := 0
	dropCount := 0
	var totalVariance float64
	serviceBreakdown := make(map[string]float64)
	var maxVariance float64
	var maxVarianceDeployment string

	for _, alert := range alerts {
		if !alert.GetBool("acknowledged") {
			unacknowledgedCount++
		}

		alertType := alert.GetString("type")
		if alertType == "SPIKE" {
			spikeCount++
		} else if alertType == "DROP" {
			dropCount++
		}

		variance := alert.GetFloat("variance")
		totalVariance += variance

		if variance > maxVariance {
			maxVariance = variance
			maxVarianceDeployment = alert.GetString("deployment")
		}

		// Aggregate service breakdown
		var breakdown map[string]float64
		alert.UnmarshalJSONField("serviceBreakdown", &breakdown)
		for service, cost := range breakdown {
			serviceBreakdown[service] += cost
		}
	}

	avgVariance := 0.0
	if totalCount > 0 {
		avgVariance = totalVariance / float64(totalCount)
	}

	// Find top 5 most expensive services
	type serviceCost struct {
		Service string  `json:"service"`
		Cost    float64 `json:"cost"`
	}
	topServices := make([]serviceCost, 0)
	for service, cost := range serviceBreakdown {
		topServices = append(topServices, serviceCost{Service: service, Cost: cost})
	}
	// Simple bubble sort for top 5
	for i := 0; i < len(topServices); i++ {
		for j := i + 1; j < len(topServices); j++ {
			if topServices[j].Cost > topServices[i].Cost {
				topServices[i], topServices[j] = topServices[j], topServices[i]
			}
		}
	}
	if len(topServices) > 5 {
		topServices = topServices[:5]
	}

	stats := map[string]any{
		"totalAlerts":            totalCount,
		"unacknowledgedAlerts":   unacknowledgedCount,
		"spikeAlerts":            spikeCount,
		"dropAlerts":             dropCount,
		"averageVariance":        avgVariance,
		"maxVariance":            maxVariance,
		"maxVarianceDeployment":  maxVarianceDeployment,
		"topExpensiveServices":   topServices,
	}

	return c.JSON(http.StatusOK, stats)
}

// RegisterAlertRoutes registers the alert management routes
func RegisterAlertRoutes(e *core.ServeEvent, app core.App) {
	ac := NewAlertController(app)
	
	group := e.Router.Group("/api/alerts", apis.RequireRecordAuth("users"))
	
	group.GET("", ac.ListAlerts)
	group.GET("/unread-count", ac.GetUnreadCount)
	group.GET("/statistics", ac.GetAlertStatistics)
	group.POST("/:alertId/acknowledge", ac.AcknowledgeAlert)
	group.POST("/acknowledge-all", ac.AcknowledgeAllAlerts)
	
	// Deployment-specific route
	e.Router.GET("/api/cost/alerts/deployment/:deploymentId", func(c echo.Context) error {
		return GetDeploymentAlerts(c, app)
	}, apis.RequireRecordAuth("users"))
}
