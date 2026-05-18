package orchestrator

import (
	"github.com/janlauber/one-click/pkg/planner"
)

// Phase 4.1 — Transaction boundary classification.
//
// Every execution stage and every node has a transaction boundary class
// that describes the transactional semantics of applying it. Classification
// is deterministic: same action + graph context → same boundary class.
//
// Boundary classification governs:
//   - Whether a stage can be retried on failure (atomic/compensatable).
//   - Whether rollback is possible (atomic/compensatable) or impossible (irreversible).
//   - Whether execution must pause for operator confirmation.

// TransactionBoundary is the complete transaction classification for one stage.
type TransactionBoundary struct {
	StageID         string                   `json:"stage_id"`
	BoundaryClass   TransactionBoundaryClass  `json:"boundary_class"`
	CanRetry        bool                     `json:"can_retry"`
	CanRollback     bool                     `json:"can_rollback"`
	CanCompensate   bool                     `json:"can_compensate"`
	RequiresPause   bool                     `json:"requires_pause"`
	Justification   string                   `json:"justification"`
	NodeBoundaries  []NodeBoundary           `json:"node_boundaries"`
}

// NodeBoundary is the transaction classification for one node within a stage.
type NodeBoundary struct {
	NodeID        string                   `json:"node_id"`
	Action        planner.ActionType       `json:"action"`
	BoundaryClass TransactionBoundaryClass  `json:"boundary_class"`
	Justification string                   `json:"justification"`
}

// ClassifyExecutionBoundary computes the transaction boundary for one stage.
// The stage boundary is the most restrictive boundary among all its nodes.
func ClassifyExecutionBoundary(stage ExecutionStage, g *planner.PlanGraph) TransactionBoundary {
	nodeBoundaries := make([]NodeBoundary, 0, len(stage.Nodes))
	for _, n := range stage.Nodes {
		nb := classifyNodeBoundaryInternal(n, g)
		nodeBoundaries = append(nodeBoundaries, nb)
	}

	// Stage boundary = most restrictive node boundary.
	stageBoundary := mostRestrictiveBoundary(nodeBoundaries)
	justification := stageBoundaryJustification(stage, stageBoundary)

	return TransactionBoundary{
		StageID:        stage.StageID,
		BoundaryClass:  stageBoundary,
		CanRetry:       stageBoundary == BoundaryAtomic || stageBoundary == BoundaryCompensatable,
		CanRollback:    stageBoundary != BoundaryIrreversible,
		CanCompensate:  stageBoundary == BoundaryCompensatable || stageBoundary == BoundaryIrreversible,
		RequiresPause:  stageBoundary == BoundaryIrreversible || stageBoundary == BoundaryOperatorConfirmRequired,
		Justification:  justification,
		NodeBoundaries: nodeBoundaries,
	}
}

// ClassifyNodeBoundary computes the transaction boundary for a single node
// by its action type and graph position.
func ClassifyNodeBoundary(nodeID string, action planner.ActionType, g *planner.PlanGraph) TransactionBoundaryClass {
	node := g.NodeIndex[nodeID]
	n := StageNode{NodeID: nodeID, Action: action}
	if node != nil {
		n.Risk = node.RiskLevel
	}
	return classifyNodeBoundaryInternal(n, g).BoundaryClass
}

// ─────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────

func classifyNodeBoundaryInternal(n StageNode, g *planner.PlanGraph) NodeBoundary {
	var class TransactionBoundaryClass
	var justification string

	node := g.NodeIndex[n.NodeID]
	hasDependents := node != nil && len(node.ReverseDependencies) > 0

	switch n.Action {
	case planner.ActionDelete:
		class = BoundaryIrreversible
		justification = "provider-side delete is irreversible in most cases; no rollback guarantee"
	case planner.ActionCreate:
		if hasDependents {
			class = BoundaryOperatorConfirmRequired
			justification = "newly created node has reverse dependencies; failure here has downstream blast radius"
		} else {
			class = BoundaryCompensatable
			justification = "create can be compensated by deleting the newly created resource"
		}
	case planner.ActionUpdate:
		if n.Risk == planner.RiskLevelCritical || n.Risk == planner.RiskLevelHigh {
			class = BoundaryOperatorConfirmRequired
			justification = "high/critical risk update; operator confirmation required before proceeding"
		} else {
			class = BoundaryCompensatable
			justification = "update can be compensated by restoring the previous configuration"
		}
	case planner.ActionRollback:
		class = BoundaryOperatorConfirmRequired
		justification = "rollback of a rollback requires operator assessment before proceeding"
	default:
		class = BoundaryAtomic
		justification = "no-op or simulate actions produce no provider-side changes; trivially atomic"
	}

	return NodeBoundary{
		NodeID:        n.NodeID,
		Action:        n.Action,
		BoundaryClass: class,
		Justification: justification,
	}
}

func mostRestrictiveBoundary(nodeBoundaries []NodeBoundary) TransactionBoundaryClass {
	rank := map[TransactionBoundaryClass]int{
		BoundaryAtomic:                  1,
		BoundaryCompensatable:           2,
		BoundaryOperatorConfirmRequired: 3,
		BoundaryIrreversible:            4,
	}
	max := 0
	result := BoundaryAtomic
	for _, nb := range nodeBoundaries {
		if r := rank[nb.BoundaryClass]; r > max {
			max = r
			result = nb.BoundaryClass
		}
	}
	return result
}

func stageBoundaryJustification(stage ExecutionStage, boundary TransactionBoundaryClass) string {
	switch boundary {
	case BoundaryIrreversible:
		return "stage contains at least one delete action — the stage as a whole is irreversible"
	case BoundaryOperatorConfirmRequired:
		return "stage contains high-risk or rollback actions requiring explicit operator confirmation before proceeding"
	case BoundaryCompensatable:
		return "stage mutations can be undone via compensation (config restore or resource deletion)"
	default:
		return "stage contains only no-ops or simulate actions; no provider state change expected"
	}
}
