package prodcert

import (
	"testing"

	"github.com/janlauber/one-click/pkg/audit"
	"github.com/janlauber/one-click/pkg/backup"
	"github.com/janlauber/one-click/pkg/certification"
	"github.com/janlauber/one-click/pkg/chaos"
	"github.com/janlauber/one-click/pkg/metrics"
	"github.com/janlauber/one-click/pkg/observability"
)

func buildPlatformCert(t *testing.T) certification.PlatformCertificationReport {
	t.Helper()
	auditRes := audit.RunProductionTruthAudit("a-1", audit.AuditInput{
		ExecutionID:         "exec-1",
		TenantID:            "tenant-a",
		ReplayDivergences:   0,
		CheckpointHashValid: true,
		PolicyDecisions:     5,
		PolicyDenials:       0,
		ContradictionCount:  0,
		WorkflowDivergences: 0,
		ArchiveHashValid:    true,
		TenantsCrossed:      false,
		ClockMonotonic:      true,
	})
	safetyRes := audit.AssessOperatorSafety("s-1", audit.OperatorSafetyInput{
		ExecutionID:           "exec-1",
		TenantID:              "tenant-a",
		AutomatedActionsCount: 3,
		ApprovedActionsCount:  3,
	})
	replayCert := audit.CertifyReplay("rc-1", "exec-1", "tenant-a", true, true, true, true, 100, 0)
	health := observability.AssessHealth(observability.ObservabilityMetrics{
		TenantID:        "tenant-a",
		ExecutionID:     "exec-1",
		TotalTasks:      10,
		CompletedTasks:  10,
		ConfidenceScore: 1.0,
	})
	return certification.RunPlatformReadinessAudit("cert-1", certification.PlatformReadinessInput{
		ExecutionID:         "exec-1",
		TenantID:            "tenant-a",
		AuditResult:         auditRes,
		SafetyAssessment:    safetyRes,
		ReplayCertification: &replayCert,
		HealthAssessment:    health,
	})
}

func buildBackupReadiness(t *testing.T) backup.ProductionReadinessReport {
	t.Helper()
	return backup.RunProductionReadinessAudit("rpt-pc", backup.ReadinessInput{
		TenantID:                  "tenant-a",
		ExecutionID:               "exec-1",
		StorageReadsHealthy:       true,
		StorageWritesHealthy:      true,
		ReplayDivergenceCount:     0,
		ReplayCertified:           true,
		ContradictionScanOK:       true,
		GovernanceDecisionCount:   5,
		GovernanceHashValid:       true,
		ArchiveEntryCount:         50,
		ArchiveHashValid:          true,
		ActiveWorkerCount:         2,
		QuarantinedWorkers:        0,
		APIEndpointsHealthy:       true,
		RBACRulesValid:            true,
		NoCredentialLeak:          true,
		LatestBackupManifestValid: true,
	})
}

func buildFullInput(t *testing.T) ProductionCertInput {
	t.Helper()
	platCert := buildPlatformCert(t)
	replayCert := audit.CertifyReplay("rc-2", "exec-1", "tenant-a", true, true, true, true, 100, 0)
	safety := audit.AssessOperatorSafety("s-2", audit.OperatorSafetyInput{
		ExecutionID:           "exec-1",
		TenantID:              "tenant-a",
		AutomatedActionsCount: 2,
		ApprovedActionsCount:  2,
	})
	health := observability.AssessHealth(observability.ObservabilityMetrics{
		TenantID:        "tenant-a",
		ExecutionID:     "exec-1",
		TotalTasks:      10,
		CompletedTasks:  10,
		ConfidenceScore: 1.0,
	})
	backupReady := buildBackupReadiness(t)

	m := metrics.NewPlatformMetrics()
	m.TasksTotal.Add(10)
	m.TasksCompleted.Add(10)
	snapshot := m.Snapshot()

	engine := chaos.NewChaosEngine(42)
	engine.InjectFault(chaos.FaultSpec{FaultID: "f-1", FaultType: chaos.FaultStorageWrite, Severity: chaos.SeverityHigh})
	engine.InjectFault(chaos.FaultSpec{FaultID: "f-2", FaultType: chaos.FaultWorkerCrash, Severity: chaos.SeverityCritical})
	engine.InjectFault(chaos.FaultSpec{FaultID: "f-3", FaultType: chaos.FaultReplayStorm, Severity: chaos.SeverityHigh})

	return ProductionCertInput{
		TenantID:         "tenant-a",
		ExecutionID:      "exec-1",
		PlatformCert:     platCert,
		ReplayCert:       replayCert,
		SafetyAssessment: safety,
		HealthAssessment: health,
		BackupReadiness:  backupReady,
		MetricsSnapshot:  snapshot,
		ChaosOutcomes:    engine.AllOutcomes(),
	}
}

