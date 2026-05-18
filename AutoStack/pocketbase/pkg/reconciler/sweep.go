package reconciler

// Crash-recovery sweep: run once at process start, BEFORE the reconciler
// ticker, to neutralize operation rows that were in_progress when the
// previous process died.
//
// Honesty principle: we never *infer* that an abandoned operation
// succeeded. The provider-side resource may be partially created or
// fully created; we cannot tell from row state alone. Marking these
// operations as `failed` (not `succeeded`) forces an operator to take
// explicit recovery action.
//
// Phase 2.1 policy: at startup, EVERY in_progress operation was treated
// as abandoned by definition, because the only process that could
// legitimately own it was the one that just died. The age threshold from
// Phase 2.0 was wrong for the startup case — a recently-started op
// (process died 30 seconds into a deploy) slipped through and left a
// permanent stuck state.
//
// Phase 2.3 policy: heartbeat-aware sweep. An op whose updated_at has
// been refreshed within heartbeatLivenessWindow is presumed live —
// either the dispatcher process is still draining (rolling restart with
// graceful shutdown) or a peer pod owns it (multi-pod). We refuse to
// reclaim such ops. An op whose updated_at == started_at has never
// heartbeated and is therefore swept regardless of recency (covers the
// "crashed before first heartbeat tick" case).
//
// Multi-pod future: pod B's startup sweep can still incorrectly
// classify pod A's live operations as abandoned IF heartbeatLivenessWindow
// has expired. The full mitigation is pod-identity stamping on
// operations (`owned_by_pod`), deferred to Phase 2.5. The heartbeat
// window is a narrow guard, not a substitute.

import (
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/dbx"
	pb "github.com/pocketbase/pocketbase"
	pbmodels "github.com/pocketbase/pocketbase/models"
)

// heartbeatLivenessWindow is the maximum age of operations.updated_at
// below which an op is presumed live and exempt from sweep reclamation.
// Set to 2 × heartbeatInterval so a single missed heartbeat is tolerated.
// See [[phase2.3/replay-safety-assessment.md]] §2 and
// [[phase2.3/ownership-integrity-review.md]] O-2.
const heartbeatLivenessWindow = 2 * heartbeatInterval

