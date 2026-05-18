package orchestrator

import (
	"github.com/janlauber/one-click/pkg/planner"
)

// ─────────────────────────────────────────────────────────────────────────
// SHARED TEST HELPERS
// ─────────────────────────────────────────────────────────────────────────

func newSimpleDesiredState() *planner.DesiredState {
	services := []planner.ServiceSpec{{
		ID:       "svc-a",
		Provider: "aws-ecs",
		Region:   "us-east-1",
		Compute:  planner.ComputeIntent{MemoryLimitMB: 512},
	}}
	deps := []planner.DependencySpec{{
		ID:       "db-a",
		Kind:     "database",
		Provider: "aws-rds",
	}}
	return planner.NewDesiredState(
		1,
		planner.EnvironmentProduction,
		services,
		deps,
		nil, nil, nil,
		planner.RolloutStrategySpec{Mode: "rolling"},
		planner.SafetyProfile{},
		nil,
	)
}

func newMultiServiceDesiredState() *planner.DesiredState {
	services := []planner.ServiceSpec{
		{ID: "svc-a", Provider: "aws-ecs", DependsOn: []string{"svc-b"}},
		{ID: "svc-b", Provider: "aws-ecs"},
		{ID: "svc-c", Provider: "aws-ecs", DependsOn: []string{"svc-b"}},
	}
	return planner.NewDesiredState(
		1,
		planner.EnvironmentProduction,
		services,
		nil, nil, nil, nil,
		planner.RolloutStrategySpec{},
		planner.SafetyProfile{},
		nil,
	)
}

func newChainDesiredState() *planner.DesiredState {
	// db → backend → frontend: strict linear chain.
	deps := []planner.DependencySpec{{ID: "db", Kind: "database", Provider: "aws-rds"}}
	services := []planner.ServiceSpec{
		{ID: "backend", Provider: "aws-ecs", DependsOn: []string{"db"}},
		{ID: "frontend", Provider: "aws-ecs", DependsOn: []string{"backend"}},
	}
	return planner.NewDesiredState(
		1,
		planner.EnvironmentProduction,
		services,
		deps,
		nil, nil, nil,
		planner.RolloutStrategySpec{},
		planner.SafetyProfile{},
		nil,
	)
}

func buildOrchestratorArtifacts(s *planner.DesiredState, actions map[string]planner.ActionType) (
	*planner.PlanGraph,
	*planner.ChangeSet,
	*planner.RollbackGraph,
	*planner.SafetyVerdict,
) {
	g := planner.BuildPlanGraph(s, actions)
	cs := planner.BuildChangeSet(g, s)
	rg := planner.BuildRollbackGraph(cs, g)
	sim := planner.SimulatePlan(g, cs, s)
	verdict := planner.EvaluateSafety(g, cs, s, sim)
	return g, cs, rg, verdict
}
