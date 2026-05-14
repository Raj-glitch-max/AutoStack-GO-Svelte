package reconciler

// This file implements the deploy/destroy dispatch path for cloud
// deployment_targets. See project-context/reconciler/deploy-dispatch-design.md
// for the design rationale.
//
// Ownership rules enforced here:
//   - The reconciler is the SOLE caller of Provider.Deploy / Provider.Destroy.
//   - At most one operation per target is in flight at any time, enforced
//     via an atomic CAS on deployment_targets.current_operation.
//   - The goroutine that opens an operations row owns it until that row
//     reaches a terminal status. No other goroutine writes to it.
//   - Stale-spec detection: every operation is stamped with the rollout's
//     `updated` timestamp at claim time. Deploy success is downgraded to
//     `succeeded_stale` if the rollout moved during the call; the next
//     cycle re-dispatches.

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/janlauber/one-click/pkg/models"
	"github.com/janlauber/one-click/pkg/providers"
	"github.com/pocketbase/dbx"
	pbmodels "github.com/pocketbase/pocketbase/models"
	"gopkg.in/yaml.v2"
)

// DeployTimeout is the upper bound on a single Deploy call from claim to
// outcome. Chosen to fit cold-image Cloud Run starts (~5-10 min) with
// headroom for revision propagation.
const DeployTimeout = 15 * time.Minute

// abandonedOpThreshold is retained for future runtime sweep (multi-pod).
// The Phase 2.1 startup sweep treats EVERY in_progress operation as
// abandoned regardless of age — see sweep.go.
const abandonedOpThreshold = 20 * time.Minute

// heartbeatInterval is how often the dispatcher refreshes
// operations.updated_at while a deploy is in flight. This is preparation
// for a future runtime sweep that uses age-since-heartbeat as a liveness
// proxy. For the single-pod Phase 2.1 startup sweep, heartbeats are not
// load-bearing but they ARE the foundation for multi-pod safety.
const heartbeatInterval = 60 * time.Second

// claimResult describes the outcome of an attempted CAS claim.
type claimResult int

const (
	claimSucceeded claimResult = iota
	claimRaceLost           // another reconciler won the CAS
	claimNotEligible        // target not in a dispatchable state
)

// shouldDispatchDeploy returns true if the target is in a state that the
// reconciler should try to claim for a deploy operation. The state-model
// admits two dispatchable entry points:
//   - pending (first-time deploy)
//   - error (operator-initiated retry would normally clear the error first;
//     for Phase 2.0 we do NOT auto-retry from `error` — the circuit
//     breaker holds and an explicit user action is required).
//
// We only auto-dispatch from `pending` AND only if no operation is in flight.
func shouldDispatchDeploy(row map[string]interface{}) bool {
	status, _ := row["status"].(string)
	if status != "pending" {
		return false
	}
	currentOp, _ := row["current_operation"].(string)
	return currentOp == ""
}

// shouldDispatchDestroy returns true if the target should be destroyed.
// Phase 2.0 treats status="deleting" as the dispatch signal. The status
// can be set by the rollout-update handler when endDate is set, or by an
// explicit destroy endpoint (deferred).
func shouldDispatchDestroy(row map[string]interface{}) bool {
	status, _ := row["status"].(string)
	if status != "deleting" {
		return false
	}
	currentOp, _ := row["current_operation"].(string)
	return currentOp == ""
}

