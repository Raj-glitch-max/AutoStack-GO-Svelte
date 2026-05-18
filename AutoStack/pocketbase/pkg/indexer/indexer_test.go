package indexer

import (
	"testing"

	"github.com/janlauber/one-click/pkg/events"
)

// ─── helpers ───────────────────────────────────────────────────────────────

func makeExecEvent(t *testing.T, id string, clock int64) events.PlatformEvent {
	t.Helper()
	return events.NewPlatformEvent(
		id, events.EventTypeExecutionStarted, events.AggregateTypeExecution,
		"agg-1", "exec-1", "tenant-1", clock, "", "", nil, nil,
	)
}

func makeContradictionEvent(t *testing.T, id string, clock int64) events.PlatformEvent {
	t.Helper()
	return events.NewPlatformEvent(
		id, events.EventTypeContradictionDetected, events.AggregateTypeVerification,
		"agg-contra", "exec-1", "tenant-1", clock, "", "", nil, nil,
	)
}

func makeDriftEvent(t *testing.T, id string, clock int64) events.PlatformEvent {
	t.Helper()
	return events.NewPlatformEvent(
		id, events.EventTypeDriftDetected, events.AggregateTypeProvider,
		"agg-drift", "exec-1", "tenant-1", clock, "", "", nil, nil,
	)
}

func makeGovEvent(t *testing.T, id string, clock int64) events.PlatformEvent {
	t.Helper()
	return events.NewPlatformEvent(
		id, events.EventTypeGovernanceDecision, events.AggregateTypeGovernance,
		"agg-gov", "exec-1", "tenant-1", clock, "", "", nil,
		map[string]string{"actor_id": "alice"},
	)
}

func makeProviderObsEvent(t *testing.T, id string, clock int64) events.PlatformEvent {
	t.Helper()
	return events.NewPlatformEvent(
		id, events.EventTypeProviderObservation, events.AggregateTypeProvider,
		"agg-prov", "exec-1", "tenant-1", clock, "", "", nil,
		map[string]string{"provider": "aws-ecs"},
	)
}

// ─── BuildIndex ────────────────────────────────────────────────────────────

func TestBuildIndex_ExecutionType(t *testing.T) {
	evts := []events.PlatformEvent{
		makeExecEvent(t, "e1", 1),
		makeExecEvent(t, "e2", 2),
		makeContradictionEvent(t, "e3", 3), // different type
	}
	idx := BuildIndex("idx-1", IndexTypeExecution, "tenant-1", evts)
	if IndexSize(idx) != 2 {
		t.Fatalf("execution index must have 2 entries, got %d", IndexSize(idx))
	}
}

func TestBuildIndex_ContradictionType(t *testing.T) {
	evts := []events.PlatformEvent{
		makeExecEvent(t, "e1", 1),
		makeContradictionEvent(t, "e2", 2),
	}
	idx := BuildIndex("idx-1", IndexTypeContradiction, "tenant-1", evts)
	if IndexSize(idx) != 1 {
		t.Fatalf("contradiction index must have 1 entry, got %d", IndexSize(idx))
	}
}

func TestBuildIndex_DriftType(t *testing.T) {
	evts := []events.PlatformEvent{
		makeExecEvent(t, "e1", 1),
		makeDriftEvent(t, "e2", 2),
	}
	idx := BuildIndex("idx-1", IndexTypeDrift, "tenant-1", evts)
	if IndexSize(idx) != 1 {
		t.Fatalf("drift index must have 1 entry, got %d", IndexSize(idx))
	}
}

func TestBuildIndex_GovernanceActionType(t *testing.T) {
	evts := []events.PlatformEvent{
		makeGovEvent(t, "e1", 1),
	}
	idx := BuildIndex("idx-1", IndexTypeGovernanceAction, "tenant-1", evts)
	if IndexSize(idx) != 1 {
		t.Fatalf("governance index must have 1 entry, got %d", IndexSize(idx))
	}
}

func TestBuildIndex_ProviderObservationType(t *testing.T) {
	evts := []events.PlatformEvent{
		makeProviderObsEvent(t, "e1", 1),
	}
	idx := BuildIndex("idx-1", IndexTypeProviderObservation, "tenant-1", evts)
	if IndexSize(idx) != 1 {
		t.Fatalf("provider observation index must have 1 entry, got %d", IndexSize(idx))
	}
}

func TestBuildIndex_TenantFiltering(t *testing.T) {
	e1 := makeExecEvent(t, "e1", 1)
	e2 := events.NewPlatformEvent("e2", events.EventTypeExecutionStarted, events.AggregateTypeExecution,
		"agg", "exec-2", "other-tenant", 2, "", "", nil, nil)
	idx := BuildIndex("idx-1", IndexTypeExecution, "tenant-1", []events.PlatformEvent{e1, e2})
	if IndexSize(idx) != 1 {
		t.Fatalf("index must exclude events from other tenants, got %d", IndexSize(idx))
	}
}

