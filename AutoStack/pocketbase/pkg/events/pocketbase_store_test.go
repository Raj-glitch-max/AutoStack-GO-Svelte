package events

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── PocketBaseEventStore ──────────────────────────────────────────────────

func makeTestStore() *PocketBaseEventStore {
	return NewPocketBaseEventStore(NewInMemoryRecordPersistence())
}

func makeTestEvent(t *testing.T, id, execID, tenantID string, clock int64) PlatformEvent {
	t.Helper()
	return NewPlatformEvent(
		id, EventTypeExecutionStarted, AggregateTypeExecution,
		"agg-1", execID, tenantID, clock, "", "", nil, nil,
	)
}

func TestPocketBaseEventStore_AppendAndLoad(t *testing.T) {
	s := makeTestStore()
	evt := makeTestEvent(t, "e1", "exec-1", "tenant-1", 1)
	if err := s.Append(context.Background(), "stream-1", evt); err != nil {
		t.Fatalf("Append must not error: %v", err)
	}
	stream, err := s.Load(context.Background(), "stream-1")
	if err != nil {
		t.Fatalf("Load must not error: %v", err)
	}
	if len(stream.Events) != 1 {
		t.Fatalf("stream must have 1 event, got %d", len(stream.Events))
	}
	if stream.Events[0].EventID != "e1" {
		t.Fatalf("event ID mismatch: %s", stream.Events[0].EventID)
	}
}

func TestPocketBaseEventStore_MultipleEvents(t *testing.T) {
	s := makeTestStore()
	for i := int64(1); i <= 5; i++ {
		evt := makeTestEvent(t, string(rune('a'+i-1)), "exec-1", "t1", i)
		if err := s.Append(context.Background(), "stream-1", evt); err != nil {
			t.Fatalf("Append event %d: %v", i, err)
		}
	}
	stream, err := s.Load(context.Background(), "stream-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(stream.Events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(stream.Events))
	}
}

func TestPocketBaseEventStore_RejectDuplicateEventID(t *testing.T) {
	s := makeTestStore()
	evt := makeTestEvent(t, "e1", "exec-1", "t1", 1)
	if err := s.Append(context.Background(), "stream-1", evt); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := s.Append(context.Background(), "stream-1", evt); err == nil {
		t.Fatal("second append with same ID must error")
	} else if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error must mention 'duplicate', got: %v", err)
	}
}

func TestPocketBaseEventStore_RejectInvalidHash(t *testing.T) {
	s := makeTestStore()
	evt := makeTestEvent(t, "e1", "exec-1", "t1", 1)
	evt.Hash = "invalid-hash"
	if err := s.Append(context.Background(), "stream-1", evt); err == nil {
		t.Fatal("append with invalid hash must error")
	}
}

func TestPocketBaseEventStore_RejectClockRegression(t *testing.T) {
	s := makeTestStore()
	e1 := makeTestEvent(t, "e1", "exec-1", "t1", 5)
	if err := s.Append(context.Background(), "stream-1", e1); err != nil {
		t.Fatalf("first append: %v", err)
	}
	e2 := makeTestEvent(t, "e2", "exec-1", "t1", 2) // clock 2 < last clock 5
	if err := s.Append(context.Background(), "stream-1", e2); err == nil {
		t.Fatal("clock regression must be rejected")
	}
}

func TestPocketBaseEventStore_LoadNotFound(t *testing.T) {
	s := makeTestStore()
	if _, err := s.Load(context.Background(), "no-such-stream"); err == nil {
		t.Fatal("Load on missing stream must error")
	}
}

func TestPocketBaseEventStore_LoadRange(t *testing.T) {
	s := makeTestStore()
	for i := int64(1); i <= 5; i++ {
		evt := makeTestEvent(t, string(rune('a'+i-1)), "exec-1", "t1", i)
		_ = s.Append(context.Background(), "stream-1", evt)
	}
	evts, err := s.LoadRange(context.Background(), "stream-1", 2, 4)
	if err != nil {
		t.Fatalf("LoadRange: %v", err)
	}
	if len(evts) != 3 {
		t.Fatalf("expected 3 events in [2,4], got %d", len(evts))
	}
	for _, e := range evts {
		if e.LogicalClock < 2 || e.LogicalClock > 4 {
			t.Fatalf("clock %d outside range [2,4]", e.LogicalClock)
		}
	}
}

func TestPocketBaseEventStore_LoadByAggregate(t *testing.T) {
	s := makeTestStore()
	e1 := makeTestEvent(t, "e1", "exec-1", "t1", 1)
	e2 := NewPlatformEvent("e2", EventTypeStageCompleted, AggregateTypeStage,
		"stage-x", "exec-1", "t1", 2, "", "", nil, nil)
	_ = s.Append(context.Background(), "stream-1", e1)
	_ = s.Append(context.Background(), "stream-1", e2)

	results, err := s.LoadByAggregate(context.Background(), "t1", "execution", "agg-1")
	if err != nil {
		t.Fatalf("LoadByAggregate: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 execution event, got %d", len(results))
	}
	if results[0].EventID != "e1" {
		t.Fatalf("wrong event returned: %s", results[0].EventID)
	}
}