// dispatchDeploy claims the target, performs Provider.Deploy, and writes
// outcome records. Returns reconcileSuccess on a clean success, reconcileFailed
// on any error, reconcileSkipped if the CAS lost a race.
//
// Phase 2.1 hardening:
//   - opID is threaded into every release call so the CAS guard works.
//   - A heartbeat goroutine refreshes operations.updated_at every minute
//     for the duration of Provider.Deploy.
//   - A dispatcher-local panic recovery defers ensures that, if anything
//     inside the dispatcher panics, the operation is marked failed and
//     the target is released — leaving the target stuck `creating` with
//     a stranded op would be the worst possible outcome.
//   - action="updated" when the target already has an external_id at
//     claim time (re-deploy), "created" otherwise (first deploy).
//
// Phase 2.3:
//   - cycleID flows through every dispatch log emission so cross-component
//     grep works against the reconciler's cycle correlation tag.
//   - targetProvider is the canonical `deployment_targets.provider` value
//     (gcp-cloudrun / aws-ecs / azure-aca). Phase 2.0 was passing
//     account.Provider (gcp / aws / azure) into deployment_history, which
//     mismatched the target's provider column. Lineage rows now use the
//     same canonical name as the target row.
//   - pendingDestroy is checked at release time. If set, the success
//     branch flips the target to `deleting` instead of `updating` so
//     the next reconcile cycle dispatches Destroy. This closes the
//     "endDate-during-in-flight-deploy silently dropped" hazard.
func (r *Reconciler) dispatchDeploy(
	ctx context.Context,
	p providers.Provider,
	account *providers.CloudAccount,
	targetID, rolloutID, preClaimExternalID string,
	manifest, rolloutRevision string,
	targetConfig map[string]interface{},
	cycleID, targetProvider string,
	pendingDestroy bool,
) (resultOut reconcileResult) {
	historyAction := "created"
	if preClaimExternalID != "" {
		historyAction = "updated"
	}

	// 1. Build the DeploySpec from the rollout manifest. Failure here is
	//    a permanent error: no point claiming the target only to fail in
	//    the same cycle. Bubble straight to error status.
	spec, err := buildDeploySpec(manifest, rolloutID)
	if err != nil {
		log.Printf("[DISPATCH_SPEC_ERR] cycle=%s target=%s error=%v", cycleID, targetID, err)
		r.markTargetError(targetID, "deploy spec parse failed: "+err.Error())
		return reconcileFailed
	}
	// Apply provider-specific target config overrides such as min_instances
	// for Cloud Run. These let operators tune provider behavior without
	// changing the core manifest schema. Silently skipped if nil/empty.
	if targetConfig != nil && len(targetConfig) > 0 {
		spec.TargetConfig = targetConfig
	}

	// 2. Open the operations row FIRST so we have an ID for the CAS claim.
	//    If CAS fails (race lost), we mark this op as cancelled and bail.
	opID, err := r.createOperation(targetID, "deploy", rolloutRevision)
	if err != nil {
		log.Printf("[DISPATCH_OP_CREATE_ERR] cycle=%s target=%s error=%v", cycleID, targetID, err)
		return reconcileFailed
	}

	// 3. Atomic CAS: take the target only if it is still pending with no
	//    in-flight operation. This is the load-bearing safety check.
	claimed, err := r.claimTarget(targetID, opID)
	if err != nil {
		log.Printf("[DISPATCH_CLAIM_ERR] cycle=%s target=%s error=%v", cycleID, targetID, err)
		r.cancelOperation(opID, "claim CAS failed: "+err.Error())
		return reconcileFailed
	}
	if claimed != claimSucceeded {
		log.Printf("[DISPATCH_CLAIM_SKIP] cycle=%s target=%s reason=race_lost", cycleID, targetID)
		r.cancelOperation(opID, "another reconciler won the claim")
		// Phase 2.7: forensic history for the cancelled attempt so the
		// timeline reflects "we tried, lost the race". Brief; no
		// to_revision since we never reached the provider.
		r.writeHistory(rolloutID, targetID, historyAction, "failed", "", "", "dispatch race lost; operation cancelled", targetProvider)
		return reconcileSkipped
	}

	log.Printf("[DISPATCH_CLAIM] cycle=%s target=%s operation=%s rollout_revision=%s action=%s", cycleID, targetID, opID, rolloutRevision, historyAction)
	r.writeHistory(rolloutID, targetID, historyAction, "in_progress", "", "", "", targetProvider)

	// Dispatcher-local panic recovery. If anything below panics — provider
	// call, completeOperation, release — we MUST still mark the op failed
	// and release the target. Leaving a target stuck `creating` with a
	// stranded `in_progress` op would require a process restart to clear,
	// because the reconciler's poll-skip-when-in-flight guard would refuse
	// to touch it.
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[DISPATCH_PANIC] cycle=%s target=%s op=%s panic=%v", cycleID, targetID, opID, rec)
			r.completeOperation(opID, "failed", "dispatcher panic")
			r.releaseTarget(opID, targetID, "creating", "error", "dispatcher panic")
			r.writeHistory(rolloutID, targetID, historyAction, "failed", "", preClaimExternalID, "dispatcher panic", targetProvider)
			resultOut = reconcileFailed
		}
	}()

	// 4. Bound the Deploy call with a hard timeout. Even if Cloud Run hangs,
	//    we must return ownership of this target eventually.
	deployCtx, cancel := context.WithTimeout(ctx, DeployTimeout)
	defer cancel()

	// Heartbeat sidecar: keeps operations.updated_at fresh so any future
	// runtime sweep treats this op as live. Scoped to ctx (outer deadline),
	// NOT deployCtx — deployCtx is cancelled when Provider.Deploy returns
	// and the op row must stay live through the full success/error branch
	// execution so sweep doesn't reclaim a live op mid-flight.
	go r.heartbeat(ctx, opID)

	log.Printf("[DEPLOY_START] cycle=%s target=%s operation=%s image=%s/%s:%s region=%s",
		cycleID, targetID, opID, spec.Image.Registry, spec.Image.Repository, spec.Image.Tag, account.Region)
	startedAt := time.Now()

	result, deployErr := p.Deploy(deployCtx, account, spec)

	durationMs := time.Since(startedAt).Milliseconds()
	log.Printf("[DEPLOY_END] cycle=%s target=%s operation=%s duration_ms=%d err=%v", cycleID, targetID, opID, durationMs, deployErr)

	// 5. Stale-spec check: did the rollout move while we were deploying?
	stale := r.rolloutMovedSince(rolloutID, rolloutRevision)

	// Phase 2.3 pending_destroy re-arm: if an endDate-set arrived while we
	// were in flight, the controller set `pending_destroy=true` instead of
	// flipping status to `deleting`. On the success path, honor that intent
	// by routing the target to `deleting` rather than `updating`. On any
	// failure path, leave the target in `error` so the operator can
	// investigate; the pending_destroy flag persists and will re-arm the
	// destroy on the next manual recovery / status transition.
	postSuccessStatus := "updating"
	postSuccessMessage := "deploy completed; awaiting convergence"
	if pendingDestroy {
		postSuccessStatus = "deleting"
		postSuccessMessage = "deploy completed; honoring pending destroy intent"
	}

	// 6. Branch on outcome.
	switch {
	case deployErr != nil:
		// Hard error: refuse to claim success. Phase 2.6: a hard error is
		// not a stale outcome; reset the stale counter so we don't carry
		// stale-loop state across unrelated failures.
		msg := sanitizeError(deployErr.Error())
		r.completeOperation(opID, "failed", msg)
		r.writeHistory(rolloutID, targetID, historyAction, "failed", "", "", msg, targetProvider)
		r.releaseTarget(opID, targetID, "creating", "error", msg)
		r.recordTargetFailureWithCategory(targetID, ClassifyError(deployErr.Error()))
		r.clearStaleCount(targetID)
		return reconcileFailed

	case result != nil && result.Status == "error":
		// Deploy returned (result, nil) but result.Status=error: provider
		// observed a Ready=FAILED condition. Same treatment as a hard error.
		msg := sanitizeError(result.Message)
		r.completeOperation(opID, "failed", msg)
		r.writeHistory(rolloutID, targetID, historyAction, "failed", "", result.ExternalID, msg, targetProvider)
		r.releaseTargetWithExternal(opID, targetID, "creating", "error", result.ExternalID, "", msg)
		r.recordTargetFailureWithCategory(targetID, FailurePermanent)
		r.clearStaleCount(targetID)
		return reconcileFailed

	case stale:
		// Deploy "succeeded" but the desired state moved underneath us.
		// Do NOT promote to running.
		//
		// Phase 2.6 stale-loop guard: track consecutive stales per target.
		// Below threshold → release to `pending` (next cycle re-dispatches
		// with the new spec). At or above threshold → release to `error`
		// with a clear message so the loop doesn't burn provider quota
		// indefinitely. Operator respec clears the count via the
		// pending-entry pass in cloud.go.
		staleN := r.noteStaleSucceeded(targetID)
		if staleN >= staleThreshold {
			msg := fmt.Sprintf("pathological stale-spec loop after %d consecutive succeeded_stale outcomes; operator action required", staleN)
			log.Printf("[DEPLOY_STALE_LOOP_HOLD] cycle=%s target=%s operation=%s stale_count=%d", cycleID, targetID, opID, staleN)
			r.completeOperation(opID, "succeeded_stale", msg)
			r.writeHistory(rolloutID, targetID, historyAction, "failed", "", result.ExternalID, msg, targetProvider)
			r.releaseTargetWithExternal(opID, targetID, "creating", "error", result.ExternalID, "", msg)
			// Don't clear staleCount here: it stays high until operator
			// respec triggers the pending-entry clear. This prevents an
			// immediate-tick repeat from re-acquiring after one cycle.
			return reconcileFailed
		}
		log.Printf("[DEPLOY_STALE] cycle=%s target=%s operation=%s stale_count=%d — rollout moved during deploy", cycleID, targetID, opID, staleN)
		r.completeOperation(opID, "succeeded_stale", "rollout updated during deploy; re-dispatching next cycle")
		r.writeHistory(rolloutID, targetID, historyAction, "failed", "", result.ExternalID, "stale spec", targetProvider)
		r.releaseTargetWithExternal(opID, targetID, "creating", "pending", result.ExternalID, "", "")
		return reconcileFailed

	default:
		// Honest success: provider returned a running-or-converging service.
		// We deliberately persist `updating` (not `running`) so the next
		// GetStatus poll is what promotes the target to `running`. This
		// defends against Cloud Run's transient Ready=SUCCEEDED flap.
		//
		// Phase 2.3: when pending_destroy is set, postSuccessStatus is
		// `deleting` and the next reconcile cycle will dispatch Destroy.
		r.completeOperation(opID, "succeeded", "deploy completed")
		r.writeHistory(rolloutID, targetID, historyAction, "success", "", result.ExternalID, "", targetProvider)
		r.releaseTargetWithExternal(opID, targetID, "creating", postSuccessStatus, result.ExternalID, result.ExternalID, postSuccessMessage)
		// Phase 2.7: forensic history when sweep reclaimed ownership
		// while we were running. The dispatcher actually saw a success
		// provider-side; the sweep already wrote an "abandoned" history
		// row. This row gives operators the dispatcher's view.
		if ok, present := r.releaseStillOwner(opID); present && !ok {
			r.writeOwnershipLostHistory(rolloutID, targetID, historyAction, "success", result.ExternalID, targetProvider)
		}
		if pendingDestroy {
			r.clearPendingDestroy(targetID)
			log.Printf("[CLOUD_DESTROY_REARMED] cycle=%s target=%s reason=in_flight_intent_honored", cycleID, targetID)
		}
		r.clearTargetFailure(targetID)
		r.clearStaleCount(targetID)
		return reconcileSuccess
	}
}

