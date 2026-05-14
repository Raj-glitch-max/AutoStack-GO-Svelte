# Chaos Scenarios Catalog — Phase 2.6

## Last Updated
2026-05-14

## Premise

Real operational chaos: provider inconsistency, delayed readiness,
partial deploy convergence, retry storms, stale reads, interrupted
deletes, interrupted rollbacks, restart-during-active-deploy,
crash-before-heartbeat, ownership-reclaim races.

Phase 2.3 walked 10 incident scenarios. Phase 2.6 adds chaos-specific
ones not yet covered:

## CS-1: Restart under sustained deployment load

**Setup:** 20 cloud rollouts, all simultaneously being deployed
following a respec wave. AutoStack pod receives SIGTERM during minute
6 of a 10-min deploy storm.

**Behavior:**
- 20 in-progress ops, all heartbeating.
- SIGTERM → Go runtime starts cancellation. PocketBase's graceful
  shutdown timing depends on its lifecycle hooks.
- Most goroutines exit when their ctx cancels.
- Process exits after timeout (default ~10s for PocketBase serve loop).

**Post-restart:**
- Startup sweep runs. Heartbeat-aware policy:
  - Ops heartbeated within last 2 min → preserved.
  - Ops older heartbeat → abandoned.
- For ops that completed Deploy provider-side but were SIGKILL'd before
  the dispatcher's `releaseTarget` ran: op stays in_progress, sweep
  abandons. Target → error. Cloud Run service exists. **Orphan-class
  state.** Operator must investigate provider-side.

**Severity:** Medium. The "deploy succeeded but AutoStack reports
error" lie is the worst case.

**Mitigation today:** Phase 2.3 heartbeat-aware sweep preserves ops
within the heartbeat window, giving them a chance to finish writing
release on graceful shutdown. Phase 2.8 graceful-shutdown work will
extend this further.

**Phase 2.6 fix:** None. Phase 2.8 work covers graceful shutdown.

## CS-2: Replay under provider lag

**Setup:** Deploy in flight. Cloud Run's API responds normally but
revision propagation is stalled (Cloud Run internal incident). Deploy
takes 18 min, past DeployTimeout.

**Behavior:**
- DeployTimeout fires at 15min.
- waitForServiceReady returns timeout.
- Deploy returns (result, nil) with status="timeout" — goes to
  `result.Status == "error"` branch in dispatcher.
- Target → error.
- Operator clears (respec). Next dispatch tries again.
- Cloud Run's prior queued revision may have finally landed; the new
  Update overwrites with a fresh revision.

**Convergence:** Eventually converges to running once Cloud Run stabilizes.

**Severity:** Low. Bounded behavior.

## CS-3: Stale-read reconciliation

**Setup:** SQLite WAL on a busy disk. Reconciler's SELECT returns a
snapshot 100ms behind a controller's just-completed write.

**Behavior:**
- Reconciler observes old status; CAS predicates use snapshot value.
- CAS UPDATE acquires write lock, sees current value.
- If current value matches predicate → succeeds. Else → 0 rows.

SQLite WAL doesn't guarantee read-after-write within a single
connection — but PocketBase typically uses one connection per
request. Cross-goroutine reads can lag.

**Severity:** Low. CAS predicates make stale reads safe.

## CS-4: Delayed revision propagation observed mid-poll

**Setup:** Cloud Run's `Service.Conditions` lag the actual revision
state by 30s. GetStatus returns RECONCILING while the revision is
actually serving traffic.

**Behavior:**
- Status-poll observes `creating` (from RECONCILING).
- Transition guard refuses `updating → creating`.
- Status stays `updating`.
- Next poll: SUCCEEDED → `running`. ✓

## CS-5: Provider timeout loop

**Setup:** Cloud Run API consistently times out at 30s for one specific
region. GetStatus times out. ClassifyError → timeout. Backoff applies.

**Behavior:**
- Per-target failure count increments.
- Circuit opens at 5.
- Cycle-level backoff applies (since timeout is not auth/quota).
- Target skipped until operator action.

**Convergence:** Bounded. ✓

## CS-6: Partial deployment success across regions

**Setup:** Not applicable today — AutoStack targets are single-region.
A multi-target rollout (different regions) would have independent
target rows; each succeeds/fails independently.

## CS-7: Interrupted rollback

**N/A.** Rollback not implemented.

## CS-8: Interrupted delete

**Setup:** Destroy in flight. Process SIGKILL after `DeleteService`
returns 200 but before `releaseTarget` runs.

**Behavior:**
- Operation in_progress, target current_operation set, status=deleting.
- Restart: sweep heartbeat-aware. Heartbeat was running (60s tick); op
  may or may not be within liveness window depending on timing.
- If within window: op preserved (which is wrong — process is gone).
  Stuck-state. **Phase 2.6 runtime sweep closes this.**
- If outside window: op abandoned, target → error.

Provider side: service is in deletion-pending, eventually NOT_FOUND.

**Operator recovery:**
- Investigate; observe NOT_FOUND in GCP.
- Manually flip status from `error` to `deleted` via PocketBase admin.
- OR set `pending_destroy=true` manually + flip status to deleting →
  next dispatch is no-op since GetService returns NOT_FOUND (Cloud Run
  provider is idempotent on NOT_FOUND).

**Phase 2.6 runtime sweep handles the stuck-window case.**

## CS-9: Crash before heartbeat

Same as Phase 2.3 OS-1 (crash before first heartbeat). First-heartbeat
guard handles it. ✓

## CS-10: Replay after cleanup

**Setup:** A target's operations and history rows are aged out by
Phase 2.5 cleanup. Subsequent incident requires reconstructing the
deploy timeline.

**Behavior:**
- Forensic data within retention windows is preserved.
- Older data is gone.
- Operator can answer "what happened in the last 90 days" but not
  "what happened a year ago".

**Severity:** By design. Documented in retention-policy.md.

## CS-11: Concurrent reconciliation cycles

Same as Phase 2.3 I-10. Go's ticker queues at most one tick. ✓

## CS-12: Ownership reclaim race

**Setup:** Dispatcher in pod A heartbeats. Pod B's runtime sweep
(Phase 2.6) checks ages. Pod A's heartbeat is fresh; sweep skips.
Pod A then crashes. Heartbeat stops. Pod B's NEXT runtime sweep (5
min later) sees stale heartbeat → abandons.

**Timeline:**
- T+0: pod A crash.
- T+0–60s: heartbeat would have ticked; no tick.
- T+120s: heartbeat liveness window expires.
- T+5min: pod B's next runtime sweep tick. Op `updated_at` > 5 min
  old. Abandons.

**Latency:** Up to 5 min before recovery starts.

**Severity:** Low. 5 min is acceptable for an automated recovery.

## CS-13: Retry amplification storm

Same as Phase 2.4 A-1/A-2/A-4. Phase 2.6 succeeded_stale guard handles
A-4. A-1/A-2 deferred to Phase 2.7 per-account work.

## Phase 2.6 implementation closes

- CS-8 stuck-window: runtime sweep.
- CS-13 succeeded_stale loop: stale-count guard.

## Related
- [[succeeded-stale-guard]]
- [[runtime-sweep-design]]
- [[../phase2.3/incident-reconstruction-assessment]]
