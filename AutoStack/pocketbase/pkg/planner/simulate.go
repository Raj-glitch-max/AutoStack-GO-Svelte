package planner

import (
	"fmt"
	"sort"
)

// Phase 4.0 — Simulation engine.
//
// SimulatePlan produces a deterministic prediction of what a plan would do
// WITHOUT contacting any provider and WITHOUT mutating any infrastructure.
//
// ─────────────────────────────────────────────────────────────────────────
// ABSOLUTE CONSTRAINTS
// ─────────────────────────────────────────────────────────────────────────
//
//   - SimulatePlan NEVER calls a provider API.
//   - SimulatePlan NEVER writes to any database.
//   - SimulatePlan NEVER triggers or schedules a mutation.
//   - SimulatePlan NEVER assumes provider-side consistency.
//
// The simulation output is advisory. Actual outcomes may differ because:
//   - Provider-side state may change between simulation and execution.
//   - Rate limits and quota exhaustion are not modeled.
//   - Network partitions and transient errors are not modeled.
//   - Same-tag/different-digest container images produce identical hashes.

// SimulationResult is the complete, read-only output of SimulatePlan.
type SimulationResult struct {
	// DesiredStateHash is the hash of the DesiredState that was simulated.
	DesiredStateHash string `json:"desired_state_hash"`

	// PlanGraphSummary is a brief description of the planned graph structure.
	PlanGraphSummary PlanGraphSummary `json:"plan_graph_summary"`

	// PredictedActions is the ordered list of predicted actions in apply order.
	PredictedActions []PredictedAction `json:"predicted_actions"`

	// RiskSummary is the aggregated risk profile across all predicted actions.
	RiskSummary RiskSummary `json:"risk_summary"`

	// RollbackChain describes the rollback sequence if the plan were aborted.
	RollbackChain []RollbackStep `json:"rollback_chain"`

	// BlastRadiusMap maps each node to the set of nodes affected if it fails.
	BlastRadiusMap []BlastRadiusEntry `json:"blast_radius_map"`

	// ConfidenceScore is a 0–100 integer representing the planner's overall
	// confidence in the simulation output. Penalized for high-risk actions,
	// irreversible steps, constraint violations, and orphan nodes.
	ConfidenceScore int `json:"confidence_score"`

	// AmbiguityWarnings lists situations where the planner has incomplete
	// information and cannot predict outcomes with high confidence.
	AmbiguityWarnings []string `json:"ambiguity_warnings"`

	// ConstraintViolations lists constraint violations detected during simulation.
	ConstraintViolations []SimulatedConstraintViolation `json:"constraint_violations"`

	// ParallelStages describes which nodes can be applied simultaneously.
	ParallelStages [][]string `json:"parallel_stages"`

	// IrreversibleNodes lists node IDs with irreversible actions.
	IrreversibleNodes []string `json:"irreversible_nodes"`

	// SimulationLimitations documents what this simulation cannot predict.
	SimulationLimitations []string `json:"simulation_limitations"`

	// IsBlocked is true when a blocking constraint or safety rule prevents
	// plan execution even if operator-approved.
	IsBlocked bool `json:"is_blocked"`

	// BlockedReason explains why IsBlocked is true, when applicable.
	BlockedReason string `json:"blocked_reason,omitempty"`
}

// PlanGraphSummary describes the PlanGraph at a glance.
type PlanGraphSummary struct {
	TotalNodes     int      `json:"total_nodes"`
	TotalEdges     int      `json:"total_edges"`
	OrphanNodes    []string `json:"orphan_nodes"`
	HasCycles      bool     `json:"has_cycles"`
	CyclicPaths    [][]string `json:"cyclic_paths,omitempty"`
	ParallelStages int      `json:"parallel_stage_count"`
}

// PredictedAction is the simulation's prediction for one node's planned action.
type PredictedAction struct {
	NodeID          string          `json:"node_id"`
	NodeType        string          `json:"node_type"`
	Action          ActionType      `json:"action"`
	Confidence      ConfidenceLevel `json:"confidence"`
	Risk            RiskLevel       `json:"risk"`
	BlastRadius     int             `json:"blast_radius"`
	IsIrreversible  bool            `json:"is_irreversible"`
	RollbackImplication string      `json:"rollback_implication"`
	AmbiguityNote   string          `json:"ambiguity_note,omitempty"`
}

// RiskSummary aggregates risk across all predicted actions.
type RiskSummary struct {
	HighestRisk         RiskLevel `json:"highest_risk"`
	CriticalActionCount int       `json:"critical_action_count"`
	HighActionCount     int       `json:"high_action_count"`
	MediumActionCount   int       `json:"medium_action_count"`
	LowActionCount      int       `json:"low_action_count"`
	IrreversibleCount   int       `json:"irreversible_count"`
	MaxBlastRadius      int       `json:"max_blast_radius"`
}

