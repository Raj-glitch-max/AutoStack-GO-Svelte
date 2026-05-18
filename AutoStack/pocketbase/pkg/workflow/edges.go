package workflow

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// EdgeType identifies the condition under which a workflow edge is traversed.
type EdgeType string

const (
	EdgeTypeSuccess           EdgeType = "success"
	EdgeTypeFailure           EdgeType = "failure"
	EdgeTypeContradiction     EdgeType = "contradiction"
	EdgeTypeTimeout           EdgeType = "timeout"
	EdgeTypeApprovalDenied    EdgeType = "approval_denied"
	EdgeTypeRollbackTriggered EdgeType = "rollback_triggered"
)

// WorkflowEdge is a directed connection between two nodes in the workflow graph.
type WorkflowEdge struct {
	EdgeID   string   `json:"edge_id"`
	FromNode string   `json:"from_node"`
	ToNode   string   `json:"to_node"`
	EdgeType EdgeType `json:"edge_type"`
	Label    string   `json:"label,omitempty"`
}

// StepTransition is an immutable record of one state change in a workflow execution.
// It forms the append-only audit trail of how execution progressed.
type StepTransition struct {
	TransitionID   string   `json:"transition_id"`
	ExecutionID    string   `json:"execution_id"`
	TenantID       string   `json:"tenant_id"`
	FromNodeID     string   `json:"from_node_id"` // empty for the initial transition
	ToNodeID       string   `json:"to_node_id"`
	EdgeType       EdgeType `json:"edge_type"`
	Attempt        int      `json:"attempt"`
	Reason         string   `json:"reason,omitempty"`
	TransitionedAt string   `json:"transitioned_at"`
	Hash           string   `json:"hash"`
}

// NewStepTransition creates an immutable StepTransition with its hash computed.
func NewStepTransition(
	transitionID, executionID, tenantID string,
	fromNodeID, toNodeID string,
	edgeType EdgeType,
	attempt int,
	reason string,
) StepTransition {
	tr := StepTransition{
		TransitionID:   transitionID,
		ExecutionID:    executionID,
		TenantID:       tenantID,
		FromNodeID:     fromNodeID,
		ToNodeID:       toNodeID,
		EdgeType:       edgeType,
		Attempt:        attempt,
		Reason:         reason,
		TransitionedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	tr.Hash = computeTransitionHash(tr)
	return tr
}

// VerifyStepTransition recomputes the hash and returns (true, "") when it matches.
func VerifyStepTransition(tr StepTransition) (bool, string) {
	stored := tr.Hash
	tr.Hash = ""
	expected := computeTransitionHash(tr)
	if stored != expected {
		return false, fmt.Sprintf("transition %q hash mismatch: stored=%s computed=%s",
			tr.TransitionID, stored, expected)
	}
	return true, ""
}

func computeTransitionHash(tr StepTransition) string {
	h := sha256.New()
	fmt.Fprintf(h, "tr:%s|exec:%s|tenant:%s|from:%s|to:%s|edge:%s|attempt:%d|ts:%s|reason:%s",
		tr.TransitionID, tr.ExecutionID, tr.TenantID,
		tr.FromNodeID, tr.ToNodeID, tr.EdgeType,
		tr.Attempt, tr.TransitionedAt, tr.Reason,
	)
	return fmt.Sprintf("%x", h.Sum(nil))
}
