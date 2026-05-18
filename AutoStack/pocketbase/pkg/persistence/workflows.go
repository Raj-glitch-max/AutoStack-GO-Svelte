package persistence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/janlauber/one-click/pkg/workflow"
)

// WorkflowPersistenceStore persists workflow transitions and snapshots.
type WorkflowPersistenceStore struct {
	kv DurableKVStore
}

// NewWorkflowPersistenceStore wraps a KV store for workflow persistence.
func NewWorkflowPersistenceStore(kv DurableKVStore) *WorkflowPersistenceStore {
	return &WorkflowPersistenceStore{kv: kv}
}

// SaveTransition persists a StepTransition.
func (s *WorkflowPersistenceStore) SaveTransition(ctx context.Context, tr workflow.StepTransition) error {
	if ok, reason := workflow.VerifyStepTransition(tr); !ok {
		return fmt.Errorf("workflow store: invalid transition hash: %s", reason)
	}
	b, err := json.Marshal(tr)
	if err != nil {
		return fmt.Errorf("workflow store: marshal: %w", err)
	}
	return s.kv.Put(ctx, "transition:"+tr.TransitionID, b)
}

// LoadTransition retrieves a transition by ID.
func (s *WorkflowPersistenceStore) LoadTransition(ctx context.Context, transitionID string) (workflow.StepTransition, bool, error) {
	b, ok, err := s.kv.Get(ctx, "transition:"+transitionID)
	if err != nil || !ok {
		return workflow.StepTransition{}, ok, err
	}
	var tr workflow.StepTransition
	if err := json.Unmarshal(b, &tr); err != nil {
		return workflow.StepTransition{}, false, err
	}
	return tr, true, nil
}

// AllTransitionsForExecution returns all transitions for an executionID.
func (s *WorkflowPersistenceStore) AllTransitionsForExecution(ctx context.Context, executionID string) ([]workflow.StepTransition, error) {
	all, err := s.kv.List(ctx, "transition:")
	if err != nil {
		return nil, err
	}
	var out []workflow.StepTransition
	for _, b := range all {
		var tr workflow.StepTransition
		if err := json.Unmarshal(b, &tr); err != nil {
			continue
		}
		if tr.ExecutionID == executionID {
			out = append(out, tr)
		}
	}
	return out, nil
}

// SaveWorkflowSnapshot persists a WorkflowSnapshot.
func (s *WorkflowPersistenceStore) SaveWorkflowSnapshot(ctx context.Context, snap workflow.WorkflowSnapshot) error {
	if ok, reason := workflow.VerifyWorkflowSnapshot(snap); !ok {
		return fmt.Errorf("workflow store: invalid snapshot hash: %s", reason)
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("workflow store: snapshot marshal: %w", err)
	}
	return s.kv.Put(ctx, "wfsnap:"+snap.SnapshotID, b)
}

// LoadWorkflowSnapshot retrieves a snapshot by ID.
func (s *WorkflowPersistenceStore) LoadWorkflowSnapshot(ctx context.Context, snapshotID string) (workflow.WorkflowSnapshot, bool, error) {
	b, ok, err := s.kv.Get(ctx, "wfsnap:"+snapshotID)
	if err != nil || !ok {
		return workflow.WorkflowSnapshot{}, ok, err
	}
	var snap workflow.WorkflowSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return workflow.WorkflowSnapshot{}, false, err
	}
	return snap, true, nil
}
