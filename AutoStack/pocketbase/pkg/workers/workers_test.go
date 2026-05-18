package workers

import (
	"testing"
)

// ─── Worker ───────────────────────────────────────────────────────────────────

func TestNewWorker_HashValid(t *testing.T) {
	w := NewWorker("w-1", "tenant-1", []string{"deploy", "verify"}, "us-east-1")
	if w.State != WorkerStateOnline {
		t.Fatalf("initial state must be online, got %s", w.State)
	}
	ok, reason := VerifyWorkerHash(w)
	if !ok {
		t.Fatalf("worker hash invalid: %s", reason)
	}
}

func TestVerifyWorkerHash_Tampered(t *testing.T) {
	w := NewWorker("w-1", "tenant-1", []string{"deploy"}, "us-east-1")
	w.Region = "tampered"
	ok, _ := VerifyWorkerHash(w)
	if ok {
		t.Fatal("tampered worker must not verify")
	}
}

func TestTransitionWorker_ValueSemantics(t *testing.T) {
	w := NewWorker("w-1", "tenant-1", nil, "")
	updated := TransitionWorker(w, WorkerStateDegraded)
	if w.State != WorkerStateOnline {
		t.Fatal("original must not be mutated")
	}
	if updated.State != WorkerStateDegraded {
		t.Fatalf("expected degraded, got %s", updated.State)
	}
	ok, reason := VerifyWorkerHash(updated)
	if !ok {
		t.Fatalf("transitioned worker hash invalid: %s", reason)
	}
}

func TestIsAvailable(t *testing.T) {
	online := NewWorker("w-1", "t1", nil, "")
	if !IsAvailable(online) {
		t.Fatal("online worker must be available")
	}
	degraded := TransitionWorker(online, WorkerStateDegraded)
	if IsAvailable(degraded) {
		t.Fatal("degraded worker must not be available")
	}
}

// ─── WorkerRegistry ───────────────────────────────────────────────────────────

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewWorkerRegistry()
	w := NewWorker("w-1", "tenant-1", nil, "")
	if err := reg.Register(w); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := reg.Get("w-1")
	if !ok {
		t.Fatal("worker must be retrievable")
	}
	if got.WorkerID != "w-1" {
		t.Fatalf("expected w-1, got %s", got.WorkerID)
	}
}

func TestRegistry_DuplicateRegister_Error(t *testing.T) {
	reg := NewWorkerRegistry()
	w := NewWorker("w-1", "tenant-1", nil, "")
	reg.Register(w)
	if err := reg.Register(w); err == nil {
		t.Fatal("duplicate register must error")
	}
}

func TestRegistry_UpdateState(t *testing.T) {
	reg := NewWorkerRegistry()
	w := NewWorker("w-1", "tenant-1", nil, "")
	reg.Register(w)
	if err := reg.UpdateState("w-1", WorkerStateDraining); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	got, _ := reg.Get("w-1")
	if got.State != WorkerStateDraining {
		t.Fatalf("expected draining, got %s", got.State)
	}
}

func TestRegistry_LiveWorkerIDs(t *testing.T) {
	reg := NewWorkerRegistry()
	reg.Register(NewWorker("w-1", "t1", nil, ""))
	reg.Register(NewWorker("w-2", "t1", nil, ""))
	reg.UpdateState("w-2", WorkerStateOffline)

	live := reg.LiveWorkerIDs()
	if _, ok := live["w-1"]; !ok {
		t.Fatal("w-1 must be live")
	}
	if _, ok := live["w-2"]; ok {
		t.Fatal("offline w-2 must not be live")
	}
}

// ─── Capabilities ─────────────────────────────────────────────────────────────

func TestHasCapability(t *testing.T) {
	w := NewWorker("w-1", "t1", []string{"deploy", "verify"}, "")
	if !HasCapability(w, "deploy") {
		t.Fatal("worker must have deploy capability")
	}
	if HasCapability(w, "scale") {
		t.Fatal("worker must not have scale capability")
	}
}

func TestCapabilitiesMatch_AllRequired(t *testing.T) {
	w := NewWorker("w-1", "t1", []string{"deploy", "verify", "scale"}, "")
	if !CapabilitiesMatch(w, []string{"deploy", "scale"}) {
		t.Fatal("worker with superset must match")
	}
	if CapabilitiesMatch(w, []string{"rollback"}) {
		t.Fatal("worker missing capability must not match")
	}
}

