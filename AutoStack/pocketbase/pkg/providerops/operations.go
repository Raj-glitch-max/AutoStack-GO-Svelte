package providerops

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// OperationType classifies the intent of a provider operation.
type OperationType string

const (
	OperationTypeDeploy     OperationType = "deploy"
	OperationTypeDestroy    OperationType = "destroy"
	OperationTypeScale      OperationType = "scale"
	OperationTypeRestart    OperationType = "restart"
	OperationTypeRollback   OperationType = "rollback"
	OperationTypeVerify     OperationType = "verify"
	OperationTypeObserve    OperationType = "observe"
	OperationTypeQuarantine OperationType = "quarantine"
)

// OperationState is the lifecycle state of a provider operation.
type OperationState string

const (
	OperationStatePending      OperationState = "pending"
	OperationStateExecuting    OperationState = "executing"
	OperationStateVerifying    OperationState = "verifying"
	OperationStateCompleted    OperationState = "completed"
	OperationStateFailed       OperationState = "failed"
	OperationStateContradicted OperationState = "contradicted"
	OperationStateRolledBack   OperationState = "rolled_back"
)

// ProviderOperation is a single, hash-locked operation record.
// CreatedAt is excluded from the hash (content-identity).
type ProviderOperation struct {
	OperationID string         `json:"operation_id"`
	ExecutionID string         `json:"execution_id"`
	TenantID    string         `json:"tenant_id"`
	ProviderID  string         `json:"provider_id"`
	Type        OperationType  `json:"type"`
	State       OperationState `json:"state"`
	ResourceRef string         `json:"resource_ref,omitempty"`
	Attempt     int            `json:"attempt"`
	CreatedAt   string         `json:"created_at"` // excluded from hash
	Hash        string         `json:"hash"`
}

// NewProviderOperation creates an operation in OperationStatePending.
func NewProviderOperation(operationID, executionID, tenantID, providerID string, opType OperationType, attempt int) ProviderOperation {
	op := ProviderOperation{
		OperationID: operationID,
		ExecutionID: executionID,
		TenantID:    tenantID,
		ProviderID:  providerID,
		Type:        opType,
		State:       OperationStatePending,
		Attempt:     attempt,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	op.Hash = computeOperationHash(op)
	return op
}

// VerifyOperationHash recomputes the hash and returns (true, "") when it matches.
func VerifyOperationHash(op ProviderOperation) (bool, string) {
	stored := op.Hash
	op.Hash = ""
	expected := computeOperationHash(op)
	if stored != expected {
		return false, fmt.Sprintf("operation %q hash mismatch: stored=%s computed=%s", op.OperationID, stored, expected)
	}
	return true, ""
}

// TransitionOperation applies a state change and returns a new ProviderOperation.
func TransitionOperation(op ProviderOperation, to OperationState) ProviderOperation {
	updated := op
	updated.State = to
	updated.Hash = ""
	updated.Hash = computeOperationHash(updated)
	return updated
}

// IsTerminalOperation returns true when the operation cannot transition further.
func IsTerminalOperation(op ProviderOperation) bool {
	switch op.State {
	case OperationStateCompleted, OperationStateFailed,
		OperationStateContradicted, OperationStateRolledBack:
		return true
	}
	return false
}

func computeOperationHash(op ProviderOperation) string {
	h := sha256.New()
	fmt.Fprintf(h, "op:%s|exec:%s|tenant:%s|provider:%s|type:%s|state:%s|resource:%s|attempt:%d",
		op.OperationID, op.ExecutionID, op.TenantID, op.ProviderID,
		op.Type, op.State, op.ResourceRef, op.Attempt)
	return fmt.Sprintf("%x", h.Sum(nil))
}
