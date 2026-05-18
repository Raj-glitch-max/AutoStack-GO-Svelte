package orchestrator

import (
	"fmt"
	"sort"

	"github.com/janlauber/one-click/pkg/planner"
)

// Phase 4.1 — Execution-layer safety evaluation.
//
// EvaluateExecutionSafety operates at the orchestration layer — above the
// planner's safety evaluation. It detects execution-specific hazards that
// are invisible at the planning layer:
//
//   1. EXCESSIVE_BLAST_RADIUS: stage-level blast radius exceeds the limit.
//   2. CASCADING_ROLLBACK_RISK: rollback of one stage triggers rollback of many.
//   3. DEPENDENCY_COLLAPSE: deleting a node that other stages depend on.
//   4. UNSAFE_PARALLEL: two parallel nodes have a hidden ordering constraint.
//   5. IRREVERSIBLE_CHAIN: a sequence of irreversible stages with no pause.
//   6. CROSS_PROVIDER_RISK: dependency chain spans multiple cloud providers.
//   7. UNSAFE_COMPENSATION: compensation strategy does not match provider type.

// ExecutionSafetyResult is the output of EvaluateExecutionSafety.
type ExecutionSafetyResult struct {
	// IsBlocked is true when at least one blocking issue was found.
	IsBlocked bool `json:"is_blocked"`

	// Issues lists all detected safety issues.
	Issues []ExecutionSafetyIssue `json:"issues"`

	// Warnings lists non-blocking safety concerns.
	Warnings []ExecutionSafetyWarning `json:"warnings"`

	// Limitations documents what this safety check cannot detect.
	Limitations []string `json:"limitations"`
}

// ExecutionSafetyIssue is one detected hazard that may block execution.
type ExecutionSafetyIssue struct {
	Rule        string `json:"rule"`
	Severity    string `json:"severity"` // "block" | "pause"
	StageID     string `json:"stage_id,omitempty"`
	NodeID      string `json:"node_id,omitempty"`
	Description string `json:"description"`
}

// ExecutionSafetyWarning is a non-blocking safety concern.
type ExecutionSafetyWarning struct {
	Rule        string `json:"rule"`
	StageID     string `json:"stage_id,omitempty"`
	NodeID      string `json:"node_id,omitempty"`
	Description string `json:"description"`
}