func TestCapabilitiesMatch_EmptyRequired(t *testing.T) {
	w := NewWorker("w-1", "t1", nil, "")
	if !CapabilitiesMatch(w, nil) {
		t.Fatal("empty requirements must always match")
	}
}

// ─── AssignWorker ─────────────────────────────────────────────────────────────

func TestAssignWorker_Deterministic(t *testing.T) {
	workers := []Worker{
		NewWorker("w-1", "t1", []string{"deploy"}, ""),
		NewWorker("w-2", "t1", []string{"deploy"}, ""),
		NewWorker("w-3", "t1", []string{"deploy"}, ""),
	}
	a1, err := AssignWorker("a-1", "task-1", "exec-1", "t1", 1, workers, []string{"deploy"})
	if err != nil {
		t.Fatalf("AssignWorker: %v", err)
	}
	a2, _ := AssignWorker("a-2", "task-1", "exec-1", "t1", 1, workers, []string{"deploy"})
	if a1.WorkerID != a2.WorkerID {
		t.Fatalf("assignment must be deterministic: got %s vs %s", a1.WorkerID, a2.WorkerID)
	}
}

func TestAssignWorker_HashValid(t *testing.T) {
	w := []Worker{NewWorker("w-1", "t1", []string{"deploy"}, "")}
	a, err := AssignWorker("a-1", "task-1", "exec-1", "t1", 1, w, nil)
	if err != nil {
		t.Fatalf("AssignWorker: %v", err)
	}
	ok, reason := VerifyWorkerAssignment(a)
	if !ok {
		t.Fatalf("assignment hash invalid: %s", reason)
	}
}

func TestAssignWorker_NoEligible_Error(t *testing.T) {
	w := []Worker{NewWorker("w-1", "t1", []string{"verify"}, "")}
	_, err := AssignWorker("a-1", "task-1", "exec-1", "t1", 1, w, []string{"deploy"})
	if err == nil {
		t.Fatal("no eligible workers must error")
	}
}

func TestAssignWorker_SkipsDegradedWorkers(t *testing.T) {
	degraded := TransitionWorker(NewWorker("w-1", "t1", []string{"deploy"}, ""), WorkerStateDegraded)
	online := NewWorker("w-2", "t1", []string{"deploy"}, "")
	a, err := AssignWorker("a-1", "task-1", "exec-1", "t1", 1, []Worker{degraded, online}, nil)
	if err != nil {
		t.Fatalf("AssignWorker: %v", err)
	}
	if a.WorkerID != "w-2" {
		t.Fatalf("must select online worker, got %s", a.WorkerID)
	}
}

// ─── OwnershipHistory ─────────────────────────────────────────────────────────

func TestOwnershipHistory_AppendAndRetrieve(t *testing.T) {
	h := NewOwnershipHistory()
	r := NewOwnershipRecord("rec-1", "task-1", "exec-1", "t1", "", "w-1", "initial")
	if err := h.Append(r); err != nil {
		t.Fatalf("Append: %v", err)
	}
	recs := h.ForTask("task-1")
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if h.CurrentOwner("task-1") != "w-1" {
		t.Fatalf("expected w-1, got %s", h.CurrentOwner("task-1"))
	}
}

func TestOwnershipRecord_HashValid(t *testing.T) {
	r := NewOwnershipRecord("rec-1", "task-1", "exec-1", "t1", "w-1", "w-2", "worker failed")
	ok, reason := VerifyOwnershipRecord(r)
	if !ok {
		t.Fatalf("ownership record hash invalid: %s", reason)
	}
}

func TestOwnershipHistory_InvalidHash_Error(t *testing.T) {
	h := NewOwnershipHistory()
	r := NewOwnershipRecord("rec-1", "task-1", "exec-1", "t1", "", "w-1", "reason")
	r.Reason = "tampered"
	if err := h.Append(r); err == nil {
		t.Fatal("tampered record must be rejected")
	}
}

// ─── HeartbeatLog ─────────────────────────────────────────────────────────────

func TestHeartbeatLog_RecordAndLast(t *testing.T) {
	log := NewHeartbeatLog()
	hb := NewWorkerHeartbeat("hb-1", "w-1", "t1", 3)
	if err := log.Record(hb); err != nil {
		t.Fatalf("Record: %v", err)
	}
	last, ok := log.LastHeartbeat("w-1")
	if !ok {
		t.Fatal("heartbeat must be retrievable")
	}
	if last.ActiveTasks != 3 {
		t.Fatalf("expected 3, got %d", last.ActiveTasks)
	}
}

