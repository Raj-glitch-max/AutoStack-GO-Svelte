package planner

import (
	"strings"
	"testing"
)

func TestExplainPlan_EmptyGraph(t *testing.T) {
	s := NewDesiredState(1, EnvironmentProduction, nil, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	set := ExplainPlan(g, cs, s)

	if set.DesiredStateHash != s.DesiredStateHash {
		t.Fatal("explanation set must carry the DesiredStateHash")
	}
	if len(set.GlobalLimitations) == 0 {
		t.Fatal("global limitations must be non-empty")
	}
}

func TestExplainPlan_HasExplanationForEveryNode(t *testing.T) {
	s := newMultiServiceState()
	actions := map[string]ActionType{
		"svc-a": ActionCreate,
		"svc-b": ActionUpdate,
		"svc-c": ActionDelete,
	}
	g := BuildPlanGraph(s, actions)
	cs := BuildChangeSet(g, s)
	set := ExplainPlan(g, cs, s)

	if len(set.Explanations) != len(g.Nodes) {
		t.Fatalf("expected %d explanations (one per node), got %d", len(g.Nodes), len(set.Explanations))
	}
}

func TestExplainPlan_WhyNeeded_Create(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate})
	cs := BuildChangeSet(g, s)
	set := ExplainPlan(g, cs, s)

	var exp *PlanExplanation
	for i := range set.Explanations {
		if set.Explanations[i].NodeID == "svc-a" {
			exp = &set.Explanations[i]
		}
	}
	if exp == nil {
		t.Fatal("explanation for svc-a not found")
	}
	if exp.WhyNeeded == "" {
		t.Fatal("WhyNeeded must be non-empty")
	}
	if exp.Action != string(ActionCreate) {
		t.Fatalf("expected action=create, got %s", exp.Action)
	}
}

func TestExplainPlan_AlternativesRejected(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate})
	cs := BuildChangeSet(g, s)
	set := ExplainPlan(g, cs, s)

	for _, exp := range set.Explanations {
		if exp.NodeID == "svc-a" {
			if len(exp.AlternativesRejected) == 0 {
				t.Fatal("alternatives rejected must be non-empty for an action node")
			}
			// Create should reject update, delete, no-op at minimum.
			for _, alt := range exp.AlternativesRejected {
				if alt.Reason == "" {
					t.Fatalf("rejected alternative %q has empty reason", alt.Action)
				}
			}
		}
	}
}

func TestExplainPlan_GlobalLimitations_Content(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	set := ExplainPlan(g, cs, s)

	// At least one limitation must mention providers.
	found := false
	for _, lim := range set.GlobalLimitations {
		if strings.Contains(lim, "provider") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("global limitations must mention provider uncertainty")
	}
}

func TestExplainPlan_NodeLimitations_DeleteHasIrreversible(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionDelete})
	cs := BuildChangeSet(g, s)
	set := ExplainPlan(g, cs, s)

	for _, exp := range set.Explanations {
		if exp.NodeID == "svc-a" {
			found := false
			for _, lim := range exp.Limitations {
				if strings.Contains(lim, "irreversible") {
					found = true
				}
			}
			if !found {
				t.Fatal("delete node limitations must mention irreversibility")
			}
		}
	}
}

func TestExplainPlan_Deterministic(t *testing.T) {
	s := newMultiServiceState()
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate})
	cs := BuildChangeSet(g, s)

	set1 := ExplainPlan(g, cs, s)
	set2 := ExplainPlan(g, cs, s)

	if len(set1.Explanations) != len(set2.Explanations) {
		t.Fatal("ExplainPlan must be deterministic: explanation count differs")
	}
	for i := range set1.Explanations {
		if set1.Explanations[i].NodeID != set2.Explanations[i].NodeID {
			t.Fatalf("explanation order non-deterministic at index %d", i)
		}
	}
}

func TestExplainPlan_PolicyAllowed_NoPolicy(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate})
	cs := BuildChangeSet(g, s)
	set := ExplainPlan(g, cs, s)

	for _, exp := range set.Explanations {
		if exp.NodeID == "svc-a" {
			if exp.PolicyAllowed == "" {
				t.Fatal("PolicyAllowed must be non-empty even without policies (should say 'none')")
			}
		}
	}
}

func TestExplainPlan_EvidenceSummary_NoRefs(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	set := ExplainPlan(g, cs, s)

	for _, exp := range set.Explanations {
		if exp.EvidenceSummary == "" {
			t.Fatalf("EvidenceSummary must be non-empty for node %s (even when no refs)", exp.NodeID)
		}
	}
}

func TestExplainPlan_OrphanNodeLimitation(t *testing.T) {
	services := []ServiceSpec{
		{ID: "orphan-svc", Provider: "aws-ecs"}, // no deps, nothing depends on it
	}
	s := NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	set := ExplainPlan(g, cs, s)

	found := false
	for _, exp := range set.Explanations {
		if exp.NodeID == "orphan-svc" {
			for _, lim := range exp.Limitations {
				if strings.Contains(lim, "orphan") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("orphan node explanation must include orphan limitation")
	}
}