// dispatchDestroy is the Destroy-side mirror of dispatchDeploy. Same
// Phase 2.1 hardening: opID threaded into release, heartbeat sidecar,
// dispatcher-local panic recovery.
//
// Phase 2.3: cycleID threaded for cross-component log correlation;
// targetProvider used for history lineage to match deployment_targets.provider.
func (r *Reconciler) dispatchDestroy(
	ctx context.Context,
	p providers.Provider,
	account *providers.CloudAccount,
	targetID, rolloutID, externalID string,
	cycleID, targetProvider string,
) (resultOut reconcileResult) {
	opID, err := r.createOperation(targetID, "destroy", "")
	if err != nil {
		log.Printf("[DISPATCH_OP_CREATE_ERR] cycle=%s target=%s error=%v", cycleID, targetID, err)
		return reconcileFailed
	}

	claimed, err := r.claimTarget(targetID, opID)
	if err != nil || claimed != claimSucceeded {
		r.cancelOperation(opID, "destroy claim lost or errored")
		// Phase 2.7: forensic history for cancelled destroy attempts.
		r.writeHistory(rolloutID, targetID, "deleted", "failed", "", externalID, "destroy claim lost or errored", targetProvider)
		return reconcileSkipped
	}

	log.Printf("[DISPATCH_CLAIM] cycle=%s target=%s operation=%s kind=destroy", cycleID, targetID, opID)
	r.writeHistory(rolloutID, targetID, "deleted", "in_progress", "", "", "", targetProvider)

	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[DISPATCH_PANIC] cycle=%s target=%s op=%s kind=destroy panic=%v", cycleID, targetID, opID, rec)
			r.completeOperation(opID, "failed", "dispatcher panic")
			r.releaseTarget(opID, targetID, "deleting", "error", "dispatcher panic")
			r.writeHistory(rolloutID, targetID, "deleted", "failed", "", "", "dispatcher panic", targetProvider)
			resultOut = reconcileFailed
		}
	}()

	destroyCtx, cancel := context.WithTimeout(ctx, DeployTimeout)
	defer cancel()

	// heartbeat is scoped to ctx (outer reconciler deadline), NOT
	// destroyCtx. destroyCtx is cancelled when Provider.Destroy returns
	// (after the confirmDeleted loop completes in Cloud Run provider),
	// and the op row must stay live through that return so sweep doesn't
	// reclaim it mid-flight. The heartbeat's own UPDATE uses
	// "WHERE status = 'in_progress'" so it stops naturally once
	// completeOperation transitions the row to terminal.
	// Phase 2.9 AW-C3 fix.
	go r.heartbeat(ctx, opID)

	target := &providers.DeploymentTarget{
		ID:         targetID,
		ExternalID: externalID,
		Provider:   account.Provider,
		Region:     account.Region,
	}

	if err := p.Destroy(destroyCtx, account, target); err != nil {
		msg := sanitizeError(err.Error())
		r.completeOperation(opID, "failed", msg)
		r.writeHistory(rolloutID, targetID, "deleted", "failed", "", "", msg, targetProvider)
		r.releaseTarget(opID, targetID, "deleting", "error", msg)
		r.recordTargetFailureWithCategory(targetID, ClassifyError(err.Error()))
		return reconcileFailed
	}

	r.completeOperation(opID, "succeeded", "destroy completed")
	r.writeHistory(rolloutID, targetID, "deleted", "success", "", "", "", targetProvider)
	r.releaseTarget(opID, targetID, "deleting", "deleted", "destroy completed")
	// On successful destroy, clear pending_destroy in case it was set by a
	// late-arriving intent during the destroy itself (paranoid; usually
	// already false).
	r.clearPendingDestroy(targetID)
	r.clearTargetFailure(targetID)
	return reconcileSuccess
}

