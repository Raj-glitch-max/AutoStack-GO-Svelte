package governance

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// EvaluatePolicy evaluates a sorted set of policies against a PolicyRequest.
//
// Evaluation order: policies sorted by Priority descending (highest first).
// First matching policy wins. If no policy matches, the default effect is Allow.
//
// FORBIDDEN: auto-approval, silent escalation, bypassing deny rules.
// Pure function: same policies + same request → same decision.
func EvaluatePolicy(policies []Policy, req PolicyRequest) PolicyDecision {
	now := time.Now().UTC().Format(time.RFC3339)

	// Sort policies by priority descending; stable sort for determinism.
	sorted := append([]Policy(nil), policies...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority > sorted[j].Priority
	})

	for _, policy := range sorted {
		if policyMatches(policy, req) {
			return PolicyDecision{
				RequestID:            req.RequestID,
				PolicyID:             policy.PolicyID,
				Effect:               policy.Effect,
				Reason:               fmt.Sprintf("policy %q (%s) matched request %q", policy.PolicyID, policy.Name, req.RequestID),
				ApplicableConditions: append([]PolicyCondition(nil), policy.Conditions...),
				DecidedAt:            now,
			}
		}
	}

	// No matching policy → default allow (audit-logged by the caller).
	return PolicyDecision{
		RequestID: req.RequestID,
		PolicyID:  "default-allow",
		Effect:    PolicyEffectAllow,
		Reason:    "no matching policy found — default allow applies",
		DecidedAt: now,
	}
}

// policyMatches returns true when ALL conditions in the policy match the request.
func policyMatches(policy Policy, req PolicyRequest) bool {
	for _, cond := range policy.Conditions {
		if !conditionMatches(cond, req) {
			return false
		}
	}
	return true
}

// conditionMatches evaluates a single PolicyCondition against a PolicyRequest.
func conditionMatches(cond PolicyCondition, req PolicyRequest) bool {
	fieldValue := requestField(cond.Field, req)
	switch cond.Operator {
	case "eq":
		return strings.EqualFold(fieldValue, cond.Value)
	case "in":
		for _, v := range strings.Split(cond.Value, ",") {
			if strings.EqualFold(strings.TrimSpace(v), fieldValue) {
				return true
			}
		}
		return false
	case "gte":
		return riskRank(fieldValue) >= riskRank(cond.Value)
	default:
		return false
	}
}

func requestField(field string, req PolicyRequest) string {
	switch field {
	case "action":
		return req.Action
	case "risk_level":
		return req.RiskLevel
	case "provider":
		return req.Provider
	case "environment":
		return req.Environment
	default:
		return ""
	}
}

func riskRank(level string) int {
	switch strings.ToLower(level) {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "critical":
		return 4
	default:
		return 0
	}
}

// DefaultPolicies returns the canonical built-in policy set.
// These are starting points — operators must review and extend them.
func DefaultPolicies() []Policy {
	return []Policy{
		{
			PolicyID: "deny-destructive-delete-prod",
			Name:     "Deny Destructive Deletes in Production",
			Effect:   PolicyEffectDeny,
			Priority: 100,
			Conditions: []PolicyCondition{
				{Field: "action", Operator: "eq", Value: "delete"},
				{Field: "environment", Operator: "eq", Value: "production"},
			},
			Limitations: []string{"Does not cover non-production environments."},
		},
		{
			PolicyID: "require-approval-high-risk",
			Name:     "Require Approval for High-Risk Operations",
			Effect:   PolicyEffectRequireApproval,
			Priority: 90,
			Conditions: []PolicyCondition{
				{Field: "risk_level", Operator: "gte", Value: "high"},
			},
			Limitations: []string{"Approval must be satisfied before execution advances."},
		},
		{
			PolicyID: "require-multi-approval-critical",
			Name:     "Require Multi-Approval for Critical Operations",
			Effect:   PolicyEffectRequireMultiApproval,
			Priority: 95,
			Conditions: []PolicyCondition{
				{Field: "risk_level", Operator: "eq", Value: "critical"},
			},
			Limitations: []string{"Requires at least two distinct approvers."},
		},
	}
}
