// Package api exposes the execution control-plane as HTTP routes on the
// PocketBase Echo instance.
//
// Operator authority contract:
//   - All mutations require an explicit operator action (no auto-advance, no auto-resume).
//   - The registry is in-memory for the process lifetime; crash recovery is
//     provided by ReplayExecution reading from the durable PocketBase stores.
//   - No endpoint triggers automatic rollback, automatic compensation, or
//     automatic re-execution of any stage.
package api

import (
	"fmt"
	"sync"

	"github.com/janlauber/one-click/pkg/executor"
)

// ExecutionRegistry holds active ExecutionRuntime instances for the current
// process. It is the in-memory lookup table for the API handlers.
//
// On process restart, runtimes are reconstructed via ReplayExecution + the
// durable PocketBase stores.
type ExecutionRegistry struct {
	mu       sync.RWMutex
	runtimes map[string]*executor.ExecutionRuntime
}

// NewExecutionRegistry constructs an empty registry.
func NewExecutionRegistry() *ExecutionRegistry {
	return &ExecutionRegistry{
		runtimes: make(map[string]*executor.ExecutionRuntime),
	}
}

// Register adds a runtime to the registry. Panics if executionID is empty.
func (r *ExecutionRegistry) Register(rt *executor.ExecutionRuntime) {
	if rt == nil {
		panic("api: Register called with nil ExecutionRuntime")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runtimes[rt.ExecutionID()] = rt
}

// Get returns the runtime for the given executionID, or false if not found.
func (r *ExecutionRegistry) Get(executionID string) (*executor.ExecutionRuntime, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.runtimes[executionID]
	return rt, ok
}

// MustGet returns the runtime or an error suitable for HTTP 404 responses.
func (r *ExecutionRegistry) MustGet(executionID string) (*executor.ExecutionRuntime, error) {
	rt, ok := r.Get(executionID)
	if !ok {
		return nil, fmt.Errorf("execution %q not found in registry — it may need to be replayed after restart", executionID)
	}
	return rt, nil
}

// Count returns the number of registered runtimes.
func (r *ExecutionRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.runtimes)
}
