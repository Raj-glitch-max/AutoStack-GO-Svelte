package providers

import (
	"testing"
)

// Phase 3.9.8 — Canonical provider state vocabulary tests.

func TestNormalizeProviderStateKnownAliases(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		// Exact canonical states (round-trip)
		{"running", CanonicalStateRunning},
		{"creating", CanonicalStateCreating},
		{"updating", CanonicalStateUpdating},
		{"error", CanonicalStateError},
		{"deleting", CanonicalStateDeleting},
		{"deleted", CanonicalStateDeleted},
		{"missing", CanonicalStateMissing},
		{"unknown", CanonicalStateUnknown},
		{"ambiguous", CanonicalStateAmbiguous},
		{"pending", CanonicalStatePending},
		{"rolling_back", CanonicalStateRollingBack},
		{"rolled_back", CanonicalStateRolledBack},
		// Case variants
		{"RUNNING", CanonicalStateRunning},
		{"Running", CanonicalStateRunning},
		{"ERROR", CanonicalStateError},
		// Provider-native aliases
		{"not_found", CanonicalStateMissing},
		{"notfound", CanonicalStateMissing},
		{"not found", CanonicalStateMissing},
		{"inactive", CanonicalStateDeleted},
		{"INACTIVE", CanonicalStateDeleted},
		{"draining", CanonicalStateDeleting},
		{"DRAINING", CanonicalStateDeleting},
		{"provisioning", CanonicalStateCreating},
		{"deprovisioning", CanonicalStateDeleting},
		{"ready", CanonicalStateRunning},
		{"READY", CanonicalStateRunning},
		{"failed", CanonicalStateError},
		{"FAILED", CanonicalStateError},
		{"rollback_complete", CanonicalStateRolledBack},
	}

	for _, tc := range cases {
		got := NormalizeProviderState(tc.input)
		if got != tc.expected {
			t.Errorf("NormalizeProviderState(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestNormalizeProviderStateEmptyReturnsUnknown(t *testing.T) {
	if got := NormalizeProviderState(""); got != CanonicalStateUnknown {
		t.Errorf("NormalizeProviderState(\"\") = %q, want %q", got, CanonicalStateUnknown)
	}
}

func TestNormalizeProviderStateUnknownPreservedVerbatim(t *testing.T) {
	// Unknown states must be preserved (lowercase) as forensic evidence.
	cases := []struct {
		input    string
		expected string
	}{
		{"DRAINING_CUSTOM", "draining_custom"},
		{"SOME_FUTURE_STATE", "some_future_state"},
		{"partially_running", "partially_running"},
	}
	for _, tc := range cases {
		got := NormalizeProviderState(tc.input)
		if got != tc.expected {
			t.Errorf("NormalizeProviderState(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestNormalizeProviderStatePreservesWhitespaceTrim(t *testing.T) {
	if got := NormalizeProviderState("  running  "); got != CanonicalStateRunning {
		t.Errorf("expected whitespace trimmed to canonical, got %q", got)
	}
}

func TestIsCanonicalProviderStateKnownStates(t *testing.T) {
	canonical := []string{
		CanonicalStateRunning, CanonicalStateCreating, CanonicalStateUpdating,
		CanonicalStateError, CanonicalStateDeleting, CanonicalStateDeleted,
		CanonicalStateMissing, CanonicalStateUnknown, CanonicalStateAmbiguous,
		CanonicalStatePending, CanonicalStateRollingBack, CanonicalStateRolledBack,
	}
	for _, s := range canonical {
		if !IsCanonicalProviderState(s) {
			t.Errorf("IsCanonicalProviderState(%q) = false; expected true", s)
		}
	}
}

func TestIsCanonicalProviderStateNonCanonical(t *testing.T) {
	nonCanonical := []string{
		"RUNNING", "active", "healthy", "deployed", "not_found", "draining",
		"", "some_future_state",
	}
	for _, s := range nonCanonical {
		if IsCanonicalProviderState(s) {
			t.Errorf("IsCanonicalProviderState(%q) = true; expected false", s)
		}
	}
}

func TestNormalizeProviderStateResultIsCanonical(t *testing.T) {
	// Every alias in the alias table must produce a canonical result.
	for raw, expected := range providerStateAliases {
		got := NormalizeProviderState(raw)
		if got != expected {
			t.Errorf("NormalizeProviderState(%q) = %q, want %q", raw, got, expected)
		}
		if !IsCanonicalProviderState(got) {
			t.Errorf("providerStateAliases[%q] = %q is not canonical", raw, got)
		}
	}
}

func TestCanonicalStateConstantsNotEmpty(t *testing.T) {
	consts := []string{
		CanonicalStateRunning, CanonicalStateCreating, CanonicalStateUpdating,
		CanonicalStateError, CanonicalStateDeleting, CanonicalStateDeleted,
		CanonicalStateMissing, CanonicalStateUnknown, CanonicalStateAmbiguous,
		CanonicalStatePending, CanonicalStateRollingBack, CanonicalStateRolledBack,
	}
	seen := map[string]bool{}
	for _, c := range consts {
		if c == "" {
			t.Error("canonical state constant must not be empty")
		}
		if seen[c] {
			t.Errorf("duplicate canonical state constant: %q", c)
		}
		seen[c] = true
	}
}
