package backup

import (
	"testing"
)

func fullPassInput() ReadinessInput {
	return ReadinessInput{
		TenantID:                  "tenant-a",
		ExecutionID:               "exec-1",
		StorageReadsHealthy:       true,
		StorageWritesHealthy:      true,
		ReplayDivergenceCount:     0,
		ReplayCertified:           true,
		UnresolvedContradictions:  2, // visible, not blocking
		ContradictionScanOK:       true,
		GovernanceDecisionCount:   5,
		GovernanceHashValid:       true,
		ArchiveEntryCount:         100,
		ArchiveHashValid:          true,
		ActiveWorkerCount:         3,
		QuarantinedWorkers:        0,
		APIEndpointsHealthy:       true,
		RBACRulesValid:            true,
		NoCredentialLeak:          true,
		LatestBackupManifestValid: true,
	}
}

func TestProductionReadinessAudit_AllPass(t *testing.T) {
	report := RunProductionReadinessAudit("rpt-1", fullPassInput())
	if !report.Ready {
		for _, g := range report.Gates {
			if !g.Passed {
				t.Errorf("gate %s failed: %s", g.Check, g.Detail)
			}
		}
		t.Fatalf("expected all gates to pass")
	}
	if report.GatesPassed != report.GatesTotal {
		t.Errorf("gates passed %d != total %d", report.GatesPassed, report.GatesTotal)
	}
}

func TestProductionReadinessAudit_StorageFail(t *testing.T) {
	in := fullPassInput()
	in.StorageWritesHealthy = false
	report := RunProductionReadinessAudit("rpt-2", in)
	if report.Ready {
		t.Fatal("should not be ready when storage writes unhealthy")
	}
	found := false
	for _, g := range report.Gates {
		if g.Check == CheckStorageHealth && !g.Passed {
			found = true
		}
	}
	if !found {
		t.Error("storage.health gate should fail")
	}
}

func TestProductionReadinessAudit_ReplayDivergence(t *testing.T) {
	in := fullPassInput()
	in.ReplayDivergenceCount = 3
	report := RunProductionReadinessAudit("rpt-3", in)
	if report.Ready {
		t.Fatal("should not be ready with replay divergences")
	}
}

func TestProductionReadinessAudit_NotReplayCertified(t *testing.T) {
	in := fullPassInput()
	in.ReplayCertified = false
	report := RunProductionReadinessAudit("rpt-4", in)
	if report.Ready {
		t.Fatal("should not be ready without replay certification")
	}
}

func TestProductionReadinessAudit_ContradictionScanFailed(t *testing.T) {
	in := fullPassInput()
	in.ContradictionScanOK = false
	report := RunProductionReadinessAudit("rpt-5", in)
	if report.Ready {
		t.Fatal("should not be ready when contradiction scan failed")
	}
}

func TestProductionReadinessAudit_UnresolvedContradictionsAllowed(t *testing.T) {
	// Unresolved contradictions are visible to operators but do not block readiness.
	in := fullPassInput()
	in.UnresolvedContradictions = 99
	in.ContradictionScanOK = true
	report := RunProductionReadinessAudit("rpt-6", in)
	if !report.Ready {
		t.Error("unresolved contradictions (visible to operators) should not block readiness")
	}
}

func TestProductionReadinessAudit_NoActiveWorkers(t *testing.T) {
	in := fullPassInput()
	in.ActiveWorkerCount = 0
	report := RunProductionReadinessAudit("rpt-7", in)
	if report.Ready {
		t.Fatal("should not be ready with no active workers")
	}
}

func TestProductionReadinessAudit_QuarantinedWorkers(t *testing.T) {
	in := fullPassInput()
	in.QuarantinedWorkers = 1
	report := RunProductionReadinessAudit("rpt-8", in)
	if report.Ready {
		t.Fatal("should not be ready with quarantined workers")
	}
}

func TestProductionReadinessAudit_CredentialLeak(t *testing.T) {
	in := fullPassInput()
	in.NoCredentialLeak = false
	report := RunProductionReadinessAudit("rpt-9", in)
	if report.Ready {
		t.Fatal("should not be ready when credential leak detected")
	}
}

func TestProductionReadinessAudit_BadRBAC(t *testing.T) {
	in := fullPassInput()
	in.RBACRulesValid = false
	report := RunProductionReadinessAudit("rpt-10", in)
	if report.Ready {
		t.Fatal("should not be ready with invalid RBAC rules")
	}
}

func TestProductionReadinessAudit_BadBackup(t *testing.T) {
	in := fullPassInput()
	in.LatestBackupManifestValid = false
	report := RunProductionReadinessAudit("rpt-11", in)
	if report.Ready {
		t.Fatal("should not be ready with invalid backup manifest")
	}
}

func TestProductionReadinessAudit_HashIntegrity(t *testing.T) {
	report := RunProductionReadinessAudit("rpt-hash", fullPassInput())
	ok, reason := VerifyProductionReadinessReport(report)
	if !ok {
		t.Fatalf("report hash should be valid: %s", reason)
	}
}

func TestProductionReadinessAudit_TamperedHashDetected(t *testing.T) {
	report := RunProductionReadinessAudit("rpt-tamper", fullPassInput())
	report.GatesPassed = 0 // tamper
	ok, _ := VerifyProductionReadinessReport(report)
	if ok {
		t.Fatal("tampered report should fail hash verification")
	}
}

func TestProductionReadinessAudit_AssessedAtExcluded(t *testing.T) {
	report := RunProductionReadinessAudit("rpt-ts", fullPassInput())
	hash1 := report.ReportHash
	report.AssessedAt = "3000-01-01T00:00:00Z"
	hash2 := computeReportHash(report)
	if hash1 != hash2 {
		t.Error("ReportHash must not change when only AssessedAt changes (content-identity)")
	}
}

func TestProductionReadinessAudit_HasLimitations(t *testing.T) {
	report := RunProductionReadinessAudit("rpt-lim", fullPassInput())
	if len(report.Limitations) == 0 {
		t.Error("production readiness report must document its limitations")
	}
}

func TestProductionReadinessAudit_GateTotalIsFixed(t *testing.T) {
	report := RunProductionReadinessAudit("rpt-total", fullPassInput())
	if report.GatesTotal != 10 {
		t.Errorf("expected 10 gates total, got %d", report.GatesTotal)
	}
}
