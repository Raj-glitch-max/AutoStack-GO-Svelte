# Operation Ownership Integrity Review — Phase 2.3

## Last Updated
2026-05-14

## The ownership invariant (recap)

> The goroutine that opens an operations row owns it for the row's
> entire lifetime, until that row reaches a terminal status.

Three writers can transition an operation row out of `in_progress`:

1. The owning dispatcher (`completeOperation` with CAS on `in_progress`).
2. The startup sweep (`SweepAbandonedOperations`).
3. Operator manual edit through PocketBase admin (out-of-band; not
   defended against programmatically).

Three writers can transition a `deployment_targets.current_operation`:

1. The owning dispatcher (`releaseTarget` with CAS on `current_operation`).
2. The startup sweep (clears `current_operation` only if it still points
   at the swept op).
3. The CAS claim (`claimTarget`) sets the field.

## Acquisition correctness

```sql
UPDATE deployment_targets
   SET current_operation = :op, last_state_change_at = :ts,
       status = CASE WHEN status = 'pending' THEN 'creating'
                    WHEN status = 'deleting' THEN 'deleting'
                    ELSE status END
 WHERE id = :id
   AND (current_operation = '' OR current_operation IS NULL)
   AND status IN ('pending', 'deleting')
```

- `current_operation` predicate handles both empty string and NULL.
  Defensive against any path that might write the empty string instead
  of NULL.
- `status IN ('pending', 'deleting')` makes the claim refuse a target
  that is in `running`, `updating`, `creating`, `error`, `deleted`,
  `stopped`. **This is the load-bearing safety check** against
  redispatching a target the dispatcher already owns.
- The `CASE` keeps status semantics correct: `pending → creating` on
  deploy claim; `deleting` stays `deleting` on destroy claim.

**Verdict:** Correct under SQLite WAL serialization. Correct under
Postgres `READ COMMITTED` because UPDATE...WHERE is row-level atomic.

## Renewal (heartbeat) correctness

```sql
UPDATE operations SET updated_at = :ts
 WHERE id = :id AND status = 'in_progress'
```

- The `status = 'in_progress'` predicate prevents the heartbeat from
  resurrecting a terminal op.
- 0 rows affected → heartbeat exits its goroutine cleanly.

**Verdict:** Correct.

## Release correctness

```sql
UPDATE deployment_targets SET ...
 WHERE id = :id AND current_operation = :op
```

- Conditional on `current_operation = :our_op` makes the release a
  no-op if the sweep (or operator) moved the row on.
- `[RELEASE_LOST_OWNERSHIP]` log emitted on 0 rows affected — minimum
  signal for incident reconstruction.

**Verdict:** Correct. The 0-rows-affected case leaks no audit row to
`deployment_history` — see [[lineage-integrity-review]] §L-3.

## Sweep correctness

```sql
SELECT id, target, kind FROM operations WHERE status = 'in_progress'
```

- All in-progress ops returned.
- Per-op: `op.status = failed`; `op.updated_at = now`;
  `op.message = "abandoned: process restart while in flight"`.
- `tgt.current_operation` cleared **only if it still points at us** —
  same defensive idiom as release. Prevents collision with a
  dispatcher that started post-sweep.

**Verdict:** Correct under single-pod restart. **Wrong** under multi-pod
because the sweep cannot tell a peer pod's live op from an abandoned
one. See O-1 below.

## Issues

### O-1: Sweep cannot distinguish live peer-pod ops

**Today:** Startup sweep marks ALL in-progress ops as abandoned. Multi-pod
boot of a second pod would corrupt the first pod's live ops.

**Severity:** High under multi-pod. Tolerable today (AutoStack ships
single-pod).

**Fix:** Add `operations.owned_by_pod` (string column) populated from
hostname or k8s downward API. Sweep refuses to touch ops owned by
peer-pod ids. Deferred — Phase 2.5.

### O-2: Sweep ignores fresh heartbeat

**Today:** Sweep marks every in-progress op as abandoned regardless of
`updated_at`. Heartbeat infrastructure exists (60s tick during deploy)
but the sweep doesn't read it.

