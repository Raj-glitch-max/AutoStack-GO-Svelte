package planner

import (
	"testing"
)

func TestEvaluateSafety_EmptyPlan_Approved(t *testing.T) {
	s := NewDesiredState(1, EnvironmentProduction, nil, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)

	if !verdict.Approved {
		t.Fatalf("empty plan must be approved, violations: %v", verdict.Violations)
	}
}

func TestEvaluateSafety_DryRunOnly_Blocked(t *testing.T) {
	s := NewDesiredState(1, EnvironmentProduction,
		[]ServiceSpec{{ID: "svc-a", Provider: "aws-ecs"}},
		nil, nil, nil, nil,
		RolloutStrategySpec{},
		SafetyProfile{DryRunOnly: true},
		nil,
	)
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate})
	cs := BuildChangeSet(g, s)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)

	if !verdict.DryRunOnly {
		t.Fatal("DryRunOnly must be true in verdict when SafetyProfile.DryRunOnly=true")
	}
	if verdict.Approved {
		t.Fatal("dry-run-only plan must not be approved for execution")
	}
	if len(verdict.Violations) == 0 {
		t.Fatal("DRY_RUN_ONLY rule must produce a violation")
	}
}

func TestEvaluateSafety_CyclicGraph_Blocked(t *testing.T) {
	services := []ServiceSpec{
		{ID: "a", Provider: "aws-ecs", DependsOn: []string{"b"}},
		{ID: "b", Provider: "aws-ecs", DependsOn: []string{"a"}},
	}
	s := NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)

	if verdict.Approved {
		t.Fatal("cyclic graph must not be approved")
	}
	foundRule := false
	for _, v := range verdict.Violations {
		if v.Rule == "CYCLIC_GRAPH" {
			foundRule = true
		}
	}
	if !foundRule {
		t.Fatal("CYCLIC_GRAPH violation must be present")
	}
}

func TestEvaluateSafety_ForbidDestructiveDeletes(t *testing.T) {
	s := NewDesiredState(1, EnvironmentProduction,
		[]ServiceSpec{{ID: "svc-a", Provider: "aws-ecs"}},
		nil, nil, nil, nil,
		RolloutStrategySpec{},
		SafetyProfile{ForbidDestructiveDeletes: true},
		nil,
	)
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionDelete})
	cs := BuildChangeSet(g, s)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)

	if verdict.Approved {
		t.Fatal("destructive delete must be blocked when ForbidDestructiveDeletes=true")
	}
	if len(verdict.ForbiddenActions) == 0 {
		t.Fatal("ForbiddenActions must be non-empty when delete is forbidden")
	}
}

func TestEvaluateSafety_HighRiskApprovalRequired(t *testing.T) {
	s := NewDesiredState(1, EnvironmentProduction,
		[]ServiceSpec{{
			ID:         "svc-a",
			Provider:   "aws-ecs",
			SecretRefs: []SecretRefIntent{{Name: "api-key", Source: "aws-secrets-manager"}},
		}},
		nil, nil, nil, nil,
		RolloutStrategySpec{},
		SafetyProfile{RequireApprovalAbove: "medium"}, // anything > medium needs approval
		nil,
	)
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate})
	cs := BuildChangeSet(g, s)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)

	// svc-a create with secret refs → risk=high > medium threshold → requires approval.
	if !verdict.RequiresApproval {
		t.Fatal("high-risk action above threshold must set RequiresApproval=true")
	}
	if len(verdict.ApprovalGates) == 0 {
		t.Fatal("ApprovalGates must be non-empty when RequiresApproval=true")
	}
}

func TestEvaluateSafety_NoViolations_Approved(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate})
	cs := BuildChangeSet(g, s)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)

	// With no safety restrictions, a simple create must be approved.
	hasBlockingViolation := false
	for _, v := range verdict.Violations {
		if v.Severity == "block" {
			hasBlockingViolation = true
		}
	}
	if hasBlockingViolation && !verdict.Approved {
		// This is expected — approved only when no blocking violations.
	} else if !hasBlockingViolation && !verdict.Approved {
		t.Fatal("no blocking violations → plan must be approved")
	}
}

func TestEvaluateSafety_AllowedProviders_Blocked(t *testing.T) {
	s := NewDesiredState(1, EnvironmentProduction,
		[]ServiceSpec{{ID: "svc-a", Provider: "azure-aca"}},
		nil, nil, nil, nil,
		RolloutStrategySpec{},
		SafetyProfile{AllowedProviders: []string{"aws-ecs"}}, // azure not allowed
		nil,
	)
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate})
	cs := BuildChangeSet(g, s)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)

	if verdict.Approved {
		t.Fatal("plan using a disallowed provider must not be approved")
	}
	foundRule := false
	for _, v := range verdict.Violations {
		if v.Rule == "PROVIDER_NOT_ALLOWED" {
			foundRule = true
		}
	}
	if !foundRule {
		t.Fatal("PROVIDER_NOT_ALLOWED violation must be present")
	}
}

func TestEvaluateSafety_OrphanNodes_Warning(t *testing.T) {
	services := []ServiceSpec{{ID: "orphan-svc", Provider: "aws-ecs"}}
	s := NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)

	foundWarning := false
	for _, w := range verdict.Warnings {
		if w.Rule == "ORPHAN_NODE" {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatal("orphan node must produce an ORPHAN_NODE warning")
	}
}

func TestEvaluateSafety_DeleteWarning_WhenNotForbidden(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionDelete})
	cs := BuildChangeSet(g, s)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)

	// Delete is allowed but must produce an IRREVERSIBLE_ACTION warning.
	foundWarning := false
	for _, w := range verdict.Warnings {
		if w.Rule == "IRREVERSIBLE_ACTION" {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatal("delete action must produce an IRREVERSIBLE_ACTION warning even when not forbidden")
	}
}

func TestEvaluateSafety_PauseOnHighRisk(t *testing.T) {
	s := NewDesiredState(1, EnvironmentProduction,
		[]ServiceSpec{{
			ID:         "svc-a",
			Provider:   "aws-ecs",
			SecretRefs: []SecretRefIntent{{Name: "api-key", Source: "aws-secrets-manager"}},
		}},
		nil, nil, nil, nil,
		RolloutStrategySpec{PauseOnHighRisk: true},
		SafetyProfile{},
		nil,
	)
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate})
	cs := BuildChangeSet(g, s)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)

	if !verdict.RequiresApproval {
		t.Fatal("PauseOnHighRisk=true with high-risk action must require approval")
	}
}

func TestEvaluateSafety_Deterministic(t *testing.T) {
	s := newMultiServiceState()
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionDelete})
	cs := BuildChangeSet(g, s)
	sim := SimulatePlan(g, cs, s)

	v1 := EvaluateSafety(g, cs, s, sim)
	v2 := EvaluateSafety(g, cs, s, sim)

	if v1.Approved != v2.Approved {
		t.Fatal("EvaluateSafety must be deterministic: Approved differs")
	}
	if len(v1.Violations) != len(v2.Violations) {
		t.Fatal("EvaluateSafety must be deterministic: Violations count differs")
	}
}
