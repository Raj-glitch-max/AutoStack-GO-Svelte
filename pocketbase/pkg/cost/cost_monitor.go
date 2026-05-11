package cost

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/intelligence"
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
	
	costFetcher, err := aws.NewActualCostFetcher(app, notifier)
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

// GetAlertThreshold returns the current global alert threshold
func (cm *CostMonitor) GetAlertThreshold() float64 {
	return cm.alertThreshold
}

// SetAlertThreshold sets the global alert threshold
func (cm *CostMonitor) SetAlertThreshold(threshold float64) {
	cm.alertThreshold = threshold
}

// getUserAlertThreshold gets the alert threshold for a specific user
// Falls back to global threshold if user has no custom preference
func (cm *CostMonitor) getUserAlertThreshold(userID string) float64 {
	user, err := cm.app.Dao().FindRecordById("users", userID)
	if err != nil {
		return cm.alertThreshold // Fallback to global
	}

	prefs := user.Get("alertPreferences")
	if prefs == nil {
		return cm.alertThreshold // Fallback to global
	}

	// Try to extract threshold from preferences
	if prefsMap, ok := prefs.(map[string]interface{}); ok {
		if threshold, ok := prefsMap["threshold"].(float64); ok {
			return threshold
		}
	}

	return cm.alertThreshold // Fallback to global
}

// shouldSendEmailAlert checks if email alerts are enabled for a user
func (cm *CostMonitor) shouldSendEmailAlert(userID string) bool {
	user, err := cm.app.Dao().FindRecordById("users", userID)
	if err != nil {
		return true // Default to enabled
	}

	prefs := user.Get("alertPreferences")
	if prefs == nil {
		return true // Default to enabled
	}

	// Try to extract emailEnabled from preferences
	if prefsMap, ok := prefs.(map[string]interface{}); ok {
		if emailEnabled, ok := prefsMap["emailEnabled"].(bool); ok {
			return emailEnabled
		}
	}

	return true // Default to enabled
}

// GetAlertsForDeployment retrieves all cost alerts for a specific deployment
func (cm *CostMonitor) GetAlertsForDeployment(deploymentID string) ([]*CostAlert, error) {
	records, err := cm.app.Dao().FindRecordsByFilter(
		"costAlerts",
		"deployment = {:deploymentId}",
		"-created",
		0,
		0,
		map[string]any{"deploymentId": deploymentID},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch alerts for deployment %s: %w", deploymentID, err)
	}

	alerts := make([]*CostAlert, len(records))
	for i, record := range records {
		alerts[i] = &CostAlert{
			ID:           record.Id,
			DeploymentID: record.GetString("deployment"),
			UserID:       record.GetString("user"),
			Type:         record.GetString("type"),
			Threshold:    record.GetFloat("threshold"),
			Triggered:    record.GetBool("triggered"),
			ActualCost:   record.GetFloat("actualCost"),
			EstimatedCost: record.GetFloat("estimatedCost"),
			Variance:     record.GetFloat("variance"),
			Message:      record.GetString("message"),
			SentAt:       record.GetDateTime("sentAt").Time(),
			Acknowledged: record.GetBool("acknowledged"),
		}
		record.UnmarshalJSONField("serviceBreakdown", &alerts[i].ServiceBreakdown)
	}

	return alerts, nil
}

// CheckCostAnomalies checks all active deployments for cost anomalies
func (cm *CostMonitor) CheckCostAnomalies() error {
	log.Println("Starting cost anomaly detection for active deployments...")
	
	deployments, err := cm.getActiveDeployments()
	if err != nil {
		return fmt.Errorf("failed to get active deployments: %w", err)
	}
	
	log.Printf("Found %d active deployments to monitor", len(deployments))
	
	anomalyCount := 0
	errorCount := 0
	
	for _, deployment := range deployments {
		if time.Since(deployment.CreatedAt) < aws.CostExplorerDelay {
			continue
		}
		
		actualCost, err := cm.getActualCost(deployment.ID)
		if err != nil {
			errorCount++
			continue
		}
		
		estimate, err := cm.getEstimate(deployment.ID)
		if err != nil {
			errorCount++
			continue
		}
		
		variance := cm.calculateVariance(actualCost.ProjectedMonthly, estimate.Total)
		threshold := cm.getDeploymentThreshold(deployment.ID)
		
		var alertType string
		if variance > threshold {
			alertType = "SPIKE"
		} else if variance < -threshold {
			alertType = "DROP"
		}
		
		if alertType != "" {
			if cm.hasUnacknowledgedAlert(deployment.ID) {
				continue
			}
			
			err := cm.sendCostAlert(deployment, actualCost, estimate, variance, threshold, alertType)
			if err != nil {
				errorCount++
				continue
			}
			anomalyCount++
		}
	}
	
	if errorCount > 0 {
		return fmt.Errorf("completed with %d errors", errorCount)
	}
	
	return nil
}

