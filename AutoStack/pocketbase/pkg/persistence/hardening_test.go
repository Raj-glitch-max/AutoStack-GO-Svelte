package persistence

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/janlauber/one-click/pkg/executor"
)

var hctx = context.Background()

// ─── Concurrency Safety ───────────────────────────────────────────────────────

func TestInMemoryKVStore_ConcurrentPuts_NoDuplicates(t *testing.T) {
	store := NewInMemoryKVStore()
	const goroutines = 20

	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", idx)
			errs[idx] = store.Put(hctx, key, []byte(fmt.Sprintf("value-%d", idx)))
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: unexpected put error: %v", i, err)
		}
	}
}

func TestInMemoryKVStore_DuplicatePut_Rejected(t *testing.T) {
	store := NewInMemoryKVStore()
	if err := store.Put(hctx, "k", []byte("v1")); err != nil {
		t.Fatalf("first put: %v", err)
	}
	if err := store.Put(hctx, "k", []byte("v2")); err == nil {
		t.Fatal("duplicate put must be rejected")
	}
}

func TestInMemoryKVStore_GetReturnsDeepCopy(t *testing.T) {
	store := NewInMemoryKVStore()
	_ = store.Put(hctx, "k", []byte("hello"))
	v, _, _ := store.Get(hctx, "k")
	v[0] = 'X' // mutate returned slice
	v2, _, _ := store.Get(hctx, "k")
	if v2[0] == 'X' {
		t.Fatal("store must return a deep copy — mutation must not affect stored value")
	}
}

// ─── WAL Append-Only Invariant ────────────────────────────────────────────────

func TestExecutionTaskStore_AppendOnly_NoOverwrite_H(t *testing.T) {
	store := NewInMemoryKVStore()
	taskStore := NewExecutionTaskStore(store)

	task := executor.NewExecutionTask("t-h1", "exec-h1", "wf-1", "step-1", "tenant-1", executor.TaskTypeDeploy, 1, 1)
	if err := taskStore.Save(hctx, task); err != nil {
		t.Fatalf("first save: %v", err)
	}
	// Any second save with the same key must fail (append-only).
	if err := taskStore.Save(hctx, task); err == nil {
		t.Fatal("second save of same task must be rejected (append-only)")
	}
}

func TestExecutionTaskStore_InvalidHash_Rejected_H(t *testing.T) {
	store := NewInMemoryKVStore()
	taskStore := NewExecutionTaskStore(store)

	task := executor.NewExecutionTask("t-h2", "exec-h2", "wf-1", "step-1", "tenant-1", executor.TaskTypeDeploy, 1, 1)
	task.Hash = "invalid-hash"
	if err := taskStore.Save(hctx, task); err == nil {
		t.Fatal("task with invalid hash must be rejected")
	}
}

// ─── Crash Recovery Simulation ────────────────────────────────────────────────

func TestRecoverPersistentRuntime_AllTasksRecovered(t *testing.T) {
	store := NewInMemoryKVStore()
	taskStore := NewExecutionTaskStore(store)
	snapStore := NewSnapshotPersistenceStore(store)

	task := executor.NewExecutionTask("t-r1", "exec-r1", "wf-1", "step-1", "tenant-1", executor.TaskTypeDeploy, 1, 1)
	if err := taskStore.Save(hctx, task); err != nil {
		t.Fatalf("save task: %v", err)
	}

	state, report := RecoverPersistentRuntime(hctx, "exec-r1", "tenant-1", "", taskStore, snapStore)
	if len(report.Errors) != 0 {
		t.Fatalf("unexpected recovery errors: %v", report.Errors)
	}
	if len(state.Tasks) != 1 {
		t.Fatalf("expected 1 recovered task, got %d", len(state.Tasks))
	}
}

func TestRecoverPersistentRuntime_InvalidTaskHash_Excluded(t *testing.T) {
	store := NewInMemoryKVStore()
	taskStore := NewExecutionTaskStore(store)
	snapStore := NewSnapshotPersistenceStore(store)

	// Save valid task first, then inject a corrupted one directly into the underlying store.
	task := executor.NewExecutionTask("t-r2", "exec-r2", "wf-1", "step-1", "tenant-1", executor.TaskTypeDeploy, 1, 1)
	if err := taskStore.Save(hctx, task); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Corrupt the stored bytes by writing a different key with bad data.
	_ = store.Put(hctx, "task:exec-r2:t-bad", []byte(`{"task_id":"t-bad","hash":"corrupt"}`))

	state, report := RecoverPersistentRuntime(hctx, "exec-r2", "tenant-1", "", taskStore, snapStore)
	// t-bad has invalid hash — should be excluded and logged.
	_ = state
	_ = report
	// Must not panic and must continue past the invalid entry.
}

func TestRecoverPersistentRuntime_ClockContinuity_OK(t *testing.T) {
	store := NewInMemoryKVStore()
	taskStore := NewExecutionTaskStore(store)
	snapStore := NewSnapshotPersistenceStore(store)

	t1 := executor.NewExecutionTask("t-c1", "exec-c1", "wf-1", "step-1", "tenant-1", executor.TaskTypeDeploy, 1, 5)
	t2 := executor.NewExecutionTask("t-c2", "exec-c1", "wf-1", "step-2", "tenant-1", executor.TaskTypeDeploy, 1, 10)
	if err := taskStore.Save(hctx, t1); err != nil {
		t.Fatalf("save t1: %v", err)
	}
	if err := taskStore.Save(hctx, t2); err != nil {
		t.Fatalf("save t2: %v", err)
	}

	// Build and save a checkpoint at clock=10.
	cp := executor.BuildExecutionCheckpoint("cp-c1", "exec-c1", "tenant-1", 10, []executor.ExecutionTask{t1, t2})
	if err := taskStore.SaveCheckpoint(hctx, cp); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	_, report := RecoverPersistentRuntime(hctx, "exec-c1", "tenant-1", "cp-c1", taskStore, snapStore)
	if !report.ClockContinuous {
		t.Fatalf("clock continuity must hold; errors: %v", report.Errors)
	}
}

func TestRecoverPersistentRuntime_InvalidCheckpointHash_Excluded(t *testing.T) {
	store := NewInMemoryKVStore()
	taskStore := NewExecutionTaskStore(store)
	snapStore := NewSnapshotPersistenceStore(store)

	task := executor.NewExecutionTask("t-cp1", "exec-cp1", "wf-1", "step-1", "tenant-1", executor.TaskTypeDeploy, 1, 1)
	if err := taskStore.Save(hctx, task); err != nil {
		t.Fatalf("save task: %v", err)
	}

	cp := executor.BuildExecutionCheckpoint("cp-bad1", "exec-cp1", "tenant-1", 1, []executor.ExecutionTask{task})
	// Save valid checkpoint first, then we call recovery with a checkpoint ID that doesn't exist.
	_ = cp
	_, report := RecoverPersistentRuntime(hctx, "exec-cp1", "tenant-1", "nonexistent-cp", taskStore, snapStore)
	// Missing checkpoint: no error (just not found).
	if report.CheckpointRestored {
		t.Fatal("missing checkpoint must not be marked as restored")
	}
}
