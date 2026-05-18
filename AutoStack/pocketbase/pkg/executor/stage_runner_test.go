package executor

import (
	"context"
	"testing"

	"github.com/janlauber/one-click/pkg/orchestrator"
	"github.com/janlauber/one-click/pkg/planner"
)

func newTestStage(stageID string, nodeIDs ...string) orchestrator.ExecutionStage {
	nodes := make([]orchestrator.StageNode, len(nodeIDs))
	for i, id := range nodeIDs {
		nodes[i] = orchestrator.StageNode{
			NodeID:   id,
			NodeType: "service",
			Provider: "aws-ecs",
			Action:   planner.ActionCreate,
			Risk:     planner.RiskLevelLow,
		}
	}
	return orchestrator.ExecutionStage{
		StageID:    stageID,
		StageOrder: 0,
		Nodes:      nodes,
	}
}

func TestStageRunner_RunStage_AllSuccess(t *testing.T) {
	r := NewStageRunner(&SucceedingProviderExecutor{})
	j := NewInMemoryJournalStore()
	a := NewInMemoryAmbiguityStore()
	stage := newTestStage("stage-0", "svc-a", "svc-b")

	rec, err := r.RunStage(context.Background(), "exec-1", stage, j, a)
	if err != nil {
		t.Fatalf("RunStage must not error: %v", err)
	}
	if rec.Outcome != StageOutcomeSuccess {
		t.Fatalf("expected %s, got %s", StageOutcomeSuccess, rec.Outcome)
	}
}

func TestStageRunner_RunStage_NodeOrderDeterministic(t *testing.T) {
	r := NewStageRunner(&SucceedingProviderExecutor{})
	j := NewInMemoryJournalStore()
	a := NewInMemoryAmbiguityStore()
	// Nodes inserted in reverse alphabetical order.
	stage := newTestStage("stage-0", "svc-z", "svc-a", "svc-m")

	rec, _ := r.RunStage(context.Background(), "exec-1", stage, j, a)
	if len(rec.Nodes) < 3 {
		t.Fatalf("expected 3 node records, got %d", len(rec.Nodes))
	}
	if rec.Nodes[0].NodeID != "svc-a" {
		t.Fatalf("first node must be svc-a (alphabetical), got %s", rec.Nodes[0].NodeID)
	}
	if rec.Nodes[1].NodeID != "svc-m" {
		t.Fatalf("second node must be svc-m, got %s", rec.Nodes[1].NodeID)
	}
	if rec.Nodes[2].NodeID != "svc-z" {
		t.Fatalf("third node must be svc-z, got %s", rec.Nodes[2].NodeID)
	}
}

func TestStageRunner_RunStage_StopsOnAmbiguity(t *testing.T) {
	r := NewStageRunner(&AmbiguousProviderExecutor{})
	j := NewInMemoryJournalStore()
	a := NewInMemoryAmbiguityStore()
	// Two nodes; ambiguity on the first should stop before the second.
	stage := newTestStage("stage-0", "svc-a", "svc-b")

	rec, _ := r.RunStage(context.Background(), "exec-1", stage, j, a)
	if rec.Outcome != StageOutcomeAmbiguous {
		t.Fatalf("expected %s on ambiguous provider, got %s", StageOutcomeAmbiguous, rec.Outcome)
	}
	if len(rec.Nodes) > 1 {
		t.Fatal("stage runner must stop immediately after first ambiguity — must not execute subsequent nodes")
	}
}

func TestStageRunner_RunStage_StopsOnValidationFailure(t *testing.T) {
	r := NewStageRunner(&NoOpProviderExecutor{}) // always validation fails
	j := NewInMemoryJournalStore()
	a := NewInMemoryAmbiguityStore()
	stage := newTestStage("stage-0", "svc-a", "svc-b")

	rec, _ := r.RunStage(context.Background(), "exec-1", stage, j, a)
	if rec.Outcome != StageOutcomeFailed {
		t.Fatalf("expected %s on validation failure, got %s", StageOutcomeFailed, rec.Outcome)
	}
	if len(rec.Nodes) > 1 {
		t.Fatal("stage runner must stop immediately after first failure")
	}
}

