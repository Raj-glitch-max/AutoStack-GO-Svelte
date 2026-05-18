package scheduler

import (
	"testing"

	"github.com/janlauber/one-click/pkg/orchestrator"
	"github.com/janlauber/one-click/pkg/planner"
)

// ─── helpers ───────────────────────────────────────────────────────────────

func makeStage(id string, order int, deps []string, risk planner.RiskLevel, providers []string, blast int) orchestrator.ExecutionStage {
	nodes := make([]orchestrator.StageNode, len(providers))
	for i, p := range providers {
		nodes[i] = orchestrator.StageNode{NodeID: id + "-node-" + p, Provider: p}
	}
	return orchestrator.ExecutionStage{
		StageID:              id,
		StageOrder:           order,
		Dependencies:         deps,
		RiskLevel:            risk,
		Nodes:                nodes,
		EstimatedBlastRadius: blast,
	}
}

func makePlan(stages ...orchestrator.ExecutionStage) *orchestrator.ExecutionPlan {
	return &orchestrator.ExecutionPlan{
		ExecutionPlanHash: "test-hash",
		Stages:            stages,
	}
}

// ─── ordering ──────────────────────────────────────────────────────────────

func TestSortQueueEntries_DependencyDepthFirst(t *testing.T) {
	entries := []QueueEntry{
		{StageID: "b", DependencyDepth: 2, RiskLevel: planner.RiskLevelLow, StageOrder: 1},
		{StageID: "a", DependencyDepth: 0, RiskLevel: planner.RiskLevelLow, StageOrder: 0},
		{StageID: "c", DependencyDepth: 1, RiskLevel: planner.RiskLevelLow, StageOrder: 2},
	}
	SortQueueEntries(entries)
	if entries[0].StageID != "a" || entries[1].StageID != "c" || entries[2].StageID != "b" {
		t.Fatalf("expected a,c,b order by depth; got %s,%s,%s", entries[0].StageID, entries[1].StageID, entries[2].StageID)
	}
}

func TestSortQueueEntries_RiskLevelTieBreaker(t *testing.T) {
	entries := []QueueEntry{
		{StageID: "x", DependencyDepth: 0, RiskLevel: planner.RiskLevelCritical, StageOrder: 0},
		{StageID: "y", DependencyDepth: 0, RiskLevel: planner.RiskLevelLow, StageOrder: 1},
	}
	SortQueueEntries(entries)
	if entries[0].StageID != "y" {
		t.Fatalf("lower risk must sort first at same depth, got %s first", entries[0].StageID)
	}
}

func TestSortQueueEntries_StageIDLexicalFinal(t *testing.T) {
	entries := []QueueEntry{
		{StageID: "z", DependencyDepth: 0, RiskLevel: planner.RiskLevelLow, StageOrder: 0},
		{StageID: "a", DependencyDepth: 0, RiskLevel: planner.RiskLevelLow, StageOrder: 0},
	}
	SortQueueEntries(entries)
	if entries[0].StageID != "a" {
		t.Fatalf("lexical ordering must be final tiebreaker, got %s first", entries[0].StageID)
	}
}

func TestComputeDependencyDepths_LinearChain(t *testing.T) {
	stages := []orchestrator.ExecutionStage{
		{StageID: "s0", Dependencies: nil},
		{StageID: "s1", Dependencies: []string{"s0"}},
		{StageID: "s2", Dependencies: []string{"s1"}},
	}
	depths := ComputeDependencyDepths(stages)
	if depths["s0"] != 0 {
		t.Fatalf("root stage depth must be 0, got %d", depths["s0"])
	}
	if depths["s1"] != 1 {
		t.Fatalf("s1 depth must be 1, got %d", depths["s1"])
	}
	if depths["s2"] != 2 {
		t.Fatalf("s2 depth must be 2, got %d", depths["s2"])
	}
}

func TestComputeDependencyDepths_Deterministic(t *testing.T) {
	stages := []orchestrator.ExecutionStage{
		{StageID: "s0", Dependencies: nil},
		{StageID: "s1", Dependencies: []string{"s0"}},
	}
	d1 := ComputeDependencyDepths(stages)
	d2 := ComputeDependencyDepths(stages)
	if d1["s0"] != d2["s0"] || d1["s1"] != d2["s1"] {
		t.Fatal("same inputs must produce same depth map")
	}
}

