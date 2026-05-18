package orchestrator

import (
	"testing"

	"github.com/janlauber/one-click/pkg/planner"
)

func TestExportExecutionSnapshot_NonNil(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	safetyResult := EvaluateExecutionSafety(plan, g, s)
	contracts := BuildExecutionContracts(g, cs)
	compPlan := BuildCompensationPlan(rg, g, cs)
	models := BuildFailureModels(g, cs)

	snap := ExportExecutionSnapshot(plan, safetyResult, contracts, compPlan, models)
	if snap == nil {
		t.Fatal("ExportExecutionSnapshot must return a non-nil snapshot")
	}
}

func TestExportExecutionSnapshot_SchemaVersion(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, nil)
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	safetyResult := EvaluateExecutionSafety(plan, g, s)
	contracts := BuildExecutionContracts(g, cs)
	compPlan := BuildCompensationPlan(rg, g, cs)
	models := BuildFailureModels(g, cs)

	snap := ExportExecutionSnapshot(plan, safetyResult, contracts, compPlan, models)
	if snap.SchemaVersion != ExecutionSnapshotSchemaVersion {
		t.Fatalf("expected schema version %q, got %q", ExecutionSnapshotSchemaVersion, snap.SchemaVersion)
	}
}

func TestExportExecutionSnapshot_ExportHashNonEmpty(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	safetyResult := EvaluateExecutionSafety(plan, g, s)
	contracts := BuildExecutionContracts(g, cs)
	compPlan := BuildCompensationPlan(rg, g, cs)
	models := BuildFailureModels(g, cs)

	snap := ExportExecutionSnapshot(plan, safetyResult, contracts, compPlan, models)
	if snap.ExportHash == "" {
		t.Fatal("ExportExecutionSnapshot must set a non-empty ExportHash")
	}
	if len(snap.ExportHash) != 64 {
		t.Fatalf("expected 64-char SHA-256 hex ExportHash, got %d chars", len(snap.ExportHash))
	}
}

func TestExportExecutionSnapshot_ExecutionPlanHashMirrored(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	safetyResult := EvaluateExecutionSafety(plan, g, s)
	contracts := BuildExecutionContracts(g, cs)
	compPlan := BuildCompensationPlan(rg, g, cs)
	models := BuildFailureModels(g, cs)

	snap := ExportExecutionSnapshot(plan, safetyResult, contracts, compPlan, models)
	if snap.ExecutionPlanHash != plan.ExecutionPlanHash {
		t.Fatalf("snapshot ExecutionPlanHash must mirror plan: want %q, got %q",
			plan.ExecutionPlanHash, snap.ExecutionPlanHash)
	}
}

func TestExportExecutionSnapshot_LimitationsNonEmpty(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, nil)
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	safetyResult := EvaluateExecutionSafety(plan, g, s)
	contracts := BuildExecutionContracts(g, cs)
	compPlan := BuildCompensationPlan(rg, g, cs)
	models := BuildFailureModels(g, cs)

	snap := ExportExecutionSnapshot(plan, safetyResult, contracts, compPlan, models)
	if len(snap.Limitations) == 0 {
		t.Fatal("ExecutionSnapshot must document limitations")
	}
}

func TestVerifyExecutionSnapshotHash_Valid(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	safetyResult := EvaluateExecutionSafety(plan, g, s)
	contracts := BuildExecutionContracts(g, cs)
	compPlan := BuildCompensationPlan(rg, g, cs)
	models := BuildFailureModels(g, cs)

	snap := ExportExecutionSnapshot(plan, safetyResult, contracts, compPlan, models)
	ok, reason := VerifyExecutionSnapshotHash(snap)
	if !ok {
		t.Fatalf("VerifyExecutionSnapshotHash must return true for unmodified snapshot: %s", reason)
	}
}

func TestVerifyExecutionSnapshotHash_Nil_ReturnsFalse(t *testing.T) {
	ok, reason := VerifyExecutionSnapshotHash(nil)
	if ok {
		t.Fatal("VerifyExecutionSnapshotHash must return false for nil snapshot")
	}
	if reason == "" {
		t.Fatal("reason must be non-empty on verification failure")
	}
}

