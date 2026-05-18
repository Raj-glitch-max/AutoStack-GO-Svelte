package platformapi

import (
	"time"

	"github.com/janlauber/one-click/pkg/observability"
	"github.com/janlauber/one-click/pkg/providerops"
)

// ContradictionView is a read-only projection of a detected contradiction.
type ContradictionView struct {
	ContradictionID string `json:"contradiction_id"`
	ExecutionID     string `json:"execution_id"`
	TenantID        string `json:"tenant_id"`
	Type            string `json:"type"`
	Severity        string `json:"severity"`
	OperationID     string `json:"operation_id,omitempty"`
	Reason          string `json:"reason"`
	DetectedAt      string `json:"detected_at"`
}

// ContradictionQueryResult groups contradiction views.
type ContradictionQueryResult struct {
	ExecutionID    string              `json:"execution_id"`
	TenantID       string              `json:"tenant_id"`
	Contradictions []ContradictionView `json:"contradictions"`
	TotalCount     int                 `json:"total_count"`
	CriticalCount  int                 `json:"critical_count"`
	QueryAt        string              `json:"query_at"`
}

// ProjectProviderContradiction converts a ProviderContradiction to an API view.
func ProjectProviderContradiction(c providerops.ProviderContradiction, severity string) ContradictionView {
	return ContradictionView{
		ContradictionID: c.ContradictionID,
		ExecutionID:     c.ExecutionID,
		TenantID:        c.TenantID,
		Type:            string(c.Type),
		Severity:        severity,
		OperationID:     c.OperationID,
		Reason:          string(c.OperationType) + " contradiction: observed=" + c.ObservedState,
		DetectedAt:      c.DetectedAt,
	}
}

// QueryContradictions builds a contradiction view from provider contradictions
// and the observability contradiction summary for an execution.
func QueryContradictions(
	provContradictions []providerops.ProviderContradiction,
	summary *observability.ContradictionSummary,
	executionID, tenantID string,
) ContradictionQueryResult {
	result := ContradictionQueryResult{
		ExecutionID: executionID,
		TenantID:    tenantID,
		QueryAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}

	// Use the summary's highest severity as a uniform signal when available.
	globalSev := "medium"
	if summary != nil && summary.HighestSeverity != "" {
		globalSev = string(summary.HighestSeverity)
	}

	for _, c := range provContradictions {
		if c.ExecutionID != executionID || c.TenantID != tenantID {
			continue
		}
		view := ProjectProviderContradiction(c, globalSev)
		result.Contradictions = append(result.Contradictions, view)
		if globalSev == string(observability.ContradictionSeverityCritical) {
			result.CriticalCount++
		}
	}

	result.TotalCount = len(result.Contradictions)
	return result
}
