package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type NotificationType string

const (
	DeploymentSuccess NotificationType = "deployment_success"
	DeploymentFailure NotificationType = "deployment_failure"
	DeploymentStarted NotificationType = "deployment_started"
	HealthAlert       NotificationType = "health_alert"
	CostAlert         NotificationType = "cost_alert"
	SecurityAlert     NotificationType = "security_alert"
)

type Notification struct {
	Type      NotificationType       `json:"type"`
	Title     string                 `json:"title"`
	Message   string                 `json:"message"`
	Severity  string                 `json:"severity"` // info, warning, error, critical
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata"`
}

type NotificationService struct {
	emailEnabled  bool
	slackEnabled  bool
	webhookURL    string
	slackWebhook  string
}

func NewNotificationService(webhookURL, slackWebhook string) *NotificationService {
	return &NotificationService{
		emailEnabled:  false, // Will be enabled when email service is configured
		slackEnabled:  slackWebhook != "",
		webhookURL:    webhookURL,
		slackWebhook:  slackWebhook,
	}
}

// Send sends a notification through all configured channels
func (n *NotificationService) Send(notification Notification) error {
	notification.Timestamp = time.Now()
	
	var errors []error
	
	// Send to webhook if configured
	if n.webhookURL != "" {
		if err := n.sendWebhook(notification); err != nil {
			errors = append(errors, fmt.Errorf("webhook error: %w", err))
		}
	}
	
	// Send to Slack if configured
	if n.slackEnabled {
		if err := n.sendSlack(notification); err != nil {
			errors = append(errors, fmt.Errorf("slack error: %w", err))
		}
	}
	
	// Send email if configured
	if n.emailEnabled {
		if err := n.sendEmail(notification); err != nil {
			errors = append(errors, fmt.Errorf("email error: %w", err))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("notification errors: %v", errors)
	}
	
	return nil
}

func (n *NotificationService) sendWebhook(notification Notification) error {
	payload, err := json.Marshal(notification)
	if err != nil {
		return err
	}
	
	resp, err := http.Post(n.webhookURL, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	
	return nil
}

func (n *NotificationService) sendSlack(notification Notification) error {
	// Format Slack message
	color := n.getSeverityColor(notification.Severity)
	
	slackPayload := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color":     color,
				"title":     notification.Title,
				"text":      notification.Message,
				"footer":    "AutoStack",
				"ts":        notification.Timestamp.Unix(),
				"fields":    n.formatMetadataFields(notification.Metadata),
			},
		},
	}
	
	payload, err := json.Marshal(slackPayload)
	if err != nil {
		return err
	}
	
	resp, err := http.Post(n.slackWebhook, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}
	
	return nil
}

func (n *NotificationService) sendEmail(notification Notification) error {
	// Placeholder for email implementation
	// Will be implemented when email service is configured
	return nil
}

func (n *NotificationService) getSeverityColor(severity string) string {
	switch severity {
	case "info":
		return "#36a64f" // green
	case "warning":
		return "#ff9900" // orange
	case "error":
		return "#ff0000" // red
	case "critical":
		return "#8b0000" // dark red
	default:
		return "#808080" // gray
	}
}

func (n *NotificationService) formatMetadataFields(metadata map[string]interface{}) []map[string]interface{} {
	fields := []map[string]interface{}{}
	
	for key, value := range metadata {
		fields = append(fields, map[string]interface{}{
			"title": key,
			"value": fmt.Sprintf("%v", value),
			"short": true,
		})
	}
	
	return fields
}

// Helper functions for common notifications

func (n *NotificationService) NotifyDeploymentSuccess(deploymentName, projectName, url string) error {
	return n.Send(Notification{
		Type:     DeploymentSuccess,
		Title:    "✅ Deployment Successful",
		Message:  fmt.Sprintf("Deployment '%s' in project '%s' completed successfully", deploymentName, projectName),
		Severity: "info",
		Metadata: map[string]interface{}{
			"deployment": deploymentName,
			"project":    projectName,
			"url":        url,
		},
	})
}

func (n *NotificationService) NotifyDeploymentFailure(deploymentName, projectName, errorMsg string) error {
	return n.Send(Notification{
		Type:     DeploymentFailure,
		Title:    "❌ Deployment Failed",
		Message:  fmt.Sprintf("Deployment '%s' in project '%s' failed: %s", deploymentName, projectName, errorMsg),
		Severity: "error",
		Metadata: map[string]interface{}{
			"deployment": deploymentName,
			"project":    projectName,
			"error":      errorMsg,
		},
	})
}

func (n *NotificationService) NotifyHealthAlert(component, message string) error {
	return n.Send(Notification{
		Type:     HealthAlert,
		Title:    "⚠️ Health Alert",
		Message:  fmt.Sprintf("Component '%s' is unhealthy: %s", component, message),
		Severity: "warning",
		Metadata: map[string]interface{}{
			"component": component,
		},
	})
}

func (n *NotificationService) NotifyCostAlert(projectName string, currentCost, budgetLimit float64) error {
	return n.Send(Notification{
		Type:     CostAlert,
		Title:    "💰 Cost Alert",
		Message:  fmt.Sprintf("Project '%s' has exceeded budget: $%.2f / $%.2f", projectName, currentCost, budgetLimit),
		Severity: "warning",
		Metadata: map[string]interface{}{
			"project":      projectName,
			"current_cost": currentCost,
			"budget_limit": budgetLimit,
		},
	})
}

func (n *NotificationService) NotifySecurityAlert(severity, message string, details map[string]interface{}) error {
	return n.Send(Notification{
		Type:     SecurityAlert,
		Title:    "🔒 Security Alert",
		Message:  message,
		Severity: severity,
		Metadata: details,
	})
}