// ─── stage queue ───────────────────────────────────────────────────────────

func TestBuildStageQueue_EmptyPlan(t *testing.T) {
	q := BuildStageQueue(makePlan())
	if !IsEmpty(q) {
		t.Fatal("empty plan must produce empty queue")
	}
}

func TestBuildStageQueue_NilPlan(t *testing.T) {
	q := BuildStageQueue(nil)
	if !IsEmpty(q) {
		t.Fatal("nil plan must produce empty queue")
	}
}

func TestBuildStageQueue_StageCount(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, planner.RiskLevelLow, []string{"aws-ecs"}, 1),
		makeStage("s1", 1, []string{"s0"}, planner.RiskLevelLow, []string{"aws-ecs"}, 1),
	)
	q := BuildStageQueue(plan)
	if QueueSize(q) != 2 {
		t.Fatalf("expected 2 entries, got %d", QueueSize(q))
	}
}

func TestBuildStageQueue_OrderDeterministic(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, planner.RiskLevelLow, []string{"aws-ecs"}, 1),
		makeStage("s1", 1, []string{"s0"}, planner.RiskLevelMedium, []string{"gcp-cloudrun"}, 2),
		makeStage("s2", 2, []string{"s1"}, planner.RiskLevelHigh, []string{"aws-ecs"}, 3),
	)
	q1 := BuildStageQueue(plan)
	q2 := BuildStageQueue(plan)
	for i := range q1.Entries {
		if q1.Entries[i].StageID != q2.Entries[i].StageID {
			t.Fatalf("queue ordering must be deterministic at index %d: %s vs %s",
				i, q1.Entries[i].StageID, q2.Entries[i].StageID)
		}
	}
}

func TestDequeue_ReturnsFrontEntry(t *testing.T) {
	plan := makePlan(makeStage("s0", 0, nil, planner.RiskLevelLow, nil, 0))
	q := BuildStageQueue(plan)
	entry, remaining, ok := Dequeue(q, map[string]bool{})
	if !ok {
		t.Fatal("Dequeue must return ok=true for a ready stage")
	}
	if entry.StageID != "s0" {
		t.Fatalf("expected s0, got %s", entry.StageID)
	}
	if !IsEmpty(remaining) {
		t.Fatal("remaining queue must be empty after sole entry dequeued")
	}
}

func TestDequeue_BlockedByDependency(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, planner.RiskLevelLow, nil, 0),
		makeStage("s1", 1, []string{"s0"}, planner.RiskLevelLow, nil, 0),
	)
	q := BuildStageQueue(plan)
	// s0 is satisfied, s1 depends on s0 and s0 is not yet complete.
	satisfied := map[string]bool{} // nothing completed yet
	// Remove s0 from queue to simulate it being dequeued.
	_, q2, _ := Dequeue(q, satisfied) // dequeues s0 (no deps)

	// Now try to dequeue s1 — but s0 hasn't been marked satisfied yet.
	_, _, ok := Dequeue(q2, satisfied)
	if ok {
		t.Fatal("s1 must not dequeue when its dependency s0 is not satisfied")
	}

	// Mark s0 satisfied.
	satisfied["s0"] = true
	_, _, ok = Dequeue(q2, satisfied)
	if !ok {
		t.Fatal("s1 must dequeue once s0 is satisfied")
	}
}

func TestContainsStage(t *testing.T) {
	plan := makePlan(makeStage("s0", 0, nil, planner.RiskLevelLow, nil, 0))
	q := BuildStageQueue(plan)
	if !ContainsStage(q, "s0") {
		t.Fatal("ContainsStage must return true for existing stage")
	}
	if ContainsStage(q, "nonexistent") {
		t.Fatal("ContainsStage must return false for absent stage")
	}
}

func TestReplayOrderFromQueue(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, planner.RiskLevelLow, nil, 0),
		makeStage("s1", 1, []string{"s0"}, planner.RiskLevelLow, nil, 0),
	)
	q := BuildStageQueue(plan)
	order := ReplayOrderFromQueue(q)
	if len(order) != 2 {
		t.Fatalf("expected 2 entries in replay order, got %d", len(order))
	}
}

// ─── concurrency ───────────────────────────────────────────────────────────

