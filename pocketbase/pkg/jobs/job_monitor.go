package jobs

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// JobMonitor tracks job execution metrics and health
type JobMonitor struct {
	app           core.App
	metrics       map[string]*JobMetrics
	metricsMutex  sync.RWMutex
	alertThresholds map[string]AlertThreshold
}

// JobMetrics tracks execution statistics for a job
type JobMetrics struct {
	JobName          string        `json:"jobName"`
	TotalExecutions  int64         `json:"totalExecutions"`
	SuccessfulRuns   int64         `json:"successfulRuns"`
	FailedRuns       int64         `json:"failedRuns"`
	LastExecution    time.Time     `json:"lastExecution"`
	LastSuccess      time.Time     `json:"lastSuccess"`
	LastFailure      time.Time     `json:"lastFailure"`
	LastError        string        `json:"lastError"`
	AverageRuntime   time.Duration `json:"averageRuntime"`
	MinRuntime       time.Duration `json:"minRuntime"`
	MaxRuntime       time.Duration `json:"maxRuntime"`
	TotalRuntime     time.Duration `json:"totalRuntime"`
	SuccessRate      float64       `json:"successRate"`
	ConsecutiveFailures int64      `json:"consecutiveFailures"`
	LastHealthCheck  time.Time     `json:"lastHealthCheck"`
	IsHealthy        bool          `json:"isHealthy"`
}

// AlertThreshold defines when to trigger alerts for a job
type AlertThreshold struct {
	MaxConsecutiveFailures int64         `json:"maxConsecutiveFailures"`
	MaxFailureRate         float64       `json:"maxFailureRate"`
	MaxRuntime             time.Duration `json:"maxRuntime"`
	MinSuccessRate         float64       `json:"minSuccessRate"`
	AlertCooldown          time.Duration `json:"alertCooldown"`
	LastAlertSent          time.Time     `json:"lastAlertSent"`
}

// JobAlert represents an alert about job health
type JobAlert struct {
	JobName     string    `json:"jobName"`
	AlertType   string    `json:"alertType"`
	Message     string    `json:"message"`
	Severity    string    `json:"severity"`
	Timestamp   time.Time `json:"timestamp"`
	Metrics     *JobMetrics `json:"metrics"`
	Resolved    bool      `json:"resolved"`
}

// NewJobMonitor creates a new job monitor instance
func NewJobMonitor(app core.App) *JobMonitor {
	monitor := &JobMonitor{
		app:             app,
		metrics:         make(map[string]*JobMetrics),
		alertThresholds: make(map[string]AlertThreshold),
	}
	
	// Set default alert thresholds
	monitor.setDefaultThresholds()
	
	return monitor
}

// setDefaultThresholds sets up default alert thresholds for different job types
func (jm *JobMonitor) setDefaultThresholds() {
	// Pricing fetch job thresholds
	jm.alertThresholds["daily-pricing-fetch"] = AlertThreshold{
		MaxConsecutiveFailures: 2,
		MaxFailureRate:         0.2, // 20% failure rate
		MaxRuntime:             30 * time.Minute,
		MinSuccessRate:         0.8, // 80% success rate
		AlertCooldown:          4 * time.Hour,
	}
	
	// Health check job thresholds
	jm.alertThresholds["pricing-cache-health-check"] = AlertThreshold{
		MaxConsecutiveFailures: 5,
		MaxFailureRate:         0.3,
		MaxRuntime:             5 * time.Minute,
		MinSuccessRate:         0.7,
		AlertCooldown:          2 * time.Hour,
	}
	
	// Cleanup job thresholds
	jm.alertThresholds["pricing-data-cleanup"] = AlertThreshold{
		MaxConsecutiveFailures: 3,
		MaxFailureRate:         0.25,
		MaxRuntime:             15 * time.Minute,
		MinSuccessRate:         0.75,
		AlertCooldown:          24 * time.Hour,
	}
}

