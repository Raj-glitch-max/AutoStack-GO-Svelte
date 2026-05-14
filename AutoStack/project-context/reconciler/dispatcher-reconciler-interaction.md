# Dispatcher / Reconciler Interaction — Phase 2.1

## Last Updated
2026-05-14

## The hazard Phase 2.1 closes

In Phase 2.0 the reconciler's status-poll path did NOT check
`deployment_targets.current_operation` before calling `Provider.GetStatus`.
During a long (multi-minute) `Provider.Deploy` call:

1. The reconciler ticker fires every 30s.
2. Each tick, the dispatcher's target appears in the SELECT result with
   `current_operation = <opID>`.
3. `shouldDispatchDeploy` returns false (lock held). `shouldDispatchDestroy`
   returns false too.
4. **Execution falls through to GetStatus.** The Cloud Run service may
   not exist yet (NOT_FOUND) or may be in `Ready=RECONCILING`.
5. The error path runs `updateTargetStatus(targetID, previousStatus,
   "error", ...)`. previousStatus is `creating` (set by the CAS claim).
   The transition guard ALLOWS `creating → error`.
6. Persisted target status flaps to `error` while the dispatcher is
   still mid-deploy.
7. Circuit breaker increments; after ~5 ticks, the target is skipped
   from polling (good) but the lie was already on disk.
8. The dispatcher eventually returns and writes `updating`. Operators
   see `creating → error → updating` per deploy.

This was the central truthfulness gap of Phase 2.0.

## The fix

In `reconcileOne`, BEFORE any dispatch or poll branch:

```go
if currentOp != "" {
    log.Printf("[DISPATCH_IN_FLIGHT] cycle=%s target=%s operation=%s", ...)
    return reconcileSkipped
}
```

Plus a second guard:

```go
if previousStatus == "deleted" {
    log.Printf("[RECONCILE_SKIP] cycle=%s target=%s reason=terminal_deleted", ...)
    return reconcileSkipped
}
```

The `deleted`-skip closes a related noise leak: a target whose service
has been destroyed used to keep getting GetStatus calls forever,
NOT_FOUND failures, and circuit-breaker churn until the circuit opened.

## Ownership semantics post-Phase 2.1

| Target state | current_operation | Who is authoritative |
|---|---|---|
| pending | NULL | Reconciler dispatcher (next tick CAS-claims) |
| pending | set | Dispatcher mid-flight (claim succeeded; release pending) |
| creating | set | Dispatcher mid-flight |
| creating | NULL | 🟠 anomalous — sweep cleaned up but didn't reset to error/pending |
| updating | NULL | Reconciler status-poll (promotes to running on next observation) |
| updating | set | 🟠 anomalous — release should have cleared |
| running | NULL | Reconciler status-poll (steady-state observation) |
| running | set | 🟠 anomalous |
| error | NULL | Operator action required |
| deleting | NULL | Reconciler dispatcher (next tick CAS-claims destroy) |
| deleting | set | Destroy dispatcher mid-flight |
| deleted | NULL | Nobody (skip-reconcile) |

The 🟠 anomalous rows shouldn't occur with the current code. If
observed, the release-CAS-lost-ownership log (`[RELEASE_LOST_OWNERSHIP]`)
provides a direct hint: another actor (sweep or external write) cleared
current_operation while we were running.

## Per-cycle correlation

Phase 2.1 adds a `cycle_id` random 8-char hex value generated in
`reconcileAll` and threaded into every per-target log emission via the
`__cycle_id` sentinel key on the row map. Operators can grep `cycle=abcd1234`
to see all activity from one cycle.

Future work (Phase 2.5): adopt `log/slog` so `cycle_id` becomes a real
field rather than a substring.

## Related
- [[operation-ownership]]
- [[deploy-dispatch-design]]
- [[sweep-and-heartbeat-semantics]]
- [[eventual-consistency-stress]]
