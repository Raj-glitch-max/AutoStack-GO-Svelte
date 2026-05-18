package orchestrator

import (
	"testing"

	"github.com/janlauber/one-click/pkg/planner"
)

func TestBuildExecutionContracts_EmptyGraph(t *testing.T) {
	s := planner.NewDesiredState(1, planner.EnvironmentProduction, nil, nil, nil, nil, nil, planner.RolloutStrategySpec{}, planner.SafetyProfile{}, nil)
	g, cs, _, _ := buildOrchestratorArtifacts(s, nil)
	contracts := BuildExecutionContracts(g, cs)
	if len(contracts) != 0 {
		t.Fatalf("empty graph → 0 contracts, got %d", len(contracts))
	}
}

func TestBuildExecutionContracts_CyclicGraph_ReturnsNil(t *testing.T) {
	services := []planner.ServiceSpec{
		{ID: "a", Provider: "aws-ecs", DependsOn: []string{"b"}},
		{ID: "b", Provider: "aws-ecs", DependsOn: []string{"a"}},
	}
	s := planner.NewDesiredState(1, planner.EnvironmentProduction, services, nil, nil, nil, nil, planner.RolloutStrategySpec{}, planner.SafetyProfile{}, nil)
	g, cs, _, _ := buildOrchestratorArtifacts(s, nil)
	contracts := BuildExecutionContracts(g, cs)
	if contracts != nil {
		t.Fatal("cyclic graph → contracts must be nil")
	}
}

func TestBuildExecutionContracts_OnePerNode(t *testing.T) {
	s := newMultiServiceDesiredState()
	g, cs, _, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
		"svc-b": planner.ActionUpdate,
		"svc-c": planner.ActionDelete,
	})
	contracts := BuildExecutionContracts(g, cs)
	if len(contracts) != len(g.Nodes) {
		t.Fatalf("expected %d contracts (one per node), got %d", len(g.Nodes), len(contracts))
	}
}

func TestBuildExecutionContracts_Delete_IsIrreversible(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, _, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	contracts := BuildExecutionContracts(g, cs)

	for _, c := range contracts {
		if c.NodeID == "svc-a" {
			if c.FailureClassification != FailureClassIrreversible {
				t.Fatalf("delete contract must have FailureClassIrreversible, got %s", c.FailureClassification)
			}
		}
	}
}

func TestBuildExecutionContracts_Create_IsRetryable(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, _, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})
	contracts := BuildExecutionContracts(g, cs)

	for _, c := range contracts {
		if c.NodeID == "svc-a" {
			if c.FailureClassification != FailureClassRetryable {
				t.Fatalf("basic create contract must have FailureClassRetryable, got %s", c.FailureClassification)
			}
		}
	}
}

func TestBuildExecutionContracts_PreconditionsNonEmpty(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, _, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})
	contracts := BuildExecutionContracts(g, cs)

	for _, c := range contracts {
		if c.NodeID == "svc-a" && len(c.Preconditions) == 0 {
			t.Fatal("create contract must have at least one precondition")
		}
	}
}

func TestBuildExecutionContracts_PostconditionsNonEmpty(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, _, _ := buildOrchestratorArtifacts(s, nil)
	contracts := BuildExecutionContracts(g, cs)

	for _, c := range contracts {
		if len(c.Postconditions) == 0 {
			t.Fatalf("contract for node %s must have at least one postcondition", c.NodeID)
		}
	}
}

func TestBuildExecutionContracts_RollbackConditionsNonEmpty(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, _, _ := buildOrchestratorArtifacts(s, nil)
	contracts := BuildExecutionContracts(g, cs)

	for _, c := range contracts {
		if len(c.RollbackConditions) == 0 {
			t.Fatalf("contract for node %s must have rollback conditions", c.NodeID)
		}
	}
}

func TestBuildExecutionContracts_Delete_Idempotent(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, _, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	contracts := BuildExecutionContracts(g, cs)

	for _, c := range contracts {
		if c.NodeID == "svc-a" {
			if !c.IdempotencyRequirements.IsIdempotent {
				t.Fatal("delete is idempotent (provider typically accepts delete-of-non-existent)")
			}
		}
	}
}

func TestBuildExecutionContracts_Create_NotIdempotent(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, _, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})
	contracts := BuildExecutionContracts(g, cs)

	for _, c := range contracts {
		if c.NodeID == "svc-a" {
			if c.IdempotencyRequirements.IsIdempotent {
				t.Fatal("create is NOT idempotent (may fail if resource already exists)")
			}
		}
	}
}

func TestBuildExecutionContracts_AmbiguousStateNonEmpty(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, _, _ := buildOrchestratorArtifacts(s, nil)
	contracts := BuildExecutionContracts(g, cs)

	for _, c := range contracts {
		if c.AmbiguousStateDefinition == "" {
			t.Fatalf("contract for node %s must define ambiguous state", c.NodeID)
		}
	}
}

func TestBuildExecutionContracts_LimitationsNonEmpty(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, _, _ := buildOrchestratorArtifacts(s, nil)
	contracts := BuildExecutionContracts(g, cs)

	for _, c := range contracts {
		if len(c.Limitations) == 0 {
			t.Fatalf("contract for node %s must have limitations", c.NodeID)
		}
	}
}

func TestBuildExecutionContracts_Deterministic(t *testing.T) {
	s := newMultiServiceDesiredState()
	g, cs, _, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})

	c1 := BuildExecutionContracts(g, cs)
	c2 := BuildExecutionContracts(g, cs)

	if len(c1) != len(c2) {
		t.Fatal("BuildExecutionContracts must be deterministic: count differs")
	}
	for i := range c1 {
		if c1[i].NodeID != c2[i].NodeID {
			t.Fatalf("contract ordering non-deterministic at index %d", i)
		}
		if c1[i].FailureClassification != c2[i].FailureClassification {
			t.Fatalf("failure class non-deterministic for node %s", c1[i].NodeID)
		}
	}
}

func TestBuildExecutionContracts_DeleteHasPreconditionWithConfirmation(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, _, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	contracts := BuildExecutionContracts(g, cs)

	for _, c := range contracts {
		if c.NodeID == "svc-a" {
			hasConfirmPrecondition := false
			for _, pc := range c.Preconditions {
				if pc.Blocking {
					hasConfirmPrecondition = true
				}
			}
			if !hasConfirmPrecondition {
				t.Fatal("delete contract must have a blocking precondition requiring operator confirmation")
			}
		}
	}
}
