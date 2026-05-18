package planner

import (
	"testing"
)

func TestBuildPlanGraph_EmptyState(t *testing.T) {
	s := NewDesiredState(1, EnvironmentProduction, nil, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)
	if !g.IsValid() {
		t.Fatal("empty graph must be valid (no cycles)")
	}
	if len(g.Nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Fatalf("expected 0 edges, got %d", len(g.Edges))
	}
}

func TestBuildPlanGraph_SingleNode(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate})
	if !g.IsValid() {
		t.Fatal("single node graph must be valid")
	}
	if len(g.Nodes) < 1 {
		t.Fatal("expected at least 1 node")
	}
	node := g.NodeIndex["svc-a"]
	if node == nil {
		t.Fatal("svc-a must be in node index")
	}
	if node.DesiredAction != ActionCreate {
		t.Fatalf("expected action=create, got %s", node.DesiredAction)
	}
}

func TestBuildPlanGraph_TopologicalOrder_DependenciesFirst(t *testing.T) {
	services := []ServiceSpec{
		{ID: "frontend", Provider: "aws-ecs", DependsOn: []string{"backend"}},
		{ID: "backend", Provider: "aws-ecs", DependsOn: []string{"db"}},
	}
	deps := []DependencySpec{{ID: "db", Kind: "database", Provider: "aws-rds"}}
	s := NewDesiredState(1, EnvironmentProduction, services, deps, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)

	if !g.IsValid() {
		t.Fatal("graph with linear dependency chain must be valid")
	}

	// db must come before backend, backend before frontend.
	order := g.TopologicalOrder
	pos := func(id string) int {
		for i, v := range order {
			if v == id {
				return i
			}
		}
		return -1
	}

	if pos("db") >= pos("backend") {
		t.Error("db must appear before backend in topological order")
	}
	if pos("backend") >= pos("frontend") {
		t.Error("backend must appear before frontend in topological order")
	}
}

func TestBuildPlanGraph_CycleDetection(t *testing.T) {
	services := []ServiceSpec{
		{ID: "a", Provider: "aws-ecs", DependsOn: []string{"b"}},
		{ID: "b", Provider: "aws-ecs", DependsOn: []string{"a"}},
	}
	s := NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)

	if g.IsValid() {
		t.Fatal("graph with a→b→a cycle must be invalid")
	}
	if len(g.CyclicPaths) == 0 {
		t.Fatal("CyclicPaths must be non-empty for a cyclic graph")
	}
	if len(g.TopologicalOrder) != 0 {
		t.Fatal("TopologicalOrder must be empty when cycles are present")
	}
}

func TestBuildPlanGraph_Deterministic(t *testing.T) {
	s := newMultiServiceState()
	g1 := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate, "svc-b": ActionUpdate})
	g2 := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate, "svc-b": ActionUpdate})

	if len(g1.TopologicalOrder) != len(g2.TopologicalOrder) {
		t.Fatal("topological order length must be identical for same input")
	}
	for i := range g1.TopologicalOrder {
		if g1.TopologicalOrder[i] != g2.TopologicalOrder[i] {
			t.Fatalf("topological order[%d] differs: %q vs %q", i, g1.TopologicalOrder[i], g2.TopologicalOrder[i])
		}
	}
}

func TestBuildPlanGraph_OrphanDetection(t *testing.T) {
	// Two services with no dependency edges between them and no deps.
	services := []ServiceSpec{
		{ID: "standalone-a", Provider: "aws-ecs"},
		{ID: "standalone-b", Provider: "aws-ecs"},
	}
	s := NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)

	if len(g.OrphanNodes) != 2 {
		t.Fatalf("expected 2 orphan nodes, got %d: %v", len(g.OrphanNodes), g.OrphanNodes)
	}
	for _, node := range g.Nodes {
		if !node.IsOrphan {
			t.Errorf("node %s should be flagged as orphan", node.ID)
		}
	}
}

func TestBuildPlanGraph_ParallelStages(t *testing.T) {
	// db has no deps → stage 0
	// svc-a depends on db → stage 1
	// svc-b depends on db → stage 1 (parallel with svc-a)
	deps := []DependencySpec{{ID: "db", Kind: "database", Provider: "aws-rds"}}
	services := []ServiceSpec{
		{ID: "svc-a", Provider: "aws-ecs", DependsOn: []string{"db"}},
		{ID: "svc-b", Provider: "aws-ecs", DependsOn: []string{"db"}},
	}
	s := NewDesiredState(1, EnvironmentProduction, services, deps, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)

	if !g.IsValid() {
		t.Fatal("graph must be valid")
	}
	if len(g.ParallelStages) < 2 {
		t.Fatalf("expected at least 2 parallel stages, got %d", len(g.ParallelStages))
	}
	// Stage 0 must contain db only.
	stage0 := g.ParallelStages[0]
	if len(stage0) != 1 || stage0[0] != "db" {
		t.Fatalf("stage 0 must contain [db], got %v", stage0)
	}
	// Stage 1 must contain both svc-a and svc-b.
	stage1 := g.ParallelStages[1]
	if len(stage1) != 2 {
		t.Fatalf("stage 1 must contain 2 nodes (svc-a, svc-b), got %v", stage1)
	}
}

func TestBuildPlanGraph_ReverseDependencies(t *testing.T) {
	services := []ServiceSpec{
		{ID: "frontend", Provider: "aws-ecs", DependsOn: []string{"backend"}},
		{ID: "backend", Provider: "aws-ecs"},
	}
	s := NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)

	backend := g.NodeIndex["backend"]
	if backend == nil {
		t.Fatal("backend node must exist")
	}
	if len(backend.ReverseDependencies) != 1 || backend.ReverseDependencies[0] != "frontend" {
		t.Fatalf("backend.ReverseDependencies must be [frontend], got %v", backend.ReverseDependencies)
	}
}

func TestBuildPlanGraph_UnknownDependencySkipped(t *testing.T) {
	// svc-a declares a dependency on "missing-node" which is not in the state.
	services := []ServiceSpec{
		{ID: "svc-a", Provider: "aws-ecs", DependsOn: []string{"missing-node"}},
	}
	s := NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	// Must not panic; unknown deps are skipped.
	g := BuildPlanGraph(s, nil)
	if !g.IsValid() {
		t.Fatal("graph with skipped unknown dep must still be valid")
	}
}

func TestBuildPlanGraph_DefaultActionIsNoOp(t *testing.T) {
	services := []ServiceSpec{{ID: "svc-a", Provider: "aws-ecs"}}
	s := NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil) // no actions provided
	node := g.NodeIndex["svc-a"]
	if node.DesiredAction != ActionNoOp {
		t.Fatalf("default action when not specified must be no-op, got %s", node.DesiredAction)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────

func newMultiServiceState() *DesiredState {
	services := []ServiceSpec{
		{ID: "svc-a", Provider: "aws-ecs", DependsOn: []string{"svc-b"}},
		{ID: "svc-b", Provider: "aws-ecs"},
		{ID: "svc-c", Provider: "aws-ecs", DependsOn: []string{"svc-b"}},
	}
	return NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil,
		RolloutStrategySpec{}, SafetyProfile{}, nil)
}
