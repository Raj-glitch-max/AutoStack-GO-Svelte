# Incident Reconstruction Assessment — Phase 2.3

## Last Updated
2026-05-14

## Method

For each scripted incident below: what is the **current** behavior,
**what an operator can see**, **what is genuinely opaque**, and
**recovery path**. The pass criterion is that an on-call engineer can
reconstruct the timeline from `deployment_history` + log lines, without
needing to read the source code.

Phase 2.3's history-at-intents and cycle-id-into-dispatch fixes
materially improve several of these scenarios. The "after Phase 2.3"
column reflects post-fix expected behavior.

---

## Incident I-1: Deploy succeeds partially, then process crashes

**Scenario:** Dispatcher posts CreateService at T+30s. GCP accepts and
queues. Process SIGKILL'd at T+45s before `waitForServiceReady` could
observe Ready. Process restarts at T+50s.

**Before Phase 2.3:**
- Sweep finds in_progress op (op was 45s old, `updated_at` from
  heartbeat is 0s old since heartbeat hasn't ticked yet — heartbeat
  ticks at 60s).
- Sweep marks op `failed: abandoned`.
- Target → error.
- History row: `failed: abandoned`.
- GCP service: continues being created behind the scenes; may exist or
  may have aborted.
- Operator: sees `error`. Reads history, sees `created in_progress` +
  `error failed: abandoned`. Goes to GCP console, finds (maybe) a
  service. Decides recovery action.

**After Phase 2.3:**
- Sweep still marks abandoned (heartbeat <60s, but heartbeat-aware
  sweep only protects ops within `2 × heartbeatInterval = 2 min`,
  and op is only 45s old — at this scenario, op IS within heartbeat
  window).
- **Wait:** Heartbeat aware sweep means we'd SKIP this op. Then target
  stays `creating` with `current_operation` set. Reconciler skips
  status-poll (in-flight guard). Dispatcher process is dead. **Target
  is stuck.**

**Phase 2.3 design decision:** The heartbeat-aware sweep introduces a
risk: an op that died before heartbeating once is now protected for 2
min, leaving the target stuck. **Mitigation:** Sweep treats ops
without any successful heartbeat (where `updated_at == started_at`)
as abandoned regardless of age. Heartbeat skipping only applies once a
heartbeat has actually fired.

**Phase 2.3 with corrected design:** Sweep checks
`updated_at > started_at` to confirm at least one heartbeat fired. If
not, sweep abandons regardless of recency. **Same outcome as before
Phase 2.3 for this scenario.** ✓

**Reconstruction quality:** Good. History has two rows
(`in_progress`/`failed: abandoned`); logs show `[STARTUP_SWEEP]`,
`[OP_ABANDONED]`. Cycle correlation possible via Phase 2.3 cycle-id.

---

## Incident I-2: Process crashes mid-destroy

**Scenario:** Dispatcher calls `Provider.Destroy`. `DeleteService`
returns 200 at T+5s. Dispatcher about to write target → deleted at
T+6s. Process SIGKILL'd at T+5.5s.

**Behavior:**
- Sweep on restart finds in_progress destroy op.
- Sweep marks op `failed: abandoned`. Target → error.
- History row: `deleted failed: abandoned`.
- GCP service: deletion in progress or already gone.
- Reconciler next tick: target is `error`, no in-flight op,
  shouldDispatchDeploy=no (status != pending), shouldDispatchDestroy=no
  (status != deleting), falls to status-poll path. GetStatus calls
  `GetService` → returns NOT_FOUND → ClassifyError → permanent. Target
  status update: previousStatus=error, next=error (since deployment
  status would be NOT_FOUND mapped to permanent error). Actually
  GetStatus would return an error to the reconciler since GetService
  returned err, so error path: updateTargetStatus(error → error,
  msg=NOT_FOUND). isAllowedTransition(error, error) = true (same).
  Status stays `error` with a NOT_FOUND drift_summary.
- **Operator sees: target=error, drift_summary contains NOT_FOUND. They
  must conclude the service is gone and manually re-mark the target as
  `deleted` (PocketBase admin) OR set endDate on a non-existent
  rollout (rollout endDate is already set...).** No clean path.

**Reconstruction quality:** Adequate. History timeline is intact.
Recovery requires admin action. Documented as a Phase 2.5 "force-mark
deleted" endpoint deferred.

