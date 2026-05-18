package executor

import (
	"context"
	"time"

	"github.com/janlauber/one-click/pkg/orchestrator"
)

// RevalidationResult holds the outcome of pre-stage drift detection.
type RevalidationResult struct {
	ExecutionID  string
	StageID      string
	DriftReports []DriftReport
	// HasDrift is true if any node reports drift.
	HasDrift bool
	// ShouldPause is true when the executor must pause before proceeding.
	// Any detected drift unconditionally sets ShouldPause=true.
	ShouldPause bool
	GeneratedAt string
	Limitations []string
}

// RevalidateStage checks for drift between snapshot generation and actual execution time
// by re-running provider.Validate for every node in the stage.
//
// Rules:
//   - This function is read-only: it never mutates provider state.
//   - If HasDrift=true the executor MUST pause and require operator confirmation.
//   - Drift detection is best-effort — network partitions may produce false negatives.
//   - The caller is responsible for acting on ShouldPause — this function only reports.
func RevalidateStage(
	ctx context.Context,
	executionID string,
	stage orchestrator.ExecutionStage,
	provider ProviderExecutor,
) *RevalidationResult {
	result := &RevalidationResult{
		ExecutionID: executionID,
		StageID:     stage.StageID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Limitations: revalidationLimitations(),
	}

	for _, node := range stage.Nodes {
		report := DriftReport{
			ExecutionID: executionID,
			StageID:     stage.StageID,
			NodeID:      node.NodeID,
			GeneratedAt: result.GeneratedAt,
		}

		req := ProviderRequest{
			ExecutionID: executionID,
			StageID:     stage.StageID,
			NodeID:      node.NodeID,
			Provider:    node.Provider,
			Action:      node.Action,
		}

		validation, err := provider.Validate(ctx, req)
		if err != nil {
			report.DriftDetected = true
			report.RecommendPause = true
			report.Observations = append(report.Observations, "provider validation error: "+err.Error())
		} else if !validation.Valid {
			report.DriftDetected = true
			report.RecommendPause = true
			report.Observations = append(report.Observations, "precondition failed: "+validation.BlockReason)
			for _, w := range validation.Warnings {
				report.Observations = append(report.Observations, "warning: "+w)
			}
		} else {
			for _, w := range validation.Warnings {
				report.Observations = append(report.Observations, "warning: "+w)
			}
		}

		result.DriftReports = append(result.DriftReports, report)
		if report.DriftDetected {
			result.HasDrift = true
			result.ShouldPause = true
		}
	}

	return result
}

func revalidationLimitations() []string {
	return []string{
		"revalidation uses provider.Validate which may return cached or stale state — not a guaranteed real-time read",
		"network partitions or provider delays may produce false negatives — drift is not always detected",
		"drift detection never auto-corrects — operator must review and confirm before proceeding",
	}
}
