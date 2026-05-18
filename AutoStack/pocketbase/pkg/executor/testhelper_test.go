package executor

import (
	"testing"

	"github.com/janlauber/one-click/pkg/orchestrator"
	"github.com/janlauber/one-click/pkg/planner"
)

// ─────────────────────────────────────────────────────────────────────────
// SNAPSHOT BUILDERS
// ─────────────────────────────────────────────────────────────────────────

func newNoOpSnapshot(t *testing.T) *orchestrator.ExecutionSnapshot {
	t.Helper()
	s := planner.NewDesiredState(
		1, planner.EnvironmentProduction,
		[]planner.ServiceSpec{{ID: "svc-a", Provider: "aws-ecs"}},
		nil, nil, nil, nil,
		planner.RolloutStrategySpec{},
		planner.SafetyProfile{},
		nil,
	)
	return buildSnapshot(t, s, nil)
}

func newCreateSnapshot(t *testing.T) *orchestrator.ExecutionSnapshot {
	t.Helper()
	s := planner.NewDesiredState(
		1, planner.EnvironmentProduction,
		[]planner.ServiceSpec{{ID: "svc-a", Provider: "aws-ecs"}},
		nil, nil, nil, nil,
		planner.RolloutStrategySpec{},
		planner.SafetyProfile{},
		nil,
	)
	return buildSnapshot(t, s, map[string]planner.ActionType{"svc-a": planner.ActionCreate})
}

func newDeleteSnapshot(t *testing.T) *orchestrator.ExecutionSnapshot {
	t.Helper()
	s := planner.NewDesiredState(
		1, planner.EnvironmentProduction,
		[]planner.ServiceSpec{{ID: "svc-a", Provider: "aws-ecs"}},
		nil, nil, nil, nil,
		planner.RolloutStrategySpec{},
		planner.SafetyProfile{},
		nil,
	)
	return buildSnapshot(t, s, map[string]planner.ActionType{"svc-a": planner.ActionDelete})
}

func newChainCreateSnapshot(t *testing.T) *orchestrator.ExecutionSnapshot {
	t.Helper()
	s := planner.NewDesiredState(
		1, planner.EnvironmentProduction,
		[]planner.ServiceSpec{
			{ID: "svc-a", Provider: "aws-ecs"},
			{ID: "svc-b", Provider: "aws-ecs", DependsOn: []string{"svc-a"}},
		},
		nil, nil, nil, nil,
		planner.RolloutStrategySpec{},
		planner.SafetyProfile{},
		nil,
	)
	return buildSnapshot(t, s, map[string]planner.ActionType{
		"svc-a": planner.ActionCreate,
		"svc-b": planner.ActionCreate,
	})
}

func buildSnapshot(t *testing.T, s *planner.DesiredState, actions map[string]planner.ActionType) *orchestrator.ExecutionSnapshot {
	t.Helper()
	g := planner.BuildPlanGraph(s, actions)
	cs := planner.BuildChangeSet(g, s)
	rg := planner.BuildRollbackGraph(cs, g)
	sim := planner.SimulatePlan(g, cs, s)
	verdict := planner.EvaluateSafety(g, cs, s, sim)
	plan := orchestrator.BuildExecutionPlan(g, cs, rg, verdict, s)
	safety := orchestrator.EvaluateExecutionSafety(plan, g, s)
	contracts := orchestrator.BuildExecutionContracts(g, cs)
	compPlan := orchestrator.BuildCompensationPlan(rg, g, cs)
	models := orchestrator.BuildFailureModels(g, cs)
	return orchestrator.ExportExecutionSnapshot(plan, safety, contracts, compPlan, models)
}

// ─────────────────────────────────────────────────────────────────────────
// STORE BUILDERS
// ─────────────────────────────────────────────────────────────────────────

func newTestStores() (JournalStore, ApprovalStore, AmbiguityStore, LeaseStore) {
	return NewInMemoryJournalStore(), NewInMemoryApprovalStore(), NewInMemoryAmbiguityStore(), NewInMemoryLeaseStore()
}

// ─────────────────────────────────────────────────────────────────────────
// RUNTIME BUILDERS
// ─────────────────────────────────────────────────────────────────────────

func newTestRuntime(t *testing.T, snap *orchestrator.ExecutionSnapshot, provider ProviderExecutor) *ExecutionRuntime {
	t.Helper()
	j, a, amb, l := newTestStores()
	rt, err := NewExecutionRuntime("exec-1", "holder-1", snap, j, a, amb, l, provider)
	if err != nil {
		t.Fatalf("failed to create runtime: %v", err)
	}
	return rt
}

func newStartedRuntime(t *testing.T, snap *orchestrator.ExecutionSnapshot, provider ProviderExecutor) *ExecutionRuntime {
	t.Helper()
	rt := newTestRuntime(t, snap, provider)
	if err := rt.Start(); err != nil {
		t.Fatalf("runtime.Start failed: %v", err)
	}
	return rt
}

// ─────────────────────────────────────────────────────────────────────────
// EXECUTOR BUILDER
// ─────────────────────────────────────────────────────────────────────────

func newTestExecutor(provider ProviderExecutor) *Executor {
	j, a, amb, l := newTestStores()
	return NewExecutor(j, a, amb, l, provider)
}
