package executor

import (
	"fmt"

	"github.com/janlauber/one-click/pkg/orchestrator"
)

// Executor is the top-level coordinator for a controlled execution lifecycle.
// It wraps ExecutionRuntime construction with snapshot integrity verification
// and provides approval/ambiguity management entry points.
//
// Executor is stateless — it holds store references, not per-execution state.
// Each call to Prepare creates a new ExecutionRuntime.
type Executor struct {
	journal   JournalStore
	approvals ApprovalStore
	ambiguity AmbiguityStore
	leases    LeaseStore
	provider  ProviderExecutor
}

// NewExecutor constructs an Executor with the given store implementations.
func NewExecutor(
	journal JournalStore,
	approvals ApprovalStore,
	ambiguity AmbiguityStore,
	leases LeaseStore,
	provider ProviderExecutor,
) *Executor {
	return &Executor{
		journal:   journal,
		approvals: approvals,
		ambiguity: ambiguity,
		leases:    leases,
		provider:  provider,
	}
}

// Prepare verifies the snapshot's export hash and creates a new ExecutionRuntime.
// Returns an error if the snapshot is nil, tampered, or the runtime cannot be created.
// The returned runtime is in StatePending — call rt.Start() to begin.
func (e *Executor) Prepare(executionID, holderID string, snap *orchestrator.ExecutionSnapshot) (*ExecutionRuntime, error) {
	if snap == nil {
		return nil, fmt.Errorf("executor: snapshot must not be nil")
	}
	ok, reason := orchestrator.VerifyExecutionSnapshotHash(snap)
	if !ok {
		return nil, fmt.Errorf("executor: snapshot integrity check failed: %s", reason)
	}
	return NewExecutionRuntime(executionID, holderID, snap, e.journal, e.approvals, e.ambiguity, e.leases, e.provider)
}

// RecordApproval persists an operator approval decision for a gate.
// The gate must have a non-empty GateID, ExecutionID, OperatorID, and OperatorRole.
func (e *Executor) RecordApproval(d ApprovalDecision) error {
	d.Approved = true
	if err := validateApprovalDecision(d); err != nil {
		return fmt.Errorf("executor: invalid approval: %w", err)
	}
	return e.approvals.RecordDecision(d)
}

// RecordRejection persists an operator rejection for a gate.
func (e *Executor) RecordRejection(d ApprovalDecision) error {
	d.Approved = false
	if err := validateApprovalDecision(d); err != nil {
		return fmt.Errorf("executor: invalid rejection: %w", err)
	}
	return e.approvals.RecordDecision(d)
}

// ResolveAmbiguity marks an ambiguity record as operator-resolved.
func (e *Executor) ResolveAmbiguity(ambiguityID, operatorID, resolution string) error {
	return e.ambiguity.Resolve(ambiguityID, operatorID, resolution)
}

// ValidatePauseRequest checks that a pause request is well-formed.
func ValidatePauseRequest(req PauseRequest) error {
	if req.ExecutionID == "" {
		return fmt.Errorf("pause: ExecutionID must be non-empty")
	}
	if req.OperatorID == "" {
		return fmt.Errorf("pause: OperatorID must be non-empty")
	}
	if req.Reason == "" {
		return fmt.Errorf("pause: Reason must be non-empty")
	}
	return nil
}

// ValidateResumeRequest checks that a resume request is well-formed.
func ValidateResumeRequest(req ResumeRequest) error {
	if req.ExecutionID == "" {
		return fmt.Errorf("resume: ExecutionID must be non-empty")
	}
	if req.OperatorID == "" {
		return fmt.Errorf("resume: OperatorID must be non-empty")
	}
	return nil
}

func validateApprovalDecision(d ApprovalDecision) error {
	if d.ExecutionID == "" {
		return fmt.Errorf("ExecutionID must be non-empty")
	}
	if d.GateID == "" {
		return fmt.Errorf("GateID must be non-empty")
	}
	if d.OperatorID == "" {
		return fmt.Errorf("OperatorID must be non-empty")
	}
	if d.OperatorRole == "" {
		return fmt.Errorf("OperatorRole must be non-empty")
	}
	return nil
}
