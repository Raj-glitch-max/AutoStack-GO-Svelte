package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// ReadinessCheckName is a named gate in the production readiness audit.
type ReadinessCheckName string

const (
	CheckStorageHealth          ReadinessCheckName = "storage.health"
	CheckReplayIntegrity        ReadinessCheckName = "replay.integrity"
	CheckContradictionIntegrity ReadinessCheckName = "contradiction.integrity"
	CheckGovernanceIntegrity    ReadinessCheckName = "governance.integrity"
	CheckArchiveRetrievability  ReadinessCheckName = "archive.retrievability"
	CheckWorkerLiveness         ReadinessCheckName = "worker.liveness"
	CheckAPIHealth              ReadinessCheckName = "api.health"
	CheckSecurityPolicy         ReadinessCheckName = "security.policy_valid"
	CheckRBACIntegrity          ReadinessCheckName = "rbac.integrity"
	CheckBackupAvailability     ReadinessCheckName = "backup.availability"
)

// ReadinessGateResult is the outcome of a single readiness check.
type ReadinessGateResult struct {
	Check  ReadinessCheckName `json:"check"`
	Passed bool               `json:"passed"`
	Detail string             `json:"detail"`
}

// ProductionReadinessReport is the output of RunProductionReadinessAudit.
// AssessedAt is excluded from ReportHash (content-identity).
type ProductionReadinessReport struct {
	ReportID    string                `json:"report_id"`
	TenantID    string                `json:"tenant_id"`
	ExecutionID string                `json:"execution_id"`
	Gates       []ReadinessGateResult `json:"gates"`
	GatesPassed int                   `json:"gates_passed"`
	GatesTotal  int                   `json:"gates_total"`
	Ready       bool                  `json:"ready"`
	Limitations []string              `json:"limitations"`
	AssessedAt  string                `json:"assessed_at"` // excluded from ReportHash
	ReportHash  string                `json:"report_hash"`
}

// ReadinessInput carries observable platform state for the audit.
// No credentials are included — only counts, flags, and hashes.
type ReadinessInput struct {
	TenantID    string
	ExecutionID string

	StorageReadsHealthy  bool
	StorageWritesHealthy bool

	ReplayDivergenceCount int
	ReplayCertified       bool

	UnresolvedContradictions int
	ContradictionScanOK      bool

	GovernanceDecisionCount int
	GovernanceHashValid     bool

	ArchiveEntryCount int
	ArchiveHashValid  bool

	ActiveWorkerCount  int
	QuarantinedWorkers int

	APIEndpointsHealthy bool

	RBACRulesValid   bool
	NoCredentialLeak bool

	LatestBackupManifestValid bool
}