// BlastRadiusEntry maps a node to all nodes affected if it fails.
type BlastRadiusEntry struct {
	NodeID        string   `json:"node_id"`
	AffectedNodes []string `json:"affected_nodes"`
	Radius        int      `json:"radius"`
}

// SimulatedConstraintViolation describes one constraint violated by the plan.
type SimulatedConstraintViolation struct {
	NodeID       string `json:"node_id"`
	ConstraintID string `json:"constraint_id"`
	Severity     string `json:"severity"`
	Description  string `json:"description"`
}

// SimulatePlan produces a full simulation of the given plan graph and change
// set WITHOUT mutating any provider or persisted state. The simulation is
// derived entirely from the already-built PlanGraph and ChangeSet.
//
// Callers MUST call g.IsValid() before SimulatePlan; simulating a cyclic
// graph returns a blocked result with the cycle paths noted.
func SimulatePlan(g *PlanGraph, cs *ChangeSet, s *DesiredState) *SimulationResult {
	result := &SimulationResult{
		DesiredStateHash:      s.DesiredStateHash,
		SimulationLimitations: simulationLimitations(),
		ParallelStages:        g.ParallelStages,
	}

	// Summarize graph structure.
	result.PlanGraphSummary = PlanGraphSummary{
		TotalNodes:     len(g.Nodes),
		TotalEdges:     len(g.Edges),
		OrphanNodes:    g.OrphanNodes,
		HasCycles:      !g.IsValid(),
		CyclicPaths:    g.CyclicPaths,
		ParallelStages: len(g.ParallelStages),
	}

	// Cyclic graph — simulation cannot proceed.
	if !g.IsValid() {
		result.IsBlocked = true
		result.BlockedReason = fmt.Sprintf("plan graph has %d cyclic dependency path(s); resolve cycles before simulating", len(g.CyclicPaths))
		result.ConfidenceScore = 0
		result.AmbiguityWarnings = append(result.AmbiguityWarnings, "cyclic dependencies prevent topological ordering — plan cannot be executed")
		return result
	}

	// Build constraint index for violation lookup.
	constraintIndex := make(map[string]ConstraintSpec, len(s.Constraints))
	for _, c := range s.Constraints {
		constraintIndex[c.ID] = c
	}

	// Process nodes in topological apply order.
	allChanges := changesByNodeID(cs)
	var riskCounts [4]int // indexed by risk rank: 0=low,1=med,2=high,3=crit
	maxBlast := 0

	for _, id := range g.TopologicalOrder {
		node := g.NodeIndex[id]
		item, hasItem := allChanges[id]

		action := node.DesiredAction
		if hasItem {
			action = item.Action
		}

		conf := confidenceForAction(action, node)
		risk := node.RiskLevel
		irreversible := action == ActionDelete
		if irreversible {
			result.IrreversibleNodes = append(result.IrreversibleNodes, id)
		}

		affected, radius := BlastRadius(id, g)
		if radius > maxBlast {
			maxBlast = radius
		}

		entry := BlastRadiusEntry{
			NodeID:        id,
			AffectedNodes: affected,
			Radius:        radius,
		}
		result.BlastRadiusMap = append(result.BlastRadiusMap, entry)

		rollbackNote := ""
		if hasItem {
			rollbackNote = item.RollbackImplication
		} else {
			rollbackNote = rollbackImplication(node)
		}

		pred := PredictedAction{
			NodeID:              id,
			NodeType:            string(node.Type),
			Action:              action,
			Confidence:          conf,
			Risk:                risk,
			BlastRadius:         radius,
			IsIrreversible:      irreversible,
			RollbackImplication: rollbackNote,
			AmbiguityNote:       ambiguityNote(node, action),
		}
		result.PredictedActions = append(result.PredictedActions, pred)

		// Tally risk counts.
		switch risk {
		case RiskLevelLow:
			riskCounts[0]++
		case RiskLevelMedium:
			riskCounts[1]++
		case RiskLevelHigh:
			riskCounts[2]++
		case RiskLevelCritical:
			riskCounts[3]++
		}

		// Collect constraint violations for actionable nodes.
		if hasItem {
			for _, cid := range item.ConstraintViolations {
				spec, ok := constraintIndex[cid]
				if !ok {
					continue
				}
				result.ConstraintViolations = append(result.ConstraintViolations, SimulatedConstraintViolation{
					NodeID:       id,
					ConstraintID: cid,
					Severity:     spec.Severity,
					Description:  spec.Description,
				})
				if spec.Severity == "block" {
					result.IsBlocked = true
					result.BlockedReason = fmt.Sprintf("blocking constraint %q on node %s: %s", cid, id, spec.Description)
				}
			}
		}
	}

	// Aggregate risk summary.
	result.RiskSummary = RiskSummary{
		HighestRisk:         cs.HighestRisk,
		LowActionCount:      riskCounts[0],
		MediumActionCount:   riskCounts[1],
		HighActionCount:     riskCounts[2],
		CriticalActionCount: riskCounts[3],
		IrreversibleCount:   len(result.IrreversibleNodes),
		MaxBlastRadius:      maxBlast,
	}

	// Build rollback chain (reverse of topological order, from BuildRollbackGraph).
	rg := BuildRollbackGraph(cs, g)
	result.RollbackChain = rg.Steps

	// Sort irreversible nodes for determinism.
	sort.Strings(result.IrreversibleNodes)

	// Collect ambiguity warnings.
	result.AmbiguityWarnings = append(result.AmbiguityWarnings, collectAmbiguityWarnings(g, cs, s)...)
	sort.Strings(result.AmbiguityWarnings)

	// Compute confidence score (0–100).
	result.ConfidenceScore = computeConfidenceScore(result)

	// Check safety profile constraints.
	if s.SafetyProfile.ForbidDestructiveDeletes && len(result.IrreversibleNodes) > 0 {
		result.IsBlocked = true
		result.BlockedReason = fmt.Sprintf("safety profile forbids destructive deletes; %d irreversible node(s) present", len(result.IrreversibleNodes))
	}
	if s.SafetyProfile.MaxBlastRadiusNodes > 0 && maxBlast > s.SafetyProfile.MaxBlastRadiusNodes {
		result.IsBlocked = true
		result.BlockedReason = fmt.Sprintf("blast radius %d exceeds safety profile limit of %d", maxBlast, s.SafetyProfile.MaxBlastRadiusNodes)
	}

	return result
}

