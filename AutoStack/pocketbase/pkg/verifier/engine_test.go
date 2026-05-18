package verifier

import (
	"strconv"
	"testing"
	"time"
)

// buildSatisfiedContext creates a VerificationContext where the stabilization window
// is met, observations are consistent and fresh, and no contradictions exist.
// All observations report the given state with TrustHigh.
func buildSatisfiedContext(t *testing.T, state string) VerificationContext {
	t.Helper()
	submittedAt := time.Now().UTC().Add(-10 * time.Minute)
	obsBase := submittedAt.Add(3 * time.Minute) // 3 min after submission satisfies the 2-min window
	// refTime: 1 min after the last observation — obs are ~1 min old, well within 5-min MaxStaleness
	lastObsOffset := time.Duration(DefaultStabilizationWindow.RequiredSamples-1) * time.Second
	refTime := obsBase.Add(lastObsOffset + time.Minute)

	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	for i := 0; i < DefaultStabilizationWindow.RequiredSamples; i++ {
		tl.Observations = append(tl.Observations, ProviderObservation{
			ObservationID:          "obs-" + strconv.Itoa(i),
			ExecutionID:            "exec-1",
			StageID:                "stage-0",
			NodeID:                 "svc-a",
			ObservedAt:             obsBase.Add(time.Duration(i) * time.Second).Format(time.RFC3339),
			ProviderState:          state,
			NativeState:            state,
			TrustLevel:             TrustHigh,
			VerificationSource:     "get_status",
			ConfidenceContribution: 1.0,
		})
	}

	return VerificationContext{
		ExecutionID:         "exec-1",
		StageID:             "stage-0",
		NodeID:              "svc-a",
		Timeline:            tl,
		StabilizationWindow: DefaultStabilizationWindow,
		Policy:              DefaultConsistencyPolicy,
		SubmittedAt:         submittedAt.Format(time.RFC3339),
		RefTime:             refTime.Format(time.RFC3339),
	}
}

func TestEngine_NoObservations_Unverifiable(t *testing.T) {
	ctx := VerificationContext{
		ExecutionID:         "exec-1",
		StageID:             "stage-0",
		NodeID:              "svc-a",
		Timeline:            NewObservationTimeline("exec-1", "stage-0", "svc-a"),
		StabilizationWindow: DefaultStabilizationWindow,
		Policy:              DefaultConsistencyPolicy,
		SubmittedAt:         time.Now().UTC().Format(time.RFC3339),
		RefTime:             time.Now().UTC().Format(time.RFC3339),
	}
	result := VerifyExecutionConsistency(ctx)
	if result.State != VerificationUnverifiable {
		t.Fatalf("no observations must produce Unverifiable, got %q", result.State)
	}
	if len(result.Limitations) == 0 {
		t.Fatal("Limitations must be non-empty")
	}
	if result.Recommendation == "" {
		t.Fatal("Recommendation must be non-empty")
	}
}

func TestEngine_Contradictions_Contradicted(t *testing.T) {
	now := time.Now().UTC()
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl, ProviderObservation{
		ObservationID: "obs-0", ExecutionID: "exec-1",
		ObservedAt:             now.Add(-2 * time.Minute).Format(time.RFC3339),
		ProviderState:          "running",
		TrustLevel:             TrustHigh,
		ConfidenceContribution: 1.0,
	})
	AppendObservation(&tl, ProviderObservation{
		ObservationID: "obs-1", ExecutionID: "exec-1",
		ObservedAt:             now.Add(-time.Minute).Format(time.RFC3339),
		ProviderState:          "failed",
		TrustLevel:             TrustHigh,
		ConfidenceContribution: 1.0,
	})

	ctx := VerificationContext{
		ExecutionID:         "exec-1",
		Timeline:            tl,
		StabilizationWindow: DefaultStabilizationWindow,
		Policy:              DefaultConsistencyPolicy,
		SubmittedAt:         now.Add(-5 * time.Minute).Format(time.RFC3339),
		// refTime 1 min in the future of now — both obs are < 3 min old, well within 5-min MaxStaleness
		RefTime: now.Add(time.Minute).Format(time.RFC3339),
	}
	result := VerifyExecutionConsistency(ctx)
	if result.State != VerificationContradicted {
		t.Fatalf("contradictory observations must produce Contradicted, got %q", result.State)
	}
	if len(result.Contradictions) == 0 {
		t.Fatal("Contradictions must be non-empty")
	}
	if len(result.Evidence) == 0 {
		t.Fatal("Evidence must be non-empty for Contradicted state")
	}
}

