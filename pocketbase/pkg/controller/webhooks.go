package controller

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/notifications"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/models"
)

// WebhookController handles webhook management
type WebhookController struct {
	app            *pocketbase.PocketBase
	webhookService *notifications.WebhookService
}

// NewWebhookController creates a new webhook controller
func NewWebhookController(app *pocketbase.PocketBase, webhookService *notifications.WebhookService) *WebhookController {
	return &WebhookController{
		app:            app,
		webhookService: webhookService,
	}
}

// CreateWebhookRequest represents a webhook creation request
type CreateWebhookRequest struct {
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Events []string `json:"events"`
	Secret string   `json:"secret,omitempty"`
}

// UpdateWebhookRequest represents a webhook update request
type UpdateWebhookRequest struct {
	Name    string   `json:"name,omitempty"`
	URL     string   `json:"url,omitempty"`
	Events  []string `json:"events,omitempty"`
	Enabled *bool    `json:"enabled,omitempty"`
}

// ListWebhooks returns all webhooks for the authenticated user
func (wc *WebhookController) ListWebhooks(c echo.Context) error {
	user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	webhooks, err := wc.app.Dao().FindRecordsByFilter(
		"webhooks",
		"user = {:userId}",
		"-created",
		0,
		0,
		map[string]any{"userId": user.Id},
	)
	if err != nil {
		return apis.NewBadRequestError("Failed to fetch webhooks", err)
	}

	return c.JSON(http.StatusOK, webhooks)
}

// GetWebhook returns a specific webhook
func (wc *WebhookController) GetWebhook(c echo.Context) error {
	webhookID := c.PathParam("id")
	user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	webhook, err := wc.app.Dao().FindRecordById("webhooks", webhookID)
	if err != nil {
		return apis.NewNotFoundError("Webhook not found", err)
	}

	// Verify ownership
	if webhook.GetString("user") != user.Id {
		return apis.NewForbiddenError("Access denied", nil)
	}

	return c.JSON(http.StatusOK, webhook)
}

// CreateWebhook creates a new webhook
func (wc *WebhookController) CreateWebhook(c echo.Context) error {
	user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	var req CreateWebhookRequest
	if err := c.Bind(&req); err != nil {
		return apis.NewBadRequestError("Invalid request body", err)
	}

	// Validate request
	if req.Name == "" {
		return apis.NewBadRequestError("Name is required", nil)
	}
	if req.URL == "" {
		return apis.NewBadRequestError("URL is required", nil)
	}
	if len(req.Events) == 0 {
		return apis.NewBadRequestError("At least one event is required", nil)
	}

	// Validate events
	validEvents := map[string]bool{
		"deploy.started":  true,
		"deploy.success":  true,
		"deploy.failed":   true,
		"cost.alert":      true,
		"health.degraded": true,
	}
	for _, event := range req.Events {
		if !validEvents[event] {
			return apis.NewBadRequestError("Invalid event type: "+event, nil)
		}
	}

	// Generate secret if not provided
	secret := req.Secret
	if secret == "" {
		secret = generateSecret()
	}

	// Create webhook record
	collection, err := wc.app.Dao().FindCollectionByNameOrId("webhooks")
	if err != nil {
		return apis.NewBadRequestError("Failed to find webhooks collection", err)
	}

	webhook := models.NewRecord(collection)
	webhook.Set("name", req.Name)
	webhook.Set("url", req.URL)
	webhook.Set("events", req.Events)
	webhook.Set("user", user.Id)
	webhook.Set("enabled", true)
	webhook.Set("secret", secret)
	webhook.Set("failureCount", 0)

	if err := wc.app.Dao().SaveRecord(webhook); err != nil {
		return apis.NewBadRequestError("Failed to create webhook", err)
	}

	return c.JSON(http.StatusCreated, webhook)
}

// UpdateWebhook updates an existing webhook
func (wc *WebhookController) UpdateWebhook(c echo.Context) error {
	webhookID := c.PathParam("id")
	user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	webhook, err := wc.app.Dao().FindRecordById("webhooks", webhookID)
	if err != nil {
		return apis.NewNotFoundError("Webhook not found", err)
	}

	// Verify ownership
	if webhook.GetString("user") != user.Id {
		return apis.NewForbiddenError("Access denied", nil)
	}

	var req UpdateWebhookRequest
	if err := c.Bind(&req); err != nil {
		return apis.NewBadRequestError("Invalid request body", err)
	}

	// Update fields
	if req.Name != "" {
		webhook.Set("name", req.Name)
	}
	if req.URL != "" {
		webhook.Set("url", req.URL)
	}
	if len(req.Events) > 0 {
		webhook.Set("events", req.Events)
	}
	if req.Enabled != nil {
		webhook.Set("enabled", *req.Enabled)
	}

	if err := wc.app.Dao().SaveRecord(webhook); err != nil {
		return apis.NewBadRequestError("Failed to update webhook", err)
	}

	return c.JSON(http.StatusOK, webhook)
}

// DeleteWebhook deletes a webhook
func (wc *WebhookController) DeleteWebhook(c echo.Context) error {
	webhookID := c.PathParam("id")
	user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	webhook, err := wc.app.Dao().FindRecordById("webhooks", webhookID)
	if err != nil {
		return apis.NewNotFoundError("Webhook not found", err)
	}

	// Verify ownership
	if webhook.GetString("user") != user.Id {
		return apis.NewForbiddenError("Access denied", nil)
	}

	if err := wc.app.Dao().DeleteRecord(webhook); err != nil {
		return apis.NewBadRequestError("Failed to delete webhook", err)
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// TestWebhook sends a test event to a webhook
func (wc *WebhookController) TestWebhook(c echo.Context) error {
	webhookID := c.PathParam("id")
	user := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	webhook, err := wc.app.Dao().FindRecordById("webhooks", webhookID)
	if err != nil {
		return apis.NewNotFoundError("Webhook not found", err)
	}

	// Verify ownership
	if webhook.GetString("user") != user.Id {
		return apis.NewForbiddenError("Access denied", nil)
	}

	// Send test webhook
	if err := wc.webhookService.TestWebhook(webhookID); err != nil {
		return apis.NewBadRequestError("Failed to send test webhook: "+err.Error(), err)
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "sent"})
}

// RegisterWebhookRoutes registers webhook management routes
func RegisterWebhookRoutes(app *pocketbase.PocketBase, e *echo.Echo, webhookService *notifications.WebhookService) {
	wc := NewWebhookController(app, webhookService)

	group := e.Group("/api/webhooks", apis.RequireRecordAuth("users"))

	group.GET("", wc.ListWebhooks)
	group.GET("/:id", wc.GetWebhook)
	group.POST("", wc.CreateWebhook)
	group.PUT("/:id", wc.UpdateWebhook)
	group.DELETE("/:id", wc.DeleteWebhook)
	group.POST("/:id/test", wc.TestWebhook)
}

// generateSecret generates a random secret for webhook signing
func generateSecret() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
