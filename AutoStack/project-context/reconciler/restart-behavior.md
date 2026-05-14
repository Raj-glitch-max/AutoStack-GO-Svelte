# Restart and Crash Recovery — What Survives, What Doesn't

## Last Updated
2026-05-13 (Phase 1.9 principal review)

## Single-process invariants

The reconciler runs in-process with the PocketBase server. A process
restart loses:

- `Reconciler.failures` (the circuit breaker map)
- `Reconciler.lastErrorTime` (the backoff timer)
- `Reconciler.started` (process-local "already started" flag)
- Any in-flight ctx-bound calls

What survives across restart:

- PocketBase rows in `deployment_targets`, `rollouts`, `cloud_accounts`.
- The Cloud Run service itself, as it exists in GCP.

## Mid-operation crash analysis

### Mid-Deploy crash
Impossible to test in production today — no code calls Deploy. When the
Deploy dispatch path is added, the following must be true to make this
safe:

- An `operations` collection must exist and be written BEFORE the call
  to the provider, so on restart the reconciler can see "this target
  has an in-flight Deploy" and decide whether to wait, resume, or roll
  forward.
- `Deploy` must be idempotent. Calling Deploy twice with the same
  `RolloutID` should converge, not create two services. The Cloud Run
  provider currently relies on `GetService` returning existing → enter
  Update path, which is convergent but races with concurrent callers.

### Mid-Destroy crash
`Destroy` is idempotent on `NOT_FOUND` (Phase 1.9 also fixed the
trivially-true UID check). Restart re-polls; provider eventually reports
the service gone; reconciler attempts to write `deleted` status (which
the transition guard refuses if previous was `deleted`, otherwise
allows).

### Mid-GetStatus crash
Restart re-polls. No harm — `GetStatus` is read-only.

## Multi-process / HA analysis

`Reconciler.started` is process-local. Two pods running this binary
both start reconcilers. Both query the same rows, both call `GetStatus`
against the same provider services, both write `deployment_targets`.

- **With SQLite (current default):** SQLite's WAL-mode write lock
  serializes writes. Reads and last-writer-wins semantics make this
  mostly survivable, but not deterministic — operators may see status
  flap depending on which pod's poll won.
- **With Postgres (a documented future path):** No serialization. Two
  pods will race writes. Lost updates are possible.

Phase 2 must add either:
- Leader election (one reconciler per cluster), OR
- Per-target row versioning (optimistic concurrency on update).

## Panic survival (Phase 1.9)

- A panic inside `reconcileOne`:
  - Logs `[PANIC] reconcileOne target=…`.
  - Increments only the panicking target's failure count (was: cleared
    the entire map).
  - Calls `recordError()` so the next cycle applies backoff.
  - Does NOT write a status — writing `error` without a verified previous
    status would bypass the transition guard.
- A panic inside `reconcileAll` (above target loop):
  - Logs `[PANIC] reconcileAll`.
  - Does NOT clear failures.
  - Process continues. Next tick will retry.

## What is NOT survivable

- Any in-flight Cloud Run Deploy (no operations collection).
- Any in-flight Rollback (refused entirely today).
- Any orphaned Cloud Run service whose `deployment_targets` row was
  deleted before `Destroy` succeeded (no orphan scan exists).

## Related
- [[reconciliation-guarantees]]
- [[lifecycle-assumptions]]
- [[dangerous-edge-cases]]
- [[deferred-operational-hardening]]
