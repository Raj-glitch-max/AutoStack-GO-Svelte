package planner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// Phase 4.0 — Plan hash engine.
//
// ComputePlanHash produces a deterministic, content-addressable hash for a
// complete plan. The hash is a function of:
//
//   - DesiredState content (via DesiredStateHash)
//   - PlanGraph topology (nodes + edges + topological order)
//   - ChangeSet action decisions
//   - Provider observation fingerprints (passed as stable strings)
//   - Drift record IDs (passed as sorted strings)
//   - Policy IDs from DesiredState
//   - Constraint IDs from DesiredState
//
// ─────────────────────────────────────────────────────────────────────────
// DESIGN INVARIANTS
// ─────────────────────────────────────────────────────────────────────────
//
//   1. DETERMINISTIC: same inputs → byte-identical hash, always.
//   2. NO TIMESTAMPS: GeneratedAt and wall-clock fields are excluded.
//   3. NO RANDOM: no UUID generation, no entropy injection.
//   4. STABLE ORDERING: all slices are sorted before hashing.
//   5. NO PROVIDER CALLS: pure function over already-computed structures.

// PlanHashInput is the canonical, sorted structure that feeds the hash.
// All fields are stable strings; slices are sorted before serialization.
type PlanHashInput struct {
	DesiredStateHash string   `json:"desired_state_hash"`
	NodeIDs          []string `json:"node_ids"`          // sorted
	EdgeTuples       []string `json:"edge_tuples"`       // "from→to" sorted
	TopologicalOrder []string `json:"topological_order"` // preserves planning order
	ActionTuples     []string `json:"action_tuples"`     // "nodeID:action" sorted
	RiskTuples       []string `json:"risk_tuples"`       // "nodeID:risk" sorted
	PolicyIDs        []string `json:"policy_ids"`        // sorted
	ConstraintIDs    []string `json:"constraint_ids"`    // sorted
	ObsFingerprints  []string `json:"obs_fingerprints"`  // sorted; caller-supplied
	DriftRecordIDs   []string `json:"drift_record_ids"`  // sorted; caller-supplied
}

// ComputePlanHash produces the plan hash from a PlanGraph, ChangeSet, and
// DesiredState. Caller may supply optional provider observation fingerprints
// and drift record IDs from the reconciler layer; pass nil to omit.
//
// The hash is deterministic: identical inputs → byte-identical hex string.
func ComputePlanHash(
	g *PlanGraph,
	cs *ChangeSet,
	s *DesiredState,
	obsFingerprints []string,
	driftRecordIDs []string,
) string {
	input := buildHashInput(g, cs, s, obsFingerprints, driftRecordIDs)
	payload, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// buildHashInput constructs the canonical PlanHashInput. All slices are
// sorted to guarantee order-independent determinism.
func buildHashInput(
	g *PlanGraph,
	cs *ChangeSet,
	s *DesiredState,
	obsFingerprints []string,
	driftRecordIDs []string,
) PlanHashInput {
	// Node IDs.
	nodeIDs := make([]string, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		nodeIDs = append(nodeIDs, n.ID)
	}
	sort.Strings(nodeIDs)

	// Edge tuples: "from→to".
	edgeTuples := make([]string, 0, len(g.Edges))
	for _, e := range g.Edges {
		edgeTuples = append(edgeTuples, e.From+"→"+e.To)
	}
	sort.Strings(edgeTuples)

	// Action tuples from all ChangeSet categories.
	allItems := append(append(append(append(
		cs.Creates, cs.Updates...), cs.Deletes...), cs.Rollbacks...), cs.NoOps...)
	actionTuples := make([]string, 0, len(allItems))
	riskTuples := make([]string, 0, len(allItems))
	for _, item := range allItems {
		actionTuples = append(actionTuples, item.NodeID+":"+string(item.Action))
		riskTuples = append(riskTuples, item.NodeID+":"+string(item.RiskEstimate))
	}
	sort.Strings(actionTuples)
	sort.Strings(riskTuples)

	// Policy IDs.
	policyIDs := make([]string, 0, len(s.Policies))
	for _, p := range s.Policies {
		policyIDs = append(policyIDs, p.ID)
	}
	sort.Strings(policyIDs)

	// Constraint IDs.
	constraintIDs := make([]string, 0, len(s.Constraints))
	for _, c := range s.Constraints {
		constraintIDs = append(constraintIDs, c.ID)
	}
	sort.Strings(constraintIDs)

	// Caller-supplied observation fingerprints and drift IDs.
	sortedObs := sortedStringsCopy(obsFingerprints)
	sortedDrifts := sortedStringsCopy(driftRecordIDs)

	return PlanHashInput{
		DesiredStateHash: s.DesiredStateHash,
		NodeIDs:          nodeIDs,
		EdgeTuples:       edgeTuples,
		TopologicalOrder: g.TopologicalOrder, // order matters here — preserves planning topology
		ActionTuples:     actionTuples,
		RiskTuples:       riskTuples,
		PolicyIDs:        policyIDs,
		ConstraintIDs:    constraintIDs,
		ObsFingerprints:  sortedObs,
		DriftRecordIDs:   sortedDrifts,
	}
}

// sortedStringsCopy returns a sorted copy of ss, or an empty slice when nil.
func sortedStringsCopy(ss []string) []string {
	if len(ss) == 0 {
		return []string{}
	}
	out := make([]string, len(ss))
	copy(out, ss)
	sort.Strings(out)
	return out
}
