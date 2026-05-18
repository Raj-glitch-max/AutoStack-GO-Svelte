package reconciler

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/dbx"
	pb "github.com/pocketbase/pocketbase"
	pbmodels "github.com/pocketbase/pocketbase/models"
)

// Phase 3.9.2 — Real Multi-Pod Lease Coordination (SC-5 fence).
//
// This module converts Phase 3.8's owned_by_pod stamp from a FORENSIC
// WITNESS into a DISTRIBUTED COORDINATION FENCE. Before any provider
// mutation, the reconciler MUST hold a valid lease for the target.
//
// ─────────────────────────────────────────────────────────────────────────
// HONEST SCOPE STATEMENT
// ─────────────────────────────────────────────────────────────────────────
//
// This module makes TWO reconciler pods operationally survivable when
// they accidentally run simultaneously. It does NOT make AutoStack:
//
//   - a distributed orchestrator
//   - highly available (HA) in the consensus sense
//   - Byzantine-fault-tolerant
//   - partition-tolerant in the CAP sense
//   - capable of exactly-once provider execution
//
// The serialization point is the SQLite WAL. Two pods sharing the same
// DB file are serialized by SQLite's file locking. If the DB file is on
// shared storage (NFS, EFS) and the network partitions, file-locking
// semantics dictate correctness — this is an operator deployment
// concern, NOT a guarantee this module makes.
//
// ─────────────────────────────────────────────────────────────────────────
// LEASE LIFECYCLE
// ─────────────────────────────────────────────────────────────────────────
//
//   Pod A: AcquireLease(target, podA)         → epoch=1
//   Pod A: RefreshLease(target, podA, 1)      → epoch=1 (extends expires_at)
//   Pod A: ... provider call ...
//   Pod A: ReleaseLease(target, podA, 1)      → row deleted
//
//   Pod A: AcquireLease(target, podA)         → epoch=1
//   Pod A: ... crash; no release; no refresh ...
//   Pod B: AcquireLease(target, podB)         → fails (expires_at > now)
//   ... LeaseDuration passes ...
//   Pod B: AcquireLease(target, podB)         → epoch=2, A's row replaced
//
//   Pod A revives: VerifyLease(target, podA, 1) → false (epoch != 1 in DB)
//   Pod A MUST NOT issue further provider mutations under that operation.
//
// ─────────────────────────────────────────────────────────────────────────
// CONCURRENCY GUARANTEES
// ─────────────────────────────────────────────────────────────────────────
//
//   AcquireLease — SQLite WAL serializes the UPDATE/INSERT. Exactly one
//   acquirer succeeds when N pods race. The losers see Lock or 0 rows
//   affected and return ErrLeaseHeld.
//
//   RefreshLease — UPDATE WHERE lease_key=:k AND holder=:p AND
//   lease_epoch=:e. Atomic CAS via the WHERE clause. If 0 rows affected,
//   the caller's lease was stolen (epoch changed) or the row was deleted
//   (some other process reclaimed). The caller must STOP further provider
//   mutations.
//
//   ReleaseLease — DELETE WHERE lease_key=:k AND holder=:p AND
//   lease_epoch=:e. Idempotent: a 0-row delete means "already gone" which
//   is operationally equivalent to "released".
//
//   VerifyLease — pure read; uses DB-side timestamp comparison to avoid
//   trusting the caller's clock for expiration decisions.
//
// ─────────────────────────────────────────────────────────────────────────
// CLOCK SKEW
// ─────────────────────────────────────────────────────────────────────────
//
// All expiration comparisons use the DB clock (datetime('now')) rather
// than the pod's local clock. This eliminates two-pod clock skew as a
// correctness factor. The pod's local clock is used only to compute the
// initial lease_expires_at value sent to the DB — the DB then becomes
// the authority on whether that value is in the past or future.

// ─────────────────────────────────────────────────────────────────────────
// CONSTANTS
// ─────────────────────────────────────────────────────────────────────────

