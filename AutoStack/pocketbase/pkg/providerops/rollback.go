package providerops

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// OperationRollbackRecord is an immutable, hashed record of a rollback decision.
// InitiatedAt is excluded from the hash (content-identity).
type OperationRollbackRecord struct {
	RollbackID      string `json:"rollback_id"`
	OperationID     string `json:"operation_id"`
	ExecutionID     string `json:"execution_id"`
	TenantID        string `json:"tenant_id"`
	ProviderID      string `json:"provider_id"`
	Reason          string `json:"reason"`
	ContradictionRef string `json:"contradiction_ref,omitempty"`
	ResourceRef     string `json:"resource_ref,omitempty"`
	Completed       bool   `json:"completed"`
	InitiatedAt     string `json:"initiated_at"` // excluded from hash
	RollbackHash    string `json:"rollback_hash"`
}

// BuildRollbackRecord creates an immutable rollback record for an operation.
func BuildRollbackRecord(rollbackID, operationID, executionID, tenantID, providerID, reason, contradictionRef, resourceRef string) OperationRollbackRecord {
	r := OperationRollbackRecord{
		RollbackID:       rollbackID,
		OperationID:      operationID,
		ExecutionID:      executionID,
		TenantID:         tenantID,
		ProviderID:       providerID,
		Reason:           reason,
		ContradictionRef: contradictionRef,
		ResourceRef:      resourceRef,
		InitiatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	r.RollbackHash = computeRollbackHash(r)
	return r
}

// CompleteRollback returns a new record with Completed=true (value semantics).
func CompleteRollback(r OperationRollbackRecord) OperationRollbackRecord {
	updated := r
	updated.Completed = true
	updated.RollbackHash = ""
	updated.RollbackHash = computeRollbackHash(updated)
	return updated
}

// VerifyRollbackHash recomputes the hash and returns (true, "") when it matches.
func VerifyRollbackHash(r OperationRollbackRecord) (bool, string) {
	stored := r.RollbackHash
	r.RollbackHash = ""
	expected := computeRollbackHash(r)
	if stored != expected {
		return false, fmt.Sprintf("rollback %q hash mismatch: stored=%s computed=%s", r.RollbackID, stored, expected)
	}
	return true, ""
}

func computeRollbackHash(r OperationRollbackRecord) string {
	h := sha256.New()
	fmt.Fprintf(h, "rollback:%s|op:%s|exec:%s|tenant:%s|provider:%s|reason:%s|contradiction:%s|resource:%s|completed:%v",
		r.RollbackID, r.OperationID, r.ExecutionID, r.TenantID, r.ProviderID,
		r.Reason, r.ContradictionRef, r.ResourceRef, r.Completed)
	return fmt.Sprintf("%x", h.Sum(nil))
}