// SweepAbandonedOperations marks abandoned operations as failed at
// startup, and releases the corresponding deployment_targets row to
// status=error. Writes an immutable deployment_history row for each
// abandonment so post-incident reconstruction reflects the recovery action.
//
// Phase 2.3 heartbeat-aware policy: an op whose updated_at is fresher
// than heartbeatLivenessWindow AND has actually heartbeated at least
// once (updated_at != started_at) is presumed live and skipped. All
// others are abandoned.
//
// MUST be called synchronously BEFORE the reconciler ticker starts. Phase
// 2.1 calls this from Reconciler.Start to make the ordering invariant
// local to the reconciler.
func SweepAbandonedOperations(app *pb.PocketBase) error {
	var ids []struct {
		ID         string `db:"id"`
		TargetID   string `db:"target"`
		Kind       string `db:"kind"`
		StartedAt  string `db:"started_at"`
		UpdatedAt  string `db:"updated_at"`
		OwnedByPod string `db:"owned_by_pod"`
	}
	err := app.Dao().DB().
		Select("id", "target", "kind", "started_at", "updated_at", "owned_by_pod").
		From("operations").
		Where(dbx.NewExp("status = 'in_progress'")).
		All(&ids)
	if err != nil {
		return fmt.Errorf("sweep query: %w", err)
	}

	if len(ids) == 0 {
		log.Printf("[STARTUP_SWEEP] no in_progress operations")
		return nil
	}

	// Phase 3.8 SC-5: log foreign-pod ops for forensic visibility before reclaim.
	thisPod := string(currentPodIdentity())

	// Partition into live (heartbeated recently and ever) vs abandoned.
	//
	// Phase 3.9.2 lease-aware reclamation: an operation row whose target
	// has an UNEXPIRED lease held by a foreign pod MUST NOT be reclaimed.
	// That foreign pod is alive (lease refresh keeps the lease alive); the
	// operation is in flight. Reclaiming would corrupt orchestration state.
	type opRow struct {
		ID, TargetID, Kind, OwnedByPod string
	}
	var abandoned []opRow
	var live []opRow
	var leaseProtected []opRow // Phase 3.9.2: foreign-pod lease is unexpired
	cutoff := time.Now().UTC().Add(-heartbeatLivenessWindow)
	for _, row := range ids {
		started, _ := time.Parse(time.RFC3339, row.StartedAt)
		updated, _ := time.Parse(time.RFC3339, row.UpdatedAt)
		neverHeartbeated := !updated.After(started)
		heartbeatStale := updated.Before(cutoff)

		// Phase 3.9.2: per-target lease lookup. A foreign-pod's UNEXPIRED
		// lease is a hard "do not reclaim" signal regardless of heartbeat age.
		// We use a separate lookup so a missing lease (legacy pre-3.9.2 op)
		// falls through to the heartbeat-aged path.
		leaseHolderIsLive := false
		leaseHolderForeign := false
		if row.TargetID != "" && thisPod != "" {
			holder, valid := lookupLeaseHolderStatus(app, row.TargetID)
			if valid && holder != "" && holder != thisPod {
				leaseHolderIsLive = true
				leaseHolderForeign = true
			}
		}

		if leaseHolderIsLive {
			leaseProtected = append(leaseProtected, opRow{row.ID, row.TargetID, row.Kind, row.OwnedByPod})
			log.Printf("[STARTUP_SWEEP_LEASE_PROTECTED] operation=%s target=%s kind=%s owned_by_pod=%s lease_holder_foreign=%v — refusing to reclaim",
				row.ID, row.TargetID, row.Kind, row.OwnedByPod, leaseHolderForeign)
			continue
		}

		if row.OwnedByPod != "" && thisPod != "" && row.OwnedByPod != thisPod {
			// Foreign-pod stamp but no live lease — operator may have run
			// two pods in the past or the previous pod crashed without
			// releasing lease. Reclaim is safe because the lease is gone.
			log.Printf("[STARTUP_SWEEP_FOREIGN_POD_NO_LEASE] operation=%s target=%s kind=%s owned_by_pod=%s this_pod=%s — lease expired or absent; reclaiming",
				row.ID, row.TargetID, row.Kind, row.OwnedByPod, thisPod)
		}
		if neverHeartbeated || heartbeatStale {
			abandoned = append(abandoned, opRow{row.ID, row.TargetID, row.Kind, row.OwnedByPod})
		} else {
			live = append(live, opRow{row.ID, row.TargetID, row.Kind, row.OwnedByPod})
		}
	}
	if len(leaseProtected) > 0 {
		log.Printf("[STARTUP_SWEEP] lease-protected ops preserved (foreign pods alive): %d", len(leaseProtected))
	}

	if len(live) > 0 {
		log.Printf("[STARTUP_SWEEP_SKIP_LIVE] %d in_progress ops within heartbeat liveness window — preserved", len(live))
		for _, l := range live {
			log.Printf("[STARTUP_SWEEP_SKIP_LIVE] operation=%s target=%s kind=%s", l.ID, l.TargetID, l.Kind)
		}
	}
	if len(abandoned) == 0 {
		log.Printf("[STARTUP_SWEEP] no abandoned operations after heartbeat check")
		return nil
	}
	log.Printf("[STARTUP_SWEEP] found %d abandoned in_progress operations at startup", len(abandoned))
	now := time.Now().UTC().Format(time.RFC3339)
	const abandonMessage = "abandoned: process restart while in flight"
	for _, row := range abandoned {
		op, err := app.Dao().FindRecordById("operations", row.ID)
		if err != nil {
			log.Printf("[STARTUP_SWEEP] find op %s: %v", row.ID, err)
			continue
		}
		op.Set("status", "failed")
		op.Set("updated_at", now)
		op.Set("message", abandonMessage)
		if err := app.Dao().SaveRecord(op); err != nil {
			log.Printf("[STARTUP_SWEEP] save op %s: %v", row.ID, err)
			continue
		}

		// Reconstructable history: every abandoned operation gets a
		// terminal history row so the timeline isn't missing an outcome.
		writeAbandonHistory(app, row.TargetID, row.Kind, abandonMessage)

		tgt, err := app.Dao().FindRecordById("deployment_targets", row.TargetID)
		if err != nil {
			log.Printf("[STARTUP_SWEEP] find target %s: %v", row.TargetID, err)
			continue
		}
		// Only clear the in-flight pointer if it still points at us. If
		// somehow it points elsewhere, leave it — that operation owns the row.
		if tgt.GetString("current_operation") == row.ID {
			tgt.Set("current_operation", "")
			tgt.Set("status", "error")
			tgt.Set("last_state_change_at", now)
			tgt.Set("last_synced", now)
			tgt.Set("drift_summary", "abandoned: process restart")
			if err := app.Dao().SaveRecord(tgt); err != nil {
				log.Printf("[STARTUP_SWEEP] save target %s: %v", row.TargetID, err)
			}
		}
		log.Printf("[OP_ABANDONED] operation=%s target=%s kind=%s", row.ID, row.TargetID, row.Kind)
	}
	return nil
}

