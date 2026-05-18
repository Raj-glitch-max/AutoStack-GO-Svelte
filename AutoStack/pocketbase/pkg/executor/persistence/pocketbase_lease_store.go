package persistence

import (
	"fmt"
	"time"

	"github.com/janlauber/one-click/pkg/executor"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/daos"
	"github.com/pocketbase/pocketbase/models"
)

const colLeases = "execution_leases"

// PocketBaseLeaseStore implements executor.LeaseStore using PocketBase.
// A lease record is created on Acquire and updated on Renew/Release.
// The unique constraint on execution_id in the DB enforces single-holder semantics.
//
// Limitations:
//   - This is a single-node lease. It does not coordinate across multiple servers.
//   - Expiry is checked at acquire time; no background sweeper is needed.
type PocketBaseLeaseStore struct {
	dao *daos.Dao
}

// Compile-time interface check.
var _ executor.LeaseStore = (*PocketBaseLeaseStore)(nil)

// NewPocketBaseLeaseStore constructs a PocketBase-backed lease store.
func NewPocketBaseLeaseStore(dao *daos.Dao) *PocketBaseLeaseStore {
	if dao == nil {
		panic("persistence: PocketBaseLeaseStore requires a non-nil Dao")
	}
	return &PocketBaseLeaseStore{dao: dao}
}

// Acquire creates or reclaims an expired lease for the given executionID.
// Returns an error if an unexpired lease is held by a different holder.
func (s *PocketBaseLeaseStore) Acquire(executionID, holderID string, duration time.Duration) (*executor.ExecutionLease, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(duration)

	existing, err := s.dao.FindFirstRecordByFilter(
		colLeases,
		"execution_id = {:id}",
		dbx.Params{"id": executionID},
	)
	if err != nil && !isNotFound(err) {
		return nil, fmt.Errorf("persistence: lease Acquire lookup failed: %w", err)
	}

	if existing != nil {
		// Check if the lease is expired or already held by this holder.
		expiry := existing.GetString("expires_at")
		if expiry != "" {
			t, parseErr := time.Parse(time.RFC3339, expiry)
			if parseErr == nil && t.After(now) && existing.GetString("holder_id") != holderID {
				return nil, fmt.Errorf("persistence: execution %q is already leased by %q until %s",
					executionID, existing.GetString("holder_id"), expiry)
			}
		}
		// Reclaim: update in place.
		existing.Set("holder_id", holderID)
		existing.Set("acquired_at", now.Format(time.RFC3339))
		existing.Set("expires_at", expiresAt.Format(time.RFC3339))
		existing.Set("expired", false)
		renewed := existing.GetInt("renewed") + 1
		existing.Set("renewed", renewed)
		if err := s.dao.SaveRecord(existing); err != nil {
			return nil, fmt.Errorf("persistence: lease reclaim failed: %w", err)
		}
		return leaseFromRecord(existing), nil
	}

	// New lease.
	col, err := s.dao.FindCollectionByNameOrId(colLeases)
	if err != nil {
		return nil, fmt.Errorf("persistence: lease Acquire: collection not found: %w", err)
	}
	r := models.NewRecord(col)
	r.Set("execution_id", executionID)
	r.Set("holder_id", holderID)
	r.Set("acquired_at", now.Format(time.RFC3339))
	r.Set("expires_at", expiresAt.Format(time.RFC3339))
	r.Set("renewed", 0)
	r.Set("expired", false)
	if err := s.dao.SaveRecord(r); err != nil {
		return nil, fmt.Errorf("persistence: lease create failed: %w", err)
	}
	return leaseFromRecord(r), nil
}

// Renew extends the expiry of an existing lease held by holderID.
func (s *PocketBaseLeaseStore) Renew(executionID, holderID string, duration time.Duration) (*executor.ExecutionLease, error) {
	record, err := s.findLease(executionID)
	if err != nil {
		return nil, err
	}
	if record.GetString("holder_id") != holderID {
		return nil, fmt.Errorf("persistence: lease Renew: holder mismatch for %q", executionID)
	}
	now := time.Now().UTC()
	record.Set("expires_at", now.Add(duration).Format(time.RFC3339))
	record.Set("renewed", record.GetInt("renewed")+1)
	if err := s.dao.SaveRecord(record); err != nil {
		return nil, fmt.Errorf("persistence: lease Renew failed: %w", err)
	}
	return leaseFromRecord(record), nil
}

// Release marks a lease as expired (does not delete the row — preserves audit history).
func (s *PocketBaseLeaseStore) Release(executionID, holderID string) error {
	record, err := s.findLease(executionID)
	if err != nil {
		return err
	}
	if record.GetString("holder_id") != holderID {
		return fmt.Errorf("persistence: lease Release: holder mismatch for %q", executionID)
	}
	record.Set("expired", true)
	record.Set("expires_at", time.Now().UTC().Format(time.RFC3339))
	return s.dao.SaveRecord(record)
}

// Get returns the current lease record for an execution.
func (s *PocketBaseLeaseStore) Get(executionID string) (*executor.ExecutionLease, error) {
	record, err := s.findLease(executionID)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return leaseFromRecord(record), nil
}

// IsHeld reports whether a valid (non-expired) lease currently exists.
func (s *PocketBaseLeaseStore) IsHeld(executionID string) (bool, error) {
	record, err := s.findLease(executionID)
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if record.GetBool("expired") {
		return false, nil
	}
	expiry := record.GetString("expires_at")
	if expiry == "" {
		return false, nil
	}
	t, err := time.Parse(time.RFC3339, expiry)
	if err != nil {
		return false, nil
	}
	return t.After(time.Now().UTC()), nil
}

func (s *PocketBaseLeaseStore) findLease(executionID string) (*models.Record, error) {
	record, err := s.dao.FindFirstRecordByFilter(
		colLeases,
		"execution_id = {:id}",
		dbx.Params{"id": executionID},
	)
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("persistence: lease not found for %q", executionID)
		}
		return nil, fmt.Errorf("persistence: lease lookup failed: %w", err)
	}
	return record, nil
}

func leaseFromRecord(r *models.Record) *executor.ExecutionLease {
	return &executor.ExecutionLease{
		LeaseID:     r.Id,
		ExecutionID: r.GetString("execution_id"),
		HolderID:    r.GetString("holder_id"),
		AcquiredAt:  r.GetString("acquired_at"),
		ExpiresAt:   r.GetString("expires_at"),
		Renewed:     r.GetInt("renewed"),
		Expired:     r.GetBool("expired"),
	}
}
