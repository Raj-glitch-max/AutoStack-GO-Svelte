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
	"encoding/json"
	"errors"
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

// shouldDispatchRollback returns true when the target has been placed into
// status="rolling_back" with no in-flight operation. The operator sets
// rolling_back via the API; the reconciler consumes it here.
//
// Phase 3.2: rollback is a capability-gated operation. The dispatch function
// (dispatchRollback) checks CapRollback before calling the provider.
func shouldDispatchRollback(row map[string]interface{}) bool {
	status, _ := row["status"].(string)
	if status != "rolling_back" {
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
	opID, err := r.createOperation(targetID, "deploy", rolloutRevision, cycleID)
	if err != nil {
		log.Printf("[DISPATCH_OP_CREATE_ERR] cycle=%s target=%s error=%v", cycleID, targetID, err)
		return reconcileFailed
	}
	// ExecStage: operation created; not yet claimed. Stage = QUEUED.
	setExecStage(r.app, opID, "", ExecStageQueued)
	// Phase 3.5: persist the full deploy spec snapshot as the immutable
	// deploy-time truth baseline. Captured BEFORE the provider call so it
	// survives provider failures and is available for drift detection,
	// rollback target reconstruction, and incident explanation.
	r.setDeployedSpec(opID, spec)

	// Phase 3.9.2: acquire the per-target execution lease BEFORE the CAS
	// claim. The lease is the multi-pod safety fence; the CAS is the
	// intra-pod safety fence. Both are needed: CAS protects against two
	// goroutines in the same process; the lease protects against two
	// processes (pods) racing on the same target.
	setExecStage(r.app, opID, ExecStageQueued, ExecStageLeaseAcquiring)
	podID := string(currentPodIdentity())
	leaseEpoch, leaseErr := r.AcquireLease(targetID, podID)
	if leaseErr != nil {
		if errors.Is(leaseErr, ErrLeaseHeld) {
			log.Printf("[LEASE_DENIED] cycle=%s target=%s pod=%s reason=foreign_pod_holds_active_lease", cycleID, targetID, podID)
		} else {
			log.Printf("[LEASE_ACQUIRE_ERR] cycle=%s target=%s pod=%s err=%v", cycleID, targetID, podID, leaseErr)
		}
		r.cancelOperation(opID, "lease acquisition denied: "+leaseErr.Error())
		setExecStage(r.app, opID, ExecStageLeaseAcquiring, ExecStageLeaseLost)
		r.RecordDecision(DecisionRecord{
			TargetID:       targetID,
			RolloutID:      rolloutID,
			OperationID:    opID,
			CycleID:        cycleID,
			DecisionType:   DecisionLeaseAcquireDenied,
			DecisionReason: "foreign_pod_holds_active_lease",
			HumanSummary:   "lease acquisition denied; foreign pod holds active lease",
			DecisionInputs: map[string]interface{}{
				"this_pod":      podID,
				"acquire_error": leaseErr.Error(),
				"kind":          "deploy",
			},
			EvidenceRefs: []EvidenceRef{{Collection: "operations", ID: opID}},
		})
		return reconcileSkipped
	}
	log.Printf("[LEASE_ACQUIRED] cycle=%s target=%s pod=%s epoch=%d", cycleID, targetID, podID, leaseEpoch)
	// Lease cleanup: always release on exit (success or any failure path).
	// Release is idempotent and CAS-checked, so multiple release attempts
	// or stale releases are safe.
	defer func() {
		if err := r.ReleaseLease(targetID, podID, leaseEpoch); err != nil {
			log.Printf("[LEASE_RELEASE_ERR] cycle=%s target=%s pod=%s epoch=%d err=%v", cycleID, targetID, podID, leaseEpoch, err)
		}
	}()

	// 3. Atomic CAS: take the target only if it is still pending with no
	//    in-flight operation. This is the load-bearing intra-pod safety
	//    check. The lease (acquired above) is the inter-pod safety check.
	claimed, err := r.claimTarget(targetID, opID)
	if err != nil {
		log.Printf("[DISPATCH_CLAIM_ERR] cycle=%s target=%s error=%v", cycleID, targetID, err)
		r.cancelOperation(opID, "claim CAS failed: "+err.Error())
		setExecStage(r.app, opID, ExecStageLeaseAcquiring, ExecStageFailed)
		return reconcileFailed
	}
	if claimed != claimSucceeded {
		log.Printf("[DISPATCH_CLAIM_SKIP] cycle=%s target=%s reason=race_lost", cycleID, targetID)
		r.cancelOperation(opID, "another reconciler won the claim")
		setExecStage(r.app, opID, ExecStageLeaseAcquiring, ExecStageFailed)
		// Phase 2.7: forensic history for the cancelled attempt so the
		// timeline reflects "we tried, lost the race". Brief; no
		// to_revision since we never reached the provider.
		r.writeHistory(rolloutID, targetID, historyAction, "failed", "", "", "dispatch race lost; operation cancelled", targetProvider, opID, "dispatch-race-lost")
		r.RecordDecision(DecisionRecord{
			TargetID:       targetID,
			RolloutID:      rolloutID,
			OperationID:    opID,
			CycleID:        cycleID,
			DecisionType:   DecisionDispatchClaimLost,
			DecisionReason: "intra_pod_cas_race_lost",
			HumanSummary:   "claimTarget CAS lost; another goroutine in this process won the claim",
			DecisionInputs: map[string]interface{}{
				"kind": "deploy",
			},
			EvidenceRefs: []EvidenceRef{{Collection: "operations", ID: opID}},
		})
		return reconcileSkipped
	}

	// ExecStage: CAS claimed. Stage = DISPATCHING (about to call provider).
	setExecStage(r.app, opID, ExecStageLeaseAcquiring, ExecStageDispatching)

	// Phase 3.9.2: start the lease refresh sidecar. The lease MUST be
	// refreshed throughout the operation's lifetime. If the refresh loop
	// detects loss, leaseLostCh signals; we check it between provider
	// boundaries and on key transitions.
	leaseLostCh := make(chan struct{}, 1)
	leaseRefreshCtx, leaseRefreshCancel := context.WithCancel(ctx)
	defer leaseRefreshCancel()
	go r.LeaseRefreshLoop(leaseRefreshCtx, targetID, podID, leaseEpoch, leaseLostCh)

	log.Printf("[DISPATCH_CLAIM] cycle=%s target=%s operation=%s rollout_revision=%s action=%s pod=%s", cycleID, targetID, opID, rolloutRevision, historyAction, currentPodIdentity())
	r.writeHistory(rolloutID, targetID, historyAction, "in_progress", "", "", "", targetProvider, opID, "")
	if r.metrics != nil {
		r.metrics.IncDeployDispatched()
	}

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
			r.writeHistory(rolloutID, targetID, historyAction, "failed", "", preClaimExternalID, "dispatcher panic", targetProvider, opID, "dispatcher-panic")
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

	// Phase 3.9.2: verify lease ownership immediately before the provider
	// mutation. If lease was lost (e.g., DB stall caused refresh to fail
	// past expiration, then another pod stole it), abort. No provider
	// mutation occurs → stage = LEASE_LOST (not InterruptedUnknownOutcome).
	if !r.VerifyLease(targetID, podID, leaseEpoch) {
		log.Printf("[LEASE_VERIFY_FAIL_PRE_DEPLOY] cycle=%s target=%s pod=%s epoch=%d", cycleID, targetID, podID, leaseEpoch)
		setExecStage(r.app, opID, ExecStageDispatching, ExecStageLeaseLost)
		r.completeOperation(opID, "failed", "lease lost before provider mutation; no cloud changes made")
		r.writeHistory(rolloutID, targetID, historyAction, "failed", "", "", "lease lost before provider call", targetProvider, opID, "lease-lost-pre-call")
		r.releaseTarget(opID, targetID, "creating", "pending", "lease lost before provider call; safe to re-dispatch")
		r.RecordDecision(DecisionRecord{
			TargetID:       targetID,
			RolloutID:      rolloutID,
			OperationID:    opID,
			CycleID:        cycleID,
			DecisionType:   DecisionLeaseLostPreCall,
			DecisionReason: "lease_invalid_pre_provider_call",
			HumanSummary:   "lease invalid before deploy call; no provider mutation occurred; safe to re-dispatch",
			DecisionInputs: map[string]interface{}{
				"this_pod":    podID,
				"lease_epoch": leaseEpoch,
				"kind":        "deploy",
			},
			EvidenceRefs: []EvidenceRef{{Collection: "operations", ID: opID}},
		})
		return reconcileFailed
	}

	// ExecStage: provider Deploy() call is imminent. Stage = PROVISIONING.
	setExecStage(r.app, opID, ExecStageDispatching, ExecStageProvisioning)

	result, deployErr := p.Deploy(deployCtx, account, spec)

	durationMs := time.Since(startedAt).Milliseconds()
	log.Printf("[DEPLOY_END] cycle=%s target=%s operation=%s duration_ms=%d err=%v", cycleID, targetID, opID, durationMs, deployErr)

	// Phase 3.9.2: check for lease loss DURING the provider call. If lost,
	// the provider mutation may have committed cloud changes but our
	// authority to claim success or failure is gone — another pod owns
	// this target now. Transition to InterruptedUnknownOutcome (operator
	// must investigate).
	select {
	case <-leaseLostCh:
		log.Printf("[LEASE_LOST_DURING_DEPLOY] cycle=%s target=%s pod=%s epoch=%d — outcome unknown", cycleID, targetID, podID, leaseEpoch)
		setExecStage(r.app, opID, ExecStageProvisioning, ExecStageInterruptedUnknownOutcome)
		r.completeOperation(opID, "failed", "lease lost during provider call; outcome unknown; operator MUST investigate")
		r.writeHistory(rolloutID, targetID, historyAction, "failed", "", "", "lease lost during provider mutation; outcome unknown", targetProvider, opID, "lease-lost-mid-call")
		// Release the target back to error so an operator (or the new lease holder)
		// can re-evaluate. We CANNOT release to "running" — that would lie.
		r.releaseTarget(opID, targetID, "creating", "error", "lease lost during deploy; cloud state unknown")
		r.RecordDecision(DecisionRecord{
			TargetID:       targetID,
			RolloutID:      rolloutID,
			OperationID:    opID,
			CycleID:        cycleID,
			DecisionType:   DecisionLeaseLostMidCall,
			DecisionReason: "lease_lost_during_provider_mutation",
			HumanSummary:   "lease lost during deploy; cloud-side outcome unknown; operator must investigate",
			DecisionInputs: map[string]interface{}{
				"this_pod":          podID,
				"lease_epoch":       leaseEpoch,
				"provider_duration": durationMs,
				"kind":              "deploy",
			},
			EvidenceRefs: []EvidenceRef{{Collection: "operations", ID: opID}},
		})
		return reconcileFailed
	default:
		// lease still held; proceed to outcome interpretation
	}

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

	// Phase 3.9.8: canonical provider name for deploy-result observations.
	deployObsProvider := providerToProviderName(account.Provider)

	// 6. Branch on outcome.
	switch {
	case deployErr != nil:
		// Hard error: refuse to claim success. Phase 2.6: a hard error is
		// not a stale outcome; reset the stale counter so we don't carry
		// stale-loop state across unrelated failures.
		// Phase 3.9.3 Runtime Integration: route through the canonical
		// retry decision engine. The action is authoritative — no implicit
		// "release-and-wait-for-tick" fallback.
		msg := sanitizeError(deployErr.Error())
		leaseRemaining := LeaseDuration
		leaseValid := r.VerifyLease(targetID, podID, leaseEpoch)
		classDecision, engineDecision := r.resolveRetryDecision(opID, p, deployErr, leaseRemaining, leaseValid)

		// Phase 3.9.8: record ambiguous deploy-result observation before any
		// runtime state changes. Deploy error means provider state is unknown.
		r.RecordProviderObservation(ProviderObservationInput{
			TargetID:          targetID,
			OperationID:       opID,
			CycleID:           cycleID,
			Provider:          deployObsProvider,
			ObservationSource: ObservationSourceDeployResult,
			ProviderState:     providers.CanonicalStateUnknown,
			Trust:             ObservationTrustAmbiguous,
			EvidenceJSON: map[string]interface{}{
				"error_class":     string(classDecision.Class),
				"native_code":     classDecision.NativeCode,
				"forensic_reason": classDecision.ForensicReason,
			},
		})

		setExecStage(r.app, opID, ExecStageProvisioning, ExecStageFailed)
		r.completeOperation(opID, "failed", msg+" ["+string(classDecision.Class)+"]")
		r.recordTargetFailureWithClass(targetID, classDecision.Class)
		r.recordRetryState(opID, classDecision.Class, r.loadRetryCount(opID)+1, engineDecision.NextAttemptAt, classDecision.NativeCode, classDecision.ForensicReason)
		r.clearStaleCount(targetID)
		r.applyRetryAction(engineDecision, classDecision, opID, rolloutID, targetID, historyAction, targetProvider, "creating", msg, cycleID)
		return reconcileFailed

	case result != nil && result.Status == "error":
		// Deploy returned (result, nil) but result.Status=error: provider
		// observed a Ready=FAILED condition. Spec-validation-style failure.
		// Phase 3.9.3: classify as RetryClassNever (provider rejected the spec).
		msg := sanitizeError(result.Message)

		// Phase 3.9.8: record authoritative failure observation — provider
		// explicitly confirmed error state.
		r.RecordProviderObservation(ProviderObservationInput{
			TargetID:          targetID,
			OperationID:       opID,
			CycleID:           cycleID,
			Provider:          deployObsProvider,
			ObservationSource: ObservationSourceDeployResult,
			ProviderState:     providers.CanonicalStateError,
			Trust:             ObservationTrustAuthoritative,
			EvidenceJSON: map[string]interface{}{
				"result_message": result.Message,
				"external_id":    result.ExternalID,
				"retry_class":    string(providers.RetryClassNever),
			},
		})

		setExecStage(r.app, opID, ExecStageProvisioning, ExecStageFailed)
		r.completeOperation(opID, "failed", msg+" [retry_never]")
		r.writeHistory(rolloutID, targetID, historyAction, "failed", "", result.ExternalID, msg, targetProvider, opID, string(providers.RetryClassNever))
		r.releaseTargetWithExternal(opID, targetID, "creating", "error", result.ExternalID, "", msg)
		r.recordTargetFailureWithClass(targetID, providers.RetryClassNever)
		r.recordRetryState(opID, providers.RetryClassNever, r.loadRetryCount(opID)+1, time.Time{}, "PROVIDER_REJECTED", msg)
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
			setExecStage(r.app, opID, ExecStageProvisioning, ExecStageFailed)
			r.completeOperation(opID, "succeeded_stale", msg)
			r.writeHistory(rolloutID, targetID, historyAction, "failed", "", result.ExternalID, msg, targetProvider, opID, "stale-spec-loop")
			r.releaseTargetWithExternal(opID, targetID, "creating", "error", result.ExternalID, "", msg)
			// Don't clear staleCount here: it stays high until operator
			// respec triggers the pending-entry clear. This prevents an
			// immediate-tick repeat from re-acquiring after one cycle.
			return reconcileFailed
		}
		log.Printf("[DEPLOY_STALE] cycle=%s target=%s operation=%s stale_count=%d — rollout moved during deploy", cycleID, targetID, opID, staleN)
		setExecStage(r.app, opID, ExecStageProvisioning, ExecStageFailed)
		r.completeOperation(opID, "succeeded_stale", "rollout updated during deploy; re-dispatching next cycle")
		r.writeHistory(rolloutID, targetID, historyAction, "failed", "", result.ExternalID, "stale spec", targetProvider, opID, "stale-spec")
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
		//
		// ExecStage: Phase 3.1 providers execute provisioning+verifying
		// atomically inside Deploy(). Advance through both stages now so the
		// operation ends at READY — the canonical terminal-success stage.
		// Write checkpoint before advancing so it lands while stage=PROVISIONING.
		// Phase 3.4 (SC-1): write rollout_revision into checkpoint_data alongside
		// provider-native keys (task_def_arn, etc.). This creates the deployed_spec
		// baseline: the reconciler can now compare rollout.updated_at vs
		// last_deployed_revision on the target to detect spec drift without a
		// full spec-content comparison.

		// Phase 3.9.8: record inferred deploy-result observation before any
		// runtime state changes. Trust is inferred — we set the target to
		// "updating" and wait for GetStatus to confirm convergence to "running".
		// RuntimeExpectedState is "" so contradiction detection is skipped;
		// the ongoing GetStatus polls (ObservationSourcePoll) handle that.
		{
			resultState := providers.NormalizeProviderState(result.Status)
			if resultState == providers.CanonicalStateUnknown || resultState == "" {
				resultState = providers.CanonicalStateCreating
			}
			r.RecordProviderObservation(ProviderObservationInput{
				TargetID:          targetID,
				OperationID:       opID,
				CycleID:           cycleID,
				Provider:          deployObsProvider,
				ObservationSource: ObservationSourceDeployResult,
				ProviderState:     resultState,
				Trust:             ObservationTrustInferred,
				EvidenceJSON: map[string]interface{}{
					"external_id":     result.ExternalID,
					"post_status":     postSuccessStatus,
					"rollout_revision": rolloutRevision,
				},
			})
		}

		checkpointWithRevision := result.CheckpointData
		if checkpointWithRevision == nil {
			checkpointWithRevision = make(map[string]interface{})
		}
		checkpointWithRevision["rollout_revision"] = rolloutRevision
		r.setCheckpointData(opID, checkpointWithRevision)
		setExecStage(r.app, opID, ExecStageProvisioning, ExecStageVerifying)
		setExecStage(r.app, opID, ExecStageVerifying, ExecStageReady)
		r.completeOperation(opID, "succeeded", "deploy completed")
		r.writeHistory(rolloutID, targetID, historyAction, "success", "", result.ExternalID, "", targetProvider, opID, "")
		r.releaseTargetWithExternal(opID, targetID, "creating", postSuccessStatus, result.ExternalID, result.ExternalID, postSuccessMessage)
		// Phase 3.4: record the deployed revision on the target so future
		// GetStatus ticks can detect spec drift by comparing rollout.updated_at
		// vs last_deployed_revision. Written after release so the target row
		// already reflects the new status.
		r.setLastDeployedRevision(targetID, rolloutRevision)
		// Phase 2.7: forensic history when sweep reclaimed ownership
		// while we were running. The dispatcher actually saw a success
		// provider-side; the sweep already wrote an "abandoned" history
		// row. This row gives operators the dispatcher's view.
		if ok, present := r.releaseStillOwner(opID); present && !ok {
			r.writeOwnershipLostHistory(rolloutID, targetID, historyAction, "success", result.ExternalID, targetProvider, opID)
		}
		if pendingDestroy {
			r.clearPendingDestroy(targetID)
			log.Printf("[CLOUD_DESTROY_REARMED] cycle=%s target=%s reason=in_flight_intent_honored", cycleID, targetID)
		}
		r.clearTargetFailure(targetID)
		r.clearStaleCount(targetID)
		// Phase 3.9.3: clear persisted retry schedule on success so the
		// next failure starts at the bottom of the backoff curve.
		r.clearRetrySchedule(targetID)
		if r.metrics != nil {
			r.metrics.IncDeploySucceeded()
		}
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
	opID, err := r.createOperation(targetID, "destroy", "", cycleID)
	if err != nil {
		log.Printf("[DISPATCH_OP_CREATE_ERR] cycle=%s target=%s error=%v", cycleID, targetID, err)
		return reconcileFailed
	}
	// ExecStage: operation created; not yet claimed. Stage = QUEUED.
	setExecStage(r.app, opID, "", ExecStageQueued)

	// Phase 3.9.2: lease acquisition before CAS claim.
	setExecStage(r.app, opID, ExecStageQueued, ExecStageLeaseAcquiring)
	podID := string(currentPodIdentity())
	leaseEpoch, leaseErr := r.AcquireLease(targetID, podID)
	if leaseErr != nil {
		if errors.Is(leaseErr, ErrLeaseHeld) {
			log.Printf("[LEASE_DENIED] cycle=%s target=%s pod=%s kind=destroy reason=foreign_pod_holds_lease", cycleID, targetID, podID)
		} else {
			log.Printf("[LEASE_ACQUIRE_ERR] cycle=%s target=%s pod=%s kind=destroy err=%v", cycleID, targetID, podID, leaseErr)
		}
		r.cancelOperation(opID, "destroy lease denied: "+leaseErr.Error())
		setExecStage(r.app, opID, ExecStageLeaseAcquiring, ExecStageLeaseLost)
		r.RecordDecision(DecisionRecord{
			TargetID:       targetID, RolloutID: rolloutID, OperationID: opID, CycleID: cycleID,
			DecisionType:   DecisionLeaseAcquireDenied,
			DecisionReason: "foreign_pod_holds_active_lease",
			HumanSummary:   "destroy lease acquisition denied; foreign pod holds active lease",
			DecisionInputs: map[string]interface{}{"this_pod": podID, "acquire_error": leaseErr.Error(), "kind": "destroy"},
			EvidenceRefs:   []EvidenceRef{{Collection: "operations", ID: opID}},
		})
		return reconcileSkipped
	}
	defer func() {
		if err := r.ReleaseLease(targetID, podID, leaseEpoch); err != nil {
			log.Printf("[LEASE_RELEASE_ERR] cycle=%s target=%s pod=%s epoch=%d kind=destroy err=%v", cycleID, targetID, podID, leaseEpoch, err)
		}
	}()

	claimed, err := r.claimTarget(targetID, opID)
	if err != nil || claimed != claimSucceeded {
		r.cancelOperation(opID, "destroy claim lost or errored")
		setExecStage(r.app, opID, ExecStageLeaseAcquiring, ExecStageFailed)
		// Phase 2.7: forensic history for cancelled destroy attempts.
		r.writeHistory(rolloutID, targetID, "deleted", "failed", "", externalID, "destroy claim lost or errored", targetProvider, opID, "")
		r.RecordDecision(DecisionRecord{
			TargetID:       targetID, RolloutID: rolloutID, OperationID: opID, CycleID: cycleID,
			DecisionType:   DecisionDispatchClaimLost,
			DecisionReason: "intra_pod_cas_race_lost",
			HumanSummary:   "destroy claimTarget CAS lost",
			DecisionInputs: map[string]interface{}{"kind": "destroy"},
			EvidenceRefs:   []EvidenceRef{{Collection: "operations", ID: opID}},
		})
		return reconcileSkipped
	}

	// ExecStage: CAS claimed. Stage = DISPATCHING (about to call provider).
	setExecStage(r.app, opID, ExecStageLeaseAcquiring, ExecStageDispatching)

	// Phase 3.9.2: lease refresh sidecar for destroy.
	leaseLostCh := make(chan struct{}, 1)
	leaseRefreshCtx, leaseRefreshCancel := context.WithCancel(ctx)
	defer leaseRefreshCancel()
	go r.LeaseRefreshLoop(leaseRefreshCtx, targetID, podID, leaseEpoch, leaseLostCh)

	log.Printf("[DISPATCH_CLAIM] cycle=%s target=%s operation=%s kind=destroy pod=%s", cycleID, targetID, opID, currentPodIdentity())
	r.writeHistory(rolloutID, targetID, "deleted", "in_progress", "", "", "", targetProvider, opID, "")
	if r.metrics != nil {
		r.metrics.IncDestroyDispatched()
	}

	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[DISPATCH_PANIC] cycle=%s target=%s op=%s kind=destroy panic=%v", cycleID, targetID, opID, rec)
			r.completeOperation(opID, "failed", "dispatcher panic")
			r.releaseTarget(opID, targetID, "deleting", "error", "dispatcher panic")
			r.writeHistory(rolloutID, targetID, "deleted", "failed", "", "", "dispatcher panic", targetProvider, opID, "dispatcher-panic")
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

	// Phase 3.9.2: lease verification before destroy call.
	if !r.VerifyLease(targetID, podID, leaseEpoch) {
		log.Printf("[LEASE_VERIFY_FAIL_PRE_DESTROY] cycle=%s target=%s pod=%s epoch=%d", cycleID, targetID, podID, leaseEpoch)
		setExecStage(r.app, opID, ExecStageDispatching, ExecStageLeaseLost)
		r.completeOperation(opID, "failed", "lease lost before destroy; no cloud changes made")
		r.writeHistory(rolloutID, targetID, "deleted", "failed", "", externalID, "lease lost before destroy", targetProvider, opID, "lease-lost-pre-call")
		// Release target back to deleting state so the next eligible holder picks it up.
		r.releaseTarget(opID, targetID, "deleting", "deleting", "lease lost; safe to re-dispatch")
		r.RecordDecision(DecisionRecord{
			TargetID:       targetID, RolloutID: rolloutID, OperationID: opID, CycleID: cycleID,
			DecisionType:   DecisionLeaseLostPreCall,
			DecisionReason: "lease_invalid_pre_provider_call",
			HumanSummary:   "lease invalid before destroy call; no provider mutation occurred",
			DecisionInputs: map[string]interface{}{"this_pod": podID, "lease_epoch": leaseEpoch, "kind": "destroy"},
			EvidenceRefs:   []EvidenceRef{{Collection: "operations", ID: opID}},
		})
		return reconcileFailed
	}

	// ExecStage: provider Destroy() initiation is imminent. Stage = DELETING.
	setExecStage(r.app, opID, ExecStageDispatching, ExecStageDeleting)

	if err := p.Destroy(destroyCtx, account, target); err != nil {
		msg := sanitizeError(err.Error())
		// Phase 3.9.3 Runtime Integration: canonical retry routing.
		leaseRemaining := LeaseDuration
		leaseValid := r.VerifyLease(targetID, podID, leaseEpoch)
		classDecision, engineDecision := r.resolveRetryDecision(opID, p, err, leaseRemaining, leaseValid)
		setExecStage(r.app, opID, ExecStageDeleting, ExecStageFailed)
		r.completeOperation(opID, "failed", msg+" ["+string(classDecision.Class)+"]")
		r.recordTargetFailureWithClass(targetID, classDecision.Class)
		r.recordRetryState(opID, classDecision.Class, r.loadRetryCount(opID)+1, engineDecision.NextAttemptAt, classDecision.NativeCode, classDecision.ForensicReason)
		r.applyRetryAction(engineDecision, classDecision, opID, rolloutID, targetID, "deleted", targetProvider, "deleting", msg, cycleID)
		return reconcileFailed
	}

	// Phase 3.9.2: check for lease loss DURING destroy. UpdateService+DeleteService
	// may have committed cloud-side; outcome is unknown if we lost ownership.
	select {
	case <-leaseLostCh:
		log.Printf("[LEASE_LOST_DURING_DESTROY] cycle=%s target=%s pod=%s epoch=%d — outcome unknown", cycleID, targetID, podID, leaseEpoch)
		setExecStage(r.app, opID, ExecStageDeleting, ExecStageInterruptedUnknownOutcome)
		r.completeOperation(opID, "failed", "lease lost during destroy; outcome unknown")
		r.writeHistory(rolloutID, targetID, "deleted", "failed", "", "", "lease lost during destroy; outcome unknown", targetProvider, opID, "lease-lost-mid-call")
		r.releaseTarget(opID, targetID, "deleting", "error", "lease lost during destroy")
		r.RecordDecision(DecisionRecord{
			TargetID:       targetID, RolloutID: rolloutID, OperationID: opID, CycleID: cycleID,
			DecisionType:   DecisionLeaseLostMidCall,
			DecisionReason: "lease_lost_during_provider_mutation",
			HumanSummary:   "lease lost during destroy; cloud-side outcome unknown",
			DecisionInputs: map[string]interface{}{"this_pod": podID, "lease_epoch": leaseEpoch, "kind": "destroy"},
			EvidenceRefs:   []EvidenceRef{{Collection: "operations", ID: opID}},
		})
		return reconcileFailed
	default:
	}

	// Phase 3.3: destroy-confirm is owned by the reconciler, not the provider.
	// Poll GetStatus until deletion is confirmed or the window elapses.
	// On timeout, exec stage moves to AMBIGUOUS and target is released to
	// "ambiguous" so the next GetStatus tick can re-confirm.
	providerName := providerToProviderName(account.Provider)
	if err := r.confirmDestroyInReconciler(destroyCtx, p, account, target, providerName, cycleID, opID); err != nil {
		msg := sanitizeError(err.Error())
		setExecStage(r.app, opID, ExecStageDeleting, ExecStageAmbiguous)
		r.completeOperation(opID, "failed", "destroy initiated but confirmation timed out: "+msg)
		r.writeHistory(rolloutID, targetID, "deleted", "failed", "", "", "destroy confirmation timeout: "+msg, targetProvider, opID, "destroy-confirm-timeout")
		r.releaseTarget(opID, targetID, "deleting", "ambiguous", "destroy confirmation timed out: "+msg)
		// Do NOT record as target failure — ambiguous is not a clean error;
		// GetStatus will resolve it on the next tick.
		return reconcileFailed
	}

	// ExecStage: reconciler confirmed deletion. Stage = DELETED (terminal).
	setExecStage(r.app, opID, ExecStageDeleting, ExecStageDeleted)
	r.completeOperation(opID, "succeeded", "destroy completed and confirmed")
	r.writeHistory(rolloutID, targetID, "deleted", "success", "", "", "", targetProvider, opID, "")
	r.releaseTarget(opID, targetID, "deleting", "deleted", "destroy completed")
	// On successful destroy, clear pending_destroy in case it was set by a
	// late-arriving intent during the destroy itself (paranoid; usually
	// already false).
	r.clearPendingDestroy(targetID)
	r.clearTargetFailure(targetID)
	r.clearRetrySchedule(targetID)
	if r.metrics != nil {
		r.metrics.IncDestroySucceeded()
	}
	return reconcileSuccess
}

// dispatchRollback is the Rollback-side mirror of dispatchDeploy/dispatchDestroy.
// It shares the same Phase 2.1 hardening: opID threaded into release, heartbeat
// sidecar, dispatcher-local panic recovery, and exec_stage wiring.
//
// Capability gate: the Provider must declare Supported=true for CapRollback
// (dispatch_tables.go). If not, we immediately flip the target to `error`
// with a clear explanation — the operator must use a manual re-deploy instead.
//
// ExecStage path: QUEUED → DISPATCHING → ROLLING_BACK → ROLLED_BACK | FAILED | AMBIGUOUS
//
// Phase 3.2: AMBIGUOUS on rollback timeout is declared but not yet implemented
// (Provider.Rollback is expected to block until confirmed in Phase 3.1 providers).
// A future provider that returns partial rollback signals will drive that path.
func (r *Reconciler) dispatchRollback(
	ctx context.Context,
	p providers.Provider,
	account *providers.CloudAccount,
	targetID, rolloutID, externalID, targetRevision string,
	cycleID, targetProvider string,
) (resultOut reconcileResult) {
	// Capability gate: refuse rollback if the provider doesn't support it.
	providerName := providerToProviderName(account.Provider)
	if !supportsCapability(providerName, providers.CapRollback) {
		notes := capabilityNotes(providerName, providers.CapRollback)
		capErr := providers.ErrCapabilityUnavailable{
			Cap:      providers.CapRollback,
			Provider: providerName,
			Notes:    notes,
		}
		log.Printf("[ROLLBACK_CAPABILITY_REFUSED] cycle=%s target=%s provider=%s reason=%v", cycleID, targetID, providerName, capErr.Error())
		r.markTargetError(targetID, "rollback not supported by provider: "+capErr.Error())
		return reconcileFailed
	}

	opID, err := r.createOperation(targetID, "rollback", targetRevision, cycleID)
	if err != nil {
		log.Printf("[DISPATCH_OP_CREATE_ERR] cycle=%s target=%s kind=rollback error=%v", cycleID, targetID, err)
		return reconcileFailed
	}
	// ExecStage: operation created; not yet claimed. Stage = QUEUED.
	setExecStage(r.app, opID, "", ExecStageQueued)

	// Phase 3.9.2: lease acquisition for rollback.
	setExecStage(r.app, opID, ExecStageQueued, ExecStageLeaseAcquiring)
	podID := string(currentPodIdentity())
	leaseEpoch, leaseErr := r.AcquireLease(targetID, podID)
	if leaseErr != nil {
		log.Printf("[LEASE_DENIED] cycle=%s target=%s pod=%s kind=rollback err=%v", cycleID, targetID, podID, leaseErr)
		r.cancelOperation(opID, "rollback lease denied: "+leaseErr.Error())
		setExecStage(r.app, opID, ExecStageLeaseAcquiring, ExecStageLeaseLost)
		r.RecordDecision(DecisionRecord{
			TargetID:       targetID, RolloutID: rolloutID, OperationID: opID, CycleID: cycleID,
			DecisionType:   DecisionLeaseAcquireDenied,
			DecisionReason: "foreign_pod_holds_active_lease",
			HumanSummary:   "rollback lease acquisition denied",
			DecisionInputs: map[string]interface{}{"this_pod": podID, "acquire_error": leaseErr.Error(), "kind": "rollback"},
			EvidenceRefs:   []EvidenceRef{{Collection: "operations", ID: opID}},
		})
		return reconcileSkipped
	}
	defer func() {
		if err := r.ReleaseLease(targetID, podID, leaseEpoch); err != nil {
			log.Printf("[LEASE_RELEASE_ERR] cycle=%s target=%s pod=%s kind=rollback err=%v", cycleID, targetID, podID, err)
		}
	}()

	claimed, err := r.claimTarget(targetID, opID)
	if err != nil || claimed != claimSucceeded {
		r.cancelOperation(opID, "rollback claim lost or errored")
		setExecStage(r.app, opID, ExecStageLeaseAcquiring, ExecStageFailed)
		r.writeHistory(rolloutID, targetID, "rollback", "failed", "", externalID, "rollback claim lost or errored", targetProvider, opID, "")
		r.RecordDecision(DecisionRecord{
			TargetID:       targetID, RolloutID: rolloutID, OperationID: opID, CycleID: cycleID,
			DecisionType:   DecisionDispatchClaimLost,
			DecisionReason: "intra_pod_cas_race_lost",
			HumanSummary:   "rollback claimTarget CAS lost",
			DecisionInputs: map[string]interface{}{"kind": "rollback"},
			EvidenceRefs:   []EvidenceRef{{Collection: "operations", ID: opID}},
		})
		return reconcileSkipped
	}

	// ExecStage: CAS claimed. Stage = DISPATCHING.
	setExecStage(r.app, opID, ExecStageLeaseAcquiring, ExecStageDispatching)

	// Phase 3.9.2: lease refresh sidecar for rollback.
	leaseLostCh := make(chan struct{}, 1)
	leaseRefreshCtx, leaseRefreshCancel := context.WithCancel(ctx)
	defer leaseRefreshCancel()
	go r.LeaseRefreshLoop(leaseRefreshCtx, targetID, podID, leaseEpoch, leaseLostCh)

	log.Printf("[DISPATCH_CLAIM] cycle=%s target=%s operation=%s kind=rollback to_revision=%s pod=%s", cycleID, targetID, opID, targetRevision, currentPodIdentity())
	r.writeHistory(rolloutID, targetID, "rollback", "in_progress", "", "", "", targetProvider, opID, "operator-rollback")
	if r.metrics != nil {
		r.metrics.IncRollbackDispatched()
	}

	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[DISPATCH_PANIC] cycle=%s target=%s op=%s kind=rollback panic=%v", cycleID, targetID, opID, rec)
			r.completeOperation(opID, "failed", "dispatcher panic")
			r.releaseTarget(opID, targetID, "rolling_back", "error", "dispatcher panic")
			r.writeHistory(rolloutID, targetID, "rollback", "failed", "", "", "dispatcher panic", targetProvider, opID, "dispatcher-panic")
			resultOut = reconcileFailed
		}
	}()

	rollbackCtx, cancel := context.WithTimeout(ctx, DeployTimeout)
	defer cancel()

	go r.heartbeat(ctx, opID)

	target := &providers.DeploymentTarget{
		ID:         targetID,
		ExternalID: externalID,
		Provider:   account.Provider,
		Region:     account.Region,
	}

	// Phase 3.9.2: verify lease before rollback call.
	if !r.VerifyLease(targetID, podID, leaseEpoch) {
		log.Printf("[LEASE_VERIFY_FAIL_PRE_ROLLBACK] cycle=%s target=%s pod=%s epoch=%d", cycleID, targetID, podID, leaseEpoch)
		setExecStage(r.app, opID, ExecStageDispatching, ExecStageLeaseLost)
		r.completeOperation(opID, "failed", "lease lost before rollback")
		r.writeHistory(rolloutID, targetID, "rollback", "failed", "", externalID, "lease lost before rollback", targetProvider, opID, "lease-lost-pre-call")
		r.releaseTarget(opID, targetID, "rolling_back", "error", "lease lost before rollback")
		r.RecordDecision(DecisionRecord{
			TargetID:       targetID, RolloutID: rolloutID, OperationID: opID, CycleID: cycleID,
			DecisionType:   DecisionLeaseLostPreCall,
			DecisionReason: "lease_invalid_pre_provider_call",
			HumanSummary:   "lease invalid before rollback call; no provider mutation occurred",
			DecisionInputs: map[string]interface{}{"this_pod": podID, "lease_epoch": leaseEpoch, "kind": "rollback"},
			EvidenceRefs:   []EvidenceRef{{Collection: "operations", ID: opID}},
		})
		return reconcileFailed
	}

	// ExecStage: provider Rollback() call is imminent. Stage = ROLLING_BACK.
	setExecStage(r.app, opID, ExecStageDispatching, ExecStageRollingBack)

	result, rollbackErr := p.Rollback(rollbackCtx, account, target, targetRevision)

	// Phase 3.9.2: check lease loss during rollback. The cloud-side
	// UpdateService may have already updated to the prior task definition;
	// outcome is unknown without our authority to confirm.
	select {
	case <-leaseLostCh:
		log.Printf("[LEASE_LOST_DURING_ROLLBACK] cycle=%s target=%s pod=%s epoch=%d", cycleID, targetID, podID, leaseEpoch)
		setExecStage(r.app, opID, ExecStageRollingBack, ExecStageInterruptedUnknownOutcome)
		r.completeOperation(opID, "failed", "lease lost during rollback; outcome unknown")
		r.writeHistory(rolloutID, targetID, "rollback", "failed", "", externalID, "lease lost during rollback", targetProvider, opID, "lease-lost-mid-call")
		r.releaseTarget(opID, targetID, "rolling_back", "error", "lease lost during rollback")
		r.recordRetryState(opID, providers.RetryClassProviderUncertain, r.loadRetryCount(opID)+1, time.Time{}, "LEASE_LOST", "lease lost mid-rollback")
		r.RecordDecision(DecisionRecord{
			TargetID:       targetID, RolloutID: rolloutID, OperationID: opID, CycleID: cycleID,
			DecisionType:   DecisionLeaseLostMidCall,
			DecisionReason: "lease_lost_during_provider_mutation",
			HumanSummary:   "lease lost during rollback; cloud-side outcome unknown",
			DecisionInputs: map[string]interface{}{"this_pod": podID, "lease_epoch": leaseEpoch, "kind": "rollback"},
			EvidenceRefs:   []EvidenceRef{{Collection: "operations", ID: opID}},
		})
		return reconcileFailed
	default:
	}
	if rollbackErr != nil {
		msg := sanitizeError(rollbackErr.Error())
		// Phase 3.9.3 Runtime Integration: canonical retry routing.
		leaseRemaining := LeaseDuration
		leaseValid := r.VerifyLease(targetID, podID, leaseEpoch)
		classDecision, engineDecision := r.resolveRetryDecision(opID, p, rollbackErr, leaseRemaining, leaseValid)

		// Phase 3.9.8: record ambiguous rollback-result observation before
		// any runtime state changes. Rollback error means provider state unknown.
		r.RecordProviderObservation(ProviderObservationInput{
			TargetID:          targetID,
			OperationID:       opID,
			CycleID:           cycleID,
			Provider:          providerName,
			ObservationSource: ObservationSourceRollbackResult,
			ProviderState:     providers.CanonicalStateUnknown,
			Trust:             ObservationTrustAmbiguous,
			EvidenceJSON: map[string]interface{}{
				"error_class":     string(classDecision.Class),
				"native_code":     classDecision.NativeCode,
				"forensic_reason": classDecision.ForensicReason,
			},
		})

		setExecStage(r.app, opID, ExecStageRollingBack, ExecStageFailed)
		r.completeOperation(opID, "failed", msg+" ["+string(classDecision.Class)+"]")
		r.recordTargetFailureWithClass(targetID, classDecision.Class)
		r.recordRetryState(opID, classDecision.Class, r.loadRetryCount(opID)+1, engineDecision.NextAttemptAt, classDecision.NativeCode, classDecision.ForensicReason)
		r.applyRetryAction(engineDecision, classDecision, opID, rolloutID, targetID, "rollback", targetProvider, "rolling_back", msg, cycleID)
		return reconcileFailed
	}
	// Provider-observed failure status (e.g., revision not found).
	if result != nil && result.Status == "error" {
		msg := sanitizeError(result.Message)

		// Phase 3.9.8: record authoritative failure observation before runtime
		// state changes. Provider explicitly confirmed rollback failed.
		r.RecordProviderObservation(ProviderObservationInput{
			TargetID:          targetID,
			OperationID:       opID,
			CycleID:           cycleID,
			Provider:          providerName,
			ObservationSource: ObservationSourceRollbackResult,
			ProviderState:     providers.CanonicalStateError,
			Trust:             ObservationTrustAuthoritative,
			EvidenceJSON: map[string]interface{}{
				"result_message": result.Message,
				"external_id":    result.ExternalID,
				"retry_class":    string(providers.RetryClassNever),
			},
		})

		setExecStage(r.app, opID, ExecStageRollingBack, ExecStageFailed)
		r.completeOperation(opID, "failed", msg+" [retry_never]")
		r.writeHistory(rolloutID, targetID, "rollback", "failed", "", result.ExternalID, msg, targetProvider, opID, string(providers.RetryClassNever))
		r.releaseTargetWithExternal(opID, targetID, "rolling_back", "error", result.ExternalID, "", msg)
		r.recordTargetFailureWithClass(targetID, providers.RetryClassNever)
		r.recordRetryState(opID, providers.RetryClassNever, r.loadRetryCount(opID)+1, time.Time{}, "PROVIDER_REJECTED", msg)
		return reconcileFailed
	}

	// ExecStage: Rollback() confirmed. Stage = ROLLED_BACK (terminal).
	// Phase 3.9.8: record authoritative rolled-back observation before any
	// runtime state changes.
	r.RecordProviderObservation(ProviderObservationInput{
		TargetID:             targetID,
		OperationID:          opID,
		CycleID:              cycleID,
		Provider:             providerName,
		ObservationSource:    ObservationSourceRollbackResult,
		ProviderState:        providers.CanonicalStateRolledBack,
		RuntimeExpectedState: "rolled_back",
		Trust:                ObservationTrustAuthoritative,
		EvidenceJSON: map[string]interface{}{
			"target_revision": targetRevision,
		},
	})

	rolledToRevision := targetRevision
	if result != nil && result.ExternalID != "" {
		rolledToRevision = result.ExternalID
	}
	setExecStage(r.app, opID, ExecStageRollingBack, ExecStageRolledBack)
	r.completeOperation(opID, "succeeded", "rollback completed")
	r.writeHistory(rolloutID, targetID, "rollback", "success", targetRevision, rolledToRevision, "", targetProvider, opID, "operator-rollback")
	r.releaseTarget(opID, targetID, "rolling_back", "rolled_back", "rollback completed")
	r.clearTargetFailure(targetID)
	r.clearRetrySchedule(targetID)
	if r.metrics != nil {
		r.metrics.IncRollbackSucceeded()
	}
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

