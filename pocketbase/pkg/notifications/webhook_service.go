package notifications

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
)

// WebhookService handles webhook delivery with retry logic
type WebhookService struct {
	app        core.App
	httpClient *http.Client
}

// WebhookEvent represents a webhook event payload
type WebhookEvent struct {
	Event     string                 `json:"event"`
	Timestamp time.Time              `json:"timestamp"`
	UserID    string                 `json:"userId"`
	Data      map[string]interface{} `json:"data"`
}

// WebhookEventType defines supported webhook events
type WebhookEventType string

const (
	EventDeployStarted   WebhookEventType = "deploy.started"
	EventDeploySuccess   WebhookEventType = "deploy.success"
	EventDeployFailed    WebhookEventType = "deploy.failed"
	EventCostAlert       WebhookEventType = "cost.alert"
	EventHealthDegraded  WebhookEventType = "health.degraded"
)

// NewWebhookService creates a new webhook service
func NewWebhookService(app core.App) *WebhookService {
	return &WebhookService{
		app: app,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// TriggerEvent sends webhook notifications for a specific event
func (ws *WebhookService) TriggerEvent(eventType WebhookEventType, userID string, data map[string]interface{}) error {
	// Find all enabled webhooks for this user that subscribe to this event
	webhooks, err := ws.app.Dao().FindRecordsByFilter(
		"webhooks",
		"user = {:userId} && enabled = true",
		"",
		0,
		0,
		map[string]any{"userId": userID},
	)
	if err != nil {
		return fmt.Errorf("failed to fetch webhooks: %w", err)
	}

	if len(webhooks) == 0 {
		return nil // No webhooks configured
	}

	// Create event payload
	event := WebhookEvent{
		Event:     string(eventType),
		Timestamp: time.Now(),
		UserID:    userID,
		Data:      data,
	}

	// Send to each webhook
	for _, webhook := range webhooks {
		// Check if webhook subscribes to this event
		if !ws.subscribesToEvent(webhook, eventType) {
			continue
		}

		// Send webhook asynchronously
		go ws.deliverWebhook(webhook, event)
	}

	return nil
}

// subscribesToEvent checks if a webhook subscribes to a specific event type
func (ws *WebhookService) subscribesToEvent(webhook *models.Record, eventType WebhookEventType) bool {
	var events []string
	webhook.UnmarshalJSONField("events", &events)

	for _, e := range events {
		if e == string(eventType) {
			return true
		}
	}
	return false
}

// deliverWebhook delivers a webhook with retry logic
func (ws *WebhookService) deliverWebhook(webhook *models.Record, event WebhookEvent) {
	url := webhook.GetString("url")
	secret := webhook.GetString("secret")

	maxRetries := 3
	backoff := time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
		}

		err := ws.sendWebhook(url, secret, event)
		if err == nil {
			// Success - update webhook record
			webhook.Set("lastTriggered", time.Now())
			webhook.Set("failureCount", 0)
			ws.app.Dao().SaveRecord(webhook)
			log.Printf("Webhook delivered successfully to %s (event: %s)", url, event.Event)
			return
		}

		log.Printf("Webhook delivery attempt %d/%d failed for %s: %v", attempt+1, maxRetries, url, err)
	}

	// All retries failed - update failure count
	failureCount := webhook.GetInt("failureCount") + 1
	webhook.Set("failureCount", failureCount)
	ws.app.Dao().SaveRecord(webhook)

	log.Printf("Webhook delivery failed after %d attempts to %s (event: %s)", maxRetries, url, event.Event)
}

// sendWebhook sends a single webhook HTTP request
func (ws *WebhookService) sendWebhook(url string, secret string, event WebhookEvent) error {
	// Marshal event to JSON
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AutoStack-Webhook/1.0")

	// Add signature if secret is provided
	if secret != "" {
		signature := ws.generateSignature(payload, secret)
		req.Header.Set("X-AutoStack-Signature", signature)
	}

	// Send request
	resp, err := ws.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// generateSignature generates HMAC-SHA256 signature for webhook payload
func (ws *WebhookService) generateSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// TestWebhook sends a test event to a webhook
func (ws *WebhookService) TestWebhook(webhookID string) error {
	webhook, err := ws.app.Dao().FindRecordById("webhooks", webhookID)
	if err != nil {
		return fmt.Errorf("webhook not found: %w", err)
	}

	testEvent := WebhookEvent{
		Event:     "test.ping",
		Timestamp: time.Now(),
		UserID:    webhook.GetString("user"),
		Data: map[string]interface{}{
			"message": "This is a test webhook from AutoStack",
		},
	}

	url := webhook.GetString("url")
	secret := webhook.GetString("secret")

	return ws.sendWebhook(url, secret, testEvent)
}