---

## Incident I-3: Cloud Run returns stale readiness mid-deploy

**Scenario:** Dispatcher posts UpdateService. Cloud Run reports
`Ready=SUCCEEDED` for the OLD revision while the new revision is still
RECONCILING.

**Behavior:**
- `waitForServiceReady` returns immediately (sees SUCCEEDED).
- Deploy returns success with stale-revision endpoint.
- Dispatcher releases target to `updating` (not `running`).
- Next status-poll: may see `RECONCILING` → status="creating". But
  transition guard refuses `updating → creating`. Status stays
  `updating`.
- Subsequent polls: traffic eventually shifts; `Ready=SUCCEEDED` on the
  new revision. Status promotes `updating → running`.

**Reconstruction quality:** Good. Logs show `[DEPLOY_END]` success,
then potentially `[TRANSITION_REFUSED] from=updating to=creating`, then
eventual `[STATE_TRANSITION] from=updating to=running`.

---

## Incident I-4: Sweep reclaims a stale ownership incorrectly

**Scenario:** Operator restarts the AutoStack pod (rolling). New pod's
sweep marks the old pod's live deploy as abandoned.

**Before Phase 2.3:**
- Sweep aggressively abandons regardless of heartbeat. New pod's first
  reconcile sees target = error, ignores it.
- Old pod's dispatcher eventually returns (it's still alive briefly).
  Tries `completeOperation` (CAS on status=in_progress fails → no-op).
  Tries `releaseTarget` (CAS on current_operation=opID fails → no-op).
  Both correctly silent; logged `[OP_COMPLETE_NOOP]` /
  `[RELEASE_LOST_OWNERSHIP]`.
- Cloud Run service: created by old pod, AutoStack reports `error`.
- Operator: investigates; sees the abandoned op and the live service.
  Decides whether to keep the service (force AutoStack to `running` via
  admin) or destroy and redeploy.

**After Phase 2.3:**
- Sweep skips ops with recent heartbeat. Old pod's op IS protected.
- Old pod completes deploy. Writes target → updating.
- New pod's reconciler observes target. Polls status. ✓

This is the key benefit: rolling restarts no longer destroy in-flight
ops.

**Reconstruction quality:** Excellent. History is uninterrupted.

---

## Incident I-5: Rollback interrupted by restart

**N/A.** Rollback is not implemented. Operator cannot trigger it. ✓

---

## Incident I-6: Deploy retry collides with delayed provider convergence

**Scenario:** Deploy succeeds at T+8min. Target → updating. At T+8.5min
rollout is respec'd. `flipCloudTargetsToPendingOnRespec` flips
updating → pending. Reconciler picks up at T+9min, dispatches deploy
again. Cloud Run is still propagating the previous revision's traffic.

**Behavior:**
- Second Deploy posts UpdateService with new spec.
- Cloud Run handles concurrent traffic-shifts via its own revision
  management. The new revision queues; old revision finishes
  propagating; new revision rolls.
- waitForServiceReady on second Deploy sees Ready=SUCCEEDED when new
  revision is stable.
- ✓ Convergent.

**Hazard:** A "stuck" period where two revisions are simultaneously
in flight provider-side. Cloud Run handles this via traffic management.
We trust the provider.

**Reconstruction quality:** Good. History shows two
`updated in_progress`/`updated success` pairs.

---

## Incident I-7: Delete replay collides with provider lag

**Scenario:** Operator sets endDate. markCloudTargetForDestroy flips
status → deleting. Dispatcher destroys at T+30s. Target → deleted at
T+31s. Cloud Run's actual deletion completes at T+90s.

**Operator action at T+45s:** Hits DELETE on rollout.
HandleRolloutDelete sees target.status=deleted, allows the cascade.
Rollout + targets + history removed.

**Provider state at T+45s:** Service still listable.

**Operator state at T+90s:** Service NOT_FOUND. Aligned with AutoStack.

**Truth gap window:** 45-89s. During this window, AutoStack reports
"deleted" but service exists.

**Severity:** Low. Service is in deletion-pending; not billable, not
servable.

**Mitigation:** Phase 2.5 post-destroy NOT_FOUND poll.

