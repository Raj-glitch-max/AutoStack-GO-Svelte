package executor

import (
	"context"
	"testing"
)

func TestRuntime_InitialState_Pending(t *testing.T) {
	snap := newNoOpSnapshot(t)
	rt := newTestRuntime(t, snap, &NoOpProviderExecutor{})
	if rt.State() != StatePending {
		t.Fatalf("initial state must be %s, got %s", StatePending, rt.State())
	}
}

func TestRuntime_NewRuntime_NilSnapshot_Error(t *testing.T) {
	j, a, amb, l := newTestStores()
	_, err := NewExecutionRuntime("exec-1", "holder-1", nil, j, a, amb, l, &NoOpProviderExecutor{})
	if err == nil {
		t.Fatal("nil snapshot must return error")
	}
}

func TestRuntime_NewRuntime_EmptyExecutionID_Error(t *testing.T) {
	snap := newNoOpSnapshot(t)
	j, a, amb, l := newTestStores()
	_, err := NewExecutionRuntime("", "holder-1", snap, j, a, amb, l, &NoOpProviderExecutor{})
	if err == nil {
		t.Fatal("empty executionID must return error")
	}
}

func TestRuntime_NewRuntime_NilProvider_Error(t *testing.T) {
	snap := newNoOpSnapshot(t)
	j, a, amb, l := newTestStores()
	_, err := NewExecutionRuntime("exec-1", "holder-1", snap, j, a, amb, l, nil)
	if err == nil {
		t.Fatal("nil provider must return error")
	}
}

func TestRuntime_Start_TransitionsToReady(t *testing.T) {
	snap := newNoOpSnapshot(t)
	rt := newTestRuntime(t, snap, &NoOpProviderExecutor{})
	if err := rt.Start(); err != nil {
		t.Fatalf("Start must succeed: %v", err)
	}
	if rt.State() != StateReady {
		t.Fatalf("after Start state must be %s, got %s", StateReady, rt.State())
	}
}

func TestRuntime_Start_AcquiresLease(t *testing.T) {
	snap := newNoOpSnapshot(t)
	_, _, _, ls := newTestStores()
	j, a, amb := NewInMemoryJournalStore(), NewInMemoryApprovalStore(), NewInMemoryAmbiguityStore()
	rt, _ := NewExecutionRuntime("exec-lease", "holder-1", snap, j, a, amb, ls, &NoOpProviderExecutor{})
	rt.Start()

	held, err := ls.IsHeld("exec-lease")
	if err != nil || !held {
		t.Fatal("Start must acquire execution lease")
	}
}

func TestRuntime_Start_AlreadyStarted_Error(t *testing.T) {
	snap := newNoOpSnapshot(t)
	rt := newTestRuntime(t, snap, &NoOpProviderExecutor{})
	rt.Start()
	if err := rt.Start(); err == nil {
		t.Fatal("calling Start twice must return error")
	}
}

func TestRuntime_Pause_FromReady_TransitionsToPaused(t *testing.T) {
	snap := newNoOpSnapshot(t)
	rt := newStartedRuntime(t, snap, &NoOpProviderExecutor{})
	if err := rt.Pause("op-1", "maintenance"); err != nil {
		t.Fatalf("Pause must succeed from Ready: %v", err)
	}
	if rt.State() != StatePaused {
		t.Fatalf("expected %s, got %s", StatePaused, rt.State())
	}
}

func TestRuntime_Pause_EmptyOperator_Error(t *testing.T) {
	snap := newNoOpSnapshot(t)
	rt := newStartedRuntime(t, snap, &NoOpProviderExecutor{})
	if err := rt.Pause("", "reason"); err == nil {
		t.Fatal("Pause with empty operatorID must error")
	}
}

func TestRuntime_Pause_WrongState_Error(t *testing.T) {
	snap := newNoOpSnapshot(t)
	rt := newTestRuntime(t, snap, &NoOpProviderExecutor{}) // state = Pending
	if err := rt.Pause("op-1", "reason"); err == nil {
		t.Fatal("Pause in Pending state must error")
	}
}

func TestRuntime_Resume_FromPaused_TransitionsToReady(t *testing.T) {
	snap := newNoOpSnapshot(t)
	rt := newStartedRuntime(t, snap, &NoOpProviderExecutor{})
	rt.Pause("op-1", "maintenance")
	if err := rt.Resume("op-1"); err != nil {
		t.Fatalf("Resume must succeed from Paused: %v", err)
	}
	if rt.State() != StateReady {
		t.Fatalf("expected %s after resume, got %s", StateReady, rt.State())
	}
}

func TestRuntime_Resume_WrongState_Error(t *testing.T) {
	snap := newNoOpSnapshot(t)
	rt := newStartedRuntime(t, snap, &NoOpProviderExecutor{}) // state = Ready
	if err := rt.Resume("op-1"); err == nil {
		t.Fatal("Resume in non-Paused state must error")
	}
}

