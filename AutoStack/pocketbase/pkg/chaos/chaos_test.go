package chaos

import (
	"fmt"
	"testing"
)

func TestChaosEngine_StorageWriteFailure(t *testing.T) {
	engine := NewChaosEngine(42)
	spec := FaultSpec{
		FaultID:   "f-storage-write-1",
		FaultType: FaultStorageWrite,
		Severity:  SeverityHigh,
		TargetID:  "tenant-a",
	}
	o := engine.InjectFault(spec)
	if !o.Injected {
		t.Error("fault should be marked as injected")
	}
	if !o.EvidenceIntact {
		t.Error("evidence must remain intact after storage write failure")
	}
	if !o.ReplayConsistent {
		t.Error("replay must remain consistent after storage write failure")
	}
}

func TestChaosEngine_WorkerCrash_NoAutoRecover(t *testing.T) {
	engine := NewChaosEngine(42)
	spec := FaultSpec{
		FaultID:   "f-worker-crash-1",
		FaultType: FaultWorkerCrash,
		Severity:  SeverityCritical,
		TargetID:  "worker-001",
	}
	o := engine.InjectFault(spec)
	if !o.EvidenceIntact {
		t.Error("evidence must be intact after worker crash")
	}
	// Worker crash requires operator action — no auto-recovery.
	if o.RecoveryDetected {
		t.Error("worker crash should NOT auto-recover: evidence>automation invariant")
	}
}

func TestChaosEngine_WorkerRestart_Recovers(t *testing.T) {
	engine := NewChaosEngine(42)
	spec := FaultSpec{
		FaultID:   "f-worker-restart-1",
		FaultType: FaultWorkerRestart,
		Severity:  SeverityMedium,
		TargetID:  "worker-001",
	}
	o := engine.InjectFault(spec)
	if !o.RecoveryDetected {
		t.Error("worker restart with lease expiry should allow recovery")
	}
	if !o.EvidenceIntact {
		t.Error("evidence must be intact after worker restart")
	}
}

func TestChaosEngine_NetworkPartition_NoAutoMerge(t *testing.T) {
	engine := NewChaosEngine(42)
	spec := FaultSpec{
		FaultID:   "f-partition-1",
		FaultType: FaultNetworkPartition,
		Severity:  SeverityCritical,
		TargetID:  "federation-node-2",
	}
	o := engine.InjectFault(spec)
	if !o.EvidenceIntact {
		t.Error("evidence must be intact during network partition")
	}
	// Partitioned nodes must not silently merge — operator resolves.
	if o.RecoveryDetected {
		t.Error("network partition should NOT auto-resolve")
	}
}

func TestChaosEngine_ClockSkew_LogicalClockUnaffected(t *testing.T) {
	engine := NewChaosEngine(42)
	spec := FaultSpec{
		FaultID:   "f-clock-skew-1",
		FaultType: FaultClockSkew,
		Severity:  SeverityLow,
		TargetID:  "node-3",
	}
	o := engine.InjectFault(spec)
	if !o.ReplayConsistent {
		t.Error("clock skew must not affect replay determinism")
	}
	if !o.RecoveryDetected {
		t.Error("clock skew is self-correcting via logical clock")
	}
}

func TestChaosEngine_DataCorruption_Detected(t *testing.T) {
	engine := NewChaosEngine(42)
	spec := FaultSpec{
		FaultID:   "f-corrupt-1",
		FaultType: FaultDataCorruption,
		Severity:  SeverityCritical,
		TargetID:  "entry-hash-xyz",
	}
	o := engine.InjectFault(spec)
	if !o.EvidenceIntact {
		t.Error("corruption attempt must be caught by hash verification")
	}
	if !o.RecoveryDetected {
		t.Error("corruption detection is a form of recovery")
	}
}

func TestChaosEngine_GovernanceDelay_NoAutoApprove(t *testing.T) {
	engine := NewChaosEngine(42)
	spec := FaultSpec{
		FaultID:   "f-gov-delay-1",
		FaultType: FaultGovernanceDelay,
		Severity:  SeverityMedium,
		TargetID:  "approval-step-prod",
	}
	o := engine.InjectFault(spec)
	if !o.EvidenceIntact {
		t.Error("evidence must be intact during governance delay")
	}
	// Auto-approval would violate governance authority.
	if o.RecoveryDetected {
		t.Error("governance delay must NOT auto-approve: operator authority invariant")
	}
}