// clearPendingDestroy resets the pending_destroy flag. Called after the
// success path of dispatchDeploy when it routed to `deleting` (we've
// consumed the intent) and after dispatchDestroy success (we've completed
// the destroy regardless).
//
// Best-effort: a write failure here leaves the flag set, which means the
// next deploy success would route to `deleting` again — a re-deploy can
// undo this in a separate cycle. Logged for visibility.
func (r *Reconciler) clearPendingDestroy(targetID string) {
	rec, err := r.app.Dao().FindRecordById("deployment_targets", targetID)
	if err != nil {
		log.Printf("[PENDING_DESTROY_CLEAR_ERR] target=%s find: %v", targetID, err)
		return
	}
	rec.Set("pending_destroy", false)
	if err := r.app.Dao().SaveRecord(rec); err != nil {
		log.Printf("[PENDING_DESTROY_CLEAR_ERR] target=%s save: %v", targetID, err)
	}
}

// createOperation inserts a new in_progress operation row and returns its ID.
func (r *Reconciler) createOperation(targetID, kind, rolloutRevision string) (string, error) {
	col, err := r.app.Dao().FindCollectionByNameOrId("operations")
	if err != nil {
		return "", fmt.Errorf("operations collection: %w", err)
	}
	rec := pbmodels.NewRecord(col)
	now := time.Now().UTC().Format(time.RFC3339)
	rec.Set("target", targetID)
	rec.Set("kind", kind)
	rec.Set("status", "in_progress")
	rec.Set("started_at", now)
	rec.Set("updated_at", now)
	rec.Set("rollout_revision", rolloutRevision)
	if err := r.app.Dao().SaveRecord(rec); err != nil {
		return "", err
	}
	return rec.Id, nil
}

