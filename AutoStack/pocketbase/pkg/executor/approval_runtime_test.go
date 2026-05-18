package executor

import (
	"testing"
)

func TestApprovalStore_RecordDecision_Valid(t *testing.T) {
	s := NewInMemoryApprovalStore()
	d := ApprovalDecision{
		ExecutionID:  "exec-1",
		GateID:       "gate-1",
		OperatorID:   "op-1",
		OperatorRole: "admin",
		Approved:     true,
	}
	if err := s.RecordDecision(d); err != nil {
		t.Fatalf("RecordDecision must succeed: %v", err)
	}
}

func TestApprovalStore_RecordDecision_EmptyExecutionID_Error(t *testing.T) {
	s := NewInMemoryApprovalStore()
	d := ApprovalDecision{GateID: "gate-1", OperatorID: "op-1"}
	if err := s.RecordDecision(d); err == nil {
		t.Fatal("RecordDecision with empty ExecutionID must error")
	}
}

func TestApprovalStore_RecordDecision_EmptyOperatorID_Error(t *testing.T) {
	s := NewInMemoryApprovalStore()
	d := ApprovalDecision{ExecutionID: "exec-1", GateID: "gate-1"}
	if err := s.RecordDecision(d); err == nil {
		t.Fatal("RecordDecision with empty OperatorID must error")
	}
}

func TestApprovalStore_GetDecision_NotFound_ReturnsNil(t *testing.T) {
	s := NewInMemoryApprovalStore()
	d, err := s.GetDecision("exec-1", "gate-unknown")
	if err != nil {
		t.Fatalf("GetDecision must not error: %v", err)
	}
	if d != nil {
		t.Fatal("undecided gate must return nil")
	}
}

func TestApprovalStore_GetDecision_Found(t *testing.T) {
	s := NewInMemoryApprovalStore()
	d := ApprovalDecision{ExecutionID: "exec-1", GateID: "gate-1", OperatorID: "op-1", Approved: true}
	s.RecordDecision(d)
	got, err := s.GetDecision("exec-1", "gate-1")
	if err != nil || got == nil {
		t.Fatal("GetDecision must return the recorded decision")
	}
	if !got.Approved {
		t.Fatal("GetDecision must return Approved=true")
	}
}

func TestApprovalStore_ListDecisions_OrderedByGateID(t *testing.T) {
	s := NewInMemoryApprovalStore()
	s.RecordDecision(ApprovalDecision{ExecutionID: "exec-1", GateID: "gate-b", OperatorID: "op-1"})
	s.RecordDecision(ApprovalDecision{ExecutionID: "exec-1", GateID: "gate-a", OperatorID: "op-1"})
	decisions, _ := s.ListDecisions("exec-1")
	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(decisions))
	}
	if decisions[0].GateID != "gate-a" {
		t.Fatal("decisions must be sorted by GateID ascending")
	}
}

func TestIsGateApproved_Approved(t *testing.T) {
	s := NewInMemoryApprovalStore()
	s.RecordDecision(ApprovalDecision{ExecutionID: "exec-1", GateID: "gate-1", OperatorID: "op-1", Approved: true})
	ok, err := IsGateApproved(s, "exec-1", "gate-1")
	if err != nil || !ok {
		t.Fatal("approved gate must return true")
	}
}

func TestIsGateApproved_NotDecided(t *testing.T) {
	s := NewInMemoryApprovalStore()
	ok, err := IsGateApproved(s, "exec-1", "gate-1")
	if err != nil || ok {
		t.Fatal("undecided gate must return false")
	}
}

func TestIsGateApproved_Rejected(t *testing.T) {
	s := NewInMemoryApprovalStore()
	s.RecordDecision(ApprovalDecision{ExecutionID: "exec-1", GateID: "gate-1", OperatorID: "op-1", Approved: false})
	ok, err := IsGateApproved(s, "exec-1", "gate-1")
	if err != nil || ok {
		t.Fatal("rejected gate must return false")
	}
}

func TestAllGatesApproved_AllApproved(t *testing.T) {
	s := NewInMemoryApprovalStore()
	s.RecordDecision(ApprovalDecision{ExecutionID: "exec-1", GateID: "g1", OperatorID: "op-1", Approved: true})
	s.RecordDecision(ApprovalDecision{ExecutionID: "exec-1", GateID: "g2", OperatorID: "op-1", Approved: true})
	ok, err := AllGatesApproved(s, "exec-1", []string{"g1", "g2"})
	if err != nil || !ok {
		t.Fatal("all approved gates must return true")
	}
}

func TestAllGatesApproved_SomeNotApproved(t *testing.T) {
	s := NewInMemoryApprovalStore()
	s.RecordDecision(ApprovalDecision{ExecutionID: "exec-1", GateID: "g1", OperatorID: "op-1", Approved: true})
	// g2 not approved
	ok, _ := AllGatesApproved(s, "exec-1", []string{"g1", "g2"})
	if ok {
		t.Fatal("partially approved gates must return false")
	}
}

func TestAllGatesApproved_EmptyList_ReturnsTrue(t *testing.T) {
	s := NewInMemoryApprovalStore()
	ok, err := AllGatesApproved(s, "exec-1", nil)
	if err != nil || !ok {
		t.Fatal("empty gate list must return true (no gates to satisfy)")
	}
}
