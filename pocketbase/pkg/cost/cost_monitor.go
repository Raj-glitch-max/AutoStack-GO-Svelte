package cost

import (
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/notifications"
)

// CostMonitor monitors deployments for cost anomalies and triggers alerts
// Validates: AC-4.1 (Alert triggered when actual cost exceeds estimate by 20%)
// Validates: AC-4.2 (Alert sent via email and in-app notification)
// Validates: AC-4.3 (Alert includes breakdown of which services exceeded budget)
type CostMonitor struct {
	app            core.App
	notifier       *notifications.EmailService
	alertThreshold float64 // default 20%
	costFetcher    *aws.ActualCostFetcher
}

// CostAlert represents a cost anomaly alert
type CostAlert struct {
	ID               string             `json:"id"`
	DeploymentID     string             `json:"deploymentId"`
	UserID           string             `json:"userId"`
	Type             string             `json:"type"`
	Threshold        float64            `json:"threshold"`
	Triggered        bool               `json:"triggered"`
	ActualCost       float64            `json:"actualCost"`
	EstimatedCost    float64            `json:"estimatedCost"`
	Variance         float64            `json:"variance"`
	Message          string             `json:"message"`
	ServiceBreakdown map[string]float64 `json:"serviceBreakdown"`
	SentAt           time.Time          `json:"sentAt"`
	Acknowledged     bool               `json:"acknowledged"`
}

// DeploymentInfo contains deployment information for monitoring
type DeploymentInfo struct {
	ID        string
	Name      string
	UserID    string
	Status    string
	CreatedAt time.Time
}

// NewCostMonitor creates a new cost monitor instance
// Validates: Default 20% alert threshold
func NewCostMonitor(app core.App) (*CostMonitor, error) {
	notifier := notifications.NewEmailService()
	
	costFetcher, err := aws.NewActualCostFetcher(app)
	if err != nil {
		return nil, fmt.Errorf("failed to create cost fetcher: %w", err)
	}
	
	return &CostMonitor{
		app:            app,
		notifier:       notifier,
		alertThreshold: 20.0, // Default 20% threshold
		costFetcher:    costFetcher,
	}, nil
}

// CheckCostAnomalies checks all active deployments for cost anomalies
// Validates: AC-4.1 (Alert triggered when actual cost exceeds estimate by 20%)
// Validates: AC-4.2 (Alert sent via email and in-app notification)
func (cm *CostMonitor) CheckCostAnomalies() error {
	log.Println("Starting cost anomaly detection for active deployments...")
	
	// Get all active deployments
	deployments, err := cm.getActiveDeployments()
	if err != nil {
		return fmt.Errorf("failed to get active deployments: %w", err)
	}
	
	log.Printf("Found %d active deployments to monitor", len(deployments))
	
	anomalyCount := 0
	errorCount := 0
	
	for _, deployment := range deployments {
		// Check if deployment is old enough for cost data
		if time.Since(deployment.CreatedAt) < aws.CostExplorerDelay {
			log.Printf("Skipping deployment %s: too new for cost data (age: %v)", 
				deployment.ID, time.Since(deployment.CreatedAt).Round(time.Hour))
			continue
		}
		
		// Get actual cost data
		actualCost, err := cm.getActualCost(deployment.ID)
		if err != nil {
			log.Printf("Warning: Could not get actual cost for deployment %s: %v", deployment.ID, err)
			errorCount++
			continue
		}
		
		// Get estimate for comparison
		estimate, err := cm.getEstimate(deployment.ID)
		if err != nil {
			log.Printf("Warning: Could not get estimate for deployment %s: %v", deployment.ID, err)
			errorCount++
			continue
		}
		
		// Calculate variance
		variance := cm.calculateVariance(actualCost.ProjectedMonthly, estimate.Total)
		
		// Get custom threshold for this deployment (if set)
		threshold := cm.getDeploymentThreshold(deployment.ID)
		
		// Check if variance exceeds threshold
		if variance > threshold {
			log.Printf("Cost anomaly detected for deployment %s: %.1f%% over estimate (threshold: %.1f%%)", 
				deployment.ID, variance, threshold)
			
			// Check if alert already exists and is not acknowledged
			if cm.hasUnacknowledgedAlert(deployment.ID) {
				log.Printf("Skipping alert for deployment %s: unacknowledged alert already exists", deployment.ID)
				continue
			}
			
			// Send cost alert
			err := cm.sendCostAlert(deployment, actualCost, estimate, variance, threshold)
			if err != nil {
				log.Printf("Failed to send cost alert for deployment %s: %v", deployment.ID, err)
				errorCount++
				continue
			}
			
			anomalyCount++
		} else {
			log.Printf("Deployment %s is within budget: %.1f%% variance (threshold: %.1f%%)", 
				deployment.ID, variance, threshold)
		}
	}
	
	log.Printf("Cost anomaly detection completed: %d anomalies detected, %d errors", anomalyCount, errorCount)
	
	if errorCount > 0 {
		return fmt.Errorf("completed with %d errors", errorCount)
	}
	
	return nil
}

