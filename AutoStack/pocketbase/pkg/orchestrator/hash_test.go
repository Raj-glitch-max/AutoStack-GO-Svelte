package orchestrator

import (
	"testing"

	"github.com/janlauber/one-click/pkg/planner"
)

func TestComputeExecutionPlanHash_NonEmpty(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, nil)
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	h := ComputeExecutionPlanHash(
		plan.PlanHash, plan.Stages, plan.RollbackStages,
		plan.CompensationStages, plan.ApprovalGates, nil,
	)
	if h == "" {
		t.Fatal("execution plan hash must be non-empty")
	}
	if len(h) != 64 {
		t.Fatalf("expected 64-char SHA-256 hex, got %d chars", len(h))
	}
}

func TestComputeExecutionPlanHash_Deterministic(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	h1 := ComputeExecutionPlanHash(
		plan.PlanHash, plan.Stages, plan.RollbackStages,
		plan.CompensationStages, plan.ApprovalGates, nil,
	)
	h2 := ComputeExecutionPlanHash(
		plan.PlanHash, plan.Stages, plan.RollbackStages,
		plan.CompensationStages, plan.ApprovalGates, nil,
	)
	if h1 != h2 {
		t.Fatalf("execution plan hash must be deterministic: %q vs %q", h1, h2)
	}
}

func TestComputeExecutionPlanHash_DifferentActions_DifferentHash(t *testing.T) {
	s := newSimpleDesiredState()

	g1, cs1, rg1, verdict1 := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})
	plan1 := BuildExecutionPlan(g1, cs1, rg1, verdict1, s)
	h1 := ComputeExecutionPlanHash(
		plan1.PlanHash, plan1.Stages, plan1.RollbackStages,
		plan1.CompensationStages, plan1.ApprovalGates, nil,
	)

	g2, cs2, rg2, verdict2 := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan2 := BuildExecutionPlan(g2, cs2, rg2, verdict2, s)
	h2 := ComputeExecutionPlanHash(
		plan2.PlanHash, plan2.Stages, plan2.RollbackStages,
		plan2.CompensationStages, plan2.ApprovalGates, nil,
	)

	if h1 == h2 {
		t.Fatal("different actions must produce different execution plan hashes")
	}
}

func TestComputeExecutionPlanHash_DifferentPlanHash_DifferentHash(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, nil)
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	h1 := ComputeExecutionPlanHash(
		"original-plan-hash", plan.Stages, plan.RollbackStages,
		plan.CompensationStages, plan.ApprovalGates, nil,
	)
	h2 := ComputeExecutionPlanHash(
		"different-plan-hash", plan.Stages, plan.RollbackStages,
		plan.CompensationStages, plan.ApprovalGates, nil,
	)
	if h1 == h2 {
		t.Fatal("different plan hash inputs must produce different execution plan hashes")
	}
}

func TestComputeExecutionPlanHash_WithContracts(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	contracts := BuildExecutionContracts(g, cs)

	h1 := ComputeExecutionPlanHash(
		plan.PlanHash, plan.Stages, plan.RollbackStages,
		plan.CompensationStages, plan.ApprovalGates, nil,
	)
	h2 := ComputeExecutionPlanHash(
		plan.PlanHash, plan.Stages, plan.RollbackStages,
		plan.CompensationStages, plan.ApprovalGates, contracts,
	)
	if h1 == h2 {
		t.Fatal("contracts must affect the execution plan hash")
	}
}

func TestComputeExecutionPlanHash_StageOrderAffectsHash(t *testing.T) {
	// Same stages but different StageOrder values → different hash.
	stages1 := []ExecutionStage{
		{StageID: "stage-0", StageOrder: 0, BoundaryClass: BoundaryAtomic},
		{StageID: "stage-1", StageOrder: 1, BoundaryClass: BoundaryAtomic},
	}
	stages2 := []ExecutionStage{
		{StageID: "stage-0", StageOrder: 1, BoundaryClass: BoundaryAtomic}, // order swapped
		{StageID: "stage-1", StageOrder: 0, BoundaryClass: BoundaryAtomic},
	}
	h1 := ComputeExecutionPlanHash("hash", stages1, nil, nil, nil, nil)
	h2 := ComputeExecutionPlanHash("hash", stages2, nil, nil, nil, nil)
	if h1 == h2 {
		t.Fatal("different stage orders must produce different execution plan hashes")
	}
}

func TestExecutionPlanHash_SetOnPlan(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, nil)
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	if plan.ExecutionPlanHash == "" {
		t.Fatal("ExecutionPlan.ExecutionPlanHash must be set by BuildExecutionPlan")
	}
	if len(plan.ExecutionPlanHash) != 64 {
		t.Fatalf("expected 64-char hash, got %d chars", len(plan.ExecutionPlanHash))
	}
}

func TestComputeExecutionPlanHash_EmptyInputs_NonEmpty(t *testing.T) {
	h := ComputeExecutionPlanHash("", nil, nil, nil, nil, nil)
	if h == "" {
		t.Fatal("hash must be non-empty even for empty inputs")
	}
}

func TestItoa_CorrectConversion(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{42, "42"},
		{100, "100"},
	}
	for _, tc := range cases {
		got := itoa(tc.n)
		if got != tc.want {
			t.Errorf("itoa(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