func TestProductionCertification_AllGatesPass(t *testing.T) {
	report := RunProductionCertification("prod-cert-1", buildFullInput(t))
	if !report.Certified {
		for _, g := range report.Gates {
			if !g.Passed {
				t.Errorf("gate %s failed: %s", g.Subsystem, g.Detail)
			}
		}
		t.Fatalf("expected all gates to pass (%d/%d passed)", report.GatesPassed, report.GatesTotal)
	}
}

func TestProductionCertification_HashIntegrity(t *testing.T) {
	report := RunProductionCertification("prod-cert-hash", buildFullInput(t))
	ok, reason := VerifyProductionCertification(report)
	if !ok {
		t.Fatalf("production cert hash invalid: %s", reason)
	}
}

func TestProductionCertification_TamperedHashDetected(t *testing.T) {
	report := RunProductionCertification("prod-cert-tamper", buildFullInput(t))
	report.GatesPassed = 0 // tamper
	ok, _ := VerifyProductionCertification(report)
	if ok {
		t.Fatal("tampered production cert should fail hash verification")
	}
}

func TestProductionCertification_CertifiedAtExcluded(t *testing.T) {
	report := RunProductionCertification("prod-cert-ts", buildFullInput(t))
	hash1 := report.CertHash
	report.CertifiedAt = "3000-01-01T00:00:00Z"
	hash2 := computeProdCertHash(report)
	if hash1 != hash2 {
		t.Error("CertHash must not change when only CertifiedAt changes (content-identity)")
	}
}

func TestProductionCertification_HasPlatformClaim(t *testing.T) {
	report := RunProductionCertification("prod-cert-claim", buildFullInput(t))
	if report.PlatformClaim == "" {
		t.Error("production certification must include a platform claim")
	}
}

func TestProductionCertification_HasLimitations(t *testing.T) {
	report := RunProductionCertification("prod-cert-lim", buildFullInput(t))
	if len(report.Limitations) == 0 {
		t.Error("production certification must document its limitations")
	}
}

func TestProductionCertification_CorrectSchemaVersion(t *testing.T) {
	report := RunProductionCertification("prod-cert-ver", buildFullInput(t))
	if report.SchemaVersion != ProductionCertSchemaVersion {
		t.Errorf("expected schema version %s, got %s", ProductionCertSchemaVersion, report.SchemaVersion)
	}
}

func TestProductionCertification_FailsWhenPlatformCertNotCertified(t *testing.T) {
	in := buildFullInput(t)
	in.PlatformCert.PlatformCertified = false
	in.PlatformCert.GatesPassed = 3
	// Recompute hash to match tampered state — cert will now fail verification.
	report := RunProductionCertification("prod-cert-platfail", in)
	if report.Certified {
		t.Fatal("should not be certified when platform cert is not certified")
	}
}

func TestProductionCertification_FailsWhenReplayNotCertified(t *testing.T) {
	in := buildFullInput(t)
	in.ReplayCert.Certified = false
	report := RunProductionCertification("prod-cert-replayfail", in)
	if report.Certified {
		t.Fatal("should not be certified when replay is not certified")
	}
}

func TestProductionCertification_FailsWhenUnsafe(t *testing.T) {
	in := buildFullInput(t)
	in.SafetyAssessment.Safe = false
	report := RunProductionCertification("prod-cert-safetyfail", in)
	if report.Certified {
		t.Fatal("should not be certified when safety assessment fails")
	}
}

func TestProductionCertification_FailsWhenCriticalHealth(t *testing.T) {
	in := buildFullInput(t)
	in.HealthAssessment.Status = observability.HealthStatusCritical
	report := RunProductionCertification("prod-cert-healthfail", in)
	if report.Certified {
		t.Fatal("should not be certified when health is critical")
	}
}

func TestProductionCertification_FailsWhenNoMetrics(t *testing.T) {
	in := buildFullInput(t)
	in.MetricsSnapshot = nil
	report := RunProductionCertification("prod-cert-metricsfail", in)
	if report.Certified {
		t.Fatal("should not be certified with no metrics snapshot")
	}
}

func TestProductionCertification_FailsWhenNoChaosOutcomes(t *testing.T) {
	in := buildFullInput(t)
	in.ChaosOutcomes = nil
	report := RunProductionCertification("prod-cert-chaosfail", in)
	if report.Certified {
		t.Fatal("should not be certified with no chaos outcomes")
	}
}

func TestProductionCertification_GateTotalIsFixed(t *testing.T) {
	report := RunProductionCertification("prod-cert-total", buildFullInput(t))
	if report.GatesTotal != 10 {
		t.Errorf("expected 10 gates total, got %d", report.GatesTotal)
	}
}
