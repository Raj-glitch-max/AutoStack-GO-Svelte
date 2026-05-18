package events

import (
	"context"
	"testing"
)

// ─── helpers ───────────────────────────────────────────────────────────────

func makeEvent(t *testing.T, eventID string, clock int64) PlatformEvent {
	t.Helper()
	return NewPlatformEvent(
		eventID, EventTypeExecutionStarted, AggregateTypeExecution,
		"agg-1", "exec-1", "tenant-1",
		clock, "", "", nil, nil,
	)
}

// ─── event hash ────────────────────────────────────────────────────────────

func TestNewPlatformEvent_HashNonEmpty(t *testing.T) {
	e := makeEvent(t, "e1", 1)
	if e.Hash == "" {
		t.Fatal("Hash must be non-empty after NewPlatformEvent")
	}
}

func TestComputeEventHash_Deterministic(t *testing.T) {
	e := makeEvent(t, "e1", 1)
	h1 := ComputeEventHash(e)
	h2 := ComputeEventHash(e)
	if h1 != h2 {
		t.Fatal("ComputeEventHash must be deterministic for same input")
	}
}

func TestComputeEventHash_Length64(t *testing.T) {
	e := makeEvent(t, "e1", 1)
	if len(e.Hash) != 64 {
		t.Fatalf("Hash must be 64 hex chars, got %d", len(e.Hash))
	}
}

func TestVerifyEventHash_Valid(t *testing.T) {
	e := makeEvent(t, "e1", 1)
	ok, reason := VerifyEventHash(e)
	if !ok {
		t.Fatalf("valid event must verify: %s", reason)
	}
}

func TestVerifyEventHash_TamperedExecutionID(t *testing.T) {
	e := makeEvent(t, "e1", 1)
	e.ExecutionID = "tampered"
	ok, _ := VerifyEventHash(e)
	if ok {
		t.Fatal("tampered ExecutionID must fail hash verification")
	}
}

func TestVerifyEventHash_TamperedEventType(t *testing.T) {
	e := makeEvent(t, "e1", 1)
	e.EventType = EventTypeExecutionFailed
	ok, _ := VerifyEventHash(e)
	if ok {
		t.Fatal("tampered EventType must fail hash verification")
	}
}

func TestVerifyEventHash_EmptyHash(t *testing.T) {
	e := makeEvent(t, "e1", 1)
	e.Hash = ""
	ok, reason := VerifyEventHash(e)
	if ok || reason == "" {
		t.Fatal("empty hash must fail with reason")
	}
}

func TestComputeEventHash_MetadataDeterministic(t *testing.T) {
	meta := map[string]string{"b": "2", "a": "1", "c": "3"}
	e := NewPlatformEvent("e1", EventTypeExecutionStarted, AggregateTypeExecution,
		"agg", "exec", "tenant", 1, "", "", nil, meta)
	// Same metadata in different insertion order.
	meta2 := map[string]string{"c": "3", "a": "1", "b": "2"}
	e2 := NewPlatformEvent("e1", EventTypeExecutionStarted, AggregateTypeExecution,
		"agg", "exec", "tenant", 1, "", "", nil, meta2)
	e2.Timestamp = e.Timestamp // normalize non-deterministic field
	e2.Hash = ComputeEventHash(e2)
	if e.Hash != e2.Hash {
		t.Fatal("metadata order must not affect hash — keys must be sorted")
	}
}

// ─── ordering ──────────────────────────────────────────────────────────────

func TestCompareEvents_ByLogicalClock(t *testing.T) {
	a := makeEvent(t, "e1", 1)
	b := makeEvent(t, "e2", 2)
	if CompareEvents(a, b) >= 0 {
		t.Fatal("lower clock must sort before higher clock")
	}
}

func TestCompareEvents_TieBreakByEventID(t *testing.T) {
	a := makeEvent(t, "ea", 1)
	b := makeEvent(t, "eb", 1)
	a.Timestamp = b.Timestamp // force timestamp tie
	a.Hash = ComputeEventHash(a)
	b.Hash = ComputeEventHash(b)
	if CompareEvents(a, b) >= 0 {
		t.Fatal("lexically earlier EventID must sort first on clock+timestamp tie")
	}
}

