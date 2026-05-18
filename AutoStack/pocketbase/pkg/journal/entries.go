// Package journal implements Phase 5.3 Execution Journaling + Causal Timelines.
//
// Core invariants:
//   - Journal entries are IMMUTABLE once created.
//   - Every entry is hash-verified before appending.
//   - Causal ordering is preserved through CausationID / ParentJournalID links.
//   - Journal state is NEVER auto-repaired — corruption is surfaced explicitly.
//   - Replay uses ONLY persisted evidence — no synthetic reconstruction.
package journal

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// JournalType identifies the kind of causal event recorded in the journal.
type JournalType string

const (
	JournalTypeWorkflowStarted       JournalType = "workflow_started"
	JournalTypeWorkflowPaused        JournalType = "workflow_paused"
	JournalTypeWorkflowResumed       JournalType = "workflow_resumed"
	JournalTypeWorkflowCompleted     JournalType = "workflow_completed"
	JournalTypeWorkflowFailed        JournalType = "workflow_failed"
	JournalTypeStepStarted           JournalType = "step_started"
	JournalTypeStepCompleted         JournalType = "step_completed"
	JournalTypeStepFailed            JournalType = "step_failed"
	JournalTypeRetryScheduled        JournalType = "retry_scheduled"
	JournalTypeRetryExecuted         JournalType = "retry_executed"
	JournalTypeApprovalRequested     JournalType = "approval_requested"
	JournalTypeApprovalGranted       JournalType = "approval_granted"
	JournalTypeApprovalDenied        JournalType = "approval_denied"
	JournalTypeProviderObserved      JournalType = "provider_observed"
	JournalTypeContradictionDetected JournalType = "contradiction_detected"
	JournalTypeLeaseTransferred      JournalType = "lease_transferred"
	JournalTypeExecutionQuarantined  JournalType = "execution_quarantined"
)

// ExecutionJournalEntry is an immutable causal record for one step of an execution.
// Hash covers all stable fields. Payload bytes are hashed separately when present.
type ExecutionJournalEntry struct {
	JournalID       string      `json:"journal_id"`
	ExecutionID     string      `json:"execution_id"`
	WorkflowID      string      `json:"workflow_id"`
	StepID          string      `json:"step_id"`
	Attempt         int         `json:"attempt"`
	JournalType     JournalType `json:"journal_type"`
	Timestamp       string      `json:"timestamp"`
	LogicalClock    int64       `json:"logical_clock"`
	CausationID     string      `json:"causation_id"`
	CorrelationID   string      `json:"correlation_id"`
	ParentJournalID string      `json:"parent_journal_id"`
	RuntimeNodeID   string      `json:"runtime_node_id"`
	DecisionRef     string      `json:"decision_ref,omitempty"`
	EventRef        string      `json:"event_ref,omitempty"`
	IncidentRef     string      `json:"incident_ref,omitempty"`
	VerificationRef string      `json:"verification_ref,omitempty"`
	TenantID        string      `json:"tenant_id"`
	Payload         []byte      `json:"payload,omitempty"`
	Hash            string      `json:"hash"`
}

// NewJournalEntry creates an ExecutionJournalEntry with its hash computed.
// Payload is deep-copied to preserve immutability.
func NewJournalEntry(
	journalID, executionID, workflowID, stepID string,
	attempt int,
	journalType JournalType,
	clock int64,
	causationID, correlationID, parentJournalID string,
	runtimeNodeID, tenantID string,
	payload []byte,
) ExecutionJournalEntry {
	e := ExecutionJournalEntry{
		JournalID:       journalID,
		ExecutionID:     executionID,
		WorkflowID:      workflowID,
		StepID:          stepID,
		Attempt:         attempt,
		JournalType:     journalType,
		Timestamp:       time.Now().UTC().Format(time.RFC3339Nano),
		LogicalClock:    clock,
		CausationID:     causationID,
		CorrelationID:   correlationID,
		ParentJournalID: parentJournalID,
		RuntimeNodeID:   runtimeNodeID,
		TenantID:        tenantID,
		Payload:         append([]byte(nil), payload...),
	}
	e.Hash = computeEntryHash(e)
	return e
}

// VerifyJournalEntry recomputes the hash and checks it matches the stored value.
func VerifyJournalEntry(e ExecutionJournalEntry) (bool, string) {
	stored := e.Hash
	e.Hash = ""
	expected := computeEntryHash(e)
	if stored != expected {
		return false, fmt.Sprintf("journal entry %q hash mismatch: stored=%s computed=%s",
			e.JournalID, stored, expected)
	}
	return true, ""
}

func computeEntryHash(e ExecutionJournalEntry) string {
	h := sha256.New()
	fmt.Fprintf(h,
		"journal:%s|exec:%s|workflow:%s|step:%s|attempt:%d|type:%s|ts:%s|clock:%d|causation:%s|correlation:%s|parent:%s|node:%s|tenant:%s|decision:%s|event:%s|incident:%s|verify:%s",
		e.JournalID, e.ExecutionID, e.WorkflowID, e.StepID,
		e.Attempt, e.JournalType, e.Timestamp, e.LogicalClock,
		e.CausationID, e.CorrelationID, e.ParentJournalID,
		e.RuntimeNodeID, e.TenantID,
		e.DecisionRef, e.EventRef, e.IncidentRef, e.VerificationRef,
	)
	if len(e.Payload) > 0 {
		h.Write(e.Payload)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
