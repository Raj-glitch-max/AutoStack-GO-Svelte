// Package workflow implements Phase 5.4 Deterministic Workflow Graph Engine.
//
// Core invariants:
//   - Traversal is deterministic for the same graph + transition history.
//   - All state changes are recorded as immutable StepTransition records.
//   - Rollback NEVER erases failed state — it extends the execution record.
//   - Replay uses only recorded transitions — no synthetic reconstruction.
//   - No speculative execution, no hidden retries.
package workflow

// NodeType identifies the kind of work a workflow node performs.
type NodeType string

const (
	NodeTypeAction       NodeType = "action"
	NodeTypeApproval     NodeType = "approval"
	NodeTypeVerification NodeType = "verification"
	NodeTypeCheckpoint   NodeType = "checkpoint"
	NodeTypeConditional  NodeType = "conditional"
	NodeTypeRollback     NodeType = "rollback"
	NodeTypePause        NodeType = "pause"
	NodeTypeNotification NodeType = "notification"
)

// NodeState tracks execution progress for one node within a workflow execution.
type NodeState string

const (
	NodeStatePending   NodeState = "pending"
	NodeStateActive    NodeState = "active"
	NodeStateCompleted NodeState = "completed"
	NodeStateFailed    NodeState = "failed"
	NodeStateSkipped   NodeState = "skipped"
	NodeStateBlocked   NodeState = "blocked" // waiting for approval or pause resume
)

// WorkflowNode is a vertex in the workflow graph.
// Immutable once the graph is built.
type WorkflowNode struct {
	NodeID      string            `json:"node_id"`
	NodeType    NodeType          `json:"node_type"`
	Label       string            `json:"label"`
	MaxAttempts int               `json:"max_attempts"` // 0 = unlimited
	TimeoutSecs int64             `json:"timeout_secs"` // 0 = no timeout
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// IsTerminal returns true when a node type signals the end of a workflow branch.
// Terminal node types have no expected outgoing edges (the engine stops after them).
func IsTerminal(n WorkflowNode) bool {
	return false // terminal-ness is determined by the absence of outgoing edges
}

// RequiresApproval returns true when the node type needs an explicit approval decision
// before the workflow can continue.
func RequiresApproval(n WorkflowNode) bool {
	return n.NodeType == NodeTypeApproval
}

// IsBlocking returns true when reaching this node should pause execution.
func IsBlocking(n WorkflowNode) bool {
	return n.NodeType == NodeTypeApproval || n.NodeType == NodeTypePause
}
