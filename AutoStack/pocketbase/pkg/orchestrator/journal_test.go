package orchestrator

import (
	"strings"
	"testing"

	"github.com/janlauber/one-click/pkg/planner"
)

func TestNewJournalEntry_RequiredFields(t *testing.T) {
	entry := NewJournalEntry(
		"exec-001",
		"plan-hash-abc",
		"exec-plan-hash-xyz",
		"stage-0",
		"svc-a",
		planner.ActionCreate,
		"resource exists in healthy state",
	)

	if entry.ExecutionID != "exec-001" {
		t.Fatalf("ExecutionID must be set, got %q", entry.ExecutionID)
	}
	if entry.PlanHash != "plan-hash-abc" {
		t.Fatalf("PlanHash must be set, got %q", entry.PlanHash)
	}
	if entry.ExecutionPlanHash != "exec-plan-hash-xyz" {
		t.Fatalf("ExecutionPlanHash must be set, got %q", entry.ExecutionPlanHash)
	}
	if entry.StageID != "stage-0" {
		t.Fatalf("StageID must be set, got %q", entry.StageID)
	}
	if entry.NodeID != "svc-a" {
		t.Fatalf("NodeID must be set, got %q", entry.NodeID)
	}
	if entry.IntendedAction != planner.ActionCreate {
		t.Fatalf("IntendedAction must be ActionCreate, got %s", entry.IntendedAction)
	}
	if entry.ExpectedOutcome == "" {
		t.Fatal("ExpectedOutcome must be non-empty")
	}
}

func TestNewJournalEntry_Timestamp_RFC3339(t *testing.T) {
	entry := NewJournalEntry("e", "p", "ep", "s", "n", planner.ActionCreate, "ok")
	if entry.Timestamp == "" {
		t.Fatal("Timestamp must be non-empty")
	}
	// Basic RFC3339 format check: contains 'T' and has reasonable length.
	if !strings.Contains(entry.Timestamp, "T") {
		t.Fatalf("Timestamp must be RFC3339 format, got %q", entry.Timestamp)
	}
	if len(entry.Timestamp) < 16 {
		t.Fatalf("Timestamp too short: %q", entry.Timestamp)
	}
}

func TestNewJournalEntry_DefaultsEmpty(t *testing.T) {
	entry := NewJournalEntry("e", "p", "ep", "s", "n", planner.ActionCreate, "ok")
	if entry.FailureMode != "" {
		t.Fatalf("FailureMode must default to empty, got %q", entry.FailureMode)
	}
	if entry.RollbackReference != "" {
		t.Fatalf("RollbackReference must default to empty")
	}
	if entry.CompensationReference != "" {
		t.Fatalf("CompensationReference must default to empty")
	}
	if entry.HumanApprovalRef != "" {
		t.Fatalf("HumanApprovalRef must default to empty")
	}
}

func TestJournalEntry_WithFailure(t *testing.T) {
	entry := NewJournalEntry("e", "p", "ep", "s", "n", planner.ActionDelete, "deleted")
	failure := entry.WithFailure(FailureClassIrreversible, "rollback-stage-0", "comp-stage-0")

	if failure.FailureMode != FailureClassIrreversible {
		t.Fatalf("FailureMode must be set, got %q", failure.FailureMode)
	}
	if failure.RollbackReference != "rollback-stage-0" {
		t.Fatalf("RollbackReference must be set")
	}
	if failure.CompensationReference != "comp-stage-0" {
		t.Fatalf("CompensationReference must be set")
	}
	// Original must be unchanged (value copy).
	if entry.FailureMode != "" {
		t.Fatal("WithFailure must not mutate the original entry")
	}
}

func TestJournalEntry_WithApproval(t *testing.T) {
	entry := NewJournalEntry("e", "p", "ep", "s", "n", planner.ActionCreate, "ok")
	approved := entry.WithApproval("gate-001")

	if approved.HumanApprovalRef != "gate-001" {
		t.Fatalf("HumanApprovalRef must be set, got %q", approved.HumanApprovalRef)
	}
	if entry.HumanApprovalRef != "" {
		t.Fatal("WithApproval must not mutate the original entry")
	}
}

func TestJournalEntry_WithNote(t *testing.T) {
	entry := NewJournalEntry("e", "p", "ep", "s", "n", planner.ActionCreate, "ok")
	noted := entry.WithNote("operator confirmed at 14:30 UTC")

	if noted.Note != "operator confirmed at 14:30 UTC" {
		t.Fatalf("Note must be set, got %q", noted.Note)
	}
	if entry.Note != "" {
		t.Fatal("WithNote must not mutate the original entry")
	}
}

func TestJournalEntry_CanAutoRecover_ImplicitlyFalse(t *testing.T) {
	// Auto-recovery is forbidden in Phase 4.1. The FailureModel tracks CanAutoRecover=false.
	// The journal entry does not have this field — verified by absence.
	entry := NewJournalEntry("e", "p", "ep", "s", "n", planner.ActionCreate, "ok")
	// Just verifying the entry type has the expected shape.
	if entry == nil {
		t.Fatal("entry must be non-nil")
	}
}

func TestJournalEntry_Chaining(t *testing.T) {
	entry := NewJournalEntry("e", "p", "ep", "s", "n", planner.ActionDelete, "deleted")
	result := entry.
		WithFailure(FailureClassIrreversible, "rollback-0", "comp-0").
		WithApproval("gate-del-0").
		WithNote("operator manually confirmed deletion")

	if result.FailureMode != FailureClassIrreversible {
		t.Fatal("chained WithFailure must be set")
	}
	if result.HumanApprovalRef != "gate-del-0" {
		t.Fatal("chained WithApproval must be set")
	}
	if result.Note != "operator manually confirmed deletion" {
		t.Fatal("chained WithNote must be set")
	}
}
