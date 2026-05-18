package executor

import (
	"fmt"
	"time"
)

// allowedTaskTransitions defines valid state machine edges.
var allowedTaskTransitions = map[TaskState][]TaskState{
	TaskStatePending:          {TaskStateQueued, TaskStateFailed, TaskStateQuarantined},
	TaskStateQueued:           {TaskStateDispatched, TaskStateFailed, TaskStateQuarantined},
	TaskStateDispatched:       {TaskStateRunning, TaskStateFailed, TaskStateQuarantined},
	TaskStateRunning:          {TaskStateCompleted, TaskStateFailed, TaskStatePaused, TaskStateVerificationWait, TaskStateRetryWait, TaskStateContradicted, TaskStateQuarantined},
	TaskStatePaused:           {TaskStateRunning, TaskStateFailed, TaskStateQuarantined},
	TaskStateRetryWait:        {TaskStateQueued, TaskStateFailed, TaskStateQuarantined},
	TaskStateVerificationWait: {TaskStateCompleted, TaskStateFailed, TaskStateContradicted, TaskStateQuarantined},
	TaskStateContradicted:     {TaskStateQuarantined, TaskStateRolledBack},
	TaskStateFailed:           {TaskStateRetryWait, TaskStateRolledBack, TaskStateQuarantined},
	// Terminal states have no outgoing transitions.
	TaskStateCompleted:  {},
	TaskStateQuarantined: {},
	TaskStateRolledBack: {},
}

// ValidateTaskTransition returns an error if the transition is not permitted.
func ValidateTaskTransition(from, to TaskState) error {
	allowed, ok := allowedTaskTransitions[from]
	if !ok {
		return fmt.Errorf("unknown task state %q", from)
	}
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("task transition %q → %q is not permitted", from, to)
}

// TransitionTask applies a state change and returns a new ExecutionTask.
// The original task is never mutated. Returns an error for invalid transitions.
func TransitionTask(t ExecutionTask, to TaskState) (ExecutionTask, error) {
	if err := ValidateTaskTransition(t.State, to); err != nil {
		return t, err
	}
	updated := t
	updated.State = to
	updated.Hash = ""
	updated.Hash = computeTaskHash(updated)
	return updated, nil
}

// MarkTaskStarted transitions to running and stamps StartedAt.
func MarkTaskStarted(t ExecutionTask, workerID string) (ExecutionTask, error) {
	if t.State != TaskStateDispatched {
		return t, fmt.Errorf("MarkTaskStarted requires dispatched state, got %s", t.State)
	}
	updated := t
	updated.State = TaskStateRunning
	updated.AssignedWorker = workerID
	updated.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	updated.Hash = ""
	updated.Hash = computeTaskHash(updated)
	return updated, nil
}

// MarkTaskCompleted transitions to completed and stamps CompletedAt.
func MarkTaskCompleted(t ExecutionTask) (ExecutionTask, error) {
	if t.State != TaskStateRunning && t.State != TaskStateVerificationWait {
		return t, fmt.Errorf("MarkTaskCompleted requires running or verification_wait, got %s", t.State)
	}
	updated := t
	updated.State = TaskStateCompleted
	updated.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	updated.Hash = ""
	updated.Hash = computeTaskHash(updated)
	return updated, nil
}

// MarkTaskFailed transitions to failed and stamps CompletedAt.
func MarkTaskFailed(t ExecutionTask) (ExecutionTask, error) {
	if err := ValidateTaskTransition(t.State, TaskStateFailed); err != nil {
		return t, err
	}
	updated := t
	updated.State = TaskStateFailed
	updated.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	updated.Hash = ""
	updated.Hash = computeTaskHash(updated)
	return updated, nil
}

// MarkTaskContradicted transitions to contradicted state.
func MarkTaskContradicted(t ExecutionTask) (ExecutionTask, error) {
	return TransitionTask(t, TaskStateContradicted)
}

// MarkTaskQuarantined transitions to quarantined state.
func MarkTaskQuarantined(t ExecutionTask) (ExecutionTask, error) {
	return TransitionTask(t, TaskStateQuarantined)
}
