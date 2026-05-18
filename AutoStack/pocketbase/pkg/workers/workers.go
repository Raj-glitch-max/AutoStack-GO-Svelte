package workers

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// WorkerState is the lifecycle state of a distributed worker.
type WorkerState string

const (
	WorkerStateOnline      WorkerState = "online"
	WorkerStateDegraded    WorkerState = "degraded"
	WorkerStateDraining    WorkerState = "draining"
	WorkerStateQuarantined WorkerState = "quarantined"
	WorkerStateOffline     WorkerState = "offline"
)

// Worker is an immutable, hashed descriptor of a runtime worker.
// RegisteredAt is excluded from the hash (content-identity).
type Worker struct {
	WorkerID     string      `json:"worker_id"`
	TenantID     string      `json:"tenant_id"`
	State        WorkerState `json:"state"`
	Capabilities []string    `json:"capabilities"`
	Region       string      `json:"region,omitempty"`
	RegisteredAt string      `json:"registered_at"` // excluded from hash
	WorkerHash   string      `json:"worker_hash"`
}

// NewWorker creates a worker in WorkerStateOnline.
func NewWorker(workerID, tenantID string, capabilities []string, region string) Worker {
	caps := make([]string, len(capabilities))
	copy(caps, capabilities)
	w := Worker{
		WorkerID:     workerID,
		TenantID:     tenantID,
		State:        WorkerStateOnline,
		Capabilities: caps,
		Region:       region,
		RegisteredAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	w.WorkerHash = computeWorkerHash(w)
	return w
}

// VerifyWorkerHash recomputes the hash and returns (true, "") when it matches.
func VerifyWorkerHash(w Worker) (bool, string) {
	stored := w.WorkerHash
	w.WorkerHash = ""
	expected := computeWorkerHash(w)
	if stored != expected {
		return false, fmt.Sprintf("worker %q hash mismatch: stored=%s computed=%s", w.WorkerID, stored, expected)
	}
	return true, ""
}

// TransitionWorker returns a new Worker with updated state (value semantics).
func TransitionWorker(w Worker, to WorkerState) Worker {
	updated := w
	updated.State = to
	updated.WorkerHash = ""
	updated.WorkerHash = computeWorkerHash(updated)
	return updated
}

// IsAvailable returns true when the worker can accept new task assignments.
func IsAvailable(w Worker) bool {
	return w.State == WorkerStateOnline
}

func computeWorkerHash(w Worker) string {
	h := sha256.New()
	fmt.Fprintf(h, "worker:%s|tenant:%s|state:%s|region:%s|caps:%d",
		w.WorkerID, w.TenantID, w.State, w.Region, len(w.Capabilities))
	for _, c := range w.Capabilities {
		fmt.Fprintf(h, "|%s", c)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
