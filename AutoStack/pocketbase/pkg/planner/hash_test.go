package planner

import (
	"testing"
)

func TestComputePlanHash_Deterministic(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate})
	cs := BuildChangeSet(g, s)

	h1 := ComputePlanHash(g, cs, s, nil, nil)
	h2 := ComputePlanHash(g, cs, s, nil, nil)
	if h1 != h2 {
		t.Fatalf("plan hash must be deterministic: %q vs %q", h1, h2)
	}
}

func TestComputePlanHash_NonEmpty(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)

	h := ComputePlanHash(g, cs, s, nil, nil)
	if h == "" {
		t.Fatal("plan hash must be non-empty")
	}
	if len(h) != 64 {
		t.Fatalf("expected 64-char hex SHA-256, got %d chars", len(h))
	}
}

func TestComputePlanHash_DifferentActions_DifferentHash(t *testing.T) {
	s := newTestDesiredState()
	g1 := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate})
	cs1 := BuildChangeSet(g1, s)
	h1 := ComputePlanHash(g1, cs1, s, nil, nil)

	g2 := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionDelete})
	cs2 := BuildChangeSet(g2, s)
	h2 := ComputePlanHash(g2, cs2, s, nil, nil)

	if h1 == h2 {
		t.Fatal("different actions must produce different plan hashes")
	}
}

func TestComputePlanHash_DifferentDesiredState_DifferentHash(t *testing.T) {
	s1 := newTestDesiredState()
	s2 := NewDesiredState(1, EnvironmentStaging, // different environment
		[]ServiceSpec{{ID: "svc-a", Provider: "aws-ecs"}},
		nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil,
	)

	g1 := BuildPlanGraph(s1, nil)
	cs1 := BuildChangeSet(g1, s1)
	h1 := ComputePlanHash(g1, cs1, s1, nil, nil)

	g2 := BuildPlanGraph(s2, nil)
	cs2 := BuildChangeSet(g2, s2)
	h2 := ComputePlanHash(g2, cs2, s2, nil, nil)

	if h1 == h2 {
		t.Fatal("different desired states must produce different plan hashes")
	}
}

func TestComputePlanHash_WithObsFingerprints(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)

	h1 := ComputePlanHash(g, cs, s, nil, nil)
	h2 := ComputePlanHash(g, cs, s, []string{"obs-fingerprint-1"}, nil)

	if h1 == h2 {
		t.Fatal("different obs fingerprints must produce different plan hashes")
	}
}

func TestComputePlanHash_ObsFingerprintsOrdering(t *testing.T) {
	// Same fingerprints in different order must produce same hash (sorted before hashing).
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)

	h1 := ComputePlanHash(g, cs, s, []string{"obs-a", "obs-b", "obs-c"}, nil)
	h2 := ComputePlanHash(g, cs, s, []string{"obs-c", "obs-a", "obs-b"}, nil)

	if h1 != h2 {
		t.Fatal("obs fingerprint ordering must not affect hash — fingerprints are sorted before hashing")
	}
}

func TestComputePlanHash_DriftIDOrdering(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)

	h1 := ComputePlanHash(g, cs, s, nil, []string{"drift-1", "drift-2", "drift-3"})
	h2 := ComputePlanHash(g, cs, s, nil, []string{"drift-3", "drift-1", "drift-2"})

	if h1 != h2 {
		t.Fatal("drift record ID ordering must not affect hash — IDs are sorted before hashing")
	}
}

func TestComputePlanHash_WithPolicies(t *testing.T) {
	s1 := NewDesiredState(1, EnvironmentProduction,
		[]ServiceSpec{{ID: "svc-a", Provider: "aws-ecs"}},
		nil,
		[]PolicySpec{{ID: "p-1", Name: "NoDelete", Category: "safety"}},
		nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil,
	)
	s2 := newTestDesiredState() // no policies

	g1 := BuildPlanGraph(s1, nil)
	cs1 := BuildChangeSet(g1, s1)
	h1 := ComputePlanHash(g1, cs1, s1, nil, nil)

	g2 := BuildPlanGraph(s2, nil)
	cs2 := BuildChangeSet(g2, s2)
	h2 := ComputePlanHash(g2, cs2, s2, nil, nil)

	if h1 == h2 {
		t.Fatal("plans with different policies must produce different hashes")
	}
}

func TestBuildHashInput_NodeIDsSorted(t *testing.T) {
	services := []ServiceSpec{
		{ID: "z-svc", Provider: "aws-ecs"},
		{ID: "a-svc", Provider: "aws-ecs"},
	}
	s := NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)

	input := buildHashInput(g, cs, s, nil, nil)
	for i := 1; i < len(input.NodeIDs); i++ {
		if input.NodeIDs[i] < input.NodeIDs[i-1] {
			t.Fatalf("node IDs in hash input must be sorted: %v", input.NodeIDs)
		}
	}
}

func TestComputePlanHash_StableAfterReorder(t *testing.T) {
	// Build the same logical state twice — order of service slice in constructor differs.
	services1 := []ServiceSpec{
		{ID: "alpha", Provider: "aws-ecs"},
		{ID: "beta", Provider: "aws-ecs"},
	}
	services2 := []ServiceSpec{
		{ID: "beta", Provider: "aws-ecs"},
		{ID: "alpha", Provider: "aws-ecs"},
	}
	s1 := NewDesiredState(1, EnvironmentProduction, services1, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	s2 := NewDesiredState(1, EnvironmentProduction, services2, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)

	g1 := BuildPlanGraph(s1, nil)
	cs1 := BuildChangeSet(g1, s1)
	h1 := ComputePlanHash(g1, cs1, s1, nil, nil)

	g2 := BuildPlanGraph(s2, nil)
	cs2 := BuildChangeSet(g2, s2)
	h2 := ComputePlanHash(g2, cs2, s2, nil, nil)

	if h1 != h2 {
		t.Fatal("service slice reordering must produce identical plan hash (normalization guarantees stability)")
	}
}
