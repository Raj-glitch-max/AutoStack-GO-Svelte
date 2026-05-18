package executor

import (
	"testing"
)

// ─── ExecutionTask ────────────────────────────────────────────────────────────

func TestNewExecutionTask_HashValid(t *testing.T) {
	task := NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", TaskTypeDeploy, 1, 10)
	if task.State != TaskStatePending {
		t.Fatalf("expected pending, got %s", task.State)
	}
	ok, reason := VerifyTaskHash(task)
	if !ok {
		t.Fatalf("hash must be valid: %s", reason)
	}
}

func TestVerifyTaskHash_Tampered(t *testing.T) {
	task := NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", TaskTypeDeploy, 1, 10)
	task.StepID = "tampered"
	ok, _ := VerifyTaskHash(task)
	if ok {
		t.Fatal("tampered task must not verify")
	}
}

func TestIsTerminalTask(t *testing.T) {
	terminal := []TaskState{TaskStateCompleted, TaskStateFailed, TaskStateContradicted, TaskStateQuarantined, TaskStateRolledBack}
	for _, s := range terminal {
		task := ExecutionTask{State: s}
		if !IsTerminalTask(task) {
			t.Fatalf("state %s must be terminal", s)
		}
	}
	nonTerminal := []TaskState{TaskStatePending, TaskStateQueued, TaskStateDispatched, TaskStateRunning}
	for _, s := range nonTerminal {
		task := ExecutionTask{State: s}
		if IsTerminalTask(task) {
			t.Fatalf("state %s must not be terminal", s)
		}
	}
}

// ─── Lifecycle ───────────────────────────────────────────────────────────────

func TestValidateTaskTransition_AllowedPath(t *testing.T) {
	if err := ValidateTaskTransition(TaskStatePending, TaskStateQueued); err != nil {
		t.Fatalf("pending→queued must be allowed: %v", err)
	}
}

func TestValidateTaskTransition_Forbidden(t *testing.T) {
	if err := ValidateTaskTransition(TaskStateCompleted, TaskStateRunning); err == nil {
		t.Fatal("completed→running must be forbidden")
	}
}

func TestTransitionTask_ValueSemantics(t *testing.T) {
	task := NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", TaskTypeDeploy, 1, 10)
	newTask, err := TransitionTask(task, TaskStateQueued)
	if err != nil {
		t.Fatalf("TransitionTask: %v", err)
	}
	if task.State != TaskStatePending {
		t.Fatal("original task must not be mutated")
	}
	if newTask.State != TaskStateQueued {
		t.Fatalf("expected queued, got %s", newTask.State)
	}
	ok, reason := VerifyTaskHash(newTask)
	if !ok {
		t.Fatalf("transitioned task hash invalid: %s", reason)
	}
}

func TestMarkTaskStarted_SetsWorker(t *testing.T) {
	task := NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", TaskTypeDeploy, 1, 10)
	task, _ = TransitionTask(task, TaskStateQueued)
	task, _ = TransitionTask(task, TaskStateDispatched)
	started, err := MarkTaskStarted(task, "worker-1")
	if err != nil {
		t.Fatalf("MarkTaskStarted: %v", err)
	}
	if started.AssignedWorker != "worker-1" {
		t.Fatalf("expected worker-1, got %s", started.AssignedWorker)
	}
	if started.StartedAt == "" {
		t.Fatal("StartedAt must be set")
	}
}

func TestMarkTaskCompleted_StampsTime(t *testing.T) {
	task := NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", TaskTypeDeploy, 1, 10)
	task, _ = TransitionTask(task, TaskStateQueued)
	task, _ = TransitionTask(task, TaskStateDispatched)
	task, _ = MarkTaskStarted(task, "worker-1")
	done, err := MarkTaskCompleted(task)
	if err != nil {
		t.Fatalf("MarkTaskCompleted: %v", err)
	}
	if done.State != TaskStateCompleted {
		t.Fatalf("expected completed, got %s", done.State)
	}
	if done.CompletedAt == "" {
		t.Fatal("CompletedAt must be set")
	}
}

// ─── DispatchTask ─────────────────────────────────────────────────────────────

