package verifier

import (
	"strconv"
	"testing"
	"time"
)

// makeTimelineSatisfying builds a timeline that meets all conditions
// for the given window: enough observations with consistent states,
// spanning enough time since submittedAt.
func makeTimelineSatisfying(t *testing.T, window StabilizationWindow, submittedAt time.Time, state string) (ObservationTimeline, string) {
	t.Helper()
	submittedStr := submittedAt.Format(time.RFC3339)
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")

	windowDur := time.Duration(window.WindowDuration)
	// Place observations across the window + a bit more.
	for i := 0; i < window.RequiredSamples; i++ {
		offset := windowDur * time.Duration(i+1) / time.Duration(window.RequiredSamples+1)
		obsTime := submittedAt.Add(offset + windowDur) // all after window closes
		obs := ProviderObservation{
			ObservationID:  "obs-" + time.Duration(i).String(),
			ExecutionID:    "exec-1",
			StageID:        "stage-0",
			NodeID:         "svc-a",
			ObservedAt:     obsTime.Format(time.RFC3339),
			ProviderState:  state,
			NativeState:    state,
			TrustLevel:     TrustHigh,
			VerificationSource: "get_status",
		}
		AppendObservation(&tl, obs)
	}
	return tl, submittedStr
}

func TestStabilizationWindow_TooFewSamples_NotSatisfied(t *testing.T) {
	window := DefaultStabilizationWindow
	now := time.Now().UTC().Add(-10 * time.Minute) // submitted 10 minutes ago
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	// Add only 1 sample (window requires 3).
	obs := ProviderObservation{
		ObservationID:  "obs-1",
		ExecutionID:    "exec-1",
		ObservedAt:     now.Add(3 * time.Minute).Format(time.RFC3339),
		ProviderState:  "running",
		TrustLevel:     TrustHigh,
	}
	AppendObservation(&tl, obs)

	if window.IsSatisfied(tl, now.Format(time.RFC3339)) {
		t.Fatal("window must not be satisfied with fewer than RequiredSamples")
	}
}

func TestStabilizationWindow_TooShortTime_NotSatisfied(t *testing.T) {
	window := DefaultStabilizationWindow // 2-minute window
	now := time.Now().UTC()
	submittedAt := now // submitted just now

	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	// Add enough samples but placed only 10 seconds after submission.
	for i := 0; i < window.RequiredSamples; i++ {
		obs := ProviderObservation{
			ObservationID:  "obs-" + time.Duration(i).String(),
			ExecutionID:    "exec-1",
			ObservedAt:     submittedAt.Add(10 * time.Second).Format(time.RFC3339),
			ProviderState:  "running",
			TrustLevel:     TrustHigh,
		}
		AppendObservation(&tl, obs)
	}

	if window.IsSatisfied(tl, submittedAt.Format(time.RFC3339)) {
		t.Fatal("window must not be satisfied when less than WindowDuration has elapsed")
	}
}

func TestStabilizationWindow_InconsistentTrailingStates_NotSatisfied(t *testing.T) {
	window := DefaultStabilizationWindow // 2-minute window, 30s consistency period
	submittedAt := time.Now().UTC().Add(-5 * time.Minute)
	// Latest obs at submittedAt+4min50s (now-10s). ConsistencyPeriod=30s → cutoff = now-40s.
	// Place the "running" observations at now-40s and now-30s so they fall inside the
	// trailing window alongside the "updating" observation — the window then sees mixed states.
	latestAt := submittedAt.Add(4*time.Minute + 50*time.Second) // now-10s

	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	// Two observations inside the trailing consistency window: state = "running"
	offsets := []time.Duration{
		4*time.Minute + 10*time.Second, // now-40s (on the boundary)
		4*time.Minute + 20*time.Second, // now-30s
	}
	for i, offset := range offsets {
		AppendObservation(&tl, ProviderObservation{
			ObservationID: "obs-" + strconv.Itoa(i),
			ExecutionID:   "exec-1",
			ObservedAt:    submittedAt.Add(offset).Format(time.RFC3339),
			ProviderState: "running",
			TrustLevel:    TrustHigh,
		})
	}
	// Latest observation also inside the trailing window but with a different state.
	AppendObservation(&tl, ProviderObservation{
		ObservationID: "obs-late",
		ExecutionID:   "exec-1",
		ObservedAt:    latestAt.Format(time.RFC3339),
		ProviderState: "updating", // contradicts the "running" obs in the same window
		TrustLevel:    TrustHigh,
	})

	if window.IsSatisfied(tl, submittedAt.Format(time.RFC3339)) {
		t.Fatal("window must not be satisfied when trailing observations are inconsistent")
	}
}

func TestStabilizationWindow_Satisfied_AllConditionsMet(t *testing.T) {
	window := DefaultStabilizationWindow
	submittedAt := time.Now().UTC().Add(-10 * time.Minute)
	tl, sub := makeTimelineSatisfying(t, window, submittedAt, "running")
	if !window.IsSatisfied(tl, sub) {
		t.Fatal("window must be satisfied when all conditions met")
	}
}

func TestStabilizationWindowForProvider_CloudRun(t *testing.T) {
	w := StabilizationWindowForProvider("gcp-cloudrun")
	if w.Provider != "gcp-cloudrun" {
		t.Fatalf("expected gcp-cloudrun window, got provider=%q", w.Provider)
	}
	if time.Duration(w.WindowDuration) != 2*time.Minute {
		t.Fatalf("expected 2-minute window for Cloud Run, got %s", time.Duration(w.WindowDuration))
	}
}

func TestStabilizationWindowForProvider_ECS(t *testing.T) {
	w := StabilizationWindowForProvider("aws-ecs")
	if w.Provider != "aws-ecs" {
		t.Fatalf("expected aws-ecs window, got %q", w.Provider)
	}
	if time.Duration(w.WindowDuration) != 5*time.Minute {
		t.Fatalf("expected 5-minute window for ECS, got %s", time.Duration(w.WindowDuration))
	}
}

func TestStabilizationWindowForProvider_Unknown_ReturnsDefault(t *testing.T) {
	w := StabilizationWindowForProvider("nonexistent-provider")
	if w.Provider != "unknown" {
		t.Fatalf("unknown provider must return DefaultStabilizationWindow, got provider=%q", w.Provider)
	}
}

func TestStabilizationWindow_SamplesSatisfied(t *testing.T) {
	window := DefaultStabilizationWindow
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	for i := 0; i < window.RequiredSamples; i++ {
		AppendObservation(&tl, ProviderObservation{ObservationID: "obs-" + time.Duration(i).String()})
	}
	if !window.SamplesSatisfied(tl) {
		t.Fatal("SamplesSatisfied must return true when RequiredSamples met")
	}
}

func TestStabilizationWindow_EmptyTimeline_NotSatisfied(t *testing.T) {
	window := DefaultStabilizationWindow
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	if window.IsSatisfied(tl, time.Now().UTC().Format(time.RFC3339)) {
		t.Fatal("empty timeline must not satisfy stabilization window")
	}
}

func TestStabilizationWindow_InvalidSubmittedAt_NotSatisfied(t *testing.T) {
	window := DefaultStabilizationWindow
	tl, _ := makeTimelineSatisfying(t, window, time.Now().UTC().Add(-10*time.Minute), "running")
	if window.IsSatisfied(tl, "not-a-timestamp") {
		t.Fatal("invalid submittedAt must not satisfy stabilization window")
	}
}
