package audit

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// AuditDimension names a category of integrity check.
type AuditDimension string

const (
	DimensionReplayIntegrity      AuditDimension = "replay_integrity"
	DimensionExecutionConsistency AuditDimension = "execution_consistency"
	DimensionPolicyConsistency    AuditDimension = "policy_consistency"
	DimensionFederationConsistency AuditDimension = "federation_consistency"
	DimensionWorkflowDivergence   AuditDimension = "workflow_divergence"
	DimensionCheckpointIntegrity  AuditDimension = "checkpoint_integrity"
	DimensionArchiveIntegrity     AuditDimension = "archive_integrity"
	DimensionTenantIsolation      AuditDimension = "tenant_isolation"
	DimensionLogicalClockContinuity AuditDimension = "logical_clock_continuity"
)

// AuditFinding is a single finding from the production truth audit.
type AuditFinding struct {
	Dimension   AuditDimension `json:"dimension"`
	Severity    string         `json:"severity"` // critical, warning, info
	Description string         `json:"description"`
	EntityRef   string         `json:"entity_ref,omitempty"`
}

// AuditInput carries all the counts needed to run a production truth audit.
// All values are derived from append-only stores — callers supply them.
type AuditInput struct {
	ExecutionID         string
	TenantID            string
	ReplayDivergences   int
	CheckpointHashValid bool
	PolicyDecisions     int
	PolicyDenials       int
	ContradictionCount  int
	WorkflowDivergences int
	ArchiveHashValid    bool
	TenantsCrossed      bool   // true if cross-tenant data access was detected
	ClockMonotonic      bool   // true if logical clock is fully monotonic
}

// AuditResult is the output of RunProductionTruthAudit.
// AuditedAt is excluded from AuditHash (content-identity).
type AuditResult struct {
	AuditID     string         `json:"audit_id"`
	ExecutionID string         `json:"execution_id"`
	TenantID    string         `json:"tenant_id"`
	Passed      bool           `json:"passed"`
	Findings    []AuditFinding `json:"findings"`
	AuditedAt   string         `json:"audited_at"` // excluded from hash
	AuditHash   string         `json:"audit_hash"`
}

// RunProductionTruthAudit performs a deterministic, multi-dimensional integrity check.
// It records all findings without halting on any single violation.
func RunProductionTruthAudit(auditID string, input AuditInput) AuditResult {
	result := AuditResult{
		AuditID:     auditID,
		ExecutionID: input.ExecutionID,
		TenantID:    input.TenantID,
		AuditedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}

	if input.ReplayDivergences > 0 {
		result.Findings = append(result.Findings, AuditFinding{
			Dimension:   DimensionReplayIntegrity,
			Severity:    "critical",
			Description: fmt.Sprintf("%d replay divergence(s) detected — execution history may be corrupted", input.ReplayDivergences),
			EntityRef:   input.ExecutionID,
		})
	}

	if !input.CheckpointHashValid {
		result.Findings = append(result.Findings, AuditFinding{
			Dimension:   DimensionCheckpointIntegrity,
			Severity:    "critical",
			Description: "checkpoint hash verification failed — checkpoint may be tampered",
			EntityRef:   input.ExecutionID,
		})
	}

	if !input.ArchiveHashValid {
		result.Findings = append(result.Findings, AuditFinding{
			Dimension:   DimensionArchiveIntegrity,
			Severity:    "critical",
			Description: "archive hash verification failed",
			EntityRef:   input.ExecutionID,
		})
	}

	if input.ContradictionCount > 0 {
		result.Findings = append(result.Findings, AuditFinding{
			Dimension:   DimensionExecutionConsistency,
			Severity:    "critical",
			Description: fmt.Sprintf("%d provider contradiction(s) detected — operator review required", input.ContradictionCount),
			EntityRef:   input.ExecutionID,
		})
	}

	if input.WorkflowDivergences > 0 {
		result.Findings = append(result.Findings, AuditFinding{
			Dimension:   DimensionWorkflowDivergence,
			Severity:    "warning",
			Description: fmt.Sprintf("%d workflow replay divergence(s) detected", input.WorkflowDivergences),
			EntityRef:   input.ExecutionID,
		})
	}

	if input.TenantsCrossed {
		result.Findings = append(result.Findings, AuditFinding{
			Dimension:   DimensionTenantIsolation,
			Severity:    "critical",
			Description: "cross-tenant data access detected — tenant isolation boundary violated",
			EntityRef:   input.TenantID,
		})
	}

	if !input.ClockMonotonic {
		result.Findings = append(result.Findings, AuditFinding{
			Dimension:   DimensionLogicalClockContinuity,
			Severity:    "warning",
			Description: "logical clock is not monotonically increasing — causal ordering may be unreliable",
			EntityRef:   input.ExecutionID,
		})
	}

	if input.PolicyDecisions > 0 && input.PolicyDenials == input.PolicyDecisions {
		result.Findings = append(result.Findings, AuditFinding{
			Dimension:   DimensionPolicyConsistency,
			Severity:    "warning",
			Description: "all policy decisions resulted in denial — execution may be fully blocked",
			EntityRef:   input.ExecutionID,
		})
	}

	hasCritical := false
	for _, f := range result.Findings {
		if f.Severity == "critical" {
			hasCritical = true
			break
		}
	}
	result.Passed = !hasCritical
	result.AuditHash = computeAuditHash(result)
	return result
}

// VerifyAuditHash recomputes the hash and returns (true, "") when it matches.
func VerifyAuditHash(r AuditResult) (bool, string) {
	stored := r.AuditHash
	r.AuditHash = ""
	expected := computeAuditHash(r)
	if stored != expected {
		return false, fmt.Sprintf("audit %q hash mismatch: stored=%s computed=%s", r.AuditID, stored, expected)
	}
	return true, ""
}

func computeAuditHash(r AuditResult) string {
	h := sha256.New()
	fmt.Fprintf(h, "audit:%s|exec:%s|tenant:%s|passed:%v|findings:%d",
		r.AuditID, r.ExecutionID, r.TenantID, r.Passed, len(r.Findings))
	for _, f := range r.Findings {
		fmt.Fprintf(h, "|f:%s/%s", f.Dimension, f.Severity)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
