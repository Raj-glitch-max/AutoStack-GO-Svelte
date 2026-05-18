package audit

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// ReplayCertificationReport certifies the deterministic replay integrity of an execution.
// CertifiedAt is excluded from CertificationHash (content-identity).
type ReplayCertificationReport struct {
	CertificationID    string `json:"certification_id"`
	ExecutionID        string `json:"execution_id"`
	TenantID           string `json:"tenant_id"`
	TimelineIntegrity  bool   `json:"timeline_integrity"`
	CheckpointValid    bool   `json:"checkpoint_valid"`
	DivergenceFree     bool   `json:"divergence_free"`
	ConfidenceConsistent bool `json:"confidence_consistent"`
	TotalEventsVerified int   `json:"total_events_verified"`
	TotalDivergences   int    `json:"total_divergences"`
	Certified          bool   `json:"certified"` // true only when all checks pass
	CertifiedAt        string `json:"certified_at"` // excluded from hash
	CertificationHash  string `json:"certification_hash"`
}

// CertifyReplay produces a tamper-evident replay certification report.
func CertifyReplay(
	certificationID, executionID, tenantID string,
	timelineIntegrity, checkpointValid, divFree, confidenceConsistent bool,
	totalEvents, totalDivergences int,
) ReplayCertificationReport {
	certified := timelineIntegrity && checkpointValid && divFree && confidenceConsistent

	r := ReplayCertificationReport{
		CertificationID:      certificationID,
		ExecutionID:          executionID,
		TenantID:             tenantID,
		TimelineIntegrity:    timelineIntegrity,
		CheckpointValid:      checkpointValid,
		DivergenceFree:       divFree,
		ConfidenceConsistent: confidenceConsistent,
		TotalEventsVerified:  totalEvents,
		TotalDivergences:     totalDivergences,
		Certified:            certified,
		CertifiedAt:          time.Now().UTC().Format(time.RFC3339Nano),
	}
	r.CertificationHash = computeCertificationHash(r)
	return r
}

// VerifyCertificationHash recomputes the hash and returns (true, "") when it matches.
func VerifyCertificationHash(r ReplayCertificationReport) (bool, string) {
	stored := r.CertificationHash
	r.CertificationHash = ""
	expected := computeCertificationHash(r)
	if stored != expected {
		return false, fmt.Sprintf("certification %q hash mismatch: stored=%s computed=%s",
			r.CertificationID, stored, expected)
	}
	return true, ""
}

func computeCertificationHash(r ReplayCertificationReport) string {
	h := sha256.New()
	fmt.Fprintf(h,
		"cert:%s|exec:%s|tenant:%s|timeline:%v|checkpoint:%v|divfree:%v|confidence:%v|events:%d|divergences:%d|certified:%v",
		r.CertificationID, r.ExecutionID, r.TenantID,
		r.TimelineIntegrity, r.CheckpointValid, r.DivergenceFree,
		r.ConfidenceConsistent, r.TotalEventsVerified, r.TotalDivergences, r.Certified)
	return fmt.Sprintf("%x", h.Sum(nil))
}
