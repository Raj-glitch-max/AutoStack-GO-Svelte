package executor

import (
	"sync"
	"testing"

	"github.com/janlauber/one-click/pkg/orchestrator"
	"github.com/janlauber/one-click/pkg/planner"
)

func newJournalEntry(executionID, stageID, nodeID string) orchestrator.ExecutionJournalEntry {
	e := orchestrator.NewJournalEntry(executionID, "", "", stageID, nodeID, planner.ActionCreate, "test")
	return *e
}

func TestJournalStore_Append_Valid(t *testing.T) {
	s := NewInMemoryJournalStore()
	e := newJournalEntry("exec-1", "stage-0", "svc-a")
	if err := s.Append(e); err != nil {
		t.Fatalf("Append must succeed: %v", err)
	}
}

func TestJournalStore_Append_EmptyExecutionID_Error(t *testing.T) {
	s := NewInMemoryJournalStore()
	e := newJournalEntry("", "stage-0", "svc-a")
	if err := s.Append(e); err == nil {
		t.Fatal("Append with empty ExecutionID must error")
	}
}

func TestJournalStore_List_Empty(t *testing.T) {
	s := NewInMemoryJournalStore()
	entries, err := s.List("exec-unknown")
	if err != nil {
		t.Fatalf("List must not error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("empty store must return 0 entries, got %d", len(entries))
	}
}

func TestJournalStore_List_ReturnsAllEntries(t *testing.T) {
	s := NewInMemoryJournalStore()
	s.Append(newJournalEntry("exec-1", "stage-0", "svc-a"))
	s.Append(newJournalEntry("exec-1", "stage-0", "svc-b"))
	s.Append(newJournalEntry("exec-2", "stage-0", "svc-a")) // different execution
	entries, _ := s.List("exec-1")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for exec-1, got %d", len(entries))
	}
}

func TestJournalStore_List_OrderedByTimestamp(t *testing.T) {
	s := NewInMemoryJournalStore()
	e1 := newJournalEntry("exec-1", "stage-0", "svc-a")
	e1.Timestamp = "2024-01-01T00:00:01Z"
	e2 := newJournalEntry("exec-1", "stage-0", "svc-b")
	e2.Timestamp = "2024-01-01T00:00:02Z"
	s.Append(e2) // append out of order
	s.Append(e1)
	entries, _ := s.List("exec-1")
	if entries[0].NodeID != "svc-a" {
		t.Fatal("entries must be sorted by Timestamp ascending")
	}
}

func TestJournalStore_AppendTransition_Valid(t *testing.T) {
	s := NewInMemoryJournalStore()
	tr := ExecutionTransition{
		ExecutionID: "exec-1",
		FromState:   StatePending,
		ToState:     StateReady,
		Reason:      "started",
		Timestamp:   "2024-01-01T00:00:00Z",
		ActorID:     "op-1",
	}
	if err := s.AppendTransition(tr); err != nil {
		t.Fatalf("AppendTransition must succeed: %v", err)
	}
}

func TestJournalStore_AppendTransition_EmptyExecutionID_Error(t *testing.T) {
	s := NewInMemoryJournalStore()
	tr := ExecutionTransition{ExecutionID: ""}
	if err := s.AppendTransition(tr); err == nil {
		t.Fatal("AppendTransition with empty ExecutionID must error")
	}
}

func TestJournalStore_ListTransitions_Ordered(t *testing.T) {
	s := NewInMemoryJournalStore()
	t1 := ExecutionTransition{ExecutionID: "exec-1", FromState: StatePending, ToState: StateReady, Timestamp: "2024-01-01T00:00:02Z"}
	t2 := ExecutionTransition{ExecutionID: "exec-1", FromState: StateReady, ToState: StateExecuting, Timestamp: "2024-01-01T00:00:01Z"}
	s.AppendTransition(t1)
	s.AppendTransition(t2)
	transitions, _ := s.ListTransitions("exec-1")
	if len(transitions) != 2 {
		t.Fatalf("expected 2 transitions, got %d", len(transitions))
	}
	if transitions[0].Timestamp > transitions[1].Timestamp {
		t.Fatal("transitions must be sorted by Timestamp ascending")
	}
}

func TestJournalStore_ListTransitions_Empty(t *testing.T) {
	s := NewInMemoryJournalStore()
	transitions, err := s.ListTransitions("exec-unknown")
	if err != nil {
		t.Fatalf("ListTransitions must not error: %v", err)
	}
	if len(transitions) != 0 {
		t.Fatalf("expected 0 transitions, got %d", len(transitions))
	}
}

func TestJournalStore_ConcurrentAppend_Safe(t *testing.T) {
	s := NewInMemoryJournalStore()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Append(newJournalEntry("exec-c", "stage-0", "svc-a"))
		}()
	}
	wg.Wait()
	entries, _ := s.List("exec-c")
	if len(entries) != 50 {
		t.Fatalf("concurrent appends must all succeed: expected 50, got %d", len(entries))
	}
}