func TestDispatchTask_Deterministic(t *testing.T) {
	task := NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", TaskTypeDeploy, 1, 10)
	task, _ = TransitionTask(task, TaskStateQueued)

	workers := []string{"worker-a", "worker-b", "worker-c"}
	_, r1, _ := DispatchTask(task, "d-1", workers)
	_, r2, _ := DispatchTask(task, "d-2", workers)
	if r1.WorkerID != r2.WorkerID {
		t.Fatalf("dispatch must be deterministic: got %s vs %s", r1.WorkerID, r2.WorkerID)
	}
}

func TestDispatchTask_HashValid(t *testing.T) {
	task := NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", TaskTypeDeploy, 1, 10)
	task, _ = TransitionTask(task, TaskStateQueued)
	_, rec, err := DispatchTask(task, "d-1", []string{"worker-1"})
	if err != nil {
		t.Fatalf("DispatchTask: %v", err)
	}
	ok, reason := VerifyDispatchRecord(rec)
	if !ok {
		t.Fatalf("dispatch record hash invalid: %s", reason)
	}
}

func TestDispatchTask_NoWorkers_Error(t *testing.T) {
	task := NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", TaskTypeDeploy, 1, 10)
	task, _ = TransitionTask(task, TaskStateQueued)
	_, _, err := DispatchTask(task, "d-1", nil)
	if err == nil {
		t.Fatal("no workers must error")
	}
}

func TestDispatchTask_WrongState_Error(t *testing.T) {
	task := NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", TaskTypeDeploy, 1, 10)
	_, _, err := DispatchTask(task, "d-1", []string{"worker-1"})
	if err == nil {
		t.Fatal("non-queued task must error on dispatch")
	}
}

// ─── RetryRecord ─────────────────────────────────────────────────────────────

func TestNewRetryRecord_HashValid(t *testing.T) {
	r := NewRetryRecord("retry-1", "t-1", "exec-1", "tenant-1", "transient error", 2, DefaultRetryPolicy)
	ok, reason := VerifyRetryRecord(r)
	if !ok {
		t.Fatalf("retry record hash invalid: %s", reason)
	}
}

func TestShouldRetry_WithinLimit(t *testing.T) {
	p := RetryPolicy{MaxAttempts: 3}
	if !ShouldRetry(p, 2) {
		t.Fatal("attempt 2 of 3 must be retryable")
	}
	if ShouldRetry(p, 3) {
		t.Fatal("attempt 3 of 3 must not be retryable")
	}
}

func TestComputeRetryBackoff_Capped(t *testing.T) {
	p := RetryPolicy{BackoffBaseSec: 10, BackoffMaxSec: 25}
	if b := ComputeRetryBackoff(p, 1); b != 10 {
		t.Fatalf("attempt 1 backoff: expected 10, got %d", b)
	}
	if b := ComputeRetryBackoff(p, 3); b != 25 {
		t.Fatalf("attempt 3 backoff should be capped at 25, got %d", b)
	}
}

// ─── ExecutionCheckpoint ──────────────────────────────────────────────────────

func TestBuildExecutionCheckpoint_HashValid(t *testing.T) {
	tasks := []ExecutionTask{
		NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", TaskTypeDeploy, 1, 1),
		NewExecutionTask("t-2", "exec-1", "wf-1", "step-2", "tenant-1", TaskTypeVerify, 1, 2),
	}
	cp := BuildExecutionCheckpoint("cp-1", "exec-1", "tenant-1", 2, tasks)
	ok, reason := VerifyExecutionCheckpoint(cp)
	if !ok {
		t.Fatalf("checkpoint hash invalid: %s", reason)
	}
}

func TestBuildExecutionCheckpoint_CountsActive(t *testing.T) {
	tasks := []ExecutionTask{
		NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", TaskTypeDeploy, 1, 1),
	}
	done := tasks[0]
	done.State = TaskStateCompleted
	done.Hash = computeTaskHash(done)

	cp := BuildExecutionCheckpoint("cp-1", "exec-1", "tenant-1", 1, []ExecutionTask{tasks[0], done})
	if cp.ActiveCount != 1 {
		t.Fatalf("expected active count 1, got %d", cp.ActiveCount)
	}
}

