package planner

import "fmt"

// Phase 4.0 — Safety policy layer.
//
// SafetyPolicyLayer evaluates a plan against a set of safety rules and
// returns a verdict. It NEVER mutates infrastructure. It NEVER calls a
// provider. It NEVER schedules execution.
//
// The safety layer is the final gate before a plan can be approved by an
// operator. Approval by the safety layer means:
//   - The plan is structurally valid (no cycles, no unknown nodes).
//   - The plan passes all configured safety rules.
//   - Operator approval is still required before execution.
//
// ─────────────────────────────────────────────────────────────────────────
// SAFETY RULES (evaluated in order)
// ─────────────────────────────────────────────────────────────────────────
//
//   1. DRY_RUN_ONLY: if SafetyProfile.DryRunOnly, plan cannot be executed.
//   2. CYCLIC_GRAPH: cyclic dependency graph blocks all execution.
//   3. DESTRUCTIVE_DELETE_FORBIDDEN: if SafetyProfile.ForbidDestructiveDeletes,
//      any delete action blocks the plan.
//   4. BLAST_RADIUS_EXCEEDED: if any node's blast radius exceeds
//      SafetyProfile.MaxBlastRadiusNodes, the plan is paused.
//   5. HIGH_RISK_APPROVAL_REQUIRED: if any action exceeds
//      SafetyProfile.RequireApprovalAbove, operator approval is required.
//   6. BLOCKED_BY_CONSTRAINTS: if ChangeSet.BlockedByConstraints, plan is blocked.
//   7. FORBIDDEN_ACTION_CATEGORY: specific action types may be declared forbidden.

// SafetyVerdict is the result of evaluating a plan through the safety layer.
type SafetyVerdict struct {
	// Approved is true when no blocking safety rule was triggered.
	// Approved does NOT mean the plan can be executed autonomously —
	// operator approval is always required before execution.
	Approved bool `json:"approved"`

	// RequiresApproval is true when one or more high-risk or approval-gated
	// actions are present. An approved plan with RequiresApproval=true
	// MUST receive explicit operator sign-off before execution.
	RequiresApproval bool `json:"requires_approval"`

	// DryRunOnly is true when the SafetyProfile mandates simulation-only output.
	DryRunOnly bool `json:"dry_run_only"`

	// Violations lists each safety rule that was triggered.
	Violations []SafetyViolation `json:"violations"`

	// Warnings lists non-blocking safety concerns.
	Warnings []SafetyWarning `json:"warnings"`

	// ApprovalGates lists the specific nodes or conditions that require
	// explicit operator approval before proceeding.
	ApprovalGates []ApprovalGate `json:"approval_gates"`

	// ForbiddenActions lists action types that are forbidden by the
	// current safety profile and appear in this plan.
	ForbiddenActions []ForbiddenAction `json:"forbidden_actions"`
}

// SafetyViolation is one triggered safety rule that blocks the plan.
type SafetyViolation struct {
	Rule        string `json:"rule"`
	Severity    string `json:"severity"` // "block" | "pause"
	Description string `json:"description"`
	NodeID      string `json:"node_id,omitempty"`
}

// SafetyWarning is a non-blocking safety concern.
type SafetyWarning struct {
	Rule        string `json:"rule"`
	Description string `json:"description"`
	NodeID      string `json:"node_id,omitempty"`
}

// ApprovalGate describes one condition that requires explicit operator sign-off.
type ApprovalGate struct {
	NodeID      string `json:"node_id"`
	Reason      string `json:"reason"`
	RiskLevel   string `json:"risk_level"`
	ActionType  string `json:"action_type"`
}

// ForbiddenAction describes a planned action that is forbidden by policy.
type ForbiddenAction struct {
	NodeID     string `json:"node_id"`
	Action     string `json:"action"`
	PolicyRule string `json:"policy_rule"`
}

