package persistence

import (
	"errors"
	"fmt"
	"time"

	"github.com/janlauber/one-click/pkg/executor"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/daos"
	"github.com/pocketbase/pocketbase/models"
)

const colApprovals = "execution_approvals"

// PocketBaseApprovalStore implements executor.ApprovalStore using PocketBase.
// Each gate decision (approve or reject) is written as an immutable record.
// GetDecision returns the most recent decision for a gate.
type PocketBaseApprovalStore struct {
	dao *daos.Dao
}

// Compile-time interface check.
var _ executor.ApprovalStore = (*PocketBaseApprovalStore)(nil)

// NewPocketBaseApprovalStore constructs a PocketBase-backed approval store.
func NewPocketBaseApprovalStore(dao *daos.Dao) *PocketBaseApprovalStore {
	if dao == nil {
		panic("persistence: PocketBaseApprovalStore requires a non-nil Dao")
	}
	return &PocketBaseApprovalStore{dao: dao}
}

// RecordDecision persists an operator approval or rejection decision.
func (s *PocketBaseApprovalStore) RecordDecision(d executor.ApprovalDecision) error {
	col, err := s.dao.FindCollectionByNameOrId(colApprovals)
	if err != nil {
		return fmt.Errorf("persistence: approvals RecordDecision: collection not found: %w", err)
	}
	r := models.NewRecord(col)
	r.Set("execution_id", d.ExecutionID)
	r.Set("gate_id", d.GateID)
	r.Set("stage_id", d.StageID)
	r.Set("approved", d.Approved)
	r.Set("operator_id", d.OperatorID)
	r.Set("operator_role", d.OperatorRole)
	r.Set("reason", d.Reason)
	ts := d.Timestamp
	if ts == "" {
		ts = time.Now().UTC().Format(time.RFC3339)
	}
	r.Set("timestamp", ts)
	return s.dao.SaveRecord(r)
}

// GetDecision returns the most recent decision for the given execution+gate pair.
// Returns nil without error if no decision has been recorded yet.
func (s *PocketBaseApprovalStore) GetDecision(executionID, gateID string) (*executor.ApprovalDecision, error) {
	record, err := s.dao.FindFirstRecordByFilter(
		colApprovals,
		"execution_id = {:eid} && gate_id = {:gid}",
		dbx.Params{"eid": executionID, "gid": gateID},
	)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("persistence: approvals GetDecision failed: %w", err)
	}
	d := &executor.ApprovalDecision{
		ExecutionID:  record.GetString("execution_id"),
		GateID:       record.GetString("gate_id"),
		StageID:      record.GetString("stage_id"),
		Approved:     record.GetBool("approved"),
		OperatorID:   record.GetString("operator_id"),
		OperatorRole: record.GetString("operator_role"),
		Reason:       record.GetString("reason"),
		Timestamp:    record.GetString("timestamp"),
	}
	return d, nil
}

// ListDecisions returns all decisions for the given execution.
func (s *PocketBaseApprovalStore) ListDecisions(executionID string) ([]executor.ApprovalDecision, error) {
	records, err := s.dao.FindRecordsByFilter(
		colApprovals,
		"execution_id = {:id}",
		"+timestamp",
		0, 0,
		dbx.Params{"id": executionID},
	)
	if err != nil {
		return nil, fmt.Errorf("persistence: approvals ListDecisions failed: %w", err)
	}
	decisions := make([]executor.ApprovalDecision, 0, len(records))
	for _, r := range records {
		decisions = append(decisions, executor.ApprovalDecision{
			ExecutionID:  r.GetString("execution_id"),
			GateID:       r.GetString("gate_id"),
			StageID:      r.GetString("stage_id"),
			Approved:     r.GetBool("approved"),
			OperatorID:   r.GetString("operator_id"),
			OperatorRole: r.GetString("operator_role"),
			Reason:       r.GetString("reason"),
			Timestamp:    r.GetString("timestamp"),
		})
	}
	return decisions, nil
}

// isNotFound returns true if the error is a PocketBase "record not found" error.
// PocketBase returns sql.ErrNoRows in wrapped form for FindFirst... queries.
func isNotFound(err error) bool {
	// PocketBase wraps sql.ErrNoRows; check message as a fallback.
	var target interface{ Error() string }
	if errors.As(err, &target) {
		msg := target.Error()
		return msg == "sql: no rows in result set"
	}
	return false
}
