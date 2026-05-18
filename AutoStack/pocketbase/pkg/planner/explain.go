package planner

import "fmt"

// Phase 4.0 — Plan explainability.
//
// Every planned action must be explainable to an operator. ExplainPlan
// produces a deterministic explanation for each node in the graph,
// answering: WHY is this needed? WHAT evidence triggered it? WHAT policy
// allowed it? WHAT alternatives were rejected? WHAT drift caused it?
// WHAT rollback exists?

// PlanExplanation is the human-readable forensic record for one planned action.
type PlanExplanation struct {
	NodeID   string `json:"node_id"`
	NodeType string `json:"node_type"`
	Action   string `json:"action"`

	// WhyNeeded is the primary reason for this action.
	WhyNeeded string `json:"why_needed"`

	// EvidenceSummary summarizes the evidence references that triggered this action.
	EvidenceSummary string `json:"evidence_summary"`

	// PolicyAllowed is the policy ID that authorized this action.
	// "none" when no policy was checked.
	PolicyAllowed string `json:"policy_allowed"`

	// AlternativesRejected lists actions that were considered and rejected
	// with the reason for rejection.
	AlternativesRejected []RejectedAlternative `json:"alternatives_rejected"`

	// DriftCause describes the provider drift (if any) that necessitated
	// this action.
	DriftCause string `json:"drift_cause"`

	// RollbackPlan describes the rollback path for this action.
	RollbackPlan string `json:"rollback_plan"`

	// RiskJustification explains why the risk level was assigned.
	RiskJustification string `json:"risk_justification"`

	// Limitations lists what the planner cannot know or guarantee about
	// this action.
	Limitations []string `json:"limitations"`
}

// RejectedAlternative is an action that was evaluated and rejected.
type RejectedAlternative struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

// PlanExplanationSet is the full explainability output for a plan.
type PlanExplanationSet struct {
	DesiredStateHash string            `json:"desired_state_hash"`
	Explanations     []PlanExplanation `json:"explanations"`
	GlobalLimitations []string         `json:"global_limitations"`
}

// ExplainPlan generates a deterministic explanation for every node in the
// PlanGraph. The output is stable: identical graph → identical explanations.
func ExplainPlan(g *PlanGraph, cs *ChangeSet, s *DesiredState) *PlanExplanationSet {
	set := &PlanExplanationSet{
		DesiredStateHash: s.DesiredStateHash,
		GlobalLimitations: globalLimitations(),
	}

	// Process in topological order for determinism.
	order := g.TopologicalOrder
	if len(order) == 0 {
		// Cyclic graph — explain all nodes in ID order.
		order = make([]string, len(g.Nodes))
		for i, n := range g.Nodes {
			order[i] = n.ID
		}
	}

	// Build a fast lookup of change items by node ID.
	changeIndex := make(map[string]ChangeItem)
	for _, item := range cs.AllChanges() {
		changeIndex[item.NodeID] = item
	}
	for _, item := range cs.NoOps {
		changeIndex[item.NodeID] = item
	}

	for _, id := range order {
		node := g.NodeIndex[id]
		item, hasItem := changeIndex[id]
		if !hasItem {
			item = ChangeItem{NodeID: id, Action: node.DesiredAction}
		}

		exp := PlanExplanation{
			NodeID:            node.ID,
			NodeType:          string(node.Type),
			Action:            string(node.DesiredAction),
			WhyNeeded:         node.WhyNeeded,
			EvidenceSummary:   summarizeEvidence(node.EvidenceRefs),
			PolicyAllowed:     summarizePolicies(node.PolicyRefs, s),
			AlternativesRejected: alternativesFor(node.DesiredAction, node),
			DriftCause:        driftCause(node, item),
			RollbackPlan:      item.RollbackImplication,
			RiskJustification: node.WhyRisk,
			Limitations:       nodeLimitations(node),
		}
		set.Explanations = append(set.Explanations, exp)
	}

	return set
}

// ─────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────

func summarizeEvidence(refs []EvidenceRef) string {
	if len(refs) == 0 {
		return "no evidence refs — decision is derived from DesiredState declaration alone"
	}
	if len(refs) == 1 {
		return fmt.Sprintf("1 evidence ref: %s/%s", refs[0].Collection, refs[0].ID)
	}
	return fmt.Sprintf("%d evidence refs (first: %s/%s)", len(refs), refs[0].Collection, refs[0].ID)
}

