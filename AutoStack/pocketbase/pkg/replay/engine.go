package replay

import (
	"time"

	"github.com/janlauber/one-click/pkg/events"
)

// ReplayExecution returns all events for the given executionID from the stream,
// sorted in canonical order. Read-only.
func ReplayExecution(stream events.EventStream, executionID string) ReplayResult {
	evts := events.FilterByExecution(stream.Events, executionID)
	var fromClock, toClock int64
	if len(evts) > 0 {
		fromClock = evts[0].LogicalClock
		toClock = evts[len(evts)-1].LogicalClock
	}
	return ReplayResult{
		ExecutionID: executionID,
		TenantID:    stream.TenantID,
		Events:      evts,
		FromClock:   fromClock,
		ToClock:     toClock,
		EventCount:  len(evts),
		Diverged:    false,
		Limitations: replayLimitations(),
		ReplayedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
}

// ReplayFromCheckpoint replays events starting from the clock position after a
// checkpoint. Events at or before checkpoint.AtClock are excluded from the
// result — the checkpoint represents their aggregate state.
//
// Read-only. Does NOT re-execute any stage.
func ReplayFromCheckpoint(
	stream events.EventStream,
	checkpoint ReplayCheckpoint,
	executionID string,
) ReplayResult {
	var evts []events.PlatformEvent
	for _, e := range stream.Events {
		if e.ExecutionID == executionID && e.LogicalClock > checkpoint.AtClock {
			evts = append(evts, e)
		}
	}
	evts = events.SortEvents(evts)

	var fromClock, toClock int64
	fromClock = checkpoint.AtClock + 1
	if len(evts) > 0 {
		toClock = evts[len(evts)-1].LogicalClock
	} else {
		toClock = checkpoint.AtClock
	}

	return ReplayResult{
		ExecutionID: executionID,
		TenantID:    stream.TenantID,
		Events:      evts,
		Checkpoints: []ReplayCheckpoint{checkpoint},
		FromClock:   fromClock,
		ToClock:     toClock,
		EventCount:  len(evts),
		Diverged:    false,
		Limitations: replayLimitations(),
		ReplayedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
}

// ReplayPartial returns events with LogicalClock in [fromClock, toClock].
// Checkpoint-assisted: if checkpoints are provided, starts from the nearest
// checkpoint at or before fromClock.
func ReplayPartial(
	stream events.EventStream,
	fromClock, toClock int64,
	checkpoints []ReplayCheckpoint,
) ReplayResult {
	// Find nearest checkpoint to reduce scan range.
	var usedCheckpoints []ReplayCheckpoint
	startClock := fromClock
	if cp := FindNearestCheckpoint(checkpoints, fromClock); cp != nil {
		usedCheckpoints = []ReplayCheckpoint{*cp}
		// Start from checkpoint position; events before it are summarized.
		if cp.AtClock < fromClock {
			startClock = cp.AtClock + 1
		}
	}

	var evts []events.PlatformEvent
	for _, e := range stream.Events {
		if e.LogicalClock >= startClock && e.LogicalClock <= toClock {
			evts = append(evts, e)
		}
	}
	evts = events.SortEvents(evts)

	return ReplayResult{
		TenantID:    stream.TenantID,
		Events:      evts,
		Checkpoints: usedCheckpoints,
		FromClock:   fromClock,
		ToClock:     toClock,
		EventCount:  len(evts),
		Diverged:    false,
		Limitations: replayLimitations(),
		ReplayedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
}

// DetectReplayDivergence compares two ReplayResults by comparing event hashes
// at each corresponding position in canonical order.
// Returns (diverged bool, divergenceAt int64) where divergenceAt is the
// LogicalClock of the first differing event (0 if no divergence).
func DetectReplayDivergence(original, replayed ReplayResult) (bool, int64) {
	orig := events.SortEvents(original.Events)
	rep := events.SortEvents(replayed.Events)

	minLen := len(orig)
	if len(rep) < minLen {
		minLen = len(rep)
	}

	for i := 0; i < minLen; i++ {
		if orig[i].Hash != rep[i].Hash {
			return true, orig[i].LogicalClock
		}
	}

	// Different lengths means divergence at the first missing position.
	if len(orig) != len(rep) {
		if minLen < len(orig) {
			return true, orig[minLen].LogicalClock
		}
		return true, rep[minLen].LogicalClock
	}

	return false, 0
}
