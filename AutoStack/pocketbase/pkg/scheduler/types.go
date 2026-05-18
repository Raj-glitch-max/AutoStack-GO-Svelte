// Package scheduler provides deterministic, replay-safe execution scheduling.
//
// Core invariant: given the same ExecutionPlan, BuildStageQueue always produces
// the identical ordering. No map iteration, no clock-dependent ordering.
//
// The scheduler NEVER:
//   - auto-progresses blocked stages
//   - runs background goroutines
//   - dynamically reorders a running queue
//   - resumes ambiguity automatically
//   - skips approval gates
package scheduler

import (
	"github.com/janlauber/one-click/pkg/orchestrator"
	"github.com/janlauber/one-click/pkg/planner"
)

const SchedulingSnapshotSchemaVersion = "4.5.0"

// SchedulerState describes the current state of the scheduler for an execution.
type SchedulerState string

const (
	// SchedulerRunning: queue has entries and is actively progressable.
	SchedulerRunning SchedulerState = "running"
	// SchedulerPaused: scheduler was explicitly paused; queue is frozen.
	SchedulerPaused SchedulerState = "paused"
	// SchedulerDrained: all stages have been dequeued; nothing left to schedule.
	SchedulerDrained SchedulerState = "drained"
	// SchedulerBlocked: one or more stages are blocked (approval/ambiguity).
	SchedulerBlocked SchedulerState = "blocked"
)

// BlockReason classifies why a stage cannot advance.
type BlockReason string

const (
	BlockReasonApprovalPending   BlockReason = "approval-pending"
	BlockReasonAmbiguityPending  BlockReason = "ambiguity-pending"
	BlockReasonDependencyPending BlockReason = "dependency-pending"
	BlockReasonPaused            BlockReason = "paused"
)

// QueueEntry is one schedulable stage in the deterministic queue.
// Ordering uses DependencyDepth → RiskLevel → StageOrder → StageID.
type QueueEntry struct {
	// StageID uniquely identifies the stage within an ExecutionPlan.
	StageID string `json:"stage_id"`
	// StageOrder is the index from the originating ExecutionStage.StageOrder.
	StageOrder int `json:"stage_order"`
	// DependencyDepth is the maximum depth from a root stage in the dependency DAG.
	// Stages with depth 0 have no dependencies.
	DependencyDepth int `json:"dependency_depth"`
	// RiskLevel is the highest risk across all nodes in the stage.
	RiskLevel planner.RiskLevel `json:"risk_level"`
	// Providers is the sorted, deduplicated list of cloud providers touched by this stage.
	Providers []string `json:"providers"`
	// BlastRadius is the EstimatedBlastRadius from the ExecutionStage.
	BlastRadius int `json:"blast_radius"`
	// DependsOn lists stageIDs that must complete before this stage.
	DependsOn []string `json:"depends_on"`
}

// StageQueue is the deterministic ordered list of schedulable stages.
// Entries are sorted once at construction and never reordered during a run.
type StageQueue struct {
	ExecutionID string       `json:"execution_id"`
	Entries     []QueueEntry `json:"entries"`
	CreatedAt   string       `json:"created_at"`
}

// ConcurrencyGroup is a set of stages that can execute in parallel.
// Parallelism is permitted only when: no dependency overlap, no provider conflict,
// no blast-radius overlap, no shared mutable provider resources.
type ConcurrencyGroup struct {
	// GroupID is a stable, deterministic identifier for this group.
	GroupID  string   `json:"group_id"`
	StageIDs []string `json:"stage_ids"`
	// Rationale explains why these stages can run concurrently.
	Rationale string `json:"rationale"`
}

// BlockedStage records why a stage cannot currently advance.
type BlockedStage struct {
	StageID       string      `json:"stage_id"`
	BlockedSince  string      `json:"blocked_since"`
	BlockReason   BlockReason `json:"block_reason"`
	DeferredCount int         `json:"deferred_count"`
}

// SchedulerFairnessState tracks execution fairness metadata.
// It is updated after every scheduling decision to detect starvation.
type SchedulerFairnessState struct {
	ExecutionID   string         `json:"execution_id"`
	BlockedStages []BlockedStage `json:"blocked_stages"`
	// DeferredCounts counts how many times each stageID has been deferred.
	DeferredCounts map[string]int `json:"deferred_counts"`
	// PauseReasons records the reason each stageID is paused.
	PauseReasons map[string]string `json:"pause_reasons"`
	// ReplayOrderingMetadata captures ordering metadata for replay determinism.
	ReplayOrderingMetadata map[string]string `json:"replay_ordering_metadata"`
	UpdatedAt              string            `json:"updated_at"`
}

// PauseRecord captures a complete, reproducible snapshot of the scheduler
// at the moment of a pause. ResumeExecutionSchedule uses this to reconstruct
// the exact same queue order — no reshuffling.
type PauseRecord struct {
	ExecutionID string `json:"execution_id"`
	PausedAt    string `json:"paused_at"`
	// PausedBy is the operator ID who issued the pause.
	PausedBy    string `json:"paused_by"`
	PauseReason string `json:"pause_reason"`
	// QueueState is the exact queue at pause time.
	QueueState StageQueue `json:"queue_state"`
	// ConcurrencyState is the concurrency groups at pause time.
	ConcurrencyState  []ConcurrencyGroup     `json:"concurrency_state"`
	FairnessState     SchedulerFairnessState `json:"fairness_state"`
	PendingApprovals  []string               `json:"pending_approvals"`
	AmbiguityBlockers []string               `json:"ambiguity_blockers"`
}

// SchedulerReplayEvent is one entry in the scheduler's replay log.
// Replay events are immutable once recorded.
type SchedulerReplayEvent struct {
	// Sequence is a monotonically increasing counter — the primary sort key.
	Sequence  int               `json:"sequence"`
	StageID   string            `json:"stage_id"`
	EventType string            `json:"event_type"` // "enqueued","dequeued","paused","resumed","blocked","deferred"
	Timestamp string            `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// SchedulingSnapshot is a tamper-evident, self-describing export of scheduler state.
// GeneratedAt and SchedulerHash are excluded from the hash to ensure content identity.
type SchedulingSnapshot struct {
	SchemaVersion     string                 `json:"schema_version"`
	ExecutionID       string                 `json:"execution_id"`
	State             SchedulerState         `json:"state"`
	Queue             StageQueue             `json:"queue"`
	ConcurrencyGroups []ConcurrencyGroup     `json:"concurrency_groups"`
	FairnessState     SchedulerFairnessState `json:"fairness_state"`
	// PauseRecord is non-nil only when State == SchedulerPaused.
	PauseRecord *PauseRecord           `json:"pause_record,omitempty"`
	ReplayLog   []SchedulerReplayEvent `json:"replay_log"`
	// ReplayOrder is the stable stageID ordering derived from the replay log.
	ReplayOrder []string `json:"replay_order"`
	// Limitations documents what this scheduler snapshot cannot guarantee.
	Limitations []string `json:"limitations"`
	GeneratedAt  string  `json:"generated_at"`
	SchedulerHash string `json:"scheduler_hash"`
}

// scheduleInput bundles ExecutionPlan stages for internal use.
// Avoids re-reading the full plan in multiple functions.
type scheduleInput struct {
	stages     []orchestrator.ExecutionStage
	depthIndex map[string]int // stageID → depth
}
