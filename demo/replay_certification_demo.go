// demo/replay_certification_demo.go demonstrates the complete replay
// certification flow: record events → replay → certify → verify.
//
// Run with: go run ./demo/replay_certification_demo.go
package main

import (
	"fmt"
	"time"

	"github.com/janlauber/one-click/pkg/audit"
	"github.com/janlauber/one-click/pkg/certification"
	"github.com/janlauber/one-click/pkg/observability"
)

func main() {
	executionID := "demo-exec-001"
	tenantID := "demo-tenant"

	fmt.Println("=== AutoStack Replay Certification Demo ===")
	fmt.Println()

	// Step 1: Run the production truth audit.
	fmt.Println("[1] Running production truth audit...")
	auditInput := audit.AuditInput{
		ExecutionID:         executionID,
		TenantID:            tenantID,
		ReplayDivergences:   0,
		CheckpointHashValid: true,
		PolicyDecisions:     12,
		PolicyDenials:       1, // one deny is fine
		ContradictionCount:  0,
		WorkflowDivergences: 0,
		ArchiveHashValid:    true,
		TenantsCrossed:      false,
		ClockMonotonic:      true,
	}
	auditResult := audit.RunProductionTruthAudit("audit-demo-001", auditInput)
	fmt.Printf("   Audit passed: %v | findings: %d\n", auditResult.Passed, len(auditResult.Findings))

	// Step 2: Operator safety assessment.
	fmt.Println("[2] Assessing operator safety...")
	safetyInput := audit.OperatorSafetyInput{
		ExecutionID:           executionID,
		TenantID:              tenantID,
		AutomatedActionsCount: 10,
		ApprovedActionsCount:  10,
		PolicyBypassAttempts:  0,
		ReplayDivergences:     0,
		QuarantinedCount:      0,
		QuarantineOverrides:   0,
	}
	safety := audit.AssessOperatorSafety("safety-demo-001", safetyInput)
	fmt.Printf("   Safe: %v | violations: %d\n", safety.Safe, len(safety.Violations))

	// Step 3: Certify the replay.
	fmt.Println("[3] Certifying replay...")
	replayCert := audit.CertifyReplay(
		"cert-demo-001", executionID, tenantID,
		true, // timeline integrity
		true, // checkpoint valid
		true, // divergence free
		true, // confidence consistent
		48,   // events verified
		0,    // divergences
	)
	fmt.Printf("   Certified: %v | events verified: %d\n", replayCert.Certified, replayCert.TotalEventsVerified)
	hashOK, _ := audit.VerifyCertificationHash(replayCert)
	fmt.Printf("   Hash valid: %v\n", hashOK)

	// Step 4: Platform health assessment.
	fmt.Println("[4] Assessing platform health...")
	health := observability.AssessHealth(observability.ObservabilityMetrics{
		ExecutionID:        executionID,
		TenantID:           tenantID,
		TotalTasks:         48,
		CompletedTasks:     46,
		FailedTasks:        2,
		ConfidenceScore:    0.96,
		ContradictionCount: 0,
	})
	fmt.Printf("   Health status: %s\n", health.Status)

	// Step 5: Run platform readiness audit.
	fmt.Println("[5] Running platform readiness audit...")
	input := certification.PlatformReadinessInput{
		ExecutionID:         executionID,
		TenantID:            tenantID,
		AuditResult:         auditResult,
		SafetyAssessment:    safety,
		ReplayCertification: &replayCert,
		HealthAssessment:    health,
	}
	report := certification.RunPlatformReadinessAudit("platform-cert-demo-001", input)
	fmt.Printf("\n=== Platform Certification Report ===\n")
	fmt.Printf("   Certification ID : %s\n", report.CertificationID)
	fmt.Printf("   Execution ID     : %s\n", report.ExecutionID)
	fmt.Printf("   Schema Version   : %s\n", report.SchemaVersion)
	fmt.Printf("   Gates Passed     : %d / %d\n", report.GatesPassed, report.GatesTotal)
	fmt.Printf("   Platform Certified: %v\n", report.PlatformCertified)
	fmt.Printf("   Certified At     : %s\n", report.CertifiedAt)
	fmt.Println()
	fmt.Println("   Gate Results:")
	for _, g := range report.Gates {
		status := "PASS"
		if !g.Passed {
			status = "FAIL"
		}
		fmt.Printf("     [%s] %s", status, g.Name)
		if g.Reason != "" {
			fmt.Printf(" — %s", g.Reason)
		}
		fmt.Println()
	}

	// Step 6: Verify certification hash.
	fmt.Println()
	fmt.Println("[6] Verifying certification hash integrity...")
	ok, reason := certification.VerifyPlatformCertification(report)
	fmt.Printf("   Hash valid: %v", ok)
	if !ok {
		fmt.Printf(" — %s", reason)
	}
	fmt.Println()

	// Step 7: Build certified snapshot.
	fmt.Println("[7] Building certified platform snapshot...")
	snap := certification.BuildCertifiedPlatformSnapshot(report, auditResult, safety, health)
	snapOK, _ := certification.VerifyCertifiedPlatformSnapshot(snap)
	fmt.Printf("   Snapshot hash valid: %v\n", snapOK)
	fmt.Printf("   Snapshot at: %s\n", snap.SnapshotAt)

	fmt.Println()
	if report.PlatformCertified {
		fmt.Println("✓ Platform is CERTIFIED for production operation.")
	} else {
		fmt.Println("✗ Platform is NOT certified — review gate failures above.")
	}

	fmt.Println()
	fmt.Println("Limitations:")
	for _, l := range report.Limitations {
		fmt.Printf("  • %s\n", l)
	}

	_ = time.Now() // suppress unused import warning
}
