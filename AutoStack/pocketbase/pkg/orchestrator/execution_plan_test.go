package orchestrator

import (
	"strings"
	"testing"

	"github.com/janlauber/one-click/pkg/planner"
)

func TestBuildExecutionPlan_EmptyGraph(t *testing.T) {
	s := planner.NewDesiredState(1, planner.EnvironmentProduction, nil, nil, nil, nil, nil, planner.RolloutStrategySpec{}, planner.SafetyProfile{}, nil)
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, nil)
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	if plan == nil {
		t.Fatal("plan must be non-nil")
	}
	if plan.SchemaVersion != ExecutionPlanSchemaVersion {
		t.Fatalf("expected SchemaVersion=%q, got %q", ExecutionPlanSchemaVersion, plan.SchemaVersion)
	}
	if len(plan.Stages) != 0 {
		t.Fatalf("empty graph → 0 stages, got %d", len(plan.Stages))
	}
}

func TestBuildExecutionPlan_CyclicGraph_Blocked(t *testing.T) {
	services := []planner.ServiceSpec{
		{ID: "a", Provider: "aws-ecs", DependsOn: []string{"b"}},
		{ID: "b", Provider: "aws-ecs", DependsOn: []string{"a"}},
	}
	s := planner.NewDesiredState(1, planner.EnvironmentProduction, services, nil, nil, nil, nil, planner.RolloutStrategySpec{}, planner.SafetyProfile{}, nil)
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, nil)
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	hasBlock := false
	for _, c := range plan.SafetyConstraints {
		if c.Severity == "block" && c.Triggered {
			hasBlock = true
		}
	}
	if !hasBlock {
		t.Fatal("cyclic graph must produce a blocking safety constraint")
	}
}

func TestBuildExecutionPlan_StageCount(t *testing.T) {
	// db → backend → frontend: should produce 3 stages.
	s := newChainDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"db":       planner.ActionCreate,
		"backend":  planner.ActionCreate,
		"frontend": planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	if len(plan.Stages) != 3 {
		t.Fatalf("linear chain → 3 stages, got %d", len(plan.Stages))
	}
}

func TestBuildExecutionPlan_StageDependenciesWired(t *testing.T) {
	s := newChainDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"db": planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	// Stage 0 must have no dependencies.
	if len(plan.Stages) > 0 && len(plan.Stages[0].Dependencies) != 0 {
		t.Fatalf("stage-0 must have no dependencies, got %v", plan.Stages[0].Dependencies)
	}
	// Stage 1 must depend on stage-0.
	if len(plan.Stages) > 1 {
		if len(plan.Stages[1].Dependencies) == 0 || !strings.Contains(plan.Stages[1].Dependencies[0], "stage-0") {
			t.Fatalf("stage-1 must depend on stage-0, got %v", plan.Stages[1].Dependencies)
		}
	}
}

func TestBuildExecutionPlan_StageOrderIsMonotonic(t *testing.T) {
	s := newChainDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, nil)
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	for i, stage := range plan.Stages {
		if stage.StageOrder != i {
			t.Fatalf("stage order must be monotonic: stage[%d].StageOrder = %d", i, stage.StageOrder)
		}
	}
}

func TestBuildExecutionPlan_DeleteStageRequiresApproval(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	found := false
	for _, stage := range plan.Stages {
		for _, node := range stage.Nodes {
			if node.NodeID == "svc-a" && stage.RequiresApproval {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("stage containing a delete node must require approval")
	}
}

func TestBuildExecutionPlan_RollbackStagesPresent(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
		"db-a":  planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	if len(plan.RollbackStages) == 0 {
		t.Fatal("actionable plan must have rollback stages")
	}
}

func TestBuildExecutionPlan_RollbackStagesOrdered(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
		"db-a":  planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	for i, rs := range plan.RollbackStages {
		if rs.StageOrder != i {
			t.Fatalf("rollback stage order must be monotonic: rollback[%d].StageOrder = %d", i, rs.StageOrder)
		}
	}
}

func TestBuildExecutionPlan_CompensationStages_ForDeletes(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	if len(plan.CompensationStages) == 0 {
		t.Fatal("delete action must produce compensation stages")
	}
}

func TestBuildExecutionPlan_ApprovalGates_Present(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	if len(plan.ApprovalGates) == 0 {
		t.Fatal("plan with delete actions must have approval gates")
	}
}

func TestBuildExecutionPlan_ExecutionPolicies_Present(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, nil)
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	if len(plan.ExecutionPolicies) == 0 {
		t.Fatal("execution plan must always have at least one execution policy")
	}
}

func TestBuildExecutionPlan_Limitations_NonEmpty(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, nil)
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	if len(plan.Limitations) == 0 {
		t.Fatal("execution plan must always document its limitations")
	}
}

func TestBuildExecutionPlan_RiskSummary_Correct(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	if plan.RiskSummary.TotalStages != len(plan.Stages) {
		t.Fatalf("RiskSummary.TotalStages (%d) must equal len(Stages) (%d)", plan.RiskSummary.TotalStages, len(plan.Stages))
	}
}

func TestBuildExecutionPlan_Deterministic(t *testing.T) {
	s := newMultiServiceDesiredState()
	actions := map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
		"svc-b": planner.ActionUpdate,
		"svc-c": planner.ActionCreate,
	}
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, actions)

	plan1 := BuildExecutionPlan(g, cs, rg, verdict, s)
	plan2 := BuildExecutionPlan(g, cs, rg, verdict, s)

	if plan1.ExecutionPlanHash != plan2.ExecutionPlanHash {
		t.Fatalf("ExecutionPlanHash must be deterministic: %q vs %q", plan1.ExecutionPlanHash, plan2.ExecutionPlanHash)
	}
	if len(plan1.Stages) != len(plan2.Stages) {
		t.Fatal("stage count must be deterministic")
	}
}

func TestBuildExecutionPlan_IrreversibleBoundary_ForDelete(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	found := false
	for _, stage := range plan.Stages {
		for _, node := range stage.Nodes {
			if node.NodeID == "svc-a" && stage.BoundaryClass == BoundaryIrreversible {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("stage containing a delete node must have BoundaryIrreversible")
	}
}

func TestBuildExecutionPlan_RollbackRefsWired(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	// At least one forward stage should have rollback refs.
	hasRefs := false
	for _, stage := range plan.Stages {
		if len(stage.RollbackStageRefs) > 0 {
			hasRefs = true
		}
	}
	if !hasRefs {
		t.Fatal("forward stages must have rollback refs wired for actionable nodes")
	}
}