func TestRuntime_Advance_EmptyPlan_Completes(t *testing.T) {
	// A plan with no stages should complete immediately.
	snap := newNoOpSnapshot(t)
	// Override stages to be empty for this test.
	snap.Plan.Stages = nil
	rt := newStartedRuntime(t, snap, &SucceedingProviderExecutor{})
	if err := rt.Advance(context.Background()); err != nil {
		t.Fatalf("Advance on empty plan must succeed: %v", err)
	}
	if rt.State() != StateCompleted {
		t.Fatalf("expected %s, got %s", StateCompleted, rt.State())
	}
}

func TestRuntime_Advance_WrongState_Error(t *testing.T) {
	snap := newNoOpSnapshot(t)
	rt := newTestRuntime(t, snap, &NoOpProviderExecutor{}) // state = Pending
	if err := rt.Advance(context.Background()); err == nil {
		t.Fatal("Advance in Pending state must error")
	}
}

func TestRuntime_Advance_NoOp_Completes(t *testing.T) {
	snap := newNoOpSnapshot(t) // no actions → all no-ops → NoOp provider still validates
	// NoOp provider rejects, so stage fails. Use Succeeding provider.
	rt := newStartedRuntime(t, snap, &SucceedingProviderExecutor{})
	// Advance until completed or until a non-Ready state.
	for rt.State() == StateReady || rt.State() == StateAwaitingApproval {
		if err := rt.Advance(context.Background()); err != nil {
			t.Fatalf("Advance error: %v", err)
		}
	}
	if rt.State() != StateCompleted {
		t.Fatalf("no-op plan with succeeding provider must complete, got %s", rt.State())
	}
}

func TestRuntime_Advance_WithAmbiguousProvider_TransitionsToAmbiguous(t *testing.T) {
	snap := newCreateSnapshot(t)
	rt := newStartedRuntime(t, snap, &AmbiguousProviderExecutor{})
	for rt.State() == StateReady || rt.State() == StateAwaitingApproval {
		if err := rt.Advance(context.Background()); err != nil {
			t.Fatalf("Advance error: %v", err)
		}
	}
	if rt.State() != StateAmbiguous {
		t.Fatalf("ambiguous provider must produce %s, got %s", StateAmbiguous, rt.State())
	}
}

func TestRuntime_Advance_WithNoOpProvider_TransitionsToRollbackRequired(t *testing.T) {
	snap := newCreateSnapshot(t)
	rt := newStartedRuntime(t, snap, &NoOpProviderExecutor{})
	for rt.State() == StateReady || rt.State() == StateAwaitingApproval {
		if err := rt.Advance(context.Background()); err != nil {
			t.Fatalf("Advance error: %v", err)
		}
	}
	if rt.State() != StateRollbackRequired {
		t.Fatalf("failing provider must produce %s, got %s", StateRollbackRequired, rt.State())
	}
}

func TestRuntime_Transitions_Persisted(t *testing.T) {
	snap := newNoOpSnapshot(t)
	snap.Plan.Stages = nil
	rt := newStartedRuntime(t, snap, &SucceedingProviderExecutor{})
	rt.Advance(context.Background())

	transitions := rt.Transitions()
	if len(transitions) < 2 { // at minimum: Pending→Ready, Ready→Completed
		t.Fatalf("expected at least 2 transitions, got %d", len(transitions))
	}
	if transitions[0].FromState != StatePending {
		t.Fatalf("first transition must be from %s, got %s", StatePending, transitions[0].FromState)
	}
}

func TestRuntime_RollbackRec_Created_OnFailure(t *testing.T) {
	snap := newCreateSnapshot(t)
	rt := newStartedRuntime(t, snap, &NoOpProviderExecutor{})
	for rt.State() == StateReady || rt.State() == StateAwaitingApproval {
		rt.Advance(context.Background())
	}
	recs := rt.RollbackRecommendations()
	if len(recs) == 0 {
		t.Fatal("failure must produce at least one rollback recommendation")
	}
	if recs[0].ExecutionID != rt.ExecutionID() {
		t.Fatal("rollback recommendation must reference this execution")
	}
	if recs[0].OperatorReviewed {
		t.Fatal("rollback recommendation must not be pre-marked as operator-reviewed")
	}
}

func TestRuntime_ResolveAmbiguity_TransitionsToReady(t *testing.T) {
	snap := newCreateSnapshot(t)
	rt := newStartedRuntime(t, snap, &AmbiguousProviderExecutor{})
	for rt.State() == StateReady || rt.State() == StateAwaitingApproval {
		rt.Advance(context.Background())
	}
	if rt.State() != StateAmbiguous {
		t.Fatalf("expected %s before resolve, got %s", StateAmbiguous, rt.State())
	}
	if err := rt.ResolveAmbiguity("op-1", "verified safe to continue"); err != nil {
		t.Fatalf("ResolveAmbiguity must succeed: %v", err)
	}
	if rt.State() != StateReady {
		t.Fatalf("expected %s after resolve, got %s", StateReady, rt.State())
	}
}

