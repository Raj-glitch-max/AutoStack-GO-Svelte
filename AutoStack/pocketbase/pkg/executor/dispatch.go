package executor

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"time"
)

// DispatchRecord is an immutable, hashed record of a task dispatch event.
// DispatchedAt is excluded from the hash (content-identity).
type DispatchRecord struct {
	DispatchID  string `json:"dispatch_id"`
	TaskID      string `json:"task_id"`
	ExecutionID string `json:"execution_id"`
	TenantID    string `json:"tenant_id"`
	WorkerID    string `json:"worker_id"`
	Attempt     int    `json:"attempt"`
	DispatchedAt string `json:"dispatched_at"` // excluded from hash
	Hash        string `json:"hash"`
}

// DispatchTask assigns a task to a worker deterministically and returns the updated task
// and an immutable dispatch record. Available workers must be non-empty.
func DispatchTask(task ExecutionTask, dispatchID string, availableWorkers []string) (ExecutionTask, DispatchRecord, error) {
	if len(availableWorkers) == 0 {
		return task, DispatchRecord{}, fmt.Errorf("dispatch: no available workers for task %q", task.TaskID)
	}
	if task.State != TaskStateQueued {
		return task, DispatchRecord{}, fmt.Errorf("dispatch: task %q must be queued, got %s", task.TaskID, task.State)
	}

	workerID := deterministicWorkerAssignment(task.TaskID, availableWorkers)

	updated := task
	updated.State = TaskStateDispatched
	updated.AssignedWorker = workerID
	updated.Hash = ""
	updated.Hash = computeTaskHash(updated)

	rec := DispatchRecord{
		DispatchID:   dispatchID,
		TaskID:       task.TaskID,
		ExecutionID:  task.ExecutionID,
		TenantID:     task.TenantID,
		WorkerID:     workerID,
		Attempt:      task.Attempt,
		DispatchedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	rec.Hash = computeDispatchHash(rec)
	return updated, rec, nil
}

// VerifyDispatchRecord recomputes the hash and returns (true, "") when it matches.
func VerifyDispatchRecord(r DispatchRecord) (bool, string) {
	stored := r.Hash
	r.Hash = ""
	expected := computeDispatchHash(r)
	if stored != expected {
		return false, fmt.Sprintf("dispatch %q hash mismatch: stored=%s computed=%s", r.DispatchID, stored, expected)
	}
	return true, ""
}

// deterministicWorkerAssignment picks a worker by hashing the taskID against the sorted
// worker list. Same taskID + same workers always produces the same assignment.
func deterministicWorkerAssignment(taskID string, workers []string) string {
	sorted := make([]string, len(workers))
	copy(sorted, workers)
	sort.Strings(sorted)

	h := sha256.New()
	fmt.Fprintf(h, "assign:%s", taskID)
	for _, w := range sorted {
		fmt.Fprintf(h, "|%s", w)
	}
	sum := h.Sum(nil)
	idx := int(sum[0])<<8|int(sum[1])
	return sorted[idx%len(sorted)]
}

func computeDispatchHash(r DispatchRecord) string {
	h := sha256.New()
	fmt.Fprintf(h, "dispatch:%s|task:%s|exec:%s|tenant:%s|worker:%s|attempt:%d",
		r.DispatchID, r.TaskID, r.ExecutionID, r.TenantID, r.WorkerID, r.Attempt)
	return fmt.Sprintf("%x", h.Sum(nil))
}
