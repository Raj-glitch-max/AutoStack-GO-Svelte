package executor

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// TaskSnapshot is a tamper-evident point-in-time capture of one task's state.
// SnapshotAt is excluded from SnapshotHash (content-identity).
type TaskSnapshot struct {
	SnapshotID   string    `json:"snapshot_id"`
	TaskID       string    `json:"task_id"`
	ExecutionID  string    `json:"execution_id"`
	TenantID     string    `json:"tenant_id"`
	State        TaskState `json:"state"`
	AssignedWorker string  `json:"assigned_worker"`
	Attempt      int       `json:"attempt"`
	LogicalClock int64     `json:"logical_clock"`
	SnapshotAt   string    `json:"snapshot_at"` // excluded from hash
	SnapshotHash string    `json:"snapshot_hash"`
}

// BuildTaskSnapshot captures the current state of a task.
func BuildTaskSnapshot(snapshotID string, t ExecutionTask) TaskSnapshot {
	snap := TaskSnapshot{
		SnapshotID:     snapshotID,
		TaskID:         t.TaskID,
		ExecutionID:    t.ExecutionID,
		TenantID:       t.TenantID,
		State:          t.State,
		AssignedWorker: t.AssignedWorker,
		Attempt:        t.Attempt,
		LogicalClock:   t.LogicalClock,
		SnapshotAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	snap.SnapshotHash = computeTaskSnapshotHash(snap)
	return snap
}

// VerifyTaskSnapshot recomputes the hash and returns (true, "") when it matches.
func VerifyTaskSnapshot(snap TaskSnapshot) (bool, string) {
	stored := snap.SnapshotHash
	snap.SnapshotHash = ""
	expected := computeTaskSnapshotHash(snap)
	if stored != expected {
		return false, fmt.Sprintf("task snapshot %q hash mismatch: stored=%s computed=%s",
			snap.SnapshotID, stored, expected)
	}
	return true, ""
}

func computeTaskSnapshotHash(snap TaskSnapshot) string {
	h := sha256.New()
	fmt.Fprintf(h, "tasksnap:%s|task:%s|exec:%s|tenant:%s|state:%s|worker:%s|attempt:%d|clock:%d",
		snap.SnapshotID, snap.TaskID, snap.ExecutionID, snap.TenantID,
		snap.State, snap.AssignedWorker, snap.Attempt, snap.LogicalClock)
	return fmt.Sprintf("%x", h.Sum(nil))
}
