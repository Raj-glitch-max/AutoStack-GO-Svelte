# Observability Integrity Assessment — Phase 2.3

## Last Updated
2026-05-14

## The observability bar

At 3 AM, an operator must be able to answer:

1. **Who owns the current operation on target X?**
2. **Why did deploy fail?**
3. **Why is reconciliation paused on target X?**
4. **Why was a replay triggered?**
5. **Why did the sweep reclaim ownership?**
6. **Why did deployment truth change?**

…using `deployment_targets`, `operations`, `deployment_history`, and
the structured log output. No source-code reading.

## Current observability surface

### Structured log emissions

Tagged emissions (grep-discoverable):

| Tag | Source | Fields |
|---|---|---|
| `[RECONCILE]` | cloud.go reconcileAll | cycle_id, cycle_start/cycle_complete, target_count, succeeded, failed, duration_ms |
| `[RECONCILE_TARGET]` | cloud.go reconcileOne | cycle, target, provider, external_id, status |
| `[RECONCILE_TARGET_COMPLETE]` | cloud.go reconcileOne | target, status |
| `[RECONCILE_SKIP]` | cloud.go reconcileOne | cycle, target, reason |
| `[DISPATCH_IN_FLIGHT]` | cloud.go reconcileOne | cycle, target, operation |
| `[STATE_TRANSITION]` | cloud.go reconcileOne | cycle, target, from, to |
| `[STATUS_UNKNOWN]` | cloud.go updateTargetStatus | target, previous, message |
| `[TRANSITION_REFUSED]` | cloud.go updateTargetStatus | target, from, to, reason |
| `[FAILURE]` | cloud.go reconcileOne | cycle, target, category, message |
| `[SUSPICION_HOLD]` | cloud.go reconcileOne | cycle, target, previous, reason |
| `[CRED_DECRYPT_FAIL]` | cloud.go reconcileOne | target, error |
| `[TARGET_CONFIG_PARSE_ERR]` | cloud.go reconcileOne | target, raw, err |
| `[PANIC]` | cloud.go reconcileAll/reconcileOne | target, panic_value |
| `[DISPATCH_CLAIM]` | dispatch.go dispatchDeploy/dispatchDestroy | target, operation, rollout_revision, action, kind |
| `[DISPATCH_SPEC_ERR]` | dispatch.go dispatchDeploy | target, error |
| `[DISPATCH_OP_CREATE_ERR]` | dispatch.go | target, error |
| `[DISPATCH_CLAIM_ERR]` | dispatch.go | target, error |
| `[DISPATCH_CLAIM_SKIP]` | dispatch.go | target, reason |
| `[DISPATCH_PANIC]` | dispatch.go | target, op, panic, [kind=destroy] |
| `[DEPLOY_START]` | dispatch.go dispatchDeploy | target, operation, image, region |
| `[DEPLOY_END]` | dispatch.go dispatchDeploy | target, operation, duration_ms, err |
| `[DEPLOY_STALE]` | dispatch.go dispatchDeploy | target, operation |
| `[RELEASE_ERR]` | dispatch.go releaseTargetWithExternal | target, op, err |
| `[RELEASE_LOST_OWNERSHIP]` | dispatch.go releaseTargetWithExternal | target, op |
| `[HEARTBEAT_FAIL]` | dispatch.go heartbeat | op, err |
| `[OP_COMPLETE_ERR]` | dispatch.go completeOperation | op, err |
| `[OP_COMPLETE_NOOP]` | dispatch.go completeOperation | op, status |
| `[HISTORY_WRITE]` | dispatch.go writeHistory | target, action, status |
| `[HISTORY_WRITE_ERR]` | dispatch.go writeHistory | error |
| `[STARTUP_SWEEP]` | sweep.go SweepAbandonedOperations | count |
| `[STARTUP_SWEEP_ERR]` | sweep.go | error |
| `[STARTUP_SWEEP_HISTORY_ERR]` | sweep.go writeAbandonHistory | error |
| `[OP_ABANDONED]` | sweep.go SweepAbandonedOperations | operation, target, kind |
| `[CLOUD_TARGET_CREATED]` | rollouts.go createPendingDeploymentTarget | rollout, target, provider, region |
| `[CLOUD_DESTROY_DEFER]` | rollouts.go markCloudTargetForDestroy | target, reason |
| `[CLOUD_DESTROY_MARKED]` | rollouts.go HandleRolloutUpdate | rollout |
| `[CLOUD_DESTROY_MARK_FAIL]` | rollouts.go | rollout, err |
| `[CLOUD_UPDATE_DEFERRED]` | rollouts.go HandleRolloutUpdate | rollout, reason |
| `[CLOUD_UPDATE_NOOP]` | rollouts.go HandleRolloutUpdate | rollout, reason |
| `[CLOUD_RESPEC_FAIL]` | rollouts.go | rollout, err |
| `[CLOUD_RESPEC_DEFER]` | rollouts.go flipCloudTargetsToPendingOnRespec | target, reason |
| `[CLOUD_RESPEC_REDISPATCH]` | rollouts.go HandleRolloutUpdate | rollout |
| `[CLOUD_DELETE_REFUSED]` | rollouts.go HandleRolloutDelete | rollout, target, status, reason |
| `[CLOUD_DELETE_ALLOWED]` | rollouts.go HandleRolloutDelete | rollout, reason |
| `[ENCRYPTION_NOT_CONFIGURED]` | main.go | error |
| `[ENCRYPTION_READY]` | main.go | key_fp |

