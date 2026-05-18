package scheduler

import (
	"fmt"
	"sort"
	"time"
)

// PauseExecutionSchedule captures a complete, reproducible scheduler snapshot.
//
// The returned PauseRecord contains the exact queue state, concurrency groups, and
// fairness state at the moment of the pause. ResumeExecutionSchedule uses this
// record to reconstruct the identical ordering — no reshuffling allowed.
//
// PausedBy MUST be non-empty; PauseReason MUST be non-empty.
func PauseExecutionSchedule(
	executionID string,
	pausedBy string,
	pauseReason string,
	queue StageQueue,
	groups []ConcurrencyGroup,
	fairness SchedulerFairnessState,
	pendingApprovals []string,
	ambiguityBlockers []string,
) (PauseRecord, error) {
	if pausedBy == "" {
		return PauseRecord{}, fmt.Errorf("scheduler: PauseExecutionSchedule requires non-empty pausedBy")
	}
	if pauseReason == "" {
		return PauseRecord{}, fmt.Errorf("scheduler: PauseExecutionSchedule requires non-empty pauseReason")
	}

	// Defensive copies — prevent mutation of the pause record after the fact.
	queueCopy := copyQueue(queue)
	groupsCopy := copyGroups(groups)
	fairnessCopy := copyFairness(fairness)
	approvalsCopy := sortedCopy(pendingApprovals)
	ambiguitiesCopy := sortedCopy(ambiguityBlockers)

	return PauseRecord{
		ExecutionID:       executionID,
		PausedAt:          time.Now().UTC().Format(time.RFC3339),
		PausedBy:          pausedBy,
		PauseReason:       pauseReason,
		QueueState:        queueCopy,
		ConcurrencyState:  groupsCopy,
		FairnessState:     fairnessCopy,
		PendingApprovals:  approvalsCopy,
		AmbiguityBlockers: ambiguitiesCopy,
	}, nil
}

// ResumeExecutionSchedule reconstructs the exact queue and concurrency groups
// from a PauseRecord. The ordering is IDENTICAL to what was present at pause time.
//
// The queue is NEVER reshuffled on resume — the same entries appear in the same
// positions. This preserves replay determinism.
func ResumeExecutionSchedule(pause PauseRecord) (StageQueue, []ConcurrencyGroup) {
	return copyQueue(pause.QueueState), copyGroups(pause.ConcurrencyState)
}

// copyQueue produces a deep copy of a StageQueue.
func copyQueue(q StageQueue) StageQueue {
	entries := make([]QueueEntry, len(q.Entries))
	for i, e := range q.Entries {
		entries[i] = QueueEntry{
			StageID:         e.StageID,
			StageOrder:      e.StageOrder,
			DependencyDepth: e.DependencyDepth,
			RiskLevel:       e.RiskLevel,
			Providers:       append([]string(nil), e.Providers...),
			BlastRadius:     e.BlastRadius,
			DependsOn:       append([]string(nil), e.DependsOn...),
		}
	}
	return StageQueue{
		ExecutionID: q.ExecutionID,
		Entries:     entries,
		CreatedAt:   q.CreatedAt,
	}
}

// copyGroups produces a deep copy of a []ConcurrencyGroup.
func copyGroups(groups []ConcurrencyGroup) []ConcurrencyGroup {
	if groups == nil {
		return nil
	}
	result := make([]ConcurrencyGroup, len(groups))
	for i, g := range groups {
		result[i] = ConcurrencyGroup{
			GroupID:   g.GroupID,
			StageIDs:  append([]string(nil), g.StageIDs...),
			Rationale: g.Rationale,
		}
	}
	return result
}

// sortedCopy returns a sorted copy of a string slice.
func sortedCopy(ss []string) []string {
	if ss == nil {
		return nil
	}
	cp := append([]string(nil), ss...)
	sort.Strings(cp)
	return cp
}
