package workflow

import "fmt"

// SchedulableNode wraps a WorkflowNode with scheduling metadata.
type SchedulableNode struct {
	Node      WorkflowNode
	IsBlocked bool   // true when node requires approval or is paused
	BlockedOn string // human-readable reason for blocking
	Attempt   int    // which attempt number this would be
}

// ScheduleNext returns the node(s) ready to execute given the current execution state.
// In this deterministic engine, at most one node is schedulable at a time.
// Returns an empty slice when: execution is complete/failed/rolled_back, or no active node.
func ScheduleNext(graph WorkflowGraph, exec WorkflowExecution) ([]SchedulableNode, error) {
	switch exec.State {
	case ExecutionStateCompleted, ExecutionStateFailed, ExecutionStateRolledBack:
		return nil, nil // terminal — nothing to schedule
	}
	if exec.ActiveNodeID == "" {
		return nil, nil
	}
	node, ok := FindNode(graph, exec.ActiveNodeID)
	if !ok {
		return nil, fmt.Errorf("active node %q not found in graph %q", exec.ActiveNodeID, graph.GraphID)
	}

	attempt := exec.Attempts[node.NodeID]
	if attempt == 0 {
		attempt = 1
	}

	sn := SchedulableNode{
		Node:    node,
		Attempt: attempt,
	}
	if IsBlocking(node) {
		sn.IsBlocked = true
		if node.NodeType == NodeTypeApproval {
			sn.BlockedOn = "waiting for approval decision"
		} else {
			sn.BlockedOn = "execution paused"
		}
	}
	return []SchedulableNode{sn}, nil
}

// IsExecutionTerminal returns true when the execution is in a state that cannot advance.
func IsExecutionTerminal(exec WorkflowExecution) bool {
	switch exec.State {
	case ExecutionStateCompleted, ExecutionStateFailed, ExecutionStateRolledBack:
		return true
	}
	return false
}

// MaxAttemptsReached returns true when the node has been attempted MaxAttempts or more times.
// Returns false when MaxAttempts is 0 (unlimited).
func MaxAttemptsReached(exec WorkflowExecution, node WorkflowNode) bool {
	if node.MaxAttempts == 0 {
		return false
	}
	return exec.Attempts[node.NodeID] >= node.MaxAttempts
}
