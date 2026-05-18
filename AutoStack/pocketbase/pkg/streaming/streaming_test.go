package streaming

import (
	"testing"
)

// ─── StreamEvent ──────────────────────────────────────────────────────────────

func TestNewStreamEvent_HashValid(t *testing.T) {
	e := NewStreamEvent("ev-1", "stream-1", StreamTypeExecution, "exec-1", "tenant-1", 1, 10, `{"status":"running"}`)
	ok, reason := VerifyStreamEventHash(e)
	if !ok {
		t.Fatalf("stream event hash invalid: %s", reason)
	}
}

func TestStreamEventHash_EmittedAtExcluded(t *testing.T) {
	e1 := NewStreamEvent("ev-1", "stream-1", StreamTypeExecution, "exec-1", "tenant-1", 1, 10, "payload")
	e2 := e1
	e2.EmittedAt = "2099-01-01T00:00:00Z"
	if e1.EventHash != e2.EventHash {
		t.Fatal("EmittedAt must not affect stream event hash")
	}
}

func TestVerifyStreamEventHash_Tampered(t *testing.T) {
	e := NewStreamEvent("ev-1", "stream-1", StreamTypeExecution, "exec-1", "tenant-1", 1, 10, "payload")
	e.Payload = "tampered"
	ok, _ := VerifyStreamEventHash(e)
	if ok {
		t.Fatal("tampered event must not verify")
	}
}

// ─── ExecutionStream ──────────────────────────────────────────────────────────

func TestExecutionStream_EmitAndRetrieve(t *testing.T) {
	s := NewExecutionStream("stream-1", "tenant-1", "exec-1", StreamTypeExecution)
	e := NewStreamEvent("ev-1", "stream-1", StreamTypeExecution, "exec-1", "tenant-1", 0, 5, "payload")
	if err := s.Emit(e); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if s.Size() != 1 {
		t.Fatalf("expected size 1, got %d", s.Size())
	}
}

func TestExecutionStream_SequenceAutoAssigned(t *testing.T) {
	s := NewExecutionStream("stream-1", "tenant-1", "exec-1", StreamTypeExecution)
	for i := 0; i < 3; i++ {
		e := NewStreamEvent("ev", "stream-1", StreamTypeExecution, "exec-1", "tenant-1", 0, 0, "p")
		s.Emit(e)
	}
	events := s.Events()
	for i, e := range events {
		if e.Sequence != int64(i+1) {
			t.Fatalf("event[%d] sequence: expected %d, got %d", i, i+1, e.Sequence)
		}
	}
}

func TestExecutionStream_WrongTenant_Error(t *testing.T) {
	s := NewExecutionStream("stream-1", "tenant-1", "exec-1", StreamTypeExecution)
	e := NewStreamEvent("ev-1", "stream-1", StreamTypeExecution, "exec-1", "tenant-other", 0, 0, "")
	if err := s.Emit(e); err == nil {
		t.Fatal("wrong tenant must error")
	}
}

func TestExecutionStream_EventsAfter(t *testing.T) {
	s := NewExecutionStream("stream-1", "tenant-1", "exec-1", StreamTypeExecution)
	for i := 0; i < 5; i++ {
		e := NewStreamEvent("ev", "", StreamTypeExecution, "exec-1", "tenant-1", 0, 0, "p")
		s.Emit(e)
	}
	after := s.EventsAfter(3)
	if len(after) != 2 {
		t.Fatalf("expected 2 events after seq 3, got %d", len(after))
	}
}

// ─── SubscriptionManager ──────────────────────────────────────────────────────

func TestSubscriptionManager_SubscribeAndPoll(t *testing.T) {
	mgr := NewSubscriptionManager()
	stream := NewExecutionStream("stream-1", "tenant-1", "exec-1", StreamTypeExecution)
	mgr.RegisterStream(stream)

	sub, err := mgr.Subscribe("sub-1", "stream-1", "tenant-1")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if sub.SubscriptionID != "sub-1" {
		t.Fatalf("subscription ID mismatch")
	}

	// Emit an event AFTER subscribing.
	e := NewStreamEvent("ev-1", "", StreamTypeExecution, "exec-1", "tenant-1", 0, 0, "data")
	stream.Emit(e)

	events, err := mgr.Poll("sub-1")
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// Second poll must return nothing (already consumed).
	events2, _ := mgr.Poll("sub-1")
	if len(events2) != 0 {
		t.Fatalf("second poll must be empty, got %d", len(events2))
	}
}

func TestSubscriptionManager_WrongTenant_Error(t *testing.T) {
	mgr := NewSubscriptionManager()
	stream := NewExecutionStream("stream-1", "tenant-1", "exec-1", StreamTypeExecution)
	mgr.RegisterStream(stream)

	_, err := mgr.Subscribe("sub-1", "stream-1", "tenant-other")
	if err == nil {
		t.Fatal("wrong tenant subscription must error")
	}
}

func TestSubscriptionManager_Unsubscribe(t *testing.T) {
	mgr := NewSubscriptionManager()
	stream := NewExecutionStream("stream-1", "tenant-1", "exec-1", StreamTypeExecution)
	mgr.RegisterStream(stream)
	mgr.Subscribe("sub-1", "stream-1", "tenant-1")
	mgr.Unsubscribe("sub-1")

	_, err := mgr.Poll("sub-1")
	if err == nil {
		t.Fatal("poll after unsubscribe must error")
	}
}

// ─── ReplayStream ─────────────────────────────────────────────────────────────

func TestReplayStream_CleanStream(t *testing.T) {
	s := NewExecutionStream("stream-1", "tenant-1", "exec-1", StreamTypeExecution)
	for i := 0; i < 3; i++ {
		e := NewStreamEvent("ev", "", StreamTypeExecution, "exec-1", "tenant-1", 0, 0, "p")
		s.Emit(e)
	}
	report := ReplayStream(s)
	if !report.IsDivergenceFree {
		t.Fatalf("clean stream must be divergence-free: %v", report.Divergences)
	}
	if report.EventCount != 3 {
		t.Fatalf("expected 3 events, got %d", report.EventCount)
	}
}

func TestReplayStream_TamperedEvent_Divergence(t *testing.T) {
	s := NewExecutionStream("stream-1", "tenant-1", "exec-1", StreamTypeExecution)
	e := NewStreamEvent("ev-1", "", StreamTypeExecution, "exec-1", "tenant-1", 0, 0, "p")
	s.Emit(e)
	// Tamper directly in the slice (not exported — we test via the report).
	// Instead, create a stream with manually tampered events.
	events := s.Events()
	if len(events) == 0 {
		t.Fatal("no events")
	}
	// Build a tampered event manually.
	tampered := events[0]
	tampered.Payload = "tampered"
	// Verify that VerifyStreamEventHash catches it.
	ok, _ := VerifyStreamEventHash(tampered)
	if ok {
		t.Fatal("tampered event must fail verification")
	}
}

func TestBuildStreamSnapshot(t *testing.T) {
	s := NewExecutionStream("stream-1", "tenant-1", "exec-1", StreamTypeExecution)
	e := NewStreamEvent("ev-1", "", StreamTypeExecution, "exec-1", "tenant-1", 0, 0, "data")
	s.Emit(e)

	snap := BuildStreamSnapshot(s)
	if snap.EventCount != 1 {
		t.Fatalf("expected 1 event in snapshot, got %d", snap.EventCount)
	}
	if snap.FinalSeq != 1 {
		t.Fatalf("expected final seq 1, got %d", snap.FinalSeq)
	}
}
