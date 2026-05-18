package orchestrator

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/janlauber/one-click/pkg/planner"
)

// Phase 4.1 — ExecutionPlan and ExecutionStage.
//
// An ExecutionPlan translates a Phase 4.0 PlanGraph into a staged, ordered,
// approval-gated execution contract. It defines WHAT happens in WHAT order
// with WHAT rollback semantics. It does NOT trigger execution.

const ExecutionPlanSchemaVersion = "4.1.0"

// StageNode is one unit of work within an ExecutionStage.
type StageNode struct {
	NodeID   string             `json:"node_id"`
	NodeType string             `json:"node_type"`
	Provider string             `json:"provider"`
	Action   planner.ActionType `json:"action"`
	Risk     planner.RiskLevel  `json:"risk"`
}

// ExecutionStage is a group of nodes that can be applied together.
// Within a stage, nodes in the same ParallelGroup can execute simultaneously.
// Stages are applied in StageOrder; each stage waits for its Dependencies.
type ExecutionStage struct {
	StageID   string `json:"stage_id"`
	StageOrder int   `json:"stage_order"`
	// ParallelGroup identifies nodes within the stage that can run concurrently.
	// All nodes in the same stage share the same ParallelGroup value.
	ParallelGroup int `json:"parallel_group"`

	Nodes        []StageNode  `json:"nodes"`
	Dependencies []string     `json:"dependencies"` // stageIDs that must complete first
	Preconditions []Precondition `json:"preconditions"`

	// RollbackStageRefs are rollback-stage IDs to execute if this stage fails.
	RollbackStageRefs []string `json:"rollback_stage_refs"`
	// CompensationRefs are compensation-plan IDs for irreversible nodes.
	CompensationRefs []string `json:"compensation_refs"`

	RiskLevel            planner.RiskLevel `json:"risk_level"`
	RequiresApproval     bool              `json:"requires_approval"`
	EstimatedBlastRadius int               `json:"estimated_blast_radius"`

	// BoundaryClass classifies the transactional semantics of this stage.
	BoundaryClass TransactionBoundaryClass `json:"boundary_class"`
}

// ExecutionPlan is the immutable, deterministic orchestration plan derived
// from a Phase 4.0 planning artifact set. It is the single input to future
// operator-initiated execution tooling.
//
// AN EXECUTION PLAN NEVER EXECUTES ANYTHING.
type ExecutionPlan struct {
	SchemaVersion     string `json:"schema_version"`
	PlanHash          string `json:"plan_hash"`
	// ExecutionPlanHash is computed by ComputeExecutionPlanHash.
	ExecutionPlanHash string `json:"execution_plan_hash"`
	// GeneratedAt is excluded from all hash computations.
	GeneratedAt string      `json:"generated_at"`
	Environment planner.Environment `json:"environment"`

	// Stages is the ordered forward-execution sequence.
	Stages []ExecutionStage `json:"stages"`
	// ApprovalGates are operator sign-off requirements for the plan.
	ApprovalGates []ApprovalGate `json:"approval_gates"`
	// RollbackStages is the ordered rollback execution sequence.
	RollbackStages []ExecutionStage `json:"rollback_stages"`
	// CompensationStages covers nodes where rollback is impossible.
	CompensationStages []CompensationStage `json:"compensation_stages"`
	// ExecutionPolicies declare how execution should proceed.
	ExecutionPolicies []ExecutionPolicy `json:"execution_policies"`
	// SafetyConstraints lists active safety rules for this execution.
	SafetyConstraints []SafetyConstraint `json:"safety_constraints"`
	// EvidenceRefs from the originating DesiredState.
	EvidenceRefs []planner.EvidenceRef `json:"evidence_refs"`
	// RiskSummary aggregates risk across all stages.
	RiskSummary ExecutionRiskSummary `json:"risk_summary"`

	// Limitations documents what this execution plan cannot guarantee.
	Limitations []string `json:"limitations"`
}