func TestBuildConcurrencyGroups_LinearChain_NoGroups(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, planner.RiskLevelLow, []string{"aws-ecs"}, 1),
		makeStage("s1", 1, []string{"s0"}, planner.RiskLevelLow, []string{"aws-ecs"}, 1),
	)
	q := BuildStageQueue(plan)
	groups := BuildConcurrencyGroups(q)
	// Linear chain — each stage at a different depth, no concurrent opportunities.
	if len(groups) != 0 {
		t.Fatalf("linear dependency chain must produce no concurrency groups, got %d", len(groups))
	}
}

func TestBuildConcurrencyGroups_SameDepthDifferentProviders(t *testing.T) {
	// Two root stages (no deps) with different providers — can be concurrent.
	plan := makePlan(
		makeStage("s0", 0, nil, planner.RiskLevelLow, []string{"aws-ecs"}, 1),
		makeStage("s1", 1, nil, planner.RiskLevelLow, []string{"gcp-cloudrun"}, 1),
	)
	q := BuildStageQueue(plan)
	groups := BuildConcurrencyGroups(q)
	if len(groups) == 0 {
		t.Fatal("two root stages with different providers must produce at least one concurrency group")
	}
	if len(groups[0].StageIDs) < 2 {
		t.Fatalf("concurrency group must contain both stages, got %d", len(groups[0].StageIDs))
	}
}

func TestBuildConcurrencyGroups_SameProvider_NoGroup(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, planner.RiskLevelLow, []string{"aws-ecs"}, 1),
		makeStage("s1", 1, nil, planner.RiskLevelLow, []string{"aws-ecs"}, 1),
	)
	q := BuildStageQueue(plan)
	groups := BuildConcurrencyGroups(q)
	for _, g := range groups {
		if len(g.StageIDs) > 1 {
			t.Fatal("stages sharing a provider must not be in the same concurrency group")
		}
	}
}

func TestBuildConcurrencyGroups_GroupIDNonEmpty(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, planner.RiskLevelLow, []string{"aws-ecs"}, 1),
		makeStage("s1", 1, nil, planner.RiskLevelLow, []string{"gcp-cloudrun"}, 1),
	)
	q := BuildStageQueue(plan)
	groups := BuildConcurrencyGroups(q)
	for _, g := range groups {
		if g.GroupID == "" {
			t.Fatal("GroupID must be non-empty")
		}
	}
}

// ─── fairness ──────────────────────────────────────────────────────────────

func TestRecordBlocked_AddsEntry(t *testing.T) {
	f := NewFairnessState("exec-1")
	f = RecordBlocked(f, "s0", BlockReasonApprovalPending)
	if BlockedCount(f) != 1 {
		t.Fatalf("expected 1 blocked stage, got %d", BlockedCount(f))
	}
}

func TestRecordBlocked_IncrementsDeferredCount(t *testing.T) {
	f := NewFairnessState("exec-1")
	f = RecordBlocked(f, "s0", BlockReasonApprovalPending)
	f = RecordBlocked(f, "s0", BlockReasonApprovalPending)
	if f.DeferredCounts["s0"] != 2 {
		t.Fatalf("expected DeferredCount=2, got %d", f.DeferredCounts["s0"])
	}
}

func TestClearBlocked_RemovesEntry(t *testing.T) {
	f := NewFairnessState("exec-1")
	f = RecordBlocked(f, "s0", BlockReasonApprovalPending)
	f = ClearBlocked(f, "s0")
	if BlockedCount(f) != 0 {
		t.Fatal("ClearBlocked must remove the stage from blocked list")
	}
}

func TestCheckStarvation_BelowThreshold(t *testing.T) {
	f := NewFairnessState("exec-1")
	for i := 0; i < MaxDeferredBeforeStarvation-1; i++ {
		f = RecordBlocked(f, "s0", BlockReasonApprovalPending)
	}
	starved, _ := CheckStarvation(f, "s0")
	if starved {
		t.Fatal("below threshold must not be starved")
	}
}

func TestCheckStarvation_AtThreshold(t *testing.T) {
	f := NewFairnessState("exec-1")
	for i := 0; i < MaxDeferredBeforeStarvation; i++ {
		f = RecordBlocked(f, "s0", BlockReasonApprovalPending)
	}
	starved, reason := CheckStarvation(f, "s0")
	if !starved {
		t.Fatal("at threshold must be starved")
	}
	if reason == "" {
		t.Fatal("starvation reason must be non-empty")
	}
}

