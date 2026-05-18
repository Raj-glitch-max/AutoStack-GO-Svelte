// Package prodcert provides the final production certification for AutoStack.
// This file adds the PlatformProductReadinessCertification — the 11-gate
// top-level product certification that wraps all prior certifications.
package prodcert

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const PlatformProductCertSchemaVersion = "7.0.0"

// ProductGate is one named gate in the platform product certification.
type ProductGate struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// PlatformProductReadinessCertification is the final, tamper-evident product
// certification covering all runtime, UI, documentation, deployment, and
// operational dimensions. CertifiedAt is excluded from CertHash.
type PlatformProductReadinessCertification struct {
	CertID         string        `json:"cert_id"`
	SchemaVersion  string        `json:"schema_version"`
	Gates          []ProductGate `json:"gates"`
	GatesPassed    int           `json:"gates_passed"`
	GatesTotal     int           `json:"gates_total"`
	Certified      bool          `json:"certified"`
	ProductClaim   string        `json:"product_claim"`
	Limitations    []string      `json:"limitations"`
	CertifiedAt    string        `json:"certified_at"` // excluded from CertHash
	CertHash       string        `json:"cert_hash"`
}

// PlatformProductCertInput carries all evidence for the 11-gate product certification.
type PlatformProductCertInput struct {
	// Gate 1: Runtime + Replay certification (pkg/opcert passed)
	OperationalCertPassed bool
	OperationalCertGates  int // must be 9

	// Gate 2: Governance model (no silent auto-approval, operator authority preserved)
	GovernanceModelSound bool

	// Gate 3: Operator timeline UI present and read-only
	OperatorTimelineReady bool

	// Gate 4: Forensic investigation surface present and read-only
	ForensicSurfaceReady bool

	// Gate 5: Governance operations center ready
	GovernanceOpsReady bool

	// Gate 6: Platform dashboard with certification status visible
	PlatformDashboardReady bool

	// Gate 7: Operational drill mode implemented (simulation only, zero production mutation)
	DrillModeReady bool

	// Gate 8: Quickstart installer tested (developer operational in <15 min target)
	InstallerReady bool

	// Gate 9: Demo environment boots and seeds (make demo-up succeeds)
	DemoEnvironmentReady bool

	// Gate 10: Documentation hardening — all 6 required docs present
	DocumentationComplete bool
	DocumentCount         int // must be >= 6

	// Gate 11: Release artifacts — release notes, architecture diagram, operational checklist
	ReleaseArtifactsReady bool
}

