package planner

import "sort"

// Phase 4.0 — ChangeSet engine.
//
// A ChangeSet describes what the planner intends to do, with full
// forensic evidence for each change. It is derived from a PlanGraph
// and NEVER triggers any provider mutation. Execution (when eventually
// implemented in Phase 4.X) is strictly operator-initiated.

// ChangeItem describes one atomic change the planner wants to make.
type ChangeItem struct {
	// NodeID is the PlanNode this change applies to.
	NodeID string `json:"node_id"`

	// Action is the intended action.
	Action ActionType `json:"action"`

	// Why explains in human-readable text why this change is needed.
	Why string `json:"why"`

	// Evidence are reconciler-layer record references that justify this change.
	Evidence []EvidenceRef `json:"evidence"`

	// TriggeringPolicy is the PolicySpec.ID that authorized this action.
	// Empty if no explicit policy governs the action.
	TriggeringPolicy string `json:"triggering_policy,omitempty"`

	// DriftRefs are provider_drifts or provider_observations record IDs
	// that caused this change to be necessary.
	DriftRefs []string `json:"drift_refs"`

	// RollbackImplication describes what would need to happen to undo this change.
	RollbackImplication string `json:"rollback_implication"`

	// RiskEstimate is the classified risk of this change.
	RiskEstimate RiskLevel `json:"risk_estimate"`

	// Confidence is the planner's confidence that this change is correct.
	Confidence ConfidenceLevel `json:"confidence"`

	// ConstraintViolations lists any ConstraintSpec IDs violated by this change.
	// A non-empty list means the change should NOT be applied without operator override.
	ConstraintViolations []string `json:"constraint_violations,omitempty"`
}

// ChangeSet is the complete set of changes the planner proposes.
// It is derived deterministically from a PlanGraph. Identical graphs
// produce identical ChangeSets.
type ChangeSet struct {
	// Creates lists nodes that need to be provisioned.
	Creates []ChangeItem `json:"creates"`
	// Updates lists nodes that need configuration changes.
	Updates []ChangeItem `json:"updates"`
	// Deletes lists nodes that need to be deprovisioned.
	Deletes []ChangeItem `json:"deletes"`
	// Rollbacks lists nodes that need to return to a previous state.
	Rollbacks []ChangeItem `json:"rollbacks"`
	// NoOps lists nodes where no action is needed.
	NoOps []ChangeItem `json:"no_ops"`

	// TotalChanges is the count of actionable changes (Creates + Updates + Deletes + Rollbacks).
	TotalChanges int `json:"total_changes"`

	// HighestRisk is the maximum risk level across all actionable changes.
	// Empty when TotalChanges == 0.
	HighestRisk RiskLevel `json:"highest_risk,omitempty"`

	// HasConstraintViolations is true when any change violates a constraint.
	HasConstraintViolations bool `json:"has_constraint_violations"`

	// BlockedByConstraints is true when a blocking constraint (severity=block)
	// prevents the plan from being executed even with operator approval.
	BlockedByConstraints bool `json:"blocked_by_constraints"`
}

