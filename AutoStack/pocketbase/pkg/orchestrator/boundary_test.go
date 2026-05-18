package orchestrator

import (
	"testing"

	"github.com/janlauber/one-click/pkg/planner"
)

func TestClassifyExecutionBoundary_Delete_Irreversible(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	for _, stage := range plan.Stages {
		for _, n := range stage.Nodes {
			if n.NodeID == "svc-a" {
				tb := ClassifyExecutionBoundary(stage, g)
				if tb.BoundaryClass != BoundaryIrreversible {
					t.Fatalf("delete stage must be BoundaryIrreversible, got %s", tb.BoundaryClass)
				}
				if tb.CanRollback {
					t.Fatal("irreversible stage must not CanRollback")
				}
				if !tb.CanCompensate {
					t.Fatal("irreversible stage must CanCompensate")
				}
				if !tb.RequiresPause {
					t.Fatal("irreversible stage must RequiresPause")
				}
			}
		}
	}
}

func TestClassifyExecutionBoundary_Create_Compensatable(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	for _, stage := range plan.Stages {
		for _, n := range stage.Nodes {
			if n.NodeID == "svc-a" {
				tb := ClassifyExecutionBoundary(stage, g)
				// svc-a has no dependents → compensatable
				if tb.BoundaryClass != BoundaryCompensatable && tb.BoundaryClass != BoundaryOperatorConfirmRequired {
					t.Fatalf("create stage must be compensatable or operator-confirm, got %s", tb.BoundaryClass)
				}
				if !tb.CanRetry {
					t.Fatal("compensatable stage must CanRetry")
				}
			}
		}
	}
}

func TestClassifyExecutionBoundary_NoOp_Atomic(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, nil)
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	for _, stage := range plan.Stages {
		for _, n := range stage.Nodes {
			if n.Action == planner.ActionNoOp {
				tb := ClassifyExecutionBoundary(stage, g)
				if tb.BoundaryClass != BoundaryAtomic {
					t.Fatalf("no-op stage must be BoundaryAtomic, got %s", tb.BoundaryClass)
				}
				if !tb.CanRollback {
					t.Fatal("atomic stage must CanRollback")
				}
				if !tb.CanRetry {
					t.Fatal("atomic stage must CanRetry")
				}
				if tb.RequiresPause {
					t.Fatal("atomic (no-op) stage must not RequiresPause")
				}
			}
		}
	}
}

func TestClassifyNodeBoundary_Delete_Irreversible(t *testing.T) {
	s := newSimpleDesiredState()
	g, _, _, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	class := ClassifyNodeBoundary("svc-a", planner.ActionDelete, g)
	if class != BoundaryIrreversible {
		t.Fatalf("delete node must be BoundaryIrreversible, got %s", class)
	}
}

func TestClassifyNodeBoundary_Create_Compensatable(t *testing.T) {
	s := newSimpleDesiredState()
	g, _, _, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})
	class := ClassifyNodeBoundary("svc-a", planner.ActionCreate, g)
	// svc-a has no reverse deps → compensatable (not operator-confirm)
	if class != BoundaryCompensatable && class != BoundaryOperatorConfirmRequired {
		t.Fatalf("create node must be compensatable or operator-confirm, got %s", class)
	}
}

func TestClassifyNodeBoundary_Rollback_OperatorConfirm(t *testing.T) {
	s := newSimpleDesiredState()
	g, _, _, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionRollback,
	})
	class := ClassifyNodeBoundary("svc-a", planner.ActionRollback, g)
	if class != BoundaryOperatorConfirmRequired {
		t.Fatalf("rollback node must be BoundaryOperatorConfirmRequired, got %s", class)
	}
}

func TestClassifyExecutionBoundary_Justification_NonEmpty(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	for _, stage := range plan.Stages {
		tb := ClassifyExecutionBoundary(stage, g)
		if tb.Justification == "" {
			t.Fatalf("stage %s boundary must have a non-empty justification", stage.StageID)
		}
	}
}

func TestClassifyExecutionBoundary_NodeBoundaries_Populated(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, nil)
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	for _, stage := range plan.Stages {
		tb := ClassifyExecutionBoundary(stage, g)
		if len(tb.NodeBoundaries) != len(stage.Nodes) {
			t.Fatalf("node boundaries count must match stage node count: %d vs %d", len(tb.NodeBoundaries), len(stage.Nodes))
		}
	}
}

func TestClassifyExecutionBoundary_MixedStage_MostRestrictive(t *testing.T) {
	// Stage with both a create and a delete → irreversible wins.
	// Build a single stage with mixed nodes manually.
	stage := ExecutionStage{
		StageID:    "stage-0",
		StageOrder: 0,
		Nodes: []StageNode{
			{NodeID: "svc-a", Action: planner.ActionCreate, Risk: planner.RiskLevelMedium},
			{NodeID: "svc-b", Action: planner.ActionDelete, Risk: planner.RiskLevelCritical},
		},
	}
	s := newMultiServiceDesiredState()
	g, _, _, _ := buildOrchestratorArtifacts(s, nil)
	tb := ClassifyExecutionBoundary(stage, g)
	if tb.BoundaryClass != BoundaryIrreversible {
		t.Fatalf("mixed stage with a delete must be BoundaryIrreversible, got %s", tb.BoundaryClass)
	}
}

func TestClassifyExecutionBoundary_HighRiskUpdate_OperatorConfirm(t *testing.T) {
	services := []planner.ServiceSpec{{
		ID:         "svc-sec",
		Provider:   "aws-ecs",
		SecretRefs: []planner.SecretRefIntent{{Name: "api-key", Source: "aws-secrets-manager"}},
	}}
	s := planner.NewDesiredState(1, planner.EnvironmentProduction, services, nil, nil, nil, nil, planner.RolloutStrategySpec{}, planner.SafetyProfile{}, nil)
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-sec": planner.ActionUpdate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	for _, stage := range plan.Stages {
		if len(stage.Nodes) > 0 && stage.Nodes[0].NodeID == "svc-sec" {
			tb := ClassifyExecutionBoundary(stage, g)
			// High risk update → operator-confirm-required
			if tb.BoundaryClass != BoundaryOperatorConfirmRequired && tb.BoundaryClass != BoundaryCompensatable {
				t.Fatalf("high-risk update must be operator-confirm or compensatable, got %s", tb.BoundaryClass)
			}
		}
	}
}