// sendCostAlert creates and sends a cost alert, enriched with an AI explanation
func (cm *CostMonitor) sendCostAlert(
	deployment DeploymentInfo,
	actualCost *aws.ActualCostData,
	estimate *EstimateData,
	variance float64,
	threshold float64,
	alertType string,
) error {
	message := fmt.Sprintf("Deployment costs %.1f%% above estimate", variance)
	if alertType == "DROP" {
		message = fmt.Sprintf("Deployment costs dropped %.1f%% below estimate", -variance)
	}

	// Enrich alert with AI explanation (non-blocking — failure is logged, not fatal)
	aiExplanation := ""
	explainer := intelligence.NewAnomalyExplainer()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if explanation, err := explainer.ExplainAnomaly(ctx, deployment.Name, actualCost.Breakdown, variance); err != nil {
		log.Printf("[CostMonitor] AI explanation failed for %s: %v", deployment.ID, err)
	} else {
		aiExplanation = explanation.Summary
		if explanation.LikelyCause != "" {
			aiExplanation += " Likely cause: " + explanation.LikelyCause
		}
	}

	alert := CostAlert{
		DeploymentID:     deployment.ID,
		UserID:           deployment.UserID,
		Type:             alertType,
		Threshold:        threshold,
		Triggered:        true,
		ActualCost:       actualCost.ProjectedMonthly,
		EstimatedCost:    estimate.Total,
		Variance:         variance,
		Message:          message,
		ServiceBreakdown: actualCost.Breakdown,
		SentAt:           time.Now(),
		Acknowledged:     false,
	}

	alertRecord, err := cm.saveAlert(alert)
	if err != nil {
		return fmt.Errorf("failed to save alert: %w", err)
	}

	// Persist AI explanation on the alert record
	if aiExplanation != "" {
		alertRecord.Set("aiExplanation", aiExplanation)
		if saveErr := cm.app.Dao().SaveRecord(alertRecord); saveErr != nil {
			log.Printf("[CostMonitor] Failed to save AI explanation: %v", saveErr)
		}
	}

	alert.ID = alertRecord.Id

	// Only send email if user has email notifications enabled
	if cm.shouldSendEmailAlert(deployment.UserID) {
		userEmail, err := cm.getUserEmail(deployment.UserID)
		if err != nil {
			log.Printf("Warning: Could not get user email for alert: %v", err)
		} else {
			alertData := &notifications.CostAlertData{
				DeploymentID:       deployment.ID,
				DeploymentName:     deployment.Name,
				EstimatedCost:      estimate.Total,
				ActualCost:         actualCost.ProjectedMonthly,
				VariancePercentage: variance,
			}
			cm.notifier.SendCostAlert(userEmail, alertData)
		}
	}

	return nil
}

