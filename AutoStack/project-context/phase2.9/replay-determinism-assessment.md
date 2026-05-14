# Phase 2 Finalization — Replay Determinism Assessment

**Last Updated:** 2026-05-14

## What This Document Is

Examines whether identical provider state + identical desired state always
converges toward identical operational truth under replay. This is the
foundation of trustworthy crash recovery.

---

## Determinism Guarantee

**Yes, for identical pairs.** Given:
- identical `deployment_targets` row (same status, external_id, rollout)
- identical provider response (same GetService conditions)
- identical desired state (same rollout manifest, rollout.updated timestamp)

The system will produce the same next state on every cycle.

---

## Why Determinism Holds

Three mechanical properties enforce this:

**1. Single-threaded per-cycle iteration.** `reconcileAll` processes rows sequentially in a single goroutine. No concurrent writes to the same target's state within one cycle.

**2. CAS-based dispatch exclusivity.** `claimTarget` atomically asserts `current_operation = '' AND status IN ('pending', 'deleting')` before a dispatcher runs. Two concurrent reconciler ticks cannot both dispatch the same target.

**3. Stateless status mapping.** `GetStatus` maps Cloud Run conditions → status strings via rules that have not changed since Phase 1.9:
- SUCCEEDED Ready → `running`
- FAILED Ready → `error`
- RECONCILING (no Ready) → `creating`
- No conditions → `unknown` (not persisted)

---

## Replay Scenarios

### Restart during active deploy

1. Process dies while `dispatchDeploy` is running with a live provider call.
2. On restart: `SweepAbandonedOperations` finds the op. If `updated_at == started_at` (never heartbeated) → marked `failed`. If `updated_at` is fresh (heartbeated at least once, within 2 min) → preserved as live.
3. Dispatcher goroutine is gone; provider call either continues to completion in GCP or is orphaned server-side.
4. On next tick: target is `creating` (released before crash) with `current_operation = opID` still set. Because `current_operation != ''`, the poll path skips and the CAS fails (op already terminal). `deleted` targets are skipped entirely.

**Result:** Deterministic. The sweep classifies the op. The target is either re-dispatched (if sweep preserved it as live) or seen as `creating`/`error` (if sweep marked failed). Re-dispatch requires operator action to clear error.

**Gap:** A target whose dispatcher completed but whose `releaseTarget` was interrupted mid-write will be `creating` with a terminal operation row. On restart, the sweep sees the op as `failed` (correct) and sets target → `error`. This is the honest state (we cannot know whether the 3-minute provider call succeeded or failed).

### Replay after provider lag (Cloud Run eventual consistency)

Cloud Run can report a service as `Ready=SUCCEEDED` in the API but still be propagating to all replicas. `GetService` might return `ConfigurationsReady=RECONCILING` for up to 60s post-deploy.

On cycle N: provider returns `creating`. Target becomes `creating`.
On cycle N+1: provider returns `running`. Target → `running`.

**Result:** Deterministic. The transition guard permits `creating → running`. The path always converges to `running`.

### Replay after stale reads

Same as above — the reconciler's status mapping is pure; the same provider response always yields the same status string.

### Replay after partial delete

1. `dispatchDestroy` calls `DeleteService` → 200 OK.
2. `confirmDeleted` starts polling.
3. Process crashes during the 5s poll loop (before NOT_FOUND observed).
4. On restart: sweep finds op. If `updated_at` is fresh (heartbeat fired) → op preserved. The `current_operation` still points at the destroyed op.
5. Because `status = 'deleting'` and `current_operation != ''`, both `shouldDispatchDeploy` and `shouldDispatchDestroy` return false.
6. **The target is stuck at `deleting` forever.**

**This is a real gap.** The `confirmDeleted` loop does not fork a background goroutine — it blocks the dispatcher's `Destroy` call. If the process dies inside `confirmDeleted`, the target row has `status = 'deleting'` and `current_operation = opID`. The CAS claim fails (op is not `in_progress` anymore — sweep marks it `failed`). Nothing re-dispatches.

**Fix:** The `confirmDeleted` loop should spawn its own goroutine such that the heartbeat persists through the poll. OR: the dispatcher's `Destroy` context should inherit the heartbeat timer so the op stays live through the confirm window.

Current design: `destroyCtx, cancel := context.WithTimeout(ctx, DeployTimeout)` — the destroy call itself has the same 15-min budget as a deploy. The confirm loop inside runs at 5s intervals for up to 60s. If the process crashes inside confirmDeleted, the heartbeat goroutine continues for up to `DeployTimeout` (15 min) — but if the process itself is gone, the goroutine is gone too. **The op heartbeat dies with the process.** The sweep will reclaim it immediately on restart.

**Result:** Not fully deterministic. A crash inside `confirmDeleted` leaves a `deleting` target with a terminal op. Recoverable only via operator action (manual status clear). **Phase 2.9 fix: heartbeat should span `confirmDeleted` to keep the sweep off this op.**

### Replay after `succeeded_stale` loop at threshold

The stale-count is in-memory. If the process restarts at staleCount=2, the guard fires again on the next dispatch rather than holding. This means the pathological loop burns quota for 3 additional cycles post-restart before holding. **This is acceptable.** The cost is 3 cycles × 30s = 90s of wasted provider quota. The guard still fires; it just takes longer. Does not corrupt state.

---

## Single vs Multi-Pod Determinism

**Single-pod: fully deterministic.** All state is in-process; SQLite serializes writes via WAL.

**Multi-pod: NOT deterministic.** Two pods can race the same CAS claim, but SQLite's WAL lock means only one write succeeds. The loser logs `[DISPATCH_CLAIM_SKIP]`. Result is correct but non-deterministic which pod wins. No corruption — just timing variance.

The multi-pod case additionally cannot distinguish a peer pod's live operation from an abandoned one in the runtime sweep. The Phase 2.7 deferred-followups document this as Phase 3 material.

---

## Verdict

**Single-pod: fundamentally sound.** The only structural non-determinism is `confirmDeleted` crash leaving a stuck `deleting` target (actionable: Phase 2.9 fix). Everything else either converges normally or correctly handles via sweep.

**Multi-pod: intentionally out of scope.** Documented as Phase 3 HA work.

---

## Related
- [[phase2.9/reconciliation-convergence-assessment]] — C-1 is the most significant non-convergence
- [[reconciliation-guarantees]] §Determinism guarantees
- [[phase2.9/chaos-survivability-assessment]]