func TestHeartbeatLog_AppendOnly(t *testing.T) {
	log := NewHeartbeatLog()
	log.Record(NewWorkerHeartbeat("hb-1", "w-1", "t1", 2))
	log.Record(NewWorkerHeartbeat("hb-2", "w-1", "t1", 5))
	all := log.AllForWorker("w-1")
	if len(all) != 2 {
		t.Fatalf("expected 2 heartbeats, got %d", len(all))
	}
}

func TestHeartbeatHash_Valid(t *testing.T) {
	hb := NewWorkerHeartbeat("hb-1", "w-1", "t1", 0)
	ok, reason := VerifyHeartbeatHash(hb)
	if !ok {
		t.Fatalf("heartbeat hash invalid: %s", reason)
	}
}

func TestHeartbeatHash_ReceivedAtExcluded(t *testing.T) {
	hb1 := NewWorkerHeartbeat("hb-1", "w-1", "t1", 0)
	hb2 := hb1
	hb2.ReceivedAt = "2099-01-01T00:00:00Z"
	if hb1.HeartbeatHash != hb2.HeartbeatHash {
		t.Fatal("ReceivedAt must not affect heartbeat hash")
	}
}

// ─── WorkerSnapshot ───────────────────────────────────────────────────────────

func TestBuildWorkerSnapshot_HashValid(t *testing.T) {
	workers := []Worker{
		NewWorker("w-1", "t1", nil, ""),
		NewWorker("w-2", "t1", nil, ""),
	}
	snap := BuildWorkerSnapshot("snap-1", "t1", workers)
	ok, reason := VerifyWorkerSnapshot(snap)
	if !ok {
		t.Fatalf("worker snapshot hash invalid: %s", reason)
	}
	if snap.TotalCount != 2 {
		t.Fatalf("expected total 2, got %d", snap.TotalCount)
	}
	if snap.OnlineCount != 2 {
		t.Fatalf("expected online 2, got %d", snap.OnlineCount)
	}
}

func TestBuildWorkerSnapshot_SnapshotAtExcluded(t *testing.T) {
	snap1 := BuildWorkerSnapshot("snap-1", "t1", nil)
	snap2 := snap1
	snap2.SnapshotAt = "2099-01-01T00:00:00Z"
	if snap1.SnapshotHash != snap2.SnapshotHash {
		t.Fatal("SnapshotAt must not affect snapshot hash")
	}
}

// ─── RecoverWorkerAssignments ─────────────────────────────────────────────────

func TestRecoverWorkerAssignments_DeadWorker(t *testing.T) {
	history := NewOwnershipHistory()
	history.Append(NewOwnershipRecord("r-1", "task-1", "exec-1", "t1", "", "w-dead", "initial"))

	liveWorkers := map[string]struct{}{"w-live": {}}
	orphans := RecoverWorkerAssignments(history, liveWorkers, []string{"task-1"})
	if len(orphans) != 1 {
		t.Fatalf("expected 1 orphaned task, got %d", len(orphans))
	}
	if orphans[0].ToWorker != "w-dead" {
		t.Fatalf("expected w-dead, got %s", orphans[0].ToWorker)
	}
}

func TestRecoverWorkerAssignments_LiveWorker_NoOrphan(t *testing.T) {
	history := NewOwnershipHistory()
	history.Append(NewOwnershipRecord("r-1", "task-1", "exec-1", "t1", "", "w-live", "initial"))

	liveWorkers := map[string]struct{}{"w-live": {}}
	orphans := RecoverWorkerAssignments(history, liveWorkers, []string{"task-1"})
	if len(orphans) != 0 {
		t.Fatalf("no orphans expected for live worker, got %d", len(orphans))
	}
}

// ─── ComputeLoad ──────────────────────────────────────────────────────────────

func TestComputeLoad(t *testing.T) {
	w1 := NewWorker("w-1", "t1", nil, "")
	w2 := TransitionWorker(NewWorker("w-2", "t1", nil, ""), WorkerStateDegraded)
	hb := NewHeartbeatLog()
	hb.Record(NewWorkerHeartbeat("hb-1", "w-1", "t1", 4))

	report := ComputeLoad([]Worker{w1, w2}, hb)
	if report.TotalWorkers != 2 {
		t.Fatalf("expected 2 total, got %d", report.TotalWorkers)
	}
	if report.AvailableWorkers != 1 {
		t.Fatalf("expected 1 available, got %d", report.AvailableWorkers)
	}
	if report.TotalActiveTasks != 4 {
		t.Fatalf("expected 4 active tasks, got %d", report.TotalActiveTasks)
	}
}