func TestSortEvents_Deterministic(t *testing.T) {
	evts := []PlatformEvent{
		makeEvent(t, "e3", 3),
		makeEvent(t, "e1", 1),
		makeEvent(t, "e2", 2),
	}
	s1 := SortEvents(evts)
	s2 := SortEvents(evts)
	for i := range s1 {
		if s1[i].EventID != s2[i].EventID {
			t.Fatalf("SortEvents must be deterministic at index %d", i)
		}
	}
}

func TestSortEvents_DoesNotMutateOriginal(t *testing.T) {
	evts := []PlatformEvent{makeEvent(t, "e2", 2), makeEvent(t, "e1", 1)}
	_ = SortEvents(evts)
	if evts[0].EventID != "e2" {
		t.Fatal("SortEvents must not mutate the original slice")
	}
}

func TestIsTotallyOrdered_True(t *testing.T) {
	evts := []PlatformEvent{makeEvent(t, "e1", 1), makeEvent(t, "e2", 2)}
	if !IsTotallyOrdered(evts) {
		t.Fatal("ascending clock order must be totally ordered")
	}
}

func TestIsTotallyOrdered_False(t *testing.T) {
	evts := []PlatformEvent{makeEvent(t, "e2", 2), makeEvent(t, "e1", 1)}
	if IsTotallyOrdered(evts) {
		t.Fatal("descending clock order must NOT be totally ordered")
	}
}

// ─── stream ─────────────────────────────────────────────────────────────────

func TestNewEventStream_EmptyHash64(t *testing.T) {
	s := NewEventStream("s1", "tenant-1", "exec-1")
	if len(s.StreamHash) != 64 {
		t.Fatalf("StreamHash must be 64 hex chars, got %d", len(s.StreamHash))
	}
}

func TestAppendEvent_Success(t *testing.T) {
	s := NewEventStream("s1", "tenant-1", "exec-1")
	e := makeEvent(t, "e1", 1)
	s2, err := AppendEvent(s, e)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if StreamLength(s2) != 1 {
		t.Fatal("stream must have 1 event after append")
	}
}

func TestAppendEvent_InvalidHash_Rejected(t *testing.T) {
	s := NewEventStream("s1", "tenant-1", "exec-1")
	e := makeEvent(t, "e1", 1)
	e.ExecutionID = "tampered" // invalidates hash
	_, err := AppendEvent(s, e)
	if err == nil {
		t.Fatal("tampered event must be rejected by AppendEvent")
	}
}

func TestAppendEvent_RegressingClock_Rejected(t *testing.T) {
	s := NewEventStream("s1", "tenant-1", "exec-1")
	e1 := makeEvent(t, "e1", 5)
	s, _ = AppendEvent(s, e1)
	e2 := makeEvent(t, "e2", 3) // regresses
	_, err := AppendEvent(s, e2)
	if err == nil {
		t.Fatal("regressing logical clock must be rejected")
	}
}

func TestAppendEvent_ValueSemantics(t *testing.T) {
	s := NewEventStream("s1", "tenant-1", "exec-1")
	e := makeEvent(t, "e1", 1)
	_, _ = AppendEvent(s, e)
	if StreamLength(s) != 0 {
		t.Fatal("AppendEvent must not mutate original stream")
	}
}

func TestEventStreamHash_ChangesOnAppend(t *testing.T) {
	s := NewEventStream("s1", "tenant-1", "exec-1")
	h1 := s.StreamHash
	e := makeEvent(t, "e1", 1)
	s2, _ := AppendEvent(s, e)
	if s2.StreamHash == h1 {
		t.Fatal("StreamHash must change when an event is appended")
	}
}

func TestEventsInRange_Inclusive(t *testing.T) {
	s := NewEventStream("s1", "tenant-1", "exec-1")
	for i := int64(1); i <= 5; i++ {
		e := makeEvent(t, string(rune('a'+i-1)), i)
		s, _ = AppendEvent(s, e)
	}
	evts := EventsInRange(s, 2, 4)
	if len(evts) != 3 {
		t.Fatalf("expected 3 events in [2,4], got %d", len(evts))
	}
}

