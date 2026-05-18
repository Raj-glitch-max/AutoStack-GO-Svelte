package orchestrator

import (
	"time"

	"github.com/janlauber/one-click/pkg/planner"
)

// ExecutionJournalOutcome is the typed, structured outcome of a journal entry.
// Replay logic MUST use this enum — never freeform Note strings.
type ExecutionJournalOutcome string

const (
	// JournalOutcomeUnknown is the zero value — outcome not yet determined.
	JournalOutcomeUnknown ExecutionJournalOutcome = ""
	// JournalOutcomeStarted marks a pre-action "before" entry.
	JournalOutcomeStarted ExecutionJournalOutcome = "started"
	// JournalOutcomeSuccess marks a node confirmed successfully executed.
	JournalOutcomeSuccess ExecutionJournalOutcome = "success"
	// JournalOutcomeAmbiguous marks a node that ended in an ambiguous provider state.
	JournalOutcomeAmbiguous ExecutionJournalOutcome = "ambiguous"
	// JournalOutcomeFailed marks a node that definitively failed.
	JournalOutcomeFailed ExecutionJournalOutcome = "failed"
	// JournalOutcomeCancelled marks a node cancelled before completion.
	JournalOutcomeCancelled ExecutionJournalOutcome = "cancelled"
)

// Phase 4.1 — Execution journal model.
//
// The ExecutionJournalEntry is the unit of audit logging for future execution.
// In Phase 4.1, this is a model definition only — no entries are persisted.
// Future execution tooling (Phase 4.X) will produce entries during operator-
// initiated plan application.
//
// Every entry is:
//   - Immutable once written.
//   - Linked to the exact plan and stage that produced it.
//   - Linked to an approval gate reference when applicable.
//   - Replayable: a complete history of entries re-derives the execution state.

// ExecutionJournalEntry is one atomic record in the execution audit log.
type ExecutionJournalEntry struct {
	// ExecutionID identifies the execution run this entry belongs to.
	// Assigned by the execution runtime (Phase 4.X), not by the planner.
	ExecutionID string `json:"execution_id"`

	// PlanHash ties this entry to the exact plan that was executing.
	PlanHash string `json:"plan_hash"`

	// ExecutionPlanHash ties this entry to the exact execution plan.
	ExecutionPlanHash string `json:"execution_plan_hash"`

	// StageID is the execution stage this entry records.
	StageID string `json:"stage_id"`

	// NodeID is the specific node this entry records.
	NodeID string `json:"node_id"`

	// IntendedAction is what the execution engine was attempting.
	IntendedAction planner.ActionType `json:"intended_action"`

	// Timestamp is the UTC RFC3339 time when this entry was recorded.
	Timestamp string `json:"timestamp"`

	// ExpectedOutcome describes what success looks like for this entry.
	ExpectedOutcome string `json:"expected_outcome"`

	// FailureMode describes the failure class if this entry represents a failure.
	// Empty when the entry is not a failure record.
	FailureMode FailureClass `json:"failure_mode,omitempty"`

	// RollbackReference is the rollback stage ID to invoke if this entry fails.
	// Empty when rollback is not applicable.
	RollbackReference string `json:"rollback_reference,omitempty"`

	// CompensationReference is the compensation stage ID for irreversible failures.
	// Empty when compensation is not applicable.
	CompensationReference string `json:"compensation_reference,omitempty"`

	// HumanApprovalRef is the approval gate ID that was satisfied before this entry.
	// Empty when no approval gate applied to this node.
	HumanApprovalRef string `json:"human_approval_ref,omitempty"`

	// Outcome is the structured, typed outcome of this journal entry.
	// Replay logic uses this field — never inspect the Note string for outcome.
	Outcome ExecutionJournalOutcome `json:"outcome,omitempty"`

	// Note is a free-form annotation for operator context.
	Note string `json:"note,omitempty"`
}

// NewJournalEntry constructs a new ExecutionJournalEntry with the required
// fields. Timestamp is set to UTC now. Optional fields are set via the
// returned struct before persisting.
//
// NewJournalEntry is the canonical constructor — do not create entries by
// struct literal directly, as required fields may be added in future versions.
func NewJournalEntry(
	executionID string,
	planHash string,
	executionPlanHash string,
	stageID string,
	nodeID string,
	action planner.ActionType,
	expectedOutcome string,
) *ExecutionJournalEntry {
	return &ExecutionJournalEntry{
		ExecutionID:       executionID,
		PlanHash:          planHash,
		ExecutionPlanHash: executionPlanHash,
		StageID:           stageID,
		NodeID:            nodeID,
		IntendedAction:    action,
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
		ExpectedOutcome:   expectedOutcome,
	}
}

// WithFailure returns a copy of the entry with failure metadata set.
func (e ExecutionJournalEntry) WithFailure(fc FailureClass, rollbackRef, compensationRef string) ExecutionJournalEntry {
	e.FailureMode = fc
	e.RollbackReference = rollbackRef
	e.CompensationReference = compensationRef
	return e
}

// WithApproval returns a copy of the entry with the approval gate reference set.
func (e ExecutionJournalEntry) WithApproval(gateID string) ExecutionJournalEntry {
	e.HumanApprovalRef = gateID
	return e
}

// WithOutcome returns a copy of the entry with the typed outcome set.
// Use this for all completion entries — never rely on Note strings for replay logic.
func (e ExecutionJournalEntry) WithOutcome(o ExecutionJournalOutcome) ExecutionJournalEntry {
	e.Outcome = o
	return e
}

// WithNote returns a copy of the entry with the note set.
func (e ExecutionJournalEntry) WithNote(note string) ExecutionJournalEntry {
	e.Note = note
	return e
}
