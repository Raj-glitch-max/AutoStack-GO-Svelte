package workflow

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// WorkflowExecutionState tracks the overall state of a running workflow.
type WorkflowExecutionState string

const (
	ExecutionStatePending     WorkflowExecutionState = "pending"
	ExecutionStateRunning     WorkflowExecutionState = "running"
	ExecutionStatePaused      WorkflowExecutionState = "paused"
	ExecutionStateBlocked     WorkflowExecutionState = "blocked"       // waiting for approval
	ExecutionStateCompleted   WorkflowExecutionState = "completed"
	ExecutionStateFailed      WorkflowExecutionState = "failed"
	ExecutionStateRollingBack WorkflowExecutionState = "rolling_back"
	ExecutionStateRolledBack  WorkflowExecutionState = "rolled_back"
)

// WorkflowExecution is the runtime state of one workflow run.
// All fields are value-typed. Mutations return new instances.
type WorkflowExecution struct {
	ExecutionID   string                 `json:"execution_id"`
	GraphID       string                 `json:"graph_id"`
	TenantID      string                 `json:"tenant_id"`
	State         WorkflowExecutionState `json:"state"`
	ActiveNodeID  string                 `json:"active_node_id"`
	Attempts      map[string]int         `json:"attempts"`    // nodeID → attempt count
	NodeStates    map[string]NodeState   `json:"node_states"` // nodeID → state
	StartedAt     string                 `json:"started_at"`
	UpdatedAt     string                 `json:"updated_at"`
	ExecutionHash string                 `json:"execution_hash"`
}

// NodeOutcome describes the result of executing the currently active node.
type NodeOutcome struct {
	NodeID string   `json:"node_id"`
	Result EdgeType `json:"result"` // which edge type to follow
	Reason string   `json:"reason,omitempty"`
}