// BuildExecutionPlan constructs an ExecutionPlan from planning artifacts.
// The PlanGraph must be valid (no cycles) before calling this function.
// If g.IsValid() is false, a blocked ExecutionPlan is returned.
//
// BuildExecutionPlan is a pure function: same inputs → same output.
// It NEVER contacts providers, writes to databases, or schedules execution.
func BuildExecutionPlan(
	g *planner.PlanGraph,
	cs *planner.ChangeSet,
	rg *planner.RollbackGraph,
	verdict *planner.SafetyVerdict,
	s *planner.DesiredState,
) *ExecutionPlan {
	plan := &ExecutionPlan{
		SchemaVersion: ExecutionPlanSchemaVersion,
		PlanHash:      s.DesiredStateHash, // placeholder; caller should use ComputePlanHash
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Environment:   s.Environment,
		EvidenceRefs:  s.EvidenceRefs,
		Limitations:   executionPlanLimitations(),
	}

	if !g.IsValid() {
		plan.SafetyConstraints = append(plan.SafetyConstraints, SafetyConstraint{
			ConstraintID: "cyclic-graph",
			Rule:         "CYCLIC_GRAPH",
			Severity:     "block",
			Triggered:    true,
			Reason:       fmt.Sprintf("plan graph has %d cyclic path(s); execution cannot be staged", len(g.CyclicPaths)),
		})
		plan.ExecutionPlanHash = ComputeExecutionPlanHash(
			plan.PlanHash, nil, nil, nil, nil, nil)
		return plan
	}

	// 1. Build forward execution stages from parallel stage groups.
	plan.Stages = buildForwardStages(g, cs, rg)

	// 2. Build rollback stages from the rollback graph.
	plan.RollbackStages = buildRollbackStages(rg, g)

	// 3. Wire rollback references into forward stages.
	wireRollbackRefs(plan.Stages, plan.RollbackStages, g)

	// 4. Build compensation stages for irreversible nodes.
	compPlan := BuildCompensationPlan(rg, g, cs)
	plan.CompensationStages = compPlan.Stages

	// Wire compensation refs.
	compIndex := make(map[string]string, len(plan.CompensationStages)) // nodeID → stageID
	for _, cs := range plan.CompensationStages {
		compIndex[cs.OriginalNodeID] = cs.StageID
	}
	for i := range plan.Stages {
		for _, node := range plan.Stages[i].Nodes {
			if cid, ok := compIndex[node.NodeID]; ok {
				plan.Stages[i].CompensationRefs = append(plan.Stages[i].CompensationRefs, cid)
			}
		}
		sort.Strings(plan.Stages[i].CompensationRefs)
	}

	// 5. Build approval gates.
	plan.ApprovalGates = BuildApprovalGates(plan.Stages, verdict, s)

	// 6. Build execution policies.
	plan.ExecutionPolicies = buildExecutionPolicies(s, verdict)

	// 7. Build safety constraints.
	plan.SafetyConstraints = buildSafetyConstraints(g, cs, verdict, s)

	// 8. Compute risk summary.
	plan.RiskSummary = computeRiskSummary(plan.Stages, g)

	// 9. Compute execution plan hash.
	plan.ExecutionPlanHash = ComputeExecutionPlanHash(
		plan.PlanHash,
		plan.Stages,
		plan.RollbackStages,
		plan.CompensationStages,
		plan.ApprovalGates,
		nil, // contracts built separately via BuildExecutionContracts
	)

	return plan
}

// ─────────────────────────────────────────────────────────────────────────
// STAGE BUILDERS
// ─────────────────────────────────────────────────────────────────────────

