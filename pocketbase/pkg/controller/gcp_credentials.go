package controller

import (
	"net/http"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/gcp"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/models"
)

// RegisterGCPCredentialRoutes registers GCP credential management endpoints
func RegisterGCPCredentialRoutes(app *pocketbase.PocketBase, e *echo.Echo, encKey []byte) {
	cm := gcp.NewCredentialManager(encKey, app)

	group := e.Group("/api/gcp", apis.RequireRecordAuth("users"))

	// Store / update GCP credentials
	group.POST("/credentials", func(c echo.Context) error {
		user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
		if user == nil {
			return apis.NewUnauthorizedError("Authentication required", nil)
		}

		var req struct {
			ProjectID         string `json:"projectId"`
			ServiceAccountKey string `json:"serviceAccountKey"`
			Region            string `json:"region"`
			StateBucketName   string `json:"stateBucketName"`
		}
		if err := c.Bind(&req); err != nil {
			return apis.NewBadRequestError("Invalid request body", err)
		}
		if req.ProjectID == "" || req.ServiceAccountKey == "" {
			return apis.NewBadRequestError("projectId and serviceAccountKey are required", nil)
		}

		creds := gcp.GCPCredentials{
			ProjectID:         req.ProjectID,
			ServiceAccountKey: req.ServiceAccountKey,
			Region:            req.Region,
			StateBucketName:   req.StateBucketName,
		}

		if err := cm.StoreCredentials(user.Id, creds); err != nil {
			return apis.NewApiError(http.StatusInternalServerError, "Failed to store credentials", err)
		}

		return c.JSON(http.StatusOK, map[string]string{"status": "stored"})
	})

	// Validate GCP credentials
	group.POST("/validate-credentials", func(c echo.Context) error {
		user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
		if user == nil {
			return apis.NewUnauthorizedError("Authentication required", nil)
		}

		creds, err := cm.GetCredentials(user.Id)
		if err != nil {
			return apis.NewBadRequestError("No GCP credentials found", err)
		}

		result, err := cm.ValidateCredentials(*creds)
		if err != nil {
			return apis.NewApiError(http.StatusInternalServerError, "Validation error", err)
		}

		if result.Valid {
			cm.UpdateValidationStatus(user.Id, true)
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"valid":     result.Valid,
			"projectId": result.ProjectID,
			"email":     result.Email,
			"error":     errorString(result.Error),
		})
	})

	// Get credential status (no secrets returned)
	group.GET("/credentials/status", func(c echo.Context) error {
		user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
		if user == nil {
			return apis.NewUnauthorizedError("Authentication required", nil)
		}

		hasValid := cm.HasValidCredentials(user.Id)
		return c.JSON(http.StatusOK, map[string]bool{"hasValidCredentials": hasValid})
	})

	// Delete credentials
	group.DELETE("/credentials", func(c echo.Context) error {
		user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
		if user == nil {
			return apis.NewUnauthorizedError("Authentication required", nil)
		}

		if err := cm.DeleteCredentials(user.Id); err != nil {
			return apis.NewApiError(http.StatusInternalServerError, "Failed to delete credentials", err)
		}

		return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
	})
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