// claimTarget performs the atomic CAS that grants this reconciler
// exclusive ownership of the target for the lifetime of opID. Returns
// claimSucceeded only if exactly one row was updated.
//
// SQLite semantics: UPDATE ... WHERE current_operation IS NULL is safe
// under concurrent writers because the WAL write lock serializes the
// statements. The same logic on Postgres requires the same approach;
// the SET ... WHERE NULL idiom is the standard CAS pattern.
func (r *Reconciler) claimTarget(targetID, opID string) (claimResult, error) {
	res, err := r.app.Dao().DB().NewQuery(
		"UPDATE deployment_targets " +
			"SET current_operation = {:op}, last_state_change_at = {:ts}, status = " +
			"CASE WHEN status = 'pending' THEN 'creating' " +
			"     WHEN status = 'deleting' THEN 'deleting' " +
			"     ELSE status END " +
			"WHERE id = {:id} " +
			"  AND (current_operation = '' OR current_operation IS NULL) " +
			"  AND status IN ('pending', 'deleting')",
	).Bind(dbx.Params{
		"op": opID,
		"ts": time.Now().UTC().Format(time.RFC3339),
		"id": targetID,
	}).Execute()
	if err != nil {
		return claimNotEligible, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return claimNotEligible, err
	}
	if n == 0 {
		return claimRaceLost, nil
	}
	return claimSucceeded, nil
}

// completeOperation transitions an operation to a terminal status.
//
// CAS guard (Phase 2.1): the transition is applied via SQL UPDATE that
// requires the current status to be `in_progress`. This prevents:
//   - the sweep marking an op `failed` after a successful dispatcher
//     return, then having the dispatcher's success overwrite it;
//   - a dispatcher double-completing the same op (e.g., success path
//     then panic-recovery path).
//
// Returning silently on a 0-row update is intentional: the op already
// reached a terminal state; nothing to do. The caller's logging context
// (e.g., the panic-recovery defer) provides enough operator visibility.
func (r *Reconciler) completeOperation(opID, status, message string) {
	res, err := r.app.Dao().DB().NewQuery(
		"UPDATE operations " +
			"SET status = {:status}, updated_at = {:ts}, message = {:msg} " +
			"WHERE id = {:id} AND status = 'in_progress'",
	).Bind(dbx.Params{
		"status": status,
		"ts":     time.Now().UTC().Format(time.RFC3339),
		"msg":    message,
		"id":     opID,
	}).Execute()
	if err != nil {
		log.Printf("[OP_COMPLETE_ERR] op=%s err=%v", opID, err)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		log.Printf("[OP_COMPLETE_NOOP] op=%s status=%s — already terminal; not flipping", opID, status)
	}
}

