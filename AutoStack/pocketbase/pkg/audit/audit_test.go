package audit

import (
	"testing"
)

// ─── RunProductionTruthAudit ──────────────────────────────────────────────────

func TestRunProductionTruthAudit_CleanInput_Passes(t *testing.T) {
	input := AuditInput{
		ExecutionID:         "exec-1",
		TenantID:            "tenant-1",
		ReplayDivergences:   0,
		CheckpointHashValid: true,
		ArchiveHashValid:    true,
		ContradictionCount:  0,
		WorkflowDivergences: 0,
		TenantsCrossed:      false,
		ClockMonotonic:      true,
	}
	result := RunProductionTruthAudit("audit-1", input)
	if !result.Passed {
		t.Fatalf("clean input must pass audit, findings: %v", result.Findings)
	}
}

func TestRunProductionTruthAudit_ReplayDivergences_Fails(t *testing.T) {
	input := AuditInput{
		ExecutionID:         "exec-1",
		TenantID:            "t1",
		ReplayDivergences:   2,
		CheckpointHashValid: true,
		ArchiveHashValid:    true,
		ClockMonotonic:      true,
	}
	result := RunProductionTruthAudit("audit-1", input)
	if result.Passed {
		t.Fatal("replay divergences must fail audit")
	}
	found := false
	for _, f := range result.Findings {
		if f.Dimension == DimensionReplayIntegrity {
			found = true
		}
	}
	if !found {
		t.Fatal("replay_integrity finding must be present")
	}
}

func TestRunProductionTruthAudit_TamperedCheckpoint_Fails(t *testing.T) {
	input := AuditInput{
		ExecutionID:         "exec-1",
		TenantID:            "t1",
		CheckpointHashValid: false,
		ArchiveHashValid:    true,
		ClockMonotonic:      true,
	}
	result := RunProductionTruthAudit("audit-1", input)
	if result.Passed {
		t.Fatal("invalid checkpoint must fail audit")
	}
}

func TestRunProductionTruthAudit_TenantCrossed_Fails(t *testing.T) {
	input := AuditInput{
		ExecutionID:         "exec-1",
		TenantID:            "t1",
		CheckpointHashValid: true,
		ArchiveHashValid:    true,
		TenantsCrossed:      true,
		ClockMonotonic:      true,
	}
	result := RunProductionTruthAudit("audit-1", input)
	if result.Passed {
		t.Fatal("tenant crossing must fail audit")
	}
}

