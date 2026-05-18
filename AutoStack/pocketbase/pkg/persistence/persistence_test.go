package persistence

import (
	"context"
	"testing"

	"github.com/janlauber/one-click/pkg/audit"
	"github.com/janlauber/one-click/pkg/executor"
	"github.com/janlauber/one-click/pkg/policy"
	"github.com/janlauber/one-click/pkg/workers"
	"github.com/janlauber/one-click/pkg/workflow"
)

var ctx = context.Background()

// ─── InMemoryKVStore ──────────────────────────────────────────────────────────

func TestKVStore_PutAndGet(t *testing.T) {
	kv := NewInMemoryKVStore()
	if err := kv.Put(ctx, "key-1", []byte("value")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	v, ok, err := kv.Get(ctx, "key-1")
	if err != nil || !ok {
		t.Fatalf("Get: err=%v ok=%v", err, ok)
	}
	if string(v) != "value" {
		t.Fatalf("expected 'value', got %q", v)
	}
}

func TestKVStore_DuplicatePut_Error(t *testing.T) {
	kv := NewInMemoryKVStore()
	kv.Put(ctx, "key-1", []byte("a"))
	if err := kv.Put(ctx, "key-1", []byte("b")); err == nil {
		t.Fatal("duplicate Put must error")
	}
}

func TestKVStore_GetNotFound(t *testing.T) {
	kv := NewInMemoryKVStore()
	_, ok, err := kv.Get(ctx, "missing")
	if err != nil || ok {
		t.Fatalf("expected not-found: err=%v ok=%v", err, ok)
	}
}

func TestKVStore_List_ByPrefix(t *testing.T) {
	kv := NewInMemoryKVStore()
	kv.Put(ctx, "task:t-1", []byte("a"))
	kv.Put(ctx, "task:t-2", []byte("b"))
	kv.Put(ctx, "checkpoint:cp-1", []byte("c"))

	results, err := kv.List(ctx, "task:")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2, got %d", len(results))
	}
}

func TestKVStore_DeepCopy(t *testing.T) {
	kv := NewInMemoryKVStore()
	original := []byte("hello")
	kv.Put(ctx, "k", original)
	original[0] = 'X'
	v, _, _ := kv.Get(ctx, "k")
	if v[0] == 'X' {
		t.Fatal("KV store must deep-copy on Put")
	}
}

// ─── ExecutionTaskStore ───────────────────────────────────────────────────────

func TestExecutionTaskStore_SaveAndLoad(t *testing.T) {
	store := NewExecutionTaskStore(NewInMemoryKVStore())
	task := executor.NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", executor.TaskTypeDeploy, 1, 10)
	if err := store.Save(ctx, task); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, ok, err := store.Load(ctx, "t-1")
	if err != nil || !ok {
		t.Fatalf("Load: err=%v ok=%v", err, ok)
	}
	if loaded.TaskID != "t-1" {
		t.Fatalf("TaskID mismatch: %s", loaded.TaskID)
	}
}

func TestExecutionTaskStore_DuplicateSave_Error(t *testing.T) {
	store := NewExecutionTaskStore(NewInMemoryKVStore())
	task := executor.NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", executor.TaskTypeDeploy, 1, 10)
	store.Save(ctx, task)
	if err := store.Save(ctx, task); err == nil {
		t.Fatal("duplicate save must error")
	}
}

func TestExecutionTaskStore_ListForExecution(t *testing.T) {
	store := NewExecutionTaskStore(NewInMemoryKVStore())
	for i, id := range []string{"t-1", "t-2", "t-3"} {
		execID := "exec-1"
		if i == 2 {
			execID = "exec-2"
		}
		store.Save(ctx, executor.NewExecutionTask(id, execID, "wf-1", "step", "tenant-1", executor.TaskTypeDeploy, 1, int64(i)))
	}
	tasks, err := store.ListForExecution(ctx, "exec-1")
	if err != nil {
		t.Fatalf("ListForExecution: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks for exec-1, got %d", len(tasks))
	}
}

func TestExecutionTaskStore_InvalidHash_Rejected(t *testing.T) {
	store := NewExecutionTaskStore(NewInMemoryKVStore())
	task := executor.NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", executor.TaskTypeDeploy, 1, 10)
	task.StepID = "tampered" // invalidate hash
	if err := store.Save(ctx, task); err == nil {
		t.Fatal("invalid hash must be rejected")
	}
}

func TestExecutionTaskStore_Checkpoint(t *testing.T) {
	store := NewExecutionTaskStore(NewInMemoryKVStore())
	tasks := []executor.ExecutionTask{
		executor.NewExecutionTask("t-1", "exec-1", "wf-1", "s1", "tenant-1", executor.TaskTypeDeploy, 1, 1),
	}
	cp := executor.BuildExecutionCheckpoint("cp-1", "exec-1", "tenant-1", 1, tasks)
	if err := store.SaveCheckpoint(ctx, cp); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	loaded, ok, err := store.LoadCheckpoint(ctx, "cp-1")
	if err != nil || !ok {
		t.Fatalf("LoadCheckpoint: err=%v ok=%v", err, ok)
	}
	if loaded.CheckpointID != "cp-1" {
		t.Fatalf("checkpoint ID mismatch")
	}
}

