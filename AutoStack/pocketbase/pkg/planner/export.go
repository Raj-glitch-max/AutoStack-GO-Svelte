package planner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Phase 4.0 — Plan snapshot export.
//
// A PlanSnapshot is the complete, self-contained, deterministic export of a
// plan. It bundles:
//   - The plan hash (deterministic fingerprint of inputs)
//   - The desired state hash (content address of planning intent)
//   - The full PlanGraph
//   - The full ChangeSet
//   - The risk summary
//   - The rollback graph
//   - Evidence refs
//   - The simulation summary
//   - A SHA-256 export checksum
//
// ─────────────────────────────────────────────────────────────────────────
// DESIGN INVARIANTS
// ─────────────────────────────────────────────────────────────────────────
//
//   1. PlanSnapshot is READ-ONLY: it describes a plan, never executes it.
//   2. ExportHash is a SHA-256 of canonical JSON with ExportHash="".
//      Identical plans produce identical export hashes.
//   3. VerifyPlanHash re-derives the plan hash from the embedded inputs
//      and compares it to the stored PlanHash — detects tampering or drift.
//   4. No credentials, no secrets, no runtime state embedded in the snapshot.

// PlanSnapshotSchemaVersion identifies the snapshot JSON shape.
const PlanSnapshotSchemaVersion = "4.0.0"

// PlanSnapshot is the complete, deterministic export of one plan.
type PlanSnapshot struct {
	// SchemaVersion identifies the JSON structure for future compatibility.
	SchemaVersion string `json:"schema_version"`

	// PlanHash is the deterministic fingerprint of the plan inputs.
	// Computed by ComputePlanHash(). Same inputs → byte-identical hash.
	PlanHash string `json:"plan_hash"`

	// GeneratedAt is the UTC RFC3339 timestamp when this snapshot was created.
	// This field is EXCLUDED from both PlanHash and ExportHash to preserve
	// content-identity semantics (same content = same hash regardless of time).
	GeneratedAt string `json:"generated_at"`

	// DesiredStateHash is the hash of the DesiredState that produced this plan.
	DesiredStateHash string `json:"desired_state_hash"`

	// Environment is the target deployment environment.
	Environment Environment `json:"environment"`

	// Graph is the full PlanGraph.
	Graph *PlanGraph `json:"graph"`

	// ChangeSet is the full ChangeSet derived from the graph.
	ChangeSet *ChangeSet `json:"change_set"`

	// RollbackGraph is the reverse-ordered rollback plan.
	RollbackGraph *RollbackGraph `json:"rollback_graph"`

	// SafetyVerdict is the result of the safety policy layer evaluation.
	SafetyVerdict *SafetyVerdict `json:"safety_verdict"`

	// SimulationSummary is a compact summary from SimulatePlan.
	SimulationSummary SimulationSummary `json:"simulation_summary"`

	// EvidenceRefs are the evidence pointers from the DesiredState.
	EvidenceRefs []EvidenceRef `json:"evidence_refs"`

	// ExportHash is the SHA-256 of the canonical JSON of this struct with
	// ExportHash set to "". Detects post-export tampering.
	ExportHash string `json:"export_hash"`
}

// SimulationSummary is a compact, hash-stable summary of a SimulationResult.
// The full SimulationResult may be large; this captures the key signals.
type SimulationSummary struct {
	ConfidenceScore       int      `json:"confidence_score"`
	IsBlocked             bool     `json:"is_blocked"`
	BlockedReason         string   `json:"blocked_reason,omitempty"`
	IrreversibleNodeCount int      `json:"irreversible_node_count"`
	AmbiguityWarningCount int      `json:"ambiguity_warning_count"`
	HighestRisk           string   `json:"highest_risk"`
	MaxBlastRadius        int      `json:"max_blast_radius"`
	SimulationLimitations []string `json:"simulation_limitations"`
}

