package controller

import (
	"net/http"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/azure"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/models"
)

// RegisterAzureCredentialRoutes registers Azure credential management endpoints
func RegisterAzureCredentialRoutes(app *pocketbase.PocketBase, e *echo.Echo, encKey []byte) {
	cm := azure.NewCredentialManager(encKey, app)

	group := e.Group("/api/azure", apis.RequireRecordAuth("users"))

	// Store / update Azure credentials
	group.POST("/credentials", func(c echo.Context) error {
		user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
		if user == nil {
			return apis.NewUnauthorizedError("Authentication required", nil)
		}

		var req struct {
			ClientID            string `json:"clientId"`
			ClientSecret        string `json:"clientSecret"`
			TenantID            string `json:"tenantId"`
			SubscriptionID      string `json:"subscriptionId"`
			Region              string `json:"region"`
			StateContainerName  string `json:"stateContainerName"`
			StateStorageAccount string `json:"stateStorageAccount"`
		}
		if err := c.Bind(&req); err != nil {
			return apis.NewBadRequestError("Invalid request body", err)
		}
		if req.ClientID == "" || req.ClientSecret == "" || req.TenantID == "" || req.SubscriptionID == "" {
			return apis.NewBadRequestError("clientId, clientSecret, tenantId, and subscriptionId are required", nil)
		}

		creds := azure.AzureCredentials{
			ClientID:            req.ClientID,
			ClientSecret:        req.ClientSecret,
			TenantID:            req.TenantID,
			SubscriptionID:      req.SubscriptionID,
			Region:              req.Region,
			StateContainerName:  req.StateContainerName,
			StateStorageAccount: req.StateStorageAccount,
		}

		if err := cm.StoreCredentials(user.Id, creds); err != nil {
			return apis.NewApiError(http.StatusInternalServerError, "Failed to store credentials", err)
		}

		return c.JSON(http.StatusOK, map[string]string{"status": "stored"})
	})

	// Validate Azure credentials
	group.POST("/validate-credentials", func(c echo.Context) error {
		user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
		if user == nil {
			return apis.NewUnauthorizedError("Authentication required", nil)
		}

		creds, err := cm.GetCredentials(user.Id)
		if err != nil {
			return apis.NewBadRequestError("No Azure credentials found", err)
		}

		result, err := cm.ValidateCredentials(*creds)
		if err != nil {
			return apis.NewApiError(http.StatusInternalServerError, "Validation error", err)
		}

		if result.Valid {
			cm.UpdateValidationStatus(user.Id, true)
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"valid":          result.Valid,
			"subscriptionId": result.SubscriptionID,
			"tenantId":       result.TenantID,
			"error":          errorString(result.Error),
		})
	})

	// Get credential status
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