// RunProductionReadinessAudit evaluates all readiness gates and returns a signed report.
func RunProductionReadinessAudit(reportID string, input ReadinessInput) ProductionReadinessReport {
	gates := []ReadinessGateResult{
		evalStorageHealth(input),
		evalReplayIntegrity(input),
		evalContradictionIntegrity(input),
		evalGovernanceIntegrity(input),
		evalArchiveRetrievability(input),
		evalWorkerLiveness(input),
		evalAPIHealth(input),
		evalSecurityPolicy(input),
		evalRBACIntegrity(input),
		evalBackupAvailability(input),
	}

	passed := 0
	for _, g := range gates {
		if g.Passed {
			passed++
		}
	}

	limitations := []string{
		"Storage health is assessed in-process; external replica lag is not measured.",
		"Worker liveness requires heartbeat data; stale heartbeats may not be detected.",
		"Security policy validity depends on caller-provided flag — independent log scanning required.",
		"Backup availability only verifies the latest manifest hash, not retrieval of payload bytes.",
	}

	report := ProductionReadinessReport{
		ReportID:    reportID,
		TenantID:    input.TenantID,
		ExecutionID: input.ExecutionID,
		Gates:       gates,
		GatesPassed: passed,
		GatesTotal:  len(gates),
		Ready:       passed == len(gates),
		Limitations: limitations,
		AssessedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	report.ReportHash = computeReportHash(report)
	return report
}

// VerifyProductionReadinessReport checks that the report hash is consistent with its content.
func VerifyProductionReadinessReport(r ProductionReadinessReport) (bool, string) {
	expected := computeReportHash(r)
	if r.ReportHash != expected {
		return false, fmt.Sprintf("report %q hash mismatch (stored=%s computed=%s)",
			r.ReportID, r.ReportHash, expected)
	}
	return true, ""
}

// --- gate evaluators ---

func evalStorageHealth(in ReadinessInput) ReadinessGateResult {
	if !in.StorageReadsHealthy || !in.StorageWritesHealthy {
		return ReadinessGateResult{Check: CheckStorageHealth, Detail: "storage reads or writes are not healthy"}
	}
	return ReadinessGateResult{Check: CheckStorageHealth, Passed: true, Detail: "reads and writes healthy"}
}

func evalReplayIntegrity(in ReadinessInput) ReadinessGateResult {
	if in.ReplayDivergenceCount > 0 {
		return ReadinessGateResult{
			Check:  CheckReplayIntegrity,
			Detail: fmt.Sprintf("%d replay divergences detected", in.ReplayDivergenceCount),
		}
	}
	if !in.ReplayCertified {
		return ReadinessGateResult{Check: CheckReplayIntegrity, Detail: "platform is not replay-certified"}
	}
	return ReadinessGateResult{Check: CheckReplayIntegrity, Passed: true, Detail: "replay certified, zero divergences"}
}

func evalContradictionIntegrity(in ReadinessInput) ReadinessGateResult {
	if !in.ContradictionScanOK {
		return ReadinessGateResult{Check: CheckContradictionIntegrity, Detail: "contradiction scan did not complete"}
	}
	return ReadinessGateResult{
		Check:  CheckContradictionIntegrity,
		Passed: true,
		Detail: fmt.Sprintf("contradiction scan complete; %d unresolved (visible to operators)", in.UnresolvedContradictions),
	}
}

func evalGovernanceIntegrity(in ReadinessInput) ReadinessGateResult {
	if !in.GovernanceHashValid {
		return ReadinessGateResult{Check: CheckGovernanceIntegrity, Detail: "governance decision hash invalid"}
	}
	return ReadinessGateResult{
		Check:  CheckGovernanceIntegrity,
		Passed: true,
		Detail: fmt.Sprintf("%d governance decisions with valid hashes", in.GovernanceDecisionCount),
	}
}

func evalArchiveRetrievability(in ReadinessInput) ReadinessGateResult {
	if !in.ArchiveHashValid {
		return ReadinessGateResult{Check: CheckArchiveRetrievability, Detail: "archive hash verification failed"}
	}
	return ReadinessGateResult{
		Check:  CheckArchiveRetrievability,
		Passed: true,
		Detail: fmt.Sprintf("archive hash valid; %d entries", in.ArchiveEntryCount),
	}
}

func evalWorkerLiveness(in ReadinessInput) ReadinessGateResult {
	if in.ActiveWorkerCount == 0 {
		return ReadinessGateResult{Check: CheckWorkerLiveness, Detail: "no active workers detected"}
	}
	if in.QuarantinedWorkers > 0 {
		return ReadinessGateResult{
			Check:  CheckWorkerLiveness,
			Detail: fmt.Sprintf("%d workers quarantined", in.QuarantinedWorkers),
		}
	}
	return ReadinessGateResult{
		Check:  CheckWorkerLiveness,
		Passed: true,
		Detail: fmt.Sprintf("%d active workers, none quarantined", in.ActiveWorkerCount),
	}
}

func evalAPIHealth(in ReadinessInput) ReadinessGateResult {
	if !in.APIEndpointsHealthy {
		return ReadinessGateResult{Check: CheckAPIHealth, Detail: "API endpoints are not healthy"}
	}
	return ReadinessGateResult{Check: CheckAPIHealth, Passed: true, Detail: "all API endpoints healthy"}
}

func evalSecurityPolicy(in ReadinessInput) ReadinessGateResult {
	if !in.NoCredentialLeak {
		return ReadinessGateResult{Check: CheckSecurityPolicy, Detail: "credential leak detected in logs"}
	}
	return ReadinessGateResult{Check: CheckSecurityPolicy, Passed: true, Detail: "no credential leaks detected"}
}

func evalRBACIntegrity(in ReadinessInput) ReadinessGateResult {
	if !in.RBACRulesValid {
		return ReadinessGateResult{Check: CheckRBACIntegrity, Detail: "RBAC rules failed validation"}
	}
	return ReadinessGateResult{Check: CheckRBACIntegrity, Passed: true, Detail: "RBAC rules valid"}
}

func evalBackupAvailability(in ReadinessInput) ReadinessGateResult {
	if !in.LatestBackupManifestValid {
		return ReadinessGateResult{Check: CheckBackupAvailability, Detail: "latest backup manifest hash invalid or missing"}
	}
	return ReadinessGateResult{Check: CheckBackupAvailability, Passed: true, Detail: "latest backup manifest verified"}
}

func computeReportHash(r ProductionReadinessReport) string {
	type stableReport struct {
		ReportID    string                `json:"report_id"`
		TenantID    string                `json:"tenant_id"`
		ExecutionID string                `json:"execution_id"`
		Gates       []ReadinessGateResult `json:"gates"`
		GatesPassed int                   `json:"gates_passed"`
		GatesTotal  int                   `json:"gates_total"`
		Ready       bool                  `json:"ready"`
		Limitations []string              `json:"limitations"`
	}
	stable := stableReport{
		ReportID:    r.ReportID,
		TenantID:    r.TenantID,
		ExecutionID: r.ExecutionID,
		Gates:       r.Gates,
		GatesPassed: r.GatesPassed,
		GatesTotal:  r.GatesTotal,
		Ready:       r.Ready,
		Limitations: r.Limitations,
	}
	b, _ := json.Marshal(stable)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
