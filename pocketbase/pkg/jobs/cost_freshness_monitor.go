package jobs

import (
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/pocketbase/models"
)

const (
	// MaxCostDataAge is the maximum acceptable age for cost data (48 hours)
	MaxCostDataAge = 48 * time.Hour
	
	// CostDataWarningAge is when we start warning about stale cost data (36 hours)
	CostDataWarningAge = 36 * time.Hour
)

// CostFreshnessMetrics tracks cost data freshness metrics
type CostFreshnessMetrics struct {
	TotalActiveDeployments    int                        `json:"totalActiveDeployments"`
	DeploymentsWithFreshData  int                        `json:"deploymentsWithFreshData"`
	DeploymentsWithStaleData  int                        `json:"deploymentsWithStaleData"`
	DeploymentsWithoutData    int                        `json:"deploymentsWithoutData"`
	OldestDataAge             time.Duration              `json:"oldestDataAge"`
	AverageDataAge            time.Duration              `json:"averageDataAge"`
	StaleDeployments          []StaleDeploymentInfo      `json:"staleDeployments"`
	CheckedAt                 time.Time                  `json:"checkedAt"`
}

// StaleDeploymentInfo contains information about a deployment with stale cost data
type StaleDeploymentInfo struct {
	DeploymentID   string        `json:"deploymentId"`
	DeploymentName string        `json:"deploymentName"`
	LastFetchedAt  time.Time     `json:"lastFetchedAt"`
	DataAge        time.Duration `json:"dataAge"`
	Status         string        `json:"status"`
	CreatedAt      time.Time     `json:"createdAt"`
}

// createCostFreshnessMonitorJob creates the cost data freshness monitoring job
func (ps *PricingScheduler) createCostFreshnessMonitorJob() func() error {
	return func() error {
		log.Println("Starting cost data freshness monitoring job...")
		
		startTime := time.Now()
		
		// Get freshness metrics
		metrics, err := ps.checkCostDataFreshness()
		if err != nil {
			return fmt.Errorf("failed to check cost data freshness: %w", err)
		}
		
		// Log metrics
		ps.logFreshnessMetrics(metrics)
		
		// Check for stale data and send alerts if necessary
		if metrics.DeploymentsWithStaleData > 0 {
			log.Printf("WARNING: %d deployments have stale cost data (>%v old)", 
				metrics.DeploymentsWithStaleData, MaxCostDataAge)
			
			// Send alerts for stale deployments
			if err := ps.sendStaleCostDataAlerts(metrics.StaleDeployments); err != nil {
				log.Printf("Failed to send stale cost data alerts: %v", err)
			}
		}
		
		// Record metrics for monitoring dashboard
		if err := ps.recordCostFreshnessMetrics(metrics); err != nil {
			log.Printf("Failed to record cost freshness metrics: %v", err)
		}
		
		duration := time.Since(startTime)
		log.Printf("Cost freshness monitoring job completed in %v", duration)
		
		return nil
	}
}

