package reconciler

import (
	"strings"
	"testing"

	"github.com/janlauber/one-click/pkg/providers"
)

// Phase 3.9.6 — Decision taxonomy + confidence pure-logic tests.
//
// DB-backed tests (RecordDecision writes, ListDecisions queries) require
// a PocketBase test fixture that this codebase doesn't have yet. Production
// validation relies on observing [INCIDENT_OPENED] + log lines tied to
// decision_type values.

func TestDecisionTypeStableStrings(t *testing.T) {
	expected := map[DecisionType]string{
		DecisionRetryScheduled:           "retry_scheduled",
		DecisionRetryExhausted:           "retry_exhausted",
		DecisionRetryRefusedNonRetryable: "retry_refused_non_retryable",
		DecisionRetryRefusedLeaseInvalid: "retry_refused_lease_invalid",
		DecisionRetryEscalatedAmbiguity:  "retry_escalated_ambiguity",
		DecisionRetryOperatorAction:      "retry_operator_action",
		DecisionLeaseLostPreCall:         "lease_lost_pre_call",
		DecisionLeaseLostMidCall:         "lease_lost_mid_call",
		DecisionLeaseAcquireDenied:       "lease_acquire_denied",
		DecisionCircuitBreakerOpen:       "circuit_breaker_open",
		DecisionCooldownActive:           "cooldown_active",
		DecisionRetryBackoffWindow:       "retry_backoff_window",
		DecisionObservationHeld:          "observation_held",
		DecisionStartupRefusedFatal:      "startup_refused_fatal",
		DecisionDispatchClaimLost:        "dispatch_claim_lost",
		DecisionContradictionDetected:    "contradiction_detected",
		// Phase 3.9.7 — provider truth
		DecisionProviderContradiction: "provider_contradiction",
		DecisionProviderUnknowable:    "provider_unknowable",
		DecisionDriftOpened:           "drift_opened",
		DecisionDriftResolved:         "drift_resolved",
		// Phase 3.10 — spec drift
		DecisionDeploySpecDriftDetected: "deployspec_drift_detected",
	}
	for dt, want := range expected {
		if string(dt) != want {
			t.Errorf("DecisionType %q changed to %q — BREAKING SCHEMA CHANGE", want, dt)
		}
	}
}

func TestAllDecisionTypesRegistryComplete(t *testing.T) {
	// 16 original (Phase 3.9.6) + 4 Phase 3.9.7 provider truth types + 1 Phase 3.10 spec drift = 21.
	expectedCount := 21
	if len(AllDecisionTypes) != expectedCount {
		t.Errorf("AllDecisionTypes has %d entries; expected %d — has a decision type been silently added or removed?", len(AllDecisionTypes), expectedCount)
	}
	seen := map[DecisionType]bool{}
	for _, dt := range AllDecisionTypes {
		if seen[dt] {
			t.Errorf("AllDecisionTypes contains duplicate %q", dt)
		}
		seen[dt] = true
	}
}

func TestConfidenceLevelStableStrings(t *testing.T) {
	expected := map[ConfidenceLevel]string{
		ConfidenceHigh:   "high",
		ConfidenceMedium: "medium",
		ConfidenceLow:    "low",
	}
	for cl, want := range expected {
		if string(cl) != want {
			t.Errorf("ConfidenceLevel %q changed to %q", want, cl)
		}
	}
}

func TestConfidenceSourceStableStrings(t *testing.T) {
	expected := map[ConfidenceSource]string{
		ConfidenceSourceDeterministicPolicy: "deterministic_policy",
		ConfidenceSourceProviderClassified:  "provider_classified",
		ConfidenceSourceProviderAmbiguous:   "provider_ambiguous",
		ConfidenceSourceLeaseAuthoritative:  "lease_authoritative",
		ConfidenceSourceLeaseInferred:       "lease_inferred",
		ConfidenceSourceClassifierFallback:  "classifier_fallback",
		ConfidenceSourceInvariantAudit:      "invariant_audit",
	}
	for cs, want := range expected {
		if string(cs) != want {
			t.Errorf("ConfidenceSource %q changed to %q", want, cs)
		}
	}
}