This is a healthy tag inventory. Each tag answers a specific operational
question.

## Issues

### O-1: `cycle_id` does not propagate into dispatch logs

**Severity:** Medium. **Impact:** Cannot correlate a `[DEPLOY_START]`
to its triggering `[RECONCILE]` cycle without timestamp inference.

**Fix:** Plumb cycle_id through `dispatchDeploy`/`dispatchDestroy` and
attach to every dispatch tag. **Landing in Phase 2.3.**

### O-2: `cycle_id` not persisted on `operations`

**Severity:** Low. **Impact:** Post-mortem reconstruction must
correlate log cycle_id to operation timestamps; the operation row
itself doesn't carry cycle.

**Fix:** Add `operations.cycle_id` text column, stamped at op creation.
**Landing in Phase 2.3** (no schema migration needed if added as a
non-required text via the operations migration extension; actually we
DO need a migration since the column doesn't exist. Defer to Phase 2.5
to avoid migration churn this phase — log-only correlation is enough.)

**Updated decision:** Defer the column add to Phase 2.5. The log
propagation alone (O-1) is enough operational improvement this phase.

### O-3: No "operation lifetime" panel

**Severity:** Low. **Impact:** Operators must query DB to see how long
an operation has been in flight.

**Fix:** Phase 2.5 — Prometheus-style `autostack_operation_age_seconds`
gauge or PocketBase view. Deferred.

### O-4: `[SUSPICION_HOLD]` at WARN level, no structured fields

**Severity:** Low. **Impact:** Hard to alert on suspicion-hold
patterns in production.

**Fix:** Phase 2.5 — adopt `log/slog` for structured logging across the
reconciler. Deferred (large refactor).

### O-5: `[HISTORY_WRITE]` always emits regardless of value

**Severity:** Negligible. Just log noise. Could be moved to debug
level. Defer.

### O-6: `[OP_COMPLETE_NOOP]` doesn't distinguish "sweep took it" from
"dispatcher self-double-completed"

**Severity:** Low. Both signal the same underlying state ("op already
terminal"); the cause matters for debugging.

**Fix:** Phase 2.5 — separate tag for sweep-conflict
(`[OP_COMPLETE_SWEEP_CONFLICT]`) vs self-double-complete
(`[OP_COMPLETE_REENTRY]`). Negligible operational benefit; deferred.

### O-7: No reconciler-internal metrics

**Severity:** Medium for production. **Impact:** No
`autostack_target_status` gauge, no `autostack_dispatch_duration`
histogram, no `autostack_circuit_open` gauge.

**Fix:** Phase 2.5 — emit log lines parseable by Prom tailers, or
adopt OpenTelemetry/Prom client. Deferred.

### O-8: Cycle-level errors don't surface backoff state

**Severity:** Low. `reconcileWithBackoff` silently skips if in backoff.
A log line "skipped cycle due to backoff (X seconds remaining)" would
clarify.

**Fix:** Phase 2.5 — emit `[CYCLE_BACKED_OFF]` at debug. Trivial.

### O-9: `[HEARTBEAT_FAIL]` doesn't escalate

**Severity:** Low. A persistent DB-busy state would log heartbeat fails
every 60s. If the heartbeat-aware sweep is shipped (Phase 2.3), this
becomes operationally relevant: a heartbeat-failing op may be
incorrectly classified as abandoned.

**Fix:** Phase 2.5 — track consecutive failures; escalate to
`[HEARTBEAT_FAIL_PERSISTENT]` at log line 5+. Trivial.

## Phase 2.3 implementation in this area

- O-1: cycle_id into dispatch logs. Plumbed.
- O-2: deferred — column add postponed.

## Deferred

- O-3, O-4, O-6, O-7, O-8, O-9: all Phase 2.5 with clear designs.

## Related
- [[incident-reconstruction-assessment]]
- [[../reconciler/dispatcher-reconciler-interaction]]