// sendCostAlert creates and sends a cost alert
// Validates: AC-4.2 (Alert sent via email and in-app notification)
// Validates: AC-4.3 (Alert includes breakdown of which services exceeded budget)
func (cm *CostMonitor) sendCostAlert(
	deployment DeploymentInfo,
	actualCost *aws.ActualCostData,
	estimate *EstimateData,
	variance float64,
	threshold float64,
) error {
	// Create alert record
	alert := CostAlert{
		DeploymentID:     deployment.ID,
		UserID:           deployment.UserID,
		Type:             "cost_overrun",
		Threshold:        threshold,
		Triggered:        true,
		ActualCost:       actualCost.ProjectedMonthly,
		EstimatedCost:    estimate.Total,
		Variance:         variance,
		Message:          fmt.Sprintf("Deployment costs %.1f%% above estimate", variance),
		ServiceBreakdown: actualCost.Breakdown,
		SentAt:           time.Now(),
		Acknowledged:     false,
	}
	
	// Save alert to database
	alertRecord, err := cm.saveAlert(alert)
	if err != nil {
		return fmt.Errorf("failed to save alert: %w", err)
	}
	
	alert.ID = alertRecord.Id
	
	// Get user email for notification
	userEmail, err := cm.getUserEmail(deployment.UserID)
	if err != nil {
		log.Printf("Warning: Could not get user email for deployment %s: %v", deployment.ID, err)
		// Don't fail if we can't send email - alert is still saved
		return nil
	}
	
	// Send email notification
	// Validates: AC-4.2 (Alert sent via email)
	alertData := &notifications.CostAlertData{
		DeploymentID:       deployment.ID,
		DeploymentName:     deployment.Name,
		EstimatedCost:      estimate.Total,
		ActualCost:         actualCost.ProjectedMonthly,
		VariancePercentage: variance,
	}
	
	err = cm.notifier.SendCostAlert(userEmail, alertData)
	if err != nil {
		log.Printf("Warning: Failed to send email alert for deployment %s: %v", deployment.ID, err)
		// Don't fail if email fails - alert is still saved in database (in-app notification)
	} else {
		log.Printf("Cost alert email sent to %s for deployment %s", userEmail, deployment.ID)
	}
	
	return nil
}

// getActiveDeployments retrieves all active deployments
func (cm *CostMonitor) getActiveDeployments() ([]DeploymentInfo, error) {
	collection, err := cm.app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		return nil, fmt.Errorf("failed to find deployments collection: %w", err)
	}
	
	// Find deployments with status "active" or "running"
	records, err := cm.app.Dao().FindRecordsByFilter(
		collection.Id,
		"status = 'active' || status = 'running'",
		"-created",
		0,
		0,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query active deployments: %w", err)
	}
	
	deployments := make([]DeploymentInfo, 0, len(records))
	for _, record := range records {
		deployments = append(deployments, DeploymentInfo{
			ID:        record.Id,
			Name:      record.GetString("name"),
			UserID:    record.GetString("user"),
			Status:    record.GetString("status"),
			CreatedAt: record.GetDateTime("created").Time(),
		})
	}
	
	return deployments, nil
}