// heartbeat bumps operations.updated_at every heartbeatInterval until ctx
// is cancelled. Spawned as a sidecar goroutine by dispatchDeploy and
// dispatchDestroy. A transient DB error is logged but does not stop the
// heartbeat — recovery is per-tick.
//
// The heartbeat MUST stop bumping once the op is no longer in_progress.
// We enforce that via a status check in the UPDATE WHERE clause: once
// the dispatcher's terminal completeOperation runs, the heartbeat's
// subsequent UPDATEs match zero rows.
func (r *Reconciler) heartbeat(ctx context.Context, opID string) {
	t := time.NewTicker(heartbeatInterval)
	defer t.Stop()
	// Phase 2.7: clear heartbeat-fail counter when this heartbeat goroutine exits.
	defer func() {
		r.heartbeatFailMu.Lock()
		delete(r.heartbeatFails, opID)
		r.heartbeatFailMu.Unlock()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			res, err := r.app.Dao().DB().NewQuery(
				"UPDATE operations SET updated_at = {:ts} " +
					"WHERE id = {:id} AND status = 'in_progress'",
			).Bind(dbx.Params{
				"ts": time.Now().UTC().Format(time.RFC3339),
				"id": opID,
			}).Execute()
			if err != nil {
				// Phase 2.7: track consecutive failures and escalate
				// after 5. Persistent failures are operationally meaningful
				// (e.g., DB busy storm) — the runtime sweep may reclaim
				// the op even though the dispatcher is alive.
				r.heartbeatFailMu.Lock()
				r.heartbeatFails[opID]++
				count := r.heartbeatFails[opID]
				r.heartbeatFailMu.Unlock()
				if count == 5 {
					log.Printf("[HEARTBEAT_FAIL_PERSISTENT] op=%s consecutive_failures=%d err=%v — runtime sweep may reclaim this op", opID, count, err)
				} else {
					log.Printf("[HEARTBEAT_FAIL] op=%s err=%v", opID, err)
				}
				continue
			}
			// Successful tick clears the failure counter.
			r.heartbeatFailMu.Lock()
			r.heartbeatFails[opID] = 0
			r.heartbeatFailMu.Unlock()
			n, _ := res.RowsAffected()
			if n == 0 {
				// Op left in_progress while we were heartbeating; stop.
				return
			}
		}
	}
}

// cancelOperation marks an operation as cancelled. Used when a CAS race
// is lost and we never actually called the provider.
func (r *Reconciler) cancelOperation(opID, reason string) {
	r.completeOperation(opID, "cancelled", reason)
}

// releaseTarget clears current_operation and writes the post-deploy status.
//
// Release-CAS guard (Phase 2.1): the UPDATE is conditional on
// `current_operation = :opID`. If the sweep, another dispatcher, or
// any external actor cleared or replaced current_operation while we
// were running, our release becomes a no-op. This prevents the
// dispatcher from overwriting a sweep's terminal `error` state when
// the dispatcher eventually returns past the abandonment threshold.
//
// The transition guard from updateTargetStatus is intentionally NOT
// applied here: the dispatcher knows the intended transition
// (pending→creating→updating, etc.) and is the authority during its
// owned window.
func (r *Reconciler) releaseTarget(opID, targetID, fromStatus, toStatus, message string) {
	r.releaseTargetWithExternal(opID, targetID, fromStatus, toStatus, "", "", message)
}

// releaseStillOwner returns true if the most-recent release-CAS for
// targetID/opID actually wrote rows (we still owned the target).
// Wired through releaseTargetWithExternal via a per-call result var.
//
// Implementation note: we track the latest release outcome on the
// dispatcher via a map keyed by opID. Lifetime: written by the release
// call, read by the immediate post-release branch in dispatchDeploy/
// dispatchDestroy. The map entry is cleaned up after read. This is
// in-memory only and tolerates restart (nothing references stale ops
// across restart by design).
//
// Kept narrowly scoped here rather than changing every release-method
// signature: the goal is forensic history visibility on lost-ownership,
// not a broader API refactor.
func (r *Reconciler) releaseStillOwner(opID string) (bool, bool) {
	r.releaseOutcomeMu.Lock()
	defer r.releaseOutcomeMu.Unlock()
	v, ok := r.releaseOutcome[opID]
	delete(r.releaseOutcome, opID)
	return v, ok
}

