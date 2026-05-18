package policy

// EnforcementResult summarizes the outcome of enforcing a policy for one action.
type EnforcementResult struct {
	Allowed          bool
	Blocked          bool   // true when blocked pending approval
	Denied           bool
	Escalated        bool
	RequiresApproval bool
	Decision         PolicyDecision
}

// EnforcePolicy evaluates a policy and returns a structured EnforcementResult.
// The underlying PolicyDecision is always returned for audit purposes.
//
// Autonomy restrictions enforced here:
//   - Autonomy MUST NOT proceed when Denied = true.
//   - Autonomy MUST NOT proceed when Escalated = true without operator review.
//   - Autonomy MUST NOT proceed when RequiresApproval = true without an ApprovalDecision.
func EnforcePolicy(policy Policy, action, decisionID string, confidence float64) (EnforcementResult, PolicyDecision) {
	decision := EvaluatePolicy(policy, action, confidence, decisionID)

	result := EnforcementResult{Decision: decision}

	switch decision.Outcome {
	case PolicyOutcomeAllow:
		result.Allowed = true
	case PolicyOutcomeDeny:
		result.Denied = true
	case PolicyOutcomeRequireApproval:
		result.Blocked = true
		result.RequiresApproval = true
	case PolicyOutcomeEscalate:
		result.Escalated = true
	}

	return result, decision
}

// IsAutonomyPermitted returns true when the EnforcementResult allows autonomous
// execution without operator intervention.
// Autonomy is NOT permitted when: denied, escalated, or requires approval.
func IsAutonomyPermitted(r EnforcementResult) bool {
	return r.Allowed && !r.Denied && !r.Escalated && !r.RequiresApproval && !r.Blocked
}
