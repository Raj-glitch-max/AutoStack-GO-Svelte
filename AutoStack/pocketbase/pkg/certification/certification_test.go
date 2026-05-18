package certification

import (
	"testing"

	"github.com/janlauber/one-click/pkg/audit"
	"github.com/janlauber/one-click/pkg/observability"
)

func makeAuditResult(passed bool) audit.AuditResult {
	return audit.RunProductionTruthAudit("audit-1", audit.AuditInput{
		ExecutionID:         "exec-1",
		TenantID:            "tenant-1",
		ReplayDivergences:   0,
		CheckpointHashValid: true,
		PolicyDecisions:     5,
		PolicyDenials:       0,
		ContradictionCount:  0,
		WorkflowDivergences: 0,
		ArchiveHashValid:    true,
		TenantsCrossed:      false,
		ClockMonotonic:      passed,
	})
}

func makeSafeAssessment() audit.OperatorSafetyAssessment {
	return audit.AssessOperatorSafety("safety-1", audit.OperatorSafetyInput{
		ExecutionID:           "exec-1",
		TenantID:              "tenant-1",
		AutomatedActionsCount: 3,
		ApprovedActionsCount:  3,
		PolicyBypassAttempts:  0,
		ReplayDivergences:     0,
		QuarantinedCount:      0,
		QuarantineOverrides:   0,
	})
}

func makeHealthy() observability.HealthAssessment {
	return observability.AssessHealth(observability.ObservabilityMetrics{
		ExecutionID:        "exec-1",
		TenantID:           "tenant-1",
		TotalTasks:         10,
		CompletedTasks:     10,
		ConfidenceScore:    1.0,
		ContradictionCount: 0,
		ReplayDivergences:  0,
	})
}

func makeReplayCert(certified bool) *audit.ReplayCertificationReport {
	r := audit.CertifyReplay("replay-cert-1", "exec-1", "tenant-1",
		certified, certified, certified, certified, 10, 0)
	return &r
}

// ─── RunPlatformReadinessAudit ────────────────────────────────────────────────

func TestRunPlatformReadinessAudit_AllGatesPass_Certified(t *testing.T) {
	input := PlatformReadinessInput{
		ExecutionID:         "exec-1",
		TenantID:            "tenant-1",
		AuditResult:         makeAuditResult(true),
		SafetyAssessment:    makeSafeAssessment(),
		ReplayCertification: makeReplayCert(true),
		HealthAssessment:    makeHealthy(),
	}
	report := RunPlatformReadinessAudit("cert-1", input)
	if !report.PlatformCertified {
		for _, g := range report.Gates {
			if !g.Passed {
				t.Logf("gate %q failed: %s", g.Name, g.Reason)
			}
		}
		t.Fatal("all gates passed — platform must be certified")
	}
}

func TestRunPlatformReadinessAudit_NoReplayCert_NotCertified(t *testing.T) {
	input := PlatformReadinessInput{
		ExecutionID:         "exec-1",
		TenantID:            "tenant-1",
		AuditResult:         makeAuditResult(true),
		SafetyAssessment:    makeSafeAssessment(),
		ReplayCertification: nil, // missing
		HealthAssessment:    makeHealthy(),
	}
	report := RunPlatformReadinessAudit("cert-2", input)
	if report.PlatformCertified {
		t.Fatal("missing replay certification must prevent platform certification")
	}
}

func TestRunPlatformReadinessAudit_CriticalHealth_NotCertified(t *testing.T) {
	critical := observability.AssessHealth(observability.ObservabilityMetrics{
		ExecutionID:        "exec-1",
		TenantID:           "tenant-1",
		TotalTasks:         10,
		CompletedTasks:     1,   // very low success
		ConfidenceScore:    0.1, // below critical threshold
		ContradictionCount: 5,
	})
	input := PlatformReadinessInput{
		ExecutionID:         "exec-1",
		TenantID:            "tenant-1",
		AuditResult:         makeAuditResult(true),
		SafetyAssessment:    makeSafeAssessment(),
		ReplayCertification: makeReplayCert(true),
		HealthAssessment:    critical,
	}
	report := RunPlatformReadinessAudit("cert-3", input)
	if report.PlatformCertified {
		t.Fatal("critical health must prevent platform certification")
	}
}

