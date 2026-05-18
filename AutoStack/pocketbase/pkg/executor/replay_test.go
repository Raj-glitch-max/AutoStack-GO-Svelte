package executor

import (
	"testing"

	"github.com/janlauber/one-click/pkg/orchestrator"
	"github.com/janlauber/one-click/pkg/planner"
)

func TestReplay_NilSnapshot_Error(t *testing.T) {
	j, a, amb, _ := newTestStores()
	_, err := ReplayExecution("exec-1", nil, j, a, amb)
	if err == nil {
		t.Fatal("ReplayExecution with nil snapshot must error")
	}
}

func TestReplay_EmptyExecutionID_Error(t *testing.T) {
	snap := newNoOpSnapshot(t)
	j, a, amb, _ := newTestStores()
	_, err := ReplayExecution("", snap, j, a, amb)
	if err == nil {
		t.Fatal("ReplayExecution with empty executionID must error")
	}
}

func TestReplay_EmptyJournal_StatePending(t *testing.T) {
	snap := newNoOpSnapshot(t)
	j, a, amb, _ := newTestStores()
	rs, err := ReplayExecution("exec-1", snap, j, a, amb)
	if err != nil {
		t.Fatalf("ReplayExecution must not error on empty journal: %v", err)
	}
	if rs.State != StatePending {
		t.Fatalf("empty journal must produce %s state, got %s", StatePending, rs.State)
	}
}

func TestReplay_ReconstructsLastState(t *testing.T) {
	snap := newNoOpSnapshot(t)
	j, a, amb, _ := newTestStores()

	j.AppendTransition(ExecutionTransition{ExecutionID: "exec-1", FromState: StatePending, ToState: StateReady, Timestamp: "2024-01-01T00:00:01Z"})
	j.AppendTransition(ExecutionTransition{ExecutionID: "exec-1", FromState: StateReady, ToState: StatePaused, Timestamp: "2024-01-01T00:00:02Z"})

	rs, err := ReplayExecution("exec-1", snap, j, a, amb)
	if err != nil {
		t.Fatalf("ReplayExecution must not error: %v", err)
	}
	if rs.State != StatePaused {
		t.Fatalf("replay must reconstruct last state: expected %s, got %s", StatePaused, rs.State)
	}
}

func TestReplay_TransitionsIncluded(t *testing.T) {
	snap := newNoOpSnapshot(t)
	j, a, amb, _ := newTestStores()
	j.AppendTransition(ExecutionTransition{ExecutionID: "exec-1", FromState: StatePending, ToState: StateReady, Timestamp: "2024-01-01T00:00:01Z"})

	rs, _ := ReplayExecution("exec-1", snap, j, a, amb)
	if len(rs.Transitions) != 1 {
		t.Fatalf("replay must include transitions, got %d", len(rs.Transitions))
	}
}

func TestReplay_ReconstructsCompletedNodes(t *testing.T) {
	snap := newCreateSnapshot(t)
	j, a, amb, _ := newTestStores()

	// Simulate a completed node via journal with typed outcome.
	entry := orchestrator.NewJournalEntry("exec-1", "", "", "stage-0", "svc-a", planner.ActionCreate, "test")
	completed := entry.WithOutcome(orchestrator.JournalOutcomeSuccess)
	j.Append(completed)

	rs, _ := ReplayExecution("exec-1", snap, j, a, amb)
	key := "stage-0:svc-a"
	if !rs.CompletedNodes[key] {
		t.Fatalf("replay must mark %q as completed", key)
	}
}

func TestReplay_CurrentStageIndex_Correct(t *testing.T) {
	snap := newChainCreateSnapshot(t) // 2 stages: stage-0 (svc-a), stage-1 (svc-b)
	j, a, amb, _ := newTestStores()

	// Mark all nodes in stage-0 as complete with typed outcome.
	if len(snap.Plan.Stages) == 0 {
		t.Skip("no stages in snapshot")
	}
	stage0 := snap.Plan.Stages[0]
	for _, node := range stage0.Nodes {
		e := orchestrator.NewJournalEntry("exec-1", "", "", stage0.StageID, node.NodeID, planner.ActionCreate, "test")
		j.Append(e.WithOutcome(orchestrator.JournalOutcomeSuccess))
	}

	rs, _ := ReplayExecution("exec-1", snap, j, a, amb)
	if rs.CurrentStageIndex != 1 {
		t.Fatalf("CurrentStageIndex must be 1 after stage-0 complete, got %d", rs.CurrentStageIndex)
	}
}

func TestReplay_PendingApprovals_Listed(t *testing.T) {
	snap := newDeleteSnapshot(t) // delete has approval gates
	j, a, amb, _ := newTestStores()

	rs, _ := ReplayExecution("exec-1", snap, j, a, amb)
	if len(snap.Plan.ApprovalGates) > 0 && len(rs.PendingApprovals) == 0 {
		t.Fatal("unapproved gates must appear in PendingApprovals")
	}
}

func TestReplay_ApprovedGates_NotListed(t *testing.T) {
	snap := newDeleteSnapshot(t)
	j, a, amb, _ := newTestStores()

	// Approve all gates.
	for _, gate := range snap.Plan.ApprovalGates {
		a.RecordDecision(ApprovalDecision{
			ExecutionID:  "exec-1",
			GateID:       gate.GateID,
			OperatorID:   "op-1",
			OperatorRole: "admin",
			Approved:     true,
		})
	}

	rs, _ := ReplayExecution("exec-1", snap, j, a, amb)
	if len(rs.PendingApprovals) != 0 {
		t.Fatalf("approved gates must not appear in PendingApprovals, got %d", len(rs.PendingApprovals))
	}
}

func TestReplay_UnresolvedAmbiguities_Listed(t *testing.T) {
	snap := newNoOpSnapshot(t)
	j, a, amb, _ := newTestStores()

	record := NewAmbiguityRecord("exec-1", "stage-0", "svc-a", "timeout", "desc")
	amb.Record(record)

	rs, _ := ReplayExecution("exec-1", snap, j, a, amb)
	if len(rs.AmbiguityIDs) == 0 {
		t.Fatal("unresolved ambiguities must appear in replay state")
	}
}

func TestReplay_ResolvedAmbiguities_NotListed(t *testing.T) {
	snap := newNoOpSnapshot(t)
	j, a, ambStore, _ := newTestStores()

	record := NewAmbiguityRecord("exec-1", "stage-0", "svc-a", "timeout", "desc")
	ambStore.Record(record)
	ambStore.Resolve(record.AmbiguityID, "op-1", "confirmed")

	rs, _ := ReplayExecution("exec-1", snap, j, a, ambStore)
	if len(rs.AmbiguityIDs) != 0 {
		t.Fatal("resolved ambiguities must not appear in replay state")
	}
}

func TestReplay_IsReadOnly_NoNewEntries(t *testing.T) {
	snap := newNoOpSnapshot(t)
	j, a, amb, _ := newTestStores()
	j.AppendTransition(ExecutionTransition{ExecutionID: "exec-1", FromState: StatePending, ToState: StateReady, Timestamp: "2024-01-01T00:00:01Z"})

	beforeEntries, _ := j.List("exec-1")
	ReplayExecution("exec-1", snap, j, a, amb)
	afterEntries, _ := j.List("exec-1")

	if len(afterEntries) != len(beforeEntries) {
		t.Fatal("ReplayExecution must be read-only — must not append new journal entries")
	}
}
