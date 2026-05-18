package api

import (
	"testing"

	"github.com/janlauber/one-click/pkg/executor"
	"github.com/janlauber/one-click/pkg/orchestrator"
	"github.com/janlauber/one-click/pkg/planner"
)

// ─── Registry tests ───────────────────────────────────────────────────────────

func TestRegistry_NewRegistry_Empty(t *testing.T) {
	r := NewExecutionRegistry()
	if r.Count() != 0 {
		t.Fatalf("new registry must be empty, got count=%d", r.Count())
	}
}

func TestRegistry_Register_And_Get(t *testing.T) {
	r := NewExecutionRegistry()
	rt := newTestRuntime(t)
	r.Register(rt)

	got, ok := r.Get("exec-1")
	if !ok {
		t.Fatal("Get must return true after Register")
	}
	if got.ExecutionID() != "exec-1" {
		t.Fatalf("Get returned wrong runtime: %s", got.ExecutionID())
	}
}

func TestRegistry_Get_NotFound_ReturnsFalse(t *testing.T) {
	r := NewExecutionRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("Get must return false for unknown execution ID")
	}
}

func TestRegistry_MustGet_NotFound_ReturnsError(t *testing.T) {
	r := NewExecutionRegistry()
	_, err := r.MustGet("nonexistent")
	if err == nil {
		t.Fatal("MustGet must return error for unknown execution ID")
	}
}

func TestRegistry_MustGet_Found_ReturnsRuntime(t *testing.T) {
	r := NewExecutionRegistry()
	rt := newTestRuntime(t)
	r.Register(rt)

	got, err := r.MustGet("exec-1")
	if err != nil {
		t.Fatalf("MustGet must succeed for registered runtime: %v", err)
	}
	if got.ExecutionID() != "exec-1" {
		t.Fatalf("wrong execution ID: %s", got.ExecutionID())
	}
}

func TestRegistry_Register_NilRuntime_Panics(t *testing.T) {
	r := NewExecutionRegistry()
	defer func() {
		if rec := recover(); rec == nil {
			t.Fatal("Register(nil) must panic")
		}
	}()
	r.Register(nil)
}

func TestRegistry_Count_Increments(t *testing.T) {
	r := NewExecutionRegistry()
	r.Register(newTestRuntimeWithID(t, "exec-a"))
	r.Register(newTestRuntimeWithID(t, "exec-b"))
	if r.Count() != 2 {
		t.Fatalf("expected count=2, got %d", r.Count())
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func newTestRuntime(t *testing.T) *executor.ExecutionRuntime {
	t.Helper()
	return newTestRuntimeWithID(t, "exec-1")
}

func newTestRuntimeWithID(t *testing.T, id string) *executor.ExecutionRuntime {
	t.Helper()
	snap := buildMinimalSnapshot(t)
	j := executor.NewInMemoryJournalStore()
	a := executor.NewInMemoryApprovalStore()
	amb := executor.NewInMemoryAmbiguityStore()
	l := executor.NewInMemoryLeaseStore()
	rt, err := executor.NewExecutionRuntime(id, "holder-1", snap, j, a, amb, l, &executor.NoOpProviderExecutor{})
	if err != nil {
		t.Fatalf("NewExecutionRuntime failed: %v", err)
	}
	return rt
}

func buildMinimalSnapshot(t *testing.T) *orchestrator.ExecutionSnapshot {
	t.Helper()
	s := planner.NewDesiredState(
		1, planner.EnvironmentProduction,
		[]planner.ServiceSpec{{ID: "svc-a", Provider: "aws-ecs"}},
		nil, nil, nil, nil,
		planner.RolloutStrategySpec{},
		planner.SafetyProfile{},
		nil,
	)
	g := planner.BuildPlanGraph(s, nil)
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
