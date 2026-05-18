package planner

import (
	"testing"
)

func TestExportPlanSnapshot_Complete(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate})
	cs := BuildChangeSet(g, s)
	rg := BuildRollbackGraph(cs, g)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)

	snap := ExportPlanSnapshot(g, cs, s, rg, verdict, sim, nil, nil)

	if snap == nil {
		t.Fatal("snapshot must be non-nil")
	}
	if snap.PlanHash == "" {
		t.Fatal("PlanHash must be non-empty")
	}
	if snap.DesiredStateHash != s.DesiredStateHash {
		t.Fatal("snapshot must carry the DesiredStateHash")
	}
	if snap.ExportHash == "" {
		t.Fatal("ExportHash must be non-empty")
	}
	if snap.SchemaVersion != PlanSnapshotSchemaVersion {
		t.Fatalf("expected SchemaVersion=%q, got %q", PlanSnapshotSchemaVersion, snap.SchemaVersion)
	}
	if snap.Graph == nil {
		t.Fatal("snapshot must embed the Graph")
	}
	if snap.ChangeSet == nil {
		t.Fatal("snapshot must embed the ChangeSet")
	}
	if snap.RollbackGraph == nil {
		t.Fatal("snapshot must embed the RollbackGraph")
	}
}

func TestExportPlanSnapshot_Deterministic(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate})
	cs := BuildChangeSet(g, s)
	rg := BuildRollbackGraph(cs, g)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)

	snap1 := ExportPlanSnapshot(g, cs, s, rg, verdict, sim, nil, nil)
	snap2 := ExportPlanSnapshot(g, cs, s, rg, verdict, sim, nil, nil)

	// PlanHash must be identical (content-addressed).
	if snap1.PlanHash != snap2.PlanHash {
		t.Fatalf("PlanHash must be deterministic: %q vs %q", snap1.PlanHash, snap2.PlanHash)
	}
	// ExportHash must be identical (GeneratedAt excluded from hash).
	if snap1.ExportHash != snap2.ExportHash {
		t.Fatalf("ExportHash must be deterministic: %q vs %q", snap1.ExportHash, snap2.ExportHash)
	}
}

func TestVerifyExportHash_Valid(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	rg := BuildRollbackGraph(cs, g)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)

	snap := ExportPlanSnapshot(g, cs, s, rg, verdict, sim, nil, nil)

	ok, reason := VerifyExportHash(snap)
	if !ok {
		t.Fatalf("export hash verification must pass for unmodified snapshot: %s", reason)
	}
}

func TestVerifyExportHash_Tampered(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	rg := BuildRollbackGraph(cs, g)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)

	snap := ExportPlanSnapshot(g, cs, s, rg, verdict, sim, nil, nil)

	// Tamper with the plan hash after export.
	snap.PlanHash = "tampered-hash"

	ok, reason := VerifyExportHash(snap)
	if ok {
		t.Fatal("tampered snapshot must fail export hash verification")
	}
	if reason == "" {
		t.Fatal("failure reason must be non-empty")
	}
}

func TestVerifyExportHash_NilSnapshot(t *testing.T) {
	ok, reason := VerifyExportHash(nil)
	if ok {
		t.Fatal("nil snapshot must fail verification")
	}
	if reason == "" {
		t.Fatal("nil snapshot failure must have a reason")
	}
}

func TestVerifyExportHash_EmptyHash(t *testing.T) {
	snap := &PlanSnapshot{} // no ExportHash
	ok, reason := VerifyExportHash(snap)
	if ok {
		t.Fatal("snapshot with empty ExportHash must fail")
	}
	if reason == "" {
		t.Fatal("failure reason must be non-empty")
	}
}

func TestVerifyPlanHash_Valid(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionCreate})
	cs := BuildChangeSet(g, s)
	rg := BuildRollbackGraph(cs, g)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)

	snap := ExportPlanSnapshot(g, cs, s, rg, verdict, sim, nil, nil)

	ok, reason := VerifyPlanHash(snap, s, nil, nil)
	if !ok {
		t.Fatalf("plan hash verification must pass for unmodified snapshot: %s", reason)
	}
}

