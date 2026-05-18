package replay

import (
	"testing"

	"github.com/janlauber/one-click/pkg/events"
)

// ─── helpers ───────────────────────────────────────────────────────────────

func makeEvent(t *testing.T, id string, clock int64, execID string) events.PlatformEvent {
	t.Helper()
	return events.NewPlatformEvent(
		id, events.EventTypeExecutionStarted, events.AggregateTypeExecution,
		"agg-1", execID, "tenant-1", clock, "", "", nil, nil,
	)
}

func buildStream(t *testing.T, evts ...events.PlatformEvent) events.EventStream {
	t.Helper()
	s := events.NewEventStream("s1", "tenant-1", "exec-1")
	for _, e := range evts {
		var err error
		s, err = events.AppendEvent(s, e)
		if err != nil {
			t.Fatalf("AppendEvent: %v", err)
		}
	}
	return s
}

// ─── checkpoint ────────────────────────────────────────────────────────────

func TestNewCheckpoint_StateHashNonEmpty(t *testing.T) {
	evts := []events.PlatformEvent{makeEvent(t, "e1", 1, "exec-1")}
	cp := NewCheckpoint("cp1", "s1", "exec-1", "tenant-1", evts)
	if cp.StateHash == "" {
		t.Fatal("StateHash must be non-empty")
	}
}

func TestNewCheckpoint_AtClockIsLastClock(t *testing.T) {
	evts := []events.PlatformEvent{
		makeEvent(t, "e1", 1, "exec-1"),
		makeEvent(t, "e2", 2, "exec-1"),
	}
	cp := NewCheckpoint("cp1", "s1", "exec-1", "tenant-1", evts)
	if cp.AtClock != 2 {
		t.Fatalf("AtClock must be 2, got %d", cp.AtClock)
	}
}

func TestVerifyCheckpoint_Valid(t *testing.T) {
	evts := []events.PlatformEvent{
		makeEvent(t, "e1", 1, "exec-1"),
		makeEvent(t, "e2", 2, "exec-1"),
	}
	cp := NewCheckpoint("cp1", "s1", "exec-1", "tenant-1", evts)
	ok, reason := VerifyCheckpoint(cp, evts)
	if !ok {
		t.Fatalf("valid checkpoint must verify: %s", reason)
	}
}

func TestVerifyCheckpoint_ExtraEvent(t *testing.T) {
	evts := []events.PlatformEvent{makeEvent(t, "e1", 1, "exec-1")}
	cp := NewCheckpoint("cp1", "s1", "exec-1", "tenant-1", evts)
	// Add an event at clock 2; checkpoint covers clock 1.
	extra := makeEvent(t, "e2", 2, "exec-1")
	allEvts := append(evts, extra)
	// VerifyCheckpoint only considers events up to AtClock=1.
	ok, reason := VerifyCheckpoint(cp, allEvts)
	if !ok {
		t.Fatalf("checkpoint should still verify when extra events exist beyond AtClock: %s", reason)
	}
}

func TestFindNearestCheckpoint_Found(t *testing.T) {
	checkpoints := []ReplayCheckpoint{
		{CheckpointID: "cp1", AtClock: 5},
		{CheckpointID: "cp2", AtClock: 10},
		{CheckpointID: "cp3", AtClock: 15},
	}
	cp := FindNearestCheckpoint(checkpoints, 12)
	if cp == nil || cp.AtClock != 10 {
		t.Fatalf("expected cp2 (clock 10), got %v", cp)
	}
}

func TestFindNearestCheckpoint_NoneBeforeFrom(t *testing.T) {
	checkpoints := []ReplayCheckpoint{{CheckpointID: "cp1", AtClock: 20}}
	cp := FindNearestCheckpoint(checkpoints, 5)
	if cp != nil {
		t.Fatal("no checkpoint before fromClock must return nil")
	}
}

func TestCheckpointStateHash_Deterministic(t *testing.T) {
	evts := []events.PlatformEvent{makeEvent(t, "e1", 1, "exec-1")}
	h1 := CheckpointStateHash(evts)
	h2 := CheckpointStateHash(evts)
	if h1 != h2 {
		t.Fatal("CheckpointStateHash must be deterministic")
	}
}

