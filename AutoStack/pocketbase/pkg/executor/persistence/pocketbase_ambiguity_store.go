package persistence

import (
	"fmt"
	"time"

	"github.com/janlauber/one-click/pkg/executor"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/daos"
	"github.com/pocketbase/pocketbase/models"
)

const colAmbiguities = "execution_ambiguities"

// PocketBaseAmbiguityStore implements executor.AmbiguityStore using PocketBase.
// Ambiguity records are written once on detection, and updated once on resolution.
// They are never deleted — resolution is a field update, not a row removal.
type PocketBaseAmbiguityStore struct {
	dao *daos.Dao
}

// Compile-time interface check.
var _ executor.AmbiguityStore = (*PocketBaseAmbiguityStore)(nil)

// NewPocketBaseAmbiguityStore constructs a PocketBase-backed ambiguity store.
func NewPocketBaseAmbiguityStore(dao *daos.Dao) *PocketBaseAmbiguityStore {
	if dao == nil {
		panic("persistence: PocketBaseAmbiguityStore requires a non-nil Dao")
	}
	return &PocketBaseAmbiguityStore{dao: dao}
}

// Record persists a new ambiguity record. The AmbiguityID must be unique.
func (s *PocketBaseAmbiguityStore) Record(a executor.ExecutionAmbiguity) error {
	col, err := s.dao.FindCollectionByNameOrId(colAmbiguities)
	if err != nil {
		return fmt.Errorf("persistence: ambiguity Record: collection not found: %w", err)
	}
	r := models.NewRecord(col)
	r.Set("ambiguity_id", a.AmbiguityID)
	r.Set("execution_id", a.ExecutionID)
	r.Set("stage_id", a.StageID)
	r.Set("node_id", a.NodeID)
	r.Set("kind", a.Kind)
	r.Set("description", a.Description)
	r.Set("provider_state", a.ProviderState)
	r.Set("detected_at", a.DetectedAt)
	r.Set("resolved", false)
	r.Set("resolution", "")
	r.Set("resolved_at", "")
	r.Set("operator_id", "")
	return s.dao.SaveRecord(r)
}

// Get returns the ambiguity record for the given ambiguityID.
func (s *PocketBaseAmbiguityStore) Get(ambiguityID string) (*executor.ExecutionAmbiguity, error) {
	record, err := s.dao.FindFirstRecordByFilter(
		colAmbiguities,
		"ambiguity_id = {:id}",
		dbx.Params{"id": ambiguityID},
	)
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("persistence: ambiguity %q not found", ambiguityID)
		}
		return nil, fmt.Errorf("persistence: ambiguity Get failed: %w", err)
	}
	return ambiguityFromRecord(record), nil
}

// ListUnresolved returns all unresolved ambiguity records for the given execution.
func (s *PocketBaseAmbiguityStore) ListUnresolved(executionID string) ([]executor.ExecutionAmbiguity, error) {
	records, err := s.dao.FindRecordsByFilter(
		colAmbiguities,
		"execution_id = {:id} && resolved = false",
		"+detected_at",
		0, 0,
		dbx.Params{"id": executionID},
	)
	if err != nil {
		return nil, fmt.Errorf("persistence: ambiguity ListUnresolved failed: %w", err)
	}
	ambiguities := make([]executor.ExecutionAmbiguity, 0, len(records))
	for _, r := range records {
		ambiguities = append(ambiguities, *ambiguityFromRecord(r))
	}
	return ambiguities, nil
}

// Resolve marks an ambiguity record as resolved by the given operator.
func (s *PocketBaseAmbiguityStore) Resolve(ambiguityID, operatorID, resolution string) error {
	record, err := s.dao.FindFirstRecordByFilter(
		colAmbiguities,
		"ambiguity_id = {:id}",
		dbx.Params{"id": ambiguityID},
	)
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("persistence: ambiguity %q not found for resolution", ambiguityID)
		}
		return fmt.Errorf("persistence: ambiguity Resolve lookup failed: %w", err)
	}
	record.Set("resolved", true)
	record.Set("resolution", resolution)
	record.Set("resolved_at", time.Now().UTC().Format(time.RFC3339))
	record.Set("operator_id", operatorID)
	return s.dao.SaveRecord(record)
}

func ambiguityFromRecord(r *models.Record) *executor.ExecutionAmbiguity {
	return &executor.ExecutionAmbiguity{
		AmbiguityID:   r.GetString("ambiguity_id"),
		ExecutionID:   r.GetString("execution_id"),
		StageID:       r.GetString("stage_id"),
		NodeID:        r.GetString("node_id"),
		Kind:          r.GetString("kind"),
		Description:   r.GetString("description"),
		ProviderState: r.GetString("provider_state"),
		DetectedAt:    r.GetString("detected_at"),
		Resolved:      r.GetBool("resolved"),
		Resolution:    r.GetString("resolution"),
		ResolvedAt:    r.GetString("resolved_at"),
		OperatorID:    r.GetString("operator_id"),
	}
}
