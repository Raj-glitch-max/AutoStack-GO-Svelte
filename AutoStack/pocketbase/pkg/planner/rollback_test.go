package planner

import (
	"testing"
)

func TestBuildRollbackGraph_EmptyChangeSet(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	rg := BuildRollbackGraph(cs, g)

	if len(rg.Steps) != 0 {
		t.Fatalf("empty changeset → 0 rollback steps, got %d", len(rg.Steps))
	}
}

func TestBuildRollbackGraph_DeleteIsIrreversible(t *testing.T) {
	s := newTestDesiredState()
	actions := map[string]ActionType{"svc-a": ActionDelete}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)
	rg := BuildRollbackGraph(cs, g)

	if !rg.HasIrreversible {
		t.Fatal("HasIrreversible must be true when a delete action is present")
	}
	if len(rg.IrreversibleNodes) == 0 {
		t.Fatal("IrreversibleNodes must be non-empty for delete actions")
	}

	found := false
	for _, step := range rg.Steps {
		if step.NodeID == "svc-a" {
			if !step.Irreversible {
				t.Fatal("delete step must be marked Irreversible")
			}
			if step.IrreversibleReason == "" {
				t.Fatal("IrreversibleReason must be non-empty")
			}
			if step.ReverseAction != ActionCreate {
				t.Fatalf("reverse of delete must be create, got %s", step.ReverseAction)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("svc-a must have a rollback step")
	}
}

func TestBuildRollbackGraph_CreateReverseIsDelete(t *testing.T) {
	s := newTestDesiredState()
	actions := map[string]ActionType{"svc-a": ActionCreate}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)
	rg := BuildRollbackGraph(cs, g)

	for _, step := range rg.Steps {
		if step.NodeID == "svc-a" {
			if step.ReverseAction != ActionDelete {
				t.Fatalf("reverse of create must be delete, got %s", step.ReverseAction)
			}
			if step.Irreversible {
				t.Fatal("create step must not be marked Irreversible")
			}
		}
	}
}

func TestBuildRollbackGraph_UpdateReverseIsRollback(t *testing.T) {
	s := newTestDesiredState()
	actions := map[string]ActionType{"svc-a": ActionUpdate}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)
	rg := BuildRollbackGraph(cs, g)

	for _, step := range rg.Steps {
		if step.NodeID == "svc-a" {
			if step.ReverseAction != ActionRollback {
				t.Fatalf("reverse of update must be rollback, got %s", step.ReverseAction)
			}
		}
	}
}

func TestBuildRollbackGraph_OrderingIsReverseOfApply(t *testing.T) {
	// db applied first, backend second.
	// Rollback must be: backend first, db second.
	deps := []DependencySpec{{ID: "db", Kind: "database", Provider: "aws-rds"}}
	services := []ServiceSpec{
		{ID: "backend", Provider: "aws-ecs", DependsOn: []string{"db"}},
	}
	s := NewDesiredState(1, EnvironmentProduction, services, deps, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	actions := map[string]ActionType{
		"db":      ActionCreate,
		"backend": ActionCreate,
	}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)
	rg := BuildRollbackGraph(cs, g)

	if len(rg.Ordering) < 2 {
		t.Fatalf("expected at least 2 nodes in rollback ordering, got %d", len(rg.Ordering))
	}

	// backend must be rolled back before db.
	backendPos, dbPos := -1, -1
	for i, id := range rg.Ordering {
		if id == "backend" {
			backendPos = i
		}
		if id == "db" {
			dbPos = i
		}
	}
	if backendPos == -1 || dbPos == -1 {
		t.Fatal("both backend and db must appear in rollback ordering")
	}
	if backendPos >= dbPos {
		t.Fatalf("backend must be rolled back before db (applied after db); backendPos=%d dbPos=%d", backendPos, dbPos)
	}
}

func TestBuildRollbackGraph_MustRollbackBefore(t *testing.T) {
	// db ← backend: rolling back db requires backend to be rolled back first.
	deps := []DependencySpec{{ID: "db", Kind: "database", Provider: "aws-rds"}}
	services := []ServiceSpec{
		{ID: "backend", Provider: "aws-ecs", DependsOn: []string{"db"}},
	}
	s := NewDesiredState(1, EnvironmentProduction, services, deps, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	actions := map[string]ActionType{"db": ActionCreate, "backend": ActionCreate}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)
	rg := BuildRollbackGraph(cs, g)

	for _, step := range rg.Steps {
		if step.NodeID == "db" {
			found := false
			for _, dep := range step.MustRollbackBefore {
				if dep == "backend" {
					found = true
				}
			}
			if !found {
				t.Fatal("db rollback step must list backend in MustRollbackBefore")
			}
		}
	}
}

func TestBuildRollbackGraph_Deterministic(t *testing.T) {
	s := newMultiServiceState()
	actions := map[string]ActionType{
		"svc-a": ActionCreate,
		"svc-b": ActionUpdate,
		"svc-c": ActionCreate,
	}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)

	rg1 := BuildRollbackGraph(cs, g)
	rg2 := BuildRollbackGraph(cs, g)

	if len(rg1.Ordering) != len(rg2.Ordering) {
		t.Fatal("rollback ordering must be deterministic")
	}
	for i := range rg1.Ordering {
		if rg1.Ordering[i] != rg2.Ordering[i] {
			t.Fatalf("rollback ordering differs at index %d", i)
		}
	}
}

func TestBuildRollbackGraph_NoteNonEmpty(t *testing.T) {
	s := newTestDesiredState()
	actions := map[string]ActionType{"svc-a": ActionCreate}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)
	rg := BuildRollbackGraph(cs, g)

	for _, step := range rg.Steps {
		if step.Note == "" {
			t.Fatalf("rollback step for %s must have a non-empty Note", step.NodeID)
		}
	}
}

func TestReverseAction(t *testing.T) {
	cases := []struct {
		action   ActionType
		expected ActionType
	}{
		{ActionCreate, ActionDelete},
		{ActionDelete, ActionCreate},
		{ActionUpdate, ActionRollback},
		{ActionRollback, ActionUpdate},
		{ActionNoOp, ActionNoOp},
	}
	for _, tc := range cases {
		got := reverseAction(tc.action)
		if got != tc.expected {
			t.Errorf("reverseAction(%s) = %s, want %s", tc.action, got, tc.expected)
		}
	}
}
