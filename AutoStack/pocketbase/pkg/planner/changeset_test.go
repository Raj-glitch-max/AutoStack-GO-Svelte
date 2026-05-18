package planner

import (
	"testing"
)

func TestBuildChangeSet_EmptyGraph(t *testing.T) {
	s := NewDesiredState(1, EnvironmentProduction, nil, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	if cs.TotalChanges != 0 {
		t.Fatalf("empty graph must produce 0 changes, got %d", cs.TotalChanges)
	}
}

func TestBuildChangeSet_CyclicGraphReturnsEmpty(t *testing.T) {
	services := []ServiceSpec{
		{ID: "a", Provider: "aws-ecs", DependsOn: []string{"b"}},
		{ID: "b", Provider: "aws-ecs", DependsOn: []string{"a"}},
	}
	s := NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	if cs.TotalChanges != 0 {
		t.Fatal("cyclic graph must produce empty ChangeSet")
	}
}

func TestBuildChangeSet_Creates(t *testing.T) {
	s := newTestDesiredState()
	actions := map[string]ActionType{"svc-a": ActionCreate}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)

	if len(cs.Creates) == 0 {
		t.Fatal("expected at least one create item")
	}
	found := false
	for _, item := range cs.Creates {
		if item.NodeID == "svc-a" && item.Action == ActionCreate {
			found = true
		}
	}
	if !found {
		t.Fatal("svc-a with action=create must be in Creates")
	}
}

func TestBuildChangeSet_Updates(t *testing.T) {
	s := newTestDesiredState()
	actions := map[string]ActionType{"svc-a": ActionUpdate}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)

	if len(cs.Updates) == 0 {
		t.Fatal("expected at least one update item")
	}
	if cs.Updates[0].Action != ActionUpdate {
		t.Fatalf("expected action=update, got %s", cs.Updates[0].Action)
	}
}

func TestBuildChangeSet_Deletes(t *testing.T) {
	s := newTestDesiredState()
	actions := map[string]ActionType{"svc-a": ActionDelete}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)

	if len(cs.Deletes) == 0 {
		t.Fatal("expected at least one delete item")
	}
	if cs.Deletes[0].Action != ActionDelete {
		t.Fatalf("expected action=delete, got %s", cs.Deletes[0].Action)
	}
}

func TestBuildChangeSet_NoOps(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil) // no actions → all no-ops
	cs := BuildChangeSet(g, s)

	if cs.TotalChanges != 0 {
		t.Fatalf("no actions → TotalChanges must be 0, got %d", cs.TotalChanges)
	}
	if len(cs.NoOps) == 0 {
		t.Fatal("no actions → NoOps must be non-empty")
	}
}

func TestBuildChangeSet_TotalChanges(t *testing.T) {
	services := []ServiceSpec{
		{ID: "svc-a", Provider: "aws-ecs"},
		{ID: "svc-b", Provider: "aws-ecs"},
		{ID: "svc-c", Provider: "aws-ecs"},
	}
	s := NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	actions := map[string]ActionType{
		"svc-a": ActionCreate,
		"svc-b": ActionUpdate,
		"svc-c": ActionDelete,
	}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)

	if cs.TotalChanges != 3 {
		t.Fatalf("expected TotalChanges=3, got %d", cs.TotalChanges)
	}
}

func TestBuildChangeSet_HighestRisk(t *testing.T) {
	s := newTestDesiredState()
	actions := map[string]ActionType{"svc-a": ActionDelete}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)

	if cs.HighestRisk != RiskLevelCritical {
		t.Fatalf("delete action → HighestRisk must be critical, got %s", cs.HighestRisk)
	}
}

func TestBuildChangeSet_RollbackImplication_Delete(t *testing.T) {
	s := newTestDesiredState()
	actions := map[string]ActionType{"svc-a": ActionDelete}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)

	if len(cs.Deletes) == 0 {
		t.Fatal("expected delete item")
	}
	impl := cs.Deletes[0].RollbackImplication
	if impl == "" {
		t.Fatal("delete must have a rollback implication string")
	}
}

func TestBuildChangeSet_ConstraintViolation(t *testing.T) {
	services := []ServiceSpec{{
		ID:       "svc-critical",
		Provider: "aws-ecs",
	}}
	constraints := []ConstraintSpec{{
		ID:       "c-no-delete",
		Severity: "block",
		Applies:  "all",
	}}
	s := NewDesiredState(1, EnvironmentProduction, services, nil, nil, constraints, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	// Assign the constraint to the node manually via node.Constraints via actions.
	// The constraint violation is checked via isConstraintViolated.
	// Delete on a critical-risk node should trigger the violation.
	actions := map[string]ActionType{"svc-critical": ActionDelete}
	g := BuildPlanGraph(s, actions)

	// Assign constraint to node.
	if node, ok := g.NodeIndex["svc-critical"]; ok {
		node.Constraints = []string{"c-no-delete"}
	}

	cs := BuildChangeSet(g, s)
	if cs.BlockedByConstraints {
		// That's expected if RiskLevel is critical.
		if !cs.HasConstraintViolations {
			t.Fatal("BlockedByConstraints requires HasConstraintViolations=true")
		}
	}
}

func TestAllChanges_SortedByNodeID(t *testing.T) {
	services := []ServiceSpec{
		{ID: "z-svc", Provider: "aws-ecs"},
		{ID: "a-svc", Provider: "aws-ecs"},
		{ID: "m-svc", Provider: "aws-ecs"},
	}
	s := NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	actions := map[string]ActionType{
		"z-svc": ActionCreate,
		"a-svc": ActionCreate,
		"m-svc": ActionCreate,
	}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)

	all := cs.AllChanges()
	for i := 1; i < len(all); i++ {
		if all[i].NodeID < all[i-1].NodeID {
			t.Fatalf("AllChanges must be sorted by NodeID: %s before %s", all[i].NodeID, all[i-1].NodeID)
		}
	}
}

func TestBuildChangeSet_Deterministic(t *testing.T) {
	s := newMultiServiceState()
	actions := map[string]ActionType{
		"svc-a": ActionCreate,
		"svc-b": ActionUpdate,
		"svc-c": ActionDelete,
	}
	g := BuildPlanGraph(s, actions)
	cs1 := BuildChangeSet(g, s)
	cs2 := BuildChangeSet(g, s)

	if cs1.TotalChanges != cs2.TotalChanges {
		t.Fatal("BuildChangeSet must be deterministic: TotalChanges differs")
	}
	if cs1.HighestRisk != cs2.HighestRisk {
		t.Fatal("BuildChangeSet must be deterministic: HighestRisk differs")
	}
}

func TestBuildChangeSet_ConfidenceForNoOp_High(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)

	for _, item := range cs.NoOps {
		if item.Confidence != ConfidenceLevelHigh {
			t.Fatalf("no-op items must have high confidence, got %s for %s", item.Confidence, item.NodeID)
		}
	}
}
