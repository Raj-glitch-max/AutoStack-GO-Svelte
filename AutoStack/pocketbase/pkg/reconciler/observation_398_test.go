package reconciler

import (
	"testing"
)

// Phase 3.9.8 — Trust classification and expected-state mapping pure-logic tests.

func TestClassifyObservationTrustFromConfidence(t *testing.T) {
	cases := []struct {
		input         string
		expectedTrust ObservationTrust
	}{
		{"high", ObservationTrustAuthoritative},
		{"medium", ObservationTrustInferred},
		{"low", ObservationTrustAmbiguous},
		// Empty/unknown defaults to inferred, not authoritative
		{"", ObservationTrustInferred},
		{"unknown_level", ObservationTrustInferred},
	}

	for _, tc := range cases {
		got := classifyObservationTrustFromConfidence(tc.input)
		if got != tc.expectedTrust {
			t.Errorf("classifyObservationTrustFromConfidence(%q) = %q, want %q",
				tc.input, got, tc.expectedTrust)
		}
	}
}

func TestClassifyObservationTrustNeverReturnsContradicted(t *testing.T) {
	// classifyObservationTrustFromConfidence must never return Contradicted —
	// that trust level is set by inline contradiction detection only.
	for _, level := range []string{"high", "medium", "low", "", "bogus"} {
		got := classifyObservationTrustFromConfidence(level)
		if got == ObservationTrustContradicted {
			t.Errorf("classifyObservationTrustFromConfidence(%q) returned Contradicted — not allowed", level)
		}
	}
}

func TestRuntimeToExpectedProviderState(t *testing.T) {
	cases := []struct {
		runtimeStatus string
		expectedState string
	}{
		// States with a reliable expectation
		{"running", "running"},
		{"updating", "updating"},
		{"error", "error"},
		{"deleted", "deleted"},
		{"rolled_back", "rolled_back"},
		// Transitional states — no reliable expectation; contradiction skipped
		{"creating", ""},
		{"pending", ""},
		{"deleting", ""},
		{"", ""},
		{"unknown_status", ""},
	}

	for _, tc := range cases {
		got := runtimeToExpectedProviderState(tc.runtimeStatus)
		if got != tc.expectedState {
			t.Errorf("runtimeToExpectedProviderState(%q) = %q, want %q",
				tc.runtimeStatus, got, tc.expectedState)
		}
	}
}

func TestRuntimeToExpectedProviderStateSkipsTransitional(t *testing.T) {
	// Transitional states must return "" so DetectObservationContradiction
	// skips contradiction detection rather than generating false positives.
	transitional := []string{"creating", "pending", "deleting", ""}
	for _, s := range transitional {
		got := runtimeToExpectedProviderState(s)
		if got != "" {
			t.Errorf("runtimeToExpectedProviderState(%q) = %q; expected empty for transitional state", s, got)
		}
	}
}

func TestRuntimeToExpectedProviderStateCoversStableStates(t *testing.T) {
	// These stable states must return a non-empty expected state.
	stable := []string{"running", "updating", "error", "deleted", "rolled_back"}
	for _, s := range stable {
		got := runtimeToExpectedProviderState(s)
		if got == "" {
			t.Errorf("runtimeToExpectedProviderState(%q) = \"\"; expected non-empty for stable state", s)
		}
	}
}

func TestClassifyObservationTrustHighPriority(t *testing.T) {
	// "high" confidence MUST map to authoritative — this is the happy path
	// for successful GetStatus calls. Downgrading this would generate false
	// drift records for normal reconcile ticks.
	got := classifyObservationTrustFromConfidence("high")
	if got != ObservationTrustAuthoritative {
		t.Errorf("high confidence must map to authoritative, got %q", got)
	}
}

func TestClassifyObservationTrustLowMeansAmbiguous(t *testing.T) {
	// "low" confidence MUST map to ambiguous — a low-confidence GetStatus
	// result is explicitly uncertain and should NOT trigger contradiction
	// detection as authoritative evidence.
	got := classifyObservationTrustFromConfidence("low")
	if got != ObservationTrustAmbiguous {
		t.Errorf("low confidence must map to ambiguous, got %q", got)
	}
}
