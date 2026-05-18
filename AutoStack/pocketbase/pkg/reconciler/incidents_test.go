package reconciler

import (
	"testing"

	"github.com/janlauber/one-click/pkg/providers"
)

// Phase 3.9.4 — Incident state machine + helpers (pure-logic tests).
//
// SCOPE STATEMENT (honest):
// These tests verify the in-Go state-machine and severity-mapping
// invariants. They do NOT exercise the DB-side CreateOrEscalateIncident
// or UpdateIncidentStatus paths — those require a PocketBase test
// fixture not yet wired in this codebase. Production validation
// depends on observing the [INCIDENT_OPENED] / [INCIDENT_ESCALATED] /
// [INCIDENT_TRANSITION] log lines under real runtime.

func TestIncidentTransitionsAllowed(t *testing.T) {
	allowedCases := []struct {
		from, to IncidentStatus
	}{
		{IncidentStatusOpen, IncidentStatusEscalating},
		{IncidentStatusOpen, IncidentStatusAcknowledged},
		{IncidentStatusOpen, IncidentStatusResolved},
		{IncidentStatusOpen, IncidentStatusSuppressed},
		{IncidentStatusEscalating, IncidentStatusAcknowledged},
		{IncidentStatusEscalating, IncidentStatusResolved},
		{IncidentStatusEscalating, IncidentStatusEscalating}, // re-escalation
		{IncidentStatusAcknowledged, IncidentStatusResolved},
		{IncidentStatusAcknowledged, IncidentStatusEscalating},
		{IncidentStatusResolved, IncidentStatusReopened},
		{IncidentStatusReopened, IncidentStatusEscalating},
		{IncidentStatusReopened, IncidentStatusResolved},
		{IncidentStatusSuppressed, IncidentStatusOpen},
		{IncidentStatusSuppressed, IncidentStatusReopened},
	}
	for _, tc := range allowedCases {
		if !IsAllowedIncidentTransition(tc.from, tc.to) {
			t.Errorf("transition %s → %s should be allowed", tc.from, tc.to)
		}
	}
}

func TestIncidentTransitionsForbidden(t *testing.T) {
	// Critical forbidden transitions — silent disappearance, auto-resolve
	// of suppressed, resurrecting from suppressed without operator step.
	forbiddenCases := []struct {
		from, to IncidentStatus
	}{
		// Cannot resolve a suppressed incident directly (must unsuppress or reopen first).
		{IncidentStatusSuppressed, IncidentStatusResolved},
		// Cannot acknowledge a resolved incident (must reopen first).
		{IncidentStatusResolved, IncidentStatusAcknowledged},
		{IncidentStatusResolved, IncidentStatusEscalating},
		{IncidentStatusResolved, IncidentStatusOpen},
		// Cannot suppress while already resolved.
		{IncidentStatusResolved, IncidentStatusSuppressed},
		// Cannot transition between escalating and open backwards.
		{IncidentStatusEscalating, IncidentStatusOpen},
		{IncidentStatusEscalating, IncidentStatusSuppressed},
		// Acknowledged → suppressed not allowed (operator must resolve or reopen).
		{IncidentStatusAcknowledged, IncidentStatusSuppressed},
		{IncidentStatusAcknowledged, IncidentStatusOpen},
	}
	for _, tc := range forbiddenCases {
		if IsAllowedIncidentTransition(tc.from, tc.to) {
			t.Errorf("transition %s → %s must be forbidden (truthfulness rule)", tc.from, tc.to)
		}
	}
}

func TestIncidentSelfTransitionPermitted(t *testing.T) {
	for _, s := range []IncidentStatus{
		IncidentStatusOpen, IncidentStatusEscalating, IncidentStatusAcknowledged,
		IncidentStatusResolved, IncidentStatusReopened, IncidentStatusSuppressed,
	} {
		if !IsAllowedIncidentTransition(s, s) {
			t.Errorf("self-transition %s → %s must be permitted (no-op)", s, s)
		}
	}
}

func TestSeverityRankMonotonic(t *testing.T) {
	if severityRank(IncidentSeverityLow) >= severityRank(IncidentSeverityMedium) {
		t.Error("severity rank must be monotonic: low < medium")
	}
	if severityRank(IncidentSeverityMedium) >= severityRank(IncidentSeverityHigh) {
		t.Error("severity rank must be monotonic: medium < high")
	}
	if severityRank(IncidentSeverityHigh) >= severityRank(IncidentSeverityCritical) {
		t.Error("severity rank must be monotonic: high < critical")
	}
}

func TestSeverityForRetryClass(t *testing.T) {
	// Auth/Quota → high (operator-action required)
	if sev := severityForRetryClass(providers.RetryClassAuth); sev != IncidentSeverityHigh {
		t.Errorf("auth severity = %s, want high", sev)
	}
	if sev := severityForRetryClass(providers.RetryClassQuota); sev != IncidentSeverityHigh {
		t.Errorf("quota severity = %s, want high", sev)
	}
	// ProviderUncertain → high (outcome unknown is operationally serious)
	if sev := severityForRetryClass(providers.RetryClassProviderUncertain); sev != IncidentSeverityHigh {
		t.Errorf("provider_uncertain severity = %s, want high", sev)
	}
	// Internal/Unknown → critical (bug or unclassified)
	if sev := severityForRetryClass(providers.RetryClassInternal); sev != IncidentSeverityCritical {
		t.Errorf("internal severity = %s, want critical", sev)
	}
	if sev := severityForRetryClass(providers.RetryClassUnknown); sev != IncidentSeverityCritical {
		t.Errorf("unknown severity = %s, want critical", sev)
	}
	// Never → medium (spec invalid; operator must fix)
	if sev := severityForRetryClass(providers.RetryClassNever); sev != IncidentSeverityMedium {
		t.Errorf("never severity = %s, want medium", sev)
	}
}

func TestIncidentStatusStableStrings(t *testing.T) {
	// Persisted strings must never change. Pin them here.
	expected := map[IncidentStatus]string{
		IncidentStatusOpen:         "open",
		IncidentStatusEscalating:   "escalating",
		IncidentStatusAcknowledged: "acknowledged",
		IncidentStatusResolved:     "resolved",
		IncidentStatusReopened:     "reopened",
		IncidentStatusSuppressed:   "suppressed",
	}
	for status, want := range expected {
		if string(status) != want {
			t.Errorf("IncidentStatus %q changed to %q — BREAKING SCHEMA CHANGE", want, status)
		}
	}
}

func TestIncidentSourceStableStrings(t *testing.T) {
	// Persisted strings must never change.
	expected := map[IncidentSource]string{
		IncidentSourceRetryExhausted:     "retry-exhausted",
		IncidentSourceAmbiguityEscalation: "ambiguity-escalation",
		IncidentSourceLeaseLossMidCall:   "lease-loss-mid-call",
		IncidentSourceInterruptedUnknown: "interrupted-unknown-outcome",
		IncidentSourceRollbackFailed:     "rollback-failed",
		IncidentSourceReclaimAfterCrash:  "reclaim-after-crash",
		IncidentSourceInvariantViolation: "invariant-violation",
	}
	for src, want := range expected {
		if string(src) != want {
			t.Errorf("IncidentSource %q changed to %q — BREAKING SCHEMA CHANGE", want, src)
		}
	}
}
