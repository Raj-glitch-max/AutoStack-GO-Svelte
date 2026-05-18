package orchestrator

import (
	"fmt"

	"github.com/janlauber/one-click/pkg/planner"
)

// Phase 4.1 — Execution contracts.
//
// An ExecutionContract defines, per node, what success means, what failure
// means, what ambiguous state means, what rollback means, and what
// compensation means. Contracts are advisory planning artifacts — they do
// not enforce provider behavior.

// ExecutionContract is the complete behavioral specification for one node's
// planned action. Every actionable node must have a contract before
// operator-initiated execution can proceed.
type ExecutionContract struct {
	NodeID   string             `json:"node_id"`
	NodeType string             `json:"node_type"`
	Provider string             `json:"provider"`

	// IntendedAction is what the execution engine should apply.
	IntendedAction planner.ActionType `json:"intended_action"`

	// Preconditions must be satisfied before applying the action.
	Preconditions []Precondition `json:"preconditions"`

	// Postconditions must be satisfied after a successful application.
	Postconditions []Postcondition `json:"postconditions"`

	// RollbackConditions describe when and how to roll back this node.
	RollbackConditions []RollbackCondition `json:"rollback_conditions"`

	// CompensationStrategy describes the recovery path when rollback is impossible.
	CompensationStrategy CompensationStrategy `json:"compensation_strategy"`

	// FailureClassification classifies how a failure of this node should be handled.
	FailureClassification FailureClass `json:"failure_classification"`

	// TimeoutClass estimates how long the action is expected to take.
	TimeoutClass TimeoutClass `json:"timeout_class"`

	// IdempotencyRequirements describes whether the action can be safely retried.
	IdempotencyRequirements IdempotencyRequirements `json:"idempotency_requirements"`

	// AmbiguousStateDefinition describes what provider-ambiguous state looks like
	// for this action and what the operator should do if it occurs.
	AmbiguousStateDefinition string `json:"ambiguous_state_definition"`

	// Limitations lists what the contract cannot guarantee.
	Limitations []string `json:"limitations"`
}

// BuildExecutionContracts produces one ExecutionContract per node in the
// PlanGraph. Contracts are deterministic: same graph + changeset → same contracts.
func BuildExecutionContracts(g *planner.PlanGraph, cs *planner.ChangeSet) []ExecutionContract {
	if !g.IsValid() {
		return nil
	}

	// Action index from changeset.
	changeIndex := make(map[string]planner.ChangeItem)
	all := append(append(append(append(
		cs.Creates, cs.Updates...), cs.Deletes...), cs.Rollbacks...), cs.NoOps...)
	for _, item := range all {
		changeIndex[item.NodeID] = item
	}

	contracts := make([]ExecutionContract, 0, len(g.TopologicalOrder))
	for _, id := range g.TopologicalOrder {
		node := g.NodeIndex[id]
		item, hasItem := changeIndex[id]
		action := node.DesiredAction
		if hasItem {
			action = item.Action
		}

		fc := ClassifyFailure(action, node.RiskLevel)
		contract := ExecutionContract{
			NodeID:                  id,
			NodeType:                string(node.Type),
			Provider:                node.Provider,
			IntendedAction:          action,
			Preconditions:           contractPreconditions(id, action, node),
			Postconditions:          contractPostconditions(action, node.RiskLevel),
			RollbackConditions:      contractRollbackConditions(id, action),
			CompensationStrategy:    compensationStrategyForAction(action, node.RiskLevel),
			FailureClassification:   fc,
			TimeoutClass:            timeoutClassForAction(action, node.RiskLevel),
			IdempotencyRequirements: idempotencyForAction(action),
			AmbiguousStateDefinition: ambiguousStateDefinition(action),
			Limitations:             contractLimitations(action),
		}
		contracts = append(contracts, contract)
	}
	return contracts
}

