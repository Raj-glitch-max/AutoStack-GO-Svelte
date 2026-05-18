package verifier

import (
	"testing"
)

func makeObs(state string, trust TrustLevel) ProviderObservation {
	obs := NewObservation("exec-1", "stage-0", "svc-a", state, state, trust, "get_status")
	return obs
}

func TestDetectContradictions_NoContradiction_EmptyResult(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl, makeObs("running", TrustHigh))
	AppendObservation(&tl, makeObs("running", TrustHigh))
	AppendObservation(&tl, makeObs("running", TrustHigh))

	cs := DetectContradictions(tl)
	if len(cs) != 0 {
		t.Fatalf("consistent timeline must have 0 contradictions, got %d", len(cs))
	}
}

func TestDetectContradictions_SuccessThenFailure_Detected(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl, makeObs("running", TrustHigh))
	AppendObservation(&tl, makeObs("failed", TrustHigh))

	cs := DetectContradictions(tl)
	if len(cs) == 0 {
		t.Fatal("running → failed must be detected as contradiction")
	}
}

func TestDetectContradictions_DeletedThenRunning_Detected(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl, makeObs("deleted", TrustHigh))
	AppendObservation(&tl, makeObs("running", TrustHigh))

	cs := DetectContradictions(tl)
	if len(cs) == 0 {
		t.Fatal("deleted → running must be detected as contradiction")
	}
}

func TestDetectContradictions_FailedThenHealthy_Detected(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl, makeObs("failed", TrustHigh))
	AppendObservation(&tl, makeObs("healthy", TrustHigh))

	cs := DetectContradictions(tl)
	if len(cs) == 0 {
		t.Fatal("failed → healthy must be detected as contradiction")
	}
}

func TestDetectContradictions_TransitionalThenSuccess_NoContradiction(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl, makeObs("pending", TrustHigh))
	AppendObservation(&tl, makeObs("running", TrustHigh))

	cs := DetectContradictions(tl)
	if len(cs) != 0 {
		t.Fatalf("pending → running is NOT a contradiction, got %d contradictions", len(cs))
	}
}

func TestDetectContradictions_ContradictionIDNonEmpty(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl, makeObs("running", TrustHigh))
	AppendObservation(&tl, makeObs("failed", TrustHigh))

	cs := DetectContradictions(tl)
	if len(cs) > 0 && cs[0].ContradictionID == "" {
		t.Fatal("ContradictionID must be non-empty")
	}
}

func TestDetectContradictions_ReasonNonEmpty(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl, makeObs("running", TrustHigh))
	AppendObservation(&tl, makeObs("failed", TrustHigh))

	cs := DetectContradictions(tl)
	if len(cs) > 0 && cs[0].Reason == "" {
		t.Fatal("Contradiction.Reason must be non-empty")
	}
}

func TestIsConsistent_ConsistentTimeline_ReturnsTrue(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl, makeObs("running", TrustHigh))
	AppendObservation(&tl, makeObs("running", TrustHigh))

	if !IsConsistent(tl) {
		t.Fatal("consistent timeline must return true from IsConsistent")
	}
}

func TestIsConsistent_InconsistentTimeline_ReturnsFalse(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl, makeObs("running", TrustHigh))
	AppendObservation(&tl, makeObs("failed", TrustHigh))

	if IsConsistent(tl) {
		t.Fatal("timeline with running→failed must return false from IsConsistent")
	}
}

func TestConvergenceVerdict_SuccessState(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl, makeObs("running", TrustHigh))
	if v := ConvergenceVerdict(tl); v != "success" {
		t.Fatalf("'running' state must produce 'success' verdict, got %q", v)
	}
}

func TestConvergenceVerdict_FailureState(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl, makeObs("failed", TrustHigh))
	if v := ConvergenceVerdict(tl); v != "failure" {
		t.Fatalf("'failed' state must produce 'failure' verdict, got %q", v)
	}
}

func TestConvergenceVerdict_TransitionalState(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl, makeObs("pending", TrustHigh))
	if v := ConvergenceVerdict(tl); v != "transitional" {
		t.Fatalf("'pending' state must produce 'transitional' verdict, got %q", v)
	}
}

func TestConvergenceVerdict_EmptyTimeline_Unknown(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	if v := ConvergenceVerdict(tl); v != "unknown" {
		t.Fatalf("empty timeline must produce 'unknown' verdict, got %q", v)
	}
}

func TestLatestProviderState_AllStale_ReturnsFalse(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	obs := makeObs("running", TrustHigh)
	obs.StaleFlag = true
	AppendObservation(&tl, obs)

	_, ok := LatestProviderState(tl)
	if ok {
		t.Fatal("all-stale timeline must return false from LatestProviderState")
	}
}

func TestAreContradictoryStates_SuccessToFailure(t *testing.T) {
	if !areContradictoryStates("running", "failed") {
		t.Fatal("running → failed must be contradictory")
	}
}

func TestAreContradictoryStates_TransitionalToSuccess_NotContradictory(t *testing.T) {
	if areContradictoryStates("pending", "running") {
		t.Fatal("pending → running is not contradictory")
	}
}