func TestCheckpointStateHash_EmptySlice(t *testing.T) {
	h := CheckpointStateHash(nil)
	if len(h) != 64 {
		t.Fatalf("empty slice hash must be 64 hex chars, got %d", len(h))
	}
}

// ─── engine — full replay ──────────────────────────────────────────────────

func TestReplayExecution_AllEvents(t *testing.T) {
	e1 := makeEvent(t, "e1", 1, "exec-1")
	e2 := makeEvent(t, "e2", 2, "exec-1")
	e3 := makeEvent(t, "e3", 3, "exec-2") // different execution
	s := buildStream(t, e1, e2, e3)

	result := ReplayExecution(s, "exec-1")
	if result.EventCount != 2 {
		t.Fatalf("expected 2 events for exec-1, got %d", result.EventCount)
	}
	if result.Diverged {
		t.Fatal("full replay must not be marked as diverged")
	}
}

func TestReplayExecution_FromClockToClockSet(t *testing.T) {
	e1 := makeEvent(t, "e1", 1, "exec-1")
	e2 := makeEvent(t, "e2", 5, "exec-1")
	s := buildStream(t, e1, e2)
	result := ReplayExecution(s, "exec-1")
	if result.FromClock != 1 || result.ToClock != 5 {
		t.Fatalf("FromClock=%d ToClock=%d, expected 1 and 5", result.FromClock, result.ToClock)
	}
}

func TestReplayExecution_LimitationsNonEmpty(t *testing.T) {
	s := events.NewEventStream("s1", "tenant-1", "exec-1")
	result := ReplayExecution(s, "exec-1")
	if len(result.Limitations) == 0 {
		t.Fatal("ReplayExecution must always include limitations")
	}
}

func TestReplayExecution_ReplayedAtSet(t *testing.T) {
	s := events.NewEventStream("s1", "tenant-1", "exec-1")
	result := ReplayExecution(s, "exec-1")
	if result.ReplayedAt == "" {
		t.Fatal("ReplayedAt must be set")
	}
}

// ─── engine — checkpoint replay ────────────────────────────────────────────

func TestReplayFromCheckpoint_ExcludesPreCheckpointEvents(t *testing.T) {
	e1 := makeEvent(t, "e1", 1, "exec-1")
	e2 := makeEvent(t, "e2", 2, "exec-1")
	e3 := makeEvent(t, "e3", 3, "exec-1")
	s := buildStream(t, e1, e2, e3)

	cp := NewCheckpoint("cp1", "s1", "exec-1", "tenant-1", []events.PlatformEvent{e1, e2})
	result := ReplayFromCheckpoint(s, cp, "exec-1")
	// Only e3 (clock 3) should be returned (after cp at clock 2).
	if result.EventCount != 1 || result.Events[0].LogicalClock != 3 {
		t.Fatalf("expected 1 post-checkpoint event at clock 3, got %d events", result.EventCount)
	}
}

func TestReplayFromCheckpoint_CheckpointInResult(t *testing.T) {
	s := events.NewEventStream("s1", "tenant-1", "exec-1")
	cp := NewCheckpoint("cp1", "s1", "exec-1", "tenant-1", nil)
	result := ReplayFromCheckpoint(s, cp, "exec-1")
	if len(result.Checkpoints) != 1 {
		t.Fatal("checkpoint must appear in ReplayResult.Checkpoints")
	}
}

// ─── engine — partial replay ───────────────────────────────────────────────

func TestReplayPartial_RangeFiltering(t *testing.T) {
	var evts []events.PlatformEvent
	for i := int64(1); i <= 10; i++ {
		evts = append(evts, makeEvent(t, string(rune('a'+i-1)), i, "exec-1"))
	}
	s := buildStream(t, evts...)
	result := ReplayPartial(s, 3, 7, nil)
	if result.EventCount != 5 {
		t.Fatalf("expected 5 events in [3,7], got %d", result.EventCount)
	}
}

func TestReplayPartial_CheckpointAssisted(t *testing.T) {
	var evts []events.PlatformEvent
	for i := int64(1); i <= 10; i++ {
		evts = append(evts, makeEvent(t, string(rune('a'+i-1)), i, "exec-1"))
	}
	s := buildStream(t, evts...)
	cp := NewCheckpoint("cp", "s1", "exec-1", "tenant-1", evts[:5])
	result := ReplayPartial(s, 6, 10, []ReplayCheckpoint{cp})
	if len(result.Checkpoints) != 1 {
		t.Fatal("checkpoint must be recorded in partial replay result")
	}
}

