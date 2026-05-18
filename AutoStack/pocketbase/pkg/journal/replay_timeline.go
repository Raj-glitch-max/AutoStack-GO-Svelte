package journal

import "fmt"

// TimelineReplayResult describes the outcome of replaying a journal timeline.
// Divergences are collected rather than halting replay — so every entry is examined.
type TimelineReplayResult struct {
	ExecutionID   string
	TenantID      string
	EntryCount    int
	RetryCount    int
	ApprovalWaits int // number of approval_requested entries
	Contradictions int
	PausedAt      int64    // LogicalClock of last workflow_paused entry; 0 if none
	ResumedCount  int
	Divergences   []string // human-readable descriptions; empty means clean replay
}

// ReplayExecutionTimeline replays a journal timeline in causal order.
// Every entry hash is verified. Clock regressions are recorded as divergences.
// The original timeline is never mutated.
func ReplayExecutionTimeline(timeline ExecutionTimeline) (TimelineReplayResult, error) {
	result := TimelineReplayResult{
		ExecutionID: timeline.ExecutionID,
		TenantID:    timeline.TenantID,
		EntryCount:  len(timeline.Entries),
	}

	var lastClock int64
	for i, e := range timeline.Entries {
		if ok, reason := VerifyJournalEntry(e); !ok {
			result.Divergences = append(result.Divergences,
				fmt.Sprintf("entry %d (id=%s): %s", i, e.JournalID, reason))
			continue
		}
		if e.LogicalClock < lastClock {
			result.Divergences = append(result.Divergences,
				fmt.Sprintf("entry %d (id=%s): clock %d regresses below last seen %d",
					i, e.JournalID, e.LogicalClock, lastClock))
		}
		if e.LogicalClock > lastClock {
			lastClock = e.LogicalClock
		}

		switch e.JournalType {
		case JournalTypeRetryExecuted:
			result.RetryCount++
		case JournalTypeApprovalRequested:
			result.ApprovalWaits++
		case JournalTypeContradictionDetected:
			result.Contradictions++
		case JournalTypeWorkflowPaused:
			result.PausedAt = e.LogicalClock
		case JournalTypeWorkflowResumed:
			result.ResumedCount++
		}
	}
	return result, nil
}

// IsDivergenceFree returns true when a replay result has no recorded divergences.
func IsDivergenceFree(r TimelineReplayResult) bool {
	return len(r.Divergences) == 0
}
