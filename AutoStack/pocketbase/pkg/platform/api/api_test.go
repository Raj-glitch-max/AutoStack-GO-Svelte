package api

import (
	"context"
	"testing"

	"github.com/janlauber/one-click/pkg/governance"
	"github.com/janlauber/one-click/pkg/scheduler"
	"github.com/janlauber/one-click/pkg/topology"
)

// ─── NoopPlatformStore ─────────────────────────────────────────────────────

func TestNoopStore_ListExecutionSummaries(t *testing.T) {
	s := NoopPlatformStore{}
	summaries, err := s.ListExecutionSummaries(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summaries) != 0 {
		t.Fatal("noop store must return empty summaries")
	}
}

func TestNoopStore_GetExecutionDetail_NotFound(t *testing.T) {
	s := NoopPlatformStore{}
	_, err := s.GetExecutionDetail(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("noop store must return not-found error")
	}
}

func TestNoopStore_GetTopologySnapshot_NotFound(t *testing.T) {
	s := NoopPlatformStore{}
	_, err := s.GetTopologySnapshot(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("noop store must return not-found error")
	}
}

func TestNoopStore_GetGovernanceSnapshot_NotFound(t *testing.T) {
	s := NoopPlatformStore{}
	_, err := s.GetGovernanceSnapshot(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("noop store must return not-found error")
	}
}

// ─── InMemoryPlatformStore ─────────────────────────────────────────────────

func TestInMemoryStore_ListExecutionSummaries(t *testing.T) {
	s := NewInMemoryPlatformStore()
	s.Summaries = []ExecutionSummary{
		{ExecutionID: "exec-1", VerificationState: "verified"},
		{ExecutionID: "exec-2", VerificationState: "contradicted"},
	}
	summaries, err := s.ListExecutionSummaries(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}
}

func TestInMemoryStore_GetTopologySnapshot(t *testing.T) {
	s := NewInMemoryPlatformStore()
	plan := topology.BuildTopologyGraph(nil)
	snap := topology.ExportTopologySnapshot(plan, nil, nil, nil)
	s.Topologies["exec-1"] = snap

	got, err := s.GetTopologySnapshot(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil topology snapshot")
	}
}

func TestInMemoryStore_GetGovernanceSnapshot(t *testing.T) {
	s := NewInMemoryPlatformStore()
	chain := governance.NewAuditChain("exec-1")
	snap := governance.ExportGovernanceSnapshot("exec-1", nil, nil, nil, chain, nil, nil, nil)
	s.Governances["exec-1"] = snap

	got, err := s.GetGovernanceSnapshot(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil governance snapshot")
	}
}

func TestInMemoryStore_GetSchedulingSnapshot(t *testing.T) {
	s := NewInMemoryPlatformStore()
	queue := scheduler.StageQueue{ExecutionID: "exec-1"}
	fair := scheduler.NewFairnessState("exec-1")
	snap := scheduler.ExportSchedulingSnapshot("exec-1", scheduler.SchedulerRunning, queue, nil, fair, nil, nil)
	s.Schedules["exec-1"] = snap

	got, err := s.GetSchedulingSnapshot(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil scheduling snapshot")
	}
}

func TestInMemoryStore_GetReplayEvents(t *testing.T) {
	s := NewInMemoryPlatformStore()
	events := []scheduler.SchedulerReplayEvent{
		{Sequence: 1, StageID: "s1", EventType: "enqueued"},
	}
	s.ReplayEvents["exec-1"] = events
	s.ReplayOrders["exec-1"] = []string{"s1"}

	gotEvents, gotOrder, err := s.GetReplayEvents(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(gotEvents))
	}
	if len(gotOrder) != 1 || gotOrder[0] != "s1" {
		t.Fatalf("unexpected replay order: %v", gotOrder)
	}
}

func TestInMemoryStore_GetReplayEvents_NotFound(t *testing.T) {
	s := NewInMemoryPlatformStore()
	_, _, err := s.GetReplayEvents(context.Background(), "missing")
	if err == nil {
		t.Fatal("missing execution must return error")
	}
}

// ─── verifySchedulingSnapshot ──────────────────────────────────────────────

func TestVerifySchedulingSnapshot_Nil(t *testing.T) {
	ok, reason := verifySchedulingSnapshot(nil)
	if ok || reason == "" {
		t.Fatal("nil snapshot must return false with non-empty reason")
	}
}

func TestVerifySchedulingSnapshot_Valid(t *testing.T) {
	queue := scheduler.StageQueue{ExecutionID: "exec-1"}
	fair := scheduler.NewFairnessState("exec-1")
	snap := scheduler.ExportSchedulingSnapshot("exec-1", scheduler.SchedulerRunning, queue, nil, fair, nil, nil)

	ok, reason := verifySchedulingSnapshot(snap)
	if !ok {
		t.Fatalf("valid snapshot must pass verification: %s", reason)
	}
}