// ─── divergence detection ──────────────────────────────────────────────────

func TestDetectReplayDivergence_NoDivergence(t *testing.T) {
	e1 := makeEvent(t, "e1", 1, "exec-1")
	e2 := makeEvent(t, "e2", 2, "exec-1")
	s := buildStream(t, e1, e2)
	r1 := ReplayExecution(s, "exec-1")
	r2 := ReplayExecution(s, "exec-1")
	diverged, at := DetectReplayDivergence(r1, r2)
	if diverged {
		t.Fatalf("identical replays must not diverge (at clock %d)", at)
	}
}

func TestDetectReplayDivergence_DifferentEvents(t *testing.T) {
	e1 := makeEvent(t, "e1", 1, "exec-1")
	e2 := makeEvent(t, "e2", 2, "exec-1")
	e3 := makeEvent(t, "e3", 2, "exec-1") // same clock, different ID
	s1 := buildStream(t, e1, e2)
	s2 := buildStream(t, e1, e3)
	r1 := ReplayExecution(s1, "exec-1")
	r2 := ReplayExecution(s2, "exec-1")
	diverged, _ := DetectReplayDivergence(r1, r2)
	if !diverged {
		t.Fatal("replays with different events must diverge")
	}
}

func TestDetectReplayDivergence_DifferentLengths(t *testing.T) {
	e1 := makeEvent(t, "e1", 1, "exec-1")
	e2 := makeEvent(t, "e2", 2, "exec-1")
	s1 := buildStream(t, e1, e2)
	s2 := buildStream(t, e1)
	r1 := ReplayExecution(s1, "exec-1")
	r2 := ReplayExecution(s2, "exec-1")
	diverged, _ := DetectReplayDivergence(r1, r2)
	if !diverged {
		t.Fatal("replays of different lengths must diverge")
	}
}

// ─── manifest ──────────────────────────────────────────────────────────────

func TestBuildReplayManifest_HashIs64Chars(t *testing.T) {
	m := BuildReplayManifest("m1", "exec-1", "tenant-1", nil, 0)
	if len(m.ManifestHash) != 64 {
		t.Fatalf("ManifestHash must be 64 hex chars, got %d", len(m.ManifestHash))
	}
}

func TestVerifyReplayManifest_Valid(t *testing.T) {
	evts := []events.PlatformEvent{makeEvent(t, "e1", 1, "exec-1")}
	cp := NewCheckpoint("cp1", "s1", "exec-1", "tenant-1", evts)
	m := BuildReplayManifest("m1", "exec-1", "tenant-1", []ReplayCheckpoint{cp}, 1)
	ok, reason := VerifyReplayManifest(m)
	if !ok {
		t.Fatalf("original manifest must verify: %s", reason)
	}
}

func TestVerifyReplayManifest_TamperedExecutionID(t *testing.T) {
	m := BuildReplayManifest("m1", "exec-1", "tenant-1", nil, 0)
	m.ExecutionID = "tampered"
	ok, _ := VerifyReplayManifest(m)
	if ok {
		t.Fatal("tampered ExecutionID must fail verification")
	}
}

func TestVerifyReplayManifest_EmptyHash(t *testing.T) {
	m := BuildReplayManifest("m1", "exec-1", "tenant-1", nil, 0)
	m.ManifestHash = ""
	ok, reason := VerifyReplayManifest(m)
	if ok || reason == "" {
		t.Fatal("empty hash must fail with reason")
	}
}

func TestBuildReplayManifest_Deterministic(t *testing.T) {
	evts := []events.PlatformEvent{makeEvent(t, "e1", 1, "exec-1")}
	cp := NewCheckpoint("cp1", "s1", "exec-1", "tenant-1", evts)
	m1 := BuildReplayManifest("m1", "exec-1", "tenant-1", []ReplayCheckpoint{cp}, 5)
	m2 := BuildReplayManifest("m1", "exec-1", "tenant-1", []ReplayCheckpoint{cp}, 5)
	if m1.ManifestHash != m2.ManifestHash {
		t.Fatal("same inputs must produce same ManifestHash")
	}
}
