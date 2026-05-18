package executor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/janlauber/one-click/pkg/orchestrator"
)

const ExecutionRuntimeSnapshotSchemaVersion = "4.2.0"

// ExecutionRuntimeSnapshot is a deterministic, tamper-evident export of the
// full execution runtime state at a point in time.
//
// ExportHash is SHA-256 over canonical JSON with ExportHash="" and GeneratedAt="".
// Identical runtime states produce identical export hashes regardless of when
// the snapshot was taken.
type ExecutionRuntimeSnapshot struct {
	SchemaVersion     string         `json:"schema_version"`
	ExecutionID       string         `json:"execution_id"`
	ExecutionPlanHash string         `json:"execution_plan_hash"`
	GeneratedAt       string         `json:"generated_at"` // excluded from ExportHash
	State             ExecutionState `json:"state"`
	CurrentStageIndex int            `json:"current_stage_index"`

	Snapshot    *orchestrator.ExecutionSnapshot `json:"snapshot"`
	Transitions []ExecutionTransition           `json:"transitions"`
	Stages      []StageExecutionRecord          `json:"stages"`

	PendingApprovals []ApprovalDecision  `json:"pending_approvals"`
	Ambiguities      []ExecutionAmbiguity `json:"ambiguities"`

	RollbackRecommendations     []RollbackRecommendation     `json:"rollback_recommendations"`
	CompensationRecommendations []CompensationRecommendation `json:"compensation_recommendations"`

	Limitations []string `json:"limitations"`
	ExportHash  string   `json:"export_hash"` // SHA-256 of content with this field=""
}

// ExportRuntimeSnapshot assembles a complete, tamper-evident runtime snapshot.
// Same inputs always produce the same ExportHash.
func ExportRuntimeSnapshot(
	executionID string,
	snap *orchestrator.ExecutionSnapshot,
	state ExecutionState,
	currentStageIndex int,
	transitions []ExecutionTransition,
	stages []StageExecutionRecord,
	pendingApprovals []ApprovalDecision,
	ambiguities []ExecutionAmbiguity,
	rollbackRecs []RollbackRecommendation,
	compensationRecs []CompensationRecommendation,
) *ExecutionRuntimeSnapshot {
	planHash := ""
	if snap != nil {
		planHash = snap.ExecutionPlanHash
	}

	rs := &ExecutionRuntimeSnapshot{
		SchemaVersion:               ExecutionRuntimeSnapshotSchemaVersion,
		ExecutionID:                 executionID,
		ExecutionPlanHash:           planHash,
		GeneratedAt:                 time.Now().UTC().Format(time.RFC3339),
		State:                       state,
		CurrentStageIndex:           currentStageIndex,
		Snapshot:                    snap,
		Transitions:                 transitions,
		Stages:                      stages,
		PendingApprovals:            pendingApprovals,
		Ambiguities:                 ambiguities,
		RollbackRecommendations:     rollbackRecs,
		CompensationRecommendations: compensationRecs,
		Limitations:                 runtimeSnapshotLimitations(),
		ExportHash:                  "",
	}
	rs.ExportHash = computeRuntimeExportHash(rs)
	return rs
}

// VerifyExecutionRuntimeSnapshot re-derives the export hash and compares against stored.
// Returns (true, "") when consistent; (false, reason) on mismatch or missing hash.
func VerifyExecutionRuntimeSnapshot(rs *ExecutionRuntimeSnapshot) (bool, string) {
	if rs == nil {
		return false, "runtime snapshot is nil"
	}
	stored := rs.ExportHash
	if stored == "" {
		return false, "runtime snapshot has no export hash"
	}
	// Work on a copy — never mutate the original.
	cp := *rs
	cp.ExportHash = ""
	recomputed := computeRuntimeExportHash(&cp)
	if recomputed == "" {
		return false, "runtime export hash computation failed"
	}
	if recomputed != stored {
		return false, "runtime export hash mismatch — snapshot may have been tampered with"
	}
	return true, ""
}

func computeRuntimeExportHash(rs *ExecutionRuntimeSnapshot) string {
	withoutTime := *rs
	withoutTime.GeneratedAt = ""
	withoutTime.ExportHash = ""
	payload, err := json.Marshal(withoutTime)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func runtimeSnapshotLimitations() []string {
	return []string{
		"runtime snapshot captures in-memory state at export time — does not guarantee provider-side state consistency",
		"GeneratedAt is excluded from the export hash; timestamps do not affect content integrity",
		"pending approvals reflect store state at export time — concurrent operator actions after export are not captured",
		"rollback and compensation recommendations are advisory only — no automatic execution",
	}
}