// ─── integrity ─────────────────────────────────────────────────────────────

func TestVerifyStreamIntegrity_Valid(t *testing.T) {
	s := NewEventStream("s1", "tenant-1", "exec-1")
	s, _ = AppendEvent(s, makeEvent(t, "e1", 1))
	s, _ = AppendEvent(s, makeEvent(t, "e2", 2))
	ok, reason := VerifyStreamIntegrity(s)
	if !ok {
		t.Fatalf("valid stream must verify: %s", reason)
	}
}

func TestVerifyStreamIntegrity_TamperedStreamHash(t *testing.T) {
	s := NewEventStream("s1", "tenant-1", "exec-1")
	s, _ = AppendEvent(s, makeEvent(t, "e1", 1))
	s.StreamHash = "badhash"
	ok, _ := VerifyStreamIntegrity(s)
	if ok {
		t.Fatal("tampered StreamHash must fail integrity check")
	}
}

func TestVerifyEventSequence_Valid(t *testing.T) {
	evts := SortEvents([]PlatformEvent{makeEvent(t, "e1", 1), makeEvent(t, "e2", 2)})
	ok, reason := VerifyEventSequence(evts)
	if !ok {
		t.Fatalf("valid ordered sequence must verify: %s", reason)
	}
}

func TestVerifyEventSequence_Unordered(t *testing.T) {
	evts := []PlatformEvent{makeEvent(t, "e2", 2), makeEvent(t, "e1", 1)}
	ok, _ := VerifyEventSequence(evts)
	if ok {
		t.Fatal("unordered sequence must fail verification")
	}
}

// ─── serialization ─────────────────────────────────────────────────────────

func TestCanonicalSerialize_Roundtrip(t *testing.T) {
	e := makeEvent(t, "e1", 1)
	data, err := CanonicalSerialize(e)
	if err != nil {
		t.Fatalf("serialize error: %v", err)
	}
	got, err := DeserializeEvent(data)
	if err != nil {
		t.Fatalf("deserialize error: %v", err)
	}
	if got.Hash != e.Hash {
		t.Fatal("hash must survive serialization roundtrip")
	}
}

func TestRoundtripEvent_Valid(t *testing.T) {
	e := makeEvent(t, "e1", 1)
	_, err := RoundtripEvent(e)
	if err != nil {
		t.Fatalf("roundtrip must succeed for valid event: %v", err)
	}
}

func TestSerializeStream_Roundtrip(t *testing.T) {
	s := NewEventStream("s1", "tenant-1", "exec-1")
	s, _ = AppendEvent(s, makeEvent(t, "e1", 1))
	data, err := SerializeStream(s)
	if err != nil {
		t.Fatalf("serialize error: %v", err)
	}
	got, err := DeserializeStream(data)
	if err != nil {
		t.Fatalf("deserialize error: %v", err)
	}
	if got.StreamHash != s.StreamHash {
		t.Fatal("StreamHash must survive serialization roundtrip")
	}
}

// ─── filters ───────────────────────────────────────────────────────────────

func TestFilterByExecution_Correct(t *testing.T) {
	e1 := makeEvent(t, "e1", 1)
	e2 := NewPlatformEvent("e2", EventTypeStageEnqueued, AggregateTypeStage,
		"agg", "exec-2", "tenant-1", 2, "", "", nil, nil)
	result := FilterByExecution([]PlatformEvent{e1, e2}, "exec-1")
	if len(result) != 1 || result[0].EventID != "e1" {
		t.Fatalf("FilterByExecution returned wrong events: %+v", result)
	}
}

func TestFilterByTenant_Correct(t *testing.T) {
	e1 := makeEvent(t, "e1", 1)
	e2 := NewPlatformEvent("e2", EventTypeStageEnqueued, AggregateTypeStage,
		"agg", "exec", "tenant-2", 2, "", "", nil, nil)
	result := FilterByTenant([]PlatformEvent{e1, e2}, "tenant-1")
	if len(result) != 1 || result[0].TenantID != "tenant-1" {
		t.Fatal("FilterByTenant returned wrong events")
	}
}

