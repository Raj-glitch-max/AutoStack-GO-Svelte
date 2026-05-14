# Phase 2 Finalization — Chaos Survivability Assessment

**Last Updated:** 2026-05-14

## Methodology

For each chaos scenario: simulate the system's execution path, evaluate
whether it produces the correct / honest outcome, and identify remaining gaps.

---

## C-1: Restart during active deploy

**What happens:**
1. `dispatchDeploy` in goroutine calls `Provider.Deploy`.
2. `deployCtx` is `context.WithTimeout(ctx, DeployTimeout)` — 15 min.
3. `heartbeat` goroutine runs alongside.
4. Process dies (OOM, SIGKILL, panics in non-reconciler goroutines).
5. On restart: `SweepAbandonedOperations` runs.

**Sweep behavior:**
- Op has `updated_at != started_at` (heartbeat fired at least once within 2 min): **preserved as live.**
- Op has `updated_at == started_at` (never heartbeated): **marked failed.**

**What survives:**
- GCP Cloud Run service: may be partially created or complete. AutoStack has no way to know.
- `deployment_targets` row: has `current_operation = opID`, `status = creating`.
- Operations row: either preserved as live (heartbeated) or marked `failed` (never heartbeated).

**If preserved as live:** On next tick, `current_operation != ''` → dispatch skipped → status poll skipped → **target stuck `creating`**. Only the runtime sweep (which runs every 5 min with a 5-min stale threshold) will reclaim it.

**If marked failed:** Target → `error`, `current_operation` cleared. Operator must respec.

**Correctness:** Honest — AutoStack can't know what happened to the Cloud Run service. The `failed` classification tells the operator to investigate and respec.

**Gap:** If the op is preserved as live (heartbeat fired once), the runtime sweep won't catch it for 5+ min. During that window, the target is stuck `creating`. This is a window of stuck state with no operator visibility into why.

**Severity:** Low — requires the crash to happen after the first heartbeat but before the runtime sweep activates. Extremely narrow window (~60s to 5min after dispatch).

---

## C-2: Restart during active destroy + confirmDeleted phase

**What happens:**
1. `Provider.Destroy` calls `DeleteService` → 200.
2. `confirmDeleted` starts polling every 5s.
3. Process dies inside `confirmDeleted`.

**Sweep analysis:**
- `confirmDeleted` is called inside `dispatchDestroy`. The `destroyCtx` has a 15-min timeout, but the process dies before any confirm loop tick completes.
- The heartbeat goroutine fires every 60s. The process dies before the second heartbeat.
- `updated_at == started_at` (never heartbeated) → sweep marks op `failed`.