func buildForwardStages(g *planner.PlanGraph, cs *planner.ChangeSet, rg *planner.RollbackGraph) []ExecutionStage {
	if len(g.ParallelStages) == 0 {
		return nil
	}

	// Build action index from changeset.
	actionIndex := make(map[string]planner.ActionType, len(g.Nodes))
	for _, n := range g.Nodes {
		actionIndex[n.ID] = n.DesiredAction
	}

	stages := make([]ExecutionStage, 0, len(g.ParallelStages))
	for i, group := range g.ParallelStages {
		stageID := "stage-" + strconv.Itoa(i)

		nodes := make([]StageNode, 0, len(group))
		for _, nodeID := range group {
			node := g.NodeIndex[nodeID]
			if node == nil {
				continue
			}
			nodes = append(nodes, StageNode{
				NodeID:   nodeID,
				NodeType: string(node.Type),
				Provider: node.Provider,
				Action:   node.DesiredAction,
				Risk:     node.RiskLevel,
			})
		}
		// Stable sort within stage for determinism.
		sort.Slice(nodes, func(a, b int) bool { return nodes[a].NodeID < nodes[b].NodeID })

		deps := []string{}
		if i > 0 {
			deps = []string{"stage-" + strconv.Itoa(i-1)}
		}

		highest := highestRiskForNodes(nodes)
		requiresApproval := stageRequiresApproval(nodes)
		maxBlast := maxBlastRadiusForNodes(nodes, g)
		boundary := classifyStagesBoundary(nodes)

		stage := ExecutionStage{
			StageID:              stageID,
			StageOrder:           i,
			ParallelGroup:        i,
			Nodes:                nodes,
			Dependencies:         deps,
			Preconditions:        stagePreconditions(i, nodes),
			RiskLevel:            highest,
			RequiresApproval:     requiresApproval,
			EstimatedBlastRadius: maxBlast,
			BoundaryClass:        boundary,
		}
		stages = append(stages, stage)
	}
	return stages
}

func buildRollbackStages(rg *planner.RollbackGraph, g *planner.PlanGraph) []ExecutionStage {
	if len(rg.Ordering) == 0 {
		return nil
	}

	stepIndex := make(map[string]planner.RollbackStep, len(rg.Steps))
	for _, step := range rg.Steps {
		stepIndex[step.NodeID] = step
	}

	stages := make([]ExecutionStage, 0, len(rg.Ordering))
	for i, nodeID := range rg.Ordering {
		stageID := "rollback-stage-" + strconv.Itoa(i)
		node := g.NodeIndex[nodeID]
		if node == nil {
			continue
		}
		step := stepIndex[nodeID]

		action := planner.ActionType(step.ReverseAction)
		risk := node.RiskLevel
		irreversible := step.Irreversible

		boundary := BoundaryAtomic
		if irreversible {
			boundary = BoundaryIrreversible
		}

		deps := []string{}
		if i > 0 {
			deps = []string{"rollback-stage-" + strconv.Itoa(i-1)}
		}

		stages = append(stages, ExecutionStage{
			StageID:      stageID,
			StageOrder:   i,
			ParallelGroup: i,
			Nodes: []StageNode{{
				NodeID:   nodeID,
				NodeType: string(node.Type),
				Provider: node.Provider,
				Action:   action,
				Risk:     risk,
			}},
			Dependencies:     deps,
			Preconditions:    rollbackStagePreconditions(step),
			RiskLevel:        risk,
			RequiresApproval: irreversible,
			BoundaryClass:    boundary,
		})
	}
	return stages
}

// wireRollbackRefs connects each forward stage to the rollback stages for its nodes.
func wireRollbackRefs(forwardStages, rollbackStages []ExecutionStage, g *planner.PlanGraph) {
	// Build nodeID → rollback stageID index.
	nodeToRollbackStage := make(map[string]string)
	for _, rs := range rollbackStages {
		for _, n := range rs.Nodes {
			nodeToRollbackStage[n.NodeID] = rs.StageID
		}
	}
	for i := range forwardStages {
		var refs []string
		for _, n := range forwardStages[i].Nodes {
			if rsID, ok := nodeToRollbackStage[n.NodeID]; ok {
				refs = append(refs, rsID)
			}
		}
		sort.Strings(refs)
		forwardStages[i].RollbackStageRefs = refs
	}
}