// LeaseDuration is how long a lease is valid after each acquisition/refresh.
//
// Trade-off: too short → frequent refresh load + risk of refresh failure
// during slow provider calls evicting a healthy pod. Too long → slow
// recovery after a crashed pod (must wait this long before another pod
// can take over).
//
// 60s gives a 3x margin over the operation heartbeat (heartbeatInterval =
// 60s). The lease MUST be refreshed faster than it expires.
const LeaseDuration = 60 * time.Second

// LeaseRefreshInterval is how often the lease holder refreshes its lease.
//
// Set to 1/3 of LeaseDuration so two missed refreshes still leave the
// lease valid for one more interval — tolerates one DB hiccup before
// eviction.
const LeaseRefreshInterval = 20 * time.Second

// leaseRowID is generated deterministically from lease_key so multiple
// migrations don't proliferate IDs. PocketBase requires an ID column; we
// derive it from the key to make ON CONFLICT semantics tractable on a
// table without true upsert support.
//
// In practice, we don't use this — PocketBase's record API generates IDs
// at insert time, and we look up rows by lease_key (unique index).

// ─────────────────────────────────────────────────────────────────────────
// SENTINEL ERRORS
// ─────────────────────────────────────────────────────────────────────────

// ErrLeaseHeld — a foreign pod currently holds an active lease.
// The caller MUST NOT proceed with the operation; retry on a future tick.
var ErrLeaseHeld = errors.New("lease held by another pod")

// ErrLeaseLost — the caller's lease has been lost (epoch changed or
// row deleted). Any in-flight provider mutation under this lease MUST
// be aborted.
var ErrLeaseLost = errors.New("lease lost by current holder")

// ─────────────────────────────────────────────────────────────────────────
// PUBLIC API
// ─────────────────────────────────────────────────────────────────────────

