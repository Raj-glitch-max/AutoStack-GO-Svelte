package orchestrator

import (
	"fmt"
	"strconv"

	"github.com/janlauber/one-click/pkg/planner"
)

// Phase 4.1 — Compensation planning.
//
// When rollback is impossible (e.g., a delete has already occurred),
// the compensation model describes the set of actions that can restore
// a safe operational state. Compensation is advisory — it is never
// automatically triggered.

// CompensationStage describes a compensating action for one node where
// rollback is not possible.
type CompensationStage struct {
	StageID        string `json:"stage_id"`
	StageOrder     int    `json:"stage_order"`
	OriginalNodeID string `json:"original_node_id"`
	OriginalAction planner.ActionType `json:"original_action"`

	// CompensationAction describes the recovery approach.
	CompensationAction CompensationStrategy `json:"compensation_action"`

	// Reason explains why compensation is needed instead of rollback.
	Reason string `json:"reason"`

	// ManualRequired is true when no automated compensation path exists.
	// An operator must physically assess and remediate the provider state.
	ManualRequired bool `json:"manual_required"`

	// Limitations documents what this compensation cannot guarantee.
	Limitations []string `json:"limitations"`
}

// CompensationPlan is the complete set of compensating actions for a plan.
type CompensationPlan struct {
	// Stages are the compensating stages, ordered for safe execution.
	Stages []CompensationStage `json:"stages"`

	// HasManualRequired is true when at least one stage requires manual operator action.
	HasManualRequired bool `json:"has_manual_required"`

	// Limitations documents what the compensation plan cannot guarantee.
	Limitations []string `json:"limitations"`
}

// BuildCompensationPlan derives a CompensationPlan for all nodes that are
// flagged as irreversible in the RollbackGraph. Nodes that CAN be rolled
// back do not need compensation stages.
//
// BuildCompensationPlan is a pure function: same inputs → same output.
func BuildCompensationPlan(rg *planner.RollbackGraph, g *planner.PlanGraph, cs *planner.ChangeSet) *CompensationPlan {
	plan := &CompensationPlan{
		Limitations: compensationPlanLimitations(),
	}

	if len(rg.IrreversibleNodes) == 0 {
		return plan
	}

	// Build action index.
	actionIndex := make(map[string]planner.ActionType)
	for _, n := range g.Nodes {
		actionIndex[n.ID] = n.DesiredAction
	}

	for i, nodeID := range rg.IrreversibleNodes {
		stageID := "compensation-stage-" + strconv.Itoa(i)
		node := g.NodeIndex[nodeID]
		action := actionIndex[nodeID]

		strategy := compensationStrategyForIrreversible(action, node)
		manualRequired := strategy == CompensationManualIntervention

		stage := CompensationStage{
			StageID:            stageID,
			StageOrder:         i,
			OriginalNodeID:     nodeID,
			OriginalAction:     action,
			CompensationAction: strategy,
			Reason:             compensationReason(action, nodeID),
			ManualRequired:     manualRequired,
			Limitations:        compensationStageLimitations(strategy),
		}
		plan.Stages = append(plan.Stages, stage)
		if manualRequired {
			plan.HasManualRequired = true
		}
	}
	return plan
}

// ─────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────

func compensationStrategyForIrreversible(action planner.ActionType, node *planner.PlanNode) CompensationStrategy {
	switch action {
	case planner.ActionDelete:
		// Deletes are the primary irreversible actions.
		if node != nil && len(node.ReverseDependencies) > 0 {
			// Other nodes depend on this deleted node — quarantine them to prevent cascade.
			return CompensationQuarantine
		}
		return CompensationRecreate
	case planner.ActionRollback:
		return CompensationManualIntervention
	default:
		return CompensationManualIntervention
	}
}

func compensationReason(action planner.ActionType, nodeID string) string {
	switch action {
	case planner.ActionDelete:
		return fmt.Sprintf("node %s was deleted — provider-side delete is irreversible without a backup restore pipeline; compensation is required to restore safe state", nodeID)
	case planner.ActionRollback:
		return fmt.Sprintf("node %s rollback failed — rollback of a rollback cannot be automated; manual operator intervention required", nodeID)
	default:
		return fmt.Sprintf("node %s action is irreversible — automated compensation is not available; operator must assess provider state", nodeID)
	}
}

func compensationStageLimitations(strategy CompensationStrategy) []string {
	base := []string{
		"compensation is advisory — it is never automatically triggered",
		"compensation does not guarantee provider availability or capacity",
	}
	switch strategy {
	case CompensationRecreate:
		return append(base,
			"recreate requires the original desired spec to be available; if the spec was modified after deletion, the recreated resource may differ",
			"recreate does not restore provider-side data (databases, volumes, etc.)",
		)
	case CompensationQuarantine:
		return append(base,
			"quarantine isolates dependent resources but does not restore the deleted node",
			"dependent resources may enter a degraded state during quarantine",
		)
	case CompensationManualIntervention:
		return append(base,
			"no automated compensation path exists; operator must physically assess and remediate provider state",
		)
	default:
		return base
	}
}

func compensationPlanLimitations() []string {
	return []string{
		"compensation plan assumes the provider supports the compensation action type",
		"compensation is never automatically triggered — operator must explicitly initiate",
		"compensation does not guarantee restoration of provider-side data (databases, volumes, etc.)",
		"compensation strategies are derived from action type and risk — they may not match the actual provider state at compensation time",
	}
}