// writeOwnershipLostHistory records a forensic history row when the
// dispatcher's release-CAS finds 0 rows affected (sweep reclaimed the
// op mid-flight). Phase 2.7: closes the lineage gap where the
// dispatcher's actual provider observation was previously invisible.
// Caller passes the dispatcher's intent (action) and observed outcome.
func (r *Reconciler) writeOwnershipLostHistory(rolloutID, targetID, action, observedOutcome, externalID, targetProvider string) {
	if rolloutID == "" || targetID == "" {
		return
	}
	msg := "dispatcher returned but sweep had reclaimed ownership"
	if observedOutcome != "" {
		msg += "; observed_outcome=" + observedOutcome
	}
	if externalID != "" {
		msg += "; external_id=" + externalID
	}
	r.writeHistory(rolloutID, targetID, action, "failed", "", externalID, msg, targetProvider)
}

// releaseTargetWithExternal additionally writes external_id and current_revision
// when the dispatcher has those values from the provider response.
func (r *Reconciler) releaseTargetWithExternal(opID, targetID, fromStatus, toStatus, externalID, currentRevision, message string) {
	now := time.Now().UTC().Format(time.RFC3339)

	// Build the SET clause dynamically so we don't overwrite columns the
	// caller didn't explicitly intend to touch. external_id and
	// current_revision are persisted only when supplied non-empty.
	setClause := "current_operation = '', " +
		"status = {:status}, " +
		"last_state_change_at = {:ts}, " +
		"last_synced = {:ts}"
	params := dbx.Params{
		"status": toStatus,
		"ts":     now,
		"op":     opID,
		"id":     targetID,
	}
	if externalID != "" {
		setClause += ", external_id = {:ext}"
		params["ext"] = externalID
	}
	if currentRevision != "" {
		setClause += ", current_revision = {:rev}"
		params["rev"] = currentRevision
	}
	if message != "" {
		setClause += ", drift_summary = {:msg}"
		params["msg"] = message
	}

	res, err := r.app.Dao().DB().NewQuery(
		"UPDATE deployment_targets SET " + setClause +
			" WHERE id = {:id} AND current_operation = {:op}",
	).Bind(params).Execute()
	if err != nil {
		log.Printf("[RELEASE_ERR] target=%s op=%s err=%v", targetID, opID, err)
		return
	}
	n, _ := res.RowsAffected()
	r.releaseOutcomeMu.Lock()
	r.releaseOutcome[opID] = n > 0
	r.releaseOutcomeMu.Unlock()
	if n == 0 {
		log.Printf("[RELEASE_LOST_OWNERSHIP] target=%s op=%s — current_operation no longer points at us; sweep or external actor took over", targetID, opID)
	}
	_ = fromStatus // reserved for future history alignment
}

// markTargetError is a fast-path failure: no operation was ever opened.
func (r *Reconciler) markTargetError(targetID, message string) {
	rec, err := r.app.Dao().FindRecordById("deployment_targets", targetID)
	if err != nil {
		return
	}
	rec.Set("status", "error")
	rec.Set("drift_summary", message)
	rec.Set("last_synced", time.Now().UTC().Format(time.RFC3339))
	rec.Set("last_state_change_at", time.Now().UTC().Format(time.RFC3339))
	_ = r.app.Dao().SaveRecord(rec)
}

// rolloutMovedSince reports whether the rollout's `updated` timestamp
// advanced past rolloutRevision while the dispatcher was running.
// Returns false on any read error — we cannot prove staleness from a
// failed read, and marking a successful deploy stale on a hiccup would
// itself be a lie.
func (r *Reconciler) rolloutMovedSince(rolloutID, rolloutRevision string) bool {
	if rolloutRevision == "" {
		return false
	}
	rec, err := r.app.Dao().FindRecordById("rollouts", rolloutID)
	if err != nil {
		return false
	}
	return rec.GetString("updated") != rolloutRevision
}

