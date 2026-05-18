package observability

// HealthStatus describes the operational status of an execution or platform component.
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusCritical  HealthStatus = "critical"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// HealthAssessment is a read-only summary derived from execution metrics.
// It is informational only — it does NOT trigger automated actions.
type HealthAssessment struct {
	ExecutionID     string       `json:"execution_id"`
	TenantID        string       `json:"tenant_id"`
	Status          HealthStatus `json:"status"`
	Reasons         []string     `json:"reasons"`
	ConfidenceScore float64      `json:"confidence_score"`
	HasContradictions bool       `json:"has_contradictions"`
	HasDivergences  bool         `json:"has_divergences"`
}

// AssessHealth derives a HealthAssessment from computed metrics.
// Thresholds: critical if confidence < 0.20 or contradictions exist;
// degraded if confidence < 0.50 or divergences exist; else healthy.
func AssessHealth(m ObservabilityMetrics) HealthAssessment {
	a := HealthAssessment{
		ExecutionID:       m.ExecutionID,
		TenantID:          m.TenantID,
		ConfidenceScore:   m.ConfidenceScore,
		HasContradictions: m.ContradictionCount > 0,
		HasDivergences:    m.ReplayDivergences > 0,
	}

	switch {
	case m.ConfidenceScore < 0.20 || m.ContradictionCount > 0:
		a.Status = HealthStatusCritical
		if m.ConfidenceScore < 0.20 {
			a.Reasons = append(a.Reasons, "confidence below critical threshold")
		}
		if m.ContradictionCount > 0 {
			a.Reasons = append(a.Reasons, "provider contradictions detected")
		}
	case m.ConfidenceScore < 0.50 || m.ReplayDivergences > 0:
		a.Status = HealthStatusDegraded
		if m.ConfidenceScore < 0.50 {
			a.Reasons = append(a.Reasons, "confidence below operating threshold")
		}
		if m.ReplayDivergences > 0 {
			a.Reasons = append(a.Reasons, "replay divergences detected")
		}
	default:
		a.Status = HealthStatusHealthy
	}
	return a
}
