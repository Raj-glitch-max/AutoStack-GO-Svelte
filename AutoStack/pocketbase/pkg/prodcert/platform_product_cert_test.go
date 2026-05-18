package prodcert

import (
	"testing"
)

func fullProductPassInput() PlatformProductCertInput {
	return PlatformProductCertInput{
		OperationalCertPassed:  true,
		OperationalCertGates:   9,
		GovernanceModelSound:   true,
		OperatorTimelineReady:  true,
		ForensicSurfaceReady:   true,
		GovernanceOpsReady:     true,
		PlatformDashboardReady: true,
		DrillModeReady:         true,
		InstallerReady:         true,
		DemoEnvironmentReady:   true,
		DocumentationComplete:  true,
		DocumentCount:          6,
		ReleaseArtifactsReady:  true,
	}
}

func TestProductCert_AllGatesPass(t *testing.T) {
	cert := RunPlatformProductCertification("pc-1", fullProductPassInput())
	if !cert.Certified {
		for _, g := range cert.Gates {
			if !g.Passed {
				t.Errorf("gate %s failed: %s", g.Name, g.Detail)
			}
		}
		t.Fatalf("expected certified=true (%d/%d passed)", cert.GatesPassed, cert.GatesTotal)
	}
}

func TestProductCert_GateTotalIsEleven(t *testing.T) {
	cert := RunPlatformProductCertification("pc-total", fullProductPassInput())
	if cert.GatesTotal != 11 {
		t.Errorf("expected 11 gates total, got %d", cert.GatesTotal)
	}
}

func TestProductCert_HashIntegrity(t *testing.T) {
	cert := RunPlatformProductCertification("pc-hash", fullProductPassInput())
	ok, reason := VerifyPlatformProductCertification(cert)
	if !ok {
		t.Fatalf("cert hash invalid: %s", reason)
	}
}

func TestProductCert_TamperedHashDetected(t *testing.T) {
	cert := RunPlatformProductCertification("pc-tamper", fullProductPassInput())
	cert.GatesPassed = 0
	ok, _ := VerifyPlatformProductCertification(cert)
	if ok {
		t.Fatal("tampered cert should fail verification")
	}
}

func TestProductCert_CertifiedAtExcluded(t *testing.T) {
	cert := RunPlatformProductCertification("pc-ts", fullProductPassInput())
	hash1 := cert.CertHash
	cert.CertifiedAt = "3000-01-01T00:00:00Z"
	hash2 := computeProductCertHash(cert)
	if hash1 != hash2 {
		t.Error("CertHash must not change when only CertifiedAt changes (content-identity)")
	}
}

func TestProductCert_FailsRuntimeCert(t *testing.T) {
	in := fullProductPassInput()
	in.OperationalCertPassed = false
	cert := RunPlatformProductCertification("pc-runtime", in)
	if cert.Certified {
		t.Fatal("should not certify when operational cert failed")
	}
}

func TestProductCert_FailsWrongGateCount(t *testing.T) {
	in := fullProductPassInput()
	in.OperationalCertGates = 8
	cert := RunPlatformProductCertification("pc-gates", in)
	if cert.Certified {
		t.Fatal("should not certify when operational cert gate count is wrong")
	}
}

func TestProductCert_FailsGovernanceModel(t *testing.T) {
	in := fullProductPassInput()
	in.GovernanceModelSound = false
	cert := RunPlatformProductCertification("pc-gov", in)
	if cert.Certified {
		t.Fatal("should not certify when governance model is unsound")
	}
}

func TestProductCert_FailsDrillMode(t *testing.T) {
	in := fullProductPassInput()
	in.DrillModeReady = false
	cert := RunPlatformProductCertification("pc-drill", in)
	if cert.Certified {
		t.Fatal("should not certify when drill mode is not ready")
	}
}

func TestProductCert_FailsDocumentationCount(t *testing.T) {
	in := fullProductPassInput()
	in.DocumentCount = 5
	cert := RunPlatformProductCertification("pc-docs", in)
	if cert.Certified {
		t.Fatal("should not certify when fewer than 6 docs are present")
	}
}

func TestProductCert_HasProductClaim(t *testing.T) {
	cert := RunPlatformProductCertification("pc-claim", fullProductPassInput())
	if cert.ProductClaim == "" {
		t.Error("product certification must include a product claim")
	}
}

func TestProductCert_HasLimitations(t *testing.T) {
	cert := RunPlatformProductCertification("pc-lim", fullProductPassInput())
	if len(cert.Limitations) == 0 {
		t.Error("product certification must document its limitations")
	}
}

func TestProductCert_SchemaVersion(t *testing.T) {
	cert := RunPlatformProductCertification("pc-schema", fullProductPassInput())
	if cert.SchemaVersion != PlatformProductCertSchemaVersion {
		t.Errorf("expected schema version %s, got %s", PlatformProductCertSchemaVersion, cert.SchemaVersion)
	}
}
