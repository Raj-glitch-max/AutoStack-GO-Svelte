// Package policy implements Phase 5.5 Governed Autonomy + Human Approval Runtime.
//
// Core invariants:
//   - Every policy decision is persisted as an immutable record.
//   - Approvals are append-only; denials are never silently dropped.
//   - Autonomy MUST NOT bypass approval policy or quarantine.
//   - Confidence degrades deterministically — no probabilistic ML scoring.
//   - Operators can always inspect, override, and deny automation.
package policy

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// PolicyScope defines the scope at which a policy applies.
type PolicyScope string

const (
	PolicyScopeGlobal    PolicyScope = "global"
	PolicyScopeTenant    PolicyScope = "tenant"
	PolicyScopeExecution PolicyScope = "execution"
	PolicyScopeWorkflow  PolicyScope = "workflow"
)

// Policy defines what is allowed, what requires approval, and what is forbidden
// for a given tenant and scope.
type Policy struct {
	PolicyID         string      `json:"policy_id"`
	TenantID         string      `json:"tenant_id"`
	Scope            PolicyScope `json:"scope"`
	Name             string      `json:"name"`
	AllowedActions   []string    `json:"allowed_actions"`
	RequiresApproval []string    `json:"requires_approval"` // action names that need approval
	ForbiddenActions []string    `json:"forbidden_actions"` // always denied
	RiskThreshold    float64     `json:"risk_threshold"`    // confidence below this triggers escalation
	MaxRetries       int         `json:"max_retries"`
	AllowRollback    bool        `json:"allow_rollback"`
	AllowQuarantine  bool        `json:"allow_quarantine"`
	CreatedAt        string      `json:"created_at"`
	PolicyHash       string      `json:"policy_hash"`
}

// NewPolicy creates a Policy with its hash computed.
func NewPolicy(policyID, tenantID, name string, scope PolicyScope) Policy {
	p := Policy{
		PolicyID:  policyID,
		TenantID:  tenantID,
		Scope:     scope,
		Name:      name,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	p.PolicyHash = computePolicyHash(p)
	return p
}

// WithActions returns a copy of the policy with the given action lists set.
func WithActions(p Policy, allowed, requiresApproval, forbidden []string) Policy {
	cp := p
	cp.AllowedActions = append([]string(nil), allowed...)
	cp.RequiresApproval = append([]string(nil), requiresApproval...)
	cp.ForbiddenActions = append([]string(nil), forbidden...)
	cp.PolicyHash = computePolicyHash(cp)
	return cp
}

// WithRiskThreshold returns a copy of the policy with the risk threshold set.
func WithRiskThreshold(p Policy, threshold float64) Policy {
	cp := p
	cp.RiskThreshold = threshold
	cp.PolicyHash = computePolicyHash(cp)
	return cp
}

// VerifyPolicyHash recomputes the hash and returns (true, "") when it matches.
func VerifyPolicyHash(p Policy) (bool, string) {
	stored := p.PolicyHash
	p.PolicyHash = ""
	expected := computePolicyHash(p)
	if stored != expected {
		return false, fmt.Sprintf("policy %q hash mismatch: stored=%s computed=%s",
			p.PolicyID, stored, expected)
	}
	return true, ""
}

// ActionAllowed returns true when the action is in AllowedActions and NOT in ForbiddenActions.
func ActionAllowed(p Policy, action string) bool {
	for _, f := range p.ForbiddenActions {
		if f == action {
			return false
		}
	}
	for _, a := range p.AllowedActions {
		if a == action {
			return true
		}
	}
	return false
}

// ActionRequiresApproval returns true when the action is in RequiresApproval.
func ActionRequiresApproval(p Policy, action string) bool {
	for _, a := range p.RequiresApproval {
		if a == action {
			return true
		}
	}
	return false
}

// ActionForbidden returns true when the action is in ForbiddenActions.
func ActionForbidden(p Policy, action string) bool {
	for _, f := range p.ForbiddenActions {
		if f == action {
			return true
		}
	}
	return false
}

func computePolicyHash(p Policy) string {
	h := sha256.New()
	fmt.Fprintf(h, "policy:%s|tenant:%s|scope:%s|name:%s|risk:%.6f|retries:%d|rollback:%v|quarantine:%v|ts:%s",
		p.PolicyID, p.TenantID, p.Scope, p.Name,
		p.RiskThreshold, p.MaxRetries, p.AllowRollback, p.AllowQuarantine, p.CreatedAt)
	for _, a := range p.AllowedActions {
		fmt.Fprintf(h, "|allow:%s", a)
	}
	for _, a := range p.RequiresApproval {
		fmt.Fprintf(h, "|approval:%s", a)
	}
	for _, a := range p.ForbiddenActions {
		fmt.Fprintf(h, "|forbid:%s", a)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
