package orchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// Phase 4.1 — Execution plan hash engine.
//
// ComputeExecutionPlanHash produces a deterministic, content-addressable
// hash of the execution orchestration plan. The hash is a function of:
//
//   - The upstream PlanHash (from Phase 4.0)
//   - Stage ordering (stageID:order tuples, sorted)
//   - Rollback stage ordering
//   - Compensation stage summaries
//   - Approval gate summaries
//   - Execution contract summaries (nodeID:action:failureClass)
//
// ─────────────────────────────────────────────────────────────────────────
// DESIGN INVARIANTS
// ─────────────────────────────────────────────────────────────────────────
//
//   1. DETERMINISTIC: same inputs → byte-identical hash, always.
//   2. NO TIMESTAMPS: GeneratedAt is excluded.
//   3. NO RANDOM: no UUID or entropy injection.
//   4. STABLE ORDERING: all slices are sorted before hashing.
//   5. NO PROVIDER CALLS: pure function over already-computed structures.

// ExecutionHashInput is the canonical, stable structure that feeds the hash.
// All slices are sorted before serialization.
type ExecutionHashInput struct {
	PlanHash             string   `json:"plan_hash"`
	StageTuples          []string `json:"stage_tuples"`            // "stageID:order:boundary" sorted
	RollbackStageTuples  []string `json:"rollback_stage_tuples"`   // sorted
	CompensationTuples   []string `json:"compensation_tuples"`     // "nodeID:strategy" sorted
	ApprovalGateTuples   []string `json:"approval_gate_tuples"`    // "gateID:stageID:role" sorted
	ContractTuples       []string `json:"contract_tuples"`         // "nodeID:action:failureClass" sorted
}

// ComputeExecutionPlanHash returns a deterministic SHA-256 hex hash of the
// execution plan's key orchestration decisions. Identical inputs always
// produce an identical hash.
func ComputeExecutionPlanHash(
	planHash string,
	stages []ExecutionStage,
	rollbackStages []ExecutionStage,
	compensationStages []CompensationStage,
	approvalGates []ApprovalGate,
	contracts []ExecutionContract,
) string {
	input := buildExecutionHashInput(planHash, stages, rollbackStages, compensationStages, approvalGates, contracts)
	payload, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// buildExecutionHashInput constructs the canonical ExecutionHashInput. All
// slices are sorted to guarantee order-independent determinism.
func buildExecutionHashInput(
	planHash string,
	stages []ExecutionStage,
	rollbackStages []ExecutionStage,
	compensationStages []CompensationStage,
	approvalGates []ApprovalGate,
	contracts []ExecutionContract,
) ExecutionHashInput {
	// Stage tuples.
	stageTuples := make([]string, 0, len(stages))
	for _, s := range stages {
		stageTuples = append(stageTuples, s.StageID+":"+itoa(s.StageOrder)+":"+string(s.BoundaryClass))
	}
	sort.Strings(stageTuples)

	// Rollback stage tuples.
	rollbackTuples := make([]string, 0, len(rollbackStages))
	for _, s := range rollbackStages {
		rollbackTuples = append(rollbackTuples, s.StageID+":"+itoa(s.StageOrder))
	}
	sort.Strings(rollbackTuples)

	// Compensation tuples.
	compTuples := make([]string, 0, len(compensationStages))
	for _, cs := range compensationStages {
		compTuples = append(compTuples, cs.OriginalNodeID+":"+string(cs.CompensationAction))
	}
	sort.Strings(compTuples)

	// Approval gate tuples.
	gateTuples := make([]string, 0, len(approvalGates))
	for _, g := range approvalGates {
		gateTuples = append(gateTuples, g.GateID+":"+g.StageID+":"+g.RequiredRole)
	}
	sort.Strings(gateTuples)

	// Contract tuples.
	contractTuples := make([]string, 0, len(contracts))
	for _, c := range contracts {
		contractTuples = append(contractTuples, c.NodeID+":"+string(c.IntendedAction)+":"+string(c.FailureClassification))
	}
	sort.Strings(contractTuples)

	return ExecutionHashInput{
		PlanHash:            planHash,
		StageTuples:         stageTuples,
		RollbackStageTuples: rollbackTuples,
		CompensationTuples:  compTuples,
		ApprovalGateTuples:  gateTuples,
		ContractTuples:      contractTuples,
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	b := make([]byte, 0, 10)
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