// ─── WorkerPersistenceStore ───────────────────────────────────────────────────

func TestWorkerPersistenceStore_SaveAndLoad(t *testing.T) {
	store := NewWorkerPersistenceStore(NewInMemoryKVStore())
	w := workers.NewWorker("w-1", "t1", []string{"deploy"}, "us-east-1")
	if err := store.SaveWorker(ctx, w); err != nil {
		t.Fatalf("SaveWorker: %v", err)
	}
	loaded, ok, err := store.LoadWorker(ctx, "w-1")
	if err != nil || !ok {
		t.Fatalf("LoadWorker: err=%v ok=%v", err, ok)
	}
	if loaded.WorkerID != "w-1" {
		t.Fatalf("WorkerID mismatch")
	}
}

func TestWorkerPersistenceStore_InvalidHash_Rejected(t *testing.T) {
	store := NewWorkerPersistenceStore(NewInMemoryKVStore())
	w := workers.NewWorker("w-1", "t1", nil, "")
	w.Region = "tampered"
	if err := store.SaveWorker(ctx, w); err == nil {
		t.Fatal("tampered worker must be rejected")
	}
}

func TestWorkerPersistenceStore_OwnershipRecord(t *testing.T) {
	store := NewWorkerPersistenceStore(NewInMemoryKVStore())
	r := workers.NewOwnershipRecord("rec-1", "task-1", "exec-1", "t1", "", "w-1", "initial")
	if err := store.SaveOwnershipRecord(ctx, r); err != nil {
		t.Fatalf("SaveOwnershipRecord: %v", err)
	}
	records, err := store.AllOwnershipRecords(ctx, "task-1")
	if err != nil {
		t.Fatalf("AllOwnershipRecords: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
}

func TestWorkerPersistenceStore_Heartbeat(t *testing.T) {
	store := NewWorkerPersistenceStore(NewInMemoryKVStore())
	hb := workers.NewWorkerHeartbeat("hb-1", "w-1", "t1", 2)
	if err := store.SaveHeartbeat(ctx, hb); err != nil {
		t.Fatalf("SaveHeartbeat: %v", err)
	}
}

// ─── GovernancePersistenceStore ───────────────────────────────────────────────

func TestGovernancePersistenceStore_Snapshot(t *testing.T) {
	store := NewGovernancePersistenceStore(NewInMemoryKVStore())
	rt := policy.NewPolicyRuntime("tenant-1")
	snap := rt.ExportSnapshot("snap-1")
	if err := store.SaveGovernanceSnapshot(ctx, snap); err != nil {
		t.Fatalf("SaveGovernanceSnapshot: %v", err)
	}
	loaded, ok, err := store.LoadGovernanceSnapshot(ctx, "snap-1")
	if err != nil || !ok {
		t.Fatalf("LoadGovernanceSnapshot: err=%v ok=%v", err, ok)
	}
	if loaded.SnapshotID != "snap-1" {
		t.Fatalf("snapshot ID mismatch")
	}
}

func TestGovernancePersistenceStore_Decision(t *testing.T) {
	store := NewGovernancePersistenceStore(NewInMemoryKVStore())
	p := policy.NewPolicy("pol-1", "tenant-1", "test", policy.PolicyScopeExecution)
	p = policy.WithActions(p, []string{"deploy"}, nil, nil)
	d := policy.EvaluatePolicy(p, "deploy", 0.9, "dec-1")
	if err := store.SaveDecision(ctx, d); err != nil {
		t.Fatalf("SaveDecision: %v", err)
	}
	decisions, err := store.AllDecisions(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("AllDecisions: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
}

// ─── WorkflowPersistenceStore ─────────────────────────────────────────────────

func TestWorkflowPersistenceStore_Transition(t *testing.T) {
	store := NewWorkflowPersistenceStore(NewInMemoryKVStore())
	tr := workflow.NewStepTransition("tr-1", "exec-1", "t1", "start", "end", workflow.EdgeTypeSuccess, 1, "")
	if err := store.SaveTransition(ctx, tr); err != nil {
		t.Fatalf("SaveTransition: %v", err)
	}
	loaded, ok, err := store.LoadTransition(ctx, "tr-1")
	if err != nil || !ok {
		t.Fatalf("LoadTransition: err=%v ok=%v", err, ok)
	}
	if loaded.TransitionID != "tr-1" {
		t.Fatalf("transition ID mismatch")
	}
}

func TestWorkflowPersistenceStore_AllTransitionsForExecution(t *testing.T) {
	store := NewWorkflowPersistenceStore(NewInMemoryKVStore())
	store.SaveTransition(ctx, workflow.NewStepTransition("tr-1", "exec-1", "t1", "a", "b", workflow.EdgeTypeSuccess, 1, ""))
	store.SaveTransition(ctx, workflow.NewStepTransition("tr-2", "exec-1", "t1", "b", "c", workflow.EdgeTypeSuccess, 1, ""))
	store.SaveTransition(ctx, workflow.NewStepTransition("tr-3", "exec-2", "t1", "a", "b", workflow.EdgeTypeSuccess, 1, ""))

	transitions, err := store.AllTransitionsForExecution(ctx, "exec-1")
	if err != nil {
		t.Fatalf("AllTransitionsForExecution: %v", err)
	}
	if len(transitions) != 2 {
		t.Fatalf("expected 2 transitions, got %d", len(transitions))
	}
}

// ─── SnapshotPersistenceStore ────────────────────────────────────────────────

func TestSnapshotPersistenceStore_PlatformSnapshot(t *testing.T) {
	store := NewSnapshotPersistenceStore(NewInMemoryKVStore())
	snap := audit.BuildPlatformRuntimeSnapshot("snap-1", "t1", 1, 1, 2, 0, 0, 3, true, nil, nil, nil, "healthy")
	if err := store.SavePlatformSnapshot(ctx, snap); err != nil {
		t.Fatalf("SavePlatformSnapshot: %v", err)
	}
	loaded, ok, err := store.LoadPlatformSnapshot(ctx, "snap-1")
	if err != nil || !ok {
		t.Fatalf("LoadPlatformSnapshot: err=%v ok=%v", err, ok)
	}
	if loaded.SnapshotID != "snap-1" {
		t.Fatalf("snapshot ID mismatch")
	}
}

func TestSnapshotPersistenceStore_ReplayCertification(t *testing.T) {
	store := NewSnapshotPersistenceStore(NewInMemoryKVStore())
	cert := audit.CertifyReplay("cert-1", "exec-1", "t1", true, true, true, true, 50, 0)
	if err := store.SaveReplayCertification(ctx, cert); err != nil {
		t.Fatalf("SaveReplayCertification: %v", err)
	}
	certs, err := store.AllCertificationsForExecution(ctx, "exec-1")
	if err != nil {
		t.Fatalf("AllCertificationsForExecution: %v", err)
	}
	if len(certs) != 1 {
		t.Fatalf("expected 1 cert, got %d", len(certs))
	}
}

// ─── RecoverPersistentRuntime ────────────────────────────────────────────────

func TestRecoverPersistentRuntime_Basic(t *testing.T) {
	kv := NewInMemoryKVStore()
	taskStore := NewExecutionTaskStore(kv)
	snapStore := NewSnapshotPersistenceStore(NewInMemoryKVStore())

	tasks := []executor.ExecutionTask{
		executor.NewExecutionTask("t-1", "exec-1", "wf-1", "s1", "tenant-1", executor.TaskTypeDeploy, 1, 5),
		executor.NewExecutionTask("t-2", "exec-1", "wf-1", "s2", "tenant-1", executor.TaskTypeVerify, 1, 10),
	}
	for _, t := range tasks {
		taskStore.Save(ctx, t)
	}
	cp := executor.BuildExecutionCheckpoint("cp-1", "exec-1", "tenant-1", 10, tasks)
	taskStore.SaveCheckpoint(ctx, cp)

	cert := audit.CertifyReplay("cert-1", "exec-1", "tenant-1", true, true, true, true, 2, 0)
	snapStore.SaveReplayCertification(ctx, cert)

	state, report := RecoverPersistentRuntime(ctx, "exec-1", "tenant-1", "cp-1", taskStore, snapStore)

	if report.TasksRecovered != 2 {
		t.Fatalf("expected 2 tasks recovered, got %d", report.TasksRecovered)
	}
	if !report.CheckpointRestored {
		t.Fatal("checkpoint must be restored")
	}
	if !report.ClockContinuous {
		t.Fatal("clock must be continuous")
	}
	if report.CertificationsLoaded != 1 {
		t.Fatalf("expected 1 certification, got %d", report.CertificationsLoaded)
	}
	if len(state.Tasks) != 2 {
		t.Fatalf("state must have 2 tasks, got %d", len(state.Tasks))
	}
	if state.LastKnownClock != 10 {
		t.Fatalf("expected last clock 10, got %d", state.LastKnownClock)
	}
}

func TestRecoverPersistentRuntime_NoCheckpoint(t *testing.T) {
	taskStore := NewExecutionTaskStore(NewInMemoryKVStore())
	task := executor.NewExecutionTask("t-1", "exec-1", "wf-1", "s1", "t1", executor.TaskTypeDeploy, 1, 1)
	taskStore.Save(ctx, task)

	_, report := RecoverPersistentRuntime(ctx, "exec-1", "t1", "", taskStore, nil)
	if report.TasksRecovered != 1 {
		t.Fatalf("expected 1 task, got %d", report.TasksRecovered)
	}
	if !report.ClockContinuous {
		t.Fatal("no checkpoint means clock is trivially continuous")
	}
}
