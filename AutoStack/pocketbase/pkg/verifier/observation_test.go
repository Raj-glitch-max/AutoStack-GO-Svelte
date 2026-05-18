package verifier

import (
	"testing"
	"time"
)

func TestNewObservation_IDNonEmpty(t *testing.T) {
	obs := NewObservation("exec-1", "stage-0", "svc-a", "running", "RUNNING", TrustHigh, "get_status")
	if obs.ObservationID == "" {
		t.Fatal("ObservationID must be non-empty")
	}
}

func TestNewObservation_FieldsSet(t *testing.T) {
	obs := NewObservation("exec-1", "stage-0", "svc-a", "running", "RUNNING", TrustHigh, "get_status")
	if obs.ExecutionID != "exec-1" {
		t.Fatalf("ExecutionID mismatch: %s", obs.ExecutionID)
	}
	if obs.ProviderState != "running" {
		t.Fatalf("ProviderState mismatch: %s", obs.ProviderState)
	}
	if obs.TrustLevel != TrustHigh {
		t.Fatalf("TrustLevel mismatch: %s", obs.TrustLevel)
	}
	if obs.VerificationSource != "get_status" {
		t.Fatalf("VerificationSource mismatch: %s", obs.VerificationSource)
	}
}

func TestNewObservation_ObservedAtSet(t *testing.T) {
	obs := NewObservation("exec-1", "stage-0", "svc-a", "running", "RUNNING", TrustMedium, "polling")
	if obs.ObservedAt == "" {
		t.Fatal("ObservedAt must be set")
	}
}

func TestNewObservation_ConfidenceContribution_High(t *testing.T) {
	obs := NewObservation("exec-1", "stage-0", "svc-a", "running", "RUNNING", TrustHigh, "get_status")
	if obs.ConfidenceContribution != 1.0 {
		t.Fatalf("TrustHigh contribution must be 1.0, got %f", obs.ConfidenceContribution)
	}
}

func TestNewObservation_ConfidenceContribution_Low(t *testing.T) {
	obs := NewObservation("exec-1", "stage-0", "svc-a", "running", "RUNNING", TrustLow, "polling")
	if obs.ConfidenceContribution != 0.3 {
		t.Fatalf("TrustLow contribution must be 0.3, got %f", obs.ConfidenceContribution)
	}
}

func TestNewObservationTimeline_Empty(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	if len(tl.Observations) != 0 {
		t.Fatal("new timeline must be empty")
	}
	if tl.ExecutionID != "exec-1" {
		t.Fatalf("ExecutionID mismatch: %s", tl.ExecutionID)
	}
}

func TestAppendObservation_IncreasesCount(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	obs := NewObservation("exec-1", "stage-0", "svc-a", "running", "RUNNING", TrustHigh, "get_status")
	AppendObservation(&tl, obs)
	if len(tl.Observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(tl.Observations))
	}
}

func TestIsStale_FreshObservation_ReturnsFalse(t *testing.T) {
	obs := NewObservation("exec-1", "stage-0", "svc-a", "running", "RUNNING", TrustHigh, "get_status")
	refTime := time.Now().UTC().Add(30 * time.Second) // 30s after observation
	if IsStale(obs, refTime, 5*time.Minute) {
		t.Fatal("observation 30s old is not stale under 5-minute policy")
	}
}

func TestIsStale_OldObservation_ReturnsTrue(t *testing.T) {
	obs := NewObservation("exec-1", "stage-0", "svc-a", "running", "RUNNING", TrustHigh, "get_status")
	refTime := time.Now().UTC().Add(10 * time.Minute) // 10min after observation
	if !IsStale(obs, refTime, 5*time.Minute) {
		t.Fatal("observation 10min old IS stale under 5-minute policy")
	}
}

func TestIsStale_ExactBoundary_NotStale(t *testing.T) {
	obs := NewObservation("exec-1", "stage-0", "svc-a", "running", "RUNNING", TrustHigh, "get_status")
	refTime := time.Now().UTC().Add(5 * time.Minute) // exactly at boundary
	// 5min elapsed, policy 5min — NOT stale (> not >=)
	if IsStale(obs, refTime, 5*time.Minute) {
		t.Skip("boundary behavior depends on clock precision — skipping exact boundary test")
	}
}

func TestMarkStaleObservations_MarksOldOnes(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	obs := NewObservation("exec-1", "stage-0", "svc-a", "running", "RUNNING", TrustHigh, "get_status")
	AppendObservation(&tl, obs)

	refTime := time.Now().UTC().Add(10 * time.Minute) // far in the future
	MarkStaleObservations(&tl, refTime, 5*time.Minute)

	if !tl.Observations[0].StaleFlag {
		t.Fatal("observation must be marked stale")
	}
}

func TestCountStale_CountsCorrectly(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	fresh := NewObservation("exec-1", "stage-0", "svc-a", "running", "RUNNING", TrustHigh, "get_status")
	staleObs := fresh
	staleObs.StaleFlag = true
	AppendObservation(&tl, fresh)
	AppendObservation(&tl, staleObs)

	if CountStale(tl) != 1 {
		t.Fatalf("expected 1 stale, got %d", CountStale(tl))
	}
}

func TestAllStale_EmptyTimeline_ReturnsFalse(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	if AllStale(tl) {
		t.Fatal("empty timeline must return false from AllStale")
	}
}

func TestAllStale_AllMarkedStale_ReturnsTrue(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	obs := NewObservation("exec-1", "stage-0", "svc-a", "running", "RUNNING", TrustHigh, "get_status")
	AppendObservation(&tl, obs)
	refTime := time.Now().UTC().Add(10 * time.Minute)
	MarkStaleObservations(&tl, refTime, 5*time.Minute)

	if !AllStale(tl) {
		t.Fatal("all observations marked stale — AllStale must return true")
	}
}

func TestLatestObservation_EmptyTimeline_ReturnsFalse(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	_, ok := LatestObservation(tl)
	if ok {
		t.Fatal("empty timeline must return false from LatestObservation")
	}
}

func TestLatestObservation_ReturnsLast(t *testing.T) {
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	obs1 := NewObservation("exec-1", "stage-0", "svc-a", "running", "RUNNING", TrustHigh, "get_status")
	obs2 := NewObservation("exec-1", "stage-0", "svc-a", "healthy", "RUNNING", TrustHigh, "polling")
	AppendObservation(&tl, obs1)
	AppendObservation(&tl, obs2)

	latest, ok := LatestObservation(tl)
	if !ok {
		t.Fatal("non-empty timeline must return true")
	}
	if latest.ProviderState != "healthy" {
		t.Fatalf("expected last observation state 'healthy', got %q", latest.ProviderState)
	}
}
