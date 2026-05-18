package reconciler

import (
	"strings"
	"testing"
)

// Phase 3.9.5 — Invariant framework tests.
//
// SCOPE STATEMENT (honest):
// These tests verify the structural correctness of the invariant registry:
//   - every Invariant has a non-empty Name + Description
//   - every Invariant has a non-nil Check function
//   - severity classifications are valid
//   - HasFatalViolation correctly identifies fatal entries
//
// They do NOT execute the Check functions because those require a DB.
// DB-backed invariant verification is documented as future work — operators
// observe [INVARIANT_VIOLATION] log lines under real runtime.

func TestInvariantsRegistryNonEmpty(t *testing.T) {
	if len(AllInvariants) == 0 {
		t.Fatal("AllInvariants must contain at least one invariant — Phase 3.9.5 mandate")
	}
	// Sanity floor: we shipped 8 invariants in the initial Phase 3.9.5 set.
	if len(AllInvariants) < 8 {
		t.Errorf("AllInvariants has %d entries; initial Phase 3.9.5 set was 8 — has an invariant been silently removed?", len(AllInvariants))
	}
}

func TestInvariantsStructuralCorrectness(t *testing.T) {
	seen := map[string]bool{}
	for _, inv := range AllInvariants {
		if inv.Name == "" {
			t.Error("invariant has empty Name")
		}
		if seen[inv.Name] {
			t.Errorf("duplicate invariant Name %q — Names must be unique for forensic clarity", inv.Name)
		}
		seen[inv.Name] = true

		if inv.Description == "" {
			t.Errorf("invariant %q has empty Description — operators rely on this for understanding violations", inv.Name)
		}
		if inv.Check == nil {
			t.Errorf("invariant %q has nil Check function", inv.Name)
		}
		switch inv.Severity {
		case InvariantSeverityFatal, InvariantSeverityHigh, InvariantSeverityWarning:
			// OK
		default:
			t.Errorf("invariant %q has invalid severity %q", inv.Name, inv.Severity)
		}
	}
}

func TestInvariantNamesUseCamelCase(t *testing.T) {
	// Convention: invariant names are CamelCase imperatives like
	// "RetryPendingMustHaveNextRetryAt". This makes log lines and incident
	// summaries readable. Tests enforce convention.
	for _, inv := range AllInvariants {
		if strings.Contains(inv.Name, " ") || strings.Contains(inv.Name, "-") || strings.Contains(inv.Name, "_") {
			t.Errorf("invariant Name %q must be CamelCase (no spaces, hyphens, or underscores)", inv.Name)
		}
		if len(inv.Name) > 0 {
			first := inv.Name[0]
			if !(first >= 'A' && first <= 'Z') {
				t.Errorf("invariant Name %q must start with uppercase letter", inv.Name)
			}
		}
	}
}

func TestHasFatalViolation(t *testing.T) {
	none := []InvariantViolation{}
	if HasFatalViolation(none) {
		t.Error("empty list must not be fatal")
	}
	onlyWarning := []InvariantViolation{
		{Severity: InvariantSeverityWarning},
	}
	if HasFatalViolation(onlyWarning) {
		t.Error("warning-only list must not be fatal")
	}
	onlyHigh := []InvariantViolation{
		{Severity: InvariantSeverityHigh},
	}
	if HasFatalViolation(onlyHigh) {
		t.Error("high-only list must not be fatal")
	}
	withFatal := []InvariantViolation{
		{Severity: InvariantSeverityHigh},
		{Severity: InvariantSeverityFatal},
		{Severity: InvariantSeverityWarning},
	}
	if !HasFatalViolation(withFatal) {
		t.Error("list containing fatal must be fatal")
	}
}

func TestInvariantSeverityStableStrings(t *testing.T) {
	// Persisted strings (incidents.severity is mapped from these) must
	// never change. Pin them.
	expected := map[InvariantSeverity]string{
		InvariantSeverityFatal:   "fatal",
		InvariantSeverityHigh:    "high",
		InvariantSeverityWarning: "warning",
	}
	for sev, want := range expected {
		if string(sev) != want {
			t.Errorf("InvariantSeverity %q changed to %q — BREAKING SCHEMA CHANGE", want, sev)
		}
	}
}

func TestMetricTruthfulnessStableStrings(t *testing.T) {
	expected := map[MetricTruthfulnessState]string{
		MetricTruthfulnessUntrusted: "untrusted_pre_rehydrate",
		MetricTruthfulnessHydrated:  "hydrated_from_db",
		MetricTruthfulnessPartial:   "partial_db_read_error",
	}
	for state, want := range expected {
		if string(state) != want {
			t.Errorf("MetricTruthfulnessState %q changed to %q", want, state)
		}
	}
}

func TestKnownFatalInvariantsExist(t *testing.T) {
	// At least one fatal invariant must exist; otherwise startup audit
	// can never refuse the reconcile loop and the fatal-classification
	// feature is dead code.
	hasFatal := false
	for _, inv := range AllInvariants {
		if inv.Severity == InvariantSeverityFatal {
			hasFatal = true
			break
		}
	}
	if !hasFatal {
		t.Error("at least one invariant must be classified Fatal; otherwise HasFatalViolation is dead code")
	}
}