// getActualCost retrieves actual cost data for a deployment
func (cm *CostMonitor) getActualCost(deploymentID string) (*aws.ActualCostData, error) {
	// Try to get cached actual cost first
	actualCost, err := cm.costFetcher.GetCachedActualCost(deploymentID)
	if err != nil {
		// If no cached data, try to fetch fresh data
		actualCost, err = cm.costFetcher.FetchActualCosts(deploymentID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch actual costs: %w", err)
		}
	}
	
	return actualCost, nil
}

// EstimateData contains estimate information
type EstimateData struct {
	Total     float64
	CreatedAt time.Time
}

// getEstimate retrieves cost estimate for a deployment
func (cm *CostMonitor) getEstimate(deploymentID string) (*EstimateData, error) {
	record, err := cm.app.Dao().FindFirstRecordByFilter(
		"costEstimates",
		"deployment = {:deploymentId}",
		map[string]any{
			"deploymentId": deploymentID,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("estimate not found: %w", err)
	}
	
	return &EstimateData{
		Total:     record.GetFloat("totalEstimate"),
		CreatedAt: record.GetDateTime("created").Time(),
	}, nil
}

// calculateVariance calculates the percentage variance between actual and estimated costs
// Validates: AC-3.4 (Variance calculation)
func (cm *CostMonitor) calculateVariance(actual, estimate float64) float64 {
	if estimate == 0 {
		return 0
	}
	return ((actual - estimate) / estimate) * 100
}

// getDeploymentThreshold gets the custom alert threshold for a deployment
// Validates: AC-4.4 (User can set custom alert thresholds per deployment)
func (cm *CostMonitor) getDeploymentThreshold(deploymentID string) float64 {
	// Try to get custom threshold from deployment settings
	record, err := cm.app.Dao().FindFirstRecordByFilter(
		"deployments",
		"id = {:deploymentId}",
		map[string]any{
			"deploymentId": deploymentID,
		},
	)
	if err != nil {
		// Return default threshold if deployment not found
		return cm.alertThreshold
	}
	
	// Check if custom threshold is set
	customThreshold := record.GetFloat("costAlertThreshold")
	if customThreshold > 0 {
		return customThreshold
	}
	
	// Return default threshold
	return cm.alertThreshold
}

// hasUnacknowledgedAlert checks if there's an unacknowledged alert for a deployment
func (cm *CostMonitor) hasUnacknowledgedAlert(deploymentID string) bool {
	record, err := cm.app.Dao().FindFirstRecordByFilter(
		"costAlerts",
		"deployment = {:deploymentId} && acknowledged = false",
		map[string]any{
			"deploymentId": deploymentID,
		},
	)
	
	return err == nil && record != nil
}

// saveAlert saves a cost alert to the database
func (cm *CostMonitor) saveAlert(alert CostAlert) (*models.Record, error) {
	collection, err := cm.app.Dao().FindCollectionByNameOrId("costAlerts")
	if err != nil {
		return nil, fmt.Errorf("failed to find costAlerts collection: %w", err)
	}
	
	record := models.NewRecord(collection)
	record.Set("deployment", alert.DeploymentID)
	record.Set("user", alert.UserID)
	record.Set("type", alert.Type)
	record.Set("threshold", alert.Threshold)
	record.Set("triggered", alert.Triggered)
	record.Set("actualCost", alert.ActualCost)
	record.Set("estimatedCost", alert.EstimatedCost)
	record.Set("variance", alert.Variance)
	record.Set("message", alert.Message)
	record.Set("serviceBreakdown", alert.ServiceBreakdown)
	record.Set("sentAt", alert.SentAt)
	record.Set("acknowledged", alert.Acknowledged)
	
	if err := cm.app.Dao().SaveRecord(record); err != nil {
		return nil, fmt.Errorf("failed to save alert record: %w", err)
	}
	
	return record, nil
}

// getUserEmail retrieves the email address for a user
func (cm *CostMonitor) getUserEmail(userID string) (string, error) {
	user, err := cm.app.Dao().FindRecordById("users", userID)
	if err != nil {
		return "", fmt.Errorf("user not found: %w", err)
	}
	
	email := user.GetString("email")
	if email == "" {
		return "", fmt.Errorf("user has no email address")
	}
	
	return email, nil
}

// SetAlertThreshold sets the default alert threshold
func (cm *CostMonitor) SetAlertThreshold(threshold float64) {
	cm.alertThreshold = threshold
}

// GetAlertThreshold returns the default alert threshold
func (cm *CostMonitor) GetAlertThreshold() float64 {
	return cm.alertThreshold
}

// AcknowledgeAlert marks an alert as acknowledged
func (cm *CostMonitor) AcknowledgeAlert(alertID string) error {
	alert, err := cm.app.Dao().FindRecordById("costAlerts", alertID)
	if err != nil {
		return fmt.Errorf("alert not found: %w", err)
	}
	
	alert.Set("acknowledged", true)
	
	if err := cm.app.Dao().SaveRecord(alert); err != nil {
		return fmt.Errorf("failed to update alert: %w", err)
	}
	
	return nil
}

// GetAlertsForDeployment retrieves all alerts for a deployment
func (cm *CostMonitor) GetAlertsForDeployment(deploymentID string) ([]*CostAlert, error) {
	records, err := cm.app.Dao().FindRecordsByFilter(
		"costAlerts",
		"deployment = {:deploymentId}",
		"-created",
		0,
		0,
		map[string]any{
			"deploymentId": deploymentID,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query alerts: %w", err)
	}
	
	alerts := make([]*CostAlert, 0, len(records))
	for _, record := range records {
		alert := &CostAlert{
			ID:               record.Id,
			DeploymentID:     record.GetString("deployment"),
			UserID:           record.GetString("user"),
			Type:             record.GetString("type"),
			Threshold:        record.GetFloat("threshold"),
			Triggered:        record.GetBool("triggered"),
			ActualCost:       record.GetFloat("actualCost"),
			EstimatedCost:    record.GetFloat("estimatedCost"),
			Variance:         record.GetFloat("variance"),
			Message:          record.GetString("message"),
			ServiceBreakdown: record.Get("serviceBreakdown").(map[string]float64),
			SentAt:           record.GetDateTime("sentAt").Time(),
			Acknowledged:     record.GetBool("acknowledged"),
		}
		alerts = append(alerts, alert)
	}
	
	return alerts, nil
}

// GetAlertsForUser retrieves all alerts for a user
func (cm *CostMonitor) GetAlertsForUser(userID string) ([]*CostAlert, error) {
	records, err := cm.app.Dao().FindRecordsByFilter(
		"costAlerts",
		"user = {:userId}",
		"-created",
		0,
		0,
		map[string]any{
			"userId": userID,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query alerts: %w", err)
	}
	
	alerts := make([]*CostAlert, 0, len(records))
	for _, record := range records {
		alert := &CostAlert{
			ID:               record.Id,
			DeploymentID:     record.GetString("deployment"),
			UserID:           record.GetString("user"),
			Type:             record.GetString("type"),
			Threshold:        record.GetFloat("threshold"),
			Triggered:        record.GetBool("triggered"),
			ActualCost:       record.GetFloat("actualCost"),
			EstimatedCost:    record.GetFloat("estimatedCost"),
			Variance:         record.GetFloat("variance"),
			Message:          record.GetString("message"),
			ServiceBreakdown: record.Get("serviceBreakdown").(map[string]float64),
			SentAt:           record.GetDateTime("sentAt").Time(),
			Acknowledged:     record.GetBool("acknowledged"),
		}
		alerts = append(alerts, alert)
	}
	
	return alerts, nil
}
