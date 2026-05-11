package aws

import (
	"fmt"
	"log"
	"time"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/notifications"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
)

// AlertManager handles cost-related alerting logic
type AlertManager struct {
	app          core.App
	emailService *notifications.EmailService
}

// NewAlertManager creates a new AlertManager instance
func NewAlertManager(app core.App, emailService *notifications.EmailService) *AlertManager {
	return &AlertManager{
		app:          app,
		emailService: emailService,
	}
}

// CheckCostThresholds evaluates actual cost data against user-defined thresholds
// and triggers alerts if necessary.
func (am *AlertManager) CheckCostThresholds(deploymentID string, actualData *ActualCostData) error {
	deployment, err := am.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		return fmt.Errorf("failed to find deployment %s: %w", deploymentID, err)
	}

	threshold := deployment.GetFloat("costAlertThreshold")
	if threshold <= 0 {
		// Use default 20% threshold if none set
		threshold = 20.0
	}

	// Get estimate for this deployment
	estimateRecord, _ := am.app.Dao().FindFirstRecordByFilter("costEstimates", "deployment = {:id}", map[string]any{"id": deployment.Id})
	if estimateRecord == nil {
		log.Printf("Warning: No estimate found for deployment %s, cannot check threshold percentage", deployment.Id)
		return nil
	}
	estimate := estimateRecord.GetFloat("totalEstimate")
	if estimate <= 0 {
		return nil
	}

	// Calculate percentage overrun
	overrunPercent := ((actualData.ProjectedMonthly - estimate) / estimate) * 100

	log.Printf("Checking cost threshold for deployment %s: projected=%.2f, estimate=%.2f, overrun=%.1f%%, threshold=%.1f%%", 
		deploymentID, actualData.ProjectedMonthly, estimate, overrunPercent, threshold)

	if overrunPercent > threshold {
		return am.triggerAlert(deployment, actualData, threshold, "budget_exceeded")
	}

	// Also check for significant variance from estimate (e.g. > 50% overrun) as an anomaly
	if actualData.Variance > 50.0 {
		return am.triggerAlert(deployment, actualData, threshold, "anomaly_detected")
	}

	return nil
}

// triggerAlert creates a new cost alert record in the database
func (am *AlertManager) triggerAlert(deployment *models.Record, actualData *ActualCostData, threshold float64, alertType string) error {
	// Check if we've already sent a similar alert recently to avoid spam
	lastAlert, err := am.app.Dao().FindFirstRecordByFilter(
		"costAlerts",
		"deployment = {:deploymentId} && type = {:type} && created > {:recent}",
		map[string]any{
			"deploymentId": deployment.Id,
			"type":         alertType,
			"recent":       time.Now().Add(-24 * time.Hour).Format("2006-01-02 15:04:05"),
		},
	)
	if err == nil && lastAlert != nil {
		log.Printf("Skipping alert %s for deployment %s: alert already sent in last 24h", alertType, deployment.Id)
		return nil
	}

	collection, err := am.app.Dao().FindCollectionByNameOrId("costAlerts")
	if err != nil {
		return fmt.Errorf("failed to find costAlerts collection: %w", err)
	}

	record := models.NewRecord(collection)
	record.Set("deployment", deployment.Id)
	record.Set("user", deployment.GetString("user"))
	record.Set("type", alertType)
	record.Set("threshold", threshold)
	record.Set("triggered", true)
	record.Set("actualCost", actualData.CostToDate)
	record.Set("variance", actualData.Variance)
	record.Set("acknowledged", false)
	
	// Set estimated cost from the actualData context or DB
	estimateRecord, _ := am.app.Dao().FindFirstRecordByFilter("costEstimates", "deployment = {:id}", map[string]any{"id": deployment.Id})
	if estimateRecord != nil {
		record.Set("estimatedCost", estimateRecord.GetFloat("totalEstimate"))
	}

	message := ""
	if alertType == "budget_exceeded" {
		message = fmt.Sprintf("Deployment '%s' is projected to cost $%.2f this month, exceeding your threshold of $%.2f.", 
			deployment.GetString("name"), actualData.ProjectedMonthly, threshold)
	} else {
		message = fmt.Sprintf("Deployment '%s' cost variance is %.1f%% above original estimate.", 
			deployment.GetString("name"), actualData.Variance)
	}
	record.Set("message", message)
	record.Set("sentAt", time.Now())

	if err := am.app.Dao().SaveRecord(record); err != nil {
		return fmt.Errorf("failed to save cost alert: %w", err)
	}

	log.Printf("ALERT TRIGGERED: %s for deployment %s", alertType, deployment.Id)
	
	// Send email notification
	if am.emailService != nil && am.emailService.IsEnabled() {
		user, err := am.app.Dao().FindRecordById("users", deployment.GetString("user"))
		if err == nil {
			email := user.Email()
			if email != "" {
				estimate := 0.0
				estimateRecord, _ := am.app.Dao().FindFirstRecordByFilter("costEstimates", "deployment = {:id}", map[string]any{"id": deployment.Id})
				if estimateRecord != nil {
					estimate = estimateRecord.GetFloat("totalEstimate")
				}

				alertData := &notifications.CostAlertData{
					DeploymentID:       deployment.Id,
					DeploymentName:     deployment.GetString("name"),
					EstimatedCost:      estimate,
					ActualCost:         actualData.ProjectedMonthly,
					VariancePercentage: actualData.Variance,
				}
				
				if err := am.emailService.SendCostAlert(email, alertData); err != nil {
					log.Printf("Error sending cost alert email: %v", err)
				}
			}
		}
	}
	
	return nil
}
