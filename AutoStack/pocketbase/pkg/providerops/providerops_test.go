package providerops

import (
	"testing"
)

// ─── ProviderOperation ────────────────────────────────────────────────────────

func TestNewProviderOperation_HashValid(t *testing.T) {
	op := NewProviderOperation("op-1", "exec-1", "tenant-1", "aws-ecs", OperationTypeDeploy, 1)
	ok, reason := VerifyOperationHash(op)
	if !ok {
		t.Fatalf("hash must be valid: %s", reason)
	}
	if op.State != OperationStatePending {
		t.Fatalf("initial state must be pending, got %s", op.State)
	}
}

func TestVerifyOperationHash_Tampered(t *testing.T) {
	op := NewProviderOperation("op-1", "exec-1", "tenant-1", "aws-ecs", OperationTypeDeploy, 1)
	op.ProviderID = "tampered"
	ok, _ := VerifyOperationHash(op)
	if ok {
		t.Fatal("tampered operation must not verify")
	}
}

func TestTransitionOperation_ValueSemantics(t *testing.T) {
	op := NewProviderOperation("op-1", "exec-1", "tenant-1", "aws-ecs", OperationTypeDeploy, 1)
	updated := TransitionOperation(op, OperationStateExecuting)
	if op.State != OperationStatePending {
		t.Fatal("original must not be mutated")
	}
	if updated.State != OperationStateExecuting {
		t.Fatalf("expected executing, got %s", updated.State)
	}
	ok, reason := VerifyOperationHash(updated)
	if !ok {
		t.Fatalf("transitioned operation hash invalid: %s", reason)
	}
}

func TestIsTerminalOperation(t *testing.T) {
	terminal := []OperationState{
		OperationStateCompleted, OperationStateFailed,
		OperationStateContradicted, OperationStateRolledBack,
	}
	for _, s := range terminal {
		op := ProviderOperation{State: s}
		if !IsTerminalOperation(op) {
			t.Fatalf("state %s must be terminal", s)
		}
	}
	if IsTerminalOperation(ProviderOperation{State: OperationStatePending}) {
		t.Fatal("pending must not be terminal")
	}
}

// ─── NormalizedProviderResult ─────────────────────────────────────────────────

func TestNewNormalizedResult_HashValid(t *testing.T) {
	r := NewNormalizedResult("r-1", "op-1", "exec-1", "tenant-1", "aws-ecs",
		ResultStateSuccess, "arn:aws:ecs:us-east-1:123:task/abc", "raw-ref", "")
	ok, reason := VerifyResultHash(r)
	if !ok {
		t.Fatalf("result hash invalid: %s", reason)
	}
}

func TestIsSuccessful(t *testing.T) {
	ok := NewNormalizedResult("r-1", "op-1", "exec-1", "tenant-1", "aws", ResultStateSuccess, "", "", "")
	if !IsSuccessful(ok) {
		t.Fatal("success result must be successful")
	}
	fail := NewNormalizedResult("r-2", "op-1", "exec-1", "tenant-1", "aws", ResultStateFailed, "", "", "")
	if IsSuccessful(fail) {
		t.Fatal("failed result must not be successful")
	}
}

func TestIsAmbiguous(t *testing.T) {
	ambig := []ResultState{ResultStateContradictory, ResultStatePartial, ResultStateUnknown}
	for _, s := range ambig {
		r := NormalizedProviderResult{State: s}
		if !IsAmbiguous(r) {
			t.Fatalf("state %s must be ambiguous", s)
		}
	}
	if IsAmbiguous(NormalizedProviderResult{State: ResultStateSuccess}) {
		t.Fatal("success must not be ambiguous")
	}
}

func TestNormalizedResult_ObservedAtExcluded(t *testing.T) {
	r1 := NewNormalizedResult("r-1", "op-1", "exec-1", "tenant-1", "aws", ResultStateSuccess, "", "", "")
	r2 := r1
	r2.ObservedAt = "2099-01-01T00:00:00Z"
	if r1.ResultHash != r2.ResultHash {
		t.Fatal("ObservedAt must not affect result hash")
	}
}

// ─── ProviderOperationEnvelope ────────────────────────────────────────────────

