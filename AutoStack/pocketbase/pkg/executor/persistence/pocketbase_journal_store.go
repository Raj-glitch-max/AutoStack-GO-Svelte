package persistence

import (
	"fmt"

	"github.com/janlauber/one-click/pkg/executor"
	"github.com/janlauber/one-click/pkg/orchestrator"
	"github.com/janlauber/one-click/pkg/planner"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/daos"
	"github.com/pocketbase/pocketbase/models"
)

const (
	colJournals    = "execution_journals"
	colTransitions = "execution_transitions"
)

// PocketBaseJournalStore implements executor.JournalStore using PocketBase collections.
// It persists both journal entries (one per node action) and FSM transitions.
//
// Durable invariants:
//   - Entries are appended only — never updated or deleted.
//   - Transitions are appended only — never updated or deleted.
//   - List() returns entries sorted by timestamp ascending (replay order).
type PocketBaseJournalStore struct {
	dao *daos.Dao
}

// Compile-time interface check.
var _ executor.JournalStore = (*PocketBaseJournalStore)(nil)

// NewPocketBaseJournalStore constructs a store backed by PocketBase.
// The dao argument must not be nil.
func NewPocketBaseJournalStore(dao *daos.Dao) *PocketBaseJournalStore {
	if dao == nil {
		panic("persistence: PocketBaseJournalStore requires a non-nil Dao")
	}
	return &PocketBaseJournalStore{dao: dao}
}

// Append persists one journal entry. Entries are immutable once written.
func (s *PocketBaseJournalStore) Append(entry orchestrator.ExecutionJournalEntry) error {
	col, err := s.dao.FindCollectionByNameOrId(colJournals)
	if err != nil {
		return fmt.Errorf("persistence: journal Append: collection not found: %w", err)
	}
	r := models.NewRecord(col)
	r.Set("execution_id", entry.ExecutionID)
	r.Set("plan_hash", entry.PlanHash)
	r.Set("execution_plan_hash", entry.ExecutionPlanHash)
	r.Set("stage_id", entry.StageID)
	r.Set("node_id", entry.NodeID)
	r.Set("intended_action", string(entry.IntendedAction))
	r.Set("timestamp", entry.Timestamp)
	r.Set("expected_outcome", entry.ExpectedOutcome)
	r.Set("failure_mode", string(entry.FailureMode))
	r.Set("rollback_reference", entry.RollbackReference)
	r.Set("compensation_reference", entry.CompensationReference)
	r.Set("human_approval_ref", entry.HumanApprovalRef)
	r.Set("outcome", string(entry.Outcome))
	r.Set("note", entry.Note)
	return s.dao.SaveRecord(r)
}

// List returns all journal entries for the given execution, sorted by timestamp ascending.
func (s *PocketBaseJournalStore) List(executionID string) ([]orchestrator.ExecutionJournalEntry, error) {
	records, err := s.dao.FindRecordsByFilter(
		colJournals,
		"execution_id = {:id}",
		"+timestamp",
		0, 0,
		dbx.Params{"id": executionID},
	)
	if err != nil {
		return nil, fmt.Errorf("persistence: journal List failed: %w", err)
	}
	entries := make([]orchestrator.ExecutionJournalEntry, 0, len(records))
	for _, r := range records {
		entries = append(entries, orchestrator.ExecutionJournalEntry{
			ExecutionID:           r.GetString("execution_id"),
			PlanHash:              r.GetString("plan_hash"),
			ExecutionPlanHash:     r.GetString("execution_plan_hash"),
			StageID:               r.GetString("stage_id"),
			NodeID:                r.GetString("node_id"),
			IntendedAction:        planner.ActionType(r.GetString("intended_action")),
			Timestamp:             r.GetString("timestamp"),
			ExpectedOutcome:       r.GetString("expected_outcome"),
			FailureMode:           orchestrator.FailureClass(r.GetString("failure_mode")),
			RollbackReference:     r.GetString("rollback_reference"),
			CompensationReference: r.GetString("compensation_reference"),
			HumanApprovalRef:      r.GetString("human_approval_ref"),
			Outcome:               orchestrator.ExecutionJournalOutcome(r.GetString("outcome")),
			Note:                  r.GetString("note"),
		})
	}
	return entries, nil
}

// AppendTransition persists one FSM state transition before it is applied.
func (s *PocketBaseJournalStore) AppendTransition(t executor.ExecutionTransition) error {
	col, err := s.dao.FindCollectionByNameOrId(colTransitions)
	if err != nil {
		return fmt.Errorf("persistence: transitions AppendTransition: collection not found: %w", err)
	}
	r := models.NewRecord(col)
	r.Set("execution_id", t.ExecutionID)
	r.Set("from_state", string(t.FromState))
	r.Set("to_state", string(t.ToState))
	r.Set("reason", t.Reason)
	r.Set("timestamp", t.Timestamp)
	r.Set("actor_id", t.ActorID)
	return s.dao.SaveRecord(r)
}

// ListTransitions returns all FSM transitions for the given execution, sorted by timestamp ascending.
func (s *PocketBaseJournalStore) ListTransitions(executionID string) ([]executor.ExecutionTransition, error) {
	records, err := s.dao.FindRecordsByFilter(
		colTransitions,
		"execution_id = {:id}",
		"+timestamp",
		0, 0,
		dbx.Params{"id": executionID},
	)
	if err != nil {
		return nil, fmt.Errorf("persistence: transitions ListTransitions failed: %w", err)
	}
	transitions := make([]executor.ExecutionTransition, 0, len(records))
	for _, r := range records {
		transitions = append(transitions, executor.ExecutionTransition{
			ExecutionID: r.GetString("execution_id"),
			FromState:   executor.ExecutionState(r.GetString("from_state")),
			ToState:     executor.ExecutionState(r.GetString("to_state")),
			Reason:      r.GetString("reason"),
			Timestamp:   r.GetString("timestamp"),
			ActorID:     r.GetString("actor_id"),
		})
	}
	return transitions, nil
}