// confirmDestroyInReconciler polls Provider.GetStatus until the target
// reports status="deleted" or the confirmation window elapses.
//
// Phase 3.3 destroy-confirm extraction: the provider's Destroy() method
// now only initiates the delete (drain + service deletion). The reconciler
// owns the confirmation loop so that timeout policy, ambiguity semantics,
// and history events are uniform across all providers.
//
// The poll interval is fixed at 5s (matching the provider-internal confirm.go
// constant). The timeout comes from the provider's CapDestroyConfirmation
// UncertaintyP99 via destroyConfirmDispatch(). An unrecognised provider
// semantic fails safe (returns error; target stays in ambiguous state).
//
// Returns nil on confirmed deletion. Returns a descriptive error on timeout
// or on an unrecognised semantic — the caller must set exec stage to
// AMBIGUOUS and release the target to "ambiguous".
func (r *Reconciler) confirmDestroyInReconciler(
	ctx context.Context,
	p providers.Provider,
	account *providers.CloudAccount,
	target *providers.DeploymentTarget,
	providerName, cycleID, opID string,
) error {
	semantic, timeout, err := destroyConfirmDispatch(providerName)
	if err != nil {
		return fmt.Errorf("destroy-confirm dispatch table error: %w", err)
	}
	if semantic == "none" || timeout == 0 {
		// Provider has no destroy confirmation; treat initiation as deletion.
		return nil
	}

	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	const pollInterval = 5 * time.Second
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	log.Printf("[DESTROY_CONFIRM_START] cycle=%s op=%s provider=%s semantic=%s timeout=%s",
		cycleID, opID, providerName, semantic, timeout)

	for {
		select {
		case <-pollCtx.Done():
			return fmt.Errorf("destroy confirmation timed out after %s (semantic=%s)", timeout, semantic)
		case <-ticker.C:
			ts, err := p.GetStatus(pollCtx, account, target)
			if err != nil {
				// Phase 3.9.8: record ambiguous destroy-poll observation before
				// continuing. Provider state is unknown on error.
				retryDecision := providers.ClassifyProviderError(err)
				r.RecordProviderObservation(ProviderObservationInput{
					TargetID:             target.ID,
					OperationID:          opID,
					CycleID:              cycleID,
					Provider:             providerName,
					ObservationSource:    ObservationSourceReconciliationRead,
					ProviderState:        providers.CanonicalStateUnknown,
					RuntimeExpectedState: "deleted",
					Trust:                ObservationTrustAmbiguous,
					EvidenceJSON: map[string]interface{}{
						"destroy_confirm_semantic": semantic,
						"error_class":              string(retryDecision.Class),
						"native_code":              retryDecision.NativeCode,
					},
				})
				log.Printf("[DESTROY_CONFIRM_POLL_ERR] cycle=%s op=%s err=%v — continuing poll", cycleID, opID, err)
				continue
			}
			// Phase 3.9.8: record destroy-poll observation. Trust derives from
			// confidence level; ambiguity overrides to ambiguous.
			{
				obsTrust := classifyObservationTrustFromConfidence(ts.ConfidenceLevel)
				if ts.Ambiguous {
					obsTrust = ObservationTrustAmbiguous
				}
				normalizedState := providers.NormalizeProviderState(ts.Status)
				if !providers.IsCanonicalProviderState(normalizedState) {
					obsTrust = ObservationTrustAmbiguous
				}
				r.RecordProviderObservation(ProviderObservationInput{
					TargetID:             target.ID,
					OperationID:          opID,
					CycleID:              cycleID,
					Provider:             providerName,
					ObservationSource:    ObservationSourceReconciliationRead,
					ProviderState:        normalizedState,
					RuntimeExpectedState: "deleted",
					Trust:                obsTrust,
					EvidenceJSON: map[string]interface{}{
						"destroy_confirm_semantic": semantic,
						"confidence_level":         ts.ConfidenceLevel,
						"native_state":             ts.NativeState,
					},
				})
			}
			if ts != nil && ts.Status == "deleted" {
				log.Printf("[DESTROY_CONFIRM_OK] cycle=%s op=%s semantic=%s", cycleID, opID, semantic)
				return nil
			}
			log.Printf("[DESTROY_CONFIRM_POLL] cycle=%s op=%s status=%s — not yet deleted", cycleID, opID, ts.Status)
		}
	}
}

