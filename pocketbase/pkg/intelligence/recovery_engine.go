package intelligence

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/pocketbase/models"
)

// RecoveryStrategy defines how to recover from a specific error
type RecoveryStrategy struct {
	Name        string
	Description string
	MaxRetries  int
	RetryDelay  time.Duration
	Execute     func(ctx context.Context, deployment *models.Record, fix *AutoFix) error
}

// RecoveryEngine handles automatic recovery from deployment failures
type RecoveryEngine struct {
	analyzer   *ErrorAnalyzer
	strategies map[string]*RecoveryStrategy
}

// NewRecoveryEngine creates a new recovery engine
func NewRecoveryEngine() *RecoveryEngine {
	engine := &RecoveryEngine{
		analyzer:   NewErrorAnalyzer(),
		strategies: make(map[string]*RecoveryStrategy),
	}
	engine.initializeStrategies()
	return engine
}

// initializeStrategies sets up recovery strategies
func (re *RecoveryEngine) initializeStrategies() {
	// Retry strategy for transient failures
	re.strategies["retry"] = &RecoveryStrategy{
		Name:        "Retry",
		Description: "Retry the operation after a delay",
		MaxRetries:  3,
		RetryDelay:  30 * time.Second,
		Execute: func(ctx context.Context, deployment *models.Record, fix *AutoFix) error {
			log.Printf("[RecoveryEngine] Executing retry strategy for deployment %s", deployment.Id)
			// The actual retry will be handled by the caller
			return nil
		},
	}

	// Rename strategy for name conflicts
	re.strategies["rename"] = &RecoveryStrategy{
		Name:        "Rename",
		Description: "Rename resource to avoid conflicts",
		MaxRetries:  1,
		RetryDelay:  0,
		Execute: func(ctx context.Context, deployment *models.Record, fix *AutoFix) error {
			log.Printf("[RecoveryEngine] Executing rename strategy for deployment %s", deployment.Id)
			// Update deployment configuration with new name
			for _, action := range fix.Actions {
				if action.Type == "update_variable" {
					// This will be handled by the terraform executor
					log.Printf("[RecoveryEngine] Will update %s to %v", action.Target, action.NewValue)
				}
			}
			return nil
		},
	}

	// Scale up strategy for resource exhaustion
	re.strategies["scale_up"] = &RecoveryStrategy{
		Name:        "Scale Up",
		Description: "Increase resource limits",
		MaxRetries:  1,
		RetryDelay:  0,
		Execute: func(ctx context.Context, deployment *models.Record, fix *AutoFix) error {
			log.Printf("[RecoveryEngine] Executing scale_up strategy for deployment %s", deployment.Id)
			// Update resource limits
			for _, action := range fix.Actions {
				if action.Type == "update_resource" {
					log.Printf("[RecoveryEngine] Will update %s to %v", action.Target, action.NewValue)
				}
			}
			return nil
		},
	}

	// Unlock strategy for state lock issues
	re.strategies["unlock"] = &RecoveryStrategy{
		Name:        "Force Unlock",
		Description: "Force unlock Terraform state",
		MaxRetries:  1,
		RetryDelay:  0,
		Execute: func(ctx context.Context, deployment *models.Record, fix *AutoFix) error {
			log.Printf("[RecoveryEngine] Executing unlock strategy for deployment %s", deployment.Id)
			// Force unlock will be handled by terraform executor
			return nil
		},
	}
}

