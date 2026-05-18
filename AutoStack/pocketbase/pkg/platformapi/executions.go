package platformapi

import (
	"fmt"
	"time"

	"github.com/janlauber/one-click/pkg/executor"
)

// ExecutionQueryFilter constrains which tasks are returned.
type ExecutionQueryFilter struct {
	ExecutionID string
	TenantID    string
	State       executor.TaskState // zero-value = all states
	Type        executor.TaskType  // zero-value = all types
}

// ExecutionTaskView is a read-only projection of an ExecutionTask — safe for API responses.
type ExecutionTaskView struct {
	TaskID      string             `json:"task_id"`
	ExecutionID string             `json:"execution_id"`
	TenantID    string             `json:"tenant_id"`
	Type        executor.TaskType  `json:"task_type"`
	State       executor.TaskState `json:"state"`
	LogicalClock int64             `json:"logical_clock"`
	HashValid   bool               `json:"hash_valid"`
	CreatedAt   string             `json:"created_at"`
	StartedAt   string             `json:"started_at,omitempty"`
	CompletedAt string             `json:"completed_at,omitempty"`
}

// ExecutionQueryResult groups tasks returned by a query.
type ExecutionQueryResult struct {
	ExecutionID string              `json:"execution_id"`
	TenantID    string              `json:"tenant_id"`
	Tasks       []ExecutionTaskView `json:"tasks"`
	TotalCount  int                 `json:"total_count"`
	QueryAt     string              `json:"query_at"`
}

// QueryExecutionTasks returns a filtered, read-only view of tasks.
// All returned tasks are verified for hash integrity; invalid tasks are flagged but included.
func QueryExecutionTasks(tasks []executor.ExecutionTask, filter ExecutionQueryFilter) ExecutionQueryResult {
	result := ExecutionQueryResult{
		ExecutionID: filter.ExecutionID,
		TenantID:    filter.TenantID,
		QueryAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}

	for _, t := range tasks {
		if filter.ExecutionID != "" && t.ExecutionID != filter.ExecutionID {
			continue
		}
		if filter.TenantID != "" && t.TenantID != filter.TenantID {
			continue
		}
		if filter.State != "" && t.State != filter.State {
			continue
		}
		if filter.Type != "" && t.TaskType != filter.Type {
			continue
		}

		ok, _ := executor.VerifyTaskHash(t)
		result.Tasks = append(result.Tasks, ExecutionTaskView{
			TaskID:       t.TaskID,
			ExecutionID:  t.ExecutionID,
			TenantID:     t.TenantID,
			Type:         t.TaskType,
			State:        t.State,
			LogicalClock: t.LogicalClock,
			HashValid:    ok,
			CreatedAt:    t.CreatedAt,
			StartedAt:    t.StartedAt,
			CompletedAt:  t.CompletedAt,
		})
	}

	result.TotalCount = len(result.Tasks)
	return result
}

// ExecutionSummary provides aggregate counts for an execution.
type ExecutionSummary struct {
	ExecutionID    string         `json:"execution_id"`
	TenantID       string         `json:"tenant_id"`
	StateCounts    map[string]int `json:"state_counts"`
	IntegrityIssues int           `json:"integrity_issues"`
	TerminalCount  int            `json:"terminal_count"`
	SummarizedAt   string         `json:"summarized_at"`
}

// SummarizeExecution aggregates state counts and integrity status across all tasks.
func SummarizeExecution(tasks []executor.ExecutionTask, executionID, tenantID string) ExecutionSummary {
	summary := ExecutionSummary{
		ExecutionID: executionID,
		TenantID:    tenantID,
		StateCounts: make(map[string]int),
		SummarizedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}

	for _, t := range tasks {
		if t.ExecutionID != executionID || t.TenantID != tenantID {
			continue
		}
		summary.StateCounts[string(t.State)]++
		if ok, _ := executor.VerifyTaskHash(t); !ok {
			summary.IntegrityIssues++
		}
		if executor.IsTerminalTask(t) {
			summary.TerminalCount++
		}
	}

	return summary
}

// FormatExecutionRef returns a canonical string reference for logging/display.
func FormatExecutionRef(executionID, tenantID string) string {
	return fmt.Sprintf("execution[%s]@tenant[%s]", executionID, tenantID)
}
