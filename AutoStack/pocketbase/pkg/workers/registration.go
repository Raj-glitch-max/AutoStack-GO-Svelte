package workers

import (
	"fmt"
	"sync"
)

// WorkerRegistry is a thread-safe, append-only registry of workers.
// Worker state can be updated; historical states are preserved in OwnershipHistory.
type WorkerRegistry struct {
	mu      sync.RWMutex
	workers map[string]Worker
}

// NewWorkerRegistry returns an empty registry.
func NewWorkerRegistry() *WorkerRegistry {
	return &WorkerRegistry{workers: make(map[string]Worker)}
}

// Register adds a worker. Returns an error on duplicate WorkerID.
func (r *WorkerRegistry) Register(w Worker) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.workers[w.WorkerID]; exists {
		return fmt.Errorf("worker %q is already registered", w.WorkerID)
	}
	r.workers[w.WorkerID] = w
	return nil
}

// UpdateState transitions a registered worker to a new state.
// Returns an error when the worker does not exist.
func (r *WorkerRegistry) UpdateState(workerID string, to WorkerState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.workers[workerID]
	if !ok {
		return fmt.Errorf("worker %q not found", workerID)
	}
	r.workers[workerID] = TransitionWorker(w, to)
	return nil
}

// Get returns the current worker descriptor. Returns (Worker{}, false) when not found.
func (r *WorkerRegistry) Get(workerID string) (Worker, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.workers[workerID]
	return w, ok
}

// AllWorkers returns a copy of all registered workers.
func (r *WorkerRegistry) AllWorkers() []Worker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Worker, 0, len(r.workers))
	for _, w := range r.workers {
		out = append(out, w)
	}
	return out
}

// AvailableWorkers returns all workers in WorkerStateOnline.
func (r *WorkerRegistry) AvailableWorkers() []Worker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Worker
	for _, w := range r.workers {
		if IsAvailable(w) {
			out = append(out, w)
		}
	}
	return out
}

// LiveWorkerIDs returns the set of workerIDs that are not offline or quarantined.
func (r *WorkerRegistry) LiveWorkerIDs() map[string]struct{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]struct{})
	for id, w := range r.workers {
		if w.State != WorkerStateOffline && w.State != WorkerStateQuarantined {
			out[id] = struct{}{}
		}
	}
	return out
}
