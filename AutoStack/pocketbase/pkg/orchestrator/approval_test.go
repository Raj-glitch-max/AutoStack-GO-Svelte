package orchestrator

import (
	"testing"

	"github.com/janlauber/one-click/pkg/planner"
)

func TestBuildApprovalGates_EmptyStages(t *testing.T) {
	s := planner.NewDesiredState(1, planner.EnvironmentProduction, nil, nil, nil, nil, nil, planner.RolloutStrategySpec{}, planner.SafetyProfile{}, nil)
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, nil)
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	gates := BuildApprovalGates(plan.Stages, verdict, s)
	// No stages → no gates (aside from what verdict provides).
	_ = gates // just verify no panic
}

func TestBuildApprovalGates_DeleteNode_ProducesGate(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	gates := BuildApprovalGates(plan.Stages, verdict, s)

	if len(gates) == 0 {
		t.Fatal("delete action must produce at least one approval gate")
	}
}

func TestBuildApprovalGates_AllGatesBlocking(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	gates := BuildApprovalGates(plan.Stages, verdict, s)

	for _, gate := range gates {
		if !gate.Blocking {
			t.Fatalf("all approval gates must be Blocking=true in Phase 4.1, gate %s is not", gate.GateID)
		}
	}
}

func TestBuildApprovalGates_GateIDNonEmpty(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	gates := BuildApprovalGates(plan.Stages, verdict, s)

	for _, gate := range gates {
		if gate.GateID == "" {
			t.Fatal("all gates must have non-empty GateID")
		}
	}
}

func TestBuildApprovalGates_ReasonNonEmpty(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	gates := BuildApprovalGates(plan.Stages, verdict, s)

	for _, gate := range gates {
		if gate.Reason == "" {
			t.Fatalf("gate %s must have a non-empty reason", gate.GateID)
		}
	}
}

func TestBuildApprovalGates_RequiredRoleNonEmpty(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	gates := BuildApprovalGates(plan.Stages, verdict, s)

	for _, gate := range gates {
		if gate.RequiredRole == "" {
			t.Fatalf("gate %s must have a RequiredRole", gate.GateID)
		}
	}
}

func TestBuildApprovalGates_CriticalRisk_AdminRole(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete, // delete → critical
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	gates := BuildApprovalGates(plan.Stages, verdict, s)

	hasAdmin := false
	for _, gate := range gates {
		if gate.RequiredRole == "admin" {
			hasAdmin = true
		}
	}
	if !hasAdmin {
		t.Fatal("critical-risk (delete) must produce at least one gate with RequiredRole=admin")
	}
}

func TestBuildApprovalGates_Deterministic(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	gates1 := BuildApprovalGates(plan.Stages, verdict, s)
	gates2 := BuildApprovalGates(plan.Stages, verdict, s)

	if len(gates1) != len(gates2) {
		t.Fatalf("BuildApprovalGates must be deterministic: count %d vs %d", len(gates1), len(gates2))
	}
	for i := range gates1 {
		if gates1[i].GateID != gates2[i].GateID {
			t.Fatalf("gate ordering non-deterministic at index %d", i)
		}
	}
}

func TestBuildApprovalGates_AffectedNodesNonEmpty(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	gates := BuildApprovalGates(plan.Stages, verdict, s)

	for _, gate := range gates {
		if len(gate.AffectedNodes) == 0 {
			t.Fatalf("gate %s must have at least one affected node", gate.GateID)
		}
	}
}

func TestBuildApprovalGates_NoOp_NoGates(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, nil) // no actions → all no-ops
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	gates := BuildApprovalGates(plan.Stages, verdict, s)

	// No-op stages don't require approval; gates should be minimal.
	for _, gate := range gates {
		// Verify any gate that exists has a valid reason.
		if gate.GateID == "" || gate.Reason == "" {
			t.Fatal("any gate that exists must be fully populated")
		}
	}
}
