package scheduler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// schedulingSnapshotLimitations documents invariant limitations.
func schedulingSnapshotLimitations() []string {
	return []string{
		"The scheduler does not enforce execution — it describes deterministic ordering only.",
		"Concurrency groups are advisory; executor must enforce actual parallelism limits.",
		"Fairness thresholds are configurable but not auto-enforced — operator review required on starvation.",
		"Pause/resume reconstruction is exact only if the PauseRecord is not modified after capture.",
		"Replay requires the original StageQueue to reconstruct remaining entries by entry order.",
	}
}

// ExportSchedulingSnapshot produces a tamper-evident, self-describing snapshot
// of the current scheduler state.
func ExportSchedulingSnapshot(
	executionID string,
	state SchedulerState,
	queue StageQueue,
	groups []ConcurrencyGroup,
	fairness SchedulerFairnessState,
	pause *PauseRecord,
	replayLog []SchedulerReplayEvent,
) *SchedulingSnapshot {
	replayOrder := ReplayOrderFromEvents(replayLog)
	if len(replayOrder) == 0 {
		replayOrder = ReplayOrderFromQueue(queue)
	}

	snap := &SchedulingSnapshot{
		SchemaVersion:     SchedulingSnapshotSchemaVersion,
		ExecutionID:       executionID,
		State:             state,
		Queue:             queue,
		ConcurrencyGroups: groups,
		FairnessState:     fairness,
		PauseRecord:       pause,
		ReplayLog:         append([]SchedulerReplayEvent(nil), replayLog...),
		ReplayOrder:       replayOrder,
		Limitations:       schedulingSnapshotLimitations(),
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
	}

	snap.SchedulerHash = computeSchedulerHash(snap)
	return snap
}

// VerifySchedulingSnapshotHash re-derives the hash and compares it to SchedulerHash.
func VerifySchedulingSnapshotHash(snap *SchedulingSnapshot) (bool, string) {
	if snap == nil {
		return false, "snapshot is nil"
	}
	if snap.SchedulerHash == "" {
		return false, "SchedulerHash is empty"
	}
	recomputed := computeSchedulerHash(snap)
	if recomputed != snap.SchedulerHash {
		return false, fmt.Sprintf("SchedulerHash mismatch: stored=%q recomputed=%q", snap.SchedulerHash, recomputed)
	}
	return true, ""
}

// computeSchedulerHash derives SHA-256 over canonical content,
// excluding GeneratedAt and SchedulerHash for content-identity hashing.
func computeSchedulerHash(snap *SchedulingSnapshot) string {
	canonical := struct {
		SchemaVersion     string                 `json:"schema_version"`
		ExecutionID       string                 `json:"execution_id"`
		State             SchedulerState         `json:"state"`
		Queue             StageQueue             `json:"queue"`
		ConcurrencyGroups []ConcurrencyGroup     `json:"concurrency_groups"`
		FairnessState     SchedulerFairnessState `json:"fairness_state"`
		PauseRecord       *PauseRecord           `json:"pause_record,omitempty"`
		ReplayOrder       []string               `json:"replay_order"`
	}{
		SchemaVersion:     snap.SchemaVersion,
		ExecutionID:       snap.ExecutionID,
		State:             snap.State,
		Queue:             snap.Queue,
		ConcurrencyGroups: snap.ConcurrencyGroups,
		FairnessState:     snap.FairnessState,
		PauseRecord:       snap.PauseRecord,
		ReplayOrder:       snap.ReplayOrder,
	}

	b, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum)
}
