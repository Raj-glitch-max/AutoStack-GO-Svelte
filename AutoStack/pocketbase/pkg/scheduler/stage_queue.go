package scheduler

import (
	"fmt"
	"time"

	"github.com/janlauber/one-click/pkg/orchestrator"
)

// BuildStageQueue constructs a deterministic, replay-safe StageQueue from an ExecutionPlan.
//
// Pure function: same ExecutionPlan always produces the same StageQueue.
// Order: DependencyDepth → RiskLevel → StageOrder → StageID (see ordering.go).
func BuildStageQueue(plan *orchestrator.ExecutionPlan) StageQueue {
	if plan == nil || len(plan.Stages) == 0 {
		return StageQueue{
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
	}

	depthIndex := ComputeDependencyDepths(plan.Stages)

	entries := make([]QueueEntry, 0, len(plan.Stages))
	for _, stage := range plan.Stages {
		entries = append(entries, QueueEntry{
			StageID:         stage.StageID,
			StageOrder:      stage.StageOrder,
			DependencyDepth: depthIndex[stage.StageID],
			RiskLevel:       stage.RiskLevel,
			Providers:       extractProviders(stage),
			BlastRadius:     stage.EstimatedBlastRadius,
			DependsOn:       append([]string(nil), stage.Dependencies...),
		})
	}

	SortQueueEntries(entries)

	return StageQueue{
		ExecutionID: executionIDFromPlan(plan),
		Entries:     entries,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
}

// Dequeue removes and returns the first QueueEntry whose dependencies are all satisfied.
// satisfiedStages is the set of already-completed stageIDs.
// Returns (entry, true) if a ready entry was found, (zero, false) if none is ready.
func Dequeue(queue StageQueue, satisfiedStages map[string]bool) (QueueEntry, StageQueue, bool) {
	for i, entry := range queue.Entries {
		if allSatisfied(entry.DependsOn, satisfiedStages) {
			remaining := make([]QueueEntry, 0, len(queue.Entries)-1)
			remaining = append(remaining, queue.Entries[:i]...)
			remaining = append(remaining, queue.Entries[i+1:]...)
			return entry, StageQueue{
				ExecutionID: queue.ExecutionID,
				Entries:     remaining,
				CreatedAt:   queue.CreatedAt,
			}, true
		}
	}
	return QueueEntry{}, queue, false
}

// QueueSize returns the number of entries remaining in the queue.
func QueueSize(queue StageQueue) int {
	return len(queue.Entries)
}

// IsEmpty returns true when the queue has no remaining entries.
func IsEmpty(queue StageQueue) bool {
	return len(queue.Entries) == 0
}

// ContainsStage returns true when the queue contains the given stageID.
func ContainsStage(queue StageQueue, stageID string) bool {
	for _, e := range queue.Entries {
		if e.StageID == stageID {
			return true
		}
	}
	return false
}

// StageQueueState derives the SchedulerState from the current queue and blocked set.
func StageQueueState(queue StageQueue, blocked []BlockedStage) SchedulerState {
	if IsEmpty(queue) {
		return SchedulerDrained
	}
	if len(blocked) > 0 {
		return SchedulerBlocked
	}
	return SchedulerRunning
}

// ReplayOrderFromQueue extracts the stable stageID ordering from a queue.
func ReplayOrderFromQueue(queue StageQueue) []string {
	order := make([]string, len(queue.Entries))
	for i, e := range queue.Entries {
		order[i] = e.StageID
	}
	return order
}

// allSatisfied returns true when every dep in deps is in the satisfied set.
func allSatisfied(deps []string, satisfied map[string]bool) bool {
	for _, dep := range deps {
		if !satisfied[dep] {
			return false
		}
	}
	return true
}

// executionIDFromPlan extracts a stable identifier from the execution plan.
func executionIDFromPlan(plan *orchestrator.ExecutionPlan) string {
	if plan == nil {
		return ""
	}
	return fmt.Sprintf("plan:%s", plan.ExecutionPlanHash)
}