// ExecuteWorkflowGraph creates the initial WorkflowExecution for a graph.
// Returns the execution in Running state with the entry node active,
// and an initial StepTransition ("" → entryNode via success).
func ExecuteWorkflowGraph(graph WorkflowGraph, executionID, transitionID, tenantID string) (WorkflowExecution, StepTransition, error) {
	if err := validateGraphForExecution(graph, tenantID); err != nil {
		return WorkflowExecution{}, StepTransition{}, err
	}
	entryNode, ok := FindNode(graph, graph.EntryNode)
	if !ok {
		return WorkflowExecution{}, StepTransition{}, fmt.Errorf("entry node %q not found", graph.EntryNode)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	exec := WorkflowExecution{
		ExecutionID:  executionID,
		GraphID:      graph.GraphID,
		TenantID:     tenantID,
		ActiveNodeID: entryNode.NodeID,
		Attempts:     map[string]int{entryNode.NodeID: 1},
		NodeStates:   map[string]NodeState{entryNode.NodeID: NodeStateActive},
		StartedAt:    now,
		UpdatedAt:    now,
	}
	exec.State = initialState(entryNode)
	exec.ExecutionHash = computeExecHash(exec)

	tr := NewStepTransition(transitionID, executionID, tenantID, "", entryNode.NodeID, EdgeTypeSuccess, 1, "workflow started")
	return exec, tr, nil
}

// AdvanceWorkflow applies a NodeOutcome to the current execution state,
// following the appropriate edge in the graph and returning the new state.
// The original exec is never mutated.
func AdvanceWorkflow(graph WorkflowGraph, exec WorkflowExecution, outcome NodeOutcome, transitionID string) (WorkflowExecution, StepTransition, error) {
	if exec.ActiveNodeID != outcome.NodeID {
		return exec, StepTransition{}, fmt.Errorf("outcome node %q does not match active node %q",
			outcome.NodeID, exec.ActiveNodeID)
	}
	if exec.State == ExecutionStateCompleted || exec.State == ExecutionStateFailed || exec.State == ExecutionStateRolledBack {
		return exec, StepTransition{}, fmt.Errorf("cannot advance execution %q in terminal state %q",
			exec.ExecutionID, exec.State)
	}

	fromNode, _ := FindNode(graph, exec.ActiveNodeID)
	next, hasNext := NextNode(graph, exec.ActiveNodeID, outcome.Result)

	newNodeStates := copyNodeStates(exec.NodeStates)
	newAttempts := copyAttempts(exec.Attempts)

	// Mark the current node as completed or failed.
	fromState := NodeStateCompleted
	if outcome.Result == EdgeTypeFailure || outcome.Result == EdgeTypeTimeout || outcome.Result == EdgeTypeContradiction || outcome.Result == EdgeTypeRollbackTriggered {
		fromState = NodeStateFailed
	}
	newNodeStates[fromNode.NodeID] = fromState

	now := time.Now().UTC().Format(time.RFC3339Nano)
	var newActiveNode string
	var newExecState WorkflowExecutionState
	var attempt int

	if !hasNext {
		// Terminal node — execution ends.
		if fromState == NodeStateCompleted {
			newExecState = ExecutionStateCompleted
		} else {
			newExecState = ExecutionStateFailed
		}
		newActiveNode = ""
		attempt = 0
	} else {
		newAttempts[next.NodeID]++
		attempt = newAttempts[next.NodeID]
		newNodeStates[next.NodeID] = NodeStateActive
		newActiveNode = next.NodeID
		newExecState = nextExecState(next, outcome.Result)
	}

	newExec := WorkflowExecution{
		ExecutionID:  exec.ExecutionID,
		GraphID:      exec.GraphID,
		TenantID:     exec.TenantID,
		State:        newExecState,
		ActiveNodeID: newActiveNode,
		Attempts:     newAttempts,
		NodeStates:   newNodeStates,
		StartedAt:    exec.StartedAt,
		UpdatedAt:    now,
	}
	newExec.ExecutionHash = computeExecHash(newExec)

	tr := NewStepTransition(transitionID, exec.ExecutionID, exec.TenantID,
		fromNode.NodeID, newActiveNode, outcome.Result, attempt, outcome.Reason)
	return newExec, tr, nil
}

// InitiateRollback marks the execution as rolling_back and follows a rollback_triggered edge.
// The failed state of the current node is PRESERVED in NodeStates.
func InitiateRollback(graph WorkflowGraph, exec WorkflowExecution, reason, transitionID string) (WorkflowExecution, StepTransition, error) {
	outcome := NodeOutcome{
		NodeID: exec.ActiveNodeID,
		Result: EdgeTypeRollbackTriggered,
		Reason: reason,
	}
	return AdvanceWorkflow(graph, exec, outcome, transitionID)
}

// ─── internal ──────────────────────────────────────────────────────────────

func validateGraphForExecution(g WorkflowGraph, tenantID string) error {
	if g.GraphID == "" {
		return fmt.Errorf("graph must have an ID")
	}
	if g.TenantID != tenantID {
		return fmt.Errorf("graph tenant %q does not match execution tenant %q", g.TenantID, tenantID)
	}
	return nil
}

func initialState(entryNode WorkflowNode) WorkflowExecutionState {
	if IsBlocking(entryNode) {
		return ExecutionStateBlocked
	}
	return ExecutionStateRunning
}

func nextExecState(n WorkflowNode, edgeType EdgeType) WorkflowExecutionState {
	if edgeType == EdgeTypeRollbackTriggered {
		return ExecutionStateRollingBack
	}
	if n.NodeType == NodeTypeRollback {
		return ExecutionStateRolledBack
	}
	if IsBlocking(n) {
		if n.NodeType == NodeTypePause {
			return ExecutionStatePaused
		}
		return ExecutionStateBlocked
	}
	return ExecutionStateRunning
}

func copyNodeStates(m map[string]NodeState) map[string]NodeState {
	out := make(map[string]NodeState, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyAttempts(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// VerifyWorkflowExecution recomputes and compares the execution hash.
func VerifyWorkflowExecution(e WorkflowExecution) (bool, string) {
	expected := computeExecHash(e)
	if e.ExecutionHash != expected {
		return false, fmt.Sprintf("execution hash mismatch: stored=%s computed=%s", e.ExecutionHash, expected)
	}
	return true, ""
}

func computeExecHash(e WorkflowExecution) string {
	h := sha256.New()
	fmt.Fprintf(h, "exec:%s|graph:%s|tenant:%s|state:%s|active:%s|started:%s|updated:%s",
		e.ExecutionID, e.GraphID, e.TenantID, e.State, e.ActiveNodeID, e.StartedAt, e.UpdatedAt)
	return fmt.Sprintf("%x", h.Sum(nil))
}
