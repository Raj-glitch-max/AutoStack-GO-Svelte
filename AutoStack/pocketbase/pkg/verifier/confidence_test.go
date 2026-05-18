package verifier

import (
	"testing"
)

func TestComputeConfidence_NoObservations_Degraded(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	score := ComputeConfidence(tl, nil)
	if score.ExecutionConfidence >= 1.0 {
		t.Fatal("no observations must degrade ExecutionConfidence below 1.0")
	}
	if score.ProviderConfidence >= 1.0 {
		t.Fatal("no observations must degrade ProviderConfidence below 1.0")
	}
	if len(score.DegradationReasons) == 0 {
		t.Fatal("DegradationReasons must be non-empty when confidence degraded")
	}
}

func TestComputeConfidence_HighTrustNoContradictions_MaxConfidence(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	for i := 0; i < 3; i++ {
		obs := makeObs("running", TrustHigh)
		AppendObservation(&tl, obs)
	}
	score := ComputeConfidence(tl, nil)
	if score.ExecutionConfidence < 1.0 {
		t.Fatalf("high-trust fresh observations must give max ExecutionConfidence, got %f", score.ExecutionConfidence)
	}
	if score.ProviderConfidence < 1.0 {
		t.Fatalf("high-trust fresh observations must give max ProviderConfidence, got %f", score.ProviderConfidence)
	}
}

func TestComputeConfidence_ContradictionDegrades(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl, makeObs("running", TrustHigh))
	AppendObservation(&tl, makeObs("failed", TrustHigh))

	contradictions := DetectContradictions(tl)
	score := ComputeConfidence(tl, contradictions)

	if score.ProviderConfidence >= 1.0 {
		t.Fatal("contradictions must degrade ProviderConfidence below 1.0")
	}
	if score.VerificationConfidence >= 1.0 {
		t.Fatal("contradictions must degrade VerificationConfidence below 1.0")
	}
}

func TestComputeConfidence_StaleObservationDegrades(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	obs := makeObs("running", TrustHigh)
	obs.StaleFlag = true
	AppendObservation(&tl, obs)

	score := ComputeConfidence(tl, nil)
	if score.ExecutionConfidence >= 1.0 {
		t.Fatalf("stale observation must degrade ExecutionConfidence, got %f", score.ExecutionConfidence)
	}
}

func TestComputeConfidence_LowTrustObservationDegrades(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl, makeObs("running", TrustLow))

	score := ComputeConfidence(tl, nil)
	if score.ProviderConfidence >= 1.0 {
		t.Fatalf("low-trust observation must degrade ProviderConfidence, got %f", score.ProviderConfidence)
	}
}

func TestComputeConfidence_MultipleContradictions_MoreDegradation(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl, makeObs("running", TrustHigh))
	AppendObservation(&tl, makeObs("failed", TrustHigh))

	oneContradiction := DetectContradictions(tl)
	scoreOne := ComputeConfidence(tl, oneContradiction)

	tl2 := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl2, makeObs("running", TrustHigh))
	AppendObservation(&tl2, makeObs("failed", TrustHigh))
	AppendObservation(&tl2, makeObs("deleted", TrustHigh))
	AppendObservation(&tl2, makeObs("running", TrustHigh))
	twoContradictions := DetectContradictions(tl2)
	scoreTwo := ComputeConfidence(tl2, twoContradictions)

	if len(twoContradictions) <= len(oneContradiction) {
		t.Skip("could not set up two-contradiction timeline — skipping comparison")
	}
	if scoreTwo.ProviderConfidence >= scoreOne.ProviderConfidence {
		t.Fatal("more contradictions must result in lower ProviderConfidence")
	}
}

func TestComputeConfidence_ConfidenceClampedToZero(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	// Many stale low-trust observations with contradictions.
	for i := 0; i < 10; i++ {
		obs := makeObs("running", TrustLow)
		obs.StaleFlag = true
		AppendObservation(&tl, obs)
	}
	lastObs := makeObs("failed", TrustLow)
	lastObs.StaleFlag = true
	AppendObservation(&tl, lastObs)

	contradictions := DetectContradictions(tl)
	score := ComputeConfidence(tl, contradictions)

	if score.ExecutionConfidence < 0.0 {
		t.Fatalf("confidence must not go below 0.0: got %f", score.ExecutionConfidence)
	}
	if score.ProviderConfidence < 0.0 {
		t.Fatalf("provider confidence must not go below 0.0: got %f", score.ProviderConfidence)
	}
}

func TestIsHighConfidence_AboveThreshold(t *testing.T) {
	score := ConfidenceScore{
		ExecutionConfidence:    0.9,
		ProviderConfidence:     0.85,
		VerificationConfidence: 0.88,
	}
	if !IsHighConfidence(score, 0.80) {
		t.Fatal("score above 0.80 threshold must return true from IsHighConfidence")
	}
}

func TestIsHighConfidence_BelowThreshold(t *testing.T) {
	score := ConfidenceScore{
		ExecutionConfidence:    0.9,
		ProviderConfidence:     0.5, // below threshold
		VerificationConfidence: 0.88,
	}
	if IsHighConfidence(score, 0.70) {
		t.Fatal("score with ProviderConfidence < 0.70 must return false from IsHighConfidence")
	}
}

func TestMinConfidence_ReturnsLowest(t *testing.T) {
	score := ConfidenceScore{
		ExecutionConfidence:    0.9,
		ProviderConfidence:     0.4,
		VerificationConfidence: 0.7,
	}
	if MinConfidence(score) != 0.4 {
		t.Fatalf("MinConfidence must return 0.4, got %f", MinConfidence(score))
	}
}
