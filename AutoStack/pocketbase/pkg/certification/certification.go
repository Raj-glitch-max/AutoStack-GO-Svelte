// Package certification implements final platform readiness certification.
// A platform certification combines audit, safety, replay, and operational state
// into a single tamper-evident claim. It does not trigger any action — it is
// evidence only.
package certification

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/janlauber/one-click/pkg/audit"
	"github.com/janlauber/one-click/pkg/observability"
)

const CertificationSchemaVersion = "6.0.0"

// ReadinessGate is a single named check in a platform readiness audit.
type ReadinessGate struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Reason  string `json:"reason,omitempty"`
}

// PlatformCertificationReport is a tamper-evident, content-identity-hashed
// certification of a platform's readiness for production operation.
// CertifiedAt is excluded from CertificationHash (content-identity).
type PlatformCertificationReport struct {
	CertificationID   string          `json:"certification_id"`
	ExecutionID       string          `json:"execution_id"`
	TenantID          string          `json:"tenant_id"`
	SchemaVersion     string          `json:"schema_version"`
	Gates             []ReadinessGate `json:"gates"`
	GatesPassed       int             `json:"gates_passed"`
	GatesTotal        int             `json:"gates_total"`
	AuditPassed       bool            `json:"audit_passed"`
	SafetyAssessed    bool            `json:"safety_assessed"`
	ReplayCertified   bool            `json:"replay_certified"`
	HealthStatus      string          `json:"health_status"`
	PlatformCertified bool            `json:"platform_certified"` // true only when ALL gates pass
	Limitations       []string        `json:"limitations"`
	CertifiedAt       string          `json:"certified_at"` // excluded from hash
	CertificationHash string          `json:"certification_hash"`
}

// CertifiedPlatformSnapshot bundles the certification report with supporting evidence.
// SnapshotAt is excluded from hash (content-identity).
type CertifiedPlatformSnapshot struct {
	CertificationID   string                      `json:"certification_id"`
	Report            PlatformCertificationReport `json:"report"`
	AuditResult       audit.AuditResult           `json:"audit_result"`
	SafetyAssessment  audit.OperatorSafetyAssessment `json:"safety_assessment"`
	HealthAssessment  observability.HealthAssessment `json:"health_assessment"`
	SnapshotAt        string                      `json:"snapshot_at"` // excluded from hash
	SnapshotHash      string                      `json:"snapshot_hash"`
}

// PlatformReadinessInput carries all signals needed for a readiness audit.
type PlatformReadinessInput struct {
	ExecutionID          string
	TenantID             string
	AuditResult          audit.AuditResult
	SafetyAssessment     audit.OperatorSafetyAssessment
	ReplayCertification  *audit.ReplayCertificationReport
	HealthAssessment     observability.HealthAssessment
}