func TestStarvedStages_Deterministic(t *testing.T) {
	f := NewFairnessState("exec-1")
	for i := 0; i < MaxDeferredBeforeStarvation; i++ {
		f = RecordBlocked(f, "s1", BlockReasonApprovalPending)
		f = RecordBlocked(f, "s0", BlockReasonApprovalPending)
	}
	starved := StarvedStages(f)
	if len(starved) != 2 {
		t.Fatalf("expected 2 starved stages, got %d", len(starved))
	}
	// Must be lexically ordered.
	if starved[0] != "s0" || starved[1] != "s1" {
		t.Fatalf("starved stages must be lexically ordered, got %v", starved)
	}
}

// ─── pause / resume ────────────────────────────────────────────────────────

func TestPauseExecutionSchedule_RequiresPausedBy(t *testing.T) {
	_, err := PauseExecutionSchedule("exec-1", "", "reason", StageQueue{}, nil, NewFairnessState("exec-1"), nil, nil)
	if err == nil {
		t.Fatal("empty pausedBy must return error")
	}
}

func TestPauseExecutionSchedule_RequiresPauseReason(t *testing.T) {
	_, err := PauseExecutionSchedule("exec-1", "operator", "", StageQueue{}, nil, NewFairnessState("exec-1"), nil, nil)
	if err == nil {
		t.Fatal("empty pauseReason must return error")
	}
}

func TestPauseResumeSchedule_PreservesQueueOrder(t *testing.T) {
	plan := makePlan(
		makeStage("s0", 0, nil, planner.RiskLevelLow, []string{"aws-ecs"}, 1),
		makeStage("s1", 1, []string{"s0"}, planner.RiskLevelLow, []string{"aws-ecs"}, 1),
	)
	q := BuildStageQueue(plan)
	f := NewFairnessState("exec-1")

	pause, err := PauseExecutionSchedule("exec-1", "operator-1", "gate approval", q, nil, f, nil, nil)
	if err != nil {
		t.Fatalf("PauseExecutionSchedule failed: %v", err)
	}

	resumed, _ := ResumeExecutionSchedule(pause)
	if QueueSize(resumed) != QueueSize(q) {
		t.Fatalf("resumed queue must have same size: want %d got %d", QueueSize(q), QueueSize(resumed))
	}
	for i := range q.Entries {
		if q.Entries[i].StageID != resumed.Entries[i].StageID {
			t.Fatalf("entry %d order changed after resume: want %s got %s",
				i, q.Entries[i].StageID, resumed.Entries[i].StageID)
		}
	}
}

func TestPauseRecord_PausedByPreserved(t *testing.T) {
	q := StageQueue{ExecutionID: "exec-1"}
	pause, _ := PauseExecutionSchedule("exec-1", "ops-team", "deployment freeze", q, nil, NewFairnessState("exec-1"), nil, nil)
	if pause.PausedBy != "ops-team" {
		t.Fatalf("PausedBy must be preserved, got %q", pause.PausedBy)
	}
	if pause.PauseReason != "deployment freeze" {
		t.Fatalf("PauseReason must be preserved, got %q", pause.PauseReason)
	}
}

// ─── replay ────────────────────────────────────────────────────────────────

func TestReplaySchedule_EmptyEvents(t *testing.T) {
	q := ReplaySchedule("exec-1", nil, StageQueue{})
	if !IsEmpty(q) {
		t.Fatal("empty events must produce empty queue")
	}
}

func TestReplaySchedule_EnqueuedNotDequeued(t *testing.T) {
	original := StageQueue{
		ExecutionID: "exec-1",
		Entries: []QueueEntry{
			{StageID: "s0"},
			{StageID: "s1"},
		},
	}
	events := []SchedulerReplayEvent{
		NewReplayEvent(1, "s0", "enqueued"),
		NewReplayEvent(2, "s1", "enqueued"),
		NewReplayEvent(3, "s0", "dequeued"), // s0 dequeued — should not appear in result
	}
	q := ReplaySchedule("exec-1", events, original)
	if ContainsStage(q, "s0") {
		t.Fatal("dequeued stage must not appear in replay result")
	}
	if !ContainsStage(q, "s1") {
		t.Fatal("enqueued-but-not-dequeued stage must appear in replay result")
	}
}

