package opcert

import (
	"testing"
)

func fullPassInput() OperationalCertInput {
	return OperationalCertInput{
		TenantID:                     "tenant-a",
		ExecutionID:                  "exec-1",
		ReplayContinuityVerified:     true,
		AuditChainIntact:             true,
		AuditEntryCount:              500,
		GovernanceDecisionsPreserved: true,
		GovernanceDecisionCount:      25,
		BackupManifestValid:          true,
		BackupPayloadValid:           true,
		ContradictionScanComplete:    true,
		UnresolvedContradictions:     3,
		StoreRecoverable:             true,
		RevocationsPersisted:         true,
		RevocationCount:              7,
		ExportHashStable:             true,
		StagingHealthEndpointOK:      true,
	}
}

func TestOperationalCertification_AllGatesPass(t *testing.T) {
	cert := RunOperationalCertification("oc-1", fullPassInput())
	if !cert.Certified {
		for _, g := range cert.Gates {
			if !g.Passed {
				t.Errorf("gate %s failed: %s", g.Name, g.Detail)
			}
		}
		t.Fatalf("expected certified=true (%d/%d passed)", cert.GatesPassed, cert.GatesTotal)
	}
}

func TestOperationalCertification_HashIntegrity(t *testing.T) {
	cert := RunOperationalCertification("oc-hash", fullPassInput())
	ok, reason := VerifyOperationalCertification(cert)
	if !ok {
		t.Fatalf("cert hash invalid: %s", reason)
	}
}

func TestOperationalCertification_TamperedHashDetected(t *testing.T) {
	cert := RunOperationalCertification("oc-tamper", fullPassInput())
	cert.GatesPassed = 0 // tamper
	ok, _ := VerifyOperationalCertification(cert)
	if ok {
		t.Fatal("tampered cert should fail verification")
	}
}

func TestOperationalCertification_CertifiedAtExcluded(t *testing.T) {
	cert := RunOperationalCertification("oc-ts", fullPassInput())
	hash1 := cert.CertHash
	cert.CertifiedAt = "3000-01-01T00:00:00Z"
	hash2 := computeOpCertHash(cert)
	if hash1 != hash2 {
		t.Error("CertHash must not change when only CertifiedAt changes (content-identity)")
	}
}

func TestOperationalCertification_FailsReplayContinuity(t *testing.T) {
	in := fullPassInput()
	in.ReplayContinuityVerified = false
	cert := RunOperationalCertification("oc-replay", in)
	if cert.Certified {
		t.Fatal("should not certify when replay continuity fails")
	}
}

func TestOperationalCertification_FailsAuditDurability(t *testing.T) {
	in := fullPassInput()
	in.AuditChainIntact = false
	cert := RunOperationalCertification("oc-audit", in)
	if cert.Certified {
		t.Fatal("should not certify when audit chain fails")
	}
}

func TestOperationalCertification_FailsGovernanceContinuity(t *testing.T) {
	in := fullPassInput()
	in.GovernanceDecisionsPreserved = false
	cert := RunOperationalCertification("oc-gov", in)
	if cert.Certified {
		t.Fatal("should not certify when governance decisions not preserved")
	}
}

func TestOperationalCertification_FailsBackupManifest(t *testing.T) {
	in := fullPassInput()
	in.BackupManifestValid = false
	cert := RunOperationalCertification("oc-backup-manifest", in)
	if cert.Certified {
		t.Fatal("should not certify when backup manifest is invalid")
	}
}

func TestOperationalCertification_FailsBackupPayload(t *testing.T) {
	in := fullPassInput()
	in.BackupPayloadValid = false
	cert := RunOperationalCertification("oc-backup-payload", in)
	if cert.Certified {
		t.Fatal("should not certify when backup payload is invalid")
	}
}

func TestOperationalCertification_FailsContradictionScan(t *testing.T) {
	in := fullPassInput()
	in.ContradictionScanComplete = false
	cert := RunOperationalCertification("oc-contradiction", in)
	if cert.Certified {
		t.Fatal("should not certify when contradiction scan incomplete")
	}
}

func TestOperationalCertification_UnresolvedContradictionsAllowed(t *testing.T) {
	// Unresolved contradictions are visible to operators — they do not block certification.
	in := fullPassInput()
	in.UnresolvedContradictions = 999
	cert := RunOperationalCertification("oc-unresolved", in)
	if !cert.Certified {
		t.Error("unresolved contradictions (visible to operators) should not block operational certification")
	}
}

func TestOperationalCertification_FailsRestartSurvivability(t *testing.T) {
	in := fullPassInput()
	in.StoreRecoverable = false
	cert := RunOperationalCertification("oc-restart", in)
	if cert.Certified {
		t.Fatal("should not certify when store is not recoverable")
	}
}

func TestOperationalCertification_FailsRevocationIntegrity(t *testing.T) {
	in := fullPassInput()
	in.RevocationsPersisted = false
	cert := RunOperationalCertification("oc-revoke", in)
	if cert.Certified {
		t.Fatal("should not certify when revocations are not persisted")
	}
}

func TestOperationalCertification_FailsDeterministicExports(t *testing.T) {
	in := fullPassInput()
	in.ExportHashStable = false
	cert := RunOperationalCertification("oc-exports", in)
	if cert.Certified {
		t.Fatal("should not certify when exports are not deterministic")
	}
}

func TestOperationalCertification_FailsStagingDeployment(t *testing.T) {
	in := fullPassInput()
	in.StagingHealthEndpointOK = false
	cert := RunOperationalCertification("oc-staging", in)
	if cert.Certified {
		t.Fatal("should not certify when staging health endpoint is not OK")
	}
}

func TestOperationalCertification_HasClaim(t *testing.T) {
	cert := RunOperationalCertification("oc-claim", fullPassInput())
	if cert.OperationalClaim == "" {
		t.Error("operational certification must include an operational claim")
	}
}

func TestOperationalCertification_HasLimitations(t *testing.T) {
	cert := RunOperationalCertification("oc-lim", fullPassInput())
	if len(cert.Limitations) == 0 {
		t.Error("operational certification must document its limitations")
	}
}

func TestOperationalCertification_GateTotalIsNine(t *testing.T) {
	cert := RunOperationalCertification("oc-total", fullPassInput())
	if cert.GatesTotal != 9 {
		t.Errorf("expected 9 gates total, got %d", cert.GatesTotal)
	}
}

func TestOperationalCertification_SchemaVersion(t *testing.T) {
	cert := RunOperationalCertification("oc-schema", fullPassInput())
	if cert.SchemaVersion != OperationalCertSchemaVersion {
		t.Errorf("expected schema version %s, got %s", OperationalCertSchemaVersion, cert.SchemaVersion)
	}
}
