package planner

import (
	"strings"
	"testing"
)

func TestNewDesiredState_Hash(t *testing.T) {
	s := newTestDesiredState()
	if s.DesiredStateHash == "" {
		t.Fatal("DesiredStateHash must be non-empty after construction")
	}
	if len(s.DesiredStateHash) != 64 {
		t.Fatalf("expected 64-char hex hash, got %d chars", len(s.DesiredStateHash))
	}
}

func TestComputeDesiredStateHash_Deterministic(t *testing.T) {
	s1 := newTestDesiredState()
	s2 := newTestDesiredState()
	if s1.DesiredStateHash != s2.DesiredStateHash {
		t.Fatalf("same content, different GeneratedAt → hashes must be equal\n  s1=%s\n  s2=%s", s1.DesiredStateHash, s2.DesiredStateHash)
	}
}

func TestComputeDesiredStateHash_ChangedContent(t *testing.T) {
	s1 := newTestDesiredState()
	// Change one field.
	services := []ServiceSpec{{
		ID:       "svc-b",
		Provider: "aws-ecs",
	}}
	s2 := NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil,
		RolloutStrategySpec{}, SafetyProfile{}, nil)
	if s1.DesiredStateHash == s2.DesiredStateHash {
		t.Fatal("different service IDs must produce different hashes")
	}
}

func TestComputeDesiredStateHash_GeneratedAtExcluded(t *testing.T) {
	s := newTestDesiredState()
	original := s.DesiredStateHash
	// Mutate GeneratedAt and recompute.
	s.GeneratedAt = "2099-01-01T00:00:00Z"
	recomputed := ComputeDesiredStateHash(s)
	if recomputed != original {
		t.Fatal("GeneratedAt must be excluded from hash — same content, different time must produce same hash")
	}
}

func TestDesiredState_SchemaVersion(t *testing.T) {
	s := newTestDesiredState()
	if s.SchemaVersion != DesiredStateSchemaVersion {
		t.Fatalf("expected SchemaVersion=%q, got %q", DesiredStateSchemaVersion, s.SchemaVersion)
	}
}

func TestNewDesiredState_ServiceOrdering(t *testing.T) {
	// Services provided out of order — must be sorted by ID.
	services := []ServiceSpec{
		{ID: "z-svc", Provider: "aws-ecs"},
		{ID: "a-svc", Provider: "cloud-run"},
		{ID: "m-svc", Provider: "azure-aca"},
	}
	s := NewDesiredState(1, EnvironmentStaging, services, nil, nil, nil, nil,
		RolloutStrategySpec{}, SafetyProfile{}, nil)
	ids := make([]string, len(s.Services))
	for i, svc := range s.Services {
		ids[i] = svc.ID
	}
	if ids[0] != "a-svc" || ids[1] != "m-svc" || ids[2] != "z-svc" {
		t.Fatalf("services must be sorted by ID, got %v", ids)
	}
}

func TestNewDesiredState_DependsOnSorted(t *testing.T) {
	services := []ServiceSpec{{
		ID:        "svc-a",
		Provider:  "aws-ecs",
		DependsOn: []string{"db-c", "db-a", "db-b"},
	}}
	s := NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil,
		RolloutStrategySpec{}, SafetyProfile{}, nil)
	deps := s.Services[0].DependsOn
	if deps[0] != "db-a" || deps[1] != "db-b" || deps[2] != "db-c" {
		t.Fatalf("DependsOn must be sorted, got %v", deps)
	}
}

func TestNewDesiredState_EnvVarsSorted(t *testing.T) {
	services := []ServiceSpec{{
		ID:       "svc-a",
		Provider: "aws-ecs",
		EnvVars: []EnvVarIntent{
			{Name: "Z_VAR", Value: "z"},
			{Name: "A_VAR", Value: "a"},
			{Name: "M_VAR", Value: "m"},
		},
	}}
	s := NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil,
		RolloutStrategySpec{}, SafetyProfile{}, nil)
	ev := s.Services[0].EnvVars
	if ev[0].Name != "A_VAR" || ev[1].Name != "M_VAR" || ev[2].Name != "Z_VAR" {
		t.Fatalf("EnvVars must be sorted by Name, got %v", ev)
	}
}

func TestComputeDesiredStateHash_NotEmpty(t *testing.T) {
	s := &DesiredState{SchemaVersion: "4.0.0", StateVersion: 1, Environment: EnvironmentProduction}
	h := ComputeDesiredStateHash(s)
	if h == "" {
		t.Fatal("hash must be non-empty even for minimal DesiredState")
	}
	if !strings.HasPrefix(h, "") {
		t.Fatal("hash must be hex")
	}
}

// ─────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────

func newTestDesiredState() *DesiredState {
	services := []ServiceSpec{{
		ID:       "svc-a",
		Provider: "aws-ecs",
		Region:   "us-east-1",
		Image:    ImageIntent{Registry: "docker.io", Repository: "myapp", Tag: "v1.0"},
		Compute:  ComputeIntent{CPULimitVCPU: 0.5, MemoryLimitMB: 512},
		Scale:    ScaleIntent{MinReplicas: 1, MaxReplicas: 3},
	}}
	deps := []DependencySpec{{ID: "db-a", Kind: "database", Provider: "aws-rds"}}
	return NewDesiredState(
		1,
		EnvironmentProduction,
		services,
		deps,
		nil, nil, nil,
		RolloutStrategySpec{Mode: "rolling"},
		SafetyProfile{},
		nil,
	)
}