// writeHistory appends an immutable record to deployment_history. Failures
// are logged but do not propagate — a write failure here must not roll
// back the actual deploy.
func (r *Reconciler) writeHistory(rolloutID, targetID, action, status, fromRevision, toRevision, message, provider string) {
	col, err := r.app.Dao().FindCollectionByNameOrId("deployment_history")
	if err != nil {
		log.Printf("[HISTORY_WRITE_ERR] collection: %v", err)
		return
	}
	rec := pbmodels.NewRecord(col)
	rec.Set("rollout", rolloutID)
	if targetID != "" {
		rec.Set("target", targetID)
	}
	rec.Set("action", action)
	rec.Set("status", status)
	if fromRevision != "" {
		rec.Set("from_revision", fromRevision)
	}
	if toRevision != "" {
		rec.Set("to_revision", toRevision)
	}
	if message != "" {
		rec.Set("message", message)
	}
	if provider != "" {
		rec.Set("provider", provider)
	}
	if err := r.app.Dao().SaveRecord(rec); err != nil {
		log.Printf("[HISTORY_WRITE_ERR] save: %v", err)
		return
	}
	log.Printf("[HISTORY_WRITE] target=%s action=%s status=%s", targetID, action, status)
}

// buildDeploySpec parses the YAML manifest stored on the rollout into a
// provider-neutral DeploySpec. The manifest schema matches pkg/models.Rollout.
//
// Parsing failures here are treated as permanent — the only fix is to
// re-edit the rollout, which is a user action.
func buildDeploySpec(manifest, rolloutID string) (*providers.DeploySpec, error) {
	if strings.TrimSpace(manifest) == "" {
		return nil, fmt.Errorf("empty manifest")
	}
	var ro models.Rollout
	if err := yaml.Unmarshal([]byte(manifest), &ro); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}

	cpuLimit := parseCPUResource(ro.Spec.Resources.Limits.CPU)
	memLimit := parseMemoryMB(ro.Spec.Resources.Limits.Memory)
	cpuReq := parseCPUResource(ro.Spec.Resources.Requests.CPU)
	memReq := parseMemoryMB(ro.Spec.Resources.Requests.Memory)

	env := make([]providers.EnvVar, 0, len(ro.Spec.Env))
	for _, e := range ro.Spec.Env {
		env = append(env, providers.EnvVar{Name: e.Name, Value: e.Value})
	}

	ifaces := make([]providers.NetworkInterface, 0, len(ro.Spec.Interfaces))
	for _, i := range ro.Spec.Interfaces {
		ifaces = append(ifaces, providers.NetworkInterface{
			ContainerPort: i.Port,
		})
	}

	return &providers.DeploySpec{
		RolloutID: rolloutID,
		Image: providers.ImageSpec{
			Registry:   ro.Spec.Image.Registry,
			Repository: ro.Spec.Image.Repository,
			Tag:        ro.Spec.Image.Tag,
		},
		Compute: providers.ComputeSpec{
			CPULimitVCPU:    cpuLimit,
			CPURequestVCPU:  cpuReq,
			MemoryLimitMB:   memLimit,
			MemoryRequestMB: memReq,
		},
		Scale: providers.ScaleSpec{
			MinReplicas:        ro.Spec.HorizontalScale.MinReplicas,
			MaxReplicas:        ro.Spec.HorizontalScale.MaxReplicas,
			ScaleTargetPercent: ro.Spec.HorizontalScale.TargetCPUUtilizationPercentage,
		},
		Network: providers.NetworkSpec{Interfaces: ifaces},
		Env:     env,
	}, nil
}

// parseCPUResource converts a Kubernetes-style CPU resource string ("500m",
// "1", "2.5") into a vCPU float. Unparseable inputs return 0, which the
// Cloud Run provider treats as "no limit set".
func parseCPUResource(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.HasSuffix(s, "m") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(s, "m"), 64)
		if err != nil {
			return 0
		}
		return n / 1000
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return n
}

// parseMemoryMB converts a Kubernetes memory string ("256Mi", "1Gi",
// "512M") into MB. Returns 0 on parse failure.
//
// Convention: Mi/Gi are binary (1024-based); M/G are decimal. We
// normalize to MB (decimal) for the Provider spec. Cloud Run's API
// accepts a "Mi" string and we re-emit Mi in provider.go, so the
// downstream representation is binary regardless.
func parseMemoryMB(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	multipliers := []struct {
		suffix string
		mul    float64
	}{
		{"Ki", 1.0 / 1024},
		{"Mi", 1},
		{"Gi", 1024},
		{"Ti", 1024 * 1024},
		{"K", 1.0 / 1000},
		{"M", 1},
		{"G", 1000},
		{"T", 1000 * 1000},
	}
	for _, m := range multipliers {
		if strings.HasSuffix(s, m.suffix) {
			n, err := strconv.ParseFloat(strings.TrimSuffix(s, m.suffix), 64)
			if err != nil {
				return 0
			}
			return int(n * m.mul)
		}
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	// Bytes; convert to MB.
	return int(n / (1024 * 1024))
}
