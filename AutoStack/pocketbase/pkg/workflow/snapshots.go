package workflow

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// WorkflowCheckpoint preserves execution state at a stable recovery point.
// CheckpointHash covers all stable fields.
type WorkflowCheckpoint struct {
	CheckpointID    string             `json:"checkpoint_id"`
	ExecutionID     string             `json:"execution_id"`
	AtNodeID        string             `json:"at_node_id"`
	AtTransitionIdx int                `json:"at_transition_idx"`
	NodeStates      map[string]NodeState `json:"node_states"`
	RetryCounters   map[string]int     `json:"retry_counters"`
	ApprovalWaits   []string           `json:"approval_waits"`
	CreatedAt       string             `json:"created_at"`
	CheckpointHash  string             `json:"checkpoint_hash"`
}

// WorkflowSnapshot is a tamper-evident export of complete workflow execution state.
// GeneratedAt is excluded from SnapshotHash (content-identity).
type WorkflowSnapshot struct {
	SnapshotID   string               `json:"snapshot_id"`
	ExecutionID  string               `json:"execution_id"`
	TenantID     string               `json:"tenant_id"`
	Graph        WorkflowGraph        `json:"graph"`
	Execution    WorkflowExecution    `json:"execution"`
	Transitions  []StepTransition     `json:"transitions"`
	Checkpoints  []WorkflowCheckpoint `json:"checkpoints"`
	GeneratedAt  string               `json:"generated_at"` // excluded from hash
	SnapshotHash string               `json:"snapshot_hash"`
}

// BuildWorkflowCheckpoint creates a checkpoint from the current execution state.
func BuildWorkflowCheckpoint(
	checkpointID string,
	exec WorkflowExecution,
	transitionIdx int,
) WorkflowCheckpoint {
	// Identify approval-blocked nodes.
	var approvalWaits []string
	for nodeID, state := range exec.NodeStates {
		if state == NodeStateBlocked {
			approvalWaits = append(approvalWaits, nodeID)
		}
	}
	cp := WorkflowCheckpoint{
		CheckpointID:    checkpointID,
		ExecutionID:     exec.ExecutionID,
		AtNodeID:        exec.ActiveNodeID,
		AtTransitionIdx: transitionIdx,
		NodeStates:      copyNodeStates(exec.NodeStates),
		RetryCounters:   copyAttempts(exec.Attempts),
		ApprovalWaits:   approvalWaits,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	cp.CheckpointHash = computeCheckpointHash(cp)
	return cp
}

// VerifyWorkflowCheckpoint recomputes the hash and returns (true, "") when it matches.
func VerifyWorkflowCheckpoint(cp WorkflowCheckpoint) (bool, string) {
	stored := cp.CheckpointHash
	cp.CheckpointHash = ""
	expected := computeCheckpointHash(cp)
	if stored != expected {
		return false, fmt.Sprintf("checkpoint %q hash mismatch: stored=%s computed=%s",
			cp.CheckpointID, stored, expected)
	}
	return true, ""
}

// BuildWorkflowSnapshot creates a tamper-evident snapshot of the full execution state.
func BuildWorkflowSnapshot(
	snapshotID string,
	graph WorkflowGraph,
	exec WorkflowExecution,
	transitions []StepTransition,
	checkpoints []WorkflowCheckpoint,
) WorkflowSnapshot {
	trCopy := make([]StepTransition, len(transitions))
	copy(trCopy, transitions)
	cpCopy := make([]WorkflowCheckpoint, len(checkpoints))
	copy(cpCopy, checkpoints)

	snap := WorkflowSnapshot{
		SnapshotID:  snapshotID,
		ExecutionID: exec.ExecutionID,
		TenantID:    exec.TenantID,
		Graph:       graph,
		Execution:   exec,
		Transitions: trCopy,
		Checkpoints: cpCopy,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	snap.SnapshotHash = computeWorkflowSnapshotHash(snap)
	return snap
}

// VerifyWorkflowSnapshot recomputes the hash and returns (true, "") when it matches.
func VerifyWorkflowSnapshot(snap WorkflowSnapshot) (bool, string) {
	stored := snap.SnapshotHash
	snap.SnapshotHash = ""
	expected := computeWorkflowSnapshotHash(snap)
	if stored != expected {
		return false, fmt.Sprintf("snapshot %q hash mismatch: stored=%s computed=%s",
			snap.SnapshotID, stored, expected)
	}
	return true, ""
}

// ─── internal ──────────────────────────────────────────────────────────────

func computeCheckpointHash(cp WorkflowCheckpoint) string {
	h := sha256.New()
	fmt.Fprintf(h, "cp:%s|exec:%s|node:%s|idx:%d|ts:%s",
		cp.CheckpointID, cp.ExecutionID, cp.AtNodeID, cp.AtTransitionIdx, cp.CreatedAt)
	for _, w := range cp.ApprovalWaits {
		fmt.Fprintf(h, "|wait:%s", w)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func computeWorkflowSnapshotHash(snap WorkflowSnapshot) string {
	h := sha256.New()
	fmt.Fprintf(h, "snap:%s|exec:%s|tenant:%s|graph:%s|state:%s|transitions:%d|checkpoints:%d",
		snap.SnapshotID, snap.ExecutionID, snap.TenantID,
		snap.Graph.GraphHash, snap.Execution.ExecutionHash,
		len(snap.Transitions), len(snap.Checkpoints))
	for _, tr := range snap.Transitions {
		fmt.Fprintf(h, "|tr:%s/%s", tr.TransitionID, tr.Hash)
	}
	for _, cp := range snap.Checkpoints {
		fmt.Fprintf(h, "|cp:%s/%s", cp.CheckpointID, cp.CheckpointHash)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
