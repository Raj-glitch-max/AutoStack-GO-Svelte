package platformapi

import (
	"time"

	"github.com/janlauber/one-click/pkg/workflow"
)

// WorkflowView is a read-only projection of a WorkflowExecution safe for API responses.
type WorkflowView struct {
	ExecutionID string                          `json:"execution_id"`
	TenantID    string                          `json:"tenant_id"`
	State       workflow.WorkflowExecutionState `json:"state"`
	CurrentNode string                          `json:"current_node"`
	NodeStates  map[string]workflow.NodeState   `json:"node_states"`
	HashValid   bool                            `json:"hash_valid"`
}

// WorkflowQueryResult groups workflow views.
type WorkflowQueryResult struct {
	Workflows  []WorkflowView `json:"workflows"`
	TotalCount int            `json:"total_count"`
	QueryAt    string         `json:"query_at"`
}

// QueryWorkflowExecutions returns read-only views for the given tenant and optional executionID.
func QueryWorkflowExecutions(executions []workflow.WorkflowExecution, tenantID, executionID string) WorkflowQueryResult {
	result := WorkflowQueryResult{
		QueryAt: time.Now().UTC().Format(time.RFC3339Nano),
	}

	for _, e := range executions {
		if e.TenantID != tenantID {
			continue
		}
		if executionID != "" && e.ExecutionID != executionID {
			continue
		}
		ok, _ := workflow.VerifyWorkflowExecution(e)
		cp := make(map[string]workflow.NodeState, len(e.NodeStates))
		for k, v := range e.NodeStates {
			cp[k] = v
		}
		result.Workflows = append(result.Workflows, WorkflowView{
			ExecutionID:  e.ExecutionID,
			TenantID:     e.TenantID,
			State:        e.State,
			CurrentNode:  e.ActiveNodeID,
			NodeStates:   cp,
			HashValid:    ok,
		})
	}

	result.TotalCount = len(result.Workflows)
	return result
}

// WorkflowProgressSummary gives counts of nodes per state across an execution.
type WorkflowProgressSummary struct {
	ExecutionID string         `json:"execution_id"`
	TenantID    string         `json:"tenant_id"`
	NodeCounts  map[string]int `json:"node_counts"`
	IsTerminal  bool           `json:"is_terminal"`
}

// SummarizeWorkflowProgress builds node-state counts for a single WorkflowExecution.
func SummarizeWorkflowProgress(exec workflow.WorkflowExecution) WorkflowProgressSummary {
	counts := make(map[string]int)
	for _, ns := range exec.NodeStates {
		counts[string(ns)]++
	}
	terminal := exec.State == workflow.ExecutionStateCompleted ||
		exec.State == workflow.ExecutionStateFailed ||
		exec.State == workflow.ExecutionStateRolledBack

	return WorkflowProgressSummary{
		ExecutionID: exec.ExecutionID,
		TenantID:    exec.TenantID,
		NodeCounts:  counts,
		IsTerminal:  terminal,
	}
}