func TestEngine_AllStale_Stale(t *testing.T) {
	now := time.Now().UTC()
	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	AppendObservation(&tl, ProviderObservation{
		ObservationID: "obs-0", ExecutionID: "exec-1",
		// 20 min old — far beyond 5-min DefaultConsistencyPolicy MaxStaleness
		ObservedAt:             now.Add(-20 * time.Minute).Format(time.RFC3339),
		ProviderState:          "running",
		TrustLevel:             TrustHigh,
		ConfidenceContribution: 1.0,
	})

	ctx := VerificationContext{
		ExecutionID:         "exec-1",
		Timeline:            tl,
		StabilizationWindow: DefaultStabilizationWindow,
		Policy:              DefaultConsistencyPolicy,
		SubmittedAt:         now.Add(-25 * time.Minute).Format(time.RFC3339),
		RefTime:             now.Format(time.RFC3339),
	}
	result := VerifyExecutionConsistency(ctx)
	if result.State != VerificationStale {
		t.Fatalf("all-stale observations must produce Stale, got %q", result.State)
	}
	if len(result.Evidence) == 0 {
		t.Fatal("Evidence must be non-empty for Stale state")
	}
}

func TestEngine_EarlyNoSamplesNoTime_Acknowledged(t *testing.T) {
	submittedAt := time.Now().UTC()
	obsTime := submittedAt.Add(30 * time.Second) // 30s after submission (< 2-min window)
	refTime := submittedAt.Add(time.Minute)       // obs is 30s old — fresh

	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	// Only 1 observation (< RequiredSamples=3) and only 30s elapsed (< WindowDuration=2min).
	AppendObservation(&tl, ProviderObservation{
		ObservationID: "obs-0", ExecutionID: "exec-1",
		ObservedAt:             obsTime.Format(time.RFC3339),
		ProviderState:          "running",
		TrustLevel:             TrustHigh,
		ConfidenceContribution: 1.0,
	})

	ctx := VerificationContext{
		ExecutionID:         "exec-1",
		Timeline:            tl,
		StabilizationWindow: DefaultStabilizationWindow,
		Policy:              DefaultConsistencyPolicy,
		SubmittedAt:         submittedAt.Format(time.RFC3339),
		RefTime:             refTime.Format(time.RFC3339),
	}
	result := VerifyExecutionConsistency(ctx)
	if result.State != VerificationAcknowledged {
		t.Fatalf("very early stage (no samples, no elapsed time) must produce Acknowledged, got %q", result.State)
	}
}

func TestEngine_SamplesMet_WindowNotClosed_Stabilizing(t *testing.T) {
	submittedAt := time.Now().UTC()
	obsTime := submittedAt.Add(30 * time.Second) // 30s elapsed < 2-min window (timeMet=false)
	refTime := submittedAt.Add(time.Minute)       // obs are fresh

	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	// Meets sample count but not window duration — must produce Stabilizing, not Acknowledged.
	for i := 0; i < DefaultStabilizationWindow.RequiredSamples; i++ {
		AppendObservation(&tl, ProviderObservation{
			ObservationID: "obs-" + strconv.Itoa(i), ExecutionID: "exec-1",
			ObservedAt:             obsTime.Format(time.RFC3339),
			ProviderState:          "running",
			TrustLevel:             TrustHigh,
			ConfidenceContribution: 1.0,
		})
	}

	ctx := VerificationContext{
		ExecutionID:         "exec-1",
		Timeline:            tl,
		StabilizationWindow: DefaultStabilizationWindow,
		Policy:              DefaultConsistencyPolicy,
		SubmittedAt:         submittedAt.Format(time.RFC3339),
		RefTime:             refTime.Format(time.RFC3339),
	}
	result := VerifyExecutionConsistency(ctx)
	if result.State != VerificationStabilizing {
		t.Fatalf("samples met but window duration not elapsed must produce Stabilizing, got %q", result.State)
	}
}

func TestEngine_Verified_HighConfidenceSuccess(t *testing.T) {
	ctx := buildSatisfiedContext(t, "running")
	result := VerifyExecutionConsistency(ctx)
	if result.State != VerificationVerified {
		t.Fatalf("satisfied window + high confidence + success state must produce Verified, got %q", result.State)
	}
	if result.VerifiedAt == "" {
		t.Fatal("VerifiedAt must be set when state is Verified")
	}
	if !result.StabilizationMet {
		t.Fatal("StabilizationMet must be true when Verified")
	}
	if len(result.Evidence) == 0 {
		t.Fatal("Evidence must be non-empty when Verified")
	}
}

func TestEngine_PartiallyVerified_LowConfidenceSuccess(t *testing.T) {
	submittedAt := time.Now().UTC().Add(-10 * time.Minute)
	// Stale observations: placed 3 min after submission but refTime=now (age=7min > 5-min MaxStaleness)
	staleBase := submittedAt.Add(3 * time.Minute)
	// Fresh observation: 1 min ago — satisfies window (latestTime-submittedAt ≈ 9min >= 2min)
	freshTime := time.Now().UTC().Add(-time.Minute)
	refTime := time.Now().UTC()

	tl := NewObservationTimeline("exec-1", "stage-0", "svc-a")
	for i := 0; i < 3; i++ {
		tl.Observations = append(tl.Observations, ProviderObservation{
			ObservationID:          "stale-" + strconv.Itoa(i),
			ExecutionID:            "exec-1",
			ObservedAt:             staleBase.Add(time.Duration(i) * time.Second).Format(time.RFC3339),
			ProviderState:          "running",
			TrustLevel:             TrustHigh,
			ConfidenceContribution: 1.0,
		})
	}
	// Fresh observation falls outside the 30-s consistency period window so the
	// trailing consistency check sees only 1 state ("running") — no conflict.
	tl.Observations = append(tl.Observations, ProviderObservation{
		ObservationID:          "fresh-0",
		ExecutionID:            "exec-1",
		ObservedAt:             freshTime.Format(time.RFC3339),
		ProviderState:          "running",
		TrustLevel:             TrustHigh,
		ConfidenceContribution: 1.0,
	})

	ctx := VerificationContext{
		ExecutionID:         "exec-1",
		Timeline:            tl,
		StabilizationWindow: DefaultStabilizationWindow,
		Policy:              DefaultConsistencyPolicy,
		SubmittedAt:         submittedAt.Format(time.RFC3339),
		RefTime:             refTime.Format(time.RFC3339),
	}
	result := VerifyExecutionConsistency(ctx)
	if result.State != VerificationPartiallyVerified {
		t.Fatalf("low confidence success (3 stale obs) must produce PartiallyVerified, got %q (confidence=%+v)",
			result.State, result.Confidence)
	}
}