// EvaluateSafety runs the safety policy layer over a plan. It returns a
// SafetyVerdict describing whether the plan can proceed, what approvals are
// needed, and what violations were found.
//
// EvaluateSafety is a pure function: same inputs → same verdict.
// It NEVER triggers execution and NEVER contacts a provider.
func EvaluateSafety(g *PlanGraph, cs *ChangeSet, s *DesiredState, sim *SimulationResult) *SafetyVerdict {
	v := &SafetyVerdict{}

	// Rule 1: DRY_RUN_ONLY.
	if s.SafetyProfile.DryRunOnly {
		v.DryRunOnly = true
		v.Violations = append(v.Violations, SafetyViolation{
			Rule:        "DRY_RUN_ONLY",
			Severity:    "block",
			Description: "safety profile is set to dry-run-only; this plan cannot be executed — simulation output only",
		})
	}

	// Rule 2: CYCLIC_GRAPH.
	if !g.IsValid() {
		v.Violations = append(v.Violations, SafetyViolation{
			Rule:        "CYCLIC_GRAPH",
			Severity:    "block",
			Description: fmt.Sprintf("plan graph has %d cyclic dependency path(s); execution is impossible", len(g.CyclicPaths)),
		})
	}

	// Rule 3: DESTRUCTIVE_DELETE_FORBIDDEN.
	if s.SafetyProfile.ForbidDestructiveDeletes {
		for _, item := range cs.Deletes {
			v.Violations = append(v.Violations, SafetyViolation{
				Rule:        "DESTRUCTIVE_DELETE_FORBIDDEN",
				Severity:    "block",
				Description: fmt.Sprintf("safety profile forbids destructive deletes; node %s has action=delete", item.NodeID),
				NodeID:      item.NodeID,
			})
			v.ForbiddenActions = append(v.ForbiddenActions, ForbiddenAction{
				NodeID:     item.NodeID,
				Action:     string(ActionDelete),
				PolicyRule: "ForbidDestructiveDeletes",
			})
		}
	}

	// Rule 4: BLAST_RADIUS_EXCEEDED.
	if s.SafetyProfile.MaxBlastRadiusNodes > 0 && sim != nil {
		for _, entry := range sim.BlastRadiusMap {
			if entry.Radius > s.SafetyProfile.MaxBlastRadiusNodes {
				v.Violations = append(v.Violations, SafetyViolation{
					Rule:        "BLAST_RADIUS_EXCEEDED",
					Severity:    "pause",
					Description: fmt.Sprintf("node %s has blast radius %d, exceeding configured maximum of %d", entry.NodeID, entry.Radius, s.SafetyProfile.MaxBlastRadiusNodes),
					NodeID:      entry.NodeID,
				})
				v.RequiresApproval = true
			}
		}
	}

	// Rule 5: HIGH_RISK_APPROVAL_REQUIRED.
	if s.SafetyProfile.RequireApprovalAbove != "" {
		threshold := riskLevelRank(RiskLevel(s.SafetyProfile.RequireApprovalAbove))
		allActionable := append(append(append(cs.Creates, cs.Updates...), cs.Deletes...), cs.Rollbacks...)
		for _, item := range allActionable {
			if riskLevelRank(item.RiskEstimate) > threshold {
				v.RequiresApproval = true
				v.ApprovalGates = append(v.ApprovalGates, ApprovalGate{
					NodeID:     item.NodeID,
					Reason:     fmt.Sprintf("risk level %q exceeds approval threshold %q", item.RiskEstimate, s.SafetyProfile.RequireApprovalAbove),
					RiskLevel:  string(item.RiskEstimate),
					ActionType: string(item.Action),
				})
			}
		}
	}

	// Rule 5b: RolloutStrategy.RequireApproval.
	if s.RolloutStrategy.RequireApproval {
		v.RequiresApproval = true
		v.Warnings = append(v.Warnings, SafetyWarning{
			Rule:        "ROLLOUT_APPROVAL_REQUIRED",
			Description: "rollout strategy mandates operator approval at each stage",
		})
	}

	// Rule 5c: PauseOnHighRisk.
	if s.RolloutStrategy.PauseOnHighRisk {
		allActionable := append(append(append(cs.Creates, cs.Updates...), cs.Deletes...), cs.Rollbacks...)
		for _, item := range allActionable {
			if item.RiskEstimate == RiskLevelHigh || item.RiskEstimate == RiskLevelCritical {
				v.RequiresApproval = true
				v.ApprovalGates = append(v.ApprovalGates, ApprovalGate{
					NodeID:     item.NodeID,
					Reason:     "rollout strategy pause-on-high-risk triggered; operator must approve before this node is applied",
					RiskLevel:  string(item.RiskEstimate),
					ActionType: string(item.Action),
				})
			}
		}
	}

	// Rule 6: BLOCKED_BY_CONSTRAINTS.
	if cs.BlockedByConstraints {
		v.Violations = append(v.Violations, SafetyViolation{
			Rule:        "BLOCKED_BY_CONSTRAINTS",
			Severity:    "block",
			Description: "one or more blocking constraints prevent plan execution; resolve constraint violations before proceeding",
		})
	}

	// Rule 7: ALLOWED_PROVIDERS.
	if len(s.SafetyProfile.AllowedProviders) > 0 {
		allowedSet := make(map[string]bool, len(s.SafetyProfile.AllowedProviders))
		for _, p := range s.SafetyProfile.AllowedProviders {
			allowedSet[p] = true
		}
		for _, node := range g.Nodes {
			if node.Provider != "" && !allowedSet[node.Provider] {
				v.Violations = append(v.Violations, SafetyViolation{
					Rule:        "PROVIDER_NOT_ALLOWED",
					Severity:    "block",
					Description: fmt.Sprintf("node %s uses provider %q which is not in the allowed provider list", node.ID, node.Provider),
					NodeID:      node.ID,
				})
			}
		}
	}

	// Warnings: orphan nodes.
	for _, id := range g.OrphanNodes {
		v.Warnings = append(v.Warnings, SafetyWarning{
			Rule:        "ORPHAN_NODE",
			Description: fmt.Sprintf("node %s has no dependency edges — verify this isolated resource is intentional", id),
			NodeID:      id,
		})
	}

	// Warnings: irreversible nodes without explicit approval gates.
	for _, item := range cs.Deletes {
		alreadyGated := false
		for _, gate := range v.ApprovalGates {
			if gate.NodeID == item.NodeID {
				alreadyGated = true
				break
			}
		}
		if !alreadyGated {
			v.Warnings = append(v.Warnings, SafetyWarning{
				Rule:        "IRREVERSIBLE_ACTION",
				Description: fmt.Sprintf("node %s will be deleted — this action is irreversible without a provider backup restore pipeline", item.NodeID),
				NodeID:      item.NodeID,
			})
		}
	}

	// Determine overall approval: approved when no blocking violations.
	v.Approved = !hasBlockingViolation(v.Violations)

	return v
}

// ─────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────

func riskLevelRank(r RiskLevel) int {
	switch r {
	case RiskLevelLow:
		return 1
	case RiskLevelMedium:
		return 2
	case RiskLevelHigh:
		return 3
	case RiskLevelCritical:
		return 4
	default:
		return 0
	}
}

func hasBlockingViolation(violations []SafetyViolation) bool {
	for _, v := range violations {
		if v.Severity == "block" {
			return true
		}
	}
	return false
}