// getActiveDeployments retrieves all active deployments across all clouds
func (cm *CostMonitor) getActiveDeployments() ([]DeploymentInfo, error) {
	deployments := make([]DeploymentInfo, 0)

	// AWS deployments
	awsRecords, err := cm.app.Dao().FindRecordsByFilter("awsDeployments", "status = 'active' || status = 'running'", "-created", 0, 0, nil)
	if err == nil {
		for _, record := range awsRecords {
			deployments = append(deployments, DeploymentInfo{
				ID:        record.Id,
				Name:      record.GetString("name"),
				UserID:    record.GetString("user"),
				Status:    record.GetString("status"),
				CreatedAt: record.GetDateTime("created").Time(),
			})
		}
	}

	// Kubernetes deployments
	k8sRecords, err := cm.app.Dao().FindRecordsByFilter("deployments", "status = 'active' || status = 'running'", "-created", 0, 0, nil)
	if err == nil {
		for _, record := range k8sRecords {
			deployments = append(deployments, DeploymentInfo{
				ID:        record.Id,
				Name:      record.GetString("name"),
				UserID:    record.GetString("user"),
				Status:    record.GetString("status"),
				CreatedAt: record.GetDateTime("created").Time(),
			})
		}
	}

	// GCP deployments (if collection exists)
	gcpRecords, err := cm.app.Dao().FindRecordsByFilter("gcpDeployments", "status = 'active' || status = 'running'", "-created", 0, 0, nil)
	if err == nil {
		for _, record := range gcpRecords {
			deployments = append(deployments, DeploymentInfo{
				ID:        record.Id,
				Name:      record.GetString("name"),
				UserID:    record.GetString("user"),
				Status:    record.GetString("status"),
				CreatedAt: record.GetDateTime("created").Time(),
			})
		}
	}

	// Azure deployments (if collection exists)
	azureRecords, err := cm.app.Dao().FindRecordsByFilter("azureDeployments", "status = 'active' || status = 'running'", "-created", 0, 0, nil)
	if err == nil {
		for _, record := range azureRecords {
			deployments = append(deployments, DeploymentInfo{
				ID:        record.Id,
				Name:      record.GetString("name"),
				UserID:    record.GetString("user"),
				Status:    record.GetString("status"),
				CreatedAt: record.GetDateTime("created").Time(),
			})
		}
	}

	return deployments, nil
}

// getActualCost retrieves actual cost data
func (cm *CostMonitor) getActualCost(deploymentID string) (*aws.ActualCostData, error) {
	actualCost, err := cm.costFetcher.GetCachedActualCost(deploymentID)
	if err != nil {
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

// getEstimate retrieves cost estimate
func (cm *CostMonitor) getEstimate(deploymentID string) (*EstimateData, error) {
	record, err := cm.app.Dao().FindFirstRecordByFilter("costEstimates", "deployment = {:deploymentId}", map[string]any{"deploymentId": deploymentID})
	if err != nil {
		return nil, fmt.Errorf("estimate not found: %w", err)
	}
	return &EstimateData{
		Total:     record.GetFloat("totalEstimate"),
		CreatedAt: record.GetDateTime("created").Time(),
	}, nil
}

// calculateVariance calculates the percentage variance
func (cm *CostMonitor) calculateVariance(actual, estimate float64) float64 {
	if estimate == 0 {
		return 0
	}
	return ((actual - estimate) / estimate) * 100
}

// getDeploymentThreshold gets the alert threshold for a deployment
// Priority: 1) Deployment-specific threshold, 2) User preference, 3) Global default
func (cm *CostMonitor) getDeploymentThreshold(deploymentID string) float64 {
	// First, try to get deployment-specific threshold
	record, err := cm.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		record, err = cm.app.Dao().FindRecordById("deployments", deploymentID)
	}

	if err == nil {
		customThreshold := record.GetFloat("costAlertThreshold")
		if customThreshold > 0 {
			return customThreshold
		}
		
		// If no deployment-specific threshold, check user preferences
		userID := record.GetString("user")
		if userID != "" {
			return cm.getUserAlertThreshold(userID)
		}
	}
	
	// Fallback to global default
	return cm.alertThreshold
}

// hasUnacknowledgedAlert checks for unacknowledged alerts
func (cm *CostMonitor) hasUnacknowledgedAlert(deploymentID string) bool {
	record, _ := cm.app.Dao().FindFirstRecordByFilter("costAlerts", "deployment = {:deploymentId} && acknowledged = false", map[string]any{"deploymentId": deploymentID})
	return record != nil
}

// saveAlert saves an alert
func (cm *CostMonitor) saveAlert(alert CostAlert) (*models.Record, error) {
	collection, err := cm.app.Dao().FindCollectionByNameOrId("costAlerts")
	if err != nil {
		return nil, err
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
		return nil, err
	}
	return record, nil
}

// getUserEmail retrieves the email address
func (cm *CostMonitor) getUserEmail(userID string) (string, error) {
	user, err := cm.app.Dao().FindRecordById("users", userID)
	if err != nil {
		return "", err
	}
	return user.GetString("email"), nil
}

// AcknowledgeAlert marks an alert as acknowledged
func (cm *CostMonitor) AcknowledgeAlert(alertID string) error {
	alert, err := cm.app.Dao().FindRecordById("costAlerts", alertID)
	if err != nil {
		return err
	}
	alert.Set("acknowledged", true)
	alert.Set("acknowledgedAt", time.Now())
	return cm.app.Dao().SaveRecord(alert)
}
