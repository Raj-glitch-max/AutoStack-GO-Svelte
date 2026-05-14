# Sweep & Heartbeat Semantics — Phase 2.1

## Last Updated
2026-05-14

## What changed in Phase 2.1

Two related changes to operation-lifecycle safety:

1. **Startup sweep marks ALL `in_progress` operations as abandoned.**
   The Phase 2.0 age threshold (20 min) was wrong for the startup case:
   a process that died 5 minutes into a deploy would leave an op with
   `updated_at` only 5 min old. The next restart's sweep wouldn't touch
   it (age < threshold). The target would be permanently stuck with
   `current_operation` pointing at a stale op, the reconciler's
   `current_operation != ""` skip-guard refusing to act on it.

2. **Heartbeat sidecar refreshes `operations.updated_at` every 60s
   during a long deploy.** This is preparation for the future runtime
   sweep that distinguishes live ops (recent heartbeat) from abandoned
   ops (stale heartbeat) in a multi-pod world.

## Sweep policy (startup, current)

Implemented in `pkg/reconciler/sweep.go`. Called synchronously inside
`Reconciler.Start()` under `startMu`, BEFORE the ticker goroutine launches:

```
SELECT id, target, kind FROM operations WHERE status = 'in_progress'

For each row:
  - operations row → status='failed', message='abandoned: process restart while in flight'
  - deployment_history row → action=<created|updated|deleted|rolled_back|error>,
                             status='failed', message='abandoned: process restart'
  - If deployment_targets.current_operation == ourID:
      → current_operation='', status='error',
        last_state_change_at=now, last_synced=now,
        drift_summary='abandoned: process restart'
```

We NEVER infer success. The provider may have a partial or full
resource; only an operator can decide what to do.

## Heartbeat policy (runtime, current)

Implemented in `pkg/reconciler/dispatch.go`. Started by
`dispatchDeploy` and `dispatchDestroy` immediately after CAS claim:

```go
go r.heartbeat(deployCtx, opID)
```

The heartbeat goroutine ticks every 60s and runs:

```sql
UPDATE operations
   SET updated_at = NOW()
 WHERE id = :op AND status = 'in_progress'
```

If 0 rows are affected (op already terminal), the heartbeat exits
silently. If the UPDATE errors transiently (DB busy), the heartbeat
logs `[HEARTBEAT_FAIL]` and continues; one missed beat is not a hazard.

Heartbeat termination happens via `deployCtx.Done()`:
- `defer cancel()` in the dispatcher fires when dispatcher returns.
- `DeployTimeout` (15 min) fires if the provider call hangs.

## Why these changes are forward-compatible with multi-pod

The Phase 2.1 startup sweep is **safe under single-pod** but **unsafe
under multi-pod**: a restarting pod B would mis-classify pod A's live
ops as abandoned. The fix for multi-pod requires pod-identity stamping
on `operations` rows (`owned_by_pod: text`), so the sweep can refuse to
touch ops owned by a different pod-id. That work is deferred.

The heartbeat is the foundation: a future runtime sweep can use:

```
SELECT id FROM operations
 WHERE status = 'in_progress'
   AND updated_at < NOW() - INTERVAL '3 minutes'
```

A 3-minute liveness window vs. 60-second heartbeat gives 3× margin,
classifies a heartbeat-failing op as dead within minutes (vs. waiting
for the 15-min DeployTimeout).

## Hazards explicitly accepted in Phase 2.1

1. **Multi-pod startup sweep**: under multi-pod operation, pod B's
   startup would corrupt pod A's live state. Document; reject as
   architectural concern for Phase 3.0.
2. **Heartbeat failure during deploy**: if the DB is unavailable for >3
   minutes during a deploy, the future runtime sweep (when added) would
   mis-classify the op as abandoned. The dispatcher would still be
   running and would later collide with the sweep's release. Defense:
   the release-CAS guard from §3 below detects this and exits silently
   without overwriting.
3. **Sweep race with the dispatcher's defer-recovery panic path**: in
   single-pod, sweep only runs at startup, so no race exists in the
   current process lifetime. In a future runtime sweep, the heartbeat
   protects live dispatchers; the release-CAS protects against the
   edge case where a hung dispatcher recovers after being marked
   abandoned.

## Release-CAS guard (§3 reference)

`releaseTarget` now updates only if `current_operation = ourID`:

```sql
UPDATE deployment_targets
   SET current_operation = '', status = :new, last_state_change_at = NOW(), ...
 WHERE id = :target AND current_operation = :our_op
```

If `RowsAffected() == 0`, we log `[RELEASE_LOST_OWNERSHIP]` and exit.
The sweep (or a future external actor) already moved the row on; our
own outcome is now historical, not authoritative.

Same pattern applies to `completeOperation`:

```sql
UPDATE operations
   SET status = :terminal, updated_at = NOW(), message = :msg
 WHERE id = :op AND status = 'in_progress'
```

This prevents the dispatcher's success from flipping a sweep-marked
`failed` back to `succeeded`.

## Related
- [[deploy-dispatch-design]]
- [[operation-ownership]]
- [[dispatcher-reconciler-interaction]]
- [[restart-behavior]]
- [[deferred-operational-hardening]]
