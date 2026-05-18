package providerops

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// ReconciliationState describes whether provider state matches intended state.
type ReconciliationState string

const (
	ReconciliationStateConsistent    ReconciliationState = "consistent"
	ReconciliationStateInconsistent  ReconciliationState = "inconsistent"
	ReconciliationStateUnknown       ReconciliationState = "unknown"
	ReconciliationStateContradictory ReconciliationState = "contradictory"
)

// ProviderReconciliation is a hashed record comparing intended vs observed state.
// ReconcileAt is excluded from the hash (content-identity).
type ProviderReconciliation struct {
	ReconciliationID string              `json:"reconciliation_id"`
	OperationID      string              `json:"operation_id"`
	ExecutionID      string              `json:"execution_id"`
	TenantID         string              `json:"tenant_id"`
	ProviderID       string              `json:"provider_id"`
	IntendedState    string              `json:"intended_state"`
	ObservedState    string              `json:"observed_state"`
	State            ReconciliationState `json:"state"`
	Drift            []string            `json:"drift,omitempty"` // field names that differ
	ReconcileAt      string              `json:"reconcile_at"`    // excluded from hash
	ReconciliationHash string            `json:"reconciliation_hash"`
}

// ReconcileProviderState compares intended and observed state strings and returns
// a hashed reconciliation record. Callers supply pre-normalized state strings.
func ReconcileProviderState(reconciliationID, operationID, executionID, tenantID, providerID, intended, observed string, drift []string) ProviderReconciliation {
	var state ReconciliationState
	switch {
	case observed == "":
		state = ReconciliationStateUnknown
	case intended == observed:
		state = ReconciliationStateConsistent
	default:
		if len(drift) > 0 {
			state = ReconciliationStateInconsistent
		} else {
			state = ReconciliationStateContradictory
		}
	}

	driftCopy := make([]string, len(drift))
	copy(driftCopy, drift)

	r := ProviderReconciliation{
		ReconciliationID: reconciliationID,
		OperationID:      operationID,
		ExecutionID:      executionID,
		TenantID:         tenantID,
		ProviderID:       providerID,
		IntendedState:    intended,
		ObservedState:    observed,
		State:            state,
		Drift:            driftCopy,
		ReconcileAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	r.ReconciliationHash = computeReconciliationHash(r)
	return r
}

// VerifyReconciliationHash recomputes the hash and returns (true, "") when it matches.
func VerifyReconciliationHash(r ProviderReconciliation) (bool, string) {
	stored := r.ReconciliationHash
	r.ReconciliationHash = ""
	expected := computeReconciliationHash(r)
	if stored != expected {
		return false, fmt.Sprintf("reconciliation %q hash mismatch: stored=%s computed=%s",
			r.ReconciliationID, stored, expected)
	}
	return true, ""
}

func computeReconciliationHash(r ProviderReconciliation) string {
	h := sha256.New()
	fmt.Fprintf(h, "recon:%s|op:%s|exec:%s|tenant:%s|provider:%s|intended:%s|observed:%s|state:%s|drift:%d",
		r.ReconciliationID, r.OperationID, r.ExecutionID, r.TenantID, r.ProviderID,
		r.IntendedState, r.ObservedState, r.State, len(r.Drift))
	for _, d := range r.Drift {
		fmt.Fprintf(h, "|%s", d)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
