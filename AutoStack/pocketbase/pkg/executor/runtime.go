package executor

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/janlauber/one-click/pkg/orchestrator"
)

// ExecutionRuntime manages the lifecycle of a single controlled execution.
//
// INVARIANTS:
//   - Every state transition is persisted to the journal BEFORE being applied.
//   - Approval gates block stage execution until the operator satisfies them.
//   - Ambiguity transitions immediately to StateAmbiguous — no silent continuation.
//   - Rollback and compensation are NEVER triggered automatically — only recommended.
//   - The snapshot is immutable; the runtime never modifies it.
type ExecutionRuntime struct {
	mu sync.Mutex

	executionID       string
	holderID          string
	state             ExecutionState
	snapshot          *orchestrator.ExecutionSnapshot
	currentStageIndex int

	journal   JournalStore
	approvals ApprovalStore
	ambiguity AmbiguityStore
	lease     LeaseStore
	runner    *StageRunner

	stageRecords     []StageExecutionRecord
	transitions      []ExecutionTransition
	rollbackRecs     []RollbackRecommendation
	compensationRecs []CompensationRecommendation
}

// NewExecutionRuntime constructs a runtime for the given snapshot.
// All store arguments must be non-nil.
func NewExecutionRuntime(
	executionID string,
	holderID string,
	snapshot *orchestrator.ExecutionSnapshot,
	journal JournalStore,
	approvals ApprovalStore,
	ambiguity AmbiguityStore,
	lease LeaseStore,
	provider ProviderExecutor,
) (*ExecutionRuntime, error) {
	if executionID == "" {
		return nil, fmt.Errorf("runtime: executionID must be non-empty")
	}
	if snapshot == nil {
		return nil, fmt.Errorf("runtime: snapshot must not be nil")
	}
	if journal == nil {
		return nil, fmt.Errorf("runtime: journal must not be nil")
	}
	if approvals == nil {
		return nil, fmt.Errorf("runtime: approvals must not be nil")
	}
	if ambiguity == nil {
		return nil, fmt.Errorf("runtime: ambiguity must not be nil")
	}
	if lease == nil {
		return nil, fmt.Errorf("runtime: lease must not be nil")
	}
	if provider == nil {
		return nil, fmt.Errorf("runtime: provider must not be nil")
	}

	return &ExecutionRuntime{
		executionID:       executionID,
		holderID:          holderID,
		state:             StatePending,
		snapshot:          snapshot,
		currentStageIndex: 0,
		journal:           journal,
		approvals:         approvals,
		ambiguity:         ambiguity,
		lease:             lease,
		runner:            NewStageRunner(provider),
	}, nil
}

// State returns the current FSM state.
func (rt *ExecutionRuntime) State() ExecutionState {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.state
}

// ExecutionID returns the execution identifier.
func (rt *ExecutionRuntime) ExecutionID() string { return rt.executionID }

// Snapshot returns the immutable execution snapshot.
func (rt *ExecutionRuntime) Snapshot() *orchestrator.ExecutionSnapshot { return rt.snapshot }

// Transitions returns a copy of all recorded state transitions.
func (rt *ExecutionRuntime) Transitions() []ExecutionTransition {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	result := make([]ExecutionTransition, len(rt.transitions))
	copy(result, rt.transitions)
	return result
}

// StageRecords returns a copy of all stage execution records.
func (rt *ExecutionRuntime) StageRecords() []StageExecutionRecord {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	result := make([]StageExecutionRecord, len(rt.stageRecords))
	copy(result, rt.stageRecords)
	return result
}

// CurrentStageIndex returns the index of the next stage to execute.
func (rt *ExecutionRuntime) CurrentStageIndex() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.currentStageIndex
}

// RollbackRecommendations returns all rollback recommendations accumulated so far.
func (rt *ExecutionRuntime) RollbackRecommendations() []RollbackRecommendation {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	result := make([]RollbackRecommendation, len(rt.rollbackRecs))
	copy(result, rt.rollbackRecs)
	return result
}