func TestChaosEngine_ReplayStorm_Deterministic(t *testing.T) {
	engine := NewChaosEngine(42)
	spec := FaultSpec{
		FaultID:   "f-replay-storm-1",
		FaultType: FaultReplayStorm,
		Severity:  SeverityHigh,
		TargetID:  "execution-abc",
	}
	o := engine.InjectFault(spec)
	if !o.ReplayConsistent {
		t.Error("concurrent replays must produce identical results")
	}
}

func TestChaosEngine_TaskTimeout_AuditRecorded(t *testing.T) {
	engine := NewChaosEngine(42)
	spec := FaultSpec{
		FaultID:   "f-timeout-1",
		FaultType: FaultTaskTimeout,
		Severity:  SeverityMedium,
		TargetID:  "task-xyz",
	}
	o := engine.InjectFault(spec)
	if !o.EvidenceIntact {
		t.Error("evidence must be intact after task timeout")
	}
	if !o.RecoveryDetected {
		t.Error("task timeout with lifecycle guard should be a recoverable state transition")
	}
}

func TestChaosEngine_UnknownFault_NotInjected(t *testing.T) {
	engine := NewChaosEngine(42)
	spec := FaultSpec{
		FaultID:   "f-unknown",
		FaultType: FaultType("unknown.fault"),
		Severity:  SeverityLow,
	}
	o := engine.InjectFault(spec)
	if o.Injected {
		t.Error("unknown fault type should not be injected")
	}
}

func TestChaosEngine_AllOutcomes_ReturnsCopy(t *testing.T) {
	engine := NewChaosEngine(42)
	engine.InjectFault(FaultSpec{FaultID: "f-1", FaultType: FaultStorageWrite, Severity: SeverityLow})
	engine.InjectFault(FaultSpec{FaultID: "f-2", FaultType: FaultWorkerCrash, Severity: SeverityHigh})

	outcomes := engine.AllOutcomes()
	if len(outcomes) != 2 {
		t.Errorf("expected 2 outcomes, got %d", len(outcomes))
	}
	// Mutate copy — should not affect engine state.
	outcomes[0].FaultID = "tampered"
	outcomes2 := engine.AllOutcomes()
	if outcomes2[0].FaultID == "tampered" {
		t.Error("AllOutcomes must return a copy, not the live slice")
	}
}

func TestChaosEngine_RunScenario_AllPass(t *testing.T) {
	engine := NewChaosEngine(99)
	scenario := ChaosScenario{
		ScenarioID:  "s-1",
		Name:        "storage-and-clock",
		Description: "Storage write failure followed by clock skew",
		Faults: []FaultSpec{
			{FaultID: "f-s1", FaultType: FaultStorageWrite, Severity: SeverityHigh},
			{FaultID: "f-s2", FaultType: FaultClockSkew, Severity: SeverityLow},
		},
	}
	result := engine.RunScenario(scenario)
	if !result.Passed {
		t.Errorf("scenario should pass: %s", result.Summary)
	}
	if len(result.Outcomes) != 2 {
		t.Errorf("expected 2 outcomes, got %d", len(result.Outcomes))
	}
}

func TestChaosEngine_FullResilience_Invariants(t *testing.T) {
	// All fault types must preserve EvidenceIntact and ReplayConsistent.
	engine := NewChaosEngine(12345)
	allFaults := []FaultType{
		FaultStorageWrite, FaultStorageRead, FaultWorkerCrash, FaultWorkerRestart,
		FaultNetworkPartition, FaultClockSkew, FaultDataCorruption,
		FaultGovernanceDelay, FaultReplayStorm, FaultTaskTimeout,
	}
	for i, ft := range allFaults {
		spec := FaultSpec{
			FaultID:   fmt.Sprintf("f-%d", i),
			FaultType: ft,
			Severity:  SeverityHigh,
			TargetID:  "target",
		}
		o := engine.InjectFault(spec)
		if !o.EvidenceIntact {
			t.Errorf("fault %s must preserve evidence integrity", ft)
		}
		if !o.ReplayConsistent {
			t.Errorf("fault %s must preserve replay consistency", ft)
		}
	}
}
