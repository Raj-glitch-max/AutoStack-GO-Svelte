package executor

import (
	"fmt"
	"time"
)

// AbandonedTask describes a task that was dispatched or running but has no worker heartbeat.
type AbandonedTask struct {
	TaskID         string    `json:"task_id"`
	ExecutionID    string    `json:"execution_id"`
	AssignedWorker string    `json:"assigned_worker"`
	LastKnownState TaskState `json:"last_known_state"`
	DetectedAt     string    `json:"detected_at"`
}

// RecoveryResult is the output of RecoverExecutionRuntime.
type RecoveryResult struct {
	ExecutionID     string          `json:"execution_id"`
	TenantID        string          `json:"tenant_id"`
	CheckpointID    string          `json:"checkpoint_id"`
	Recovered       []ExecutionTask `json:"recovered"`
	Abandoned       []AbandonedTask `json:"abandoned"`
	TerminalTasks   []ExecutionTask `json:"terminal_tasks"`
	RecoveredAt     string          `json:"recovered_at"`
}

// RecoverExecutionRuntime rebuilds task states from a checkpoint.
// Tasks in terminal states are reported separately. Non-terminal tasks are
// recovered to their checkpointed state. Tasks that were dispatched/running
// without a live worker are flagged as abandoned — never silently resurrected.
//
// liveWorkers is the set of workerIDs currently reporting heartbeats.
func RecoverExecutionRuntime(cp ExecutionCheckpoint, liveWorkers map[string]struct{}) (RecoveryResult, error) {
	if ok, reason := VerifyExecutionCheckpoint(cp); !ok {
		return RecoveryResult{}, fmt.Errorf("recovery: checkpoint integrity failed: %s", reason)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	result := RecoveryResult{
		ExecutionID:  cp.ExecutionID,
		TenantID:     cp.TenantID,
		CheckpointID: cp.CheckpointID,
		RecoveredAt:  now,
	}

	for _, entry := range cp.Tasks {
		task := ExecutionTask{
			TaskID:         entry.TaskID,
			ExecutionID:    cp.ExecutionID,
			TenantID:       cp.TenantID,
			State:          entry.State,
			AssignedWorker: entry.AssignedWorker,
			Attempt:        entry.Attempt,
			LogicalClock:   entry.LogicalClock,
			RetryRef:       entry.RetryRef,
		}
		task.Hash = computeTaskHash(task)

		if IsTerminalTask(task) {
			result.TerminalTasks = append(result.TerminalTasks, task)
			continue
		}

		// If the task was dispatched/running but the worker is gone, flag as abandoned.
		if (entry.State == TaskStateDispatched || entry.State == TaskStateRunning) &&
			entry.AssignedWorker != "" {
			if _, alive := liveWorkers[entry.AssignedWorker]; !alive {
				result.Abandoned = append(result.Abandoned, AbandonedTask{
					TaskID:         entry.TaskID,
					ExecutionID:    cp.ExecutionID,
					AssignedWorker: entry.AssignedWorker,
					LastKnownState: entry.State,
					DetectedAt:     now,
				})
				continue
			}
		}

		result.Recovered = append(result.Recovered, task)
	}

	return result, nil
}

// DetectAbandonedTasks scans a task list for tasks assigned to workers not in liveWorkers.
// Returns evidence only — callers decide what to do with it.
func DetectAbandonedTasks(tasks []ExecutionTask, liveWorkers map[string]struct{}) []AbandonedTask {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var out []AbandonedTask
	for _, t := range tasks {
		if t.AssignedWorker == "" || IsTerminalTask(t) {
			continue
		}
		if t.State != TaskStateDispatched && t.State != TaskStateRunning {
			continue
		}
		if _, alive := liveWorkers[t.AssignedWorker]; !alive {
			out = append(out, AbandonedTask{
				TaskID:         t.TaskID,
				ExecutionID:    t.ExecutionID,
				AssignedWorker: t.AssignedWorker,
				LastKnownState: t.State,
				DetectedAt:     now,
			})
		}
	}
	return out
}