// CompensationRecommendations returns all compensation recommendations.
func (rt *ExecutionRuntime) CompensationRecommendations() []CompensationRecommendation {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	result := make([]CompensationRecommendation, len(rt.compensationRecs))
	copy(result, rt.compensationRecs)
	return result
}

// Start transitions from StatePending to StateReady and acquires the execution lease.
// Returns error if already started or lease cannot be acquired.
func (rt *ExecutionRuntime) Start() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.state != StatePending {
		return fmt.Errorf("runtime: Start called in state %s, expected %s", rt.state, StatePending)
	}
	if _, err := rt.lease.Acquire(rt.executionID, rt.holderID, DefaultLeaseDuration); err != nil {
		return fmt.Errorf("runtime: lease acquisition failed: %w", err)
	}
	return rt.transition(StateReady, "execution started", rt.holderID)
}

// Pause transitions to StatePaused. Must be called by an operator.
// Allowed from: StateReady, StateExecuting, StateAwaitingApproval.
func (rt *ExecutionRuntime) Pause(operatorID, reason string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	switch rt.state {
	case StateReady, StateExecuting, StateAwaitingApproval:
	default:
		return fmt.Errorf("runtime: Pause not allowed in state %s", rt.state)
	}
	if operatorID == "" {
		return fmt.Errorf("runtime: Pause requires a non-empty operatorID")
	}
	return rt.transition(StatePaused, "manual pause: "+reason, operatorID)
}

// Resume transitions from StatePaused to StateReady.
func (rt *ExecutionRuntime) Resume(operatorID string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.state != StatePaused {
		return fmt.Errorf("runtime: Resume called in state %s, expected %s", rt.state, StatePaused)
	}
	if operatorID == "" {
		return fmt.Errorf("runtime: Resume requires a non-empty operatorID")
	}
	return rt.transition(StateReady, "manual resume by "+operatorID, operatorID)
}

// Advance attempts to run the next pending stage.
// Allowed from: StateReady, StateAwaitingApproval.
//
// If approval gates for the current stage are not satisfied:
//   - Transitions to StateAwaitingApproval (if not already there) and returns nil.
//
// If gates are satisfied (or none required):
//   - Executes the stage.
//   - Transitions to StateCompleted, StateAmbiguous, StateRollbackRequired, or StateReady
//     depending on outcome.
//
// The caller is responsible for checking State() after Advance returns and
// deciding what operator action is required next. Advance never auto-continues.
func (rt *ExecutionRuntime) Advance(ctx context.Context) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.state != StateReady && rt.state != StateAwaitingApproval {
		return fmt.Errorf("runtime: Advance not allowed in state %s", rt.state)
	}

	stages := rt.snapshot.Plan.Stages
	if rt.currentStageIndex >= len(stages) {
		return rt.transition(StateCompleted, "all stages complete", "system")
	}

	stage := stages[rt.currentStageIndex]

	// Check approval gates for this stage.
	gateIDs := gateIDsForStage(rt.snapshot.Plan.ApprovalGates, stage.StageID)
	if len(gateIDs) > 0 {
		approved, err := AllGatesApproved(rt.approvals, rt.executionID, gateIDs)
		if err != nil {
			return fmt.Errorf("runtime: approval check failed: %w", err)
		}
		if !approved {
			if rt.state != StateAwaitingApproval {
				return rt.transition(StateAwaitingApproval, "stage "+stage.StageID+" requires approval", "system")
			}
			return nil // already waiting; operator has not yet approved
		}
		// Gates satisfied. If we were awaiting, return to Ready before executing.
		if rt.state == StateAwaitingApproval {
			if err := rt.transition(StateReady, "all gates approved for "+stage.StageID, "system"); err != nil {
				return err
			}
		}
	}

	if err := rt.transition(StateExecuting, "executing stage "+stage.StageID, "system"); err != nil {
		return err
	}

	rec, _ := rt.runner.RunStage(ctx, rt.executionID, stage, rt.journal, rt.ambiguity)
	rt.stageRecords = append(rt.stageRecords, rec)

	switch rec.Outcome {
	case StageOutcomeSuccess:
		rt.currentStageIndex++
		if rt.currentStageIndex >= len(stages) {
			return rt.transition(StateCompleted, "all stages complete", "system")
		}
		return rt.transition(StateReady, "stage "+stage.StageID+" complete", "system")

	case StageOutcomeAmbiguous:
		rt.appendRollbackRec(stage, "provider ambiguity in stage "+stage.StageID)
		return rt.transition(StateAmbiguous, "ambiguity detected in stage "+stage.StageID, "system")

	case StageOutcomeFailed:
		rt.appendRollbackRec(stage, "stage "+stage.StageID+" failed")
		return rt.transition(StateRollbackRequired, "stage "+stage.StageID+" failed — operator action required", "system")

	default:
		return rt.transition(StateReady, "stage "+stage.StageID+" outcome: "+string(rec.Outcome), "system")
	}
}

