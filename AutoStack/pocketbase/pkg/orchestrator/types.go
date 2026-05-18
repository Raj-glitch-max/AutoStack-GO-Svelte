package orchestrator

// Phase 4.1 — Execution Orchestration Foundation.
//
// Shared enum types and small structs used across the orchestrator package.
// All types here are purely declarative — no functions, no side effects.
//
// ─────────────────────────────────────────────────────────────────────────
// PHASE 4.1 ABSOLUTE CONSTRAINTS
// ─────────────────────────────────────────────────────────────────────────
//
//   - This package NEVER calls a provider API.
//   - This package NEVER mutates infrastructure.
//   - This package NEVER schedules deployments.
//   - This package NEVER auto-executes plans.
//   - This package NEVER auto-rollbacks.
//   - This package NEVER auto-compensates.
//   - This package NEVER runs daemon workers or async loops.
//
// Every function here is a pure computation over planning artifacts.

import "github.com/janlauber/one-click/pkg/planner"

// ─────────────────────────────────────────────────────────────────────────
// FAILURE CLASSIFICATION
// ─────────────────────────────────────────────────────────────────────────

// FailureClass categorizes how an execution failure should be handled.
// Classification is deterministic: same action + risk level → same class.
type FailureClass string

const (
	// FailureClassTransient is a short-lived failure that may self-resolve.
	// No action is recommended beyond monitoring.
	FailureClassTransient FailureClass = "transient"

	// FailureClassRetryable is a failure that can be safely retried without
	// operator intervention, provided idempotency is satisfied.
	FailureClassRetryable FailureClass = "retryable"

	// FailureClassCompensatable is a failure where a compensating action can
	// restore a safe state without full rollback.
	FailureClassCompensatable FailureClass = "compensatable"

	// FailureClassRollbackRequired is a failure where the node must be
	// rolled back to prevent an inconsistent state.
	FailureClassRollbackRequired FailureClass = "rollback-required"

	// FailureClassIrreversible is a failure where neither rollback nor
	// compensation is possible. Manual intervention is required.
	FailureClassIrreversible FailureClass = "irreversible"

	// FailureClassProviderAmbiguous is a failure where the provider's actual
	// state is unknown. The operator must verify before proceeding.
	FailureClassProviderAmbiguous FailureClass = "provider-ambiguous"

	// FailureClassOperatorRequired is a failure that requires an explicit
	// human decision before any further action is taken.
	FailureClassOperatorRequired FailureClass = "operator-required"
)

// ─────────────────────────────────────────────────────────────────────────
// TRANSACTION BOUNDARIES
// ─────────────────────────────────────────────────────────────────────────

// TransactionBoundaryClass classifies the transactional semantics of an
// execution stage or node. Classification is deterministic.
type TransactionBoundaryClass string

const (
	// BoundaryAtomic means the stage can be applied as an all-or-nothing unit.
	// On failure, all nodes in the stage are rolled back.
	BoundaryAtomic TransactionBoundaryClass = "atomic"

	// BoundaryCompensatable means a failure can be addressed by applying a
	// compensating action rather than a full rollback.
	BoundaryCompensatable TransactionBoundaryClass = "compensatable"

	// BoundaryIrreversible means the stage cannot be rolled back and has no
	// compensation path. Operator must verify state manually before proceeding.
	BoundaryIrreversible TransactionBoundaryClass = "irreversible"

	// BoundaryOperatorConfirmRequired means execution must pause after this
	// stage and wait for explicit operator sign-off before continuing.
	BoundaryOperatorConfirmRequired TransactionBoundaryClass = "operator-confirm-required"
)

// ─────────────────────────────────────────────────────────────────────────
// COMPENSATION STRATEGIES
// ─────────────────────────────────────────────────────────────────────────

// CompensationStrategy describes how to restore a safe state when rollback
// is impossible. Classification is advisory; execution is operator-initiated.
type CompensationStrategy string

