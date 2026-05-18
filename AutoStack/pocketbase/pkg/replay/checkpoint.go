package replay

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/janlauber/one-click/pkg/events"
)

// NewCheckpoint creates a ReplayCheckpoint at the tail of the provided event slice.
// StateHash is SHA-256 over the event hashes in order — identical to the
// events.EventCheckpoint.StateHash convention so snapshots are interchangeable.
func NewCheckpoint(
	checkpointID, streamID, executionID, tenantID string,
	evts []events.PlatformEvent,
) ReplayCheckpoint {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var atClock int64
	var atTimestamp string
	if len(evts) > 0 {
		last := evts[len(evts)-1]
		atClock = last.LogicalClock
		atTimestamp = last.Timestamp
	}
	return ReplayCheckpoint{
		CheckpointID: checkpointID,
		StreamID:     streamID,
		ExecutionID:  executionID,
		TenantID:     tenantID,
		AtClock:      atClock,
		AtTimestamp:  atTimestamp,
		StateHash:    CheckpointStateHash(evts),
		CreatedAt:    now,
	}
}

// CheckpointStateHash derives SHA-256 over all event hashes up to the checkpoint.
// Same algorithm as events.computeCheckpointStateHash for cross-package consistency.
func CheckpointStateHash(evts []events.PlatformEvent) string {
	if len(evts) == 0 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte("")))
	}
	h := sha256.New()
	for _, e := range evts {
		fmt.Fprintf(h, "%s|", e.Hash)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// VerifyCheckpoint recomputes the StateHash from the provided events and
// checks it matches the stored hash.
func VerifyCheckpoint(checkpoint ReplayCheckpoint, evts []events.PlatformEvent) (bool, string) {
	// Only include events up to and including AtClock.
	var inScope []events.PlatformEvent
	for _, e := range evts {
		if e.LogicalClock <= checkpoint.AtClock {
			inScope = append(inScope, e)
		}
	}
	expected := CheckpointStateHash(inScope)
	if checkpoint.StateHash != expected {
		return false, fmt.Sprintf("checkpoint %q StateHash mismatch: expected %q got %q",
			checkpoint.CheckpointID, expected, checkpoint.StateHash)
	}
	return true, ""
}

// FindNearestCheckpoint returns the checkpoint whose AtClock is the largest
// value ≤ fromClock. Returns nil if no checkpoint qualifies.
func FindNearestCheckpoint(checkpoints []ReplayCheckpoint, fromClock int64) *ReplayCheckpoint {
	var best *ReplayCheckpoint
	for i := range checkpoints {
		cp := &checkpoints[i]
		if cp.AtClock <= fromClock {
			if best == nil || cp.AtClock > best.AtClock {
				best = cp
			}
		}
	}
	return best
}