func TestFilterByEventTypes_Correct(t *testing.T) {
	e1 := makeEvent(t, "e1", 1)
	e2 := NewPlatformEvent("e2", EventTypeExecutionFailed, AggregateTypeExecution,
		"agg", "exec", "tenant-1", 2, "", "", nil, nil)
	result := FilterByEventTypes([]PlatformEvent{e1, e2}, EventTypeExecutionFailed)
	if len(result) != 1 || result[0].EventID != "e2" {
		t.Fatal("FilterByEventTypes returned wrong events")
	}
}

// ─── subscriptions ─────────────────────────────────────────────────────────

func TestMatchesSubscription_TenantMismatch(t *testing.T) {
	e := makeEvent(t, "e1", 1)
	sub := NewSubscription("s1", "other-tenant", nil, nil)
	if MatchesSubscription(e, sub) {
		t.Fatal("event from different tenant must not match subscription")
	}
}

func TestMatchesSubscription_AllCriteria(t *testing.T) {
	e := makeEvent(t, "e1", 1)
	sub := NewSubscription("s1", "tenant-1",
		[]AggregateType{AggregateTypeExecution},
		[]EventType{EventTypeExecutionStarted},
	)
	sub.ExecutionIDs = []string{"exec-1"}
	if !MatchesSubscription(e, sub) {
		t.Fatal("event matching all criteria must return true")
	}
}

func TestFilterBySubscription_MultipleEvents(t *testing.T) {
	e1 := makeEvent(t, "e1", 1)
	e2 := NewPlatformEvent("e2", EventTypeExecutionFailed, AggregateTypeExecution,
		"agg", "exec-1", "tenant-1", 2, "", "", nil, nil)
	sub := NewSubscription("s1", "tenant-1", nil, []EventType{EventTypeExecutionStarted})
	result := FilterBySubscription([]PlatformEvent{e1, e2}, sub)
	if len(result) != 1 || result[0].EventID != "e1" {
		t.Fatal("only events matching EventType filter must be returned")
	}
}

// ─── fabric snapshot ───────────────────────────────────────────────────────

func TestExportFabricSnapshot_NonNil(t *testing.T) {
	snap := ExportFabricSnapshot("tenant-1", "exec-1", nil, nil, nil)
	if snap == nil {
		t.Fatal("ExportFabricSnapshot must return non-nil")
	}
}

func TestExportFabricSnapshot_HashIs64Chars(t *testing.T) {
	snap := ExportFabricSnapshot("tenant-1", "exec-1", nil, nil, nil)
	if len(snap.IntegrityHash) != 64 {
		t.Fatalf("IntegrityHash must be 64 hex chars, got %d", len(snap.IntegrityHash))
	}
}

func TestVerifyFabricSnapshot_Valid(t *testing.T) {
	e := makeEvent(t, "e1", 1)
	snap := ExportFabricSnapshot("tenant-1", "exec-1", []PlatformEvent{e}, nil, nil)
	ok, reason := VerifyFabricSnapshot(snap)
	if !ok {
		t.Fatalf("original snapshot must verify: %s", reason)
	}
}

func TestVerifyFabricSnapshot_TamperedExecutionID(t *testing.T) {
	snap := ExportFabricSnapshot("tenant-1", "exec-1", nil, nil, nil)
	snap.ExecutionID = "tampered"
	ok, _ := VerifyFabricSnapshot(snap)
	if ok {
		t.Fatal("tampered ExecutionID must fail verification")
	}
}

func TestVerifyFabricSnapshot_Nil(t *testing.T) {
	ok, reason := VerifyFabricSnapshot(nil)
	if ok || reason == "" {
		t.Fatal("nil snapshot must return false with reason")
	}
}

