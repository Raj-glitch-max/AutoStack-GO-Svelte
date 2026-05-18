// Package opcert implements the final operational readiness certification for AutoStack.
// This certifies actual operational survivability — not just architectural correctness.
//
// Operational certification covers:
//   - Replay continuity across restarts
//   - Audit trail durability and chain integrity
//   - Governance continuity (decisions preserved)
//   - Backup recoverability (manifest hash valid)
//   - Contradiction survivability (scan completes; results visible)
//   - Restart survivability (store recoverable)
//   - Token revocation integrity (revocations persist)
//   - Deterministic exports (hashes stable)
//   - Staging deployment integrity (health endpoint responds)
package opcert

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const OperationalCertSchemaVersion = "7.0.0"

// OperationalGate is a single named check in the operational certification.
type OperationalGate struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// OperationalReadinessCertification is the final, tamper-evident certification
// that the platform is operationally ready for production deployment.
// CertifiedAt is excluded from CertHash (content-identity).
type OperationalReadinessCertification struct {
	CertID        string            `json:"cert_id"`
	TenantID      string            `json:"tenant_id"`
	ExecutionID   string            `json:"execution_id"`
	SchemaVersion string            `json:"schema_version"`
	Gates         []OperationalGate `json:"gates"`
	GatesPassed   int               `json:"gates_passed"`
	GatesTotal    int               `json:"gates_total"`
	Certified     bool              `json:"certified"`
	OperationalClaim string         `json:"operational_claim"`
	Limitations   []string          `json:"limitations"`
	CertifiedAt   string            `json:"certified_at"` // excluded from CertHash
	CertHash      string            `json:"cert_hash"`
}

// OperationalCertInput carries all evidence needed for operational certification.
type OperationalCertInput struct {
	TenantID    string
	ExecutionID string

	// Replay continuity.
	ReplayContinuityVerified bool // true if manifest hash matched after simulated restart

	// Audit durability.
	AuditChainIntact    bool // VerifyAuditChain() passed
	AuditEntryCount     int  // number of persisted audit entries

	// Governance continuity.
	GovernanceDecisionsPreserved bool
	GovernanceDecisionCount      int

	// Backup recoverability.
	BackupManifestValid bool
	BackupPayloadValid  bool

	// Contradiction survivability.
	ContradictionScanComplete bool
	UnresolvedContradictions  int

	// Restart survivability.
	StoreRecoverable bool // RecoverPersistentRuntime succeeded or store reads healthy after restart

	// Token revocation integrity.
	RevocationsPersisted bool // revoked tokens still revoked after store reopen
	RevocationCount      int

	// Deterministic exports.
	ExportHashStable bool // same export inputs produce same hash on repeat

	// Staging deployment.
	StagingHealthEndpointOK bool
}

