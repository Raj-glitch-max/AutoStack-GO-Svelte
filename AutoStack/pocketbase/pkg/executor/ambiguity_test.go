package executor

import (
	"testing"
)

func TestAmbiguityStore_Record_Valid(t *testing.T) {
	s := NewInMemoryAmbiguityStore()
	a := NewAmbiguityRecord("exec-1", "stage-0", "svc-a", "timeout", "provider timed out")
	if err := s.Record(a); err != nil {
		t.Fatalf("Record must succeed: %v", err)
	}
}

func TestAmbiguityStore_Record_EmptyID_Error(t *testing.T) {
	s := NewInMemoryAmbiguityStore()
	a := ExecutionAmbiguity{AmbiguityID: "", ExecutionID: "exec-1"}
	if err := s.Record(a); err == nil {
		t.Fatal("Record with empty AmbiguityID must error")
	}
}

func TestAmbiguityStore_Record_EmptyExecutionID_Error(t *testing.T) {
	s := NewInMemoryAmbiguityStore()
	a := ExecutionAmbiguity{AmbiguityID: "amb-1", ExecutionID: ""}
	if err := s.Record(a); err == nil {
		t.Fatal("Record with empty ExecutionID must error")
	}
}

func TestAmbiguityStore_Get_NotFound_ReturnsNil(t *testing.T) {
	s := NewInMemoryAmbiguityStore()
	a, err := s.Get("nonexistent")
	if err != nil || a != nil {
		t.Fatal("Get for unknown ID must return nil without error")
	}
}

func TestAmbiguityStore_Get_Found(t *testing.T) {
	s := NewInMemoryAmbiguityStore()
	orig := NewAmbiguityRecord("exec-1", "stage-0", "svc-a", "timeout", "desc")
	s.Record(orig)
	got, err := s.Get(orig.AmbiguityID)
	if err != nil || got == nil {
		t.Fatal("Get must return the recorded ambiguity")
	}
	if got.Kind != "timeout" {
		t.Fatalf("ambiguity kind mismatch: want timeout, got %s", got.Kind)
	}
}

func TestAmbiguityStore_ListUnresolved_Empty(t *testing.T) {
	s := NewInMemoryAmbiguityStore()
	list, err := s.ListUnresolved("exec-1")
	if err != nil || len(list) != 0 {
		t.Fatal("ListUnresolved must return empty slice for unknown execution")
	}
}

func TestAmbiguityStore_ListUnresolved_FiltersResolved(t *testing.T) {
	s := NewInMemoryAmbiguityStore()
	a1 := NewAmbiguityRecord("exec-1", "stage-0", "svc-a", "timeout", "desc")
	a2 := NewAmbiguityRecord("exec-1", "stage-0", "svc-b", "partial", "desc")
	s.Record(a1)
	s.Record(a2)
	s.Resolve(a1.AmbiguityID, "op-1", "safe to continue")

	list, _ := s.ListUnresolved("exec-1")
	if len(list) != 1 {
		t.Fatalf("ListUnresolved must return only unresolved: expected 1, got %d", len(list))
	}
	if list[0].AmbiguityID != a2.AmbiguityID {
		t.Fatal("ListUnresolved must return the correct unresolved ambiguity")
	}
}

func TestAmbiguityStore_ListUnresolved_Ordered(t *testing.T) {
	s := NewInMemoryAmbiguityStore()
	// Inject two records with deterministic IDs that sort correctly.
	a1 := ExecutionAmbiguity{AmbiguityID: "amb-aaa", ExecutionID: "exec-1"}
	a2 := ExecutionAmbiguity{AmbiguityID: "amb-zzz", ExecutionID: "exec-1"}
	s.Record(a2)
	s.Record(a1)
	list, _ := s.ListUnresolved("exec-1")
	if list[0].AmbiguityID != "amb-aaa" {
		t.Fatal("ListUnresolved must be ordered by AmbiguityID ascending")
	}
}

func TestAmbiguityStore_Resolve_Valid(t *testing.T) {
	s := NewInMemoryAmbiguityStore()
	a := NewAmbiguityRecord("exec-1", "stage-0", "svc-a", "timeout", "desc")
	s.Record(a)
	if err := s.Resolve(a.AmbiguityID, "op-1", "confirmed safe"); err != nil {
		t.Fatalf("Resolve must succeed: %v", err)
	}
	got, _ := s.Get(a.AmbiguityID)
	if !got.Resolved {
		t.Fatal("ambiguity must be marked resolved")
	}
	if got.OperatorID != "op-1" {
		t.Fatalf("resolved OperatorID must match: want op-1, got %s", got.OperatorID)
	}
}

func TestAmbiguityStore_Resolve_AlreadyResolved_Error(t *testing.T) {
	s := NewInMemoryAmbiguityStore()
	a := NewAmbiguityRecord("exec-1", "stage-0", "svc-a", "timeout", "desc")
	s.Record(a)
	s.Resolve(a.AmbiguityID, "op-1", "first resolution")
	if err := s.Resolve(a.AmbiguityID, "op-2", "second resolution"); err == nil {
		t.Fatal("Resolve on already-resolved ambiguity must error")
	}
}

func TestAmbiguityStore_Resolve_EmptyOperator_Error(t *testing.T) {
	s := NewInMemoryAmbiguityStore()
	a := NewAmbiguityRecord("exec-1", "stage-0", "svc-a", "timeout", "desc")
	s.Record(a)
	if err := s.Resolve(a.AmbiguityID, "", "resolution"); err == nil {
		t.Fatal("Resolve with empty operatorID must error")
	}
}

func TestAmbiguityStore_Resolve_NotFound_Error(t *testing.T) {
	s := NewInMemoryAmbiguityStore()
	if err := s.Resolve("nonexistent", "op-1", "resolution"); err == nil {
		t.Fatal("Resolve for unknown AmbiguityID must error")
	}
}

func TestNewAmbiguityRecord_StableID(t *testing.T) {
	a1 := NewAmbiguityRecord("exec-1", "stage-0", "svc-a", "timeout", "desc1")
	a2 := NewAmbiguityRecord("exec-1", "stage-0", "svc-a", "timeout", "desc2")
	if a1.AmbiguityID != a2.AmbiguityID {
		t.Fatal("AmbiguityID must be stable given same executionID/stageID/nodeID/kind")
	}
}

func TestNewAmbiguityRecord_DifferentKind_DifferentID(t *testing.T) {
	a1 := NewAmbiguityRecord("exec-1", "stage-0", "svc-a", "timeout", "desc")
	a2 := NewAmbiguityRecord("exec-1", "stage-0", "svc-a", "partial", "desc")
	if a1.AmbiguityID == a2.AmbiguityID {
		t.Fatal("different Kind must produce different AmbiguityID")
	}
}
