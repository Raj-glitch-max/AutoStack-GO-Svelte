package providerops

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// ProviderOperationEnvelope binds a provider operation to all its evidence refs.
// EnvelopedAt is excluded from the hash (content-identity).
type ProviderOperationEnvelope struct {
	EnvelopeID       string      `json:"envelope_id"`
	OperationID      string      `json:"operation_id"`
	ExecutionID      string      `json:"execution_id"`
	TenantID         string      `json:"tenant_id"`
	ProviderID       string      `json:"provider_id"`
	OperationType    OperationType `json:"operation_type"`
	ResultState      ResultState `json:"result_state"`
	ResultRef        string      `json:"result_ref,omitempty"`
	RetryRef         string      `json:"retry_ref,omitempty"`
	VerificationRef  string      `json:"verification_ref,omitempty"`
	ContradictionRef string      `json:"contradiction_ref,omitempty"`
	RollbackRef      string      `json:"rollback_ref,omitempty"`
	RawEvidenceRef   string      `json:"raw_evidence_ref,omitempty"`
	ExecutionRef     string      `json:"execution_ref,omitempty"`
	EnvelopedAt      string      `json:"enveloped_at"` // excluded from hash
	EnvelopeHash     string      `json:"envelope_hash"`
}

// NewProviderOperationEnvelope creates an envelope linking an operation to its result.
func NewProviderOperationEnvelope(envelopeID string, op ProviderOperation, result NormalizedProviderResult) ProviderOperationEnvelope {
	env := ProviderOperationEnvelope{
		EnvelopeID:    envelopeID,
		OperationID:   op.OperationID,
		ExecutionID:   op.ExecutionID,
		TenantID:      op.TenantID,
		ProviderID:    op.ProviderID,
		OperationType: op.Type,
		ResultState:   result.State,
		ResultRef:     result.ResultID,
		RawEvidenceRef: result.RawEvidence,
		EnvelopedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	env.EnvelopeHash = computeEnvelopeHash(env)
	return env
}

// VerifyEnvelopeHash recomputes the hash and returns (true, "") when it matches.
func VerifyEnvelopeHash(env ProviderOperationEnvelope) (bool, string) {
	stored := env.EnvelopeHash
	env.EnvelopeHash = ""
	expected := computeEnvelopeHash(env)
	if stored != expected {
		return false, fmt.Sprintf("envelope %q hash mismatch: stored=%s computed=%s", env.EnvelopeID, stored, expected)
	}
	return true, ""
}

func computeEnvelopeHash(env ProviderOperationEnvelope) string {
	h := sha256.New()
	fmt.Fprintf(h, "env:%s|op:%s|exec:%s|tenant:%s|provider:%s|optype:%s|result:%s|result_ref:%s|retry:%s|verify:%s|contradiction:%s|rollback:%s|evidence:%s|execref:%s",
		env.EnvelopeID, env.OperationID, env.ExecutionID, env.TenantID, env.ProviderID,
		env.OperationType, env.ResultState, env.ResultRef,
		env.RetryRef, env.VerificationRef, env.ContradictionRef,
		env.RollbackRef, env.RawEvidenceRef, env.ExecutionRef)
	return fmt.Sprintf("%x", h.Sum(nil))
}
