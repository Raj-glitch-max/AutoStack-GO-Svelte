package platformapi

import (
	"time"

	"github.com/janlauber/one-click/pkg/audit"
)

// CertificationView is a read-only projection of a ReplayCertificationReport.
type CertificationView struct {
	CertificationID     string `json:"certification_id"`
	ExecutionID         string `json:"execution_id"`
	TenantID            string `json:"tenant_id"`
	Certified           bool   `json:"certified"`
	TimelineIntegrity   bool   `json:"timeline_integrity"`
	CheckpointValid     bool   `json:"checkpoint_valid"`
	DivergenceFree      bool   `json:"divergence_free"`
	ConfidenceConsistent bool  `json:"confidence_consistent"`
	TotalEventsVerified int    `json:"total_events_verified"`
	TotalDivergences    int    `json:"total_divergences"`
	HashValid           bool   `json:"hash_valid"`
	CertifiedAt         string `json:"certified_at"`
}

// CertificationQueryResult groups certification views for an execution.
type CertificationQueryResult struct {
	ExecutionID    string              `json:"execution_id"`
	TenantID       string              `json:"tenant_id"`
	Certifications []CertificationView `json:"certifications"`
	TotalCount     int                 `json:"total_count"`
	CertifiedCount int                 `json:"certified_count"`
	QueryAt        string              `json:"query_at"`
}

// ProjectCertification converts a ReplayCertificationReport to an API view.
func ProjectCertification(r audit.ReplayCertificationReport) CertificationView {
	ok, _ := audit.VerifyCertificationHash(r)
	return CertificationView{
		CertificationID:      r.CertificationID,
		ExecutionID:          r.ExecutionID,
		TenantID:             r.TenantID,
		Certified:            r.Certified,
		TimelineIntegrity:    r.TimelineIntegrity,
		CheckpointValid:      r.CheckpointValid,
		DivergenceFree:       r.DivergenceFree,
		ConfidenceConsistent: r.ConfidenceConsistent,
		TotalEventsVerified:  r.TotalEventsVerified,
		TotalDivergences:     r.TotalDivergences,
		HashValid:            ok,
		CertifiedAt:          r.CertifiedAt,
	}
}

// QueryCertifications returns all certification views for an execution and tenant.
func QueryCertifications(certs []audit.ReplayCertificationReport, executionID, tenantID string) CertificationQueryResult {
	result := CertificationQueryResult{
		ExecutionID: executionID,
		TenantID:    tenantID,
		QueryAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}

	for _, r := range certs {
		if r.ExecutionID != executionID || r.TenantID != tenantID {
			continue
		}
		view := ProjectCertification(r)
		result.Certifications = append(result.Certifications, view)
		if view.Certified {
			result.CertifiedCount++
		}
	}

	result.TotalCount = len(result.Certifications)
	return result
}

// LatestCertification returns the most-recently-issued certification for an execution,
// or (zero, false) if none exist.
func LatestCertification(certs []audit.ReplayCertificationReport, executionID, tenantID string) (CertificationView, bool) {
	var latest *audit.ReplayCertificationReport
	for i := range certs {
		if certs[i].ExecutionID != executionID || certs[i].TenantID != tenantID {
			continue
		}
		if latest == nil || certs[i].CertifiedAt > latest.CertifiedAt {
			cp := certs[i]
			latest = &cp
		}
	}
	if latest == nil {
		return CertificationView{}, false
	}
	return ProjectCertification(*latest), true
}
