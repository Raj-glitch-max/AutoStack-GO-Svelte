package workers

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"time"
)

// WorkerAssignment is an immutable, hashed record of a task-to-worker binding.
// AssignedAt is excluded from the hash (content-identity).
type WorkerAssignment struct {
	AssignmentID string `json:"assignment_id"`
	TaskID       string `json:"task_id"`
	ExecutionID  string `json:"execution_id"`
	TenantID     string `json:"tenant_id"`
	WorkerID     string `json:"worker_id"`
	Attempt      int    `json:"attempt"`
	AssignedAt   string `json:"assigned_at"` // excluded from hash
	AssignmentHash string `json:"assignment_hash"`
}

// AssignWorker selects a worker deterministically from available workers that match
// the required capabilities. Returns an error when no eligible worker exists.
func AssignWorker(assignmentID, taskID, executionID, tenantID string, attempt int, available []Worker, required []string) (WorkerAssignment, error) {
	eligible := FilterByCapabilities(available, required)
	online := make([]Worker, 0, len(eligible))
	for _, w := range eligible {
		if IsAvailable(w) {
			online = append(online, w)
		}
	}
	if len(online) == 0 {
		return WorkerAssignment{}, fmt.Errorf("no available worker with capabilities %v for task %q", required, taskID)
	}

	workerID := deterministicWorkerSelection(taskID, online)
	a := WorkerAssignment{
		AssignmentID: assignmentID,
		TaskID:       taskID,
		ExecutionID:  executionID,
		TenantID:     tenantID,
		WorkerID:     workerID,
		Attempt:      attempt,
		AssignedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	a.AssignmentHash = computeAssignmentHash(a)
	return a, nil
}

// VerifyWorkerAssignment recomputes the hash and returns (true, "") when it matches.
func VerifyWorkerAssignment(a WorkerAssignment) (bool, string) {
	stored := a.AssignmentHash
	a.AssignmentHash = ""
	expected := computeAssignmentHash(a)
	if stored != expected {
		return false, fmt.Sprintf("assignment %q hash mismatch: stored=%s computed=%s",
			a.AssignmentID, stored, expected)
	}
	return true, ""
}

// deterministicWorkerSelection picks a worker by hashing the taskID against sorted worker IDs.
func deterministicWorkerSelection(taskID string, workers []Worker) string {
	ids := make([]string, len(workers))
	for i, w := range workers {
		ids[i] = w.WorkerID
	}
	sort.Strings(ids)

	h := sha256.New()
	fmt.Fprintf(h, "worker-select:%s", taskID)
	for _, id := range ids {
		fmt.Fprintf(h, "|%s", id)
	}
	sum := h.Sum(nil)
	idx := int(sum[0])<<8|int(sum[1])
	return ids[idx%len(ids)]
}

func computeAssignmentHash(a WorkerAssignment) string {
	h := sha256.New()
	fmt.Fprintf(h, "assign:%s|task:%s|exec:%s|tenant:%s|worker:%s|attempt:%d",
		a.AssignmentID, a.TaskID, a.ExecutionID, a.TenantID, a.WorkerID, a.Attempt)
	return fmt.Sprintf("%x", h.Sum(nil))
}
