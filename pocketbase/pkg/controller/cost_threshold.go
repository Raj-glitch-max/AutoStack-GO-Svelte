package controller

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
)

// SetCostAlertThresholdRequest represents the request to set a cost alert threshold
type SetCostAlertThresholdRequest struct {
	Threshold float64 `json:"threshold"`
}

// CostAlertThresholdResponse represents the response for threshold operations
type CostAlertThresholdResponse struct {
	DeploymentID string  `json:"deploymentId"`
	Threshold    float64 `json:"threshold"`
	IsDefault    bool    `json:"isDefault"`
}

// RegisterCostThresholdRoutes registers the cost threshold API routes
// Validates: AC-4.4 (User can set custom alert thresholds per deployment)
func RegisterCostThresholdRoutes(app core.App, e *echo.Echo) {
	e.GET("/api/cost/threshold/:deploymentId", func(c echo.Context) error {
		return GetCostAlertThreshold(c, app)
	}, apis.RequireRecordAuth())

	e.PUT("/api/cost/threshold/:deploymentId", func(c echo.Context) error {
		return SetCostAlertThreshold(c, app)
	}, apis.RequireRecordAuth())
}

// GetCostAlertThreshold retrieves the cost alert threshold for a deployment
// GET /api/cost/threshold/:deploymentId
// Validates: AC-4.4 (User can set custom alert thresholds per deployment)
func GetCostAlertThreshold(c echo.Context, app core.App) error {
	deploymentID := c.PathParam("deploymentId")
	if deploymentID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "deployment ID is required",
		})
	}

	// Get authenticated user
	authRecord, _ := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if authRecord == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "authentication required",
		})
	}

	// Find deployment
	deployment, err := app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "deployment not found",
		})
	}

	// Verify user owns the deployment
	if deployment.GetString("user") != authRecord.Id {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "you don't have permission to access this deployment",
		})
	}

	// Get threshold (returns 0 if not set)
	threshold := deployment.GetFloat("costAlertThreshold")
	isDefault := threshold == 0

	// If no custom threshold, use default 20%
	if isDefault {
		threshold = 20.0
	}

	response := CostAlertThresholdResponse{
		DeploymentID: deploymentID,
		Threshold:    threshold,
		IsDefault:    isDefault,
	}

	return c.JSON(http.StatusOK, response)
}

// SetCostAlertThreshold sets the cost alert threshold for a deployment
// PUT /api/cost/threshold/:deploymentId
// Validates: AC-4.4 (User can set custom alert thresholds per deployment)
func SetCostAlertThreshold(c echo.Context, app core.App) error {
	deploymentID := c.PathParam("deploymentId")
	if deploymentID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "deployment ID is required",
		})
	}

	// Get authenticated user
	authRecord, _ := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if authRecord == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "authentication required",
		})
	}

	// Parse request body
	var req SetCostAlertThresholdRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
	}

	// Validate threshold
	if err := validateThreshold(req.Threshold); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
	}

	// Find deployment
	deployment, err := app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "deployment not found",
		})
	}

	// Verify user owns the deployment
	if deployment.GetString("user") != authRecord.Id {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "you don't have permission to modify this deployment",
		})
	}

	// Update threshold
	deployment.Set("costAlertThreshold", req.Threshold)

	if err := app.Dao().SaveRecord(deployment); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to update threshold",
		})
	}

	response := CostAlertThresholdResponse{
		DeploymentID: deploymentID,
		Threshold:    req.Threshold,
		IsDefault:    false,
	}

	return c.JSON(http.StatusOK, response)
}

// validateThreshold validates that a threshold value is valid
// Validates: Threshold must be a positive number
func validateThreshold(threshold float64) error {
	if threshold < 0 {
		return fmt.Errorf("threshold must be a positive number, got %.2f", threshold)
	}

	if threshold > 1000 {
		return fmt.Errorf("threshold must be less than 1000%%, got %.2f", threshold)
	}

	return nil
}