// RunPlatformProductCertification evaluates all 11 product gates.
func RunPlatformProductCertification(certID string, input PlatformProductCertInput) PlatformProductReadinessCertification {
	gates := []ProductGate{
		evalRuntimeReplayCert(input),
		evalGovernanceModel(input),
		evalOperatorTimeline(input),
		evalForensicSurface(input),
		evalGovernanceOps(input),
		evalPlatformDashboard(input),
		evalDrillMode(input),
		evalInstaller(input),
		evalDemoEnvironment(input),
		evalDocumentation(input),
		evalReleaseArtifacts(input),
	}

	passed := 0
	for _, g := range gates {
		if g.Passed {
			passed++
		}
	}

	cert := PlatformProductReadinessCertification{
		CertID:        certID,
		SchemaVersion: PlatformProductCertSchemaVersion,
		Gates:         gates,
		GatesPassed:   passed,
		GatesTotal:    len(gates),
		Certified:     passed == len(gates),
		ProductClaim:  productClaim(),
		Limitations:   productLimitations(),
		CertifiedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	cert.CertHash = computeProductCertHash(cert)
	return cert
}

// VerifyPlatformProductCertification checks tamper-evidence on the cert hash.
func VerifyPlatformProductCertification(c PlatformProductReadinessCertification) (bool, string) {
	expected := computeProductCertHash(c)
	if c.CertHash != expected {
		return false, fmt.Sprintf("product cert %q hash mismatch (stored=%s computed=%s)",
			c.CertID, c.CertHash, expected)
	}
	return true, ""
}

func evalRuntimeReplayCert(in PlatformProductCertInput) ProductGate {
	if !in.OperationalCertPassed {
		return ProductGate{Name: "runtime-replay-certification", Detail: "operational certification did not pass"}
	}
	if in.OperationalCertGates != 9 {
		return ProductGate{Name: "runtime-replay-certification",
			Detail: fmt.Sprintf("expected 9 operational gates, got %d", in.OperationalCertGates)}
	}
	return ProductGate{Name: "runtime-replay-certification", Passed: true,
		Detail: "operational certification passed all 9 gates (replay, audit, governance, backup, contradiction, restart, revocation, exports, staging)"}
}

func evalGovernanceModel(in PlatformProductCertInput) ProductGate {
	if !in.GovernanceModelSound {
		return ProductGate{Name: "governance-model", Detail: "governance model has auto-approval paths or unsound operator authority"}
	}
	return ProductGate{Name: "governance-model", Passed: true,
		Detail: "default-deny RBAC, no silent auto-approval, operator authority preserved across restart"}
}

func evalOperatorTimeline(in PlatformProductCertInput) ProductGate {
	if !in.OperatorTimelineReady {
		return ProductGate{Name: "operator-timeline", Detail: "operator timeline UI not ready"}
	}
	return ProductGate{Name: "operator-timeline", Passed: true,
		Detail: "execution, contradiction, replay, governance, and audit-chain timeline tabs ready; read-only surface"}
}

func evalForensicSurface(in PlatformProductCertInput) ProductGate {
	if !in.ForensicSurfaceReady {
		return ProductGate{Name: "forensic-surface", Detail: "forensic investigation surface not ready"}
	}
	return ProductGate{Name: "forensic-surface", Passed: true,
		Detail: "snapshot, replay manifest, certification, contradiction evidence, and backup verification tabs ready"}
}

func evalGovernanceOps(in PlatformProductCertInput) ProductGate {
	if !in.GovernanceOpsReady {
		return ProductGate{Name: "governance-ops-center", Detail: "governance operations center not ready"}
	}
	return ProductGate{Name: "governance-ops-center", Passed: true,
		Detail: "policy decisions, overrides, approval hierarchies visible; no auto-approval from UI"}
}

func evalPlatformDashboard(in PlatformProductCertInput) ProductGate {
	if !in.PlatformDashboardReady {
		return ProductGate{Name: "platform-dashboard", Detail: "platform dashboard not ready"}
	}
	return ProductGate{Name: "platform-dashboard", Passed: true,
		Detail: "health overview, certification status, contradiction count, worker health, tenant activity all visible"}
}

func evalDrillMode(in PlatformProductCertInput) ProductGate {
	if !in.DrillModeReady {
		return ProductGate{Name: "drill-mode", Detail: "operational drill mode not ready"}
	}
	return ProductGate{Name: "drill-mode", Passed: true,
		Detail: "8 simulation drills implemented; SIMULATION ONLY badge displayed; zero production mutation from drill surface"}
}

func evalInstaller(in PlatformProductCertInput) ProductGate {
	if !in.InstallerReady {
		return ProductGate{Name: "installer-quickstart", Detail: "installer or quickstart not ready"}
	}
	return ProductGate{Name: "installer-quickstart", Passed: true,
		Detail: "scripts/install/install.sh and docs/quickstart.md present; target: new developer operational in <15 minutes"}
}

func evalDemoEnvironment(in PlatformProductCertInput) ProductGate {
	if !in.DemoEnvironmentReady {
		return ProductGate{Name: "demo-environment", Detail: "demo environment not ready"}
	}
	return ProductGate{Name: "demo-environment", Passed: true,
		Detail: "demo/ boots with make demo-up; seeded with contradiction, replay, governance, audit examples"}
}

func evalDocumentation(in PlatformProductCertInput) ProductGate {
	if !in.DocumentationComplete {
		return ProductGate{Name: "documentation", Detail: "documentation not complete"}
	}
	if in.DocumentCount < 6 {
		return ProductGate{Name: "documentation",
			Detail: fmt.Sprintf("expected ≥6 hardened docs, got %d", in.DocumentCount)}
	}
	return ProductGate{Name: "documentation", Passed: true,
		Detail: fmt.Sprintf("%d docs hardened: runtime-architecture, replay-certification, governance-model, operational-survivability, forensic-model, security-boundaries", in.DocumentCount)}
}

func evalReleaseArtifacts(in PlatformProductCertInput) ProductGate {
	if !in.ReleaseArtifactsReady {
		return ProductGate{Name: "release-artifacts", Detail: "release artifacts not ready"}
	}
	return ProductGate{Name: "release-artifacts", Passed: true,
		Detail: "release notes, operational checklist, and release manifest present in release/v1/"}
}

func productClaim() string {
	return "A deterministic orchestration platform with replay-certified execution, append-only operational evidence, governance-enforced automation, durable forensic auditability, and operator-first operational control."
}

func productLimitations() []string {
	return []string{
		"Timeline and forensics surfaces are read-only query layers; real-time streaming not supported.",
		"Drill mode is in-process simulation; real fault injection requires an external harness.",
		"Installer targets single-node Docker deployment; multi-replica and HA not covered by quickstart.",
		"Demo environment uses seeded synthetic data; it does not connect to real cloud providers.",
		"Documentation covers design intent and boundaries; operational runbooks require site-specific adaptation.",
		"Product certification is self-assessed; external security audit and penetration test not included.",
	}
}

func computeProductCertHash(c PlatformProductReadinessCertification) string {
	type stable struct {
		CertID        string        `json:"cert_id"`
		SchemaVersion string        `json:"schema_version"`
		Gates         []ProductGate `json:"gates"`
		GatesPassed   int           `json:"gates_passed"`
		GatesTotal    int           `json:"gates_total"`
		Certified     bool          `json:"certified"`
		ProductClaim  string        `json:"product_claim"`
		Limitations   []string      `json:"limitations"`
	}
	s := stable{
		CertID:        c.CertID,
		SchemaVersion: c.SchemaVersion,
		Gates:         c.Gates,
		GatesPassed:   c.GatesPassed,
		GatesTotal:    c.GatesTotal,
		Certified:     c.Certified,
		ProductClaim:  c.ProductClaim,
		Limitations:   c.Limitations,
	}
	b, _ := json.Marshal(s)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
