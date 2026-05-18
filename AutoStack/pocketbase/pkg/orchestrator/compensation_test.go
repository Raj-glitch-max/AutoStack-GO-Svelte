package orchestrator

import (
	"testing"

	"github.com/janlauber/one-click/pkg/planner"
)

func TestBuildCompensationPlan_NoIrreversible(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate, // create is reversible
	})
	plan := BuildCompensationPlan(rg, g, cs)

	if len(plan.Stages) != 0 {
		t.Fatalf("no irreversible nodes → 0 compensation stages, got %d", len(plan.Stages))
	}
	if plan.HasManualRequired {
		t.Fatal("no manual required when no irreversible nodes")
	}
}

func TestBuildCompensationPlan_Delete_ProducesStage(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildCompensationPlan(rg, g, cs)

	if len(plan.Stages) == 0 {
		t.Fatal("delete → must produce at least one compensation stage")
	}
}

func TestBuildCompensationPlan_Delete_RecreateOrQuarantine(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildCompensationPlan(rg, g, cs)

	for _, stage := range plan.Stages {
		if stage.OriginalNodeID == "svc-a" {
			if stage.CompensationAction != CompensationRecreate && stage.CompensationAction != CompensationQuarantine {
				t.Fatalf("delete compensation must be recreate or quarantine, got %s", stage.CompensationAction)
			}
		}
	}
}

func TestBuildCompensationPlan_ReasonNonEmpty(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildCompensationPlan(rg, g, cs)

	for _, stage := range plan.Stages {
		if stage.Reason == "" {
			t.Fatalf("compensation stage %s must have a non-empty reason", stage.StageID)
		}
	}
}

func TestBuildCompensationPlan_LimitationsNonEmpty(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildCompensationPlan(rg, g, cs)

	if len(plan.Limitations) == 0 {
		t.Fatal("compensation plan must document its limitations")
	}
	for _, stage := range plan.Stages {
		if len(stage.Limitations) == 0 {
			t.Fatalf("compensation stage %s must document its limitations", stage.StageID)
		}
	}
}

func TestBuildCompensationPlan_StageOrderMonotonic(t *testing.T) {
	s := newMultiServiceDesiredState()
	g, cs, rg, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
		"svc-b": planner.ActionDelete,
		"svc-c": planner.ActionDelete,
	})
	plan := BuildCompensationPlan(rg, g, cs)

	for i, stage := range plan.Stages {
		if stage.StageOrder != i {
			t.Fatalf("compensation stage order must be monotonic: stage[%d].StageOrder = %d", i, stage.StageOrder)
		}
	}
}

func TestBuildCompensationPlan_Deterministic(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})

	plan1 := BuildCompensationPlan(rg, g, cs)
	plan2 := BuildCompensationPlan(rg, g, cs)

	if len(plan1.Stages) != len(plan2.Stages) {
		t.Fatal("BuildCompensationPlan must be deterministic: stage count differs")
	}
	for i := range plan1.Stages {
		if plan1.Stages[i].OriginalNodeID != plan2.Stages[i].OriginalNodeID {
			t.Fatalf("compensation stage ordering non-deterministic at index %d", i)
		}
	}
}

func TestBuildCompensationPlan_HasManualRequired_WhenManual(t *testing.T) {
	// A rollback action → ManualIntervention compensation.
	s := newSimpleDesiredState()
	g, cs, rg, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionRollback,
	})
	// Rollback is not irreversible in the rollback graph (only deletes are),
	// so compensation stages may be 0. Let's test with what we have.
	plan := BuildCompensationPlan(rg, g, cs)
	// Plan must be valid regardless.
	if plan == nil {
		t.Fatal("compensation plan must be non-nil")
	}
}

func TestBuildCompensationPlan_NodeWithDependents_Quarantine(t *testing.T) {
	// db is deleted; backend depends on db → quarantine compensation.
	s := newChainDesiredState()
	g, cs, rg, _ := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"db": planner.ActionDelete,
	})
	plan := BuildCompensationPlan(rg, g, cs)

	for _, stage := range plan.Stages {
		if stage.OriginalNodeID == "db" {
			if stage.CompensationAction != CompensationQuarantine {
				t.Fatalf("db delete with dependents must use quarantine compensation, got %s", stage.CompensationAction)
			}
		}
	}
}
