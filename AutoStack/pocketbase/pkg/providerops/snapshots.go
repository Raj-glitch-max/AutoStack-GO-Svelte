package providerops

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// ProviderOpsSnapshot captures a point-in-time view of all operations for an execution.
// SnapshotAt is excluded from SnapshotHash (content-identity).
type ProviderOpsSnapshot struct {
	SnapshotID      string                  `json:"snapshot_id"`
	ExecutionID     string                  `json:"execution_id"`
	TenantID        string                  `json:"tenant_id"`
	Operations      []ProviderOperation     `json:"operations"`
	Contradictions  []ProviderContradiction `json:"contradictions"`
	Rollbacks       []OperationRollbackRecord `json:"rollbacks"`
	Reconciliations []ProviderReconciliation `json:"reconciliations"`
	SnapshotAt      string                  `json:"snapshot_at"` // excluded from hash
	SnapshotHash    string                  `json:"snapshot_hash"`
}

// BuildProviderOpsSnapshot assembles a tamper-evident snapshot.
func BuildProviderOpsSnapshot(snapshotID, executionID, tenantID string,
	ops []ProviderOperation,
	contradictions []ProviderContradiction,
	rollbacks []OperationRollbackRecord,
	reconciliations []ProviderReconciliation,
) ProviderOpsSnapshot {
	opsCopy := make([]ProviderOperation, len(ops))
	copy(opsCopy, ops)
	cCopy := make([]ProviderContradiction, len(contradictions))
	copy(cCopy, contradictions)
	rCopy := make([]OperationRollbackRecord, len(rollbacks))
	copy(rCopy, rollbacks)
	reconCopy := make([]ProviderReconciliation, len(reconciliations))
	copy(reconCopy, reconciliations)

	snap := ProviderOpsSnapshot{
		SnapshotID:      snapshotID,
		ExecutionID:     executionID,
		TenantID:        tenantID,
		Operations:      opsCopy,
		Contradictions:  cCopy,
		Rollbacks:       rCopy,
		Reconciliations: reconCopy,
		SnapshotAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	snap.SnapshotHash = computeProviderOpsSnapshotHash(snap)
	return snap
}

// VerifyProviderOpsSnapshot recomputes the hash and returns (true, "") when it matches.
func VerifyProviderOpsSnapshot(snap ProviderOpsSnapshot) (bool, string) {
	stored := snap.SnapshotHash
	snap.SnapshotHash = ""
	expected := computeProviderOpsSnapshotHash(snap)
	if stored != expected {
		return false, fmt.Sprintf("providerops snapshot %q hash mismatch: stored=%s computed=%s",
			snap.SnapshotID, stored, expected)
	}
	return true, ""
}

func computeProviderOpsSnapshotHash(snap ProviderOpsSnapshot) string {
	h := sha256.New()
	fmt.Fprintf(h, "provsnap:%s|exec:%s|tenant:%s|ops:%d|contradictions:%d|rollbacks:%d|reconciliations:%d",
		snap.SnapshotID, snap.ExecutionID, snap.TenantID,
		len(snap.Operations), len(snap.Contradictions), len(snap.Rollbacks), len(snap.Reconciliations))
	for _, op := range snap.Operations {
		fmt.Fprintf(h, "|op:%s/%s", op.OperationID, op.Hash)
	}
	for _, c := range snap.Contradictions {
		fmt.Fprintf(h, "|c:%s/%s", c.ContradictionID, c.ContradictionHash)
	}
	for _, r := range snap.Rollbacks {
		fmt.Fprintf(h, "|rb:%s/%s", r.RollbackID, r.RollbackHash)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
