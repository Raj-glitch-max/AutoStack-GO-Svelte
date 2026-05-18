package executor

import (
	"fmt"

	"github.com/janlauber/one-click/pkg/orchestrator"
)

// ReplayState is the reconstructed execution state derived from persisted records.
// Replay is read-only — it does NOT re-execute any completed actions.
type ReplayState struct {
	ExecutionID       string
	State             ExecutionState
	CurrentStageIndex int
	// CompletedNodes maps "stageID:nodeID" → true for nodes confirmed complete.
	CompletedNodes   map[string]bool
	// PendingApprovals lists gate IDs that have not yet been approved.
	PendingApprovals []string
	// AmbiguityIDs lists unresolved ambiguity IDs from the store.
	AmbiguityIDs     []string
	Transitions      []ExecutionTransition
}

// ReplayExecution reconstructs execution state from persisted journal and store data.
// It is called after a process restart to resume a previously started execution.
//
// Rules:
//   - Replay must NOT duplicate completed actions.
//   - Replay must preserve the ordering from transitions.
//   - Replay is a read-only reconstruction — no writes, no state mutations.
//   - The caller inspects ReplayState and decides what operator action is required.
func ReplayExecution(
	executionID string,
	snapshot *orchestrator.ExecutionSnapshot,
	journal JournalStore,
	approvals ApprovalStore,
	ambiguityStore AmbiguityStore,
) (*ReplayState, error) {
	if executionID == "" {
		return nil, fmt.Errorf("replay: executionID must be non-empty")
	}
	if snapshot == nil {
		return nil, fmt.Errorf("replay: snapshot must not be nil")
	}
	if snapshot.Plan == nil {
		return nil, fmt.Errorf("replay: snapshot has no plan")
	}

	rs := &ReplayState{
		ExecutionID:    executionID,
		State:          StatePending,
		CompletedNodes: make(map[string]bool),
	}

	// Reconstruct FSM state from the last persisted transition.
	transitions, err := journal.ListTransitions(executionID)
	if err != nil {
		return nil, fmt.Errorf("replay: failed to list transitions: %w", err)
	}
	rs.Transitions = transitions
	for _, t := range transitions {
		rs.State = t.ToState
	}

	// Reconstruct completed nodes from journal entries.
	entries, err := journal.List(executionID)
	if err != nil {
		return nil, fmt.Errorf("replay: failed to list journal entries: %w", err)
	}
	for _, e := range entries {
		if e.Outcome == orchestrator.JournalOutcomeSuccess && e.StageID != "" && e.NodeID != "" {
			rs.CompletedNodes[e.StageID+":"+e.NodeID] = true
		}
	}

	// Reconstruct current stage index: find the first stage not fully complete.
	stages := snapshot.Plan.Stages
	rs.CurrentStageIndex = 0
	for i, stage := range stages {
		allComplete := true
		for _, node := range stage.Nodes {
			if !rs.CompletedNodes[stage.StageID+":"+node.NodeID] {
				allComplete = false
				break
			}
		}
		if allComplete && len(stage.Nodes) > 0 {
			rs.CurrentStageIndex = i + 1
		} else {
			break
		}
	}

	// Reconstruct pending approvals: gates not yet approved.
	for _, gate := range snapshot.Plan.ApprovalGates {
		approved, err := IsGateApproved(approvals, executionID, gate.GateID)
		if err != nil {
			return nil, fmt.Errorf("replay: approval check failed for gate %s: %w", gate.GateID, err)
		}
		if !approved {
			rs.PendingApprovals = append(rs.PendingApprovals, gate.GateID)
		}
	}

	// Reconstruct unresolved ambiguities.
	ambiguities, err := ambiguityStore.ListUnresolved(executionID)
	if err != nil {
		return nil, fmt.Errorf("replay: ambiguity list failed: %w", err)
	}
	for _, a := range ambiguities {
		rs.AmbiguityIDs = append(rs.AmbiguityIDs, a.AmbiguityID)
	}

	return rs, nil
}
