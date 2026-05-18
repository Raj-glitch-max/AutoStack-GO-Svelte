package orchestrator

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/janlauber/one-click/pkg/planner"
)

// Phase 4.1 — Approval gate system.
//
// Approval gates are operator sign-off requirements that must be satisfied
// before execution of a stage can proceed. They are generated deterministically
// from stage risk levels, safety verdicts, and rollout strategy.
//
// Gates are NEVER automatically bypassed. Approval is always human-initiated.

// ApprovalGate is a mandatory operator checkpoint before a stage can execute.
// It differs from the planner-layer ApprovalGate: this one is stage-scoped,
// has a role requirement, and carries an explicit blocking flag.
type ApprovalGate struct {
	// GateID is a stable, unique identifier for this gate.
	GateID string `json:"gate_id"`

	// StageID is the execution stage this gate protects.
	StageID string `json:"stage_id"`

	// Reason explains why operator approval is required.
	Reason string `json:"reason"`

	// RequiredRole is the minimum role that can approve this gate.
	// "operator" | "admin" | "security-team"
	RequiredRole string `json:"required_role"`

	// RiskThreshold is the risk level that triggered this gate.
	RiskThreshold string `json:"risk_threshold"`

	// AffectedNodes lists the node IDs this gate protects.
	AffectedNodes []string `json:"affected_nodes"`

	// Blocking is true when execution cannot proceed without gate approval.
	// Always true in Phase 4.1.
	Blocking bool `json:"blocking"`
}

// BuildApprovalGates derives all operator checkpoints from the execution
// stages, planner safety verdict, and desired state rollout strategy.
//
// BuildApprovalGates is deterministic: same inputs → same gates.
func BuildApprovalGates(stages []ExecutionStage, verdict *planner.SafetyVerdict, s *planner.DesiredState) []ApprovalGate {
	var gates []ApprovalGate
	gateCounter := 0

	for _, stage := range stages {
		// Gate: stage itself requires approval (high/critical risk or delete present).
		if stage.RequiresApproval {
			gateID := "gate-" + strconv.Itoa(gateCounter)
			gateCounter++
			affectedNodes := nodeIDsForStage(stage)
			gates = append(gates, ApprovalGate{
				GateID:        gateID,
				StageID:       stage.StageID,
				Reason:        approvalReason(stage),
				RequiredRole:  requiredRoleForRisk(stage.RiskLevel),
				RiskThreshold: string(stage.RiskLevel),
				AffectedNodes: affectedNodes,
				Blocking:      true,
			})
		}

		// Gate: stage has irreversible actions (delete nodes).
		for _, node := range stage.Nodes {
			if node.Action == planner.ActionDelete {
				gateID := "gate-" + strconv.Itoa(gateCounter)
				gateCounter++
				gates = append(gates, ApprovalGate{
					GateID:        gateID,
					StageID:       stage.StageID,
					Reason:        fmt.Sprintf("node %s has action=delete — irreversible without backup restore pipeline; explicit operator confirmation required", node.NodeID),
					RequiredRole:  "admin",
					RiskThreshold: string(planner.RiskLevelCritical),
					AffectedNodes: []string{node.NodeID},
					Blocking:      true,
				})
			}
		}
	}

	// Gates from planner safety verdict.
	if verdict != nil {
		for i, pg := range verdict.ApprovalGates {
			gateID := "gate-planner-" + strconv.Itoa(i)
			// Find the stage for this node.
			stageID := stageIDForNode(pg.NodeID, stages)
			gates = append(gates, ApprovalGate{
				GateID:        gateID,
				StageID:       stageID,
				Reason:        pg.Reason,
				RequiredRole:  requiredRoleForRisk(planner.RiskLevel(pg.RiskLevel)),
				RiskThreshold: pg.RiskLevel,
				AffectedNodes: []string{pg.NodeID},
				Blocking:      true,
			})
		}
	}

	// Deduplicate gates protecting the same (stageID, nodeID) pair.
	gates = deduplicateGates(gates)

	// Sort for determinism.
	sort.Slice(gates, func(i, j int) bool {
		if gates[i].StageID != gates[j].StageID {
			return gates[i].StageID < gates[j].StageID
		}
		return gates[i].GateID < gates[j].GateID
	})

	return gates
}

// ─────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────

func approvalReason(stage ExecutionStage) string {
	switch stage.RiskLevel {
	case planner.RiskLevelCritical:
		return fmt.Sprintf("stage %s has critical-risk nodes — explicit operator approval required before proceeding", stage.StageID)
	case planner.RiskLevelHigh:
		return fmt.Sprintf("stage %s has high-risk nodes — operator confirmation required before applying changes", stage.StageID)
	default:
		if stage.BoundaryClass == BoundaryIrreversible {
			return fmt.Sprintf("stage %s contains irreversible actions — operator must confirm before proceeding", stage.StageID)
		}
		return fmt.Sprintf("stage %s requires approval per rollout strategy or safety profile", stage.StageID)
	}
}

func requiredRoleForRisk(risk planner.RiskLevel) string {
	switch risk {
	case planner.RiskLevelCritical:
		return "admin"
	case planner.RiskLevelHigh:
		return "operator"
	default:
		return "operator"
	}
}

func nodeIDsForStage(stage ExecutionStage) []string {
	ids := make([]string, len(stage.Nodes))
	for i, n := range stage.Nodes {
		ids[i] = n.NodeID
	}
	sort.Strings(ids)
	return ids
}

func stageIDForNode(nodeID string, stages []ExecutionStage) string {
	for _, s := range stages {
		for _, n := range s.Nodes {
			if n.NodeID == nodeID {
				return s.StageID
			}
		}
	}
	return "unknown-stage"
}

func deduplicateGates(gates []ApprovalGate) []ApprovalGate {
	seen := make(map[string]bool)
	result := make([]ApprovalGate, 0, len(gates))
	for _, g := range gates {
		key := g.StageID + "|" + g.Reason
		if !seen[key] {
			seen[key] = true
			result = append(result, g)
		}
	}
	return result
}