// ─────────────────────────────────────────────────────────────────────────
// POLICY AND CONSTRAINT BUILDERS
// ─────────────────────────────────────────────────────────────────────────

func buildExecutionPolicies(s *planner.DesiredState, verdict *planner.SafetyVerdict) []ExecutionPolicy {
	var policies []ExecutionPolicy

	if verdict != nil && verdict.RequiresApproval {
		policies = append(policies, ExecutionPolicy{
			PolicyID:  "pause-on-approval",
			Name:      "Pause Before High-Risk Stages",
			Rule:      "pause-on-approval-required",
			AppliesTo: "all",
		})
	}
	if s.SafetyProfile.ForbidDestructiveDeletes {
		policies = append(policies, ExecutionPolicy{
			PolicyID:  "no-destructive-delete",
			Name:      "Forbid Destructive Deletes",
			Rule:      "forbid-destructive-deletes",
			AppliesTo: "all",
		})
	}
	if s.RolloutStrategy.PauseOnHighRisk {
		policies = append(policies, ExecutionPolicy{
			PolicyID:  "pause-on-high-risk",
			Name:      "Pause on High-Risk Node",
			Rule:      "pause-on-high-risk",
			AppliesTo: "all",
		})
	}
	if s.RolloutStrategy.RequireApproval {
		policies = append(policies, ExecutionPolicy{
			PolicyID:  "rollout-approval",
			Name:      "Rollout Approval Required at Each Stage",
			Rule:      "require-approval-per-stage",
			AppliesTo: "all",
		})
	}
	// Always add sequential rollback policy.
	policies = append(policies, ExecutionPolicy{
		PolicyID:  "sequential-rollback",
		Name:      "Sequential Rollback Ordering",
		Rule:      "sequential-rollback",
		AppliesTo: "all",
	})
	return policies
}

func buildSafetyConstraints(g *planner.PlanGraph, cs *planner.ChangeSet, verdict *planner.SafetyVerdict, s *planner.DesiredState) []SafetyConstraint {
	var constraints []SafetyConstraint

	if verdict != nil {
		for _, v := range verdict.Violations {
			constraints = append(constraints, SafetyConstraint{
				ConstraintID: v.Rule,
				Rule:         v.Rule,
				Severity:     v.Severity,
				Triggered:    true,
				Reason:       v.Description,
			})
		}
	}
	if cs.BlockedByConstraints {
		constraints = append(constraints, SafetyConstraint{
			ConstraintID: "plan-constraint-block",
			Rule:         "BLOCKED_BY_PLAN_CONSTRAINTS",
			Severity:     "block",
			Triggered:    true,
			Reason:       "one or more plan-level constraints are blocking execution",
		})
	}
	return constraints
}

func computeRiskSummary(stages []ExecutionStage, g *planner.PlanGraph) ExecutionRiskSummary {
	riskRank := map[planner.RiskLevel]int{
		planner.RiskLevelLow:      1,
		planner.RiskLevelMedium:   2,
		planner.RiskLevelHigh:     3,
		planner.RiskLevelCritical: 4,
	}
	summary := ExecutionRiskSummary{TotalStages: len(stages)}
	maxRank := 0
	for _, stage := range stages {
		summary.TotalNodes += len(stage.Nodes)
		if stage.RequiresApproval {
			summary.StagesRequiringApproval++
		}
		if stage.BoundaryClass == BoundaryIrreversible {
			summary.IrreversibleStageCount++
		}
		if stage.EstimatedBlastRadius > summary.MaxBlastRadiusAcrossStages {
			summary.MaxBlastRadiusAcrossStages = stage.EstimatedBlastRadius
		}
		if r := riskRank[stage.RiskLevel]; r > maxRank {
			maxRank = r
			summary.HighestRisk = stage.RiskLevel
		}
	}
	return summary
}