// setCheckpointData writes provider-specific checkpoint state to the
// operations.checkpoint_data JSON column. Called at exec stage transitions
// to make provider-internal state visible to operators.
//
// Phase 3.2 behaviour: data is WRITTEN for observability but NOT READ for
// continuation — restart uses the mark-failed-redispatch pattern. Phase 4
// will add smart continuation from checkpoint.
//
// Best-effort: errors are logged but do not propagate.
func (r *Reconciler) setCheckpointData(opID string, data map[string]interface{}) {
	if len(data) == 0 {
		return
	}
	j, err := json.Marshal(data)
	if err != nil {
		log.Printf("[CHECKPOINT_MARSHAL_ERR] op=%s err=%v", opID, err)
		return
	}
	_, err = r.app.Dao().DB().NewQuery(
		"UPDATE operations SET checkpoint_data = {:data}, updated_at = {:ts} WHERE id = {:id}",
	).Bind(dbx.Params{
		"data": j,
		"ts":   time.Now().UTC().Format(time.RFC3339),
		"id":   opID,
	}).Execute()
	if err != nil {
		log.Printf("[CHECKPOINT_WRITE_ERR] op=%s err=%v", opID, err)
	}
}

// setLastDeployedRevision updates deployment_targets.last_deployed_revision
// to the given rolloutRevision after a successful deploy. This is the Phase 3.4
// (SC-1) reconciliation baseline: future GetStatus ticks compare the desired
// rollout.updated_at against this value to detect spec drift.
//
// Best-effort: errors are logged but do not propagate. A missing write means
// drift_state will stay "unknown" for this target until the next successful deploy.
func (r *Reconciler) setLastDeployedRevision(targetID, rolloutRevision string) {
	if targetID == "" || rolloutRevision == "" {
		return
	}
	_, err := r.app.Dao().DB().NewQuery(
		"UPDATE deployment_targets SET last_deployed_revision = {:rev} WHERE id = {:id}",
	).Bind(dbx.Params{
		"rev": rolloutRevision,
		"id":  targetID,
	}).Execute()
	if err != nil {
		log.Printf("[SET_DEPLOYED_REVISION_ERR] target=%s err=%v", targetID, err)
	}
}

