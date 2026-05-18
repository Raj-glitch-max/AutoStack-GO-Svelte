package observability

// ObservabilityMetrics is derived entirely from append-only evidence.
// It is NEVER the source of truth — it is a read-only view for operators.
// The underlying event stores and journals remain the authoritative record.
type ObservabilityMetrics struct {
	TenantID           string  `json:"tenant_id"`
	ExecutionID        string  `json:"execution_id"`
	TotalTasks         int     `json:"total_tasks"`
	CompletedTasks     int     `json:"completed_tasks"`
	FailedTasks        int     `json:"failed_tasks"`
	ContradictedTasks  int     `json:"contradicted_tasks"`
	QuarantinedTasks   int     `json:"quarantined_tasks"`
	RetryCount         int     `json:"retry_count"`
	ApprovalWaits      int     `json:"approval_waits"`
	ConfidenceScore    float64 `json:"confidence_score"`
	ContradictionCount int     `json:"contradiction_count"`
	ReplayDivergences  int     `json:"replay_divergences"`
	StreamEventCount   int     `json:"stream_event_count"`
	WorkerCount        int     `json:"worker_count"`
	AvailableWorkers   int     `json:"available_workers"`
}

// TaskSummary is a minimal struct callers use to compute metrics.
type TaskSummary struct {
	State string
}

// ComputeMetrics derives an ObservabilityMetrics from raw signal counts.
// All inputs are counts derived from append-only stores — no inference.
func ComputeMetrics(
	tenantID, executionID string,
	totalTasks, completedTasks, failedTasks, contradictedTasks, quarantinedTasks int,
	retryCount, approvalWaits, contradictionCount, replayDivergences int,
	confidenceScore float64,
	streamEventCount, workerCount, availableWorkers int,
) ObservabilityMetrics {
	return ObservabilityMetrics{
		TenantID:           tenantID,
		ExecutionID:        executionID,
		TotalTasks:         totalTasks,
		CompletedTasks:     completedTasks,
		FailedTasks:        failedTasks,
		ContradictedTasks:  contradictedTasks,
		QuarantinedTasks:   quarantinedTasks,
		RetryCount:         retryCount,
		ApprovalWaits:      approvalWaits,
		ConfidenceScore:    confidenceScore,
		ContradictionCount: contradictionCount,
		ReplayDivergences:  replayDivergences,
		StreamEventCount:   streamEventCount,
		WorkerCount:        workerCount,
		AvailableWorkers:   availableWorkers,
	}
}
