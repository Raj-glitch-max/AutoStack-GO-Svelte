package workflow

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// ExecutionPlan is a static analysis of a WorkflowGraph before execution.
// It identifies structural properties without executing any steps.
type ExecutionPlan struct {
	PlanID           string     `json:"plan_id"`
	GraphID          string     `json:"graph_id"`
	TenantID         string     `json:"tenant_id"`
	ReachableNodes   []string   `json:"reachable_nodes"`
	ApprovalGates    []string   `json:"approval_gates"`    // nodeIDs of approval nodes
	RollbackNodes    []string   `json:"rollback_nodes"`    // nodeIDs of rollback nodes
	PausePoints      []string   `json:"pause_points"`      // nodeIDs of pause nodes
	VerificationPoints []string `json:"verification_points"` // nodeIDs of verification nodes
	CheckpointNodes  []string   `json:"checkpoint_nodes"`  // nodeIDs of checkpoint nodes
	RequiresApproval bool       `json:"requires_approval"`
	HasRollback      bool       `json:"has_rollback"`
	NodeCount        int        `json:"node_count"`
	EdgeCount        int        `json:"edge_count"`
	CreatedAt        string     `json:"created_at"` // excluded from hash
	PlanHash         string     `json:"plan_hash"`
}

// PlanWorkflowExecution performs static analysis of a graph and returns an ExecutionPlan.
// Does not execute any steps. Errors if the graph is invalid.
func PlanWorkflowExecution(graph WorkflowGraph, planID string) (ExecutionPlan, error) {
	if ok, reason := VerifyWorkflowGraph(graph); !ok {
		return ExecutionPlan{}, fmt.Errorf("invalid graph: %s", reason)
	}

	reachable := ReachableNodeIDs(graph)

	var approvalGates, rollbackNodes, pausePoints, verificationPoints, checkpointNodes []string
	for _, nodeID := range reachable {
		n, ok := FindNode(graph, nodeID)
		if !ok {
			continue
		}
		switch n.NodeType {
		case NodeTypeApproval:
			approvalGates = append(approvalGates, n.NodeID)
		case NodeTypeRollback:
			rollbackNodes = append(rollbackNodes, n.NodeID)
		case NodeTypePause:
			pausePoints = append(pausePoints, n.NodeID)
		case NodeTypeVerification:
			verificationPoints = append(verificationPoints, n.NodeID)
		case NodeTypeCheckpoint:
			checkpointNodes = append(checkpointNodes, n.NodeID)
		}
	}

	plan := ExecutionPlan{
		PlanID:             planID,
		GraphID:            graph.GraphID,
		TenantID:           graph.TenantID,
		ReachableNodes:     reachable,
		ApprovalGates:      approvalGates,
		RollbackNodes:      rollbackNodes,
		PausePoints:        pausePoints,
		VerificationPoints: verificationPoints,
		CheckpointNodes:    checkpointNodes,
		RequiresApproval:   len(approvalGates) > 0,
		HasRollback:        len(rollbackNodes) > 0,
		NodeCount:          len(graph.Nodes),
		EdgeCount:          len(graph.Edges),
		CreatedAt:          time.Now().UTC().Format(time.RFC3339Nano),
	}
	plan.PlanHash = computePlanHash(plan)
	return plan, nil
}

// VerifyExecutionPlan recomputes the plan hash.
func VerifyExecutionPlan(plan ExecutionPlan) (bool, string) {
	stored := plan.PlanHash
	plan.PlanHash = ""
	expected := computePlanHash(plan)
	if stored != expected {
		return false, fmt.Sprintf("plan %q hash mismatch: stored=%s computed=%s",
			plan.PlanID, stored, expected)
	}
	return true, ""
}

func computePlanHash(p ExecutionPlan) string {
	h := sha256.New()
	fmt.Fprintf(h, "plan:%s|graph:%s|tenant:%s|nodes:%d|edges:%d|reachable:",
		p.PlanID, p.GraphID, p.TenantID, p.NodeCount, p.EdgeCount)
	for _, id := range p.ReachableNodes {
		fmt.Fprintf(h, "%s,", id)
	}
	fmt.Fprintf(h, "|approvals:")
	for _, id := range p.ApprovalGates {
		fmt.Fprintf(h, "%s,", id)
	}
	fmt.Fprintf(h, "|rollbacks:")
	for _, id := range p.RollbackNodes {
		fmt.Fprintf(h, "%s,", id)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
