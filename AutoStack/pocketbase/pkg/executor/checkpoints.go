package executor

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// TaskCheckpointEntry captures the persisted state of one task at checkpoint time.
type TaskCheckpointEntry struct {
	TaskID         string    `json:"task_id"`
	State          TaskState `json:"state"`
	AssignedWorker string    `json:"assigned_worker"`
	Attempt        int       `json:"attempt"`
	LogicalClock   int64     `json:"logical_clock"`
	RetryRef       string    `json:"retry_ref,omitempty"`
}

// ExecutionCheckpoint is a tamper-evident snapshot of all active task states.
// CreatedAt is excluded from CheckpointHash (content-identity).
type ExecutionCheckpoint struct {
	CheckpointID string                `json:"checkpoint_id"`
	ExecutionID  string                `json:"execution_id"`
	TenantID     string                `json:"tenant_id"`
	AtClock      int64                 `json:"at_clock"`
	Tasks        []TaskCheckpointEntry `json:"tasks"`
	ActiveCount  int                   `json:"active_count"`
	CreatedAt    string                `json:"created_at"` // excluded from hash
	CheckpointHash string              `json:"checkpoint_hash"`
}

// BuildExecutionCheckpoint captures all provided tasks into a checkpoint.
func BuildExecutionCheckpoint(checkpointID, executionID, tenantID string, atClock int64, tasks []ExecutionTask) ExecutionCheckpoint {
	entries := make([]TaskCheckpointEntry, len(tasks))
	activeCount := 0
	for i, t := range tasks {
		entries[i] = TaskCheckpointEntry{
			TaskID:         t.TaskID,
			State:          t.State,
			AssignedWorker: t.AssignedWorker,
			Attempt:        t.Attempt,
			LogicalClock:   t.LogicalClock,
			RetryRef:       t.RetryRef,
		}
		if !IsTerminalTask(t) {
			activeCount++
		}
	}
	cp := ExecutionCheckpoint{
		CheckpointID: checkpointID,
		ExecutionID:  executionID,
		TenantID:     tenantID,
		AtClock:      atClock,
		Tasks:        entries,
		ActiveCount:  activeCount,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	cp.CheckpointHash = computeCheckpointHash(cp)
	return cp
}

// VerifyExecutionCheckpoint recomputes the hash and returns (true, "") when it matches.
func VerifyExecutionCheckpoint(cp ExecutionCheckpoint) (bool, string) {
	stored := cp.CheckpointHash
	cp.CheckpointHash = ""
	expected := computeCheckpointHash(cp)
	if stored != expected {
		return false, fmt.Sprintf("checkpoint %q hash mismatch: stored=%s computed=%s", cp.CheckpointID, stored, expected)
	}
	return true, ""
}

func computeCheckpointHash(cp ExecutionCheckpoint) string {
	h := sha256.New()
	fmt.Fprintf(h, "cp:%s|exec:%s|tenant:%s|clock:%d|tasks:%d|active:%d",
		cp.CheckpointID, cp.ExecutionID, cp.TenantID, cp.AtClock,
		len(cp.Tasks), cp.ActiveCount)
	for _, e := range cp.Tasks {
		fmt.Fprintf(h, "|t:%s/%s/%s/%d/%d", e.TaskID, e.State, e.AssignedWorker, e.Attempt, e.LogicalClock)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
