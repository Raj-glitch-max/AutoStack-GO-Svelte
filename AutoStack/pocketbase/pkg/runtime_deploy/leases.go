package runtime_deploy

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/models"
)

// LeaseTTL is how long a lease is valid without a fresh heartbeat. The
// reconciler heartbeats every ~LeaseTTL/3. A worker that hasn't heartbeated
// within LeaseTTL is considered crashed and another worker may take over.
const LeaseTTL = 30 * time.Second

// workerIDOnce ensures the same process always reports the same worker id
// for its lifetime. PID alone would race across container restarts; we
// concatenate boot timestamp.
var (
	workerIDOnce sync.Once
	workerID     string
)

// WorkerID returns this process's stable lease worker identifier.
func WorkerID() string {
	workerIDOnce.Do(func() {
		workerID = fmt.Sprintf("worker-pid%d-%d", os.Getpid(), time.Now().UnixNano())
	})
	return workerID
}

// ErrLeaseHeldByOther is returned by AcquireLease when another live worker
// holds the lease and the current process is not the holder.
var ErrLeaseHeldByOther = errors.New("runtime_deploy: lease held by another live worker")

// AcquireLease attempts to take ownership of a deployment's reconciliation
// lease for this process. Behavior:
//   - no lease exists                  → create one, return nil
//   - lease held by this worker        → refresh heartbeat, return nil
//   - lease held by other worker live  → return ErrLeaseHeldByOther
//   - lease held by other worker stale → take over, return nil
//
// This is single-writer coordination, NOT distributed consensus. Two engines
// racing on the same DB will pick up the same `no lease exists` branch and
// both create one — the UNIQUE INDEX on (deployment) makes one of them lose
// the race with a SQL constraint violation, which we treat as "the other won"
// and return ErrLeaseHeldByOther.
func (s *Store) AcquireLease(deploymentID string) error {
	coll, err := s.app.Dao().FindCollectionByNameOrId("runtime_deployment_leases")
	if err != nil {
		return fmt.Errorf("AcquireLease: collection not found: %w", err)
	}

	now := time.Now().UTC()
	expires := now.Add(LeaseTTL)

	existing, err := s.findLease(deploymentID)
	if err != nil {
		return err
	}
	if existing == nil {
		rec := models.NewRecord(coll)
		rec.Set("deployment", deploymentID)
		rec.Set("worker_id", WorkerID())
		rec.Set("acquired_at", pbDate(now))
		rec.Set("heartbeat_at", pbDate(now))
		rec.Set("expires_at", pbDate(expires))
		if err := s.app.Dao().SaveRecord(rec); err != nil {
			// Likely UNIQUE INDEX race. Reload and re-evaluate.
			again, reErr := s.findLease(deploymentID)
			if reErr == nil && again != nil {
				return s.maybeHeartbeatOrSteal(again, now, expires)
			}
			return fmt.Errorf("AcquireLease: insert lease: %w", err)
		}
		return nil
	}
	return s.maybeHeartbeatOrSteal(existing, now, expires)
}

// HeartbeatLease extends our existing lease. Returns ErrLeaseHeldByOther if
// the lease was stolen by another process in the meantime.
func (s *Store) HeartbeatLease(deploymentID string) error {
	existing, err := s.findLease(deploymentID)
	if err != nil {
		return err
	}
	if existing == nil {
		// No lease at all — re-acquire from scratch.
		return s.AcquireLease(deploymentID)
	}
	if existing.GetString("worker_id") != WorkerID() {
		return ErrLeaseHeldByOther
	}
	now := time.Now().UTC()
	existing.Set("heartbeat_at", pbDate(now))
	existing.Set("expires_at", pbDate(now.Add(LeaseTTL)))
	if err := s.app.Dao().SaveRecord(existing); err != nil {
		return fmt.Errorf("HeartbeatLease: save: %w", err)
	}
	return nil
}

// ReleaseLease removes the lease unconditionally if held by this worker.
// Called during graceful shutdown so a restart doesn't have to wait for TTL.
func (s *Store) ReleaseLease(deploymentID string) error {
	existing, err := s.findLease(deploymentID)
	if err != nil || existing == nil {
		return err
	}
	if existing.GetString("worker_id") != WorkerID() {
		return nil
	}
	return s.app.Dao().DeleteRecord(existing)
}

// LeaseHolder returns the current worker_id holding the lease and a boolean
// indicating whether it's still live. Useful for UI display + tests.
func (s *Store) LeaseHolder(deploymentID string) (string, bool, error) {
	rec, err := s.findLease(deploymentID)
	if err != nil || rec == nil {
		return "", false, err
	}
	// Same PB v0.22 date round-trip workaround. The auto-managed `updated`
	// time + LeaseTTL is the effective expiration — every heartbeat saves
	// the record, which refreshes `updated`.
	exp := rec.GetUpdated().Time().Add(LeaseTTL)
	return rec.GetString("worker_id"), time.Now().UTC().Before(exp), nil
}

func (s *Store) findLease(deploymentID string) (*models.Record, error) {
	recs, err := s.app.Dao().FindRecordsByFilter("runtime_deployment_leases",
		"deployment = {:d}", "", 1, 0,
		map[string]interface{}{"d": deploymentID})
	if err := emptyOnNoRows(err); err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, nil
	}
	return recs[0], nil
}

func (s *Store) maybeHeartbeatOrSteal(rec *models.Record, now, expires time.Time) error {
	holder := rec.GetString("worker_id")
	// Same PB v0.22 date round-trip workaround. The auto-managed `updated`
	// time + LeaseTTL is the effective expiration — every heartbeat saves
	// the record, which refreshes `updated`.
	exp := rec.GetUpdated().Time().Add(LeaseTTL)
	if holder == WorkerID() {
		// Already ours — refresh.
		rec.Set("heartbeat_at", pbDate(now))
		rec.Set("expires_at", pbDate(expires))
		return s.app.Dao().SaveRecord(rec)
	}
	if now.Before(exp) {
		// Held by another live worker.
		return ErrLeaseHeldByOther
	}
	// Stale — take over. Record the takeover with a fresh acquired_at.
	rec.Set("worker_id", WorkerID())
	rec.Set("acquired_at", pbDate(now))
	rec.Set("heartbeat_at", pbDate(now))
	rec.Set("expires_at", pbDate(expires))
	return s.app.Dao().SaveRecord(rec)
}

// suppress unused linter for testing-only sql shim
var _ = sql.ErrNoRows
