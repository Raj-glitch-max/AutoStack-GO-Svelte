package policy

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// EscalationTrigger identifies what caused an escalation.
type EscalationTrigger string

const (
	EscalationTriggerTimeout            EscalationTrigger = "timeout"
	EscalationTriggerLowConfidence      EscalationTrigger = "low_confidence"
	EscalationTriggerContradiction      EscalationTrigger = "contradiction"
	EscalationTriggerPolicyDenial       EscalationTrigger = "policy_denial"
	EscalationTriggerReplayDivergence   EscalationTrigger = "replay_divergence"
	EscalationTriggerApprovalExpired    EscalationTrigger = "approval_expired"
	EscalationTriggerMaxRetriesExceeded EscalationTrigger = "max_retries_exceeded"
)

// EscalationRecord is an immutable record of one escalation event.
// Resolution is recorded as a separate field — the original record is never mutated.
type EscalationRecord struct {
	EscalationID string            `json:"escalation_id"`
	ExecutionID  string            `json:"execution_id"`
	TenantID     string            `json:"tenant_id"`
	Trigger      EscalationTrigger `json:"trigger"`
	Reason       string            `json:"reason"`
	EscalatedAt  string            `json:"escalated_at"`
	ResolvedAt   string            `json:"resolved_at,omitempty"`
	Resolved     bool              `json:"resolved"`
	Hash         string            `json:"hash"`
}

// NewEscalationRecord creates an unresolved EscalationRecord with its hash computed.
func NewEscalationRecord(
	escalationID, executionID, tenantID string,
	trigger EscalationTrigger,
	reason string,
) EscalationRecord {
	r := EscalationRecord{
		EscalationID: escalationID,
		ExecutionID:  executionID,
		TenantID:     tenantID,
		Trigger:      trigger,
		Reason:       reason,
		EscalatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		Resolved:     false,
	}
	r.Hash = computeEscalationHash(r)
	return r
}

// ResolveEscalation returns a new EscalationRecord marked as resolved.
// The original record is never mutated. The new hash covers the resolution fields.
func ResolveEscalation(rec EscalationRecord) EscalationRecord {
	resolved := rec
	resolved.Resolved = true
	resolved.ResolvedAt = time.Now().UTC().Format(time.RFC3339Nano)
	resolved.Hash = computeEscalationHash(resolved)
	return resolved
}

// VerifyEscalationRecord recomputes the hash and returns (true, "") when it matches.
func VerifyEscalationRecord(rec EscalationRecord) (bool, string) {
	stored := rec.Hash
	rec.Hash = ""
	expected := computeEscalationHash(rec)
	if stored != expected {
		return false, fmt.Sprintf("escalation %q hash mismatch: stored=%s computed=%s",
			rec.EscalationID, stored, expected)
	}
	return true, ""
}

// ApprovalTimeoutEscalation creates an escalation from an expired approval request.
func ApprovalTimeoutEscalation(escalationID string, req ApprovalRequest) EscalationRecord {
	reason := fmt.Sprintf("approval request %q expired at %s without a decision", req.RequestID, req.ExpiresAt)
	return NewEscalationRecord(escalationID, req.ExecutionID, req.TenantID,
		EscalationTriggerApprovalExpired, reason)
}

func computeEscalationHash(r EscalationRecord) string {
	h := sha256.New()
	fmt.Fprintf(h, "esc:%s|exec:%s|tenant:%s|trigger:%s|reason:%s|ts:%s|resolved:%v|resolved_at:%s",
		r.EscalationID, r.ExecutionID, r.TenantID,
		r.Trigger, r.Reason, r.EscalatedAt, r.Resolved, r.ResolvedAt)
	return fmt.Sprintf("%x", h.Sum(nil))
}