func TestPolicyVersionPinned(t *testing.T) {
	// Bump intentionally on policy semantics changes; not on cosmetic edits.
	// Phase 3.9.7: bumped from 3.9.6.0 → 3.9.7.0 (provider trust policy added).
	if PolicyVersion != "3.9.7.0" {
		t.Errorf("PolicyVersion = %q, want %q", PolicyVersion, "3.9.7.0")
	}
}

func TestConfidenceForLeaseDecisions(t *testing.T) {
	cl, cs := confidenceFor(DecisionRetryRefusedLeaseInvalid, nil, false)
	if cl != ConfidenceHigh || cs != ConfidenceSourceLeaseAuthoritative {
		t.Errorf("lease invalid: got (%s, %s), want (high, lease_authoritative)", cl, cs)
	}
	cl, cs = confidenceFor(DecisionLeaseLostMidCall, nil, true)
	if cl != ConfidenceLow || cs != ConfidenceSourceLeaseInferred {
		t.Errorf("lease lost mid-call: got (%s, %s), want (low, lease_inferred)", cl, cs)
	}
	cl, cs = confidenceFor(DecisionLeaseLostPreCall, nil, true)
	if cl != ConfidenceHigh || cs != ConfidenceSourceLeaseAuthoritative {
		t.Errorf("lease lost pre-call: got (%s, %s), want (high, lease_authoritative)", cl, cs)
	}
}

func TestConfidenceForRetryDecisions(t *testing.T) {
	// Unknown class → low / classifier_fallback
	unknownClass := &providers.RetryDecision{Class: providers.RetryClassUnknown}
	cl, cs := confidenceFor(DecisionRetryScheduled, unknownClass, true)
	if cl != ConfidenceLow || cs != ConfidenceSourceClassifierFallback {
		t.Errorf("retry+unknown_class: got (%s, %s), want (low, classifier_fallback)", cl, cs)
	}
	// Provider-uncertain → medium / provider_ambiguous
	pUncertain := &providers.RetryDecision{Class: providers.RetryClassProviderUncertain}
	cl, cs = confidenceFor(DecisionRetryExhausted, pUncertain, true)
	if cl != ConfidenceMedium || cs != ConfidenceSourceProviderAmbiguous {
		t.Errorf("retry+provider_uncertain: got (%s, %s), want (medium, provider_ambiguous)", cl, cs)
	}
	// Auth class → high / provider_classified
	authClass := &providers.RetryDecision{Class: providers.RetryClassAuth}
	cl, cs = confidenceFor(DecisionRetryOperatorAction, authClass, true)
	if cl != ConfidenceHigh || cs != ConfidenceSourceProviderClassified {
		t.Errorf("retry+auth: got (%s, %s), want (high, provider_classified)", cl, cs)
	}
}

func TestConfidenceForAmbiguityEscalation(t *testing.T) {
	cl, cs := confidenceFor(DecisionRetryEscalatedAmbiguity, nil, true)
	if cl != ConfidenceMedium || cs != ConfidenceSourceProviderAmbiguous {
		t.Errorf("escalate_ambiguity: got (%s, %s), want (medium, provider_ambiguous)", cl, cs)
	}
}

func TestConfidenceForDeterministicSuppressions(t *testing.T) {
	for _, dt := range []DecisionType{
		DecisionCircuitBreakerOpen,
		DecisionCooldownActive,
		DecisionRetryBackoffWindow,
		DecisionStartupRefusedFatal,
		DecisionDispatchClaimLost,
	} {
		cl, cs := confidenceFor(dt, nil, true)
		if cl != ConfidenceHigh || cs != ConfidenceSourceDeterministicPolicy {
			t.Errorf("%s: got (%s, %s), want (high, deterministic_policy)", dt, cl, cs)
		}
	}
}

func TestConfidenceForObservationHeld(t *testing.T) {
	cl, cs := confidenceFor(DecisionObservationHeld, nil, true)
	if cl != ConfidenceMedium || cs != ConfidenceSourceDeterministicPolicy {
		t.Errorf("observation_held: got (%s, %s), want (medium, deterministic_policy)", cl, cs)
	}
}