func TestNewProviderOperationEnvelope_HashValid(t *testing.T) {
	op := NewProviderOperation("op-1", "exec-1", "tenant-1", "aws-ecs", OperationTypeDeploy, 1)
	result := NewNormalizedResult("r-1", "op-1", "exec-1", "tenant-1", "aws-ecs", ResultStateSuccess, "", "", "")
	env := NewProviderOperationEnvelope("env-1", op, result)
	ok, reason := VerifyEnvelopeHash(env)
	if !ok {
		t.Fatalf("envelope hash invalid: %s", reason)
	}
}

func TestEnvelope_EnvelopedAtExcluded(t *testing.T) {
	op := NewProviderOperation("op-1", "exec-1", "tenant-1", "aws-ecs", OperationTypeDeploy, 1)
	result := NewNormalizedResult("r-1", "op-1", "exec-1", "tenant-1", "aws-ecs", ResultStateSuccess, "", "", "")
	env1 := NewProviderOperationEnvelope("env-1", op, result)
	env2 := env1
	env2.EnvelopedAt = "2099-01-01T00:00:00Z"
	if env1.EnvelopeHash != env2.EnvelopeHash {
		t.Fatal("EnvelopedAt must not affect envelope hash")
	}
}

// ─── DetectProviderContradiction ──────────────────────────────────────────────

func TestDetectProviderContradiction_DeploySuccessResourceMissing(t *testing.T) {
	input := ContradictionInput{
		OperationID:    "op-1",
		OperationType:  OperationTypeDeploy,
		ReportedResult: ResultStateSuccess,
		ResourceExists: false,
	}
	c, err := DetectProviderContradiction("c-1", "exec-1", "tenant-1", "aws-ecs", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("contradiction must be detected")
	}
	if c.Type != ContradictionDeploySuccessResourceMissing {
		t.Fatalf("expected deploy_success_resource_missing, got %s", c.Type)
	}
	ok, reason := VerifyContradictionHash(*c)
	if !ok {
		t.Fatalf("contradiction hash invalid: %s", reason)
	}
}

