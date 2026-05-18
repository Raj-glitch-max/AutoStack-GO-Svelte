package verifier

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// VerificationSnapshotSchemaVersion is the canonical schema version for Phase 4.4.
const VerificationSnapshotSchemaVersion = "4.4.0"

// ExportVerificationSnapshot produces a tamper-evident, self-describing snapshot
// of a verification result.
//
// Hashing invariant: GeneratedAt and ExportHash are excluded from the hash input
// so that time-of-export does not invalidate the hash. Two exports of the same
// result must produce identical ExportHash values.
func ExportVerificationSnapshot(
	result VerificationResult,
	tl ObservationTimeline,
	window StabilizationWindow,
	policy ConsistencyPolicy,
) *VerificationSnapshot {
	snap := &VerificationSnapshot{
		SchemaVersion:       VerificationSnapshotSchemaVersion,
		ExecutionID:         result.ExecutionID,
		StageID:             result.StageID,
		NodeID:              result.NodeID,
		State:               result.State,
		ObservationCount:    len(tl.Observations),
		ContradictionCount:  len(result.Contradictions),
		Confidence:          result.Confidence,
		StabilizationMet:    result.StabilizationMet,
		StabilizationWindow: window,
		Policy:              policy,
		Evidence:            result.Evidence,
		Contradictions:      result.Contradictions,
		Recommendation:      result.Recommendation,
		Limitations:         result.Limitations,
		VerifiedAt:          result.VerifiedAt,
		GeneratedAt:         time.Now().UTC().Format(time.RFC3339),
	}

	snap.ExportHash = computeVerificationHash(snap)
	return snap
}

// VerifyVerificationSnapshot re-derives the hash and compares it to the stored
// ExportHash. Returns (false, reason) if the snapshot has been tampered with.
func VerifyVerificationSnapshot(snap *VerificationSnapshot) (bool, string) {
	if snap == nil {
		return false, "snapshot is nil"
	}
	if snap.ExportHash == "" {
		return false, "ExportHash is empty"
	}
	recomputed := computeVerificationHash(snap)
	if recomputed != snap.ExportHash {
		return false, fmt.Sprintf("ExportHash mismatch: stored=%q recomputed=%q", snap.ExportHash, recomputed)
	}
	return true, ""
}

// computeVerificationHash derives the SHA-256 hash of the snapshot content,
// excluding GeneratedAt and ExportHash to ensure content identity.
func computeVerificationHash(snap *VerificationSnapshot) string {
	canonical := struct {
		SchemaVersion       string              `json:"schema_version"`
		ExecutionID         string              `json:"execution_id"`
		StageID             string              `json:"stage_id"`
		NodeID              string              `json:"node_id"`
		State               VerificationState   `json:"state"`
		ObservationCount    int                 `json:"observation_count"`
		ContradictionCount  int                 `json:"contradiction_count"`
		Confidence          ConfidenceScore     `json:"confidence"`
		StabilizationMet    bool                `json:"stabilization_met"`
		StabilizationWindow StabilizationWindow `json:"stabilization_window"`
		Policy              ConsistencyPolicy   `json:"policy"`
		Evidence            []string            `json:"evidence,omitempty"`
		Contradictions      []Contradiction     `json:"contradictions,omitempty"`
		Recommendation      string              `json:"recommendation"`
		Limitations         []string            `json:"limitations"`
		VerifiedAt          string              `json:"verified_at,omitempty"`
	}{
		SchemaVersion:       snap.SchemaVersion,
		ExecutionID:         snap.ExecutionID,
		StageID:             snap.StageID,
		NodeID:              snap.NodeID,
		State:               snap.State,
		ObservationCount:    snap.ObservationCount,
		ContradictionCount:  snap.ContradictionCount,
		Confidence:          snap.Confidence,
		StabilizationMet:    snap.StabilizationMet,
		StabilizationWindow: snap.StabilizationWindow,
		Policy:              snap.Policy,
		Evidence:            snap.Evidence,
		Contradictions:      snap.Contradictions,
		Recommendation:      snap.Recommendation,
		Limitations:         snap.Limitations,
		VerifiedAt:          snap.VerifiedAt,
	}

	b, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum)
}
