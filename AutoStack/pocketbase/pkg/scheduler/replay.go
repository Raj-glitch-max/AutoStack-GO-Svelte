package scheduler

import (
	"sort"
	"time"
)

// ReplaySchedule reconstructs a deterministic StageQueue from a sequence of
// scheduler replay events. The same event log always produces the same result.
//
// Events are processed in ascending Sequence order. Only "enqueued" events
// that have not been subsequently "dequeued" contribute to the output queue.
// The result preserves the original QueueEntry ordering from the events.
func ReplaySchedule(executionID string, events []SchedulerReplayEvent, originalQueue StageQueue) StageQueue {
	if len(events) == 0 {
		return StageQueue{
			ExecutionID: executionID,
			CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		}
	}

	// Sort events by sequence for deterministic processing.
	sorted := append([]SchedulerReplayEvent(nil), events...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Sequence < sorted[j].Sequence
	})

	// Track which stages have been enqueued and then dequeued.
	enqueued := make(map[string]bool)
	dequeued := make(map[string]bool)

	for _, ev := range sorted {
		switch ev.EventType {
		case "enqueued":
			enqueued[ev.StageID] = true
		case "dequeued":
			dequeued[ev.StageID] = true
		}
	}

	// Remaining stages: enqueued but NOT dequeued.
	// Use the original queue's entry order for determinism.
	entryIndex := make(map[string]QueueEntry, len(originalQueue.Entries))
	for _, e := range originalQueue.Entries {
		entryIndex[e.StageID] = e
	}

	var remaining []QueueEntry
	for _, e := range originalQueue.Entries {
		// Include stages that were enqueued but not yet dequeued.
		// Also include stages that appear in the original queue but have no events
		// (they were never processed — still pending).
		if !dequeued[e.StageID] {
			remaining = append(remaining, e)
		}
	}

	return StageQueue{
		ExecutionID: executionID,
		Entries:     remaining,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
}

// NewReplayEvent creates a new SchedulerReplayEvent with the next sequence number.
// sequence must be strictly greater than the previous event's sequence.
func NewReplayEvent(sequence int, stageID, eventType string) SchedulerReplayEvent {
	return SchedulerReplayEvent{
		Sequence:  sequence,
		StageID:   stageID,
		EventType: eventType,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// ReplayOrderFromEvents derives the stable stageID ordering from a set of replay events.
// Order is determined by the first "enqueued" event for each stageID (ascending Sequence).
func ReplayOrderFromEvents(events []SchedulerReplayEvent) []string {
	sorted := append([]SchedulerReplayEvent(nil), events...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Sequence < sorted[j].Sequence
	})

	seen := make(map[string]bool)
	var order []string
	for _, ev := range sorted {
		if ev.EventType == "enqueued" && !seen[ev.StageID] {
			seen[ev.StageID] = true
			order = append(order, ev.StageID)
		}
	}
	return order
}
