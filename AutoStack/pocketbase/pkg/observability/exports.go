package observability

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// ObservabilityExport is a tamper-evident, read-only snapshot of all
// observability state for an execution. It is derived from append-only evidence
// and is NEVER used as a source of truth.
// ExportedAt is excluded from the ExportHash (content-identity).
type ObservabilityExport struct {
	ExportID             string               `json:"export_id"`
	ExecutionID          string               `json:"execution_id"`
	TenantID             string               `json:"tenant_id"`
	Metrics              ObservabilityMetrics `json:"metrics"`
	Health               HealthAssessment     `json:"health"`
	Timeline             ObservabilityTimeline `json:"timeline"`
	ContradictionSummary ContradictionSummary `json:"contradiction_summary"`
	ExportedAt           string               `json:"exported_at"` // excluded from hash
	ExportHash           string               `json:"export_hash"`
}

// BuildObservabilityExport assembles a tamper-evident observability export.
func BuildObservabilityExport(
	exportID, executionID, tenantID string,
	metrics ObservabilityMetrics,
	health HealthAssessment,
	timeline ObservabilityTimeline,
	contradictions ContradictionSummary,
) ObservabilityExport {
	exp := ObservabilityExport{
		ExportID:             exportID,
		ExecutionID:          executionID,
		TenantID:             tenantID,
		Metrics:              metrics,
		Health:               health,
		Timeline:             timeline,
		ContradictionSummary: contradictions,
		ExportedAt:           time.Now().UTC().Format(time.RFC3339Nano),
	}
	exp.ExportHash = computeExportHash(exp)
	return exp
}

// VerifyObservabilityExport recomputes the hash and returns (true, "") when it matches.
func VerifyObservabilityExport(exp ObservabilityExport) (bool, string) {
	stored := exp.ExportHash
	exp.ExportHash = ""
	expected := computeExportHash(exp)
	if stored != expected {
		return false, fmt.Sprintf("observability export %q hash mismatch: stored=%s computed=%s",
			exp.ExportID, stored, expected)
	}
	return true, ""
}

func computeExportHash(exp ObservabilityExport) string {
	h := sha256.New()
	fmt.Fprintf(h,
		"obs:%s|exec:%s|tenant:%s|tasks:%d|confidence:%.6f|contradictions:%d|divergences:%d|health:%s|timeline:%d",
		exp.ExportID, exp.ExecutionID, exp.TenantID,
		exp.Metrics.TotalTasks, exp.Metrics.ConfidenceScore,
		exp.Metrics.ContradictionCount, exp.Metrics.ReplayDivergences,
		exp.Health.Status, len(exp.Timeline.Entries))
	return fmt.Sprintf("%x", h.Sum(nil))
}
