package planner

import (
	"testing"
)

func TestClassifyActionRisk_NoOp(t *testing.T) {
	r := ClassifyActionRisk(ActionNoOp, ServiceSpec{})
	if r != RiskLevelLow {
		t.Fatalf("no-op must be low risk, got %s", r)
	}
}

func TestClassifyActionRisk_Delete_AlwaysCritical(t *testing.T) {
	r := ClassifyActionRisk(ActionDelete, ServiceSpec{})
	if r != RiskLevelCritical {
		t.Fatalf("delete must always be critical, got %s", r)
	}
}

func TestClassifyActionRisk_Delete_WithSecrets_StillCritical(t *testing.T) {
	svc := ServiceSpec{
		SecretRefs: []SecretRefIntent{{Name: "db-pass", Source: "aws-secrets-manager"}},
	}
	r := ClassifyActionRisk(ActionDelete, svc)
	if r != RiskLevelCritical {
		t.Fatalf("delete with secrets must still be critical, got %s", r)
	}
}

func TestClassifyActionRisk_Rollback_High(t *testing.T) {
	r := ClassifyActionRisk(ActionRollback, ServiceSpec{})
	if r != RiskLevelHigh {
		t.Fatalf("rollback must be high risk, got %s", r)
	}
}

func TestClassifyActionRisk_Simulate_Low(t *testing.T) {
	r := ClassifyActionRisk(ActionSimulate, ServiceSpec{})
	if r != RiskLevelLow {
		t.Fatalf("simulate must be low risk, got %s", r)
	}
}

func TestClassifyActionRisk_Create_WithSecrets_High(t *testing.T) {
	svc := ServiceSpec{
		SecretRefs: []SecretRefIntent{{Name: "api-key", Source: "aws-secrets-manager"}},
	}
	r := ClassifyActionRisk(ActionCreate, svc)
	if r != RiskLevelHigh {
		t.Fatalf("create with secret refs must be high risk, got %s", r)
	}
}

func TestClassifyActionRisk_Create_NoSecrets_Medium(t *testing.T) {
	svc := ServiceSpec{
		Compute: ComputeIntent{CPULimitVCPU: 1, MemoryLimitMB: 512},
	}
	r := ClassifyActionRisk(ActionCreate, svc)
	if r != RiskLevelMedium {
		t.Fatalf("create without secrets must be medium, got %s", r)
	}
}

func TestClassifyActionRisk_Update_WithSecretEnvVar_High(t *testing.T) {
	svc := ServiceSpec{
		EnvVars: []EnvVarIntent{{Name: "DB_PASSWORD", Value: "redacted"}},
	}
	r := ClassifyActionRisk(ActionUpdate, svc)
	if r != RiskLevelHigh {
		t.Fatalf("update with secret-named env var must be high, got %s", r)
	}
}

func TestClassifyActionRisk_Update_ComputeOnly_Medium(t *testing.T) {
	svc := ServiceSpec{
		Compute: ComputeIntent{MemoryLimitMB: 1024},
	}
	r := ClassifyActionRisk(ActionUpdate, svc)
	if r != RiskLevelMedium {
		t.Fatalf("update with compute-only change must be medium, got %s", r)
	}
}

func TestClassifyActionRisk_Update_AnnotationOnly_Low(t *testing.T) {
	svc := ServiceSpec{
		Annotations: map[string]string{"cost-center": "eng"},
	}
	r := ClassifyActionRisk(ActionUpdate, svc)
	if r != RiskLevelLow {
		t.Fatalf("update with annotation-only change must be low, got %s", r)
	}
}

func TestClassifyDeleteRisk_AlwaysCritical(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	r := ClassifyDeleteRisk("svc-a", g)
	if r != RiskLevelCritical {
		t.Fatalf("delete risk must be critical, got %s", r)
	}
}

func TestClassifyDeleteRisk_UnknownNode_High(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	r := ClassifyDeleteRisk("non-existent", g)
	if r != RiskLevelHigh {
		t.Fatalf("delete risk for unknown node must be high, got %s", r)
	}
}

func TestBlastRadius_SelfOnly(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	affected, radius := BlastRadius("svc-a", g)
	if radius < 1 {
		t.Fatal("blast radius must include at least the node itself")
	}
	found := false
	for _, id := range affected {
		if id == "svc-a" {
			found = true
		}
	}
	if !found {
		t.Fatal("blast radius must include the originating node")
	}
}

func TestBlastRadius_TransitiveDependents(t *testing.T) {
	// db ← backend ← frontend
	deps := []DependencySpec{{ID: "db", Kind: "database", Provider: "aws-rds"}}
	services := []ServiceSpec{
		{ID: "backend", Provider: "aws-ecs", DependsOn: []string{"db"}},
		{ID: "frontend", Provider: "aws-ecs", DependsOn: []string{"backend"}},
	}
	s := NewDesiredState(1, EnvironmentProduction, services, deps, nil, nil, nil,
		RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)

	affected, radius := BlastRadius("db", g)
	if radius != 3 {
		t.Fatalf("blast radius from db must be 3 (db+backend+frontend), got %d", radius)
	}
	_ = affected
}

func TestBlastRadius_LeafNode(t *testing.T) {
	services := []ServiceSpec{
		{ID: "backend", Provider: "aws-ecs", DependsOn: []string{"svc-leaf"}},
		{ID: "svc-leaf", Provider: "aws-ecs"},
	}
	s := NewDesiredState(1, EnvironmentProduction, services, nil, nil, nil, nil,
		RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)

	// Failing svc-leaf propagates to backend.
	_, radius := BlastRadius("svc-leaf", g)
	if radius < 2 {
		t.Fatalf("blast radius from leaf must include leaf + backend, got %d", radius)
	}
}

func TestBlastRadius_Deterministic(t *testing.T) {
	s := newMultiServiceState()
	g := BuildPlanGraph(s, nil)
	a1, r1 := BlastRadius("svc-b", g)
	a2, r2 := BlastRadius("svc-b", g)
	if r1 != r2 {
		t.Fatalf("blast radius must be deterministic: %d vs %d", r1, r2)
	}
	for i := range a1 {
		if a1[i] != a2[i] {
			t.Fatalf("blast radius affected nodes must be deterministic at index %d", i)
		}
	}
}

func TestHasSecretEnvVars(t *testing.T) {
	cases := []struct {
		name     string
		vars     []EnvVarIntent
		expected bool
	}{
		{"empty", nil, false},
		{"plain env", []EnvVarIntent{{Name: "APP_MODE", Value: "prod"}}, false},
		{"password", []EnvVarIntent{{Name: "DB_PASSWORD", Value: "x"}}, true},
		{"token", []EnvVarIntent{{Name: "API_TOKEN", Value: "x"}}, true},
		{"secret", []EnvVarIntent{{Name: "MY_SECRET", Value: "x"}}, true},
		{"key", []EnvVarIntent{{Name: "PRIVATE_KEY", Value: "x"}}, true},
		{"auth", []EnvVarIntent{{Name: "AUTH_HEADER", Value: "x"}}, true},
		{"credential", []EnvVarIntent{{Name: "AWS_CREDENTIAL", Value: "x"}}, true},
		{"pass", []EnvVarIntent{{Name: "REDIS_PASS", Value: "x"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasSecretEnvVars(tc.vars)
			if got != tc.expected {
				t.Fatalf("hasSecretEnvVars(%q) = %v, want %v", tc.name, got, tc.expected)
			}
		})
	}
}