func TestBuildIndex_Deterministic(t *testing.T) {
	evts := []events.PlatformEvent{
		makeExecEvent(t, "e2", 2),
		makeExecEvent(t, "e1", 1),
	}
	idx1 := BuildIndex("idx-1", IndexTypeExecution, "tenant-1", evts)
	idx2 := BuildIndex("idx-1", IndexTypeExecution, "tenant-1", evts)
	if idx1.SourceHash != idx2.SourceHash {
		t.Fatal("BuildIndex must be deterministic — same events produce same SourceHash")
	}
	if IndexSize(idx1) != IndexSize(idx2) {
		t.Fatal("BuildIndex must produce same number of entries for same input")
	}
}

func TestBuildIndex_SourceHashNonEmpty(t *testing.T) {
	evts := []events.PlatformEvent{makeExecEvent(t, "e1", 1)}
	idx := BuildIndex("idx-1", IndexTypeExecution, "tenant-1", evts)
	if idx.SourceHash == "" {
		t.Fatal("SourceHash must be non-empty")
	}
}

// ─── RebuildIndex ──────────────────────────────────────────────────────────

func TestRebuildIndex_IncrementRebuildCount(t *testing.T) {
	evts := []events.PlatformEvent{makeExecEvent(t, "e1", 1)}
	idx := BuildIndex("idx-1", IndexTypeExecution, "tenant-1", evts)
	rebuilt := RebuildIndex(idx, evts)
	if rebuilt.RebuildCount != idx.RebuildCount+1 {
		t.Fatalf("RebuildCount must increment: expected %d got %d",
			idx.RebuildCount+1, rebuilt.RebuildCount)
	}
}

func TestRebuildIndex_SameHash(t *testing.T) {
	evts := []events.PlatformEvent{makeExecEvent(t, "e1", 1)}
	idx := BuildIndex("idx-1", IndexTypeExecution, "tenant-1", evts)
	rebuilt := RebuildIndex(idx, evts)
	if idx.SourceHash != rebuilt.SourceHash {
		t.Fatal("rebuild from same events must produce same SourceHash")
	}
}

// ─── QueryIndex ────────────────────────────────────────────────────────────

func TestQueryIndex_EmptyQuery_ReturnsAll(t *testing.T) {
	evts := []events.PlatformEvent{makeExecEvent(t, "e1", 1), makeExecEvent(t, "e2", 2)}
	idx := BuildIndex("idx-1", IndexTypeExecution, "tenant-1", evts)
	result := QueryIndex(idx, nil)
	if len(result) != IndexSize(idx) {
		t.Fatal("empty query must return all entries")
	}
}

func TestQueryIndex_MatchAttribute(t *testing.T) {
	evts := []events.PlatformEvent{makeExecEvent(t, "e1", 1)}
	idx := BuildIndex("idx-1", IndexTypeExecution, "tenant-1", evts)
	result := QueryIndex(idx, map[string]string{"execution_id": "exec-1"})
	if len(result) != 1 {
		t.Fatalf("query by execution_id must return 1 entry, got %d", len(result))
	}
}

func TestQueryIndex_NoMatch(t *testing.T) {
	evts := []events.PlatformEvent{makeExecEvent(t, "e1", 1)}
	idx := BuildIndex("idx-1", IndexTypeExecution, "tenant-1", evts)
	result := QueryIndex(idx, map[string]string{"execution_id": "nonexistent"})
	if len(result) != 0 {
		t.Fatal("query with no matching attribute must return empty")
	}
}

// ─── IsStale ───────────────────────────────────────────────────────────────

func TestIsStale_SameEvents_NotStale(t *testing.T) {
	evts := []events.PlatformEvent{makeExecEvent(t, "e1", 1)}
	idx := BuildIndex("idx-1", IndexTypeExecution, "tenant-1", evts)
	if IsStale(idx, evts) {
		t.Fatal("index built from same events must not be stale")
	}
}

func TestIsStale_NewEvent_Stale(t *testing.T) {
	evts := []events.PlatformEvent{makeExecEvent(t, "e1", 1)}
	idx := BuildIndex("idx-1", IndexTypeExecution, "tenant-1", evts)
	newEvts := append(evts, makeExecEvent(t, "e2", 2))
	if !IsStale(idx, newEvts) {
		t.Fatal("index must be stale when new events exist")
	}
}

// ─── SortedEntries ─────────────────────────────────────────────────────────

func TestSortedEntries_Ordered(t *testing.T) {
	evts := []events.PlatformEvent{
		makeExecEvent(t, "e3", 3),
		makeExecEvent(t, "e1", 1),
		makeExecEvent(t, "e2", 2),
	}
	idx := BuildIndex("idx-1", IndexTypeExecution, "tenant-1", evts)
	sorted := SortedEntries(idx.Entries)
	for i := 1; i < len(sorted); i++ {
		if sorted[i].LogicalClock < sorted[i-1].LogicalClock {
			t.Fatal("SortedEntries must be ordered by LogicalClock ascending")
		}
	}
}