func TestPocketBaseEventStore_NextLogicalClock_Monotonic(t *testing.T) {
	s := makeTestStore()
	ctx := context.Background()
	prev := int64(0)
	for i := 0; i < 10; i++ {
		c, err := s.NextLogicalClock(ctx)
		if err != nil {
			t.Fatalf("NextLogicalClock: %v", err)
		}
		if c <= prev {
			t.Fatalf("clock must be strictly monotonic: got %d after %d", c, prev)
		}
		prev = c
	}
}

func TestPocketBaseEventStore_StreamHashReproducible(t *testing.T) {
	// Two stores with the same events must produce the same StreamHash.
	s1 := makeTestStore()
	s2 := makeTestStore()
	for i := int64(1); i <= 3; i++ {
		evt := makeTestEvent(t, string(rune('a'+i-1)), "exec-1", "t1", i)
		_ = s1.Append(context.Background(), "s", evt)
		_ = s2.Append(context.Background(), "s", evt)
	}
	st1, _ := s1.Load(context.Background(), "s")
	st2, _ := s2.Load(context.Background(), "s")
	if st1.StreamHash != st2.StreamHash {
		t.Fatal("same events must produce same StreamHash")
	}
}

func TestPocketBaseEventStore_EmptyStreamID(t *testing.T) {
	s := makeTestStore()
	evt := makeTestEvent(t, "e1", "exec-1", "t1", 1)
	if err := s.Append(context.Background(), "", evt); err == nil {
		t.Fatal("empty streamID must be rejected")
	}
}

// ─── InMemoryRecordPersistence ─────────────────────────────────────────────

func TestInMemoryRecordPersistence_AppendAndFind(t *testing.T) {
	p := NewInMemoryRecordPersistence()
	r := EventStorageRecord{
		StreamID: "s1", EventID: "e1", TenantID: "t1", LogicalClock: 1,
		Body: []byte(`{"event_id":"e1"}`),
	}
	if err := p.AppendRecord(r); err != nil {
		t.Fatalf("AppendRecord: %v", err)
	}
	found, ok, err := p.FindRecord("e1")
	if err != nil || !ok {
		t.Fatalf("FindRecord must find e1: %v %v", ok, err)
	}
	if found.EventID != "e1" {
		t.Fatal("found wrong record")
	}
}

func TestInMemoryRecordPersistence_DuplicateRejected(t *testing.T) {
	p := NewInMemoryRecordPersistence()
	r := EventStorageRecord{StreamID: "s1", EventID: "e1", TenantID: "t1"}
	_ = p.AppendRecord(r)
	if err := p.AppendRecord(r); err == nil {
		t.Fatal("duplicate event must be rejected")
	}
}

func TestInMemoryRecordPersistence_ClockMonotonic(t *testing.T) {
	p := NewInMemoryRecordPersistence()
	prev := int64(0)
	for i := 0; i < 5; i++ {
		c, err := p.NextClock()
		if err != nil || c <= prev {
			t.Fatalf("clock must be monotonic: got %d after %d", c, prev)
		}
		prev = c
	}
}

// ─── LogicalClockAllocator ─────────────────────────────────────────────────

func TestLogicalClockAllocator_Monotonic(t *testing.T) {
	dir := t.TempDir()
	alloc, err := NewLogicalClockAllocator(filepath.Join(dir, "clock"))
	if err != nil {
		t.Fatalf("NewLogicalClockAllocator: %v", err)
	}
	prev := int64(0)
	for i := 0; i < 10; i++ {
		c, err := alloc.Next()
		if err != nil {
			t.Fatalf("Next(): %v", err)
		}
		if c <= prev {
			t.Fatalf("clock must be monotonic: got %d after %d", c, prev)
		}
		prev = c
	}
}

func TestLogicalClockAllocator_PersistAcrossReopens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clock")
	alloc1, _ := NewLogicalClockAllocator(path)
	for i := 0; i < 5; i++ {
		_, _ = alloc1.Next()
	}
	last := alloc1.Current()

	alloc2, err := NewLogicalClockAllocator(path)
	if err != nil {
		t.Fatalf("reopen allocator: %v", err)
	}
	if alloc2.Current() != last {
		t.Fatalf("reopened clock must resume from %d, got %d", last, alloc2.Current())
	}
	next, _ := alloc2.Next()
	if next != last+1 {
		t.Fatalf("next after reopen must be %d, got %d", last+1, next)
	}
}

func TestLogicalClockAllocator_StartsAtZero(t *testing.T) {
	dir := t.TempDir()
	alloc, _ := NewLogicalClockAllocator(filepath.Join(dir, "clock"))
	if alloc.Current() != 0 {
		t.Fatal("fresh allocator must start at 0")
	}
	c, _ := alloc.Next()
	if c != 1 {
		t.Fatalf("first Next() must return 1, got %d", c)
	}
}

func TestLogicalClockAllocator_BadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clock")
	_ = os.WriteFile(path, []byte("not-a-number"), 0600)
	if _, err := NewLogicalClockAllocator(path); err == nil {
		t.Fatal("corrupt clock file must error on open")
	}
}