// createOperation inserts a new in_progress operation row and returns its ID.
// Phase 3.8: stamps owned_by_pod with the current process's identity for
// SC-5 ownership forensic visibility.
// Phase 3.9.6: stamps cycle_id (column existed since 1715300008 but was
// previously unused). Lets operators correlate operations + decisions
// by reconcile cycle.
func (r *Reconciler) createOperation(targetID, kind, rolloutRevision, cycleID string) (string, error) {
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
	// Phase 3.8: ownership stamp. Identity initialized at Start() time.
	rec.Set("owned_by_pod", string(currentPodIdentity()))
	// Phase 3.9.6: cycle correlation.
	if cycleID != "" {
		rec.Set("cycle_id", cycleID)
	}
	if err := r.app.Dao().SaveRecord(rec); err != nil {
		return "", err
	}
	return rec.Id, nil
}

// setDeployedSpec persists the Phase 3.5 deployed_spec JSON snapshot on
// the operation. Called from dispatchDeploy after spec construction so
// the spec is captured BEFORE the provider call — preserving the deploy-time
// truth baseline even if Deploy() fails.
//
// Best-effort: errors are logged but do not propagate. A missing
// deployed_spec means full-content drift detection is unavailable for
// this operation; revision-based drift detection still functions.
func (r *Reconciler) setDeployedSpec(opID string, spec *providers.DeploySpec) {
	if opID == "" || spec == nil {
		return
	}
	j, err := json.Marshal(spec)
	if err != nil {
		log.Printf("[DEPLOYED_SPEC_MARSHAL_ERR] op=%s err=%v", opID, err)
		return
	}
	_, err = r.app.Dao().DB().NewQuery(
		"UPDATE operations SET deployed_spec = {:spec}, updated_at = {:ts} WHERE id = {:id}",
	).Bind(dbx.Params{
		"spec": j,
		"ts":   time.Now().UTC().Format(time.RFC3339),
		"id":   opID,
	}).Execute()
	if err != nil {
		log.Printf("[DEPLOYED_SPEC_WRITE_ERR] op=%s err=%v", opID, err)
	}
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
func (r *Reconciler) writeOwnershipLostHistory(rolloutID, targetID, action, observedOutcome, externalID, targetProvider, opID string) {
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
	r.writeHistory(rolloutID, targetID, action, "failed", "", externalID, msg, targetProvider, opID, "sweep-ownership-reclaim")
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
//
// Phase 3.3: operationID links the history row to the operations table entry
// that caused it. Pass the opID where available; pass "" for paths that
// do not have an operation row (e.g. pre-claim cancel, sweep events).
//
// triggeredBy is a human-readable cause string for operator forensics
// (e.g. "stale-spec-loop", "operator-rollback", "ambiguity-expiry").
// Pass "" when the cause is implicit from the action field.
func (r *Reconciler) writeHistory(rolloutID, targetID, action, status, fromRevision, toRevision, message, provider, operationID, triggeredBy string) {
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
	if operationID != "" {
		rec.Set("operation_id", operationID)
	}
	if triggeredBy != "" {
		rec.Set("triggered_by", triggeredBy)
	}
	if err := r.app.Dao().SaveRecord(rec); err != nil {
		log.Printf("[HISTORY_WRITE_ERR] save: %v", err)
		return
	}
	log.Printf("[HISTORY_WRITE] target=%s action=%s status=%s op=%s", targetID, action, status, operationID)
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
