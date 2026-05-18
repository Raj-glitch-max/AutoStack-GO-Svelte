package orchestrator

import (
	"testing"

	"github.com/janlauber/one-click/pkg/planner"
)

func TestClassifyFailure_Delete_Irreversible(t *testing.T) {
	fc := ClassifyFailure(planner.ActionDelete, planner.RiskLevelCritical)
	if fc != FailureClassIrreversible {
		t.Fatalf("delete must always be irreversible, got %s", fc)
	}
}

func TestClassifyFailure_Delete_AnyRisk_Irreversible(t *testing.T) {
	for _, risk := range []planner.RiskLevel{planner.RiskLevelLow, planner.RiskLevelMedium, planner.RiskLevelHigh, planner.RiskLevelCritical} {
		fc := ClassifyFailure(planner.ActionDelete, risk)
		if fc != FailureClassIrreversible {
			t.Fatalf("delete at risk=%s must be irreversible, got %s", risk, fc)
		}
	}
}

func TestClassifyFailure_Create_Low_Retryable(t *testing.T) {
	fc := ClassifyFailure(planner.ActionCreate, planner.RiskLevelLow)
	if fc != FailureClassRetryable {
		t.Fatalf("create/low must be retryable, got %s", fc)
	}
}

func TestClassifyFailure_Create_Critical_ProviderAmbiguous(t *testing.T) {
	fc := ClassifyFailure(planner.ActionCreate, planner.RiskLevelCritical)
	if fc != FailureClassProviderAmbiguous {
		t.Fatalf("create/critical must be provider-ambiguous, got %s", fc)
	}
}

func TestClassifyFailure_Update_High_RollbackRequired(t *testing.T) {
	fc := ClassifyFailure(planner.ActionUpdate, planner.RiskLevelHigh)
	if fc != FailureClassRollbackRequired {
		t.Fatalf("update/high must be rollback-required, got %s", fc)
	}
}

func TestClassifyFailure_Update_Critical_RollbackRequired(t *testing.T) {
	fc := ClassifyFailure(planner.ActionUpdate, planner.RiskLevelCritical)
	if fc != FailureClassRollbackRequired {
		t.Fatalf("update/critical must be rollback-required, got %s", fc)
	}
}

func TestClassifyFailure_Update_Medium_Compensatable(t *testing.T) {
	fc := ClassifyFailure(planner.ActionUpdate, planner.RiskLevelMedium)
	if fc != FailureClassCompensatable {
		t.Fatalf("update/medium must be compensatable, got %s", fc)
	}
}

func TestClassifyFailure_Update_Low_Retryable(t *testing.T) {
	fc := ClassifyFailure(planner.ActionUpdate, planner.RiskLevelLow)
	if fc != FailureClassRetryable {
		t.Fatalf("update/low must be retryable, got %s", fc)
	}
}

func TestClassifyFailure_Rollback_High_OperatorRequired(t *testing.T) {
	fc := ClassifyFailure(planner.ActionRollback, planner.RiskLevelHigh)
	if fc != FailureClassOperatorRequired {
		t.Fatalf("rollback/high must be operator-required, got %s", fc)
	}
}

func TestClassifyFailure_Rollback_Low_RollbackRequired(t *testing.T) {
	fc := ClassifyFailure(planner.ActionRollback, planner.RiskLevelLow)
	if fc != FailureClassRollbackRequired {
		t.Fatalf("rollback/low must be rollback-required, got %s", fc)
	}
}

func TestClassifyFailure_NoOp_Transient(t *testing.T) {
	fc := ClassifyFailure(planner.ActionNoOp, planner.RiskLevelLow)
	if fc != FailureClassTransient {
		t.Fatalf("no-op must be transient, got %s", fc)
	}
}

func TestClassifyFailure_Simulate_Transient(t *testing.T) {
	fc := ClassifyFailure(planner.ActionSimulate, planner.RiskLevelLow)
	if fc != FailureClassTransient {
		t.Fatalf("simulate must be transient, got %s", fc)
	}
}

func TestBuildFailureModels_EmptyGraph(t *testing.T) {
	s := planner.NewDesiredState(1, planner.EnvironmentProduction, nil, nil, nil, nil, nil, planner.RolloutStrategySpec{}, planner.SafetyProfile{}, nil)
	g, cs, _, _ := buildOrchestratorArtifacts(s, nil)
	models := BuildFailureModels(g, cs)
	if len(models) != 0 {
		t.Fatalf("empty graph → 0 failure models, got %d", len(models))
	}
}

func TestBuildFailureModels_CyclicGraph_ReturnsNil(t *testing.T) {
	services := []planner.ServiceSpec{
		{ID: "a", Provider: "aws-ecs", DependsOn: []string{"b"}},
		{ID: "b", Provider: "aws-ecs", DependsOn: []string{"a"}},
	}
	s := planner.NewDesiredState(1, planner.EnvironmentProduction, services, nil, nil, nil, nil, planner.RolloutStrategySpec{}, planner.SafetyProfile{}, nil)
	g, cs, _, _ := buildOrchestratorArtifacts(s, nil)
	models := BuildFailureModels(g, cs)
	if models != nil {
		t.Fatal("cyclic graph → models must be nil")
	}
}

func TestBuildFailureModels_OnePerNode(t *testing.T) {
	s := newMultiServiceDesiredState()
	g, cs, _, _ := buildOrchestratorArtifacts(s, nil)
	models := BuildFailureModels(g, cs)
	if len(models) != len(g.Nodes) {
		t.Fatalf("expected %d failure models, got %d", len(g.Nodes), len(models))
	}
}

func TestBuildFailureModels_CanAutoRecover_AlwaysFalse(t *testing.T) {
	s := newMultiServiceDesiredState()
	g, cs, _, _ := buildOrchestratorArtifacts(s, nil)
	models := BuildFailureModels(g, cs)
	for _, m := range models {
		if m.CanAutoRecover {
			t.Fatalf("CanAutoRecover must be false for all models in Phase 4.1, got true for %s", m.NodeID)
		}
	}
}

func TestBuildFailureModels_HandlingGuidanceNonEmpty(t *testing.T) {
	s := newMultiServiceDesiredState()
	g, cs, _, _ := buildOrchestratorArtifacts(s, nil)
	models := BuildFailureModels(g, cs)
	for _, m := range models {
		if m.HandlingGuidance == "" {
			t.Fatalf("handling guidance must be non-empty for node %s", m.NodeID)
		}
	}
}

func TestBuildFailureModels_Deterministic(t *testing.T) {
	s := newMultiServiceDesiredState()
	g, cs, _, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})

	m1 := BuildFailureModels(g, cs)
	m2 := BuildFailureModels(g, cs)

	if len(m1) != len(m2) {
		t.Fatal("BuildFailureModels must be deterministic: count differs")
	}
	for i := range m1 {
		if m1[i].NodeID != m2[i].NodeID {
			t.Fatalf("failure model ordering non-deterministic at index %d", i)
		}
		if m1[i].FailureClass != m2[i].FailureClass {
			t.Fatalf("failure class non-deterministic for node %s", m1[i].NodeID)
		}
	}
}
