package executor

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// TaskState is the explicit FSM state for a single execution task.
type TaskState string

const (
	TaskStatePending            TaskState = "pending"
	TaskStateQueued             TaskState = "queued"
	TaskStateDispatched         TaskState = "dispatched"
	TaskStateRunning            TaskState = "running"
	TaskStatePaused             TaskState = "paused"
	TaskStateRetryWait          TaskState = "retry_wait"
	TaskStateVerificationWait   TaskState = "verification_wait"
	TaskStateCompleted          TaskState = "completed"
	TaskStateFailed             TaskState = "failed"
	TaskStateContradicted       TaskState = "contradicted"
	TaskStateQuarantined        TaskState = "quarantined"
	TaskStateRolledBack         TaskState = "rolled_back"
)

// TaskType classifies what a task does.
type TaskType string

const (
	TaskTypeDeploy       TaskType = "deploy"
	TaskTypeDestroy      TaskType = "destroy"
	TaskTypeScale        TaskType = "scale"
	TaskTypeRestart      TaskType = "restart"
	TaskTypeRollback     TaskType = "rollback"
	TaskTypeVerify       TaskType = "verify"
	TaskTypeObserve      TaskType = "observe"
	TaskTypeQuarantine   TaskType = "quarantine"
	TaskTypeCheckpoint   TaskType = "checkpoint"
	TaskTypeNotification TaskType = "notification"
)

// ExecutionTask is the atomic unit of execution. Immutable after creation.
// CreatedAt, StartedAt, CompletedAt are excluded from the hash (content-identity).
type ExecutionTask struct {
	TaskID            string    `json:"task_id"`
	ExecutionID       string    `json:"execution_id"`
	WorkflowID        string    `json:"workflow_id"`
	StepID            string    `json:"step_id"`
	TenantID          string    `json:"tenant_id"`
	TaskType          TaskType  `json:"task_type"`
	Attempt           int       `json:"attempt"`
	State             TaskState `json:"state"`
	AssignedWorker    string    `json:"assigned_worker"`
	LogicalClock      int64     `json:"logical_clock"`
	PolicyDecisionRef string    `json:"policy_decision_ref,omitempty"`
	JournalRef        string    `json:"journal_ref,omitempty"`
	RetryRef          string    `json:"retry_ref,omitempty"`
	VerificationRef   string    `json:"verification_ref,omitempty"`
	CreatedAt         string    `json:"created_at"`  // excluded from hash
	StartedAt         string    `json:"started_at"`  // excluded from hash
	CompletedAt       string    `json:"completed_at"` // excluded from hash
	Hash              string    `json:"hash"`
}

// NewExecutionTask creates a new task in TaskStatePending with a computed hash.
func NewExecutionTask(
	taskID, executionID, workflowID, stepID, tenantID string,
	taskType TaskType,
	attempt int,
	logicalClock int64,
) ExecutionTask {
	t := ExecutionTask{
		TaskID:       taskID,
		ExecutionID:  executionID,
		WorkflowID:   workflowID,
		StepID:       stepID,
		TenantID:     tenantID,
		TaskType:     taskType,
		Attempt:      attempt,
		State:        TaskStatePending,
		LogicalClock: logicalClock,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	t.Hash = computeTaskHash(t)
	return t
}

// VerifyTaskHash recomputes the hash and returns (true, "") when it matches.
func VerifyTaskHash(t ExecutionTask) (bool, string) {
	stored := t.Hash
	t.Hash = ""
	expected := computeTaskHash(t)
	if stored != expected {
		return false, fmt.Sprintf("task %q hash mismatch: stored=%s computed=%s", t.TaskID, stored, expected)
	}
	return true, ""
}

// IsTerminalTask returns true when the task cannot transition further.
func IsTerminalTask(t ExecutionTask) bool {
	switch t.State {
	case TaskStateCompleted, TaskStateFailed, TaskStateContradicted,
		TaskStateQuarantined, TaskStateRolledBack:
		return true
	}
	return false
}

func computeTaskHash(t ExecutionTask) string {
	h := sha256.New()
	fmt.Fprintf(h,
		"task:%s|exec:%s|workflow:%s|step:%s|tenant:%s|type:%s|attempt:%d|state:%s|worker:%s|clock:%d|policy:%s|journal:%s|retry:%s|verify:%s",
		t.TaskID, t.ExecutionID, t.WorkflowID, t.StepID, t.TenantID,
		t.TaskType, t.Attempt, t.State, t.AssignedWorker, t.LogicalClock,
		t.PolicyDecisionRef, t.JournalRef, t.RetryRef, t.VerificationRef,
	)
	return fmt.Sprintf("%x", h.Sum(nil))
}
