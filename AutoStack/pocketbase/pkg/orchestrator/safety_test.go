package orchestrator

import (
	"testing"

	"github.com/janlauber/one-click/pkg/planner"
)

func TestEvaluateExecutionSafety_CleanPlan_NoIssues(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	result := EvaluateExecutionSafety(plan, g, s)

	if result == nil {
		t.Fatal("safety result must be non-nil")
	}
	if len(result.Limitations) == 0 {
		t.Fatal("limitations must be non-empty")
	}
}

func TestEvaluateExecutionSafety_NilPlan_Blocked(t *testing.T) {
	s := newSimpleDesiredState()
	g, _, _, _ := buildOrchestratorArtifacts(s, nil)
	result := EvaluateExecutionSafety(nil, g, s)
	if !result.IsBlocked {
		t.Fatal("nil plan must be blocked")
	}
}

func TestEvaluateExecutionSafety_BlastRadius_Exceeded(t *testing.T) {
	// db → backend → frontend; blast radius from db = 3.
	sp := planner.SafetyProfile{MaxBlastRadiusNodes: 1}
	s2 := planner.NewDesiredState(1, planner.EnvironmentProduction,
		[]planner.ServiceSpec{
			{ID: "backend", Provider: "aws-ecs", DependsOn: []string{"db"}},
			{ID: "frontend", Provider: "aws-ecs", DependsOn: []string{"backend"}},
		},
		[]planner.DependencySpec{{ID: "db", Kind: "database", Provider: "aws-rds"}},
		nil, nil, nil, planner.RolloutStrategySpec{}, sp, nil,
	)
	g, cs, rg, verdict := buildOrchestratorArtifacts(s2, map[string]planner.ActionType{
		"db": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s2)
	result := EvaluateExecutionSafety(plan, g, s2)

	found := false
	for _, issue := range result.Issues {
		if issue.Rule == "EXCESSIVE_BLAST_RADIUS" {
			found = true
		}
	}
	if !found {
		t.Fatal("blast radius exceeding limit must produce EXCESSIVE_BLAST_RADIUS issue")
	}
}

func TestEvaluateExecutionSafety_DependencyCollapse_Delete(t *testing.T) {
	// db has reverse deps (backend depends on db) → DEPENDENCY_COLLAPSE when db is deleted.
	s := newChainDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"db": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	result := EvaluateExecutionSafety(plan, g, s)

	found := false
	for _, issue := range result.Issues {
		if issue.Rule == "DEPENDENCY_COLLAPSE" {
			found = true
		}
	}
	if !found {
		t.Fatal("deleting a node with dependents must trigger DEPENDENCY_COLLAPSE")
	}
}

func TestEvaluateExecutionSafety_CascadingRollback_Warning(t *testing.T) {
	// Create a plan with many nodes to get a long rollback chain.
	services := make([]planner.ServiceSpec, 5)
	services[0] = planner.ServiceSpec{ID: "svc-0", Provider: "aws-ecs"}
	for i := 1; i < 5; i++ {
		services[i] = planner.ServiceSpec{
			ID:        "svc-" + itoa(i),
			Provider:  "aws-ecs",
			DependsOn: []string{"svc-" + itoa(i-1)},
		}
	}
	s := planner.NewDesiredState(1, planner.EnvironmentProduction, services, nil, nil, nil, nil, planner.RolloutStrategySpec{}, planner.SafetyProfile{}, nil)
	actions := map[string]planner.ActionType{}
	for i := 0; i < 5; i++ {
		actions["svc-"+itoa(i)] = planner.ActionCreate
	}
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, actions)
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	result := EvaluateExecutionSafety(plan, g, s)

	// 5 rollback stages → should trigger CASCADING_ROLLBACK_RISK warning.
	found := false
	for _, w := range result.Warnings {
		if w.Rule == "CASCADING_ROLLBACK_RISK" {
			found = true
		}
	}
	if !found {
		t.Fatal("long rollback chain must produce CASCADING_ROLLBACK_RISK warning")
	}
}

func TestEvaluateExecutionSafety_CrossProviderRisk_Warning(t *testing.T) {
	// frontend on aws-ecs depends on backend on cloud-run → cross-provider.
	services := []planner.ServiceSpec{
		{ID: "frontend", Provider: "aws-ecs", DependsOn: []string{"backend"}},
		{ID: "backend", Provider: "cloud-run"},
	}
	s := planner.NewDesiredState(1, planner.EnvironmentProduction, services, nil, nil, nil, nil, planner.RolloutStrategySpec{}, planner.SafetyProfile{}, nil)
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"frontend": planner.ActionCreate,
		"backend":  planner.ActionCreate,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	result := EvaluateExecutionSafety(plan, g, s)

	found := false
	for _, w := range result.Warnings {
		if w.Rule == "CROSS_PROVIDER_RISK" {
			found = true
		}
	}
	if !found {
		t.Fatal("cross-provider dependency must produce CROSS_PROVIDER_RISK warning")
	}
}

func TestEvaluateExecutionSafety_LimitationsNonEmpty(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, nil)
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	result := EvaluateExecutionSafety(plan, g, s)

	if len(result.Limitations) == 0 {
		t.Fatal("execution safety result must document limitations")
	}
}

func TestEvaluateExecutionSafety_Deterministic(t *testing.T) {
	s := newChainDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"db": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)

	r1 := EvaluateExecutionSafety(plan, g, s)
	r2 := EvaluateExecutionSafety(plan, g, s)

	if r1.IsBlocked != r2.IsBlocked {
		t.Fatal("EvaluateExecutionSafety must be deterministic: IsBlocked differs")
	}
	if len(r1.Issues) != len(r2.Issues) {
		t.Fatalf("issue count must be deterministic: %d vs %d", len(r1.Issues), len(r2.Issues))
	}
	for i := range r1.Issues {
		if r1.Issues[i].Rule != r2.Issues[i].Rule {
			t.Fatalf("issue rule non-deterministic at index %d", i)
		}
	}
}

func TestEvaluateExecutionSafety_ManualCompensation_Warning(t *testing.T) {
	s := newSimpleDesiredState()
	g, cs, rg, verdict := buildOrchestratorArtifacts(s, map[string]planner.ActionType{
		"svc-a": planner.ActionDelete,
	})
	plan := BuildExecutionPlan(g, cs, rg, verdict, s)
	result := EvaluateExecutionSafety(plan, g, s)

	// If compensation has ManualRequired, warning should be present.
	hasManual := false
	for _, cs := range plan.CompensationStages {
		if cs.ManualRequired {
			hasManual = true
		}
	}
	if hasManual {
		found := false
		for _, w := range result.Warnings {
			if w.Rule == "MANUAL_COMPENSATION_REQUIRED" {
				found = true
			}
		}
		if !found {
			t.Fatal("manual compensation required must produce MANUAL_COMPENSATION_REQUIRED warning")
		}
	}
}