func TestConfidenceForContradictionDetected(t *testing.T) {
	cl, cs := confidenceFor(DecisionContradictionDetected, nil, true)
	if cl != ConfidenceHigh || cs != ConfidenceSourceInvariantAudit {
		t.Errorf("contradiction: got (%s, %s), want (high, invariant_audit)", cl, cs)
	}
}

func TestShouldDedupeSuppressionTypeLogic(t *testing.T) {
	// Dedupe-eligible types — fire on every tick during persistent state.
	for _, dt := range []DecisionType{
		DecisionCooldownActive, DecisionRetryBackoffWindow,
		DecisionCircuitBreakerOpen, DecisionObservationHeld,
	} {
		if !shouldDedupeSuppressionType(dt) {
			t.Errorf("%s should be dedupe-eligible", dt)
		}
	}
	// One-shot types — must bypass dedupe.
	for _, dt := range []DecisionType{
		DecisionRetryScheduled, DecisionRetryExhausted,
		DecisionLeaseLostPreCall, DecisionLeaseLostMidCall,
		DecisionLeaseAcquireDenied, DecisionStartupRefusedFatal,
		DecisionContradictionDetected, DecisionRetryEscalatedAmbiguity,
		DecisionRetryOperatorAction, DecisionDispatchClaimLost,
	} {
		if shouldDedupeSuppressionType(dt) {
			t.Errorf("%s must NOT be dedupe-eligible (one-shot decision must always persist)", dt)
		}
	}
}

func TestPolicyNameForType(t *testing.T) {
	// Sample mapping checks — verify each subsystem maps correctly.
	cases := map[DecisionType]string{
		DecisionRetryScheduled:       PolicyNameRetryEngine,
		DecisionRetryExhausted:       PolicyNameRetryEngine,
		DecisionLeaseLostPreCall:     PolicyNameLeaseEngine,
		DecisionLeaseLostMidCall:     PolicyNameLeaseEngine,
		DecisionLeaseAcquireDenied:   PolicyNameLeaseEngine,
		DecisionCircuitBreakerOpen:   PolicyNameCircuitBreaker,
		DecisionCooldownActive:       PolicyNameLifecycleGuard,
		DecisionRetryBackoffWindow:   PolicyNameLifecycleGuard,
		DecisionObservationHeld:      PolicyNameObservationGuard,
		DecisionStartupRefusedFatal:  PolicyNameStartupAudit,
		DecisionContradictionDetected: PolicyNameContradiction,
	}
	for dt, want := range cases {
		got := policyNameForType(dt)
		if got != want {
			t.Errorf("policyNameForType(%s) = %s, want %s", dt, got, want)
		}
	}
}

func TestDedupeSuppressionWindowBlocksWithinWindow(t *testing.T) {
	// First call records — should NOT be deduped.
	if shouldDedupeDecision("t1", DecisionCooldownActive, "reason1") {
		t.Error("first call should not be deduped")
	}
	// Immediate second call within window — should be deduped.
	if !shouldDedupeDecision("t1", DecisionCooldownActive, "reason1") {
		t.Error("second call within window should be deduped")
	}
	// Different reason — NOT deduped.
	if shouldDedupeDecision("t1", DecisionCooldownActive, "different_reason") {
		t.Error("different reason should not be deduped")
	}
	// Different target — NOT deduped.
	if shouldDedupeDecision("t2", DecisionCooldownActive, "reason1") {
		t.Error("different target should not be deduped")
	}
}

func TestPolicyNamesNoSpacesOrUppercase(t *testing.T) {
	// Convention: policy names are snake_case lowercase for stable filtering.
	names := []string{
		PolicyNameRetryEngine, PolicyNameLifecycleGuard, PolicyNameLeaseEngine,
		PolicyNameInvariantAudit, PolicyNameCircuitBreaker, PolicyNameStartupAudit,
		PolicyNameContradiction, PolicyNameObservationGuard,
	}
	for _, n := range names {
		if strings.ContainsAny(n, " ABCDEFGHIJKLMNOPQRSTUVWXYZ-") {
			t.Errorf("policy name %q must be snake_case lowercase", n)
		}
	}
}