// ExportPlanSnapshot assembles a complete PlanSnapshot from all planning
// artifacts. The snapshot is self-contained and can be stored, transmitted,
// and verified independently of the live planning engine state.
//
// obsFingerprints and driftRecordIDs are optional reconciler-layer inputs
// that affect the PlanHash; pass nil to omit.
func ExportPlanSnapshot(
	g *PlanGraph,
	cs *ChangeSet,
	s *DesiredState,
	rg *RollbackGraph,
	verdict *SafetyVerdict,
	sim *SimulationResult,
	obsFingerprints []string,
	driftRecordIDs []string,
) *PlanSnapshot {
	snap := &PlanSnapshot{
		SchemaVersion:    PlanSnapshotSchemaVersion,
		PlanHash:         ComputePlanHash(g, cs, s, obsFingerprints, driftRecordIDs),
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		DesiredStateHash: s.DesiredStateHash,
		Environment:      s.Environment,
		Graph:            g,
		ChangeSet:        cs,
		RollbackGraph:    rg,
		SafetyVerdict:    verdict,
		SimulationSummary: toSimulationSummary(sim),
		EvidenceRefs:     s.EvidenceRefs,
		ExportHash:       "",
	}

	// Compute export hash: SHA-256 of canonical JSON with ExportHash="".
	snap.ExportHash = computeExportHash(snap)
	return snap
}

// VerifyPlanHash re-derives the plan hash from the embedded graph, changeset,
// and desired state hash, then compares it to the stored PlanHash.
//
// Returns (true, "") when the hash is consistent.
// Returns (false, reason) when the hash does not match or inputs are missing.
//
// VerifyPlanHash does NOT contact any provider and does NOT mutate any state.
func VerifyPlanHash(snap *PlanSnapshot, s *DesiredState, obsFingerprints []string, driftRecordIDs []string) (bool, string) {
	if snap == nil {
		return false, "snapshot is nil"
	}
	if snap.Graph == nil {
		return false, "snapshot graph is nil — cannot recompute hash"
	}
	if snap.ChangeSet == nil {
		return false, "snapshot changeset is nil — cannot recompute hash"
	}
	if s == nil {
		return false, "desired state is nil — cannot recompute hash"
	}

	recomputed := ComputePlanHash(snap.Graph, snap.ChangeSet, s, obsFingerprints, driftRecordIDs)
	if recomputed == "" {
		return false, "hash computation failed — check planner inputs"
	}
	if recomputed != snap.PlanHash {
		return false, "plan hash mismatch: stored=" + snap.PlanHash + " recomputed=" + recomputed + " — plan inputs may have changed or the snapshot was modified"
	}
	return true, ""
}

// VerifyExportHash re-derives the export hash by zeroing the ExportHash field
// and re-marshaling the snapshot. Returns (true, "") when consistent.
func VerifyExportHash(snap *PlanSnapshot) (bool, string) {
	if snap == nil {
		return false, "snapshot is nil"
	}
	stored := snap.ExportHash
	if stored == "" {
		return false, "snapshot has no export hash"
	}

	// Work on a copy so we don't mutate the original.
	copy := *snap
	copy.ExportHash = ""
	recomputed := computeExportHash(&copy)
	if recomputed == "" {
		return false, "export hash computation failed"
	}
	if recomputed != stored {
		return false, "export hash mismatch: stored=" + stored + " recomputed=" + recomputed + " — snapshot may have been tampered with"
	}
	return true, ""
}

// ─────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────

func computeExportHash(snap *PlanSnapshot) string {
	// GeneratedAt changes per call; exclude it from the hash by zeroing it in
	// the temporary copy so content identity drives the hash.
	withoutTime := *snap
	withoutTime.GeneratedAt = ""
	withoutTime.ExportHash = ""
	payload, err := json.Marshal(withoutTime)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func toSimulationSummary(sim *SimulationResult) SimulationSummary {
	if sim == nil {
		return SimulationSummary{
			SimulationLimitations: simulationLimitations(),
		}
	}
	highest := ""
	if sim.RiskSummary.HighestRisk != "" {
		highest = string(sim.RiskSummary.HighestRisk)
	}
	return SimulationSummary{
		ConfidenceScore:       sim.ConfidenceScore,
		IsBlocked:             sim.IsBlocked,
		BlockedReason:         sim.BlockedReason,
		IrreversibleNodeCount: sim.RiskSummary.IrreversibleCount,
		AmbiguityWarningCount: len(sim.AmbiguityWarnings),
		HighestRisk:           highest,
		MaxBlastRadius:        sim.RiskSummary.MaxBlastRadius,
		SimulationLimitations: sim.SimulationLimitations,
	}
}
