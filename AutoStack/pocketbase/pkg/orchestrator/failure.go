package orchestrator

import (
	"github.com/janlauber/one-click/pkg/planner"
)

// Phase 4.1 — Failure classification model.
//
// Failure classification is deterministic: same action + risk level → same class.
// No ML, no adaptive learning, no guessing. The classification is a pure function
// of the planning-time information available.
//
// ─────────────────────────────────────────────────────────────────────────
// FAILURE CLASS DECISION TREE
// ─────────────────────────────────────────────────────────────────────────
//
//   ActionDelete           → Irreversible (delete cannot be undone)
//   ActionRollback + High  → OperatorRequired (rollback of rollback needs human)
//   ActionCreate + Critical→ ProviderAmbiguous (critical creates may partially succeed)
//   ActionCreate           → Retryable (creates are usually retryable)
//   ActionUpdate + Critical→ RollbackRequired (must restore previous state on failure)
//   ActionUpdate + High    → RollbackRequired
//   ActionUpdate + Medium  → Compensatable
//   ActionUpdate + Low     → Retryable
//   ActionRollback         → RollbackRequired (standard rollback failure)
//   ActionNoOp             → Transient (no-op cannot meaningfully fail)
//   default                → ProviderAmbiguous

// FailureModel is the per-node failure classification derived from planning data.
type FailureModel struct {
	NodeID                string             `json:"node_id"`
	Action                planner.ActionType `json:"action"`
	Risk                  planner.RiskLevel  `json:"risk"`
	FailureClass          FailureClass       `json:"failure_class"`
	HandlingGuidance      string             `json:"handling_guidance"`
	// CanAutoRecover is always false in Phase 4.1. Auto-recovery is forbidden.
	CanAutoRecover        bool               `json:"can_auto_recover"`
	RecommendedNextAction string             `json:"recommended_next_action"`
}

// ClassifyFailure returns the deterministic FailureClass for an action at a
// given risk level. Same inputs always produce the same classification.
func ClassifyFailure(action planner.ActionType, risk planner.RiskLevel) FailureClass {
	switch action {
	case planner.ActionDelete:
		return FailureClassIrreversible

	case planner.ActionCreate:
		switch risk {
		case planner.RiskLevelCritical:
			return FailureClassProviderAmbiguous
		default:
			return FailureClassRetryable
		}

	case planner.ActionUpdate:
		switch risk {
		case planner.RiskLevelCritical, planner.RiskLevelHigh:
			return FailureClassRollbackRequired
		case planner.RiskLevelMedium:
			return FailureClassCompensatable
		default:
			return FailureClassRetryable
		}

	case planner.ActionRollback:
		switch risk {
		case planner.RiskLevelCritical, planner.RiskLevelHigh:
			return FailureClassOperatorRequired
		default:
			return FailureClassRollbackRequired
		}

	case planner.ActionNoOp, planner.ActionSimulate:
		return FailureClassTransient

	default:
		return FailureClassProviderAmbiguous
	}
}

// BuildFailureModels produces one FailureModel per node in the PlanGraph.
// Models are produced in topological order for determinism.
func BuildFailureModels(g *planner.PlanGraph, cs *planner.ChangeSet) []FailureModel {
	if !g.IsValid() {
		return nil
	}
	models := make([]FailureModel, 0, len(g.TopologicalOrder))
	for _, id := range g.TopologicalOrder {
		node := g.NodeIndex[id]
		fc := ClassifyFailure(node.DesiredAction, node.RiskLevel)
		models = append(models, FailureModel{
			NodeID:                id,
			Action:                node.DesiredAction,
			Risk:                  node.RiskLevel,
			FailureClass:          fc,
			HandlingGuidance:      failureHandlingGuidance(fc, node.DesiredAction),
			CanAutoRecover:        false, // never in Phase 4.1
			RecommendedNextAction: recommendedNextAction(fc),
		})
	}
	return models
}

// ─────────────────────────────────────────────────────────────────────────
// GUIDANCE HELPERS
// ─────────────────────────────────────────────────────────────────────────

func failureHandlingGuidance(fc FailureClass, action planner.ActionType) string {
	switch fc {
	case FailureClassTransient:
		return "monitor provider state; failure may resolve without intervention"
	case FailureClassRetryable:
		return "retry the action after verifying provider state is unchanged; ensure idempotency conditions are met"
	case FailureClassCompensatable:
		return "apply the compensation strategy to restore a safe state; verify compensation outcome before continuing"
	case FailureClassRollbackRequired:
		return "initiate rollback for this node before any further execution; do not proceed to the next stage until rollback is confirmed"
	case FailureClassIrreversible:
		return "action is irreversible — assess provider state manually; if resource must be restored, use the backup restore pipeline"
	case FailureClassProviderAmbiguous:
		return "provider state is unknown — do NOT retry automatically; verify actual provider state before deciding on next action"
	case FailureClassOperatorRequired:
		return "human operator must assess the failure and explicitly choose the next action; do not auto-recover"
	default:
		return "consult operator for guidance; failure class is unknown"
	}
}

func recommendedNextAction(fc FailureClass) string {
	switch fc {
	case FailureClassTransient:
		return "wait and monitor; retry if failure persists beyond provider SLA"
	case FailureClassRetryable:
		return "retry after confirming provider state is consistent with pre-action state"
	case FailureClassCompensatable:
		return "execute compensation stage; confirm compensation outcome with operator"
	case FailureClassRollbackRequired:
		return "execute rollback stage for this node; await operator confirmation of rollback success"
	case FailureClassIrreversible:
		return "halt execution; operator must assess provider state and initiate manual recovery"
	case FailureClassProviderAmbiguous:
		return "halt execution; operator must verify actual provider state before proceeding"
	case FailureClassOperatorRequired:
		return "halt execution; await explicit operator decision"
	default:
		return "halt execution; consult operator"
	}
}