// ─────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────

func changesByNodeID(cs *ChangeSet) map[string]ChangeItem {
	all := append(append(append(append(cs.Creates, cs.Updates...), cs.Deletes...), cs.Rollbacks...), cs.NoOps...)
	idx := make(map[string]ChangeItem, len(all))
	for _, item := range all {
		idx[item.NodeID] = item
	}
	return idx
}

func ambiguityNote(node *PlanNode, action ActionType) string {
	switch {
	case node.IsOrphan:
		return "orphan node: no dependency edges; verify this is intentional"
	case action == ActionUpdate:
		return "spec divergence assumed from DesiredState intent; actual provider state not polled"
	case action == ActionDelete:
		return "provider-side delete is irreversible in most providers; verify before applying"
	default:
		return ""
	}
}

func collectAmbiguityWarnings(g *PlanGraph, cs *ChangeSet, s *DesiredState) []string {
	var warnings []string

	if len(g.OrphanNodes) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d orphan node(s) detected: %v — verify these are intentional isolated resources", len(g.OrphanNodes), g.OrphanNodes))
	}
	if cs.HasConstraintViolations {
		warnings = append(warnings, "one or more constraint violations detected; operator review required before execution")
	}
	if cs.BlockedByConstraints {
		warnings = append(warnings, "blocking constraint(s) present — plan execution is prevented until constraints are resolved")
	}
	if s.SafetyProfile.DryRunOnly {
		warnings = append(warnings, "safety profile is set to dry-run-only; this plan cannot be executed, only simulated")
	}
	// Count deletes.
	if len(cs.Deletes) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d destructive delete(s) in plan — these are irreversible without a provider backup restore pipeline", len(cs.Deletes)))
	}
	warnings = append(warnings, "image digest comparison is not supported — tag equality is the only image identity check; same-tag/different-digest drift is undetectable")
	warnings = append(warnings, "simulation does not predict provider rate limits, quota exhaustion, or transient failures")
	return warnings
}

func computeConfidenceScore(r *SimulationResult) int {
	score := 100

	// Penalize for critical/high risk actions.
	score -= r.RiskSummary.CriticalActionCount * 15
	score -= r.RiskSummary.HighActionCount * 8

	// Penalize for irreversible nodes.
	score -= r.RiskSummary.IrreversibleCount * 10

	// Penalize for constraint violations.
	score -= len(r.ConstraintViolations) * 5

	// Penalize for orphan nodes.
	score -= len(r.PlanGraphSummary.OrphanNodes) * 3

	// Penalize for blocked plan.
	if r.IsBlocked {
		score -= 20
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

func simulationLimitations() []string {
	return []string{
		"simulation operates on DesiredState intent only — actual provider state is not polled and may differ",
		"image digest comparison is not supported — tag equality is the only image identity check",
		"provider rate limits, quota exhaustion, and transient errors are not modeled",
		"partial success states (some nodes applied, others failed) are not modeled — simulation assumes all-or-nothing",
		"network partition scenarios are not modeled",
		"provider-side side effects (IAM propagation delay, DNS TTL, CDN cache) are not modeled",
		"simulation cannot predict provider-specific behavior for concurrent mutations",
		"blast radius estimates assume sequential failure propagation — parallel failure modes are not modeled",
	}
}