**Result:** `operations.status = failed`, `current_operation = opID` (still set, since release didn't run). Sweep set `deployment_targets.status = error`, `current_operation = ''` (cleared).

**Next cycle:** `shouldDispatchDestroy` returns false (status is `error`, not `deleting`). No re-dispatch. Target stuck `error`.

**Gap:** This is a double-gap: crash during `confirmDeleted` + sweep marks error but doesn't redispatch. The "destroy already done on provider side" (DeleteService returned 200) is correct — the service IS deleted in GCP. But AutoStack's `deployment_targets` row says `error`.

**Severity:** Medium — recoverable if operator checks GCP console and manually clears the `error` target. But the target SHOULD be `deleted` since the destroy API succeeded.

---

## C-3: Provider timeout storms

**Scenario:** Cloud Run API returns 503s for 10 minutes straight.

**Cycle behavior:**
- Every `GetStatus` call: `err != nil` → `recordTargetFailureWithCategory(category=transient)`.
- Per-target circuit breaker: increment each time.
- After 5 consecutive failures: circuit opens; target skipped.

**Backoff behavior:**
- Cycle-level backoff fires due to `recordError()` triggering `lastErrorTime`.
- Backoff = `BackoffBase * 2^maxFailures`. At max failures, 5-minute backoff.
- No cycles run for 5 minutes.

**During backoff:** No API calls made. Target stays at whatever status it was.

**After backoff:** Cycle resumes; tries again. Repeat until Cloud Run recovers.

**Correctness:** ✅ Correct transient behavior. The circuit breaker prevents 5 calls per cycle per failing target during an outage.

**Gap:** The backoff doubles at each cycle even when the majority of targets are healthy. A single failing target causes global backoff for all targets. This is a known consequence of global `lastErrorTime` vs per-target backoff.

---

## C-4: Stale ownership reclaim (sweep wins vs dispatcher)

**Scenario:** Dispatcher dispatching deploy; sweep's 5-min runtime-sweep threshold fires while dispatcher is running.

**Mechanism:**
1. Dispatcher acquires op X on target T.
2. Runtime sweep runs; op X `updated_at` is within 5 min → no reclaim (first heartbeat guard).
3. Dispatcher takes 6+ min; runtime sweep fires again; op X `updated_at` is now stale (>5 min since last heartbeat).
4. Runtime sweep marks op X as `failed`.
5. Dispatcher completes `Provider.Deploy` successfully.
6. Dispatcher calls `completeOperation(succeeded)` → SQL: `UPDATE ... WHERE status = 'in_progress'` → **0 rows affected** (op is already `failed`). Silent no-op.
7. Dispatcher calls `releaseTargetWithExternal` → CAS `WHERE current_operation = opX` → **0 rows affected** (sweep cleared it).
8. `RELEASE_LOST_OWNERSHIP` logged.
9. `writeOwnershipLostHistory` called with observed outcome = `success` (dispatcher did observe success).

**Correctness:** ✅ The sweep is authoritative. The dispatcher's success observation is logged but does not overwrite the sweep's terminal `failed` classification. `deployment_targets` shows `error`. History shows the dispatcher's `success` row and the sweep's `abandoned` row separately.

**Operator visibility:** Two deployment_history rows: one says `success` (dispatcher's claim), one says `failed, message=abandoned: heartbeat went stale`. This is confusing but truthful — it shows both what happened.

---

## C-5: Replay during stale reads (GetStatus returns outdated)

**Scenario:** Cloud Run returns `Ready=SUCCEEDED` immediately but the service URL hasn't propagated globally.

**Behavior:** First poll: `running`. Second poll: still `running`. No ambiguity.

If Cloud Run returns a stale `RECONCILING` right after `Ready=SUCCEEDED`:
- First poll: `creating`.
- Second poll: `running`.
- Transition guard permits `creating → running`.

**Correctness:** ✅ Converges correctly.

---

## C-6: Delayed deletion convergence

Covered in [[phase2.9/drift-survivability-assessment]] D-4. Phase 2.8 fix is in place.

---

## C-7: Interrupted cleanup (releaseTarget fails after completeOperation)

**Scenario:** `completeOperation(succeeded)` runs (updates op to `succeeded`). Process panics before `releaseTarget` is called.

**Result:** Op = `succeeded`. Target = `creating` (not yet updated). `current_operation = opID` (not cleared).

**Sweep on restart:** Op is `succeeded` (terminal) — not touched by sweep (sweep only touches `in_progress`). Target still `creating` with `current_operation = opID`.

**Next cycles:** `current_operation != ''` → skip dispatch. `current_operation != ''` → skip poll. **Target stuck `creating` forever.**

**Gap:** `releaseTarget` is not idempotent on panic. If it doesn't run, the target is orphaned until operator manually clears `current_operation`.

**Severity:** Narrow — requires panic between `completeOperation` and `releaseTarget`, which are adjacent in the code (lines 291 and 293). Panic-recovery is present in `dispatchDeploy` but only catches panics in the dispatcher itself, not in the caller of dispatchDeploy.

**Fix (Phase 2.9):** Refactor so `completeOperation` and `releaseTarget` are one atomic DB transaction, or have the deferred panic recovery also attempt release.

---

## C-8: Reconciliation retry storms

**Scenario:** 5 targets, each with auth errors. Circuit opens on each after 5 failures.

- Cycle 1-5: fail, circuit +1 each.
- Cycle 6: all 5 circuits open — all skipped → `[CIRCUIT_OPEN] log per target`.
- No retry storm on auth errors — they are correctly non-retryable.

**If transient:** Circuit state is in-memory and resets on process restart. A process restart during transient failures clears all circuits — retry storms could emerge from restart. However, `recordTargetFailure` per failure in the same process would need to accumulate 5 before circuits open, so restart loss is bounded.

**Severity:** Low — worst case: one extra cycle of retry after restart, then circuits re-engage.

---

## C-9: Delayed heartbeat propagation

**Scenario:** Heartbeat goroutine is alive but DB is overloaded (write latency > 60s).

**Behavior:**
- First missed heartbeat tick: `HEARTBEAT_FAIL` logged, failure counter +1.
- 5 consecutive failures: `[HEARTBEAT_FAIL_PERSISTENT]`.
- Heartbeat's UPDATE `WHERE id = opID AND status = 'in_progress'` won't match if the op already went terminal.
- If DB is just slow (not down), the heartbeat continue firing.

**Sweep won't falsely reclaim** as long as the op's `updated_at` is fresher than the sweep's cutoff (2 × 60s = 2 min for startup; 5 min for runtime). A heartbeat tick of 90s would not trigger false reclaim.

**Correctness:** ✅ Heartbeat design is resilient to transient tick failures.

---

## Chaos Survivability Verdict

| Scenario | Correct? | Gap |
|---|---|---|
| Restart during deploy | Mostly ✅ | `creating` + preserved op → stuck `creating` until runtime sweep |
| Restart during destroy+confirm | Partial | Service deleted, target `error` — stuck target |
| Provider timeout storms | ✅ | No API storm; circuit breaker works |
| Sweep wins vs dispatcher | ✅ | Correct authority; confusing but honest |
| Replay during stale reads | ✅ | Converges |
| Interrupted cleanup (panic before release) | Partial | Target stuck `creating` — operator must intervene |
| Reconciliation retry storms | ✅ | Auth errors circuit immediately; transients bounded |
| Delayed heartbeat | ✅ | Heartbeat resilience handles tick failures |

**Most significant:** C-7 (panic between completeOperation and releaseTarget) and C-2 (restart during confirmDeleted) are the two gaps.

**Structural:** Both are Phase 2.9 scope and fixable without architectural change.

**Multi-pod chaos:** Not modeled — intentionally out of scope until Phase 3.

---

## Related
- [[phase2.9/replay-determinism-assessment]] — C-2 is the confirmDeleted gap
- [[phase2.9/lifecycle-closure-assessment]] — C-7 is the release gap
- [[sweep-and-heartbeat-semantics]]