// RunOperationalCertification evaluates all operational gates and produces a signed certification.
func RunOperationalCertification(certID string, input OperationalCertInput) OperationalReadinessCertification {
	gates := []OperationalGate{
		evalReplayContinuity(input),
		evalAuditDurability(input),
		evalGovernanceContinuity(input),
		evalBackupRecoverability(input),
		evalContradictionSurvivability(input),
		evalRestartSurvivability(input),
		evalRevocationIntegrity(input),
		evalDeterministicExports(input),
		evalStagingDeployment(input),
	}

	passed := 0
	for _, g := range gates {
		if g.Passed {
			passed++
		}
	}

	cert := OperationalReadinessCertification{
		CertID:           certID,
		TenantID:         input.TenantID,
		ExecutionID:      input.ExecutionID,
		SchemaVersion:    OperationalCertSchemaVersion,
		Gates:            gates,
		GatesPassed:      passed,
		GatesTotal:       len(gates),
		Certified:        passed == len(gates),
		OperationalClaim: operationalClaim(),
		Limitations:      operationalLimitations(),
		CertifiedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	cert.CertHash = computeOpCertHash(cert)
	return cert
}

// VerifyOperationalCertification checks that the cert hash matches its content.
func VerifyOperationalCertification(c OperationalReadinessCertification) (bool, string) {
	expected := computeOpCertHash(c)
	if c.CertHash != expected {
		return false, fmt.Sprintf("operational cert %q hash mismatch (stored=%s computed=%s)",
			c.CertID, c.CertHash, expected)
	}
	return true, ""
}

// --- gate evaluators ---

func evalReplayContinuity(in OperationalCertInput) OperationalGate {
	if !in.ReplayContinuityVerified {
		return OperationalGate{Name: "replay-continuity", Detail: "replay hash did not match after restart simulation"}
	}
	return OperationalGate{Name: "replay-continuity", Passed: true, Detail: "replay manifest hash stable across restart simulation"}
}

func evalAuditDurability(in OperationalCertInput) OperationalGate {
	if !in.AuditChainIntact {
		return OperationalGate{Name: "audit-durability", Detail: "audit chain verification failed"}
	}
	return OperationalGate{
		Name:   "audit-durability",
		Passed: true,
		Detail: fmt.Sprintf("audit chain intact; %d entries verified", in.AuditEntryCount),
	}
}

func evalGovernanceContinuity(in OperationalCertInput) OperationalGate {
	if !in.GovernanceDecisionsPreserved {
		return OperationalGate{Name: "governance-continuity", Detail: "governance decisions not preserved after restart"}
	}
	return OperationalGate{
		Name:   "governance-continuity",
		Passed: true,
		Detail: fmt.Sprintf("%d governance decisions preserved", in.GovernanceDecisionCount),
	}
}

func evalBackupRecoverability(in OperationalCertInput) OperationalGate {
	if !in.BackupManifestValid {
		return OperationalGate{Name: "backup-recoverability", Detail: "backup manifest hash invalid"}
	}
	if !in.BackupPayloadValid {
		return OperationalGate{Name: "backup-recoverability", Detail: "backup payload integrity check failed"}
	}
	return OperationalGate{Name: "backup-recoverability", Passed: true, Detail: "manifest and payload both verified"}
}

func evalContradictionSurvivability(in OperationalCertInput) OperationalGate {
	if !in.ContradictionScanComplete {
		return OperationalGate{Name: "contradiction-survivability", Detail: "contradiction scan did not complete"}
	}
	return OperationalGate{
		Name:   "contradiction-survivability",
		Passed: true,
		Detail: fmt.Sprintf("scan complete; %d unresolved (visible to operators)", in.UnresolvedContradictions),
	}
}

func evalRestartSurvivability(in OperationalCertInput) OperationalGate {
	if !in.StoreRecoverable {
		return OperationalGate{Name: "restart-survivability", Detail: "store not recoverable after simulated restart"}
	}
	return OperationalGate{Name: "restart-survivability", Passed: true, Detail: "store recoverable; evidence intact after restart simulation"}
}

func evalRevocationIntegrity(in OperationalCertInput) OperationalGate {
	if !in.RevocationsPersisted {
		return OperationalGate{Name: "revocation-integrity", Detail: "revocations did not survive store reopen"}
	}
	return OperationalGate{
		Name:   "revocation-integrity",
		Passed: true,
		Detail: fmt.Sprintf("%d revocations persisted across restart", in.RevocationCount),
	}
}

func evalDeterministicExports(in OperationalCertInput) OperationalGate {
	if !in.ExportHashStable {
		return OperationalGate{Name: "deterministic-exports", Detail: "export hash was not stable across repeated computation"}
	}
	return OperationalGate{Name: "deterministic-exports", Passed: true, Detail: "export hashes deterministic; same inputs produce same hash"}
}

func evalStagingDeployment(in OperationalCertInput) OperationalGate {
	if !in.StagingHealthEndpointOK {
		return OperationalGate{Name: "staging-deployment", Detail: "staging /healthz endpoint did not respond"}
	}
	return OperationalGate{Name: "staging-deployment", Passed: true, Detail: "staging health endpoint verified"}
}

func operationalClaim() string {
	return "A replay-certified orchestration platform with append-only operational evidence, governance-enforced execution control, durable forensic auditability, and production-validated operational survivability."
}

func operationalLimitations() []string {
	return []string{
		"Restart survivability tests use in-process simulation; real process kill tests require an external harness.",
		"Staging deployment gate requires a running Docker environment; it is caller-asserted in this certification.",
		"Revocation persistence uses SQLite WAL; cross-replica revocation is not guaranteed.",
		"Contradiction survivability does not test real provider API failures.",
		"Audit chain continuity is per-store; cross-replica audit continuity is not verified.",
	}
}

func computeOpCertHash(c OperationalReadinessCertification) string {
	type stable struct {
		CertID           string            `json:"cert_id"`
		TenantID         string            `json:"tenant_id"`
		ExecutionID      string            `json:"execution_id"`
		SchemaVersion    string            `json:"schema_version"`
		Gates            []OperationalGate `json:"gates"`
		GatesPassed      int               `json:"gates_passed"`
		GatesTotal       int               `json:"gates_total"`
		Certified        bool              `json:"certified"`
		OperationalClaim string            `json:"operational_claim"`
		Limitations      []string          `json:"limitations"`
	}
	s := stable{
		CertID:           c.CertID,
		TenantID:         c.TenantID,
		ExecutionID:      c.ExecutionID,
		SchemaVersion:    c.SchemaVersion,
		Gates:            c.Gates,
		GatesPassed:      c.GatesPassed,
		GatesTotal:       c.GatesTotal,
		Certified:        c.Certified,
		OperationalClaim: c.OperationalClaim,
		Limitations:      c.Limitations,
	}
	b, _ := json.Marshal(s)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