// RunPlatformReadinessAudit evaluates all readiness gates and produces a
// PlatformCertificationReport. It does not halt on gate failure — all gates
// are evaluated and all failures are recorded.
func RunPlatformReadinessAudit(certificationID string, input PlatformReadinessInput) PlatformCertificationReport {
	var gates []ReadinessGate

	// Gate 1: Production truth audit must pass.
	auditGate := ReadinessGate{Name: "production-truth-audit", Passed: input.AuditResult.Passed}
	if !auditGate.Passed {
		auditGate.Reason = fmt.Sprintf("%d audit finding(s) require attention", len(input.AuditResult.Findings))
	}
	gates = append(gates, auditGate)

	// Gate 2: Operator safety must be verified.
	safetyOK, _ := audit.VerifySafetyHash(input.SafetyAssessment)
	safetyGate := ReadinessGate{Name: "operator-safety-hash", Passed: safetyOK && input.SafetyAssessment.Safe}
	if !safetyGate.Passed {
		if !safetyOK {
			safetyGate.Reason = "safety assessment hash is invalid"
		} else {
			safetyGate.Reason = "safety assessment reports unsafe conditions"
		}
	}
	gates = append(gates, safetyGate)

	// Gate 3: Replay must be certified.
	replayGate := ReadinessGate{Name: "replay-certification"}
	if input.ReplayCertification == nil {
		replayGate.Passed = false
		replayGate.Reason = "no replay certification found"
	} else {
		ok, _ := audit.VerifyCertificationHash(*input.ReplayCertification)
		replayGate.Passed = ok && input.ReplayCertification.Certified
		if !replayGate.Passed {
			if !ok {
				replayGate.Reason = "replay certification hash invalid"
			} else {
				replayGate.Reason = "replay certification not achieved (not all checks passed)"
			}
		}
	}
	gates = append(gates, replayGate)

	// Gate 4: Platform health must not be critical.
	healthGate := ReadinessGate{Name: "platform-health"}
	healthGate.Passed = input.HealthAssessment.Status != observability.HealthStatusCritical
	if !healthGate.Passed {
		healthGate.Reason = "platform health is critical"
	}
	gates = append(gates, healthGate)

	// Gate 5: Audit hash integrity.
	auditHashGate := ReadinessGate{Name: "audit-hash-integrity"}
	auditHashOK, _ := audit.VerifyAuditHash(input.AuditResult)
	auditHashGate.Passed = auditHashOK
	if !auditHashGate.Passed {
		auditHashGate.Reason = "audit result hash is invalid"
	}
	gates = append(gates, auditHashGate)

	// Count.
	passed := 0
	for _, g := range gates {
		if g.Passed {
			passed++
		}
	}

	allPassed := passed == len(gates)

	r := PlatformCertificationReport{
		CertificationID:   certificationID,
		ExecutionID:       input.ExecutionID,
		TenantID:          input.TenantID,
		SchemaVersion:     CertificationSchemaVersion,
		Gates:             gates,
		GatesPassed:       passed,
		GatesTotal:        len(gates),
		AuditPassed:       input.AuditResult.Passed,
		SafetyAssessed:    input.SafetyAssessment.Safe,
		ReplayCertified:   input.ReplayCertification != nil && input.ReplayCertification.Certified,
		HealthStatus:      string(input.HealthAssessment.Status),
		PlatformCertified: allPassed,
		Limitations:       platformLimitations(),
		CertifiedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	r.CertificationHash = computePlatformCertHash(r)
	return r
}

// VerifyPlatformCertification recomputes and compares the certification hash.
func VerifyPlatformCertification(r PlatformCertificationReport) (bool, string) {
	stored := r.CertificationHash
	r.CertificationHash = ""
	expected := computePlatformCertHash(r)
	if stored != expected {
		return false, fmt.Sprintf("certification hash mismatch: stored=%s computed=%s", stored, expected)
	}
	return true, ""
}

// BuildCertifiedPlatformSnapshot bundles a certification report with supporting evidence.
func BuildCertifiedPlatformSnapshot(
	report PlatformCertificationReport,
	auditResult audit.AuditResult,
	safety audit.OperatorSafetyAssessment,
	health observability.HealthAssessment,
) CertifiedPlatformSnapshot {
	snap := CertifiedPlatformSnapshot{
		CertificationID:  report.CertificationID,
		Report:           report,
		AuditResult:      auditResult,
		SafetyAssessment: safety,
		HealthAssessment: health,
		SnapshotAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	snap.SnapshotHash = computeSnapshotHash(snap)
	return snap
}

// VerifyCertifiedPlatformSnapshot recomputes and verifies the snapshot hash.
func VerifyCertifiedPlatformSnapshot(snap CertifiedPlatformSnapshot) (bool, string) {
	stored := snap.SnapshotHash
	snap.SnapshotHash = ""
	expected := computeSnapshotHash(snap)
	if stored != expected {
		return false, fmt.Sprintf("snapshot hash mismatch: stored=%s computed=%s", stored, expected)
	}
	return true, ""
}

func computePlatformCertHash(r PlatformCertificationReport) string {
	h := sha256.New()
	fmt.Fprintf(h, "cert:%s|exec:%s|tenant:%s|schema:%s|passed:%d|total:%d|certified:%v",
		r.CertificationID, r.ExecutionID, r.TenantID, r.SchemaVersion,
		r.GatesPassed, r.GatesTotal, r.PlatformCertified)
	for _, g := range r.Gates {
		fmt.Fprintf(h, "|gate:%s/%v", g.Name, g.Passed)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func computeSnapshotHash(snap CertifiedPlatformSnapshot) string {
	h := sha256.New()
	fmt.Fprintf(h, "snap:%s|cert:%s|audit:%s|safety:%v|health:%s",
		snap.CertificationID,
		snap.Report.CertificationHash,
		snap.AuditResult.AuditHash,
		snap.SafetyAssessment.Safe,
		snap.HealthAssessment.Status,
	)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func platformLimitations() []string {
	return []string{
		"Platform certification is evidence only — it does not trigger any deployment action.",
		"Certification is point-in-time. Runtime state may change after this report is produced.",
		"A certified platform with health=degraded may still be partially impaired.",
		"Replay certification covers the execution's recorded event set — future divergences are not predicted.",
	}
}