func TestBuildExecutionCheckpoint_CreatedAtExcluded(t *testing.T) {
	tasks := []ExecutionTask{
		NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", TaskTypeDeploy, 1, 1),
	}
	cp1 := BuildExecutionCheckpoint("cp-1", "exec-1", "tenant-1", 1, tasks)
	cp2 := cp1
	cp2.CreatedAt = "2099-01-01T00:00:00Z"
	if cp1.CheckpointHash != cp2.CheckpointHash {
		t.Fatal("CreatedAt must not affect checkpoint hash")
	}
}

// ─── RecoverExecutionRuntime ──────────────────────────────────────────────────

func TestRecoverExecutionRuntime_RecoveredAndTerminal(t *testing.T) {
	running := NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", TaskTypeDeploy, 1, 1)
	running, _ = TransitionTask(running, TaskStateQueued)
	running, _ = TransitionTask(running, TaskStateDispatched)
	running, _ = MarkTaskStarted(running, "worker-live")

	done := NewExecutionTask("t-2", "exec-1", "wf-1", "step-2", "tenant-1", TaskTypeVerify, 1, 2)
	done, _ = MarkTaskCompleted(ExecutionTask{
		TaskID: done.TaskID, ExecutionID: done.ExecutionID, TenantID: done.TenantID,
		State: TaskStateRunning, Hash: computeTaskHash(ExecutionTask{State: TaskStateRunning}),
	})

	cp := BuildExecutionCheckpoint("cp-1", "exec-1", "tenant-1", 3, []ExecutionTask{running, done})

	liveWorkers := map[string]struct{}{"worker-live": {}}
	result, err := RecoverExecutionRuntime(cp, liveWorkers)
	if err != nil {
		t.Fatalf("recovery error: %v", err)
	}
	if len(result.Recovered) != 1 {
		t.Fatalf("expected 1 recovered, got %d", len(result.Recovered))
	}
	if len(result.Abandoned) != 0 {
		t.Fatalf("expected 0 abandoned, got %d", len(result.Abandoned))
	}
}

func TestRecoverExecutionRuntime_AbandonedTask(t *testing.T) {
	task := NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", TaskTypeDeploy, 1, 1)
	task, _ = TransitionTask(task, TaskStateQueued)
	task, _ = TransitionTask(task, TaskStateDispatched)
	task, _ = MarkTaskStarted(task, "worker-dead")

	cp := BuildExecutionCheckpoint("cp-1", "exec-1", "tenant-1", 1, []ExecutionTask{task})
	liveWorkers := map[string]struct{}{}
	result, err := RecoverExecutionRuntime(cp, liveWorkers)
	if err != nil {
		t.Fatalf("recovery error: %v", err)
	}
	if len(result.Abandoned) != 1 {
		t.Fatalf("expected 1 abandoned task, got %d", len(result.Abandoned))
	}
}

func TestRecoverExecutionRuntime_TamperedCheckpoint_Error(t *testing.T) {
	tasks := []ExecutionTask{NewExecutionTask("t-1", "exec-1", "wf-1", "s-1", "tenant-1", TaskTypeDeploy, 1, 1)}
	cp := BuildExecutionCheckpoint("cp-1", "exec-1", "tenant-1", 1, tasks)
	cp.ActiveCount = 99 // tamper
	_, err := RecoverExecutionRuntime(cp, nil)
	if err == nil {
		t.Fatal("tampered checkpoint must error")
	}
}

// ─── TaskSnapshot ─────────────────────────────────────────────────────────────

func TestBuildTaskSnapshot_HashValid(t *testing.T) {
	task := NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", TaskTypeDeploy, 1, 5)
	snap := BuildTaskSnapshot("snap-1", task)
	ok, reason := VerifyTaskSnapshot(snap)
	if !ok {
		t.Fatalf("task snapshot hash invalid: %s", reason)
	}
}

func TestBuildTaskSnapshot_SnapshotAtExcluded(t *testing.T) {
	task := NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", TaskTypeDeploy, 1, 5)
	snap1 := BuildTaskSnapshot("snap-1", task)
	snap2 := snap1
	snap2.SnapshotAt = "2099-01-01T00:00:00Z"
	if snap1.SnapshotHash != snap2.SnapshotHash {
		t.Fatal("SnapshotAt must not affect snapshot hash")
	}
}