// RecordJobExecution records the execution of a job
func (jm *JobMonitor) RecordJobExecution(jobName string, duration time.Duration, err error) {
	jm.metricsMutex.Lock()
	defer jm.metricsMutex.Unlock()
	
	// Get or create metrics for this job
	metrics, exists := jm.metrics[jobName]
	if !exists {
		metrics = &JobMetrics{
			JobName:     jobName,
			MinRuntime:  duration,
			MaxRuntime:  duration,
			IsHealthy:   true,
		}
		jm.metrics[jobName] = metrics
	}
	
	// Update execution metrics
	metrics.TotalExecutions++
	metrics.LastExecution = time.Now()
	metrics.TotalRuntime += duration
	
	// Update runtime statistics
	if duration < metrics.MinRuntime || metrics.MinRuntime == 0 {
		metrics.MinRuntime = duration
	}
	if duration > metrics.MaxRuntime {
		metrics.MaxRuntime = duration
	}
	metrics.AverageRuntime = time.Duration(int64(metrics.TotalRuntime) / metrics.TotalExecutions)
	
	// Update success/failure metrics
	if err != nil {
		metrics.FailedRuns++
		metrics.LastFailure = time.Now()
		metrics.LastError = err.Error()
		metrics.ConsecutiveFailures++
	} else {
		metrics.SuccessfulRuns++
		metrics.LastSuccess = time.Now()
		metrics.ConsecutiveFailures = 0
		metrics.LastError = ""
	}
	
	// Calculate success rate
	if metrics.TotalExecutions > 0 {
		metrics.SuccessRate = float64(metrics.SuccessfulRuns) / float64(metrics.TotalExecutions)
	}
	
	// Update health status
	metrics.IsHealthy = jm.evaluateJobHealth(jobName, metrics)
	metrics.LastHealthCheck = time.Now()
	
	// Check if alerts should be triggered
	jm.checkAlertConditions(jobName, metrics, err)
	
	// Persist metrics to database
	if err := jm.persistMetrics(jobName, metrics); err != nil {
		log.Printf("Failed to persist job metrics for %s: %v", jobName, err)
	}
}

// evaluateJobHealth determines if a job is healthy based on its metrics
func (jm *JobMonitor) evaluateJobHealth(jobName string, metrics *JobMetrics) bool {
	threshold, exists := jm.alertThresholds[jobName]
	if !exists {
		// No thresholds defined, assume healthy if no recent failures
		return metrics.ConsecutiveFailures == 0
	}
	
	// Check consecutive failures
	if metrics.ConsecutiveFailures >= threshold.MaxConsecutiveFailures {
		return false
	}
	
	// Check success rate (only if we have enough data)
	if metrics.TotalExecutions >= 5 && metrics.SuccessRate < threshold.MinSuccessRate {
		return false
	}
	
	// Check if last execution was too slow
	if metrics.LastExecution.After(time.Time{}) && 
	   metrics.AverageRuntime > threshold.MaxRuntime {
		return false
	}
	
	return true
}

// checkAlertConditions checks if any alert conditions are met
func (jm *JobMonitor) checkAlertConditions(jobName string, metrics *JobMetrics, lastError error) {
	threshold, exists := jm.alertThresholds[jobName]
	if !exists {
		return
	}
	
	// Check cooldown period
	if time.Since(threshold.LastAlertSent) < threshold.AlertCooldown {
		return
	}
	
	var alerts []JobAlert
	
	// Check consecutive failures
	if metrics.ConsecutiveFailures >= threshold.MaxConsecutiveFailures {
		alert := JobAlert{
			JobName:   jobName,
			AlertType: "consecutive_failures",
			Message: fmt.Sprintf("Job %s has failed %d consecutive times", 
				jobName, metrics.ConsecutiveFailures),
			Severity:  "high",
			Timestamp: time.Now(),
			Metrics:   metrics,
		}
		alerts = append(alerts, alert)
	}
	
	// Check success rate
	if metrics.TotalExecutions >= 10 && metrics.SuccessRate < threshold.MinSuccessRate {
		alert := JobAlert{
			JobName:   jobName,
			AlertType: "low_success_rate",
			Message: fmt.Sprintf("Job %s success rate (%.1f%%) is below threshold (%.1f%%)", 
				jobName, metrics.SuccessRate*100, threshold.MinSuccessRate*100),
			Severity:  "medium",
			Timestamp: time.Now(),
			Metrics:   metrics,
		}
		alerts = append(alerts, alert)
	}
	
	// Check runtime
	if metrics.AverageRuntime > threshold.MaxRuntime {
		alert := JobAlert{
			JobName:   jobName,
			AlertType: "slow_execution",
			Message: fmt.Sprintf("Job %s average runtime (%v) exceeds threshold (%v)", 
				jobName, metrics.AverageRuntime, threshold.MaxRuntime),
			Severity:  "low",
			Timestamp: time.Now(),
			Metrics:   metrics,
		}
		alerts = append(alerts, alert)
	}
	
	// Send alerts
	for _, alert := range alerts {
		if err := jm.sendAlert(alert); err != nil {
			log.Printf("Failed to send alert for job %s: %v", jobName, err)
		} else {
			// Update last alert sent time
			threshold.LastAlertSent = time.Now()
			jm.alertThresholds[jobName] = threshold
		}
	}
}

