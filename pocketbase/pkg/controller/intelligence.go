package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/models"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/intelligence"
)

// RegisterIntelligenceRoutes registers intelligence and error analysis routes
func RegisterIntelligenceRoutes(app *pocketbase.PocketBase, e *echo.Echo) {
	// Analyze deployment error
	e.POST("/api/intelligence/analyze/:deploymentId", func(c echo.Context) error {
		// Require authentication
		authRecord, _ := c.Get(apis.ContextAuthRecordKey).(*models.Record)
		if authRecord == nil {
			return apis.NewUnauthorizedError("Authentication required", nil)
		}

		deploymentID := c.PathParam("deploymentId")
		if deploymentID == "" {
			return apis.NewBadRequestError("Deployment ID is required", nil)
		}

		// Get deployment
		deployment, err := app.Dao().FindRecordById("awsDeployments", deploymentID)
		if err != nil {
			return apis.NewNotFoundError("Deployment not found", err)
		}

		// Verify ownership
		if deployment.GetString("user") != authRecord.Id {
			return apis.NewForbiddenError("Access denied", nil)
		}

		// Get error logs from request body
		var requestData struct {
			ErrorLogs string `json:"error_logs"`
		}
		if err := c.Bind(&requestData); err != nil {
			return apis.NewBadRequestError("Invalid request data", err)
		}

		if requestData.ErrorLogs == "" {
			return apis.NewBadRequestError("Error logs are required", nil)
		}

		// Analyze the error
		analyzer := intelligence.NewErrorAnalyzer()
		ctx := context.Background()
		analysis := analyzer.AnalyzeTerraformError(ctx, requestData.ErrorLogs)

		// Save analysis to database
		collection, err := app.Dao().FindCollectionByNameOrId("recoveryAttempts")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to find collection",
			})
		}
		recoveryAttempt := models.NewRecord(collection)
		recoveryAttempt.Set("deployment", deploymentID)
		recoveryAttempt.Set("errorType", analysis.ErrorType)
		recoveryAttempt.Set("errorCategory", analysis.Category)
		recoveryAttempt.Set("severity", analysis.Severity)
		recoveryAttempt.Set("description", analysis.Description)
		recoveryAttempt.Set("rootCause", analysis.RootCause)
		recoveryAttempt.Set("suggestedFix", analysis.SuggestedFix)
		recoveryAttempt.Set("autoFixable", analysis.AutoFixable)
		recoveryAttempt.Set("confidence", analysis.Confidence)
		recoveryAttempt.Set("errorLogs", requestData.ErrorLogs)
		recoveryAttempt.Set("status", "pending")
		recoveryAttempt.Set("attemptNumber", 1)

		if err := app.Dao().SaveRecord(recoveryAttempt); err != nil {
			return apis.NewBadRequestError("Failed to save analysis", err)
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"analysis":  analysis,
			"attemptId": recoveryAttempt.Id,
		})
	}, apis.ActivityLogger(app))

	// Get recovery history for a deployment
	e.GET("/api/intelligence/recovery-history/:deploymentId", func(c echo.Context) error {
		// Require authentication
		authRecord, _ := c.Get(apis.ContextAuthRecordKey).(*models.Record)
		if authRecord == nil {
			return apis.NewUnauthorizedError("Authentication required", nil)
		}

		deploymentID := c.PathParam("deploymentId")
		if deploymentID == "" {
			return apis.NewBadRequestError("Deployment ID is required", nil)
		}

		// Get deployment
		deployment, err := app.Dao().FindRecordById("awsDeployments", deploymentID)
		if err != nil {
			return apis.NewNotFoundError("Deployment not found", err)
		}

		// Verify ownership
		if deployment.GetString("user") != authRecord.Id {
			return apis.NewForbiddenError("Access denied", nil)
		}

		// Get recovery attempts
		attempts, err := app.Dao().FindRecordsByFilter(
			"recoveryAttempts",
			"deployment = {:deploymentId}",
			"-created",
			100,
			0,
			map[string]interface{}{
				"deploymentId": deploymentID,
			},
		)
		if err != nil {
			return apis.NewBadRequestError("Failed to fetch recovery history", err)
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"attempts": attempts,
			"total":    len(attempts),
		})
	}, apis.ActivityLogger(app))

	// Get recovery statistics for user
	e.GET("/api/intelligence/stats", func(c echo.Context) error {
		// Require authentication
		authRecord, _ := c.Get(apis.ContextAuthRecordKey).(*models.Record)
		if authRecord == nil {
			return apis.NewUnauthorizedError("Authentication required", nil)
		}

		// Get all deployments for user
		deployments, err := app.Dao().FindRecordsByFilter(
			"awsDeployments",
			"user = {:userId}",
			"",
			1000,
			0,
			map[string]interface{}{
				"userId": authRecord.Id,
			},
		)
		if err != nil {
			return apis.NewBadRequestError("Failed to fetch deployments", err)
		}

		// Get all recovery attempts for user's deployments
		deploymentIDs := make([]string, len(deployments))
		for i, d := range deployments {
			deploymentIDs[i] = d.Id
		}

		if len(deploymentIDs) == 0 {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"totalAttempts":    0,
				"successfulFixes":  0,
				"failedFixes":      0,
				"successRate":      0.0,
				"avgRecoveryTime":  0.0,
				"mostCommonErrors": []interface{}{},
			})
		}

		// Build filter for recovery attempts
		filter := "deployment.user.id = {:userId}"
		attempts, err := app.Dao().FindRecordsByFilter(
			"recoveryAttempts",
			filter,
			"-created",
			1000,
			0,
			map[string]interface{}{
				"userId": authRecord.Id,
			},
		)
		if err != nil {
			return apis.NewBadRequestError("Failed to fetch recovery attempts", err)
		}

		// Calculate statistics
		totalAttempts := len(attempts)
		successfulFixes := 0
		failedFixes := 0
		totalDuration := 0.0
		errorCounts := make(map[string]int)

		for _, attempt := range attempts {
			status := attempt.GetString("status")
			if status == "success" {
				successfulFixes++
			} else if status == "failed" {
				failedFixes++
			}

			// Calculate duration
			if !attempt.GetDateTime("completedAt").IsZero() {
				duration := attempt.GetDateTime("completedAt").Time().Sub(attempt.GetDateTime("created").Time())
				totalDuration += duration.Seconds()
			}

			// Count error types
			errorType := attempt.GetString("errorType")
			errorCounts[errorType]++
		}

		successRate := 0.0
		if totalAttempts > 0 {
			successRate = float64(successfulFixes) / float64(totalAttempts) * 100
		}

		avgRecoveryTime := 0.0
		if successfulFixes > 0 {
			avgRecoveryTime = totalDuration / float64(successfulFixes)
		}

		// Get most common errors
		type errorCount struct {
			ErrorType string `json:"error_type"`
			Count     int    `json:"count"`
		}
		mostCommonErrors := make([]errorCount, 0)
		for errorType, count := range errorCounts {
			mostCommonErrors = append(mostCommonErrors, errorCount{
				ErrorType: errorType,
				Count:     count,
			})
		}

		// Sort by count (simple bubble sort for small datasets)
		for i := 0; i < len(mostCommonErrors); i++ {
			for j := i + 1; j < len(mostCommonErrors); j++ {
				if mostCommonErrors[j].Count > mostCommonErrors[i].Count {
					mostCommonErrors[i], mostCommonErrors[j] = mostCommonErrors[j], mostCommonErrors[i]
				}
			}
		}

		// Limit to top 5
		if len(mostCommonErrors) > 5 {
			mostCommonErrors = mostCommonErrors[:5]
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"totalAttempts":    totalAttempts,
			"successfulFixes":  successfulFixes,
			"failedFixes":      failedFixes,
			"successRate":      successRate,
			"avgRecoveryTime":  avgRecoveryTime,
			"mostCommonErrors": mostCommonErrors,
		})
	}, apis.ActivityLogger(app))

	// Attempt recovery for a failed deployment
	e.POST("/api/intelligence/recover/:deploymentId", func(c echo.Context) error {
		// Require authentication
		authRecord, _ := c.Get(apis.ContextAuthRecordKey).(*models.Record)
		if authRecord == nil {
			return apis.NewUnauthorizedError("Authentication required", nil)
		}

		deploymentID := c.PathParam("deploymentId")
		if deploymentID == "" {
			return apis.NewBadRequestError("Deployment ID is required", nil)
		}

		// Get deployment
		deployment, err := app.Dao().FindRecordById("awsDeployments", deploymentID)
		if err != nil {
			return apis.NewNotFoundError("Deployment not found", err)
		}

		// Verify ownership
		if deployment.GetString("user") != authRecord.Id {
			return apis.NewForbiddenError("Access denied", nil)
		}

		// Get error logs from request body
		var requestData struct {
			ErrorLogs string `json:"error_logs"`
		}
		if err := c.Bind(&requestData); err != nil {
			return apis.NewBadRequestError("Invalid request data", err)
		}

		if requestData.ErrorLogs == "" {
			return apis.NewBadRequestError("Error logs are required", nil)
		}

		// Get attempt number
		existingAttempts, _ := app.Dao().FindRecordsByFilter(
			"recoveryAttempts",
			"deployment = {:deploymentId}",
			"-created",
			1,
			0,
			map[string]interface{}{
				"deploymentId": deploymentID,
			},
		)
		attemptNum := len(existingAttempts) + 1

		// Attempt recovery
		recoveryEngine := intelligence.NewRecoveryEngine()
		ctx := context.Background()
		
		startTime := time.Now()
		recoveryAttempt, err := recoveryEngine.AttemptRecovery(ctx, deployment, requestData.ErrorLogs, attemptNum)
		duration := time.Since(startTime).Seconds()

		// Save recovery attempt to database
		collection, collErr := app.Dao().FindCollectionByNameOrId("recoveryAttempts")
		if collErr != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to find collection",
			})
		}
		attemptRecord := models.NewRecord(collection)
		attemptRecord.Set("deployment", deploymentID)
		attemptRecord.Set("errorType", recoveryAttempt.Analysis.ErrorType)
		attemptRecord.Set("errorCategory", recoveryAttempt.Analysis.Category)
		attemptRecord.Set("severity", recoveryAttempt.Analysis.Severity)
		attemptRecord.Set("description", recoveryAttempt.Analysis.Description)
		attemptRecord.Set("rootCause", recoveryAttempt.Analysis.RootCause)
		attemptRecord.Set("suggestedFix", recoveryAttempt.Analysis.SuggestedFix)
		attemptRecord.Set("autoFixable", recoveryAttempt.Analysis.AutoFixable)
		attemptRecord.Set("confidence", recoveryAttempt.Analysis.Confidence)
		attemptRecord.Set("errorLogs", requestData.ErrorLogs)
		attemptRecord.Set("status", recoveryAttempt.Status)
		attemptRecord.Set("attemptNumber", attemptNum)
		attemptRecord.Set("strategy", recoveryAttempt.Strategy)
		attemptRecord.Set("durationSeconds", duration)
		
		if recoveryAttempt.CompletedAt != nil {
			attemptRecord.Set("completedAt", *recoveryAttempt.CompletedAt)
		}
		
		if recoveryAttempt.Fix != nil {
			attemptRecord.Set("fixApplied", recoveryAttempt.Fix)
		}

		if err := app.Dao().SaveRecord(attemptRecord); err != nil {
			return apis.NewBadRequestError("Failed to save recovery attempt", err)
		}

		if err != nil {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"success":  false,
				"error":    err.Error(),
				"attempt":  recoveryAttempt,
				"attemptId": attemptRecord.Id,
			})
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"success":  true,
			"attempt":  recoveryAttempt,
			"attemptId": attemptRecord.Id,
		})
	}, apis.ActivityLogger(app))
}