func TestRunProductionTruthAudit_WorkflowDivergences_Warning_Passes(t *testing.T) {
	input := AuditInput{
		ExecutionID:         "exec-1",
		TenantID:            "t1",
		CheckpointHashValid: true,
		ArchiveHashValid:    true,
		WorkflowDivergences: 1, // warning only, not critical
		ClockMonotonic:      true,
	}
	result := RunProductionTruthAudit("audit-1", input)
	if !result.Passed {
		t.Fatal("workflow divergence warnings alone must not fail audit")
	}
	hasWarning := false
	for _, f := range result.Findings {
		if f.Dimension == DimensionWorkflowDivergence && f.Severity == "warning" {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Fatal("workflow_divergence warning must be present")
	}
}

func TestRunProductionTruthAudit_HashValid(t *testing.T) {
	input := AuditInput{
		ExecutionID:         "exec-1",
		TenantID:            "t1",
		CheckpointHashValid: true,
		ArchiveHashValid:    true,
		ClockMonotonic:      true,
	}
	result := RunProductionTruthAudit("audit-1", input)
	ok, reason := VerifyAuditHash(result)
	if !ok {
		t.Fatalf("audit hash invalid: %s", reason)
	}
}

func TestRunProductionTruthAudit_AuditedAtExcluded(t *testing.T) {
	input := AuditInput{ExecutionID: "e1", TenantID: "t1", CheckpointHashValid: true, ArchiveHashValid: true, ClockMonotonic: true}
	r1 := RunProductionTruthAudit("audit-1", input)
	r2 := r1
	r2.AuditedAt = "2099-01-01T00:00:00Z"
	if r1.AuditHash != r2.AuditHash {
		t.Fatal("AuditedAt must not affect audit hash")
	}
}

// ─── CertifyReplay ────────────────────────────────────────────────────────────

func TestCertifyReplay_AllPass_Certified(t *testing.T) {
	r := CertifyReplay("cert-1", "exec-1", "t1", true, true, true, true, 100, 0)
	if !r.Certified {
		t.Fatal("all checks passing must be certified")
	}
	ok, reason := VerifyCertificationHash(r)
	if !ok {
		t.Fatalf("certification hash invalid: %s", reason)
	}
}

func TestCertifyReplay_OneFailing_NotCertified(t *testing.T) {
	r := CertifyReplay("cert-1", "exec-1", "t1", true, false, true, true, 100, 0)
	if r.Certified {
		t.Fatal("failed checkpoint must not be certified")
	}
}

func TestCertifyReplay_CertifiedAtExcluded(t *testing.T) {
	r1 := CertifyReplay("cert-1", "exec-1", "t1", true, true, true, true, 50, 0)
	r2 := r1
	r2.CertifiedAt = "2099-01-01T00:00:00Z"
	if r1.CertificationHash != r2.CertificationHash {
		t.Fatal("CertifiedAt must not affect certification hash")
	}
}

// ─── ExecutionForensicsBundle ────────────────────────────────────────────────

func TestBuildForensicsBundle_HashValid(t *testing.T) {
	journals := []ForensicRef{{Kind: "journal", ID: "j-1", Hash: "abc"}}
	events := []ForensicRef{{Kind: "event", ID: "e-1", Hash: "def"}}
	bundle := BuildForensicsBundle("bundle-1", "exec-1", "t1",
		journals, events, nil, nil, nil, nil, nil, nil)
	ok, reason := VerifyBundleHash(bundle)
	if !ok {
		t.Fatalf("bundle hash invalid: %s", reason)
	}
}

func TestBuildForensicsBundle_BundledAtExcluded(t *testing.T) {
	b1 := BuildForensicsBundle("b-1", "exec-1", "t1", nil, nil, nil, nil, nil, nil, nil, nil)
	b2 := b1
	b2.BundledAt = "2099-01-01T00:00:00Z"
	if b1.BundleHash != b2.BundleHash {
		t.Fatal("BundledAt must not affect bundle hash")
	}
}

func TestBuildForensicsBundle_DeepCopy(t *testing.T) {
	journals := []ForensicRef{{Kind: "journal", ID: "j-1"}}
	bundle := BuildForensicsBundle("b-1", "exec-1", "t1",
		journals, nil, nil, nil, nil, nil, nil, nil)
	journals[0].ID = "mutated"
	if bundle.Journals[0].ID == "mutated" {
		t.Fatal("bundle must deep-copy refs")
	}
}

// ─── AssessOperatorSafety ─────────────────────────────────────────────────────

func TestAssessOperatorSafety_Safe(t *testing.T) {
	input := OperatorSafetyInput{
		ExecutionID:           "exec-1",
		TenantID:              "t1",
		AutomatedActionsCount: 5,
		ApprovedActionsCount:  5,
	}
	a := AssessOperatorSafety("safety-1", input)
	if !a.Safe {
		t.Fatalf("fully approved automation must be safe: %v", a.Violations)
	}
	ok, reason := VerifySafetyHash(a)
	if !ok {
		t.Fatalf("safety hash invalid: %s", reason)
	}
}

func TestAssessOperatorSafety_UnsafeAutomation(t *testing.T) {
	input := OperatorSafetyInput{
		ExecutionID:           "exec-1",
		TenantID:              "t1",
		AutomatedActionsCount: 5,
		ApprovedActionsCount:  3,
	}
	a := AssessOperatorSafety("safety-1", input)
	if a.Safe {
		t.Fatal("unapproved automation must not be safe")
	}
	found := false
	for _, v := range a.Violations {
		if v.ViolationType == ViolationUnsafeAutomation {
			found = true
		}
	}
	if !found {
		t.Fatal("unsafe_automation violation must be present")
	}
}

func TestAssessOperatorSafety_PolicyBypass(t *testing.T) {
	input := OperatorSafetyInput{
		ExecutionID:          "exec-1",
		TenantID:             "t1",
		PolicyBypassAttempts: 2,
		ApprovedActionsCount: 0,
	}
	a := AssessOperatorSafety("safety-1", input)
	if a.Safe {
		t.Fatal("policy bypass must not be safe")
	}
}

func TestAssessOperatorSafety_UnauthorizedOperator(t *testing.T) {
	input := OperatorSafetyInput{
		ExecutionID:            "exec-1",
		TenantID:               "t1",
		UnauthorizedOperatorID: "unknown-actor",
	}
	a := AssessOperatorSafety("safety-1", input)
	if a.Safe {
		t.Fatal("unauthorized operator must not be safe")
	}
}

func TestAssessOperatorSafety_QuarantineWarning_StillSafe(t *testing.T) {
	input := OperatorSafetyInput{
		ExecutionID:         "exec-1",
		TenantID:            "t1",
		QuarantinedCount:    2,
		QuarantineOverrides: 1,
	}
	a := AssessOperatorSafety("safety-1", input)
	// Quarantine suppression is a warning, not critical.
	if !a.Safe {
		t.Fatalf("quarantine warning alone must not make unsafe: %v", a.Violations)
	}
	hasWarning := false
	for _, v := range a.Violations {
		if v.ViolationType == ViolationQuarantineSuppression {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Fatal("quarantine_suppression warning must be present")
	}
}

func TestAssessOperatorSafety_AssessedAtExcluded(t *testing.T) {
	input := OperatorSafetyInput{ExecutionID: "exec-1", TenantID: "t1"}
	a1 := AssessOperatorSafety("safety-1", input)
	a2 := a1
	a2.AssessedAt = "2099-01-01T00:00:00Z"
	if a1.AssessmentHash != a2.AssessmentHash {
		t.Fatal("AssessedAt must not affect safety hash")
	}
}

// ─── PlatformRuntimeSnapshot ──────────────────────────────────────────────────

func TestBuildPlatformRuntimeSnapshot_HashValid(t *testing.T) {
	snap := BuildPlatformRuntimeSnapshot("snap-1", "t1",
		10, 5, 3, 2, 0, 4, true,
		nil, nil, nil, "healthy")
	ok, reason := VerifyPlatformSnapshot(snap)
	if !ok {
		t.Fatalf("platform snapshot hash invalid: %s", reason)
	}
}

func TestBuildPlatformRuntimeSnapshot_SnapshotAtExcluded(t *testing.T) {
	s1 := BuildPlatformRuntimeSnapshot("snap-1", "t1", 0, 0, 0, 0, 0, 0, true, nil, nil, nil, "healthy")
	s2 := s1
	s2.SnapshotAt = "2099-01-01T00:00:00Z"
	if s1.SnapshotHash != s2.SnapshotHash {
		t.Fatal("SnapshotAt must not affect platform snapshot hash")
	}
}

func TestBuildPlatformRuntimeSnapshot_WithCertifications(t *testing.T) {
	cert := CertifyReplay("cert-1", "exec-1", "t1", true, true, true, true, 100, 0)
	auditInput := AuditInput{ExecutionID: "exec-1", TenantID: "t1", CheckpointHashValid: true, ArchiveHashValid: true, ClockMonotonic: true}
	auditResult := RunProductionTruthAudit("audit-1", auditInput)
	safetyInput := OperatorSafetyInput{ExecutionID: "exec-1", TenantID: "t1"}
	safetyResult := AssessOperatorSafety("safety-1", safetyInput)

	snap := BuildPlatformRuntimeSnapshot("snap-1", "t1",
		1, 1, 2, 0, 0, 3, true,
		[]ReplayCertificationReport{cert},
		[]AuditResult{auditResult},
		[]OperatorSafetyAssessment{safetyResult},
		"healthy")

	ok, reason := VerifyPlatformSnapshot(snap)
	if !ok {
		t.Fatalf("platform snapshot with certs hash invalid: %s", reason)
	}
}
