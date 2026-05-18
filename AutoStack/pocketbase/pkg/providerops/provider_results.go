package providerops

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// ResultState is the canonical outcome of a provider operation.
type ResultState string

const (
	ResultStateSuccess       ResultState = "success"
	ResultStateFailed        ResultState = "failed"
	ResultStateContradictory ResultState = "contradictory"
	ResultStatePartial       ResultState = "partial"
	ResultStateUnknown       ResultState = "unknown"
	ResultStateTimeout       ResultState = "timeout"
)

// NormalizedProviderResult translates raw provider evidence into a canonical outcome.
// ObservedAt is excluded from the hash (content-identity).
type NormalizedProviderResult struct {
	ResultID    string      `json:"result_id"`
	OperationID string      `json:"operation_id"`
	ExecutionID string      `json:"execution_id"`
	TenantID    string      `json:"tenant_id"`
	ProviderID  string      `json:"provider_id"`
	State       ResultState `json:"state"`
	ResourceID  string      `json:"resource_id,omitempty"`
	RawEvidence string      `json:"raw_evidence,omitempty"` // reference, not full payload
	Reason      string      `json:"reason,omitempty"`
	ObservedAt  string      `json:"observed_at"` // excluded from hash
	ResultHash  string      `json:"result_hash"`
}

// NewNormalizedResult creates a canonical result record.
func NewNormalizedResult(resultID, operationID, executionID, tenantID, providerID string, state ResultState, resourceID, rawEvidence, reason string) NormalizedProviderResult {
	r := NormalizedProviderResult{
		ResultID:    resultID,
		OperationID: operationID,
		ExecutionID: executionID,
		TenantID:    tenantID,
		ProviderID:  providerID,
		State:       state,
		ResourceID:  resourceID,
		RawEvidence: rawEvidence,
		Reason:      reason,
		ObservedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	r.ResultHash = computeResultHash(r)
	return r
}

// VerifyResultHash recomputes the hash and returns (true, "") when it matches.
func VerifyResultHash(r NormalizedProviderResult) (bool, string) {
	stored := r.ResultHash
	r.ResultHash = ""
	expected := computeResultHash(r)
	if stored != expected {
		return false, fmt.Sprintf("result %q hash mismatch: stored=%s computed=%s", r.ResultID, stored, expected)
	}
	return true, ""
}

// IsSuccessful returns true only for clean success results.
func IsSuccessful(r NormalizedProviderResult) bool {
	return r.State == ResultStateSuccess
}

// IsAmbiguous returns true for states that require operator review.
func IsAmbiguous(r NormalizedProviderResult) bool {
	return r.State == ResultStateContradictory || r.State == ResultStatePartial || r.State == ResultStateUnknown
}

func computeResultHash(r NormalizedProviderResult) string {
	h := sha256.New()
	fmt.Fprintf(h, "result:%s|op:%s|exec:%s|tenant:%s|provider:%s|state:%s|resource:%s|evidence:%s|reason:%s",
		r.ResultID, r.OperationID, r.ExecutionID, r.TenantID, r.ProviderID,
		r.State, r.ResourceID, r.RawEvidence, r.Reason)
	return fmt.Sprintf("%x", h.Sum(nil))
}