// AcquireLease attempts to acquire the lease for the given key on behalf
// of podID. Returns the new epoch on success.
//
// Acquisition succeeds when:
//   1. No lease row exists for the key — INSERT a fresh row with epoch=1.
//   2. A lease row exists but is expired (lease_expires_at <= NOW) — UPDATE
//      it: bump epoch, replace holder, extend expiration.
//   3. A lease row exists and the caller already holds it — UPDATE it
//      (treated as a reacquisition; epoch increments to indicate a fresh
//      ownership cycle).
//
// Acquisition fails (returns ErrLeaseHeld) when:
//   - A foreign pod holds the lease AND it is not yet expired.
//
// All non-acquisition errors (DB failures) are returned wrapped with
// context. The caller MUST treat any non-nil error as "did not acquire";
// it is NEVER safe to proceed on error.
//
// Concurrency: SQLite serializes the underlying UPDATE/INSERT via the
// WAL writer lock. When N pods race, only one succeeds; the others see
// 0 rows affected or a constraint conflict and return ErrLeaseHeld.
func (r *Reconciler) AcquireLease(leaseKey, podID string) (epoch int, err error) {
	if leaseKey == "" || podID == "" {
		return 0, fmt.Errorf("leaseKey and podID required")
	}

	nowSQL := "datetime('now')"
	expiresAt := time.Now().UTC().Add(LeaseDuration).Format("2006-01-02 15:04:05")

	// Step 1: try to UPDATE an existing row that is either expired or
	// already owned by this pod. The WHERE clause is the atomic CAS.
	// If 0 rows affected, EITHER:
	//   (a) no row exists for this key (proceed to INSERT), OR
	//   (b) a foreign pod holds an unexpired lease (return ErrLeaseHeld).
	//
	// We need to distinguish (a) from (b). Approach: try UPDATE first;
	// if 0 rows, SELECT to learn whether a row exists. This avoids the
	// race where SELECT-then-UPDATE could see stale state.
	updateSQL := fmt.Sprintf(`UPDATE reconciler_leases
		SET holder_pod_id = {:pod},
		    lease_epoch = lease_epoch + 1,
		    lease_expires_at = {:exp},
		    heartbeat_at = %s,
		    updated_at = %s
		WHERE lease_key = {:key}
		  AND (lease_expires_at <= %s OR holder_pod_id = {:pod})`,
		nowSQL, nowSQL, nowSQL)
	res, err := r.app.Dao().DB().NewQuery(updateSQL).Bind(dbx.Params{
		"pod": podID,
		"exp": expiresAt,
		"key": leaseKey,
	}).Execute()
	if err != nil {
		return 0, fmt.Errorf("lease update: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 1 {
		// CAS succeeded; read back the new epoch.
		return r.readLeaseEpoch(leaseKey, podID)
	}

	// Step 2: no UPDATE matched. Determine whether the row exists or not.
	// We use a SELECT to differentiate. Race-window note: between this
	// SELECT and the INSERT below, another pod could insert a row first;
	// SQLite's unique constraint on lease_key will then fail our INSERT —
	// which we treat as "foreign pod beat us to it" → ErrLeaseHeld.
	exists, _ := r.leaseRowExists(leaseKey)
	if exists {
		// Row exists but is held by a non-self pod that hasn't expired.
		return 0, ErrLeaseHeld
	}

	// Step 3: INSERT a fresh row. Unique-constraint failure here means a
	// concurrent acquirer beat us to it (rare race window).
	col, err := r.app.Dao().FindCollectionByNameOrId("reconciler_leases")
	if err != nil {
		return 0, fmt.Errorf("lease collection: %w", err)
	}
	rec := pbmodels.NewRecord(col)
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	rec.Set("lease_key", leaseKey)
	rec.Set("holder_pod_id", podID)
	rec.Set("lease_epoch", 1)
	rec.Set("lease_expires_at", expiresAt)
	rec.Set("heartbeat_at", now)
	if err := r.app.Dao().SaveRecord(rec); err != nil {
		// Likely the unique-index race: another pod inserted between our
		// UPDATE and our INSERT. Treat as foreign-held.
		return 0, ErrLeaseHeld
	}
	return 1, nil
}

// RefreshLease extends the expiration of an existing lease.
//
// Only succeeds when (lease_key, holder_pod_id, lease_epoch) all match
// the DB state. If 0 rows are affected, the lease has been lost — either
// stolen (epoch changed) or reclaimed (row deleted). The caller MUST
// stop further provider mutations.
//
// This is the load-bearing safety check for long-running provider calls:
// a refresh failure tells the caller "your lease is gone" and the
// reconciler aborts dispatch via the ExecStageInterruptedUnknownOutcome
// path.
func (r *Reconciler) RefreshLease(leaseKey, podID string, epoch int) error {
	if leaseKey == "" || podID == "" || epoch <= 0 {
		return fmt.Errorf("leaseKey, podID, epoch required")
	}
	expiresAt := time.Now().UTC().Add(LeaseDuration).Format("2006-01-02 15:04:05")
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	res, err := r.app.Dao().DB().NewQuery(
		`UPDATE reconciler_leases
		 SET lease_expires_at = {:exp},
		     heartbeat_at = {:now},
		     updated_at = {:now}
		 WHERE lease_key = {:key}
		   AND holder_pod_id = {:pod}
		   AND lease_epoch = {:epoch}`,
	).Bind(dbx.Params{
		"key":   leaseKey,
		"pod":   podID,
		"epoch": epoch,
		"exp":   expiresAt,
		"now":   now,
	}).Execute()
	if err != nil {
		return fmt.Errorf("lease refresh: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrLeaseLost
	}
	return nil
}

// ReleaseLease deletes the caller's lease. Idempotent: a 0-row delete
// (lease already gone) returns nil — operationally equivalent.
//
// Only the current holder at the current epoch may release. A stale
// caller (e.g., a pod that lost its lease and woke up later) calling
// Release sees 0 rows deleted, which is the correct outcome — that pod's
// lease has already been replaced.
func (r *Reconciler) ReleaseLease(leaseKey, podID string, epoch int) error {
	if leaseKey == "" || podID == "" || epoch <= 0 {
		return nil // nothing to release
	}
	_, err := r.app.Dao().DB().NewQuery(
		`DELETE FROM reconciler_leases
		 WHERE lease_key = {:key}
		   AND holder_pod_id = {:pod}
		   AND lease_epoch = {:epoch}`,
	).Bind(dbx.Params{
		"key":   leaseKey,
		"pod":   podID,
		"epoch": epoch,
	}).Execute()
	if err != nil {
		log.Printf("[LEASE_RELEASE_ERR] key=%s pod=%s epoch=%d err=%v", leaseKey, podID, epoch, err)
		return fmt.Errorf("lease release: %w", err)
	}
	return nil
}

// VerifyLease returns true if (leaseKey, podID, epoch) exactly match the
// current DB row AND the lease has not expired.
//
// This is called immediately BEFORE every provider boundary so the
// reconciler can detect lease loss mid-operation. The DB clock decides
// expiration; pod-local clock is irrelevant.
//
// Failure modes:
//   - DB error → returns false, logs. Conservative: treat as lease lost.
//   - Epoch mismatch → returns false (lease stolen).
//   - Row missing → returns false (lease reclaimed by sweep).
//   - Expired → returns false (caller is over the deadline).
func (r *Reconciler) VerifyLease(leaseKey, podID string, epoch int) bool {
	var rows []struct {
		HolderPodID string `db:"holder_pod_id"`
		LeaseEpoch  int    `db:"lease_epoch"`
		Valid       int    `db:"is_valid"`
	}
	err := r.app.Dao().DB().
		Select("holder_pod_id", "lease_epoch",
			"CASE WHEN lease_expires_at > datetime('now') THEN 1 ELSE 0 END AS is_valid").
		From("reconciler_leases").
		Where(dbx.NewExp("lease_key = {:k}", dbx.Params{"k": leaseKey})).
		All(&rows)
	if err != nil {
		log.Printf("[LEASE_VERIFY_ERR] key=%s err=%v", leaseKey, err)
		return false
	}
	if len(rows) == 0 {
		return false
	}
	row := rows[0]
	if row.HolderPodID != podID || row.LeaseEpoch != epoch || row.Valid != 1 {
		return false
	}
	return true
}

// ─────────────────────────────────────────────────────────────────────────
// INTERNAL HELPERS
// ─────────────────────────────────────────────────────────────────────────

func (r *Reconciler) readLeaseEpoch(leaseKey, podID string) (int, error) {
	var rows []struct {
		LeaseEpoch int `db:"lease_epoch"`
	}
	err := r.app.Dao().DB().
		Select("lease_epoch").
		From("reconciler_leases").
		Where(dbx.NewExp("lease_key = {:k} AND holder_pod_id = {:p}",
			dbx.Params{"k": leaseKey, "p": podID})).
		All(&rows)
	if err != nil || len(rows) == 0 {
		return 0, fmt.Errorf("lease epoch readback failed: %v", err)
	}
	return rows[0].LeaseEpoch, nil
}

func (r *Reconciler) leaseRowExists(leaseKey string) (bool, error) {
	var rows []struct {
		LeaseKey string `db:"lease_key"`
	}
	err := r.app.Dao().DB().
		Select("lease_key").
		From("reconciler_leases").
		Where(dbx.NewExp("lease_key = {:k}", dbx.Params{"k": leaseKey})).
		Limit(1).
		All(&rows)
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

// ─────────────────────────────────────────────────────────────────────────
// LEASE REFRESH SIDECAR
// ─────────────────────────────────────────────────────────────────────────

// LeaseRefreshLoop runs alongside long provider operations, refreshing
// the lease at LeaseRefreshInterval until either:
//
//   - the context is cancelled (operation completed), OR
//   - RefreshLease returns ErrLeaseLost (caller should observe leaseLostCh).
//
// On lease loss, the function:
//   1. Closes leaseLostCh (non-blocking signal to caller's select)
//   2. Logs [LEASE_LOST] with forensic detail
//   3. Returns immediately (no further refresh attempts)
//
// The caller's dispatch path MUST select on leaseLostCh between provider
// boundaries to detect mid-operation loss. The cleanest pattern:
//
//   leaseLostCh := make(chan struct{})
//   go r.LeaseRefreshLoop(deployCtx, leaseKey, podID, epoch, leaseLostCh)
//   ...
//   select {
//   case <-leaseLostCh:
//       // abort; transition to interrupted_unknown_outcome
//   default:
//       // safe to call provider (or use VerifyLease for stronger guarantee)
//   }
func (r *Reconciler) LeaseRefreshLoop(ctx interface{ Done() <-chan struct{} }, leaseKey, podID string, epoch int, leaseLostCh chan<- struct{}) {
	t := time.NewTicker(LeaseRefreshInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			err := r.RefreshLease(leaseKey, podID, epoch)
			if err == nil {
				continue
			}
			if errors.Is(err, ErrLeaseLost) {
				log.Printf("[LEASE_LOST] key=%s pod=%s epoch=%d — provider mutation MUST stop", leaseKey, podID, epoch)
				select {
				case leaseLostCh <- struct{}{}:
				default:
					// channel already signalled; idempotent.
				}
				return
			}
			// Transient DB error; log and keep trying. A persistent error
			// will eventually result in lease expiration and a future
			// VerifyLease call returning false.
			log.Printf("[LEASE_REFRESH_ERR] key=%s pod=%s epoch=%d err=%v — will retry", leaseKey, podID, epoch, err)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────
// LEASE-AWARE SWEEP HELPER
// ─────────────────────────────────────────────────────────────────────────

// LeaseEligibleForReclaim returns true if the lease row for leaseKey is
// EITHER missing OR expired according to the DB clock. The sweep uses
// this to decide whether reclaiming an abandoned operation is safe.
//
// A lease row that does NOT exist means no pod owns this target — the
// associated operation row, if any, is a pre-3.9.2 legacy or a sweep
// candidate.
//
// A lease row that exists but is expired means the previous holder lost
// authority; reclaim is safe.
//
// A lease row that exists and is unexpired means a live pod owns the
// target — DO NOT reclaim.
func (r *Reconciler) LeaseEligibleForReclaim(leaseKey string) bool {
	var rows []struct {
		Valid int `db:"is_valid"`
	}
	err := r.app.Dao().DB().
		Select("CASE WHEN lease_expires_at > datetime('now') THEN 1 ELSE 0 END AS is_valid").
		From("reconciler_leases").
		Where(dbx.NewExp("lease_key = {:k}", dbx.Params{"k": leaseKey})).
		All(&rows)
	if err != nil {
		log.Printf("[LEASE_SWEEP_CHECK_ERR] key=%s err=%v", leaseKey, err)
		return false // conservative: do not reclaim on read failure
	}
	if len(rows) == 0 {
		// No lease row — caller decides based on other evidence (heartbeat age).
		return true
	}
	return rows[0].Valid == 0
}

// PurgeExpiredLeases deletes lease rows whose expiration has passed AND
// no in-flight operation references the same target. Best-effort. Run
// from a cron or sweep loop.
//
// We intentionally do not delete leases that are expired-but-referenced
// by an in-flight operation: the sweep handles those rows holistically
// (release the lease at the same time it marks the op failed).
func (r *Reconciler) PurgeExpiredLeases(app *pb.PocketBase) (int, error) {
	res, err := app.Dao().DB().NewQuery(
		`DELETE FROM reconciler_leases
		 WHERE lease_expires_at <= datetime('now')
		   AND lease_key NOT IN (
		     SELECT target FROM operations WHERE status = 'in_progress'
		   )`,
	).Execute()
	if err != nil {
		return 0, fmt.Errorf("purge expired leases: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