func TestVerifyPlanHash_DifferentInputs_Fails(t *testing.T) {
	s1 := newTestDesiredState()
	g1 := BuildPlanGraph(s1, map[string]ActionType{"svc-a": ActionCreate})
	cs1 := BuildChangeSet(g1, s1)
	rg1 := BuildRollbackGraph(cs1, g1)
	sim1 := SimulatePlan(g1, cs1, s1)
	v1 := EvaluateSafety(g1, cs1, s1, sim1)
	snap := ExportPlanSnapshot(g1, cs1, s1, rg1, v1, sim1, nil, nil)

	// Verify with a different DesiredState → hash mismatch.
	s2 := NewDesiredState(1, EnvironmentStaging,
		[]ServiceSpec{{ID: "svc-a", Provider: "aws-ecs"}},
		nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil,
	)
	g2 := BuildPlanGraph(s2, map[string]ActionType{"svc-a": ActionCreate})
	cs2 := BuildChangeSet(g2, s2)

	ok, _ := VerifyPlanHash(snap, s2, nil, nil)
	_ = cs2
	if ok {
		t.Fatal("verifying with wrong DesiredState must fail")
	}
}

func TestVerifyPlanHash_NilSnapshot(t *testing.T) {
	ok, reason := VerifyPlanHash(nil, newTestDesiredState(), nil, nil)
	if ok {
		t.Fatal("nil snapshot must fail")
	}
	if reason == "" {
		t.Fatal("nil snapshot must have a failure reason")
	}
}

func TestVerifyPlanHash_NilGraph(t *testing.T) {
	snap := &PlanSnapshot{PlanHash: "x", Graph: nil}
	ok, reason := VerifyPlanHash(snap, newTestDesiredState(), nil, nil)
	if ok {
		t.Fatal("nil graph in snapshot must fail plan hash verification")
	}
	if reason == "" {
		t.Fatal("failure reason must be non-empty")
	}
}

func TestVerifyPlanHash_WithObsFingerprints(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	rg := BuildRollbackGraph(cs, g)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)

	obs := []string{"obs-1", "obs-2"}
	snap := ExportPlanSnapshot(g, cs, s, rg, verdict, sim, obs, nil)

	ok, _ := VerifyPlanHash(snap, s, obs, nil)
	if !ok {
		t.Fatal("plan hash must verify correctly with matching obs fingerprints")
	}

	// Wrong obs fingerprints → should fail.
	ok2, _ := VerifyPlanHash(snap, s, []string{"different-obs"}, nil)
	if ok2 {
		t.Fatal("plan hash must fail with mismatched obs fingerprints")
	}
}

func TestPlanSnapshot_SimulationSummary(t *testing.T) {
	s := newTestDesiredState()
	g := BuildPlanGraph(s, map[string]ActionType{"svc-a": ActionDelete})
	cs := BuildChangeSet(g, s)
	rg := BuildRollbackGraph(cs, g)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)

	snap := ExportPlanSnapshot(g, cs, s, rg, verdict, sim, nil, nil)

	if snap.SimulationSummary.IrreversibleNodeCount == 0 {
		t.Fatal("simulation summary must report irreversible node count for delete actions")
	}
	if snap.SimulationSummary.HighestRisk == "" {
		t.Fatal("simulation summary highest risk must be non-empty")
	}
	if len(snap.SimulationSummary.SimulationLimitations) == 0 {
		t.Fatal("simulation summary must include limitations")
	}
}

func TestPlanSnapshot_Environment(t *testing.T) {
	s := NewDesiredState(1, EnvironmentStaging, nil, nil, nil, nil, nil, RolloutStrategySpec{}, SafetyProfile{}, nil)
	g := BuildPlanGraph(s, nil)
	cs := BuildChangeSet(g, s)
	rg := BuildRollbackGraph(cs, g)
	sim := SimulatePlan(g, cs, s)
	verdict := EvaluateSafety(g, cs, s, sim)
	snap := ExportPlanSnapshot(g, cs, s, rg, verdict, sim, nil, nil)

	if snap.Environment != EnvironmentStaging {
		t.Fatalf("snapshot must carry the environment, got %q", snap.Environment)
	}
}