// checkCostDataFreshness checks the freshness of cost data for all active deployments
func (ps *PricingScheduler) checkCostDataFreshness() (*CostFreshnessMetrics, error) {
	metrics := &CostFreshnessMetrics{
		StaleDeployments: make([]StaleDeploymentInfo, 0),
		CheckedAt:        time.Now(),
	}
	
	// Get all active deployments
	deployments, err := ps.app.Dao().FindRecordsByFilter(
		"deployments",
		"status = 'active'",
		"-created",
		0,
		0,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get active deployments: %w", err)
	}
	
	metrics.TotalActiveDeployments = len(deployments)
	
	if len(deployments) == 0 {
		log.Println("No active deployments found for cost freshness check")
		return metrics, nil
	}
	
	log.Printf("Checking cost data freshness for %d active deployments", len(deployments))
	
	var totalDataAge time.Duration
	var oldestDataAge time.Duration
	dataAgeCount := 0
	
	for _, deployment := range deployments {
		deploymentID := deployment.Id
		deploymentName := deployment.GetString("name")
		createdAt := deployment.GetDateTime("created").Time()
		
		// Check if deployment is old enough to have cost data (48 hours)
		deploymentAge := time.Since(createdAt)
		if deploymentAge < MaxCostDataAge {
			log.Printf("Deployment %s is too new for cost data (age: %v)", deploymentID, deploymentAge)
			continue
		}
		
		// Get actual cost data for this deployment
		actualCost, err := ps.app.Dao().FindFirstRecordByFilter(
			"actualCosts",
			"deployment = {:deploymentId}",
			map[string]any{
				"deploymentId": deploymentID,
			},
		)
		
		if err != nil {
			// No cost data found
			log.Printf("No cost data found for deployment %s (age: %v)", deploymentID, deploymentAge)
			metrics.DeploymentsWithoutData++
			
			// Add to stale deployments list
			metrics.StaleDeployments = append(metrics.StaleDeployments, StaleDeploymentInfo{
				DeploymentID:   deploymentID,
				DeploymentName: deploymentName,
				LastFetchedAt:  time.Time{}, // Zero time indicates no data
				DataAge:        deploymentAge,
				Status:         "no_data",
				CreatedAt:      createdAt,
			})
			continue
		}
		
		// Check when cost data was last fetched
		lastFetchedAt := actualCost.GetDateTime("fetchedAt").Time()
		dataAge := time.Since(lastFetchedAt)
		
		// Track data age statistics
		totalDataAge += dataAge
		dataAgeCount++
		
		if dataAge > oldestDataAge {
			oldestDataAge = dataAge
		}
		
		// Check if data is stale
		if dataAge > MaxCostDataAge {
			log.Printf("Stale cost data for deployment %s: last fetched %v ago", deploymentID, dataAge)
			metrics.DeploymentsWithStaleData++
			
			metrics.StaleDeployments = append(metrics.StaleDeployments, StaleDeploymentInfo{
				DeploymentID:   deploymentID,
				DeploymentName: deploymentName,
				LastFetchedAt:  lastFetchedAt,
				DataAge:        dataAge,
				Status:         "stale",
				CreatedAt:      createdAt,
			})
		} else if dataAge > CostDataWarningAge {
			log.Printf("Cost data aging for deployment %s: last fetched %v ago", deploymentID, dataAge)
			metrics.DeploymentsWithFreshData++ // Still considered fresh, but aging
		} else {
			metrics.DeploymentsWithFreshData++
		}
	}
	
	// Calculate average data age
	if dataAgeCount > 0 {
		metrics.AverageDataAge = totalDataAge / time.Duration(dataAgeCount)
	}
	metrics.OldestDataAge = oldestDataAge
	
	return metrics, nil
}

// logFreshnessMetrics logs cost data freshness metrics
func (ps *PricingScheduler) logFreshnessMetrics(metrics *CostFreshnessMetrics) {
	log.Printf("Cost Data Freshness Metrics:")
	log.Printf("  Total Active Deployments: %d", metrics.TotalActiveDeployments)
	log.Printf("  Deployments with Fresh Data: %d", metrics.DeploymentsWithFreshData)
	log.Printf("  Deployments with Stale Data: %d", metrics.DeploymentsWithStaleData)
	log.Printf("  Deployments without Data: %d", metrics.DeploymentsWithoutData)
	
	if metrics.AverageDataAge > 0 {
		log.Printf("  Average Data Age: %v", metrics.AverageDataAge.Round(time.Minute))
	}
	
	if metrics.OldestDataAge > 0 {
		log.Printf("  Oldest Data Age: %v", metrics.OldestDataAge.Round(time.Minute))
	}
	
	// Calculate freshness percentage
	if metrics.TotalActiveDeployments > 0 {
		freshnessPercentage := float64(metrics.DeploymentsWithFreshData) / float64(metrics.TotalActiveDeployments) * 100
		log.Printf("  Data Freshness: %.1f%%", freshnessPercentage)
	}
}

// sendStaleCostDataAlerts sends alerts for deployments with stale cost data
func (ps *PricingScheduler) sendStaleCostDataAlerts(staleDeployments []StaleDeploymentInfo) error {
	if len(staleDeployments) == 0 {
		return nil
	}
	
	log.Printf("Sending alerts for %d deployments with stale cost data", len(staleDeployments))
	
	for _, deployment := range staleDeployments {
		// Create alert message
		var alertMessage string
		if deployment.Status == "no_data" {
			alertMessage = fmt.Sprintf(
				"Cost data has never been fetched for deployment '%s' (ID: %s). "+
				"Deployment is %v old and should have cost data available.",
				deployment.DeploymentName,
				deployment.DeploymentID,
				time.Since(deployment.CreatedAt).Round(time.Hour),
			)
		} else {
			alertMessage = fmt.Sprintf(
				"Cost data for deployment '%s' (ID: %s) is stale. "+
				"Last fetched %v ago (threshold: %v).",
				deployment.DeploymentName,
				deployment.DeploymentID,
				deployment.DataAge.Round(time.Hour),
				MaxCostDataAge,
			)
		}
		
		log.Printf("Alert: %s", alertMessage)
		
		// TODO: Integrate with notification system to send email/Slack alerts
		// For now, we'll create an alert record in the database
		if err := ps.createCostFreshnessAlert(deployment, alertMessage); err != nil {
			log.Printf("Failed to create alert for deployment %s: %v", deployment.DeploymentID, err)
			continue
		}
	}
	
	return nil
}