func TestRunPlatformReadinessAudit_AllGatesEvaluated_NeverHalts(t *testing.T) {
	// Everything fails — all gates must still be evaluated.
	input := PlatformReadinessInput{
		ExecutionID:         "exec-1",
		TenantID:            "tenant-1",
		AuditResult:         audit.AuditResult{}, // empty/invalid
		SafetyAssessment:    audit.OperatorSafetyAssessment{},
		ReplayCertification: nil,
		HealthAssessment:    observability.HealthAssessment{Status: observability.HealthStatusCritical},
	}
	report := RunPlatformReadinessAudit("cert-4", input)
	if report.GatesTotal == 0 {
		t.Fatal("all gates must be evaluated even when all fail")
	}
	if report.PlatformCertified {
		t.Fatal("must not be certified when all gates fail")
	}
}

// ─── Hash Integrity ───────────────────────────────────────────────────────────

func TestVerifyPlatformCertification_ValidHash(t *testing.T) {
	input := PlatformReadinessInput{
		ExecutionID:         "exec-1",
		TenantID:            "tenant-1",
		AuditResult:         makeAuditResult(true),
		SafetyAssessment:    makeSafeAssessment(),
		ReplayCertification: makeReplayCert(true),
		HealthAssessment:    makeHealthy(),
	}
	report := RunPlatformReadinessAudit("cert-5", input)
	ok, reason := VerifyPlatformCertification(report)
	if !ok {
		t.Fatalf("certification hash must be valid: %s", reason)
	}
}

func TestVerifyPlatformCertification_TamperedHash_Fails(t *testing.T) {
	input := PlatformReadinessInput{
		ExecutionID:         "exec-1",
		TenantID:            "tenant-1",
		AuditResult:         makeAuditResult(true),
		SafetyAssessment:    makeSafeAssessment(),
		ReplayCertification: makeReplayCert(true),
		HealthAssessment:    makeHealthy(),
	}
	report := RunPlatformReadinessAudit("cert-6", input)
	report.GatesPassed = 99 // tamper
	ok, _ := VerifyPlatformCertification(report)
	if ok {
		t.Fatal("tampered report must not verify")
	}
}

// ─── CertifiedPlatformSnapshot ───────────────────────────────────────────────

func TestBuildCertifiedPlatformSnapshot_HashValid(t *testing.T) {
	input := PlatformReadinessInput{
		ExecutionID:         "exec-1",
		TenantID:            "tenant-1",
		AuditResult:         makeAuditResult(true),
		SafetyAssessment:    makeSafeAssessment(),
		ReplayCertification: makeReplayCert(true),
		HealthAssessment:    makeHealthy(),
	}
	report := RunPlatformReadinessAudit("cert-7", input)
	snap := BuildCertifiedPlatformSnapshot(report, input.AuditResult, input.SafetyAssessment, input.HealthAssessment)
	ok, reason := VerifyCertifiedPlatformSnapshot(snap)
	if !ok {
		t.Fatalf("snapshot hash must be valid: %s", reason)
	}
}

func TestBuildCertifiedPlatformSnapshot_TamperedHash_Fails(t *testing.T) {
	input := PlatformReadinessInput{
		ExecutionID:         "exec-1",
		TenantID:            "tenant-1",
		AuditResult:         makeAuditResult(true),
		SafetyAssessment:    makeSafeAssessment(),
		ReplayCertification: makeReplayCert(true),
		HealthAssessment:    makeHealthy(),
	}
	report := RunPlatformReadinessAudit("cert-8", input)
	snap := BuildCertifiedPlatformSnapshot(report, input.AuditResult, input.SafetyAssessment, input.HealthAssessment)
	// Tamper the certification hash inside the report — this changes what computeSnapshotHash sees.
	snap.Report.CertificationHash = "tampered"
	ok, _ := VerifyCertifiedPlatformSnapshot(snap)
	if ok {
		t.Fatal("tampered snapshot must not verify")
	}
}

func TestRunPlatformReadinessAudit_Limitations_Present(t *testing.T) {
	input := PlatformReadinessInput{
		ExecutionID:      "exec-1",
		TenantID:         "tenant-1",
		AuditResult:      makeAuditResult(true),
		SafetyAssessment: makeSafeAssessment(),
		HealthAssessment: makeHealthy(),
	}
	report := RunPlatformReadinessAudit("cert-9", input)
	if len(report.Limitations) == 0 {
		t.Fatal("platform certification must declare its limitations")
	}
}
