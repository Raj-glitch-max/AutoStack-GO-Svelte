# Phase 2 Finalization — Lifecycle Closure Assessment

**Last Updated:** 2026-05-14

## Terminal Lifecycle Paths

### 1. Deploy → success

```
pending → claim → creating → [Provider.Deploy]
  → waitForServiceReady (poll GetService until Ready=SUCCEEDED)
  → dispatchDeploy success branch
  → completeOperation(succeeded)
  → writeHistory(success)
  → releaseTargetWithExternal(creating, updating, externalID)
  → next cycle: GetStatus(running) → updateTargetStatus(updating→running)
```

- **Ownership released:** Yes — `releaseTargetWithExternal` clears `current_operation`.
- **Lineage written:** Yes — `writeHistory` with `action=created|updated, status=success`.
- **Observability emitted:** `[DEPLOY_START]`, `[DEPLOY_END]`, `[HISTORY_WRITE]`.
- **Terminal state:** `updating` → next poll → `running`.

### 2. Deploy → hard error

```
pending → claim → creating → [Provider.Deploy returns error]
  → completeOperation(failed)
  → writeHistory(failed)
  → releaseTarget(creating, error)
  → recordTargetFailure (circuit +1)
```

- **Ownership released:** Yes.
- **Lineage written:** Yes, `status=failed`.
- **Terminal state:** `error`, circuit holds. Operator must respec to clear.

### 3. Deploy → provider reports error (result.Status="error")

Same as hard error: `result.Status=error` goes to the same branch as a hard error.
Releases to `error`.

### 4. Deploy → stale (rollout moved during call)

```
pending → creating → [Deploy succeeds, RolloutMovedSince=true]
  → noteStaleSucceeded++
  → if staleCount < 3: releaseTarget(creating, pending)
  → if staleCount ≥ 3: releaseTargetWithExternal(creating, error, "stale loop")
```

- **Ownership released:** Yes.
- **Lineage written:** Yes, `status=failed, message=stale spec`.
- **Terminal if loop:** After 3rd stale, `error` with explicit loop message.

### 5. Destroy → success

```
deleting → claim → [Provider.Destroy]
  → DeleteService + confirmDeleted (NOT_FOUND poll)
  → completeOperation(succeeded)
  → writeHistory(success)
  → releaseTarget(deleting, deleted)
```

- **Ownership released:** Yes.
- **Lineage written:** Yes, `action=deleted, status=success`.
- **Terminal state:** `deleted`. Poll-skip guard (`previousStatus == "deleted"`) prevents re-entry.

### 6. Destroy → hard error

```
deleting → claim → [Provider.Destroy returns error]
  → completeOperation(failed)
  → writeHistory(failed)
  → releaseTarget(deleting, error)
  → recordTargetFailure
```

- **Ownership released:** Yes.
- **Lineage written:** Yes, `status=failed`.
- **Terminal state:** `error`. `pending_destroy` flag still set on error target.

### 7. Crash mid-deploy (sweep reclaims)

```
in_progress → [heartbeat active]
  → SweepAbandonedOperations finds op (never heartbeated or stale)
  → op.status = failed
  → writeAbandonHistory
  → target.current_operation cleared, status=error
```

- **Ownership claimed by sweep.** Dispatcher goroutine is dead.
- **Lineage:** Sweep writes an `error` history row.
- **Terminal state:** `error`. Dispatcher's own lineage is lost (never wrote success).

### 8. Crash mid-destroy (confirmDeleted crash)

```
[Provider.Destroy running → DeleteService done, confirmDeleted polling]
  → process crashes in confirm loop
  → SweepAbandonedOperations: op has `updated_at == started_at` (no heartbeats fired)
    OR: heartbeat was active and op is preserved as live
  → if never heartbeated: op marked failed
  → if heartbeated: op preserved as live
  → target has current_operation = opId (not cleared, since op terminal)
  → On next tick: shouldDispatchDestroy checks status=deleting AND current_operation=''
    → current_operation != '', skip dispatch
    → poll skip: currentOp != '', skip poll
    → HONESTY GUARD: previousStatus == 'deleted', skip poll
    → result: NO ACTION. Target stuck at deleting.
```

**This is a closure gap.** The `deleting` target is stuck. The only recovery is operator clearing `current_operation` to `''` so the next cycle redispatches.

**Phase 2.9 fix identified:** `confirmDeleted` runs inside the `DeployTimeout` context, and the heartbeat goroutine is scoped to the same context. If the process dies inside `confirmDeleted`, the heartbeat dies with it. The 15-min `DeployTimeout` gives headroom, but if the process crashes before `DeployTimeout` fires (or inside the 60s confirm window), the sweep treats the op as abandoned.

**Fix:** Move `confirmDeleted` to run as a detached goroutine that survives dispatch return, or extend the heartbeat scope to span confirmDeleted with a separate context. This is a narrow, actionable Phase 2.9 fix.

---

## Interruptable vs Atomic Paths

| Path | Atomic? | Interrupt Result |
|---|---|---|
| Deploy spec parse | Atomic (fails before claim) | No claim taken |
| Create operation row | Atomic | If fails before claim: no row, no claim |
| CAS claim | Atomic (DB-level) | If lost: cancel op, no dispatch |
| Provider.Deploy | Not atomic | On crash: sweep reclaims |
| waitForServiceReady | Not atomic | On crash: sweep reclaims |
| confirmDeleted | Not atomic | On crash: stuck deleting (gap above) |
| releaseTarget | Not atomic | On crash: target may be inconsistent with op state |

---

## Verdict

**All terminal paths release ownership and write lineage.** The closure gap is `confirmDeleted` crash leaving a `deleting` target with a terminal op and no path to re-dispatch. This is a Phase 2.9 fix (see [[phase2.9/replay-determinism-assessment]]).

The `running + pending_destroy` gap (from [[phase2.9/reconciliation-convergence-assessment]] C-1) is also a closure gap: destroy intent is never consumed when the target is in `running` state.

**Structural vs temporary:** Both are temporary — discrete goroutine and state-transition fixes, not architectural redesign.

---

## Related
- [[phase2.9/reconciliation-convergence-assessment]] — C-1 closure gap
- [[phase2.9/replay-determinism-assessment]] — confirmDeleted crash gap
- [[deploy-dispatch-design]]