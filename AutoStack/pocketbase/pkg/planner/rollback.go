package planner

import "sort"

// Phase 4.0 — Rollback graph synthesis.
//
// Every mutation candidate in a ChangeSet has a rollback path synthesized
// by the planner. The rollback graph is the reverse topological ordering
// of the apply graph, with additional safety annotations.
//
// ─────────────────────────────────────────────────────────────────────────
// HONEST CONSTRAINTS
// ─────────────────────────────────────────────────────────────────────────
//
//   1. IRREVERSIBLE ACTIONS: destructive deletes (ActionDelete) cannot be
//      rolled back by the planner. The provider may not support restore.
//      These are flagged as Irreversible = true.
//
//   2. PARTIAL SUCCESS: the rollback graph assumes all-or-nothing apply.
//      Partial success (some nodes applied, others failed) produces a
//      state that the rollback graph does not model precisely. Operators
//      must investigate the actual provider state before running a rollback.
//
//   3. NO AUTOMATIC ROLLBACK: the rollback graph is a PLAN, not an
//      execution. The runtime never automatically triggers rollbacks.
//      Operator approval is required.

// RollbackStep describes how to undo one change item.
type RollbackStep struct {
	// NodeID is the node to rollback.
	NodeID string `json:"node_id"`

	// ReverseAction is the action to apply during rollback.
	ReverseAction ActionType `json:"reverse_action"`

	// Irreversible is true when the original action cannot be undone.
	Irreversible bool `json:"irreversible"`

	// IrreversibleReason explains why, when Irreversible is true.
	IrreversibleReason string `json:"irreversible_reason,omitempty"`

	// MustRollbackBefore lists node IDs that must be rolled back BEFORE
	// this node can be rolled back.
	MustRollbackBefore []string `json:"must_rollback_before"`

	// Note provides additional guidance to the operator.
	Note string `json:"note,omitempty"`
}

// RollbackBlocker is a dependency that prevents a node from being rolled back.
type RollbackBlocker struct {
	BlockedNodeID  string `json:"blocked_node_id"`
	BlockingNodeID string `json:"blocking_node_id"`
	Reason         string `json:"reason"`
}

// RollbackGraph is the complete rollback plan for a ChangeSet.
type RollbackGraph struct {
	// Steps is the ordered list of rollback operations.
	// The ordering is the reverse of the topological apply order:
	// nodes applied last are rolled back first.
	Steps []RollbackStep `json:"steps"`

	// Ordering is the node IDs in rollback execution order.
	Ordering []string `json:"ordering"`

	// Blockers lists dependency pairs that constrain rollback ordering.
	Blockers []RollbackBlocker `json:"blockers"`

	// IrreversibleNodes lists node IDs that cannot be rolled back.
	IrreversibleNodes []string `json:"irreversible_nodes"`

	// HasIrreversible is true when any step is irreversible.
	HasIrreversible bool `json:"has_irreversible"`
}

// BuildRollbackGraph synthesizes a rollback plan from a ChangeSet and
// the PlanGraph's topological order. The rollback order is the reverse
// of the apply order: the last node applied is the first to be rolled back.
func BuildRollbackGraph(cs *ChangeSet, g *PlanGraph) *RollbackGraph {
	rg := &RollbackGraph{}

	// Only actionable changes have rollback steps.
	actionable := cs.AllChanges()
	if len(actionable) == 0 {
		return rg
	}

	// Build rollback step index.
	stepIndex := make(map[string]*RollbackStep, len(actionable))
	for _, item := range actionable {
		step := &RollbackStep{
			NodeID:        item.NodeID,
			ReverseAction: reverseAction(item.Action),
			Irreversible:  item.Action == ActionDelete,
		}
		if step.Irreversible {
			step.IrreversibleReason = "provider-side delete cannot be reversed without a backup restore pipeline"
			rg.IrreversibleNodes = append(rg.IrreversibleNodes, item.NodeID)
			rg.HasIrreversible = true
		}
		step.Note = rollbackNote(item.Action)
		stepIndex[item.NodeID] = step
	}
	sort.Strings(rg.IrreversibleNodes)

	// Rollback ordering = reverse of topological apply order.
	// Filter to only nodes that are in the ChangeSet.
	actionableSet := make(map[string]bool, len(actionable))
	for _, item := range actionable {
		actionableSet[item.NodeID] = true
	}

	rollbackOrder := reverseOrder(g.TopologicalOrder, actionableSet)
	rg.Ordering = rollbackOrder

	// Compute MustRollbackBefore for each step.
	// If node B depends on node A, then during rollback A must be rolled back
	// BEFORE B (because B was applied AFTER A, so B must be undone first).
	for _, id := range rollbackOrder {
		step := stepIndex[id]
		node := g.NodeIndex[id]
		if node == nil {
			continue
		}
		// In rollback order, nodes that depend on this node in forward direction
		// (i.e., reverse deps) must be rolled back first.
		for _, revDep := range node.ReverseDependencies {
			if actionableSet[revDep] {
				step.MustRollbackBefore = append(step.MustRollbackBefore, revDep)
				rg.Blockers = append(rg.Blockers, RollbackBlocker{
					BlockedNodeID:  id,
					BlockingNodeID: revDep,
					Reason:         revDep + " depends on " + id + " and must be rolled back first",
				})
			}
		}
		sort.Strings(step.MustRollbackBefore)
		rg.Steps = append(rg.Steps, *step)
	}

	// Sort blockers for determinism.
	sort.Slice(rg.Blockers, func(i, j int) bool {
		if rg.Blockers[i].BlockedNodeID != rg.Blockers[j].BlockedNodeID {
			return rg.Blockers[i].BlockedNodeID < rg.Blockers[j].BlockedNodeID
		}
		return rg.Blockers[i].BlockingNodeID < rg.Blockers[j].BlockingNodeID
	})

	return rg
}

// ─────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────

func reverseAction(action ActionType) ActionType {
	switch action {
	case ActionCreate:
		return ActionDelete // undo a create by deleting
	case ActionDelete:
		return ActionCreate // undo a delete by recreating — but this is irreversible in practice
	case ActionUpdate:
		return ActionRollback // undo an update by restoring previous spec
	case ActionRollback:
		return ActionUpdate // undo a rollback by re-applying current spec
	default:
		return ActionNoOp
	}
}

func rollbackNote(action ActionType) string {
	switch action {
	case ActionCreate:
		return "rollback deletes the newly created resource; verify no data was written before proceeding"
	case ActionDelete:
		return "WARNING: this action is irreversible — delete cannot be undone without a backup restore pipeline"
	case ActionUpdate:
		return "rollback requires the previous spec snapshot; ensure it was captured before the update was applied"
	default:
		return ""
	}
}

func reverseOrder(topo []string, filter map[string]bool) []string {
	result := make([]string, 0, len(filter))
	for i := len(topo) - 1; i >= 0; i-- {
		if filter[topo[i]] {
			result = append(result, topo[i])
		}
	}
	return result
}