// ─────────────────────────────────────────────────────────────────────────
// CONTRACT HELPERS
// ─────────────────────────────────────────────────────────────────────────

func contractPreconditions(nodeID string, action planner.ActionType, node *planner.PlanNode) []Precondition {
	var pcs []Precondition

	// All actions require dependency readiness.
	if len(node.Dependencies) > 0 {
		pcs = append(pcs, Precondition{
			Description: fmt.Sprintf("all dependencies of %s (%v) must be in a healthy state", nodeID, node.Dependencies),
			FailureMode: "halt; dependencies not ready",
			Blocking:    true,
		})
	}

	switch action {
	case planner.ActionCreate:
		pcs = append(pcs, Precondition{
			Description: fmt.Sprintf("node %s must not exist in provider state (create would fail otherwise)", nodeID),
			FailureMode: "halt; check for existing resource before applying create",
			Blocking:    true,
		})
	case planner.ActionUpdate:
		pcs = append(pcs, Precondition{
			Description: fmt.Sprintf("node %s must exist in provider state (update requires existing resource)", nodeID),
			FailureMode: "halt; resource not found — cannot apply update",
			Blocking:    true,
		})
	case planner.ActionDelete:
		pcs = append(pcs, Precondition{
			Description: fmt.Sprintf("operator has explicitly confirmed deletion of node %s", nodeID),
			FailureMode: "halt; explicit operator confirmation required",
			Blocking:    true,
		})
		pcs = append(pcs, Precondition{
			Description: fmt.Sprintf("no active provider resources depend on node %s at execution time", nodeID),
			FailureMode: "warn; dependent resources may enter a broken state if this node is deleted first",
			Blocking:    false,
		})
	case planner.ActionRollback:
		pcs = append(pcs, Precondition{
			Description: fmt.Sprintf("a valid previous-state snapshot exists for node %s", nodeID),
			FailureMode: "halt; cannot rollback without a snapshot of prior state",
			Blocking:    true,
		})
	}
	return pcs
}

func contractPostconditions(action planner.ActionType, risk planner.RiskLevel) []Postcondition {
	switch action {
	case planner.ActionCreate:
		return []Postcondition{
			{
				Description:  "resource exists in provider state and is in a healthy/running status",
				VerifyMethod: "poll-status",
				TimeoutClass: TimeoutModerate,
			},
		}
	case planner.ActionUpdate:
		return []Postcondition{
			{
				Description:  "resource reflects the new desired spec in provider state",
				VerifyMethod: "poll-status",
				TimeoutClass: timeoutClassForAction(action, risk),
			},
		}
	case planner.ActionDelete:
		return []Postcondition{
			{
				Description:  "resource no longer exists in provider state",
				VerifyMethod: "poll-status",
				TimeoutClass: TimeoutModerate,
			},
		}
	case planner.ActionRollback:
		return []Postcondition{
			{
				Description:  "resource reflects the previous-state spec in provider state",
				VerifyMethod: "poll-status",
				TimeoutClass: TimeoutSlow,
			},
			{
				Description:  "operator has confirmed rollback outcome is acceptable",
				VerifyMethod: "operator-confirm",
				TimeoutClass: TimeoutVerySlow,
			},
		}
	default:
		return []Postcondition{{
			Description:  "no change applied; resource state is unchanged",
			VerifyMethod: "poll-status",
			TimeoutClass: TimeoutFast,
		}}
	}
}

func contractRollbackConditions(nodeID string, action planner.ActionType) []RollbackCondition {
	if action == planner.ActionDelete {
		// Delete is irreversible; rollback condition is still defined but marked
		// as requiring manual intervention.
		return []RollbackCondition{{
			TriggerDescription: fmt.Sprintf("node %s was deleted — rollback requires provider backup restore", nodeID),
			RollbackStageRef:   "",    // no rollback stage; requires manual action
			AutoTrigger:        false, // never auto-trigger
		}}
	}
	return []RollbackCondition{{
		TriggerDescription: fmt.Sprintf("node %s failed to reach postconditions within timeout", nodeID),
		RollbackStageRef:   "rollback-stage-for-" + nodeID,
		AutoTrigger:        false, // operator must initiate rollback
	}}
}

