package executor

import (
	"testing"
)

func TestExecutor_Prepare_ValidSnapshot(t *testing.T) {
	snap := newCreateSnapshot(t)
	e := newTestExecutor(&SucceedingProviderExecutor{})
	rt, err := e.Prepare("exec-1", "holder-1", snap)
	if err != nil {
		t.Fatalf("Prepare must succeed with valid snapshot: %v", err)
	}
	if rt == nil {
		t.Fatal("Prepare must return non-nil runtime")
	}
	if rt.State() != StatePending {
		t.Fatalf("initial state must be %s, got %s", StatePending, rt.State())
	}
}

func TestExecutor_Prepare_NilSnapshot_Error(t *testing.T) {
	e := newTestExecutor(&NoOpProviderExecutor{})
	_, err := e.Prepare("exec-1", "holder-1", nil)
	if err == nil {
		t.Fatal("Prepare with nil snapshot must return error")
	}
}

func TestExecutor_Prepare_TamperedSnapshot_Error(t *testing.T) {
	snap := newCreateSnapshot(t)
	snap.SchemaVersion = "tampered" // invalidates ExportHash
	e := newTestExecutor(&NoOpProviderExecutor{})
	_, err := e.Prepare("exec-1", "holder-1", snap)
	if err == nil {
		t.Fatal("Prepare with tampered snapshot must return error")
	}
}

func TestExecutor_RecordApproval_Valid(t *testing.T) {
	e := newTestExecutor(&NoOpProviderExecutor{})
	d := ApprovalDecision{
		ExecutionID:  "exec-1",
		GateID:       "gate-1",
		StageID:      "stage-0",
		OperatorID:   "op-1",
		OperatorRole: "admin",
		Reason:       "looks good",
	}
	if err := e.RecordApproval(d); err != nil {
		t.Fatalf("RecordApproval must succeed: %v", err)
	}
}

func TestExecutor_RecordApproval_MissingFields_Error(t *testing.T) {
	e := newTestExecutor(&NoOpProviderExecutor{})
	d := ApprovalDecision{ExecutionID: "exec-1"} // missing GateID, OperatorID, OperatorRole
	if err := e.RecordApproval(d); err == nil {
		t.Fatal("RecordApproval with missing fields must return error")
	}
}

func TestExecutor_RecordRejection_SetsApprovedFalse(t *testing.T) {
	_, a, _, _ := newTestStores()
	e := NewExecutor(NewInMemoryJournalStore(), a, NewInMemoryAmbiguityStore(), NewInMemoryLeaseStore(), &NoOpProviderExecutor{})
	d := ApprovalDecision{
		ExecutionID:  "exec-1",
		GateID:       "gate-1",
		OperatorID:   "op-1",
		OperatorRole: "admin",
	}
	if err := e.RecordRejection(d); err != nil {
		t.Fatalf("RecordRejection must succeed: %v", err)
	}
	decision, _ := a.GetDecision("exec-1", "gate-1")
	if decision == nil {
		t.Fatal("rejection must be persisted")
	}
	if decision.Approved {
		t.Fatal("rejection must set Approved=false")
	}
}

func TestExecutor_ResolveAmbiguity_Valid(t *testing.T) {
	_, _, amb, _ := newTestStores()
	e := NewExecutor(NewInMemoryJournalStore(), NewInMemoryApprovalStore(), amb, NewInMemoryLeaseStore(), &NoOpProviderExecutor{})
	record := NewAmbiguityRecord("exec-1", "stage-0", "svc-a", "timeout", "provider timed out")
	if err := amb.Record(record); err != nil {
		t.Fatalf("setup: Record ambiguity failed: %v", err)
	}
	if err := e.ResolveAmbiguity(record.AmbiguityID, "op-1", "confirmed safe"); err != nil {
		t.Fatalf("ResolveAmbiguity must succeed: %v", err)
	}
	got, _ := amb.Get(record.AmbiguityID)
	if !got.Resolved {
		t.Fatal("ambiguity must be marked resolved after ResolveAmbiguity")
	}
}

func TestValidatePauseRequest_Valid(t *testing.T) {
	req := PauseRequest{ExecutionID: "exec-1", OperatorID: "op-1", Reason: "maintenance"}
	if err := ValidatePauseRequest(req); err != nil {
		t.Fatalf("valid pause request must not error: %v", err)
	}
}

func TestValidatePauseRequest_MissingReason_Error(t *testing.T) {
	req := PauseRequest{ExecutionID: "exec-1", OperatorID: "op-1"}
	if err := ValidatePauseRequest(req); err == nil {
		t.Fatal("pause request without reason must error")
	}
}

func TestValidateResumeRequest_Valid(t *testing.T) {
	req := ResumeRequest{ExecutionID: "exec-1", OperatorID: "op-1"}
	if err := ValidateResumeRequest(req); err != nil {
		t.Fatalf("valid resume request must not error: %v", err)
	}
}

func TestValidateResumeRequest_MissingOperator_Error(t *testing.T) {
	req := ResumeRequest{ExecutionID: "exec-1"}
	if err := ValidateResumeRequest(req); err == nil {
		t.Fatal("resume request without operatorID must error")
	}
}
