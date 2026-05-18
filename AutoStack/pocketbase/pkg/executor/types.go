package executor

import (
	"github.com/janlauber/one-click/pkg/orchestrator"
	"github.com/janlauber/one-click/pkg/planner"
)

// ExecutionState is the explicit FSM state of a controlled execution.
// Transitions between states must be persisted before taking effect.
type ExecutionState string

const (
	StatePending              ExecutionState = "pending"
	StateAwaitingApproval     ExecutionState = "awaiting_approval"
	StateReady                ExecutionState = "ready"
	StateExecuting            ExecutionState = "executing"
	StatePaused               ExecutionState = "paused"
	StateBlocked              ExecutionState = "blocked"
	StateAmbiguous            ExecutionState = "ambiguous"
	StateRollbackRequired     ExecutionState = "rollback_required"
	StateCompensationRequired ExecutionState = "compensation_required"
	StateFailed               ExecutionState = "failed"
	StateCompleted            ExecutionState = "completed"
	StateCancelled            ExecutionState = "cancelled"
)

// NodeOutcome is the result of executing a single node.
type NodeOutcome string

const (
	OutcomePending   NodeOutcome = "pending"
	OutcomeSuccess   NodeOutcome = "success"
	OutcomeAmbiguous NodeOutcome = "ambiguous"
	OutcomeFailed    NodeOutcome = "failed"
	OutcomeSkipped   NodeOutcome = "skipped"
)

// StageOutcome is the result of executing a stage.
type StageOutcome string

const (
	StageOutcomePending   StageOutcome = "pending"
	StageOutcomeSuccess   StageOutcome = "success"
	StageOutcomeAmbiguous StageOutcome = "ambiguous"
	StageOutcomeFailed    StageOutcome = "failed"
	StageOutcomeBlocked   StageOutcome = "blocked"
)

// ExecutionTransition records a single state change in the execution FSM.
// Every transition is persisted to the journal before being applied.
type ExecutionTransition struct {
	ExecutionID string         `json:"execution_id"`
	FromState   ExecutionState `json:"from_state"`
	ToState     ExecutionState `json:"to_state"`
	Reason      string         `json:"reason"`
	Timestamp   string         `json:"timestamp"`
	// ActorID is the operator ID or "system" for automatic transitions.
	ActorID string `json:"actor_id"`
}

// NodeExecutionRecord is the per-node execution audit record.
type NodeExecutionRecord struct {
	ExecutionID         string                    `json:"execution_id"`
	StageID             string                    `json:"stage_id"`
	NodeID              string                    `json:"node_id"`
	Action              planner.ActionType        `json:"action"`
	Provider            string                    `json:"provider"`
	State               ExecutionState            `json:"state"`
	Outcome             NodeOutcome               `json:"outcome"`
	ProviderRequestID   string                    `json:"provider_request_id"`
	StartedAt           string                    `json:"started_at"`
	CompletedAt         string                    `json:"completed_at"`
	RetryCount          int                       `json:"retry_count"`
	AmbiguityID         string                    `json:"ambiguity_id"`
	RollbackRef         string                    `json:"rollback_ref"`
	CompensationRef     string                    `json:"compensation_ref"`
	OperatorApprovalRef string                    `json:"operator_approval_ref"`
	EvidenceRefs        []planner.EvidenceRef     `json:"evidence_refs"`
	FailureClass        orchestrator.FailureClass `json:"failure_class"`
	ErrorDetail         string                    `json:"error_detail"`
}

// StageExecutionRecord is the per-stage execution audit record.
type StageExecutionRecord struct {
	ExecutionID string               `json:"execution_id"`
	StageID     string               `json:"stage_id"`
	StageOrder  int                  `json:"stage_order"`
	State       ExecutionState       `json:"state"`
	Outcome     StageOutcome         `json:"outcome"`
	StartedAt   string               `json:"started_at"`
	CompletedAt string               `json:"completed_at"`
	Nodes       []NodeExecutionRecord `json:"nodes"`
}

// ApprovalDecision records an operator's approval or rejection of a gate.
// Approval decisions survive restart and are required before blocked stages advance.
type ApprovalDecision struct {
	DecisionID   string `json:"decision_id"`
	ExecutionID  string `json:"execution_id"`
	GateID       string `json:"gate_id"`
	StageID      string `json:"stage_id"`
	Approved     bool   `json:"approved"`
	OperatorID   string `json:"operator_id"`
	OperatorRole string `json:"operator_role"`
	Reason       string `json:"reason"`
	Timestamp    string `json:"timestamp"`
}

// ExecutionLease prevents concurrent executors from operating on the same execution.
// Single-node only — no distributed consensus.
type ExecutionLease struct {
	LeaseID     string `json:"lease_id"`
	ExecutionID string `json:"execution_id"`
	HolderID    string `json:"holder_id"`
	AcquiredAt  string `json:"acquired_at"`
	ExpiresAt   string `json:"expires_at"`
	Renewed     int    `json:"renewed"`
	Expired     bool   `json:"expired"`
}

