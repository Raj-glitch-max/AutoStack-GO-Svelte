package orchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Phase 4.1 — Execution snapshot export.
//
// An ExecutionSnapshot is the complete, self-contained, tamper-evident export
// of an execution orchestration plan. It bundles the execution plan, safety
// result, contracts, and compensation plan with an SHA-256 export checksum.
//
// ─────────────────────────────────────────────────────────────────────────
// DESIGN INVARIANTS
// ─────────────────────────────────────────────────────────────────────────
//
//   1. ExecutionSnapshot is READ-ONLY: it describes a plan, never executes it.
//   2. ExportHash is SHA-256 of canonical JSON with ExportHash="" and
//      GeneratedAt="". Identical orchestration plans produce identical hashes.
//   3. VerifyExecutionSnapshotHash re-derives the hash and compares.
//   4. No credentials, secrets, or runtime state embedded in the snapshot.

const ExecutionSnapshotSchemaVersion = "4.1.0"

// ExecutionSnapshot is the complete, deterministic export of one execution plan.
type ExecutionSnapshot struct {
	SchemaVersion     string `json:"schema_version"`
	ExecutionPlanHash string `json:"execution_plan_hash"`
	// GeneratedAt is excluded from all hash computations.
	GeneratedAt string `json:"generated_at"`

	// ExecutionPlan is the full orchestration plan.
	Plan *ExecutionPlan `json:"plan"`

	// SafetyResult is the execution-layer safety evaluation.
	SafetyResult *ExecutionSafetyResult `json:"safety_result"`

	// Contracts is the per-node execution contracts.
	Contracts []ExecutionContract `json:"contracts"`

	// CompensationPlan is the recovery plan for irreversible nodes.
	CompensationPlan *CompensationPlan `json:"compensation_plan"`

	// FailureModels is the per-node failure classification.
	FailureModels []FailureModel `json:"failure_models"`

	// Limitations documents what this snapshot cannot guarantee.
	Limitations []string `json:"limitations"`

	// ExportHash is SHA-256 of canonical JSON with ExportHash="" and GeneratedAt="".
	ExportHash string `json:"export_hash"`
}

// ExportExecutionSnapshot assembles a complete ExecutionSnapshot from all
// orchestration artifacts. The snapshot is self-contained and tamper-evident.
//
// ExportExecutionSnapshot is a pure function: same inputs → same snapshot hash.
func ExportExecutionSnapshot(
	plan *ExecutionPlan,
	safetyResult *ExecutionSafetyResult,
	contracts []ExecutionContract,
	compensationPlan *CompensationPlan,
	failureModels []FailureModel,
) *ExecutionSnapshot {
	snap := &ExecutionSnapshot{
		SchemaVersion:     ExecutionSnapshotSchemaVersion,
		ExecutionPlanHash: "",
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
		Plan:              plan,
		SafetyResult:      safetyResult,
		Contracts:         contracts,
		CompensationPlan:  compensationPlan,
		FailureModels:     failureModels,
		Limitations:       executionSnapshotLimitations(),
		ExportHash:        "",
	}

	if plan != nil {
		snap.ExecutionPlanHash = plan.ExecutionPlanHash
	}

	snap.ExportHash = computeExecutionExportHash(snap)
	return snap
}

// VerifyExecutionSnapshotHash re-derives the export hash by zeroing the
// ExportHash and GeneratedAt fields and re-marshaling the snapshot.
//
// Returns (true, "") when consistent.
// Returns (false, reason) on mismatch or missing hash.
func VerifyExecutionSnapshotHash(snap *ExecutionSnapshot) (bool, string) {
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
	recomputed := computeExecutionExportHash(&copy)
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

func computeExecutionExportHash(snap *ExecutionSnapshot) string {
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

func executionSnapshotLimitations() []string {
	return []string{
		"execution snapshot describes orchestration intent only — it does not guarantee provider availability",
		"snapshot export hash covers plan structure; provider-side state changes between export and execution are not detected",
		"approval gate references in the snapshot must be satisfied by operator action; they are not auto-enforced",
		"compensation and rollback strategies in the snapshot are advisory; actual recovery paths depend on provider capabilities at execution time",
	}
}
