package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// EmailService handles sending emails via Resend
type EmailService struct {
	apiKey    string
	fromEmail string
	enabled   bool
	client    *http.Client
	apiURL    string
}

// ResendEmailRequest represents the Resend API request
type ResendEmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

// ResendEmailResponse represents the Resend API response
type ResendEmailResponse struct {
	ID string `json:"id"`
}

// CostAlertData represents a cost alert notification
type CostAlertData struct {
	DeploymentID       string
	DeploymentName     string
	EstimatedCost      float64
	ActualCost         float64
	VariancePercentage float64
	Threshold          float64            // Alert threshold percentage
	ServiceBreakdown   map[string]float64 // Service name -> cost
}

// DeploymentData represents a deployment notification
type DeploymentData struct {
	ID            string
	Name          string
	URL           string
	EstimatedCost float64
	ErrorMessage  string
}

// NewEmailService creates a new email service
func NewEmailService() *EmailService {
	apiKey := os.Getenv("RESEND_API_KEY")
	fromEmail := os.Getenv("RESEND_FROM_EMAIL")
	
	if fromEmail == "" {
		fromEmail = "onboarding@resend.dev"
	}
	
	enabled := apiKey != ""
	if !enabled {
		log.Println("[Email] RESEND_API_KEY not set - email notifications disabled")
	} else {
		log.Printf("[Email] Email service enabled - sending from %s", fromEmail)
	}
	
	return &EmailService{
		apiKey:    apiKey,
		fromEmail: fromEmail,
		enabled:   enabled,
		apiURL:    "https://api.resend.com/emails",
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SendCostAlert sends a cost alert email using the professional template
func (s *EmailService) SendCostAlert(userEmail string, alert *CostAlertData) error {
	if !s.enabled {
		log.Printf("[Email] Skipping cost alert email (service disabled)")
		return nil
	}
	
	// Get base URL from environment or use default
	baseURL := os.Getenv("APP_BASE_URL")
	if baseURL == "" {
		baseURL = "https://autostack.io"
	}
	
	// Format the alert data for the template
	templateData := FormatCostAlertData(alert, baseURL)
	
	// Render the email template
	emailTemplate, err := RenderCostAlertEmail(templateData)
	if err != nil {
		log.Printf("[Email] Failed to render cost alert template: %v", err)
		// Fallback to simple email if template rendering fails
		return s.sendSimpleCostAlert(userEmail, alert)
	}
	
	return s.sendEmail(userEmail, emailTemplate.Subject, emailTemplate.HTMLBody)
}

// sendSimpleCostAlert sends a simple cost alert email as fallback
func (s *EmailService) sendSimpleCostAlert(userEmail string, alert *CostAlertData) error {
	subject := fmt.Sprintf("⚠️ Cost Alert: %s", alert.DeploymentName)
	
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #f59e0b; color: white; padding: 20px; border-radius: 5px 5px 0 0; }
        .content { background: #f9fafb; padding: 20px; border: 1px solid #e5e7eb; }
        .alert-box { background: #fef3c7; border-left: 4px solid #f59e0b; padding: 15px; margin: 15px 0; }
        .stats { background: white; padding: 15px; border-radius: 5px; margin: 15px 0; }
        .stat-row { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid #e5e7eb; }
        .stat-label { font-weight: bold; }
        .stat-value { color: #dc2626; }
        .button { display: inline-block; background: #2563eb; color: white; padding: 12px 24px; text-decoration: none; border-radius: 5px; margin-top: 15px; }
        .footer { text-align: center; color: #6b7280; font-size: 12px; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h2>⚠️ Cost Alert</h2>
        </div>
        <div class="content">
            <div class="alert-box">
                <strong>Your deployment has exceeded the estimated cost!</strong>
            </div>
            
            <p>Deployment: <strong>%s</strong></p>
            
            <div class="stats">
                <div class="stat-row">
                    <span class="stat-label">Estimated Cost:</span>
                    <span>$%.2f/month</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">Actual Cost:</span>
                    <span class="stat-value">$%.2f/month</span>
                </div>
                <div class="stat-row">
                    <span class="stat-label">Variance:</span>
                    <span class="stat-value">+%.1f%%</span>
                </div>
            </div>
            
            <p>We recommend reviewing your deployment configuration to optimize costs.</p>
            
            <a href="https://autostack.io/deployments/%s" class="button">View Deployment</a>
        </div>
        <div class="footer">
            <p>AutoStack - Smart Cloud Deployments</p>
            <p>You're receiving this because you have cost alerts enabled.</p>
        </div>
    </div>
</body>
</html>
`, alert.DeploymentName, alert.EstimatedCost, alert.ActualCost, alert.VariancePercentage, alert.DeploymentID)
	
	return s.sendEmail(userEmail, subject, html)
}

// SendDeploymentFailed sends a deployment failure notification
func (s *EmailService) SendDeploymentFailed(userEmail string, deployment *DeploymentData) error {
	if !s.enabled {
		log.Printf("[Email] Skipping deployment failed email (service disabled)")
		return nil
	}
	
	subject := fmt.Sprintf("❌ Deployment Failed: %s", deployment.Name)
	
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #dc2626; color: white; padding: 20px; border-radius: 5px 5px 0 0; }
        .content { background: #f9fafb; padding: 20px; border: 1px solid #e5e7eb; }
        .error-box { background: #fee2e2; border-left: 4px solid #dc2626; padding: 15px; margin: 15px 0; }
        .button { display: inline-block; background: #2563eb; color: white; padding: 12px 24px; text-decoration: none; border-radius: 5px; margin-top: 15px; }
        .footer { text-align: center; color: #6b7280; font-size: 12px; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h2>❌ Deployment Failed</h2>
        </div>
        <div class="content">
            <p>Your deployment <strong>%s</strong> has failed.</p>
            
            <div class="error-box">
                <strong>Error:</strong><br>
                <code>%s</code>
            </div>
            
            <p>Our AI system may have attempted automatic recovery. Please check the deployment details for more information.</p>
            
            <a href="https://autostack.io/deployments/%s" class="button">View Details</a>
        </div>
        <div class="footer">
            <p>AutoStack - Smart Cloud Deployments</p>
        </div>
    </div>
</body>
</html>
`, deployment.Name, deployment.ErrorMessage, deployment.ID)
	
	return s.sendEmail(userEmail, subject, html)
}

// SendDeploymentSuccess sends a deployment success notification
func (s *EmailService) SendDeploymentSuccess(userEmail string, deployment *DeploymentData) error {
	if !s.enabled {
		log.Printf("[Email] Skipping deployment success email (service disabled)")
		return nil
	}
	
	subject := fmt.Sprintf("✅ Deployment Successful: %s", deployment.Name)
	
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #10b981; color: white; padding: 20px; border-radius: 5px 5px 0 0; }
        .content { background: #f9fafb; padding: 20px; border: 1px solid #e5e7eb; }
        .success-box { background: #d1fae5; border-left: 4px solid #10b981; padding: 15px; margin: 15px 0; }
        .info-box { background: white; padding: 15px; border-radius: 5px; margin: 15px 0; }
        .button { display: inline-block; background: #2563eb; color: white; padding: 12px 24px; text-decoration: none; border-radius: 5px; margin-top: 15px; }
        .footer { text-align: center; color: #6b7280; font-size: 12px; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h2>✅ Deployment Successful</h2>
        </div>
        <div class="content">
            <div class="success-box">
                <strong>Your deployment is now live!</strong>
            </div>
            
            <p>Deployment: <strong>%s</strong></p>
            
            <div class="info-box">
                <p><strong>URL:</strong> <a href="%s">%s</a></p>
                <p><strong>Estimated Cost:</strong> $%.2f/month</p>
            </div>
            
            <p>Your application is now accessible and running. We'll monitor costs and notify you of any significant changes.</p>
            
            <a href="https://autostack.io/deployments/%s" class="button">View Deployment</a>
        </div>
        <div class="footer">
            <p>AutoStack - Smart Cloud Deployments</p>
        </div>
    </div>
</body>
</html>
`, deployment.Name, deployment.URL, deployment.URL, deployment.EstimatedCost, deployment.ID)
	
	return s.sendEmail(userEmail, subject, html)
}

// sendEmail sends an email using Resend API
func (s *EmailService) sendEmail(to, subject, html string) error {
	if !s.enabled {
		log.Printf("[Email] Would send email to %s: %s (service disabled)", to, subject)
		return nil
	}
	
	// Prepare request
	reqBody := ResendEmailRequest{
		From:    s.fromEmail,
		To:      []string{to},
		Subject: subject,
		HTML:    html,
	}
	
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal email request: %w", err)
	}
	
	// Create HTTP request
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	req, err := http.NewRequestWithContext(ctx, "POST", s.apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.apiKey))
	
	// Execute request
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	defer resp.Body.Close()
	
	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	
	// Check status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("resend API error (status %d): %s", resp.StatusCode, string(body))
	}
	
	// Parse response
	var resendResp ResendEmailResponse
	if err := json.Unmarshal(body, &resendResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}
	
	log.Printf("[Email] Successfully sent email to %s (ID: %s)", to, resendResp.ID)
	return nil
}

// IsEnabled returns whether email service is enabled
func (s *EmailService) IsEnabled() bool {
	return s.enabled
}
