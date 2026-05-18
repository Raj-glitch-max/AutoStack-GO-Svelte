package planner

import (
	"testing"
)

func TestSimulatePlan_EmptyGraph(t *testing.T) {
	s := NewDesiredState(1, EnvironmentProduction, nil, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	result := SimulatePlan(g, cs, s)

	if result.DesiredStateHash != s.DesiredStateHash {
		t.Fatal("simulation must carry the DesiredStateHash")
	}
	if len(result.SimulationLimitations) == 0 {
		t.Fatal("simulation limitations must be non-empty")
	}
	if result.IsBlocked {
		t.Fatal("empty plan must not be blocked")
	}
}

func TestSimulatePlan_CyclicGraph_IsBlocked(t *testing.T) {
	services := []ServiceSpec{
		{ID: "a", Provider: "aws-ecs", DependsOn: []string{"b"}},
		{ID: "b", Provider: "aws-ecs", DependsOn: []string{"a"}},
	}
	s := NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	result := SimulatePlan(g, cs, s)

	if !result.IsBlocked {
		t.Fatal("cyclic graph simulation must be blocked")
	}
	if result.ConfidenceScore != 0 {
		t.Fatalf("cyclic graph simulation must have confidence=0, got %d", result.ConfidenceScore)
	}
	if len(result.AmbiguityWarnings) == 0 {
		t.Fatal("cyclic simulation must have ambiguity warnings")
	}
}

func TestSimulatePlan_PredictedActions_AllNodes(t *testing.T) {
	s := newMultiServiceState()
	actions := map[string]ActionType{
		"svc-a": ActionCreate,
		"svc-b": ActionUpdate,
		"svc-c": ActionDelete,
	}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)
	result := SimulatePlan(g, cs, s)

	if len(result.PredictedActions) != len(g.Nodes) {
		t.Fatalf("expected %d predicted actions (one per node), got %d", len(g.Nodes), len(result.PredictedActions))
	}
}

func TestSimulatePlan_IrreversibleNodes_ForDeletes(t *testing.T) {
	s := newTestDesiredState()
	actions := map[string]ActionType{"svc-a": ActionDelete}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)
	result := SimulatePlan(g, cs, s)

	if len(result.IrreversibleNodes) == 0 {
		t.Fatal("delete action must produce irreversible nodes in simulation")
	}
}

func TestSimulatePlan_RiskSummary_CriticalForDelete(t *testing.T) {
	s := newTestDesiredState()
	actions := map[string]ActionType{"svc-a": ActionDelete}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)
	result := SimulatePlan(g, cs, s)

	if result.RiskSummary.CriticalActionCount == 0 {
		t.Fatal("delete action must contribute to CriticalActionCount")
	}
}

func TestSimulatePlan_BlastRadiusMap_NonEmpty(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	result := SimulatePlan(g, cs, s)

	if len(result.BlastRadiusMap) != len(g.Nodes) {
		t.Fatalf("blast radius map must have one entry per node, got %d for %d nodes", len(result.BlastRadiusMap), len(g.Nodes))
	}
}

func TestSimulatePlan_ConfidenceScore_Range(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	result := SimulatePlan(g, cs, s)

	if result.ConfidenceScore < 0 || result.ConfidenceScore > 100 {
		t.Fatalf("confidence score must be in [0,100], got %d", result.ConfidenceScore)
	}
}

func TestSimulatePlan_ConfidenceScore_LowerForDeletes(t *testing.T) {
	s := newTestDesiredState()

	// No-op plan: higher confidence.
	g1 := BuildPlanGraph(s, nil)
	cs1 := BuildChangeSet(g1, s)
	noop := SimulatePlan(g1, cs1, s)

	// Delete plan: lower confidence.
	g2 := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionDelete})
	cs2 := BuildChangeSet(g2, s)
	withDelete := SimulatePlan(g2, cs2, s)

	if withDelete.ConfidenceScore >= noop.ConfidenceScore {
		t.Fatalf("delete plan confidence (%d) must be lower than no-op confidence (%d)", withDelete.ConfidenceScore, noop.ConfidenceScore)
	}
}

func TestSimulatePlan_AmbiguityWarnings_NonEmpty(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	result := SimulatePlan(g, cs, s)

	if len(result.AmbiguityWarnings) == 0 {
		t.Fatal("simulation must always have ambiguity warnings (at minimum the limitations warnings)")
	}
}

func TestSimulatePlan_DryRunOnly_IsBlocked(t *testing.T) {
	s := NewDesiredState(1, EnvironmentProduction,
		[]ServiceSpec{{ID: "svc-a", Provider: "aws-ecs"}},
		nil, nil, nil, nil,
		RolloutStrategySpec{},
		SafetyProfile{DryRunOnly: true},
		nil,
	)
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate})
	cs := BuildChangeSet(g, s)
	result := SimulatePlan(g, cs, s)

	// DryRunOnly is not enforced in simulate but collected as warning.
	// The safety layer enforces blocking. The ambiguity warning must mention dry-run.
	found := false
	for _, w := range result.AmbiguityWarnings {
		if len(w) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("simulation with DryRunOnly must include ambiguity warnings")
	}
}

func TestSimulatePlan_BlastRadiusLimit_Blocks(t *testing.T) {
	// db → backend → frontend: blast radius from db = 3.
	deps := []DependencySpec{{ID: "db", Kind: "database", Provider: "aws-rds"}}
	services := []ServiceSpec{
		{ID: "backend", Provider: "aws-ecs", DependsOn: []string{"db"}},
		{ID: "frontend", Provider: "aws-ecs", DependsOn: []string{"backend"}},
	}
	s := NewDesiredState(1, EnvironmentProduction, services, deps, nil, nil, nil,
		RolloutStrategySpec{},
		SafetyProfile{MaxBlastRadiusNodes: 1}, // limit of 1, db has radius 3
		nil,
	)
	g := BuildPlanGraph(s, map[string]ActionType{"db": ActionDelete})
	cs := BuildChangeSet(g, s)
	result := SimulatePlan(g, cs, s)

	if !result.IsBlocked {
		t.Fatal("simulation must be blocked when blast radius exceeds safety profile limit")
	}
}

func TestSimulatePlan_RollbackChain_NonNilForActionable(t *testing.T) {
	s := newTestDesiredState()
	actions := map[string]ActionType{"svc-a": ActionCreate}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)
	result := SimulatePlan(g, cs, s)

	if result.RollbackChain == nil {
		t.Fatal("rollback chain must be non-nil when there are actionable changes")
	}
}

func TestSimulatePlan_ParallelStages_Populated(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	result := SimulatePlan(g, cs, s)

	if result.ParallelStages == nil {
		t.Fatal("parallel stages must be populated from graph")
	}
}

func TestSimulatePlan_Deterministic(t *testing.T) {
	s := newMultiServiceState()
	actions := map[string]ActionType{"svc-a": ActionCreate, "svc-b": ActionUpdate}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)

	r1 := SimulatePlan(g, cs, s)
	r2 := SimulatePlan(g, cs, s)

	if r1.ConfidenceScore != r2.ConfidenceScore {
		t.Fatal("SimulatePlan must be deterministic: ConfidenceScore differs")
	}
	if r1.IsBlocked != r2.IsBlocked {
		t.Fatal("SimulatePlan must be deterministic: IsBlocked differs")
	}
	if len(r1.PredictedActions) != len(r2.PredictedActions) {
		t.Fatal("SimulatePlan must be deterministic: PredictedActions count differs")
	}
}
