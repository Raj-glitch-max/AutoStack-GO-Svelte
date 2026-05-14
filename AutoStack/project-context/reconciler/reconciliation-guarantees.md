# Reconciliation — What the System Actually Guarantees

## Last Updated
2026-05-13 (Phase 1.9 principal review)

## What the reconciler IS

A single-threaded, in-process, status-polling loop.

- Runs in the same process as the PocketBase server.
- Ticks every `Interval` (default 30s).
- Per cycle: queries `deployment_targets` joined with `rollouts` and
  `cloud_accounts`, then iterates rows sequentially.
- Per row: calls the provider's `GetStatus` and writes the result to
  `deployment_targets.status` if a transition guard permits.

## What the reconciler IS NOT

- **Not a deploy dispatcher.** Nothing in this loop or any other path
  calls `Provider.Deploy`. See [[lifecycle-assumptions]].
- **Not a rollback executor.** `Provider.Rollback` is refused with
  `ErrNotImplemented`.
- **Not a multi-worker scheduler.** Earlier `MaxConcurrency` config was
  dead and is removed in Phase 1.9.
- **Not HA-safe.** Two backend pods → two reconcilers writing to the
  same target rows. SQLite file-locking is currently the only mitigation.
  Any move to Postgres needs leader election or row-versioned writes.
- **Not a history writer.** Nothing writes to `deployment_history`.
- **Not an operation tracker.** No persisted operations collection
  exists; `Operation` is fictional today.

## Concurrency model

- Per-tick reconciliation runs in a single goroutine.
- `Reconciler.failures` is read/written under `failureMu`.
- `Reconciler.lastErrorTime` is read/written under `lastErrorMu`.
- `Reconciler.started` is guarded by `startMu`.

Phase 1.9 fixed a previous data race: `getFailureCount` iterated
`failures` while holding `lastErrorMu` (the wrong lock). The replacement
`backoffDuration` reads `failures` under `failureMu`.

## Determinism guarantees

Given identical desired state and identical provider state, the reconciler:

- Always calls the same `GetStatus` URL.
- Maps the same conditions to the same status string (Ready precedence
  over Configurations as of Phase 1.9).
- Applies the same transition guard. A regression-attempting observation
  is refused on cycle N, cycle N+1, and so on.

It does NOT guarantee identical timing — backoff state, circuit state,
and clock-wall timing vary between cycles.

## Circuit breaker

- Per-target failure count in `Reconciler.failures`.
- Threshold default: 5. When exceeded, target is skipped (logged
  `circuit open`).
- Reset to 0 on a successful `GetStatus`.
- **Auth and quota errors do NOT increment** the count. They require
  external intervention; retrying wastes API quota.

A panic in `reconcileOne` increments only the panicking target's count
(Phase 1.9 fix). It used to clear the whole map.

## Backoff

- Cycle-level only. Per-target backoff = circuit breaker.
- `lastErrorTime` is set when any of:
  - the top-level `deployment_targets` query fails,
  - any per-target `GetStatus` fails with a category other than
    `auth`/`quota`,
  - any per-target panic occurs.
- Backoff window = `BackoffBase * 2^maxFailureCount`, capped at
  `BackoffMax`.
- The window is read from `failures` under `failureMu` (Phase 1.9 fix).

## Failure visibility

Tagged log emissions:
- `[RECONCILE] cycle_start target_count=N`
- `[RECONCILE] cycle_complete target_count=N succeeded=S failed=F duration_ms=...`
- `[RECONCILE_TARGET] target=… provider=… external_id=…`
- `[RECONCILE_TARGET_COMPLETE] target=… status=…`
- `[STATE_TRANSITION] target=… from=… to=…`
- `[FAILURE] target=… category=… message=…`
- `[TRANSITION_REFUSED] target=… from=… to=… reason=…`
- `[STATUS_UNKNOWN] target=… previous=… message=…`
- `[PANIC] reconcileAll|reconcileOne target=…: …`

Missing today (see [[deferred-operational-hardening]]):
- Correlation IDs (`cycle_id`, per-attempt ID).
- Structured logger.
- Per-target last-success / last-attempt counters in the record.

## Related
- [[lifecycle-assumptions]]
- [[restart-behavior]]
- [[dangerous-edge-cases]]
- [[deferred-operational-hardening]]