// runtimeSweepInterval is how often the periodic runtime sweep runs.
// Set to 5 min to give the heartbeat liveness window (2 min) a
// comfortable margin without hammering the DB.
const runtimeSweepInterval = 5 * time.Minute

// runtimeSweepStaleAge is the minimum age past which an in-progress
// operation is presumed dead. Generously larger than the heartbeat
// liveness window so the runtime sweep never races a healthy heartbeat.
const runtimeSweepStaleAge = 5 * time.Minute

// RunRuntimeSweepLoop starts a goroutine that periodically reclaims
// operations whose heartbeat has lapsed past runtimeSweepStaleAge.
// Phase 2.6: closes the post-first-heartbeat-death stuck-state hazard
// (see [[phase2.6/runtime-sweep-design.md]] and
// [[phase2.4/ownership-integrity-review.md]] OS-2, OS-7).
//
// Multi-pod safety: this sweep is unsafe under multi-pod (it cannot
// distinguish a peer pod's live op from an abandoned one). Documented
// as single-pod only. Phase 2.7 pod-identity stamping work will lift
// the constraint.
func RunRuntimeSweepLoop(app *pb.PocketBase, stop <-chan struct{}) {
	ticker := time.NewTicker(runtimeSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if err := RuntimeSweep(app); err != nil {
				log.Printf("[RUNTIME_SWEEP_ERR] %v", err)
			}
		}
	}
}