func compensationStrategyForAction(action planner.ActionType, risk planner.RiskLevel) CompensationStrategy {
	switch action {
	case planner.ActionDelete:
		return CompensationRecreate
	case planner.ActionUpdate:
		if risk == planner.RiskLevelHigh || risk == planner.RiskLevelCritical {
			return CompensationRestoreConfig
		}
		return CompensationRestoreConfig
	case planner.ActionCreate:
		return CompensationQuarantine
	case planner.ActionRollback:
		return CompensationManualIntervention
	default:
		return CompensationNone
	}
}

func timeoutClassForAction(action planner.ActionType, risk planner.RiskLevel) TimeoutClass {
	switch action {
	case planner.ActionDelete:
		return TimeoutModerate
	case planner.ActionCreate:
		if risk == planner.RiskLevelHigh || risk == planner.RiskLevelCritical {
			return TimeoutSlow
		}
		return TimeoutModerate
	case planner.ActionUpdate:
		if risk == planner.RiskLevelHigh || risk == planner.RiskLevelCritical {
			return TimeoutSlow
		}
		return TimeoutModerate
	case planner.ActionRollback:
		return TimeoutSlow
	case planner.ActionNoOp, planner.ActionSimulate:
		return TimeoutFast
	default:
		return TimeoutModerate
	}
}

func idempotencyForAction(action planner.ActionType) IdempotencyRequirements {
	switch action {
	case planner.ActionCreate:
		return IdempotencyRequirements{
			IsIdempotent: false,
			Caveat:       "create may fail if the resource already exists; verify before retrying",
		}
	case planner.ActionUpdate:
		return IdempotencyRequirements{
			IsIdempotent: true,
			Caveat:       "update is idempotent if the desired spec is unchanged; re-applying the same spec is safe",
		}
	case planner.ActionDelete:
		return IdempotencyRequirements{
			IsIdempotent: true,
			Caveat:       "delete is idempotent only if the provider treats delete-of-non-existent as success",
		}
	case planner.ActionRollback:
		return IdempotencyRequirements{
			IsIdempotent: false,
			Caveat:       "rollback is not idempotent; applying rollback twice may leave the resource in an undefined state",
		}
	default:
		return IdempotencyRequirements{IsIdempotent: true}
	}
}

func ambiguousStateDefinition(action planner.ActionType) string {
	switch action {
	case planner.ActionCreate:
		return "resource is partially initialized: provider acknowledges the request but has not reported healthy status within the timeout window; operator should verify provider state before retrying"
	case planner.ActionUpdate:
		return "resource is partially updated: provider applied some spec fields but not others; operator should compare desired spec against provider state before retrying"
	case planner.ActionDelete:
		return "resource deletion is in progress but not confirmed: provider acknowledged the request but resource is not yet absent from provider state; operator should wait and verify before assuming success or failure"
	case planner.ActionRollback:
		return "rollback is partially applied: previous spec may be partially restored; operator must manually verify provider state before declaring success or initiating compensation"
	default:
		return "no mutation applied; provider state should be unchanged; verify if uncertain"
	}
}

func contractLimitations(action planner.ActionType) []string {
	lims := []string{
		"contract cannot guarantee provider API availability at execution time",
		"contract cannot predict provider-side concurrent mutations",
	}
	if action == planner.ActionDelete {
		lims = append(lims, "provider-side delete is irreversible in most cases — no restore guarantee without a backup")
	}
	if action == planner.ActionRollback {
		lims = append(lims, "rollback requires a valid previous-state snapshot; if no snapshot exists, rollback is impossible")
	}
	return lims
}
