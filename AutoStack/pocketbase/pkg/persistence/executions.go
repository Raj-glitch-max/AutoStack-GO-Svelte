package persistence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/janlauber/one-click/pkg/executor"
)

const taskKeyPrefix = "task:"

// ExecutionTaskStore persists ExecutionTasks in a DurableKVStore.
type ExecutionTaskStore struct {
	kv DurableKVStore
}

// NewExecutionTaskStore wraps a KV store for task persistence.
func NewExecutionTaskStore(kv DurableKVStore) *ExecutionTaskStore {
	return &ExecutionTaskStore{kv: kv}
}

// Save persists an ExecutionTask. Returns an error on duplicate TaskID.
func (s *ExecutionTaskStore) Save(ctx context.Context, t executor.ExecutionTask) error {
	if ok, reason := executor.VerifyTaskHash(t); !ok {
		return fmt.Errorf("task store: refusing to persist task with invalid hash: %s", reason)
	}
	b, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("task store: marshal: %w", err)
	}
	return s.kv.Put(ctx, taskKeyPrefix+t.TaskID, b)
}

// Load retrieves an ExecutionTask by ID. Returns (zero, false, nil) when not found.
func (s *ExecutionTaskStore) Load(ctx context.Context, taskID string) (executor.ExecutionTask, bool, error) {
	b, ok, err := s.kv.Get(ctx, taskKeyPrefix+taskID)
	if err != nil || !ok {
		return executor.ExecutionTask{}, ok, err
	}
	var t executor.ExecutionTask
	if err := json.Unmarshal(b, &t); err != nil {
		return executor.ExecutionTask{}, false, fmt.Errorf("task store: unmarshal: %w", err)
	}
	return t, true, nil
}

// ListForExecution returns all tasks for a given executionID.
func (s *ExecutionTaskStore) ListForExecution(ctx context.Context, executionID string) ([]executor.ExecutionTask, error) {
	all, err := s.kv.List(ctx, taskKeyPrefix)
	if err != nil {
		return nil, err
	}
	var out []executor.ExecutionTask
	for _, b := range all {
		var t executor.ExecutionTask
		if err := json.Unmarshal(b, &t); err != nil {
			continue
		}
		if t.ExecutionID == executionID {
			out = append(out, t)
		}
	}
	return out, nil
}

// SaveCheckpoint persists an ExecutionCheckpoint.
func (s *ExecutionTaskStore) SaveCheckpoint(ctx context.Context, cp executor.ExecutionCheckpoint) error {
	if ok, reason := executor.VerifyExecutionCheckpoint(cp); !ok {
		return fmt.Errorf("task store: refusing to persist checkpoint with invalid hash: %s", reason)
	}
	b, err := json.Marshal(cp)
	if err != nil {
		return fmt.Errorf("task store: checkpoint marshal: %w", err)
	}
	return s.kv.Put(ctx, "checkpoint:"+cp.CheckpointID, b)
}

// LoadCheckpoint retrieves a checkpoint by ID.
func (s *ExecutionTaskStore) LoadCheckpoint(ctx context.Context, checkpointID string) (executor.ExecutionCheckpoint, bool, error) {
	b, ok, err := s.kv.Get(ctx, "checkpoint:"+checkpointID)
	if err != nil || !ok {
		return executor.ExecutionCheckpoint{}, ok, err
	}
	var cp executor.ExecutionCheckpoint
	if err := json.Unmarshal(b, &cp); err != nil {
		return executor.ExecutionCheckpoint{}, false, err
	}
	return cp, true, nil
}