// RuntimeSweep performs a single runtime sweep pass. Reclaims ops with
// stale heartbeat (no refresh within runtimeSweepStaleAge). Same write
// pattern as the startup sweep, except it doesn't pre-filter the
// first-heartbeat case (runtime sweep is for ops that DID heartbeat
// once and then went stale).
func RuntimeSweep(app *pb.PocketBase) error {
	var rows []struct {
		ID        string `db:"id"`
		TargetID  string `db:"target"`
		Kind      string `db:"kind"`
		UpdatedAt string `db:"updated_at"`
	}
	cutoff := time.Now().UTC().Add(-runtimeSweepStaleAge).Format(time.RFC3339)
	err := app.Dao().DB().
		Select("id", "target", "kind", "updated_at").
		From("operations").
		Where(dbx.NewExp("status = 'in_progress' AND updated_at < {:cutoff}", dbx.Params{"cutoff": cutoff})).
		All(&rows)
	if err != nil {
		return fmt.Errorf("runtime sweep query: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	log.Printf("[RUNTIME_SWEEP] found %d stale-heartbeat in_progress operations cutoff=%s", len(rows), cutoff)
	now := time.Now().UTC().Format(time.RFC3339)
	const reclaimMsg = "abandoned: heartbeat went stale (runtime sweep)"
	thisPod := string(currentPodIdentity())
	for _, row := range rows {
		// Phase 3.9.2: refuse reclaim if a foreign live lease holds the target.
		// This is the critical multi-pod safety check at runtime.
		if row.TargetID != "" {
			holder, valid := lookupLeaseHolderStatus(app, row.TargetID)
			if valid && holder != thisPod && holder != "" {
				log.Printf("[RUNTIME_SWEEP_LEASE_PROTECTED] operation=%s target=%s lease_holder=%s — refusing to reclaim live foreign lease",
					row.ID, row.TargetID, holder)
				continue
			}
		}

		op, err := app.Dao().FindRecordById("operations", row.ID)
		if err != nil {
			log.Printf("[RUNTIME_SWEEP] find op %s: %v", row.ID, err)
			continue
		}
		// CAS-style guard: only flip in_progress → failed. If the op was
		// already terminal (race with the dispatcher's completeOperation),
		// FindRecordById would have returned the current state and our
		// Set+Save would clobber a terminal status. Re-check before saving.
		if op.GetString("status") != "in_progress" {
			continue
		}
		op.Set("status", "failed")
		op.Set("updated_at", now)
		op.Set("message", reclaimMsg)
		if err := app.Dao().SaveRecord(op); err != nil {
			log.Printf("[RUNTIME_SWEEP] save op %s: %v", row.ID, err)
			continue
		}

		writeAbandonHistory(app, row.TargetID, row.Kind, reclaimMsg)

		tgt, err := app.Dao().FindRecordById("deployment_targets", row.TargetID)
		if err != nil {
			log.Printf("[RUNTIME_SWEEP] find target %s: %v", row.TargetID, err)
			continue
		}
		if tgt.GetString("current_operation") == row.ID {
			tgt.Set("current_operation", "")
			tgt.Set("status", "error")
			tgt.Set("last_state_change_at", now)
			tgt.Set("last_synced", now)
			tgt.Set("drift_summary", reclaimMsg)
			if err := app.Dao().SaveRecord(tgt); err != nil {
				log.Printf("[RUNTIME_SWEEP] save target %s: %v", row.TargetID, err)
			}
		}
		log.Printf("[OP_ABANDONED_RUNTIME] operation=%s target=%s kind=%s", row.ID, row.TargetID, row.Kind)
	}
	return nil
}

// lookupLeaseHolderStatus returns (holderPodID, valid) for a lease key.
// valid is true ONLY if a lease row exists AND lease_expires_at is in the
// future. Used by the sweep to refuse reclaiming operations owned by a
// live peer pod.
//
// DB error → (empty, false): conservative. The sweep falls through to
// heartbeat-aged reclaim — accepting the small risk of double-dispatch
// rather than the larger risk of stuck operations.
func lookupLeaseHolderStatus(app *pb.PocketBase, leaseKey string) (holder string, valid bool) {
	var rows []struct {
		HolderPodID string `db:"holder_pod_id"`
		Valid       int    `db:"is_valid"`
	}
	err := app.Dao().DB().
		Select("holder_pod_id",
			"CASE WHEN lease_expires_at > datetime('now') THEN 1 ELSE 0 END AS is_valid").
		From("reconciler_leases").
		Where(dbx.NewExp("lease_key = {:k}", dbx.Params{"k": leaseKey})).
		All(&rows)
	if err != nil || len(rows) == 0 {
		return "", false
	}
	return rows[0].HolderPodID, rows[0].Valid == 1
}

// writeAbandonHistory inserts a deployment_history row reflecting the
// sweep's failed-abandonment action. Failure is logged but does not
// propagate — the abandonment itself is what matters operationally.
func writeAbandonHistory(app *pb.PocketBase, targetID, kind, message string) {
	if targetID == "" {
		return
	}
	tgt, err := app.Dao().FindRecordById("deployment_targets", targetID)
	if err != nil {
		log.Printf("[STARTUP_SWEEP_HISTORY_ERR] find target %s: %v", targetID, err)
		return
	}
	col, err := app.Dao().FindCollectionByNameOrId("deployment_history")
	if err != nil {
		log.Printf("[STARTUP_SWEEP_HISTORY_ERR] collection: %v", err)
		return
	}
	action := "error"
	switch kind {
	case "deploy":
		action = "error"
	case "destroy":
		action = "deleted"
	case "rollback":
		action = "rolled_back"
	}
	rec := pbmodels.NewRecord(col)
	rec.Set("rollout", tgt.GetString("rollout"))
	rec.Set("target", targetID)
	rec.Set("action", action)
	rec.Set("status", "failed")
	rec.Set("message", message)
	rec.Set("provider", tgt.GetString("provider"))
	if err := app.Dao().SaveRecord(rec); err != nil {
		log.Printf("[STARTUP_SWEEP_HISTORY_ERR] save: %v", err)
	}
}