const (
	// CompensationRecreate re-provisions the resource from scratch using the
	// last known desired spec.
	CompensationRecreate CompensationStrategy = "recreate"

	// CompensationRebindNetwork re-establishes network bindings to replace
	// ones lost during a failed or partially-applied network change.
	CompensationRebindNetwork CompensationStrategy = "rebind-network"

	// CompensationRestoreScaling reverts replica counts and autoscaling policy
	// to the pre-change values.
	CompensationRestoreScaling CompensationStrategy = "restore-scaling"

	// CompensationRestoreConfig applies the previous configuration snapshot
	// to revert an update without full resource recreation.
	CompensationRestoreConfig CompensationStrategy = "restore-config"

	// CompensationQuarantine isolates the affected resource from the rest of
	// the system to prevent cascading failures while manual remediation occurs.
	CompensationQuarantine CompensationStrategy = "quarantine"

	// CompensationManualIntervention flags the resource for operator action.
	// No automated compensation is possible; a human must assess the state.
	CompensationManualIntervention CompensationStrategy = "manual-intervention"

	// CompensationNone means no compensation is needed (e.g., a no-op).
	CompensationNone CompensationStrategy = "none"
)

// ─────────────────────────────────────────────────────────────────────────
// TIMEOUT CLASSES
// ─────────────────────────────────────────────────────────────────────────

// TimeoutClass classifies how long an action is expected to take.
// These are planning-time estimates; actual timeouts are not enforced here.
type TimeoutClass string

const (
	TimeoutFast     TimeoutClass = "fast"      // expected < 30s
	TimeoutModerate TimeoutClass = "moderate"  // expected 30s–5m
	TimeoutSlow     TimeoutClass = "slow"      // expected 5m–30m
	TimeoutVerySlow TimeoutClass = "very-slow" // expected > 30m
)

// ─────────────────────────────────────────────────────────────────────────
// CONTRACT BUILDING BLOCKS
// ─────────────────────────────────────────────────────────────────────────

// Precondition describes something that must be true before an action is
// applied. Preconditions are advisory — they define what future execution
// tooling should verify, not what this package enforces.
type Precondition struct {
	Description string `json:"description"`
	// FailureMode describes what should happen if this precondition is not met.
	FailureMode string `json:"failure_mode"`
	// Blocking is true when execution must halt if this precondition fails.
	Blocking bool `json:"blocking"`
}

// Postcondition describes something that must be true after a successful
// action. Postconditions are advisory verification criteria.
type Postcondition struct {
	Description  string       `json:"description"`
	VerifyMethod string       `json:"verify_method"` // "poll-status" | "event-based" | "operator-confirm"
	TimeoutClass TimeoutClass `json:"timeout_class"`
}

// RollbackCondition describes when a rollback should be triggered.
// AutoTrigger is always false in Phase 4.1 — no autonomous rollback.
type RollbackCondition struct {
	TriggerDescription string `json:"trigger_description"`
	RollbackStageRef   string `json:"rollback_stage_ref"`
	// AutoTrigger is always false. Rollback requires operator initiation.
	AutoTrigger bool `json:"auto_trigger"`
}

// IdempotencyRequirements describes whether an action can be safely retried
// without producing duplicate or inconsistent provider state.
type IdempotencyRequirements struct {
	IsIdempotent bool   `json:"is_idempotent"`
	Caveat       string `json:"caveat,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────
// EXECUTION POLICIES
// ─────────────────────────────────────────────────────────────────────────

// ExecutionPolicy declares a rule for how execution should proceed.
// Policies are advisory — they describe the intended execution behavior.
type ExecutionPolicy struct {
	PolicyID  string `json:"policy_id"`
	Name      string `json:"name"`
	Rule      string `json:"rule"`       // e.g. "pause-on-failure", "sequential-rollback"
	AppliesTo string `json:"applies_to"` // "all" | specific stageID
}

// SafetyConstraint is an active safety rule for this execution plan.
type SafetyConstraint struct {
	ConstraintID string `json:"constraint_id"`
	Rule         string `json:"rule"`
	Severity     string `json:"severity"` // "block" | "warn"
	Triggered    bool   `json:"triggered"`
	Reason       string `json:"reason,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────
// RISK SUMMARY
// ─────────────────────────────────────────────────────────────────────────

// ExecutionRiskSummary aggregates risk information across all execution stages.
type ExecutionRiskSummary struct {
	HighestRisk                planner.RiskLevel `json:"highest_risk"`
	TotalStages                int               `json:"total_stages"`
	StagesRequiringApproval    int               `json:"stages_requiring_approval"`
	IrreversibleStageCount     int               `json:"irreversible_stage_count"`
	MaxBlastRadiusAcrossStages int               `json:"max_blast_radius_across_stages"`
	TotalNodes                 int               `json:"total_nodes"`
}