**Severity:** Low for the single-pod hard-crash case (a dead process
can't heartbeat). **Medium** for the single-pod rolling-restart case
(SIGTERM → graceful shutdown → new pod starts before old pod fully
exits → new pod's sweep sees old pod's recent heartbeat).

**Fix:** Sweep ignores ops whose `updated_at` is within
`2 × heartbeatInterval` (2 minutes). Lives in Phase 2.3. **Landing
this phase.**

This is forward-compatible with O-1 — the eventual pod-identity check
will be added as a second AND-predicate on the same SELECT.

### O-3: `current_operation` ambiguity table from Phase 2.1 still has gaps

The dispatcher-reconciler-interaction doc identifies "anomalous" rows:

- `creating` with `current_operation = NULL` — sweep cleaned but didn't
  reset to error or pending.
- `updating` with `current_operation set` — release should have cleared.
- `running` with `current_operation set` — same anomaly.

**Today:** These are LOGGED via `[RELEASE_LOST_OWNERSHIP]` if they
occur due to a CAS conflict, but no auto-repair exists.

**Phase 2.5 work:** A passive scanner that detects and logs anomalies.
Not enough operational evidence yet to justify auto-repair.

### O-4: Operator manual edit in PocketBase admin

**Today:** The PocketBase admin UI grants write access to a logged-in
admin. An admin can:
- Set `current_operation` to an arbitrary value (claim a target).
- Flip a target's status to anything (bypass transition guard).
- Mark an in-progress op `succeeded` (lie).

**Defense:** None programmatically. Documented as "trusted role".

**Phase 2.5 / Phase 3 work:** Restrict admin write on these collections,
or surface a warning in the admin UI. Deferred.

### O-5: Heartbeat goroutine leaks if defer-cancel doesn't fire

**Today:** `dispatchDeploy` starts `go r.heartbeat(deployCtx, opID)`.
`deployCtx` is cancelled by:
1. The dispatcher's `defer cancel()` after the function returns.
2. `DeployTimeout` (15 min) firing.
3. Parent `ctx` cancellation (rare; parent is `reconcileOne`'s ctx,
   which has a 30s timeout for status-poll but a `DeployTimeout` for
   dispatch — both still bounded).

A panic before the defer is registered? Looking at code: `defer cancel()`
is registered on the line AFTER `deployCtx, cancel := ...`. So if
`buildDeploySpec` panics, no cancel is registered, but heartbeat
hasn't started yet either. ✓

If `createOperation` panics, same — heartbeat not started yet. ✓

If `claimTarget` panics, same. ✓

If `Provider.Deploy` panics, dispatcher's recovery defer fires; the
release/complete paths run; then function returns → `defer cancel()`
fires → heartbeat exits. ✓

**Verdict:** No leak path. ✓

### O-6: Heartbeat survives but op already terminal — wasted DB work

**Today:** Heartbeat does `UPDATE ... WHERE status = 'in_progress'`. If
the op was marked terminal between two heartbeat ticks (by sweep or by
dispatcher's `completeOperation`), the next heartbeat affects 0 rows
and the goroutine returns. One wasted UPDATE per such case. Negligible.

### O-7: Sweep race with a dispatcher whose op was claimed milliseconds ago

**Today:** Sweep runs ONCE under `startMu`, BEFORE the ticker. So at
sweep time, no dispatcher in THIS process is running. A pod restart
mid-deploy means the dispatcher in the previous process is gone.

**Verdict:** Single-pod, no race.

**Multi-pod:** See O-1.

## Phase 2.3 implementation in this area

- O-2: Heartbeat-aware sweep. Landing.

## Deferred

- O-1: Pod-identity stamping. Phase 2.5 — blocks multi-pod.
- O-3: Anomaly scanner. Phase 2.5+ if evidence accumulates.
- O-4: Admin restriction. Phase 3.

## Related
- [[replay-safety-assessment]]
- [[lro-survivability-review]]
- [[../reconciler/operation-ownership]]
- [[../reconciler/sweep-and-heartbeat-semantics]]