**Reconstruction quality:** N/A — history is cascade-deleted with the
rollout. Documented limitation.

---

## Incident I-8: Stale-spec replay after restart

**Scenario:** Deploy in flight at T+5min. Operator respecs rollout at
T+6min. Process crash at T+7min. Restart at T+8min.

**Behavior:**
- Sweep marks op failed: abandoned. Target → error.
- Operator must manually clear error. **Or** Phase 2.5's pending-destroy
  pattern could be extended to "pending-respec" to retain the intent.
  Currently no such mechanism.

**Today's recovery:** Operator sets endDate on rollout (ends it),
creates a new rollout with the desired spec. New target row, new
dispatch. Convergent but operator-heavy.

**Reconstruction quality:** Adequate. Logs show panic OR sweep activity.

---

## Incident I-9: Cloud Run revision propagation delay observed by polling

**Scenario:** Cloud Run accepts UpdateService instantly (returns 200)
but traffic shift takes 45s.

**Behavior:**
- `waitForServiceReady` polls every 5s. May observe Ready=SUCCEEDED on
  the new revision after ~50s.
- During the wait, GetStatus from a separate poll cycle would also
  observe the in-flight state — but the dispatcher's in-flight guard
  prevents that path (reconciler skips when current_operation is set).
- Dispatcher returns success after revision stabilizes.
- Target → updating → running.

**Reconstruction quality:** Excellent.

---

## Incident I-10: Duplicate reconcile cycle after timeout

**Scenario:** Ticker fires at T+0s. Cycle takes 35s due to slow DB
queries. Ticker fires again at T+30s while cycle is mid-flight.

**Behavior:**
- Go's ticker queues at most one event in the channel. So a 35s cycle
  with a 30s ticker means the second tick is queued and fires
  immediately when the first cycle returns. Result: back-to-back
  cycles, no concurrency.
- Within either cycle, per-target ordering is deterministic; same
  targets processed.
- For in-flight ops, the second cycle's in-flight guard correctly
  skips. ✓

**Hazard:** Cycles can drift later than the configured 30s interval if
DB is slow.

**Reconstruction quality:** Good — every cycle has its own cycle_id.

---

## Summary

| Scenario | Today | After Phase 2.3 |
|---|---|---|
| I-1 partial deploy crash | sweep abandons → error | same |
| I-2 mid-destroy crash | sweep abandons → error → operator-manual recovery | same |
| I-3 stale provider readiness | updating-not-running latch holds | same |
| I-4 sweep reclaims live op | wrong — abandons live op | **fixed** by heartbeat-aware sweep |
| I-5 rollback interrupted | N/A | N/A |
| I-6 deploy retry vs prop delay | convergent via Cloud Run revision mgmt | same |
| I-7 delete vs prop delay | brief lie window | same; Phase 2.5 fix designed |
| I-8 stale-spec replay | operator-heavy recovery | same |
| I-9 revision propagation | waitForServiceReady handles | same |
| I-10 duplicate ticker | Go ticker queueing prevents | same |

**Phase 2.3 materially improves I-4 (rolling restart safety).** Other
scenarios are either already adequately handled or have designs
deferred to Phase 2.5.

## Operator-facing checklist for incident debugging

1. **Find the affected target:** `SELECT * FROM deployment_targets
   WHERE id = ...`. Read `status`, `current_operation`,
   `last_state_change_at`, `drift_summary`.
2. **Find the most recent op:** `SELECT * FROM operations WHERE
   target = ... ORDER BY started_at DESC LIMIT 5`. Read `status`,
   `message`, `rollout_revision`.
3. **Reconstruct timeline:** `SELECT * FROM deployment_history WHERE
   target = ... ORDER BY created`. Read `action`, `status`, `message`,
   `from_revision`, `to_revision`.
4. **Find the cycle that did the work:** grep logs for the most recent
   `[DISPATCH_CLAIM]` against this target, note `cycle=<id>`, then
   `grep "cycle=<id>"` for the full cycle activity.
5. **Cross-check provider:** GCP console / gcloud / Cloud Run API call.

If any of steps 1-4 leaves you guessing, file a Phase 2.3-tracked
observability gap.

## Related
- [[lineage-integrity-review]]
- [[observability-integrity]]
- [[../reconciler/deploy-dispatch-design]]
