package reconciler

import (
	"testing"
)

// Phase 3.9.6 — Contradiction registry pure-logic tests.

func TestContradictionRegistryNonEmpty(t *testing.T) {
	if len(AllContradictions) == 0 {
		t.Fatal("AllContradictions must contain at least one check — Phase 3.9.6 mandate")
	}
	// 5 original (Phase 3.9.6) + 2 Phase 3.9.7 provider truth checks + 1 Phase 3.9.9 silence check = 8.
	if len(AllContradictions) < 8 {
		t.Errorf("AllContradictions has %d entries; Phase 3.9.9 set is 8 — has a check been removed?", len(AllContradictions))
	}
}

func TestContradictionStructuralCorrectness(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range AllContradictions {
		if c.Name == "" {
			t.Error("contradiction has empty Name")
		}
		if seen[c.Name] {
			t.Errorf("duplicate contradiction Name %q", c.Name)
		}
		seen[c.Name] = true

		if c.Description == "" {
			t.Errorf("contradiction %q has empty Description", c.Name)
		}
		if c.Check == nil {
			t.Errorf("contradiction %q has nil Check function", c.Name)
		}
		switch c.Severity {
		case InvariantSeverityFatal, InvariantSeverityHigh, InvariantSeverityWarning:
			// OK
		default:
			t.Errorf("contradiction %q has invalid severity %q", c.Name, c.Severity)
		}
	}
}

func TestContradictionSeveritiesNotFatal(t *testing.T) {
	// Contradictions should NEVER be fatal — they don't block startup
	// the way fatal invariants do. Operators must investigate but the
	// runtime continues. Fatal contradictions would block forever because
	// there's no automatic resolution.
	for _, c := range AllContradictions {
		if c.Severity == InvariantSeverityFatal {
			t.Errorf("contradiction %q is fatal; contradictions must be High/Warning so the runtime continues while operator investigates", c.Name)
		}
	}
}