func summarizePolicies(policyRefs []string, s *DesiredState) string {
	if len(policyRefs) == 0 {
		return "none — no explicit policy governs this action"
	}
	// Find the policy name for the first ref.
	for _, p := range s.Policies {
		if p.ID == policyRefs[0] {
			return fmt.Sprintf("%s (id=%s)", p.Name, p.ID)
		}
	}
	return policyRefs[0]
}

func alternativesFor(chosen ActionType, node *PlanNode) []RejectedAlternative {
	all := []ActionType{ActionCreate, ActionUpdate, ActionDelete, ActionRollback, ActionNoOp}
	var rejected []RejectedAlternative
	for _, alt := range all {
		if alt == chosen {
			continue
		}
		rejected = append(rejected, RejectedAlternative{
			Action: string(alt),
			Reason: whyAlternativeRejected(alt, chosen, node),
		})
	}
	return rejected
}

func whyAlternativeRejected(alt, chosen ActionType, node *PlanNode) string {
	switch {
	case chosen == ActionCreate && alt == ActionUpdate:
		return "update requires existing resource; resource does not yet exist in provider state"
	case chosen == ActionCreate && alt == ActionDelete:
		return "delete requires existing resource; resource does not yet exist"
	case chosen == ActionCreate && alt == ActionNoOp:
		return "no-op is invalid: DesiredState declares this resource but it does not exist"
	case chosen == ActionUpdate && alt == ActionCreate:
		return "create would fail: resource already exists in provider state"
	case chosen == ActionUpdate && alt == ActionDelete:
		return "delete not selected: resource is still declared in DesiredState"
	case chosen == ActionUpdate && alt == ActionNoOp:
		return "no-op is invalid: spec diverges from provider state"
	case chosen == ActionDelete && alt == ActionCreate:
		return "create not selected: resource is absent from DesiredState"
	case chosen == ActionDelete && alt == ActionUpdate:
		return "update not selected: resource is absent from DesiredState; deletion is required"
	case chosen == ActionDelete && alt == ActionNoOp:
		return "no-op is invalid: resource must be removed per DesiredState"
	case chosen == ActionNoOp && alt == ActionCreate:
		return "create not needed: resource already matches desired state"
	case chosen == ActionNoOp && alt == ActionUpdate:
		return "update not needed: no spec divergence detected"
	case chosen == ActionNoOp && alt == ActionDelete:
		return "delete not selected: resource is still declared in DesiredState"
	case alt == ActionRollback:
		return "rollback not selected: no previous failure state to restore from"
	case chosen == ActionRollback && alt != ActionRollback:
		return fmt.Sprintf("%s not selected: resource is in a failed state requiring rollback, not standard action", alt)
	default:
		return fmt.Sprintf("not selected in favor of %s based on DesiredState diff", chosen)
	}
}

func driftCause(node *PlanNode, item ChangeItem) string {
	if len(item.DriftRefs) == 0 {
		if node.DesiredAction == ActionUpdate {
			return "spec divergence detected between DesiredState and current provider state; no specific drift record referenced"
		}
		return "no provider drift caused this action"
	}
	return fmt.Sprintf("caused by %d provider drift record(s): %v", len(item.DriftRefs), item.DriftRefs)
}

func nodeLimitations(node *PlanNode) []string {
	lims := []string{
		"planner cannot verify provider-side idempotency — apply may fail if provider has concurrent mutations",
		"image digest comparison is not supported — same-tag/different-digest drift is not detectable",
	}
	if node.DesiredAction == ActionDelete {
		lims = append(lims, "provider-side delete is irreversible in most cases — no restore guarantee")
	}
	if node.IsOrphan {
		lims = append(lims, "this node is an orphan (no dependency edges) — verify this is intentional")
	}
	return lims
}

func globalLimitations() []string {
	return []string{
		"the planner operates on DesiredState intent — it does not poll providers; actual provider state may differ",
		"plan hash is deterministic for identical inputs but does not account for provider-side state changes between planning and execution",
		"no distributed coordination — in a multi-instance setup, concurrent planning is possible; plans are not coordinated",
		"simulation does not predict provider-specific rate limits, quota exhaustion, or transient errors",
		"rollback paths are synthesized from the plan structure; they do not account for partial success states mid-apply",
	}
}