// ResolveAmbiguity transitions from StateAmbiguous to StateReady after operator review.
// The resolution text is stored in the journal.
func (rt *ExecutionRuntime) ResolveAmbiguity(operatorID, resolution string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.state != StateAmbiguous {
		return fmt.Errorf("runtime: ResolveAmbiguity called in state %s, expected %s", rt.state, StateAmbiguous)
	}
	if operatorID == "" {
		return fmt.Errorf("runtime: ResolveAmbiguity requires a non-empty operatorID")
	}
	return rt.transition(StateReady, "operator resolved ambiguity: "+resolution, operatorID)
}

// Cancel transitions to StateCancelled and persists a CancellationRecord.
// Allowed from any non-terminal state. Operator ID and reason are required.
// Cancellation is irreversible — the execution cannot be resumed after cancellation.
func (rt *ExecutionRuntime) Cancel(operatorID, reason string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	switch rt.state {
	case StateCompleted, StateFailed, StateCancelled:
		return fmt.Errorf("runtime: Cancel not allowed in terminal state %s", rt.state)
	}
	if operatorID == "" {
		return fmt.Errorf("runtime: Cancel requires a non-empty operatorID")
	}
	if reason == "" {
		return fmt.Errorf("runtime: Cancel requires a non-empty reason")
	}
	return rt.transition(StateCancelled, "cancelled by "+operatorID+": "+reason, operatorID)
}

// ─────────────────────────────────────────────────────────────────────────
// INTERNAL HELPERS — all called with rt.mu held
// ─────────────────────────────────────────────────────────────────────────

// transition persists and applies a state change atomically.
// Must be called with rt.mu held.
func (rt *ExecutionRuntime) transition(to ExecutionState, reason, actorID string) error {
	t := ExecutionTransition{
		ExecutionID: rt.executionID,
		FromState:   rt.state,
		ToState:     to,
		Reason:      reason,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		ActorID:     actorID,
	}
	// Persist BEFORE applying.
	if err := rt.journal.AppendTransition(t); err != nil {
		return fmt.Errorf("runtime: failed to persist transition %s→%s: %w", rt.state, to, err)
	}
	rt.transitions = append(rt.transitions, t)
	rt.state = to
	return nil
}

func (rt *ExecutionRuntime) appendRollbackRec(stage orchestrator.ExecutionStage, reason string) {
	rollbackRef := ""
	if len(stage.RollbackStageRefs) > 0 {
		rollbackRef = stage.RollbackStageRefs[0]
	}
	rt.rollbackRecs = append(rt.rollbackRecs, RollbackRecommendation{
		RecommendationID: "rollback:" + rt.executionID + ":" + stage.StageID,
		ExecutionID:      rt.executionID,
		StageID:          stage.StageID,
		RollbackStageRef: rollbackRef,
		Reason:           reason,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		OperatorReviewed: false,
	})
}

// gateIDsForStage returns the gate IDs that belong to a specific stage, sorted.
func gateIDsForStage(gates []orchestrator.ApprovalGate, stageID string) []string {
	var ids []string
	for _, g := range gates {
		if g.StageID == stageID {
			ids = append(ids, g.GateID)
		}
	}
	sort.Strings(ids)
	return ids
}