// BuildChangeSet derives a ChangeSet from a PlanGraph and the DesiredState's
// constraints. The graph must be valid (no cycles).
func BuildChangeSet(g *PlanGraph, s *DesiredState) *ChangeSet {
	if !g.IsValid() {
		// Cannot build a ChangeSet from a cyclic graph.
		return &ChangeSet{}
	}

	cs := &ChangeSet{}

	// Build constraint index: constraintID → ConstraintSpec.
	constraintIndex := make(map[string]ConstraintSpec, len(s.Constraints))
	for _, c := range s.Constraints {
		constraintIndex[c.ID] = c
	}

	// Process nodes in topological order for determinism.
	for _, id := range g.TopologicalOrder {
		node := g.NodeIndex[id]
		item := ChangeItem{
			NodeID:              node.ID,
			Action:              node.DesiredAction,
			Why:                 node.WhyNeeded,
			Evidence:            node.EvidenceRefs,
			RiskEstimate:        node.RiskLevel,
			Confidence:          confidenceForAction(node.DesiredAction, node),
			RollbackImplication: rollbackImplication(node),
		}

		// Check constraints.
		var violations []string
		for _, cid := range node.Constraints {
			spec, ok := constraintIndex[cid]
			if !ok {
				continue
			}
			if isConstraintViolated(spec, node) {
				violations = append(violations, cid)
				if spec.Severity == "block" {
					cs.BlockedByConstraints = true
				}
			}
		}
		if len(violations) > 0 {
			item.ConstraintViolations = violations
			cs.HasConstraintViolations = true
		}

		switch node.DesiredAction {
		case ActionCreate:
			cs.Creates = append(cs.Creates, item)
		case ActionUpdate:
			cs.Updates = append(cs.Updates, item)
		case ActionDelete:
			cs.Deletes = append(cs.Deletes, item)
		case ActionRollback:
			cs.Rollbacks = append(cs.Rollbacks, item)
		default:
			cs.NoOps = append(cs.NoOps, item)
		}
	}

	cs.TotalChanges = len(cs.Creates) + len(cs.Updates) + len(cs.Deletes) + len(cs.Rollbacks)
	cs.HighestRisk = highestRisk(cs)
	return cs
}

// ─────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────

func confidenceForAction(action ActionType, node *PlanNode) ConfidenceLevel {
	// Confidence is high for no-ops (we're certain nothing changes).
	// Confidence is medium for creates/updates (provider may respond differently).
	// Confidence is low for deletes/rollbacks (provider-side effects are uncertain).
	switch action {
	case ActionNoOp:
		return ConfidenceLevelHigh
	case ActionSimulate:
		return ConfidenceLevelHigh
	case ActionCreate, ActionUpdate:
		if node.RiskLevel == RiskLevelHigh || node.RiskLevel == RiskLevelCritical {
			return ConfidenceLevelLow
		}
		return ConfidenceLevelMedium
	case ActionDelete, ActionRollback:
		return ConfidenceLevelLow
	default:
		return ConfidenceLevelMedium
	}
}

func rollbackImplication(node *PlanNode) string {
	switch node.DesiredAction {
	case ActionCreate:
		return "rollback requires deleting the newly created resource; provider-side restoration is not guaranteed"
	case ActionUpdate:
		return "rollback requires re-applying the previous spec; requires a stored snapshot of the previous state"
	case ActionDelete:
		return "IRREVERSIBLE — no rollback path; deleted resources cannot be restored without a backup"
	case ActionRollback:
		return "rollback of a rollback requires re-applying the target spec from scratch"
	default:
		return "no rollback required — no-op changes are inherently reversible"
	}
}

func isConstraintViolated(c ConstraintSpec, node *PlanNode) bool {
	// Simple constraint evaluation: if the constraint applies to this node
	// (or applies to "all") and the node's action is a destructive action
	// while the constraint describes a safety boundary.
	if c.Applies != "all" && c.Applies != node.ID {
		return false
	}
	// A destructive delete constraint violation.
	if node.DesiredAction == ActionDelete && node.RiskLevel == RiskLevelCritical {
		return true
	}
	return false
}

func highestRisk(cs *ChangeSet) RiskLevel {
	levels := map[RiskLevel]int{
		RiskLevelLow:      1,
		RiskLevelMedium:   2,
		RiskLevelHigh:     3,
		RiskLevelCritical: 4,
	}
	max := 0
	var maxLevel RiskLevel
	allItems := append(append(append(cs.Creates, cs.Updates...), cs.Deletes...), cs.Rollbacks...)
	for _, item := range allItems {
		if v := levels[item.RiskEstimate]; v > max {
			max = v
			maxLevel = item.RiskEstimate
		}
	}
	return maxLevel
}

// AllChanges returns all actionable change items in topological order.
func (cs *ChangeSet) AllChanges() []ChangeItem {
	all := make([]ChangeItem, 0, cs.TotalChanges)
	all = append(all, cs.Creates...)
	all = append(all, cs.Updates...)
	all = append(all, cs.Deletes...)
	all = append(all, cs.Rollbacks...)
	sort.Slice(all, func(i, j int) bool { return all[i].NodeID < all[j].NodeID })
	return all
}
