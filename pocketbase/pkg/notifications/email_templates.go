package notifications

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	textTemplate "text/template"
)

//go:embed templates/*.html templates/*.txt
var templateFS embed.FS

// CostAlertTemplateData contains all data needed for cost alert email templates
// Validates: AC-4.2 (Alert sent via email)
// Validates: AC-4.3 (Alert includes breakdown of which services exceeded budget)
type CostAlertTemplateData struct {
	DeploymentID       string
	DeploymentName     string
	EstimatedCost      string             // Formatted as "123.45"
	ActualCost         string             // Formatted as "156.78"
	VariancePercentage string             // Formatted as "26.7"
	Threshold          string             // Formatted as "20.0"
	ServiceBreakdown   map[string]string  // Service name -> formatted cost
	DeploymentURL      string
	SettingsURL        string
	SupportURL         string
}

// EmailTemplate represents an email with both HTML and plain text versions
type EmailTemplate struct {
	Subject  string
	HTMLBody string
	TextBody string
}

// RenderCostAlertEmail renders the cost alert email template with the provided data
// Returns both HTML and plain text versions for maximum email client compatibility
// Validates: AC-4.2 (Alert sent via email)
// Validates: AC-4.3 (Alert includes breakdown of which services exceeded budget)
func RenderCostAlertEmail(data *CostAlertTemplateData) (*EmailTemplate, error) {
	// Render HTML version
	htmlTemplate, err := template.ParseFS(templateFS, "templates/cost_alert.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML template: %w", err)
	}
	
	var htmlBuf bytes.Buffer
	if err := htmlTemplate.Execute(&htmlBuf, data); err != nil {
		return nil, fmt.Errorf("failed to execute HTML template: %w", err)
	}
	
	// Render plain text version
	txtTemplate, err := textTemplate.ParseFS(templateFS, "templates/cost_alert.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to parse text template: %w", err)
	}
	
	var txtBuf bytes.Buffer
	if err := txtTemplate.Execute(&txtBuf, data); err != nil {
		return nil, fmt.Errorf("failed to execute text template: %w", err)
	}
	
	// Generate subject line
	subject := fmt.Sprintf("⚠️ Cost Alert: %s (+%s%%)", data.DeploymentName, data.VariancePercentage)
	
	return &EmailTemplate{
		Subject:  subject,
		HTMLBody: htmlBuf.String(),
		TextBody: txtBuf.String(),
	}, nil
}

// FormatCostAlertData converts raw cost alert data into template-ready formatted data
// Validates: TR-2.4 (Round to 2 decimal places for display)
func FormatCostAlertData(alert *CostAlertData, baseURL string) *CostAlertTemplateData {
	// Format service breakdown with proper decimal places
	formattedBreakdown := make(map[string]string)
	if alert.ServiceBreakdown != nil {
		for service, cost := range alert.ServiceBreakdown {
			formattedBreakdown[service] = fmt.Sprintf("%.2f", cost)
		}
	}
	
	return &CostAlertTemplateData{
		DeploymentID:       alert.DeploymentID,
		DeploymentName:     alert.DeploymentName,
		EstimatedCost:      fmt.Sprintf("%.2f", alert.EstimatedCost),
		ActualCost:         fmt.Sprintf("%.2f", alert.ActualCost),
		VariancePercentage: fmt.Sprintf("%.1f", alert.VariancePercentage),
		Threshold:          fmt.Sprintf("%.1f", alert.Threshold),
		ServiceBreakdown:   formattedBreakdown,
		DeploymentURL:      fmt.Sprintf("%s/deployments/%s", baseURL, alert.DeploymentID),
		SettingsURL:        fmt.Sprintf("%s/settings/alerts", baseURL),
		SupportURL:         fmt.Sprintf("%s/support", baseURL),
	}
}
