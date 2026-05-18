package platformapi

import (
	"time"

	"github.com/janlauber/one-click/pkg/governance"
)

// GovernanceView is a read-only projection of a GovernanceSnapshot.
type GovernanceView struct {
	ExecutionID  string `json:"execution_id"`
	PolicyCount  int    `json:"policy_count"`
	RoleCount    int    `json:"role_count"`
	DecisionCount int   `json:"decision_count"`
	HashValid    bool   `json:"hash_valid"`
	GeneratedAt  string `json:"generated_at"`
}

// GovernanceQueryResult wraps the governance view and pending approvals.
type GovernanceQueryResult struct {
	View           *GovernanceView       `json:"view,omitempty"`
	PendingSteps   []governance.ApprovalStep `json:"pending_steps,omitempty"`
	QueriedAt      string                `json:"queried_at"`
}

// QueryGovernanceState builds a read-only view from the current governance snapshot
// and any live approval hierarchy.
func QueryGovernanceState(snap *governance.GovernanceSnapshot, hier *governance.ApprovalHierarchy) GovernanceQueryResult {
	result := GovernanceQueryResult{
		QueriedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}

	if snap != nil {
		ok, _ := governance.VerifyGovernanceSnapshot(snap)
		result.View = &GovernanceView{
			ExecutionID:   snap.ExecutionID,
			PolicyCount:   len(snap.Policies),
			RoleCount:     len(snap.Roles),
			DecisionCount: len(snap.Decisions),
			HashValid:     ok,
			GeneratedAt:   snap.GeneratedAt,
		}
	}

	if hier != nil {
		result.PendingSteps = governance.PendingSteps(*hier)
	}

	return result
}

// GovernanceDecisionView is a read-only projection of a PolicyDecision.
type GovernanceDecisionView struct {
	RequestID  string `json:"request_id"`
	PolicyID   string `json:"policy_id"`
	Effect     string `json:"effect"`
	Reason     string `json:"reason,omitempty"`
	DecidedAt  string `json:"decided_at"`
}

// ProjectPolicyDecision converts a PolicyDecision to a safe API view.
func ProjectPolicyDecision(d governance.PolicyDecision) GovernanceDecisionView {
	return GovernanceDecisionView{
		RequestID: d.RequestID,
		PolicyID:  d.PolicyID,
		Effect:    string(d.Effect),
		Reason:    d.Reason,
		DecidedAt: d.DecidedAt,
	}
}
