package audit

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// PlatformRuntimeSnapshot is a tamper-evident, point-in-time capture of the entire
// platform runtime state across executions, workers, governance, and archives.
// SnapshotAt is excluded from SnapshotHash (content-identity).
type PlatformRuntimeSnapshot struct {
	SnapshotID            string                     `json:"snapshot_id"`
	TenantID              string                     `json:"tenant_id"`
	ExecutionCount        int                        `json:"execution_count"`
	WorkflowCount         int                        `json:"workflow_count"`
	ActiveWorkers         int                        `json:"active_workers"`
	PendingApprovals      int                        `json:"pending_approvals"`
	OpenContradictions    int                        `json:"open_contradictions"`
	ArchiveIntegrity      bool                       `json:"archive_integrity"`
	FederationNodeCount   int                        `json:"federation_node_count"`
	ReplayCertifications  []ReplayCertificationReport `json:"replay_certifications"`
	AuditResults          []AuditResult              `json:"audit_results"`
	SafetyAssessments     []OperatorSafetyAssessment `json:"safety_assessments"`
	ObservabilityState    string                     `json:"observability_state"` // healthy|degraded|critical
	SnapshotAt            string                     `json:"snapshot_at"`         // excluded from hash
	SnapshotHash          string                     `json:"snapshot_hash"`
}

// BuildPlatformRuntimeSnapshot assembles a tamper-evident platform snapshot.
func BuildPlatformRuntimeSnapshot(
	snapshotID, tenantID string,
	executionCount, workflowCount, activeWorkers int,
	pendingApprovals, openContradictions, federationNodes int,
	archiveIntegrity bool,
	certifications []ReplayCertificationReport,
	auditResults []AuditResult,
	safetyAssessments []OperatorSafetyAssessment,
	observabilityState string,
) PlatformRuntimeSnapshot {
	certsCopy := make([]ReplayCertificationReport, len(certifications))
	copy(certsCopy, certifications)
	auditsCopy := make([]AuditResult, len(auditResults))
	copy(auditsCopy, auditResults)
	safetyCopy := make([]OperatorSafetyAssessment, len(safetyAssessments))
	copy(safetyCopy, safetyAssessments)

	snap := PlatformRuntimeSnapshot{
		SnapshotID:           snapshotID,
		TenantID:             tenantID,
		ExecutionCount:       executionCount,
		WorkflowCount:        workflowCount,
		ActiveWorkers:        activeWorkers,
		PendingApprovals:     pendingApprovals,
		OpenContradictions:   openContradictions,
		ArchiveIntegrity:     archiveIntegrity,
		FederationNodeCount:  federationNodes,
		ReplayCertifications: certsCopy,
		AuditResults:         auditsCopy,
		SafetyAssessments:    safetyCopy,
		ObservabilityState:   observabilityState,
		SnapshotAt:           time.Now().UTC().Format(time.RFC3339Nano),
	}
	snap.SnapshotHash = computePlatformSnapshotHash(snap)
	return snap
}

// VerifyPlatformSnapshot recomputes the hash and returns (true, "") when it matches.
func VerifyPlatformSnapshot(snap PlatformRuntimeSnapshot) (bool, string) {
	stored := snap.SnapshotHash
	snap.SnapshotHash = ""
	expected := computePlatformSnapshotHash(snap)
	if stored != expected {
		return false, fmt.Sprintf("platform snapshot %q hash mismatch: stored=%s computed=%s",
			snap.SnapshotID, stored, expected)
	}
	return true, ""
}

func computePlatformSnapshotHash(snap PlatformRuntimeSnapshot) string {
	h := sha256.New()
	fmt.Fprintf(h,
		"platform:%s|tenant:%s|execs:%d|workflows:%d|workers:%d|approvals:%d|contradictions:%d|archive:%v|federation:%d|certs:%d|audits:%d|safety:%d|obs:%s",
		snap.SnapshotID, snap.TenantID,
		snap.ExecutionCount, snap.WorkflowCount, snap.ActiveWorkers,
		snap.PendingApprovals, snap.OpenContradictions,
		snap.ArchiveIntegrity, snap.FederationNodeCount,
		len(snap.ReplayCertifications), len(snap.AuditResults), len(snap.SafetyAssessments),
		snap.ObservabilityState)
	for _, c := range snap.ReplayCertifications {
		fmt.Fprintf(h, "|cert:%s/%v", c.CertificationID, c.Certified)
	}
	for _, a := range snap.AuditResults {
		fmt.Fprintf(h, "|audit:%s/%v", a.AuditID, a.Passed)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