func TestVerifyExecutionSnapshotHash_EmptyHash_ReturnsFalse(t *testing.T) {
	snap := &ExecutionSnapshot{ExportHash: ""}
	ok, reason := VerifyExecutionSnapshotHash(snap)
	if ok {
		t.Fatal("VerifyExecutionSnapshotHash must return false when ExportHash is empty")
	}
	if reason == "" {
		t.Fatal("reason must be non-empty on verification failure")
	}
}

func TestVerifyExecutionSnapshotHash_TamperedSchemaVersion_DetectsChange(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	safetyResult := EvaluateExecutionSafety(plan, g, s)
	contracts := BuildExecutionContracts(g, cs)
	compPlan := BuildCompensationPlan(rg, g, cs)
	models := BuildFailureModels(g, cs)

	snap := ExportExecutionSnapshot(plan, safetyResult, contracts, compPlan, models)
	snap.SchemaVersion = "tampered"

	ok, _ := VerifyExecutionSnapshotHash(snap)
	if ok {
		t.Fatal("tampered SchemaVersion must cause hash verification to fail")
	}
}

func TestVerifyExecutionSnapshotHash_TamperedPlanHash_DetectsChange(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	safetyResult := EvaluateExecutionSafety(plan, g, s)
	contracts := BuildExecutionContracts(g, cs)
	compPlan := BuildCompensationPlan(rg, g, cs)
	models := BuildFailureModels(g, cs)

	snap := ExportExecutionSnapshot(plan, safetyResult, contracts, compPlan, models)
	snap.ExecutionPlanHash = "tampered-hash"

	ok, _ := VerifyExecutionSnapshotHash(snap)
	if ok {
		t.Fatal("tampered ExecutionPlanHash must cause hash verification to fail")
	}
}

func TestExportExecutionSnapshot_GeneratedAtExcludedFromHash(t *testing.T) {
	// Same inputs twice → same ExportHash despite different GeneratedAt timestamps.
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	safetyResult := EvaluateExecutionSafety(plan, g, s)
	contracts := BuildExecutionContracts(g, cs)
	compPlan := BuildCompensationPlan(rg, g, cs)
	models := BuildFailureModels(g, cs)

	snap1 := ExportExecutionSnapshot(plan, safetyResult, contracts, compPlan, models)
	snap2 := ExportExecutionSnapshot(plan, safetyResult, contracts, compPlan, models)

	// Manually override timestamps to be different so we can confirm hash is still equal.
	snap1.GeneratedAt = "2024-01-01T00:00:00Z"
	snap2.GeneratedAt = "2025-06-01T12:00:00Z"

	// Recompute via the exported verify function.
	ok1, _ := VerifyExecutionSnapshotHash(snap1)
	ok2, _ := VerifyExecutionSnapshotHash(snap2)
	if !ok1 || !ok2 {
		t.Fatal("GeneratedAt change must not invalidate ExportHash")
	}
}

func TestExportExecutionSnapshot_NilPlan_StillExports(t *testing.T) {
	snap := ExportExecutionSnapshot(nil, nil, nil, nil, nil)
	if snap == nil {
		t.Fatal("ExportExecutionSnapshot must handle nil inputs without panicking")
	}
	if snap.ExportHash == "" {
		t.Fatal("ExportHash must be set even when all inputs are nil")
	}
}

func TestExportExecutionSnapshot_Deterministic(t *testing.T) {
	s := newMultiServiceDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
		"svc-b": planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	safetyResult := EvaluateExecutionSafety(plan, g, s)
	contracts := BuildExecutionContracts(g, cs)
	compPlan := BuildCompensationPlan(rg, g, cs)
	models := BuildFailureModels(g, cs)

	snap1 := ExportExecutionSnapshot(plan, safetyResult, contracts, compPlan, models)
	snap2 := ExportExecutionSnapshot(plan, safetyResult, contracts, compPlan, models)

	if snap1.ExportHash != snap2.ExportHash {
		t.Fatalf("ExportExecutionSnapshot must be deterministic: %q vs %q",
			snap1.ExportHash, snap2.ExportHash)
	}
}