// EvaluateExecutionSafety runs the orchestration-layer safety checks over an
// ExecutionPlan and the underlying PlanGraph.
//
// EvaluateExecutionSafety is a pure function: same inputs → same result.
// It NEVER contacts providers or mutates state.
func EvaluateExecutionSafety(plan *ExecutionPlan, g *planner.PlanGraph, s *planner.DesiredState) *ExecutionSafetyResult {
	result := &ExecutionSafetyResult{
		Limitations: executionSafetyLimitations(),
	}

	if plan == nil || g == nil {
		result.Issues = append(result.Issues, ExecutionSafetyIssue{
			Rule:        "NULL_INPUT",
			Severity:    "block",
			Description: "execution plan or plan graph is nil; cannot evaluate safety",
		})
		result.IsBlocked = true
		return result
	}

	// Rule 1: EXCESSIVE_BLAST_RADIUS.
	if s != nil && s.SafetyProfile.MaxBlastRadiusNodes > 0 {
		for _, stage := range plan.Stages {
			if stage.EstimatedBlastRadius > s.SafetyProfile.MaxBlastRadiusNodes {
				result.Issues = append(result.Issues, ExecutionSafetyIssue{
					Rule:        "EXCESSIVE_BLAST_RADIUS",
					Severity:    "pause",
					StageID:     stage.StageID,
					Description: fmt.Sprintf("stage %s has blast radius %d, exceeding configured maximum of %d", stage.StageID, stage.EstimatedBlastRadius, s.SafetyProfile.MaxBlastRadiusNodes),
				})
			}
		}
	}

	// Rule 2: CASCADING_ROLLBACK_RISK.
	if len(plan.RollbackStages) > 3 {
		cascadeCount := len(plan.RollbackStages)
		result.Warnings = append(result.Warnings, ExecutionSafetyWarning{
			Rule:        "CASCADING_ROLLBACK_RISK",
			Description: fmt.Sprintf("rollback chain has %d stages — a failure during rollback itself could cascade further; operator must be prepared for complex rollback management", cascadeCount),
		})
	}

	// Rule 3: DEPENDENCY_COLLAPSE — deleting a node that other stages depend on.
	deleteNodeSet := make(map[string]bool)
	for _, stage := range plan.Stages {
		for _, node := range stage.Nodes {
			if node.Action == planner.ActionDelete {
				deleteNodeSet[node.NodeID] = true
			}
		}
	}
	for delNodeID := range deleteNodeSet {
		node := g.NodeIndex[delNodeID]
		if node == nil {
			continue
		}
		if len(node.ReverseDependencies) > 0 {
			result.Issues = append(result.Issues, ExecutionSafetyIssue{
				Rule:        "DEPENDENCY_COLLAPSE",
				Severity:    "pause",
				NodeID:      delNodeID,
				Description: fmt.Sprintf("node %s is scheduled for deletion but has %d reverse dependencies %v — deleting it collapses dependent services; operator must confirm dependency migration before proceeding", delNodeID, len(node.ReverseDependencies), node.ReverseDependencies),
			})
		}
	}

	// Rule 4: UNSAFE_PARALLEL — two parallel nodes in the same stage where one
	// is a delete and the other depends on the deleted node.
	for _, stage := range plan.Stages {
		stageNodeSet := make(map[string]bool)
		for _, n := range stage.Nodes {
			stageNodeSet[n.NodeID] = true
		}
		for _, n := range stage.Nodes {
			if n.Action == planner.ActionDelete {
				node := g.NodeIndex[n.NodeID]
				if node == nil {
					continue
				}
				for _, revDep := range node.ReverseDependencies {
					if stageNodeSet[revDep] {
						result.Issues = append(result.Issues, ExecutionSafetyIssue{
							Rule:        "UNSAFE_PARALLEL",
							Severity:    "block",
							StageID:     stage.StageID,
							NodeID:      n.NodeID,
							Description: fmt.Sprintf("node %s (delete) and its dependent %s are in the same parallel stage %s — this is unsafe; the dependent must be handled before the delete", n.NodeID, revDep, stage.StageID),
						})
					}
				}
			}
		}
	}

	// Rule 5: IRREVERSIBLE_CHAIN — three or more consecutive irreversible stages
	// with no approval gate between them.
	consecutiveIrreversible := 0
	for i, stage := range plan.Stages {
		if stage.BoundaryClass == BoundaryIrreversible {
			consecutiveIrreversible++
			if consecutiveIrreversible >= 3 {
				result.Issues = append(result.Issues, ExecutionSafetyIssue{
					Rule:        "IRREVERSIBLE_CHAIN",
					Severity:    "pause",
					StageID:     plan.Stages[i].StageID,
					Description: fmt.Sprintf("at least 3 consecutive irreversible stages detected ending at %s — no recovery path exists if execution fails mid-chain; operator must add approval gates between irreversible stages", plan.Stages[i].StageID),
				})
				break
			}
		} else {
			consecutiveIrreversible = 0
		}
	}

	// Rule 6: CROSS_PROVIDER_RISK — dependency chain crosses provider boundaries.
	crossProviderPairs := detectCrossProviderDependencies(g)
	for _, pair := range crossProviderPairs {
		result.Warnings = append(result.Warnings, ExecutionSafetyWarning{
			Rule:        "CROSS_PROVIDER_RISK",
			NodeID:      pair[0],
			Description: fmt.Sprintf("node %s (provider: %s) depends on node %s (provider: %s) — cross-provider dependencies have no transactional guarantee; a failure on one provider cannot be atomically rolled back on the other", pair[0], pair[1], pair[2], pair[3]),
		})
	}

	// Rule 7: UNSAFE_COMPENSATION — irreversible node with only manual-intervention compensation.
	for _, stage := range plan.Stages {
		for _, node := range stage.Nodes {
			if node.Action == planner.ActionDelete && len(plan.CompensationStages) > 0 {
				for _, cs := range plan.CompensationStages {
					if cs.OriginalNodeID == node.NodeID && cs.ManualRequired {
						result.Warnings = append(result.Warnings, ExecutionSafetyWarning{
							Rule:        "MANUAL_COMPENSATION_REQUIRED",
							StageID:     stage.StageID,
							NodeID:      node.NodeID,
							Description: fmt.Sprintf("node %s will be irreversibly deleted and its compensation requires manual operator intervention — ensure a responsible operator is available before proceeding", node.NodeID),
						})
					}
				}
			}
		}
	}

	// Determine blocking status.
	for _, issue := range result.Issues {
		if issue.Severity == "block" || issue.Severity == "pause" {
			result.IsBlocked = true
			break
		}
	}

	// Sort issues and warnings for determinism.
	sort.Slice(result.Issues, func(i, j int) bool {
		if result.Issues[i].Rule != result.Issues[j].Rule {
			return result.Issues[i].Rule < result.Issues[j].Rule
		}
		return result.Issues[i].StageID < result.Issues[j].StageID
	})
	sort.Slice(result.Warnings, func(i, j int) bool {
		return result.Warnings[i].Rule < result.Warnings[j].Rule
	})

	return result
}

// ─────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────

// detectCrossProviderDependencies returns pairs of [fromID, fromProvider, toID, toProvider]
// for each edge that crosses provider boundaries.
func detectCrossProviderDependencies(g *planner.PlanGraph) [][4]string {
	var pairs [][4]string
	seen := make(map[string]bool)
	for _, edge := range g.Edges {
		fromNode := g.NodeIndex[edge.From]
		toNode := g.NodeIndex[edge.To]
		if fromNode == nil || toNode == nil {
			continue
		}
		if fromNode.Provider == "" || toNode.Provider == "" {
			continue
		}
		if fromNode.Provider == toNode.Provider {
			continue
		}
		key := edge.From + "|" + edge.To
		if seen[key] {
			continue
		}
		seen[key] = true
		pairs = append(pairs, [4]string{edge.From, fromNode.Provider, edge.To, toNode.Provider})
	}
	// Sort for determinism.
	sort.Slice(pairs, func(i, j int) bool { return pairs[i][0] < pairs[j][0] })
	return pairs
}

func executionSafetyLimitations() []string {
	return []string{
		"execution safety evaluation operates on plan structure only — actual provider state is not polled",
		"cross-provider dependency deadlocks cannot be detected without provider-side state; graph-level cycle detection covers circular deps only",
		"rate limits, quota exhaustion, and provider-side concurrency limits are not modeled",
		"blast radius estimates assume sequential failure propagation; simultaneous failures across multiple stages are not modeled",
		"compensation strategy validity is not verified against provider capabilities — the provider may not support the suggested compensation action",
	}
}