func TestStageRunner_RunStage_EmptyStage_Success(t *testing.T) {
	r := NewStageRunner(&SucceedingProviderExecutor{})
	j := NewInMemoryJournalStore()
	a := NewInMemoryAmbiguityStore()
	stage := orchestrator.ExecutionStage{StageID: "stage-0", StageOrder: 0}

	rec, err := r.RunStage(context.Background(), "exec-1", stage, j, a)
	if err != nil {
		t.Fatalf("empty stage must not error: %v", err)
	}
	if rec.Outcome != StageOutcomeSuccess {
		t.Fatalf("empty stage must succeed, got %s", rec.Outcome)
	}
}

func TestStageRunner_JournalEntriesPresent(t *testing.T) {
	r := NewStageRunner(&SucceedingProviderExecutor{})
	j := NewInMemoryJournalStore()
	a := NewInMemoryAmbiguityStore()
	stage := newTestStage("stage-0", "svc-a")

	r.RunStage(context.Background(), "exec-1", stage, j, a)

	entries, _ := j.List("exec-1")
	if len(entries) == 0 {
		t.Fatal("RunStage must produce journal entries")
	}
}

func TestStageRunner_JournalBeforeAndAfter(t *testing.T) {
	r := NewStageRunner(&SucceedingProviderExecutor{})
	j := NewInMemoryJournalStore()
	a := NewInMemoryAmbiguityStore()
	stage := newTestStage("stage-0", "svc-a")

	r.RunStage(context.Background(), "exec-1", stage, j, a)

	entries, _ := j.List("exec-1")
	// Expect at least 2 entries: pre-action and post-success.
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 journal entries (before+after), got %d", len(entries))
	}
}

func TestStageRunner_AmbiguityRecordPersisted(t *testing.T) {
	r := NewStageRunner(&AmbiguousProviderExecutor{})
	j := NewInMemoryJournalStore()
	a := NewInMemoryAmbiguityStore()
	stage := newTestStage("stage-0", "svc-a")

	r.RunStage(context.Background(), "exec-1", stage, j, a)

	unresolved, _ := a.ListUnresolved("exec-1")
	if len(unresolved) == 0 {
		t.Fatal("ambiguity must be persisted in AmbiguityStore")
	}
}

func TestStageRunner_StartTimestamp_Set(t *testing.T) {
	r := NewStageRunner(&SucceedingProviderExecutor{})
	j := NewInMemoryJournalStore()
	a := NewInMemoryAmbiguityStore()
	stage := newTestStage("stage-0", "svc-a")

	rec, _ := r.RunStage(context.Background(), "exec-1", stage, j, a)
	if rec.StartedAt == "" {
		t.Fatal("StageExecutionRecord.StartedAt must be set")
	}
}

func TestStageRunner_CompletedTimestamp_Set(t *testing.T) {
	r := NewStageRunner(&SucceedingProviderExecutor{})
	j := NewInMemoryJournalStore()
	a := NewInMemoryAmbiguityStore()
	stage := newTestStage("stage-0", "svc-a")

	rec, _ := r.RunStage(context.Background(), "exec-1", stage, j, a)
	if rec.CompletedAt == "" {
		t.Fatal("StageExecutionRecord.CompletedAt must be set")
	}
}

func TestStageRunner_NodeOutcomeSuccess_ProviderRequestIDSet(t *testing.T) {
	r := NewStageRunner(&SucceedingProviderExecutor{})
	j := NewInMemoryJournalStore()
	a := NewInMemoryAmbiguityStore()
	stage := newTestStage("stage-0", "svc-a")

	rec, _ := r.RunStage(context.Background(), "exec-1", stage, j, a)
	if len(rec.Nodes) == 0 {
		t.Fatal("expected node records")
	}
	if rec.Nodes[0].ProviderRequestID == "" {
		t.Fatal("ProviderRequestID must be set on success")
	}
}