func TestRuntime_ResolveAmbiguity_WrongState_Error(t *testing.T) {
	snap := newNoOpSnapshot(t)
	rt := newStartedRuntime(t, snap, &NoOpProviderExecutor{})
	if err := rt.ResolveAmbiguity("op-1", "resolution"); err == nil {
		t.Fatal("ResolveAmbiguity in non-Ambiguous state must error")
	}
}

func TestRuntime_Cancel_FromReady_TransitionsToCancelled(t *testing.T) {
	snap := newNoOpSnapshot(t)
	rt := newStartedRuntime(t, snap, &NoOpProviderExecutor{})
	if err := rt.Cancel("op-1", "testing cancellation"); err != nil {
		t.Fatalf("Cancel from Ready must succeed: %v", err)
	}
	if rt.State() != StateCancelled {
		t.Fatalf("expected %s after Cancel, got %s", StateCancelled, rt.State())
	}
}

func TestRuntime_Cancel_FromPaused_TransitionsToCancelled(t *testing.T) {
	snap := newNoOpSnapshot(t)
	rt := newStartedRuntime(t, snap, &NoOpProviderExecutor{})
	rt.Pause("op-1", "maintenance")
	if err := rt.Cancel("op-1", "no longer needed"); err != nil {
		t.Fatalf("Cancel from Paused must succeed: %v", err)
	}
	if rt.State() != StateCancelled {
		t.Fatalf("expected %s, got %s", StateCancelled, rt.State())
	}
}

func TestRuntime_Cancel_EmptyOperator_Error(t *testing.T) {
	snap := newNoOpSnapshot(t)
	rt := newStartedRuntime(t, snap, &NoOpProviderExecutor{})
	if err := rt.Cancel("", "reason"); err == nil {
		t.Fatal("Cancel with empty operatorID must error")
	}
}

func TestRuntime_Cancel_EmptyReason_Error(t *testing.T) {
	snap := newNoOpSnapshot(t)
	rt := newStartedRuntime(t, snap, &NoOpProviderExecutor{})
	if err := rt.Cancel("op-1", ""); err == nil {
		t.Fatal("Cancel with empty reason must error")
	}
}

func TestRuntime_Cancel_FromCompleted_Error(t *testing.T) {
	snap := newNoOpSnapshot(t)
	snap.Plan.Stages = nil
	rt := newStartedRuntime(t, snap, &SucceedingProviderExecutor{})
	rt.Advance(context.Background()) // completes immediately (no stages)
	if rt.State() != StateCompleted {
		t.Fatalf("expected completed state for setup, got %s", rt.State())
	}
	if err := rt.Cancel("op-1", "too late"); err == nil {
		t.Fatal("Cancel in Completed (terminal) state must error")
	}
}

func TestRuntime_Cancel_TransitionPersisted(t *testing.T) {
	snap := newNoOpSnapshot(t)
	rt := newStartedRuntime(t, snap, &NoOpProviderExecutor{})
	rt.Cancel("op-1", "testing")
	transitions := rt.Transitions()
	last := transitions[len(transitions)-1]
	if last.ToState != StateCancelled {
		t.Fatalf("cancel transition must be persisted: expected %s, got %s", StateCancelled, last.ToState)
	}
	if last.ActorID != "op-1" {
		t.Fatalf("cancel transition actor must be op-1, got %s", last.ActorID)
	}
}

func TestRuntime_Advance_DeleteStage_RequiresApprovalGate(t *testing.T) {
	snap := newDeleteSnapshot(t)
	rt := newStartedRuntime(t, snap, &SucceedingProviderExecutor{})
	// Delete stages always have approval gates.
	if err := rt.Advance(context.Background()); err != nil {
		t.Fatalf("Advance must not error: %v", err)
	}
	if rt.State() != StateAwaitingApproval {
		t.Fatalf("delete stage must require approval, expected %s got %s", StateAwaitingApproval, rt.State())
	}
}

func TestRuntime_Advance_AfterApproval_Proceeds(t *testing.T) {
	snap := newDeleteSnapshot(t)
	j, a, amb, ls := newTestStores()
	rt, _ := NewExecutionRuntime("exec-del", "holder-1", snap, j, a, amb, ls, &SucceedingProviderExecutor{})
	rt.Start()

	// First Advance transitions to AwaitingApproval.
	rt.Advance(context.Background())
	if rt.State() != StateAwaitingApproval {
		t.Fatalf("expected AwaitingApproval, got %s", rt.State())
	}

	// Satisfy all gates.
	for _, gate := range snap.Plan.ApprovalGates {
		d := ApprovalDecision{
			ExecutionID:  "exec-del",
			GateID:       gate.GateID,
			StageID:      gate.StageID,
			OperatorID:   "op-1",
			OperatorRole: "admin",
			Approved:     true,
		}
		a.RecordDecision(d)
	}

	// Second Advance: all gates approved, should proceed to execute.
	if err := rt.Advance(context.Background()); err != nil {
		t.Fatalf("Advance after approval must not error: %v", err)
	}
	// State should no longer be AwaitingApproval.
	if rt.State() == StateAwaitingApproval {
		t.Fatal("after all gates approved, Advance must proceed past AwaitingApproval")
	}
}