// ExecutionAmbiguity is a first-class record of provider ambiguity.
// Ambiguity pauses execution and requires operator review.
// NO silent retries after ambiguity.
type ExecutionAmbiguity struct {
	AmbiguityID   string `json:"ambiguity_id"`
	ExecutionID   string `json:"execution_id"`
	StageID       string `json:"stage_id"`
	NodeID        string `json:"node_id"`
	Kind          string `json:"kind"` // "timeout"|"partial_apply"|"unverifiable"|"conflicting_state"|"verify_error"
	Description   string `json:"description"`
	ProviderState string `json:"provider_state"`
	DetectedAt    string `json:"detected_at"`
	Resolved      bool   `json:"resolved"`
	Resolution    string `json:"resolution"`
	ResolvedAt    string `json:"resolved_at"`
	OperatorID    string `json:"operator_id"`
}

// DriftReport is produced by pre-stage revalidation.
// Drift detection pauses execution but never auto-corrects.
type DriftReport struct {
	ExecutionID    string   `json:"execution_id"`
	StageID        string   `json:"stage_id"`
	NodeID         string   `json:"node_id"`
	DriftDetected  bool     `json:"drift_detected"`
	Observations   []string `json:"observations"`
	RecommendPause bool     `json:"recommend_pause"`
	GeneratedAt    string   `json:"generated_at"`
}

// RollbackRecommendation is persisted when execution detects a need for rollback.
// It NEVER triggers rollback automatically — operator action required.
type RollbackRecommendation struct {
	RecommendationID string                    `json:"recommendation_id"`
	ExecutionID      string                    `json:"execution_id"`
	StageID          string                    `json:"stage_id"`
	NodeID           string                    `json:"node_id"`
	FailureClass     orchestrator.FailureClass `json:"failure_class"`
	Reason           string                    `json:"reason"`
	RollbackStageRef string                    `json:"rollback_stage_ref"`
	CreatedAt        string                    `json:"created_at"`
	OperatorReviewed bool                      `json:"operator_reviewed"`
}

// CompensationRecommendation is persisted when execution detects a need for compensation.
// It NEVER triggers compensation automatically — operator action required.
type CompensationRecommendation struct {
	RecommendationID     string                            `json:"recommendation_id"`
	ExecutionID          string                            `json:"execution_id"`
	StageID              string                            `json:"stage_id"`
	NodeID               string                            `json:"node_id"`
	Strategy             orchestrator.CompensationStrategy `json:"strategy"`
	Reason               string                            `json:"reason"`
	CompensationStageRef string                            `json:"compensation_stage_ref"`
	CreatedAt            string                            `json:"created_at"`
	OperatorReviewed     bool                              `json:"operator_reviewed"`
}

// ProviderRequest carries the context for a provider operation.
// No credentials or secrets — those are resolved by the provider implementation.
type ProviderRequest struct {
	ExecutionID string             `json:"execution_id"`
	StageID     string             `json:"stage_id"`
	NodeID      string             `json:"node_id"`
	Provider    string             `json:"provider"`
	Action      planner.ActionType `json:"action"`
	// Context carries safe, non-secret metadata for the operation.
	Context map[string]string `json:"context"`
}

// ProviderValidation is the result of ProviderExecutor.Validate.
type ProviderValidation struct {
	Valid       bool     `json:"valid"`
	Warnings    []string `json:"warnings"`
	BlockReason string   `json:"block_reason"`
}

// ProviderResult is the result of ProviderExecutor.Execute.
type ProviderResult struct {
	RequestID     string      `json:"request_id"`
	Outcome       NodeOutcome `json:"outcome"`
	AmbiguityKind string      `json:"ambiguity_kind"` // non-empty when provider state is ambiguous
	ErrorDetail   string      `json:"error_detail"`
}

// ProviderVerification is the result of ProviderExecutor.Verify.
type ProviderVerification struct {
	Verified      bool     `json:"verified"`
	AmbiguityKind string   `json:"ambiguity_kind"`
	Evidence      []string `json:"evidence"`
}

// PauseRequest describes an operator-initiated pause.
type PauseRequest struct {
	ExecutionID string `json:"execution_id"`
	OperatorID  string `json:"operator_id"`
	Reason      string `json:"reason"`
}

// ResumeRequest describes an operator-initiated resume.
type ResumeRequest struct {
	ExecutionID string `json:"execution_id"`
	OperatorID  string `json:"operator_id"`
}

// CancellationRecord is written when an operator cancels an execution.
// It captures who cancelled, when, and why — never triggers automatic actions.
type CancellationRecord struct {
	ExecutionID string `json:"execution_id"`
	OperatorID  string `json:"operator_id"`
	Reason      string `json:"reason"`
	CancelledAt string `json:"cancelled_at"`
}