// sendAlert sends a job alert
func (jm *JobMonitor) sendAlert(alert JobAlert) error {
	log.Printf("JOB ALERT [%s]: %s - %s", alert.Severity, alert.AlertType, alert.Message)
	
	// TODO: Integrate with notification system
	// This could send emails, Slack messages, or push notifications
	
	// For now, just log the alert
	alertJSON, _ := json.MarshalIndent(alert, "", "  ")
	log.Printf("Alert details: %s", alertJSON)
	
	// Store alert in database for dashboard
	return jm.persistAlert(alert)
}

// persistMetrics saves job metrics to the database
func (jm *JobMonitor) persistMetrics(jobName string, metrics *JobMetrics) error {
	// TODO: Implement database persistence
	// This would save to a jobMetrics collection for dashboard display
	
	log.Printf("Persisting metrics for job %s: Success rate: %.1f%%, Avg runtime: %v", 
		jobName, metrics.SuccessRate*100, metrics.AverageRuntime)
	
	return nil
}

// persistAlert saves an alert to the database
func (jm *JobMonitor) persistAlert(alert JobAlert) error {
	// TODO: Implement database persistence
	// This would save to a jobAlerts collection for dashboard display
	
	log.Printf("Persisting alert: %s - %s", alert.AlertType, alert.Message)
	
	return nil
}

// GetJobMetrics returns metrics for a specific job
func (jm *JobMonitor) GetJobMetrics(jobName string) (*JobMetrics, error) {
	jm.metricsMutex.RLock()
	defer jm.metricsMutex.RUnlock()
	
	metrics, exists := jm.metrics[jobName]
	if !exists {
		return nil, fmt.Errorf("no metrics found for job %s", jobName)
	}
	
	// Return a copy to avoid race conditions
	metricsCopy := *metrics
	return &metricsCopy, nil
}

// GetAllJobMetrics returns metrics for all monitored jobs
func (jm *JobMonitor) GetAllJobMetrics() map[string]*JobMetrics {
	jm.metricsMutex.RLock()
	defer jm.metricsMutex.RUnlock()
	
	// Return copies to avoid race conditions
	result := make(map[string]*JobMetrics)
	for jobName, metrics := range jm.metrics {
		metricsCopy := *metrics
		result[jobName] = &metricsCopy
	}
	
	return result
}

// GetSystemHealth returns overall system health based on job metrics
func (jm *JobMonitor) GetSystemHealth() (bool, []string) {
	jm.metricsMutex.RLock()
	defer jm.metricsMutex.RUnlock()
	
	var issues []string
	allHealthy := true
	
	for jobName, metrics := range jm.metrics {
		if !metrics.IsHealthy {
			allHealthy = false
			
			if metrics.ConsecutiveFailures > 0 {
				issues = append(issues, fmt.Sprintf("Job %s has %d consecutive failures", 
					jobName, metrics.ConsecutiveFailures))
			}
			
			if metrics.SuccessRate < 0.8 && metrics.TotalExecutions >= 5 {
				issues = append(issues, fmt.Sprintf("Job %s has low success rate: %.1f%%", 
					jobName, metrics.SuccessRate*100))
			}
		}
		
		// Check for stale jobs (haven't run recently)
		expectedInterval := jm.getExpectedInterval(jobName)
		if expectedInterval > 0 && time.Since(metrics.LastExecution) > expectedInterval*2 {
			allHealthy = false
			issues = append(issues, fmt.Sprintf("Job %s hasn't run recently (last: %v)", 
				jobName, metrics.LastExecution.Format("2006-01-02 15:04:05")))
		}
	}
	
	return allHealthy, issues
}

// getExpectedInterval returns the expected execution interval for a job
func (jm *JobMonitor) getExpectedInterval(jobName string) time.Duration {
	switch jobName {
	case "daily-pricing-fetch":
		return 24 * time.Hour
	case "pricing-cache-health-check":
		return 1 * time.Hour
	case "pricing-data-cleanup":
		return 7 * 24 * time.Hour // Weekly
	default:
		return 0 // Unknown interval
	}
}

// ResetJobMetrics resets metrics for a specific job
func (jm *JobMonitor) ResetJobMetrics(jobName string) error {
	jm.metricsMutex.Lock()
	defer jm.metricsMutex.Unlock()
	
	if _, exists := jm.metrics[jobName]; !exists {
		return fmt.Errorf("no metrics found for job %s", jobName)
	}
	
	// Reset to initial state
	jm.metrics[jobName] = &JobMetrics{
		JobName:   jobName,
		IsHealthy: true,
	}
	
	log.Printf("Reset metrics for job: %s", jobName)
	return nil
}

// SetAlertThreshold updates alert thresholds for a job
func (jm *JobMonitor) SetAlertThreshold(jobName string, threshold AlertThreshold) {
	jm.alertThresholds[jobName] = threshold
	log.Printf("Updated alert thresholds for job: %s", jobName)
}