// createCostFreshnessAlert creates an alert record for stale cost data
func (ps *PricingScheduler) createCostFreshnessAlert(deployment StaleDeploymentInfo, message string) error {
	// Check if an alert already exists for this deployment
	existingAlert, err := ps.app.Dao().FindFirstRecordByFilter(
		"costAlerts",
		"deployment = {:deploymentId} && type = 'cost_data_stale' && acknowledged = false",
		map[string]any{
			"deploymentId": deployment.DeploymentID,
		},
	)
	
	// If alert already exists and is recent (within 24 hours), don't create a duplicate
	if err == nil && existingAlert != nil {
		createdAt := existingAlert.GetDateTime("created").Time()
		if time.Since(createdAt) < 24*time.Hour {
			log.Printf("Recent alert already exists for deployment %s, skipping", deployment.DeploymentID)
			return nil
		}
	}
	
	// Get the deployment record to find the user
	deploymentRecord, err := ps.app.Dao().FindRecordById("deployments", deployment.DeploymentID)
	if err != nil {
		return fmt.Errorf("failed to find deployment record: %w", err)
	}
	
	userID := deploymentRecord.GetString("user")
	
	// Create new alert
	collection, err := ps.app.Dao().FindCollectionByNameOrId("costAlerts")
	if err != nil {
		return fmt.Errorf("failed to find costAlerts collection: %w", err)
	}
	
	record := models.NewRecord(collection)
	record.Set("deployment", deployment.DeploymentID)
	record.Set("user", userID)
	record.Set("type", "cost_data_stale")
	record.Set("threshold", 0.0) // Not applicable for freshness alerts
	record.Set("triggered", true)
	record.Set("actualCost", 0.0)
	record.Set("estimatedCost", 0.0)
	record.Set("variance", 0.0)
	record.Set("message", message)
	record.Set("sentAt", time.Now())
	record.Set("acknowledged", false)
	
	if err := ps.app.Dao().SaveRecord(record); err != nil {
		return fmt.Errorf("failed to save alert record: %w", err)
	}
	
	log.Printf("Created cost freshness alert for deployment %s", deployment.DeploymentID)
	return nil
}

// recordCostFreshnessMetrics records cost freshness metrics for monitoring
func (ps *PricingScheduler) recordCostFreshnessMetrics(metrics *CostFreshnessMetrics) error {
	log.Printf("Recording cost freshness metrics...")
	
	// TODO: Store metrics in a monitoring collection or external monitoring system
	// This could be implemented as a separate collection: costFreshnessMetrics
	/*
	collection, err := ps.app.Dao().FindCollectionByNameOrId("costFreshnessMetrics")
	if err != nil {
		return fmt.Errorf("failed to find costFreshnessMetrics collection: %w", err)
	}
	
	record := models.NewRecord(collection)
	record.Set("totalActiveDeployments", metrics.TotalActiveDeployments)
	record.Set("deploymentsWithFreshData", metrics.DeploymentsWithFreshData)
	record.Set("deploymentsWithStaleData", metrics.DeploymentsWithStaleData)
	record.Set("deploymentsWithoutData", metrics.DeploymentsWithoutData)
	record.Set("oldestDataAge", metrics.OldestDataAge.Seconds())
	record.Set("averageDataAge", metrics.AverageDataAge.Seconds())
	record.Set("checkedAt", metrics.CheckedAt)
	
	return ps.app.Dao().SaveRecord(record)
	*/
	
	return nil
}

// GetCostDataFreshnessStatus returns the current cost data freshness status
func (ps *PricingScheduler) GetCostDataFreshnessStatus() (*CostFreshnessMetrics, error) {
	return ps.checkCostDataFreshness()
}

// GetStaleDeployments returns a list of deployments with stale cost data
func (ps *PricingScheduler) GetStaleDeployments() ([]StaleDeploymentInfo, error) {
	metrics, err := ps.checkCostDataFreshness()
	if err != nil {
		return nil, err
	}
	
	return metrics.StaleDeployments, nil
}

// IsCostDataFresh checks if cost data is fresh for a specific deployment
func (ps *PricingScheduler) IsCostDataFresh(deploymentID string) (bool, time.Duration, error) {
	// Get actual cost data
	actualCost, err := ps.app.Dao().FindFirstRecordByFilter(
		"actualCosts",
		"deployment = {:deploymentId}",
		map[string]any{
			"deploymentId": deploymentID,
		},
	)
	
	if err != nil {
		return false, 0, fmt.Errorf("no cost data found for deployment: %w", err)
	}
	
	lastFetchedAt := actualCost.GetDateTime("fetchedAt").Time()
	dataAge := time.Since(lastFetchedAt)
	
	isFresh := dataAge <= MaxCostDataAge
	
	return isFresh, dataAge, nil
}
