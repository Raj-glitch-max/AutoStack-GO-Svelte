package scheduler

import (
	"fmt"
	"sort"
	"time"
)

const (
	// MaxDeferredBeforeStarvation is the number of deferrals before a stage is
	// considered starved. Operators must be notified when this threshold is reached.
	MaxDeferredBeforeStarvation = 5

	// maxBlockDuration is the threshold after which a blocked stage is flagged
	// as potentially starved (for the starvation check only — no auto-resolution).
	maxBlockDurationSeconds = 300 // 5 minutes
)

// NewFairnessState initialises an empty fairness state for an execution.
func NewFairnessState(executionID string) SchedulerFairnessState {
	return SchedulerFairnessState{
		ExecutionID:            executionID,
		BlockedStages:          nil,
		DeferredCounts:         make(map[string]int),
		PauseReasons:           make(map[string]string),
		ReplayOrderingMetadata: make(map[string]string),
		UpdatedAt:              time.Now().UTC().Format(time.RFC3339),
	}
}

// RecordBlocked records that stageID is blocked for the given reason.
// If stageID is already blocked, its DeferredCount is incremented.
// Returns the updated SchedulerFairnessState (value semantics — no mutation).
func RecordBlocked(fairness SchedulerFairnessState, stageID string, reason BlockReason) SchedulerFairnessState {
	// Check if already tracked.
	for i, b := range fairness.BlockedStages {
		if b.StageID == stageID {
			updated := copyFairness(fairness)
			updated.BlockedStages[i].DeferredCount++
			updated.DeferredCounts[stageID]++
			updated.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			return updated
		}
	}

	// New blocked entry.
	updated := copyFairness(fairness)
	updated.BlockedStages = append(updated.BlockedStages, BlockedStage{
		StageID:       stageID,
		BlockedSince:  time.Now().UTC().Format(time.RFC3339),
		BlockReason:   reason,
		DeferredCount: 0,
	})
	updated.DeferredCounts[stageID]++
	updated.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return updated
}

// ClearBlocked removes a stage from the blocked set (it has become unblocked).
func ClearBlocked(fairness SchedulerFairnessState, stageID string) SchedulerFairnessState {
	updated := copyFairness(fairness)
	filtered := fairness.BlockedStages[:0:0]
	for _, b := range fairness.BlockedStages {
		if b.StageID != stageID {
			filtered = append(filtered, b)
		}
	}
	updated.BlockedStages = filtered
	updated.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return updated
}

// RecordPause records a pause reason for a stage.
func RecordPause(fairness SchedulerFairnessState, stageID, reason string) SchedulerFairnessState {
	updated := copyFairness(fairness)
	updated.PauseReasons[stageID] = reason
	updated.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return updated
}

// RecordReplayMeta persists ordering metadata for a stage's replay record.
func RecordReplayMeta(fairness SchedulerFairnessState, stageID, meta string) SchedulerFairnessState {
	updated := copyFairness(fairness)
	updated.ReplayOrderingMetadata[stageID] = meta
	updated.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return updated
}

// CheckStarvation returns a non-empty reason string if stageID is at risk of starvation.
// Starvation is defined as: DeferredCount ≥ MaxDeferredBeforeStarvation.
// The caller decides what to do with this information — no auto-resolution.
func CheckStarvation(fairness SchedulerFairnessState, stageID string) (starved bool, reason string) {
	count := fairness.DeferredCounts[stageID]
	if count >= MaxDeferredBeforeStarvation {
		return true, fmt.Sprintf(
			"stage %s has been deferred %d times (threshold: %d) — operator review required",
			stageID, count, MaxDeferredBeforeStarvation,
		)
	}
	return false, ""
}

// StarvedStages returns all stageIDs that have reached the starvation threshold.
func StarvedStages(fairness SchedulerFairnessState) []string {
	var starved []string
	// Deterministic order: sort stageIDs first.
	ids := make([]string, 0, len(fairness.DeferredCounts))
	for id := range fairness.DeferredCounts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if fairness.DeferredCounts[id] >= MaxDeferredBeforeStarvation {
			starved = append(starved, id)
		}
	}
	return starved
}

// BlockedCount returns the number of currently-blocked stages.
func BlockedCount(fairness SchedulerFairnessState) int {
	return len(fairness.BlockedStages)
}

// copyFairness performs a shallow-safe copy of SchedulerFairnessState.
// Maps and slices are copied to prevent mutation of the original.
func copyFairness(f SchedulerFairnessState) SchedulerFairnessState {
	deferred := make(map[string]int, len(f.DeferredCounts))
	for k, v := range f.DeferredCounts {
		deferred[k] = v
	}
	pauses := make(map[string]string, len(f.PauseReasons))
	for k, v := range f.PauseReasons {
		pauses[k] = v
	}
	replayMeta := make(map[string]string, len(f.ReplayOrderingMetadata))
	for k, v := range f.ReplayOrderingMetadata {
		replayMeta[k] = v
	}
	blocked := append([]BlockedStage(nil), f.BlockedStages...)
	return SchedulerFairnessState{
		ExecutionID:            f.ExecutionID,
		BlockedStages:          blocked,
		DeferredCounts:         deferred,
		PauseReasons:           pauses,
		ReplayOrderingMetadata: replayMeta,
		UpdatedAt:              f.UpdatedAt,
	}
}