// RecoveryAttempt represents a single recovery attempt
type RecoveryAttempt struct {
	ID           string           `json:"id"`
	DeploymentID string           `json:"deployment_id"`
	Analysis     *ErrorAnalysis   `json:"analysis"`
	Fix          *AutoFix         `json:"fix"`
	Strategy     string           `json:"strategy"`
	Status       string           `json:"status"` // pending, in_progress, success, failed
	AttemptNum   int              `json:"attempt_num"`
	StartedAt    time.Time        `json:"started_at"`
	CompletedAt  *time.Time       `json:"completed_at,omitempty"`
	Error        string           `json:"error,omitempty"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// AttemptRecovery attempts to recover from a deployment failure
func (re *RecoveryEngine) AttemptRecovery(ctx context.Context, deployment *models.Record, errorLogs string, attemptNum int) (*RecoveryAttempt, error) {
	attempt := &RecoveryAttempt{
		ID:           fmt.Sprintf("recovery_%d", time.Now().UnixNano()),
		DeploymentID: deployment.Id,
		AttemptNum:   attemptNum,
		StartedAt:    time.Now(),
		Status:       "in_progress",
		Metadata:     make(map[string]interface{}),
	}

	// Step 1: Analyze the error
	deploymentType := "terraform"
	if deployment.Collection().Name == "deployments" {
		deploymentType = "kubernetes"
	}

	analysis := re.analyzer.AnalyzeError(ctx, errorLogs, deploymentType)
	attempt.Analysis = analysis

	log.Printf("[RecoveryEngine] Error analysis complete: type=%s, severity=%s, confidence=%.2f",
		analysis.ErrorType, analysis.Severity, analysis.Confidence)

	// Step 2: Check if auto-fixable
	if !analysis.AutoFixable {
		attempt.Status = "failed"
		attempt.Error = "Error is not auto-fixable"
		now := time.Now()
		attempt.CompletedAt = &now
		return attempt, fmt.Errorf("error is not auto-fixable: %s", analysis.Description)
	}

	// Step 3: Generate fix
	fix, err := re.analyzer.SuggestAutoFix(ctx, analysis, deployment)
	if err != nil {
		attempt.Status = "failed"
		attempt.Error = err.Error()
		now := time.Now()
		attempt.CompletedAt = &now
		return attempt, fmt.Errorf("failed to generate fix: %w", err)
	}
	attempt.Fix = fix
	attempt.Strategy = fix.FixType

	log.Printf("[RecoveryEngine] Auto-fix generated: type=%s, actions=%d, confidence=%.2f",
		fix.FixType, len(fix.Actions), fix.Confidence)

	// Step 4: Check confidence threshold
	if fix.Confidence < 0.7 {
		attempt.Status = "failed"
		attempt.Error = "Fix confidence too low"
		now := time.Now()
		attempt.CompletedAt = &now
		return attempt, fmt.Errorf("fix confidence too low: %.2f", fix.Confidence)
	}

	// Step 5: Execute recovery strategy
	strategy, exists := re.strategies[fix.FixType]
	if !exists {
		attempt.Status = "failed"
		attempt.Error = "Unknown recovery strategy"
		now := time.Now()
		attempt.CompletedAt = &now
		return attempt, fmt.Errorf("unknown recovery strategy: %s", fix.FixType)
	}

	// Check if we've exceeded max retries
	if attemptNum > strategy.MaxRetries {
		attempt.Status = "failed"
		attempt.Error = "Max retries exceeded"
		now := time.Now()
		attempt.CompletedAt = &now
		return attempt, fmt.Errorf("max retries exceeded: %d", attemptNum)
	}

	// Execute the strategy
	err = strategy.Execute(ctx, deployment, fix)
	if err != nil {
		attempt.Status = "failed"
		attempt.Error = err.Error()
		now := time.Now()
		attempt.CompletedAt = &now
		return attempt, fmt.Errorf("strategy execution failed: %w", err)
	}

	attempt.Status = "success"
	now := time.Now()
	attempt.CompletedAt = &now

	log.Printf("[RecoveryEngine] Recovery attempt successful: deployment=%s, strategy=%s",
		deployment.Id, strategy.Name)

	return attempt, nil
}

// ShouldAttemptRecovery determines if recovery should be attempted
func (re *RecoveryEngine) ShouldAttemptRecovery(deployment *models.Record, attemptNum int) bool {
	// Don't attempt recovery if:
	// 1. Too many attempts already
	if attemptNum > 3 {
		return false
	}

	// 2. Deployment is in terminal state
	status := deployment.GetString("status")
	if status == "destroyed" || status == "cancelled" {
		return false
	}

	// 3. Auto-recovery is disabled for this deployment
	if deployment.GetBool("autoRecoveryDisabled") {
		return false
	}

	return true
}

// GetRecoveryHistory returns the recovery history for a deployment
func (re *RecoveryEngine) GetRecoveryHistory(deploymentID string) ([]*RecoveryAttempt, error) {
	// This would query the database for recovery attempts
	// For now, return empty slice
	return make([]*RecoveryAttempt, 0), nil
}

// RecoveryStats contains statistics about recovery attempts
type RecoveryStats struct {
	TotalAttempts    int     `json:"total_attempts"`
	SuccessfulFixes  int     `json:"successful_fixes"`
	FailedFixes      int     `json:"failed_fixes"`
	SuccessRate      float64 `json:"success_rate"`
	AvgRecoveryTime  float64 `json:"avg_recovery_time_seconds"`
	MostCommonErrors []struct {
		ErrorType string `json:"error_type"`
		Count     int    `json:"count"`
	} `json:"most_common_errors"`
}

// GetRecoveryStats returns statistics about recovery attempts
func (re *RecoveryEngine) GetRecoveryStats(ctx context.Context, userID string) (*RecoveryStats, error) {
	// This would query the database for statistics
	// For now, return empty stats
	stats := &RecoveryStats{
		TotalAttempts:   0,
		SuccessfulFixes: 0,
		FailedFixes:     0,
		SuccessRate:     0.0,
		AvgRecoveryTime: 0.0,
		MostCommonErrors: make([]struct {
			ErrorType string `json:"error_type"`
			Count     int    `json:"count"`
		}, 0),
	}

	return stats, nil
}


// GetAnalyzer returns the error analyzer instance
func (re *RecoveryEngine) GetAnalyzer() *ErrorAnalyzer {
	return re.analyzer
}