func TestDetectProviderContradiction_DeleteSuccessResourceAlive(t *testing.T) {
	input := ContradictionInput{
		OperationID:    "op-1",
		OperationType:  OperationTypeDestroy,
		ReportedResult: ResultStateSuccess,
		ResourceExists: true,
	}
	c, err := DetectProviderContradiction("c-1", "exec-1", "tenant-1", "aws-ecs", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil || c.Type != ContradictionDeleteSuccessResourceAlive {
		t.Fatalf("expected delete_success_resource_alive, got %v", c)
	}
}

func TestDetectProviderContradiction_RollbackSuccessUnhealthy(t *testing.T) {
	input := ContradictionInput{
		OperationID:     "op-1",
		OperationType:   OperationTypeRollback,
		ReportedResult:  ResultStateSuccess,
		ResourceHealthy: false,
	}
	c, _ := DetectProviderContradiction("c-1", "exec-1", "tenant-1", "aws", input)
	if c == nil || c.Type != ContradictionRollbackSuccessUnhealthy {
		t.Fatal("rollback_success_unhealthy must be detected")
	}
}

func TestDetectProviderContradiction_ScaleCapacityUnchanged(t *testing.T) {
	input := ContradictionInput{
		OperationID:       "op-1",
		OperationType:     OperationTypeScale,
		ReportedResult:    ResultStateSuccess,
		RequestedCapacity: 5,
		ObservedCapacity:  0,
	}
	c, _ := DetectProviderContradiction("c-1", "exec-1", "tenant-1", "aws", input)
	if c == nil || c.Type != ContradictionScaleAcknowledgedCapacityUnchanged {
		t.Fatal("scale_acknowledged_capacity_unchanged must be detected")
	}
}

func TestDetectProviderContradiction_NoContradiction(t *testing.T) {
	input := ContradictionInput{
		OperationID:    "op-1",
		OperationType:  OperationTypeDeploy,
		ReportedResult: ResultStateSuccess,
		ResourceExists: true,
	}
	c, err := DetectProviderContradiction("c-1", "exec-1", "tenant-1", "aws-ecs", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c != nil {
		t.Fatal("no contradiction expected when signals are consistent")
	}
}

// ─── OperationRollbackRecord ──────────────────────────────────────────────────

func TestBuildRollbackRecord_HashValid(t *testing.T) {
	r := BuildRollbackRecord("rb-1", "op-1", "exec-1", "tenant-1", "aws-ecs", "contradiction detected", "c-1", "arn:resource")
	ok, reason := VerifyRollbackHash(r)
	if !ok {
		t.Fatalf("rollback hash invalid: %s", reason)
	}
}

func TestCompleteRollback_ValueSemantics(t *testing.T) {
	r := BuildRollbackRecord("rb-1", "op-1", "exec-1", "tenant-1", "aws-ecs", "reason", "", "")
	completed := CompleteRollback(r)
	if r.Completed {
		t.Fatal("original must not be mutated")
	}
	if !completed.Completed {
		t.Fatal("completed rollback must have Completed=true")
	}
	ok, reason := VerifyRollbackHash(completed)
	if !ok {
		t.Fatalf("completed rollback hash invalid: %s", reason)
	}
}

// ─── ProviderReconciliation ───────────────────────────────────────────────────

func TestReconcileProviderState_Consistent(t *testing.T) {
	r := ReconcileProviderState("recon-1", "op-1", "exec-1", "tenant-1", "aws-ecs", "running", "running", nil)
	if r.State != ReconciliationStateConsistent {
		t.Fatalf("expected consistent, got %s", r.State)
	}
	ok, reason := VerifyReconciliationHash(r)
	if !ok {
		t.Fatalf("reconciliation hash invalid: %s", reason)
	}
}

func TestReconcileProviderState_Inconsistent(t *testing.T) {
	r := ReconcileProviderState("recon-1", "op-1", "exec-1", "tenant-1", "aws-ecs", "running", "stopped", []string{"status"})
	if r.State != ReconciliationStateInconsistent {
		t.Fatalf("expected inconsistent, got %s", r.State)
	}
}

func TestReconcileProviderState_Unknown(t *testing.T) {
	r := ReconcileProviderState("recon-1", "op-1", "exec-1", "tenant-1", "aws-ecs", "running", "", nil)
	if r.State != ReconciliationStateUnknown {
		t.Fatalf("expected unknown, got %s", r.State)
	}
}

func TestReconciliationHash_ReconcileAtExcluded(t *testing.T) {
	r1 := ReconcileProviderState("recon-1", "op-1", "exec-1", "tenant-1", "aws", "running", "running", nil)
	r2 := r1
	r2.ReconcileAt = "2099-01-01T00:00:00Z"
	if r1.ReconciliationHash != r2.ReconciliationHash {
		t.Fatal("ReconcileAt must not affect reconciliation hash")
	}
}

// ─── ProviderOpsSnapshot ─────────────────────────────────────────────────────

func TestBuildProviderOpsSnapshot_HashValid(t *testing.T) {
	op := NewProviderOperation("op-1", "exec-1", "tenant-1", "aws-ecs", OperationTypeDeploy, 1)
	snap := BuildProviderOpsSnapshot("snap-1", "exec-1", "tenant-1",
		[]ProviderOperation{op}, nil, nil, nil)
	ok, reason := VerifyProviderOpsSnapshot(snap)
	if !ok {
		t.Fatalf("snapshot hash invalid: %s", reason)
	}
}

func TestBuildProviderOpsSnapshot_SnapshotAtExcluded(t *testing.T) {
	op := NewProviderOperation("op-1", "exec-1", "tenant-1", "aws-ecs", OperationTypeDeploy, 1)
	snap1 := BuildProviderOpsSnapshot("snap-1", "exec-1", "tenant-1",
		[]ProviderOperation{op}, nil, nil, nil)
	snap2 := snap1
	snap2.SnapshotAt = "2099-01-01T00:00:00Z"
	if snap1.SnapshotHash != snap2.SnapshotHash {
		t.Fatal("SnapshotAt must not affect snapshot hash")
	}
}

func TestBuildProviderOpsSnapshot_DeepCopy(t *testing.T) {
	op := NewProviderOperation("op-1", "exec-1", "tenant-1", "aws-ecs", OperationTypeDeploy, 1)
	ops := []ProviderOperation{op}
	snap := BuildProviderOpsSnapshot("snap-1", "exec-1", "tenant-1", ops, nil, nil, nil)
	ops[0].ProviderID = "mutated"
	if snap.Operations[0].ProviderID == "mutated" {
		t.Fatal("snapshot must deep-copy operations")
	}
}
