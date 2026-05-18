package policy

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// PolicyRuntime is the top-level governance state for one tenant's execution session.
// Thread-safe. All records are append-only.
type PolicyRuntime struct {
	mu          sync.RWMutex
	tenantID    string
	policies    []Policy
	decisions   []PolicyDecision
	approvals   *ApprovalQueue
	escalations []EscalationRecord
	assessments []RuntimeConfidenceAssessment
}

// NewPolicyRuntime creates an empty PolicyRuntime for the given tenant.
func NewPolicyRuntime(tenantID string) *PolicyRuntime {
	return &PolicyRuntime{
		tenantID:  tenantID,
		approvals: NewApprovalQueue(),
	}
}

// AddPolicy appends a policy. Errors on invalid hash or wrong tenant.
func (r *PolicyRuntime) AddPolicy(p Policy) error {
	if p.TenantID != r.tenantID {
		return fmt.Errorf("policy tenant %q does not match runtime tenant %q", p.TenantID, r.tenantID)
	}
	if ok, reason := VerifyPolicyHash(p); !ok {
		return fmt.Errorf("policy hash invalid: %s", reason)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policies = append(r.policies, p)
	return nil
}

// Enforce finds the first applicable policy for the action and enforces it.
// Returns ErrNoPolicy when no policy matches.
func (r *PolicyRuntime) Enforce(action, decisionID string, confidence float64) (EnforcementResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var policy *Policy
	for i := range r.policies {
		for _, a := range r.policies[i].AllowedActions {
			if a == action {
				policy = &r.policies[i]
				break
			}
		}
		if policy != nil {
			break
		}
		// Also check forbidden and requires-approval lists.
		for _, a := range r.policies[i].ForbiddenActions {
			if a == action {
				policy = &r.policies[i]
				break
			}
		}
		if policy != nil {
			break
		}
		for _, a := range r.policies[i].RequiresApproval {
			if a == action {
				policy = &r.policies[i]
				break
			}
		}
		if policy != nil {
			break
		}
	}

	if policy == nil {
		// No policy → default deny.
		policy = &Policy{
			PolicyID: "default",
			TenantID: r.tenantID,
		}
	}

	result, decision := EnforcePolicy(*policy, action, decisionID, confidence)
	r.decisions = append(r.decisions, decision)
	return result, nil
}

// RecordApprovalRequest adds an approval request to the queue.
func (r *PolicyRuntime) RecordApprovalRequest(req ApprovalRequest) error {
	return r.approvals.Request(req)
}

// RecordApprovalDecision records a decision for a pending request.
func (r *PolicyRuntime) RecordApprovalDecision(dec ApprovalDecision) error {
	return r.approvals.Decide(dec)
}

// AddEscalation appends an escalation record.
func (r *PolicyRuntime) AddEscalation(rec EscalationRecord) error {
	if ok, reason := VerifyEscalationRecord(rec); !ok {
		return fmt.Errorf("escalation hash invalid: %s", reason)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.escalations = append(r.escalations, rec)
	return nil
}

// RecordAssessment appends a confidence assessment.
func (r *PolicyRuntime) RecordAssessment(a RuntimeConfidenceAssessment) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.assessments = append(r.assessments, a)
}

// AllDecisions returns a copy of all policy decisions.
func (r *PolicyRuntime) AllDecisions() []PolicyDecision {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]PolicyDecision, len(r.decisions))
	copy(out, r.decisions)
	return out
}

// AllEscalations returns a copy of all escalation records.
func (r *PolicyRuntime) AllEscalations() []EscalationRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]EscalationRecord, len(r.escalations))
	copy(out, r.escalations)
	return out
}

// ApprovalQueue returns the underlying approval queue for direct inspection.
func (r *PolicyRuntime) ApprovalQueue() *ApprovalQueue {
	return r.approvals
}

// ExportSnapshot builds a tamper-evident GovernanceRuntimeSnapshot.
func (r *PolicyRuntime) ExportSnapshot(snapshotID string) GovernanceRuntimeSnapshot {
	r.mu.RLock()
	policies := make([]Policy, len(r.policies))
	copy(policies, r.policies)
	decisions := make([]PolicyDecision, len(r.decisions))
	copy(decisions, r.decisions)
	escalations := make([]EscalationRecord, len(r.escalations))
	copy(escalations, r.escalations)
	assessments := make([]RuntimeConfidenceAssessment, len(r.assessments))
	copy(assessments, r.assessments)
	r.mu.RUnlock()

	approvals := r.approvals.AllRequests()

	snap := GovernanceRuntimeSnapshot{
		SnapshotID:  snapshotID,
		TenantID:    r.tenantID,
		Policies:    policies,
		Decisions:   decisions,
		Approvals:   approvals,
		Escalations: escalations,
		Assessments: assessments,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	snap.SnapshotHash = computeGovernanceSnapshotHash(snap)
	return snap
}

// GovernanceRuntimeSnapshot is a tamper-evident export of all governance state.
// GeneratedAt is excluded from SnapshotHash (content-identity).
type GovernanceRuntimeSnapshot struct {
	SnapshotID   string                        `json:"snapshot_id"`
	TenantID     string                        `json:"tenant_id"`
	Policies     []Policy                      `json:"policies"`
	Decisions    []PolicyDecision              `json:"decisions"`
	Approvals    []ApprovalRequest             `json:"approvals"`
	Escalations  []EscalationRecord            `json:"escalations"`
	Assessments  []RuntimeConfidenceAssessment `json:"assessments"`
	GeneratedAt  string                        `json:"generated_at"` // excluded from hash
	SnapshotHash string                        `json:"snapshot_hash"`
}

// VerifyGovernanceSnapshot recomputes the hash and returns (true, "") when it matches.
func VerifyGovernanceSnapshot(snap GovernanceRuntimeSnapshot) (bool, string) {
	stored := snap.SnapshotHash
	snap.SnapshotHash = ""
	expected := computeGovernanceSnapshotHash(snap)
	if stored != expected {
		return false, fmt.Sprintf("governance snapshot %q hash mismatch: stored=%s computed=%s",
			snap.SnapshotID, stored, expected)
	}
	return true, ""
}

func computeGovernanceSnapshotHash(snap GovernanceRuntimeSnapshot) string {
	h := sha256.New()
	fmt.Fprintf(h, "gov:%s|tenant:%s|policies:%d|decisions:%d|approvals:%d|escalations:%d|assessments:%d",
		snap.SnapshotID, snap.TenantID,
		len(snap.Policies), len(snap.Decisions),
		len(snap.Approvals), len(snap.Escalations), len(snap.Assessments))
	for _, p := range snap.Policies {
		fmt.Fprintf(h, "|pol:%s/%s", p.PolicyID, p.PolicyHash)
	}
	for _, d := range snap.Decisions {
		fmt.Fprintf(h, "|dec:%s/%s", d.DecisionID, d.Hash)
	}
	for _, e := range snap.Escalations {
		fmt.Fprintf(h, "|esc:%s/%s", e.EscalationID, e.Hash)
	}
	for _, a := range snap.Assessments {
		fmt.Fprintf(h, "|assess:%s/%s", a.AssessmentID, a.AssessmentHash)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
