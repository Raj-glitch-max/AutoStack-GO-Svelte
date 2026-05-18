package observability

// ContradictionSeverity classifies the impact of a contradiction.
type ContradictionSeverity string

const (
	ContradictionSeverityCritical ContradictionSeverity = "critical"
	ContradictionSeverityHigh     ContradictionSeverity = "high"
	ContradictionSeverityMedium   ContradictionSeverity = "medium"
)

// ContradictionSummary is a read-only view of detected contradictions for an execution.
type ContradictionSummary struct {
	ExecutionID      string                `json:"execution_id"`
	TenantID         string                `json:"tenant_id"`
	TotalCount       int                   `json:"total_count"`
	ResolvedCount    int                   `json:"resolved_count"`
	UnresolvedCount  int                   `json:"unresolved_count"`
	ByType           map[string]int        `json:"by_type"`
	HighestSeverity  ContradictionSeverity `json:"highest_severity"`
	ConfidenceImpact float64               `json:"confidence_impact"`
}

// ContradictionInput is the minimal data needed to produce a summary entry.
type ContradictionInput struct {
	Type             string  `json:"type"`
	Resolved         bool    `json:"resolved"`
	ConfidenceImpact float64 `json:"confidence_impact"`
}

// SummarizeContradictions produces a ContradictionSummary from raw contradiction data.
// Derived from append-only evidence — read-only.
func SummarizeContradictions(executionID, tenantID string, contradictions []ContradictionInput) ContradictionSummary {
	summary := ContradictionSummary{
		ExecutionID: executionID,
		TenantID:    tenantID,
		TotalCount:  len(contradictions),
		ByType:      make(map[string]int),
	}
	for _, c := range contradictions {
		summary.ByType[c.Type]++
		summary.ConfidenceImpact += c.ConfidenceImpact
		if c.Resolved {
			summary.ResolvedCount++
		} else {
			summary.UnresolvedCount++
		}
	}

	// Classify severity by total confidence impact.
	switch {
	case summary.ConfidenceImpact >= 0.40:
		summary.HighestSeverity = ContradictionSeverityCritical
	case summary.ConfidenceImpact >= 0.25:
		summary.HighestSeverity = ContradictionSeverityHigh
	default:
		summary.HighestSeverity = ContradictionSeverityMedium
	}
	return summary
}