// ─────────────────────────────────────────────────────────────────────────
// STAGE HELPERS
// ─────────────────────────────────────────────────────────────────────────

func highestRiskForNodes(nodes []StageNode) planner.RiskLevel {
	rank := map[planner.RiskLevel]int{
		planner.RiskLevelLow:      1,
		planner.RiskLevelMedium:   2,
		planner.RiskLevelHigh:     3,
		planner.RiskLevelCritical: 4,
	}
	max := 0
	var highest planner.RiskLevel = planner.RiskLevelLow
	for _, n := range nodes {
		if r := rank[n.Risk]; r > max {
			max = r
			highest = n.Risk
		}
	}
	return highest
}

func stageRequiresApproval(nodes []StageNode) bool {
	for _, n := range nodes {
		if n.Action == planner.ActionDelete || n.Risk == planner.RiskLevelCritical || n.Risk == planner.RiskLevelHigh {
			return true
		}
	}
	return false
}

func maxBlastRadiusForNodes(nodes []StageNode, g *planner.PlanGraph) int {
	max := 0
	for _, n := range nodes {
		_, r := planner.BlastRadius(n.NodeID, g)
		if r > max {
			max = r
		}
	}
	return max
}

func classifyStagesBoundary(nodes []StageNode) TransactionBoundaryClass {
	// Highest-severity boundary wins.
	hasIrreversible := false
	hasCompensatable := false
	for _, n := range nodes {
		if n.Action == planner.ActionDelete {
			hasIrreversible = true
		} else if n.Action == planner.ActionUpdate || n.Action == planner.ActionCreate {
			hasCompensatable = true
		}
	}
	if hasIrreversible {
		return BoundaryIrreversible
	}
	if hasCompensatable {
		return BoundaryCompensatable
	}
	return BoundaryAtomic
}

func stagePreconditions(stageOrder int, nodes []StageNode) []Precondition {
	var pcs []Precondition
	if stageOrder > 0 {
		pcs = append(pcs, Precondition{
			Description: fmt.Sprintf("all nodes in stage-%d must have completed successfully", stageOrder-1),
			FailureMode: "halt execution; do not proceed to this stage",
			Blocking:    true,
		})
	}
	for _, n := range nodes {
		if n.Action == planner.ActionDelete {
			pcs = append(pcs, Precondition{
				Description: fmt.Sprintf("operator has confirmed deletion of node %s is intentional", n.NodeID),
				FailureMode: "halt; require explicit operator confirmation before deleting",
				Blocking:    true,
			})
		}
	}
	return pcs
}

func rollbackStagePreconditions(step planner.RollbackStep) []Precondition {
	pcs := []Precondition{{
		Description: "operator has explicitly initiated rollback for this node",
		FailureMode: "rollback must not auto-trigger; halt if operator approval is absent",
		Blocking:    true,
	}}
	if len(step.MustRollbackBefore) > 0 {
		pcs = append(pcs, Precondition{
			Description: fmt.Sprintf("dependent nodes %v must be rolled back before this node", step.MustRollbackBefore),
			FailureMode: "halt; dependent nodes must be rolled back first to maintain dependency ordering",
			Blocking:    true,
		})
	}
	return pcs
}

func executionPlanLimitations() []string {
	return []string{
		"execution plan defines intent only — it does not guarantee provider availability or capacity",
		"stage timing estimates are not enforced; actual execution durations may vary significantly",
		"provider-side state may change between plan export and operator-initiated execution",
		"rollback stages are plans, not guarantees — provider restore may not be available for all resource types",
		"compensation strategies are advisory; manual intervention may be required for provider-ambiguous states",
		"no distributed coordination — concurrent execution attempts are not prevented by this plan",
		"image digest changes between plan time and execution time are not detectable",
		"approval gates must be satisfied by operator action before execution proceeds; they are not auto-enforced",
	}
}