func TestEngine_FailureState_Unverifiable(t *testing.T) {
	// 3 consistent "failed" observations are not contradictory (same state throughout).
	// After the stabilization window closes, ConvergenceVerdict = "failure" → Unverifiable.
	ctx := buildSatisfiedContext(t, "failed")
	result := VerifyExecutionConsistency(ctx)
	if result.State != VerificationUnverifiable {
		t.Fatalf("failure state after stabilization must produce Unverifiable, got %q", result.State)
	}
}

func TestEngine_TransitionalState_Stabilizing(t *testing.T) {
	// 3 consistent "pending" observations after the stabilization window → Stabilizing.
	ctx := buildSatisfiedContext(t, "pending")
	result := VerifyExecutionConsistency(ctx)
	if result.State != VerificationStabilizing {
		t.Fatalf("transitional state after stabilization must produce Stabilizing, got %q", result.State)
	}
}

func TestEngine_ExecutionIDPreserved(t *testing.T) {
	ctx := buildSatisfiedContext(t, "running")
	ctx.ExecutionID = "my-exec"
	ctx.StageID = "stage-x"
	ctx.NodeID = "node-y"
	result := VerifyExecutionConsistency(ctx)
	if result.ExecutionID != "my-exec" {
		t.Fatalf("ExecutionID must be preserved in result, got %q", result.ExecutionID)
	}
	if result.StageID != "stage-x" {
		t.Fatalf("StageID must be preserved in result, got %q", result.StageID)
	}
	if result.NodeID != "node-y" {
		t.Fatalf("NodeID must be preserved in result, got %q", result.NodeID)
	}
}

func TestEngine_LimitationsAlwaysPresent(t *testing.T) {
	cases := []struct {
		name string
		ctx  VerificationContext
	}{
		{"no observations", VerificationContext{
			Timeline:            NewObservationTimeline("exec-1", "stage-0", "svc-a"),
			StabilizationWindow: DefaultStabilizationWindow,
			Policy:              DefaultConsistencyPolicy,
			SubmittedAt:         time.Now().UTC().Format(time.RFC3339),
			RefTime:             time.Now().UTC().Format(time.RFC3339),
		}},
		{"verified", buildSatisfiedContext(t, "running")},
		{"failure state", buildSatisfiedContext(t, "failed")},
		{"transitional", buildSatisfiedContext(t, "pending")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := VerifyExecutionConsistency(tc.ctx)
			if len(result.Limitations) == 0 {
				t.Fatalf("Limitations must be non-empty in all results (state=%s)", result.State)
			}
		})
	}
}

func TestEngine_Deterministic_SameInputSameOutput(t *testing.T) {
	ctx := buildSatisfiedContext(t, "running")
	r1 := VerifyExecutionConsistency(ctx)
	r2 := VerifyExecutionConsistency(ctx)
	if r1.State != r2.State {
		t.Fatalf("same inputs must produce same State: %q vs %q", r1.State, r2.State)
	}
	if r1.Confidence.ExecutionConfidence != r2.Confidence.ExecutionConfidence {
		t.Fatal("same inputs must produce same ExecutionConfidence")
	}
	if r1.Confidence.ProviderConfidence != r2.Confidence.ProviderConfidence {
		t.Fatal("same inputs must produce same ProviderConfidence")
	}
}

func TestEngine_RecommendationNonEmpty(t *testing.T) {
	cases := []VerificationContext{
		{
			Timeline:            NewObservationTimeline("e", "s", "n"),
			StabilizationWindow: DefaultStabilizationWindow,
			Policy:              DefaultConsistencyPolicy,
			SubmittedAt:         time.Now().UTC().Format(time.RFC3339),
			RefTime:             time.Now().UTC().Format(time.RFC3339),
		},
		buildSatisfiedContext(t, "running"),
		buildSatisfiedContext(t, "failed"),
	}
	for _, ctx := range cases {
		result := VerifyExecutionConsistency(ctx)
		if result.Recommendation == "" {
			t.Fatalf("Recommendation must be non-empty (state=%s)", result.State)
		}
	}
}