func TestReplayOrderFromEvents_AscendingSequence(t *testing.T) {
	events := []SchedulerReplayEvent{
		NewReplayEvent(2, "s1", "enqueued"),
		NewReplayEvent(1, "s0", "enqueued"),
		NewReplayEvent(3, "s0", "dequeued"),
	}
	order := ReplayOrderFromEvents(events)
	// Must be s0 first (sequence 1 < sequence 2).
	if len(order) != 2 || order[0] != "s0" || order[1] != "s1" {
		t.Fatalf("replay order must follow sequence: got %v", order)
	}
}

// ─── snapshot ──────────────────────────────────────────────────────────────

func TestExportSchedulingSnapshot_NonNil(t *testing.T) {
	q := BuildStageQueue(makePlan(makeStage("s0", 0, nil, planner.RiskLevelLow, nil, 0)))
	snap := ExportSchedulingSnapshot("exec-1", SchedulerRunning, q, nil, NewFairnessState("exec-1"), nil, nil)
	if snap == nil {
		t.Fatal("ExportSchedulingSnapshot must return non-nil")
	}
}

func TestExportSchedulingSnapshot_SchemaVersion(t *testing.T) {
	snap := ExportSchedulingSnapshot("exec-1", SchedulerRunning, StageQueue{}, nil, NewFairnessState("exec-1"), nil, nil)
	if snap.SchemaVersion != SchedulingSnapshotSchemaVersion {
		t.Fatalf("SchemaVersion must be %q, got %q", SchedulingSnapshotSchemaVersion, snap.SchemaVersion)
	}
}

func TestExportSchedulingSnapshot_HashIs64Chars(t *testing.T) {
	snap := ExportSchedulingSnapshot("exec-1", SchedulerRunning, StageQueue{}, nil, NewFairnessState("exec-1"), nil, nil)
	if len(snap.SchedulerHash) != 64 {
		t.Fatalf("SchedulerHash must be 64 hex chars, got len=%d", len(snap.SchedulerHash))
	}
}

func TestVerifySchedulingSnapshotHash_Valid(t *testing.T) {
	snap := ExportSchedulingSnapshot("exec-1", SchedulerRunning, StageQueue{}, nil, NewFairnessState("exec-1"), nil, nil)
	ok, reason := VerifySchedulingSnapshotHash(snap)
	if !ok {
		t.Fatalf("original snapshot must verify successfully: %s", reason)
	}
}

func TestVerifySchedulingSnapshotHash_TamperedState(t *testing.T) {
	snap := ExportSchedulingSnapshot("exec-1", SchedulerRunning, StageQueue{}, nil, NewFairnessState("exec-1"), nil, nil)
	snap.State = SchedulerDrained
	ok, _ := VerifySchedulingSnapshotHash(snap)
	if ok {
		t.Fatal("tampered State must fail verification")
	}
}

func TestVerifySchedulingSnapshotHash_Nil(t *testing.T) {
	ok, reason := VerifySchedulingSnapshotHash(nil)
	if ok || reason == "" {
		t.Fatal("nil snapshot must return false with non-empty reason")
	}
}

func TestExportSchedulingSnapshot_Deterministic(t *testing.T) {
	q := BuildStageQueue(makePlan(makeStage("s0", 0, nil, planner.RiskLevelLow, nil, 0)))
	f := NewFairnessState("exec-1")
	s1 := ExportSchedulingSnapshot("exec-1", SchedulerRunning, q, nil, f, nil, nil)
	s2 := ExportSchedulingSnapshot("exec-1", SchedulerRunning, q, nil, f, nil, nil)
	if s1.SchedulerHash != s2.SchedulerHash {
		t.Fatal("same inputs must produce same SchedulerHash")
	}
}

func TestSchedulingSnapshot_LimitationsNonEmpty(t *testing.T) {
	snap := ExportSchedulingSnapshot("exec-1", SchedulerRunning, StageQueue{}, nil, NewFairnessState("exec-1"), nil, nil)
	if len(snap.Limitations) == 0 {
		t.Fatal("Limitations must be non-empty")
	}
}
