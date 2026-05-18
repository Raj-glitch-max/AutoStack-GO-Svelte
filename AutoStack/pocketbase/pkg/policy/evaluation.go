package policy

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// PolicyOutcome is the result of a policy evaluation.
type PolicyOutcome string

const (
	PolicyOutcomeAllow           PolicyOutcome = "allow"
	PolicyOutcomeDeny            PolicyOutcome = "deny"
	PolicyOutcomeRequireApproval PolicyOutcome = "require_approval"
	PolicyOutcomeEscalate        PolicyOutcome = "escalate"
)

// PolicyDecision is an immutable record of one policy evaluation.
// All inputs and the resulting outcome are persisted; nothing is inferred after the fact.
type PolicyDecision struct {
	DecisionID       string        `json:"decision_id"`
	PolicyID         string        `json:"policy_id"`
	TenantID         string        `json:"tenant_id"`
	Action           string        `json:"action"`
	Outcome          PolicyOutcome `json:"outcome"`
	MatchedRules     []string      `json:"matched_rules"`
	InputConfidence  float64       `json:"input_confidence"`
	EscalationReason string        `json:"escalation_reason,omitempty"`
	RequiresApproval bool          `json:"requires_approval"`
	DecidedAt        string        `json:"decided_at"`
	Hash             string        `json:"hash"`
}

// EvaluatePolicy evaluates whether the action is allowed under the given policy,
// considering the current confidence score. Returns an immutable PolicyDecision.
//
// Decision logic (in order):
//  1. If action is forbidden → deny
//  2. If confidence < policy.RiskThreshold → escalate
//  3. If action requires approval → require_approval
//  4. If action is allowed → allow
//  5. Default → deny (deny by default, not allow by default)
func EvaluatePolicy(policy Policy, action string, confidence float64, decisionID string) PolicyDecision {
	d := PolicyDecision{
		DecisionID:      decisionID,
		PolicyID:        policy.PolicyID,
		TenantID:        policy.TenantID,
		Action:          action,
		InputConfidence: confidence,
		DecidedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}

	if ActionForbidden(policy, action) {
		d.Outcome = PolicyOutcomeDeny
		d.MatchedRules = []string{"forbidden_actions"}
		d.Hash = computeDecisionHash(d)
		return d
	}

	if policy.RiskThreshold > 0 && confidence < policy.RiskThreshold {
		d.Outcome = PolicyOutcomeEscalate
		d.EscalationReason = fmt.Sprintf("confidence %.3f below threshold %.3f", confidence, policy.RiskThreshold)
		d.MatchedRules = []string{"risk_threshold"}
		d.Hash = computeDecisionHash(d)
		return d
	}

	if ActionRequiresApproval(policy, action) {
		d.Outcome = PolicyOutcomeRequireApproval
		d.RequiresApproval = true
		d.MatchedRules = []string{"requires_approval"}
		d.Hash = computeDecisionHash(d)
		return d
	}

	if ActionAllowed(policy, action) {
		d.Outcome = PolicyOutcomeAllow
		d.MatchedRules = []string{"allowed_actions"}
		d.Hash = computeDecisionHash(d)
		return d
	}

	// Default deny — not explicitly allowed.
	d.Outcome = PolicyOutcomeDeny
	d.MatchedRules = []string{"default_deny"}
	d.Hash = computeDecisionHash(d)
	return d
}

// VerifyDecisionHash recomputes the hash and returns (true, "") when it matches.
func VerifyDecisionHash(d PolicyDecision) (bool, string) {
	stored := d.Hash
	d.Hash = ""
	expected := computeDecisionHash(d)
	if stored != expected {
		return false, fmt.Sprintf("decision %q hash mismatch: stored=%s computed=%s",
			d.DecisionID, stored, expected)
	}
	return true, ""
}

func computeDecisionHash(d PolicyDecision) string {
	h := sha256.New()
	fmt.Fprintf(h, "decision:%s|policy:%s|tenant:%s|action:%s|outcome:%s|confidence:%.6f|approval:%v|escalation:%s|ts:%s",
		d.DecisionID, d.PolicyID, d.TenantID, d.Action,
		d.Outcome, d.InputConfidence, d.RequiresApproval, d.EscalationReason, d.DecidedAt)
	for _, r := range d.MatchedRules {
		fmt.Fprintf(h, "|rule:%s", r)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