func TestExportFabricSnapshot_Deterministic(t *testing.T) {
	e := makeEvent(t, "e1", 1)
	s1 := ExportFabricSnapshot("tenant-1", "exec-1", []PlatformEvent{e}, nil, nil)
	s2 := ExportFabricSnapshot("tenant-1", "exec-1", []PlatformEvent{e}, nil, nil)
	if s1.IntegrityHash != s2.IntegrityHash {
		t.Fatal("same inputs must produce same IntegrityHash")
	}
}

func TestNewEventCheckpoint_StateHashNonEmpty(t *testing.T) {
	evts := []PlatformEvent{makeEvent(t, "e1", 1), makeEvent(t, "e2", 2)}
	cp := NewEventCheckpoint("cp1", "s1", "exec-1", "tenant-1", evts)
	if cp.StateHash == "" {
		t.Fatal("StateHash must be non-empty")
	}
	if cp.AtClock != 2 {
		t.Fatalf("AtClock must be last event clock, got %d", cp.AtClock)
	}
}

func TestVerifyCheckpointStateHash_Valid(t *testing.T) {
	evts := []PlatformEvent{makeEvent(t, "e1", 1), makeEvent(t, "e2", 2)}
	cp := NewEventCheckpoint("cp1", "s1", "exec-1", "tenant-1", evts)
	ok, reason := VerifyCheckpointStateHash(cp, evts)
	if !ok {
		t.Fatalf("original checkpoint must verify: %s", reason)
	}
}

func TestVerifyCheckpointStateHash_ExtraEvent(t *testing.T) {
	evts := []PlatformEvent{makeEvent(t, "e1", 1)}
	cp := NewEventCheckpoint("cp1", "s1", "exec-1", "tenant-1", evts)
	evts = append(evts, makeEvent(t, "e2", 2))
	ok, _ := VerifyCheckpointStateHash(cp, evts)
	if ok {
		t.Fatal("checkpoint with extra events must fail verification")
	}
}

// ─── persistence ───────────────────────────────────────────────────────────

func TestInMemoryEventStore_AppendAndLoad(t *testing.T) {
	store := NewInMemoryEventStore()
	ctx := context.Background()
	clock, _ := store.NextLogicalClock(ctx)
	e := makeEvent(t, "e1", clock)
	if err := store.Append(ctx, "stream-1", e); err != nil {
		t.Fatalf("unexpected append error: %v", err)
	}
	stream, err := store.Load(ctx, "stream-1")
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if StreamLength(stream) != 1 {
		t.Fatalf("expected 1 event, got %d", StreamLength(stream))
	}
}

func TestInMemoryEventStore_Load_NotFound(t *testing.T) {
	store := NewInMemoryEventStore()
	_, err := store.Load(context.Background(), "missing")
	if err == nil {
		t.Fatal("loading missing stream must return error")
	}
}

func TestInMemoryEventStore_NextLogicalClock_Monotonic(t *testing.T) {
	store := NewInMemoryEventStore()
	ctx := context.Background()
	c1, _ := store.NextLogicalClock(ctx)
	c2, _ := store.NextLogicalClock(ctx)
	if c2 <= c1 {
		t.Fatalf("logical clock must be strictly monotonic: c1=%d c2=%d", c1, c2)
	}
}

func TestInMemoryEventStore_LoadByAggregate(t *testing.T) {
	store := NewInMemoryEventStore()
	ctx := context.Background()
	c1, _ := store.NextLogicalClock(ctx)
	e1 := NewPlatformEvent("e1", EventTypeExecutionStarted, AggregateTypeExecution,
		"agg-A", "exec-1", "t1", c1, "", "", nil, nil)
	c2, _ := store.NextLogicalClock(ctx)
	e2 := NewPlatformEvent("e2", EventTypeExecutionStarted, AggregateTypeExecution,
		"agg-B", "exec-1", "t1", c2, "", "", nil, nil)
	_ = store.Append(ctx, "s1", e1)
	_ = store.Append(ctx, "s1", e2)
	results, err := store.LoadByAggregate(ctx, "t1", "execution", "agg-A")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].AggregateID != "agg-A" {
		t.Fatalf("unexpected results: %+v", results)
	}
}
