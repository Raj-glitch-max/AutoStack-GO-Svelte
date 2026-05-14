# Ownership Integrity Review — Phase 2.4 (Chaos-Aware)

## Last Updated
2026-05-14

## Premise

Phase 2.3 verified ownership correctness for the steady-state and
common-failure cases. Phase 2.4 stress-tests ownership under chaos:
lease expiry timing, delayed heartbeat propagation,
crash-before-heartbeat, reclaim races, delayed release, split-brain
possibilities, replay ownership divergence.

## Ownership lease characteristics

| Property | Today |
|---|---|
| Lease mechanism | `deployment_targets.current_operation = op.id` |
| Lease grant | CAS UPDATE with `(current_operation IS NULL OR '') AND status IN (...)` |
| Lease holder identity | The dispatcher goroutine in this process |
| Lease renewal | `operations.updated_at` refreshed every 60s by heartbeat |
| Lease expiry policy | Heartbeat-aware sweep: ops outside `2 × heartbeatInterval` window AND with `updated_at > started_at` are abandoned |
| Lease release | CAS UPDATE with `WHERE current_operation = our_op_id` |
| Lease takeover | Sweep marks abandoned op `failed` and clears `current_operation` |

## Stress-test scenarios

### OS-1: Crash exactly between CAS claim and first heartbeat

**Timeline:**
- T+0: createOperation → operations row with `started_at=updated_at=T+0`.
- T+0.05: claimTarget CAS succeeds → target.current_operation set.
- T+0.06: dispatcher goroutine starts heartbeat goroutine.
- T+0.1: process SIGKILL.
- Process restart at T+1.
- T+1: startup sweep runs.

**Heartbeat behavior:** Never fired. `updated_at == started_at`.

**Sweep behavior (Phase 2.3):**
- `neverHeartbeated = !updated.After(started)` → true.
- Always marked abandoned. ✓

**Outcome:** Operation marked failed, target → error. The Cloud Run
service was never touched (Deploy hadn't been called yet either). No
orphan. ✓

**Verdict:** ✓ Safe.

### OS-2: Crash after heartbeat fired once, then DB busy

**Timeline:**
- T+0: createOperation.
- T+60: heartbeat #1 fires → `updated_at=T+60`.
- T+90: process SIGKILL (DB was busy after T+60; heartbeat didn't
  retry).
- Process restart at T+91.
- T+91: startup sweep runs. Heartbeat window cutoff = T+91 - 120 =
  T-29. `updated_at=T+60` > T-29 → within window. **Skipped as live.**

**Hazard:** Live-classified op whose dispatcher is dead.

**Behavior:**
- Sweep does not touch the op or target.
- Target remains in `creating` with `current_operation=op.id`.
- Reconciler's in-flight skip-guard fires on every tick.
- Target is stuck forever.

**Severity:** Medium. Phase 2.3 heartbeat-aware sweep CAN leave a
target stuck if the process dies after the first successful heartbeat
but before the next one fires. The first-heartbeat guard (OS-1)
doesn't help here.

**Mitigation today:** Operator manually unsticks the target via
PocketBase admin (clear `current_operation`, set status to `error` or
`pending`).

**Phase 2.4 fix considered:** A *runtime* sweep that periodically
checks for ops whose `updated_at` is OLDER than the liveness window
(opposite of startup sweep) and marks them abandoned. This catches
post-first-heartbeat death.

**Decision:** Land in Phase 2.6 (chaos survivability). The runtime
sweep is the symmetric counterpart of the startup sweep and closes
this gap.

### OS-3: Process restart during a 14-min deploy

**Timeline:**
- T+0: dispatcher starts deploy.
- T+60, T+120, ..., T+780: heartbeats fire (13 heartbeats).
- T+840 (14 min): provider returns success.
- T+841: dispatcher calls completeOperation, releaseTarget.
- T+842: process SIGKILL.
- Process restart at T+843.

**Behavior:**
- The deploy succeeded; op is `succeeded`; target was released to
  `updating`.
- Sweep finds NO in-progress op for this target.
- Reconciler's status-poll continues from `updating` → eventual
  `running`.
- ✓ Convergent. Excellent.

### OS-4: Process restart during a 16-min deploy (DeployTimeout=15min)

**Timeline:**
- T+0: dispatcher starts deploy.
- T+60, T+120, ..., T+840: 14 heartbeats.
- T+900 (15 min): DeployTimeout fires. ctx.Done().
- waitForServiceReady returns "cancelled" error.
- Deploy returns (result, nil) with status="cancelled".
- Dispatcher's `result.Status == "error"` branch:
  - completeOperation(opID, "failed", "service readiness check
    cancelled").
  - releaseTarget(opID, ..., "creating", "error", msg).
- T+901: heartbeat exits.
- T+910: process SIGKILL.
- Process restart at T+911.

**Behavior:**
- The op was already marked failed at T+900.
- Sweep finds no in-progress op for this target. ✓

### OS-5: Two pods boot simultaneously after AutoStack pod restart

**Timeline:**
- T+0: Pod A boots, starts running deploys.
- T+5min: Pod A has 3 in-progress deploys, all heartbeating.
- T+5min01s: Pod B boots (operator misconfigured the deployment to
  scale to 2).

**Pod B startup sweep behavior:**
- Sees 3 in-progress ops, all with recent heartbeats.
- Heartbeat-aware policy: all classified as live, skipped.
- ✓ Pod A's work is preserved.

**Subsequent cycles:**
- Both pods' reconcilers run.
- Both compete for the SAME pending targets via CAS.
- Each CAS race produces one winner, one cancel.
- For status-poll: both pods read the same target rows, both call
  GetStatus, both attempt to write last_synced + status. SQLite WAL
  serializes writes; last-writer-wins. Status flap possible but
  bounded.

**Severity:** Phase 2.3 heartbeat-aware sweep already preserves Pod A's
work. But:
- Two pods polling status doubles API calls.
- Status writes race; potentially confusing logs.

**Mitigation today:** Don't run multiple pods. Documented in
[[../phase2.3/remaining-operational-blockers]].

**Phase 2.4 fix:** None. Pod-identity stamping is Phase 2.5 work.

### OS-6: Heartbeat goroutine leaks beyond dispatcher return

**Timeline:**
- T+0: dispatcher starts. Heartbeat goroutine launched with
  `deployCtx`.
- T+10s: dispatcher returns. `defer cancel()` fires → deployCtx done.
- T+10.01s: heartbeat goroutine's `<-ctx.Done()` case fires → return.

**Behavior:** Heartbeat exits within ~milliseconds. No leak. ✓

**Edge case:** If `defer cancel()` is somehow not called (panic before
defer registration? No — `cancel` is captured at line of declaration,
defer is registered before any potential panic in the function body),
the heartbeat would run until the parent context (from `reconcileOne`)
expires. Worst case is bounded by DeployTimeout.

### OS-7: CAS race with sweep

**Setup:** A dispatcher (Pod A) holds an op. Pod A SIGKILL'd. Pod B
boots. Sweep is heartbeat-aware:

Case (a): heartbeat fired recently → sweep skips → op stays in-progress.
Reconciler in-flight guard skips. **Stuck.** (See OS-2.)

Case (b): heartbeat stale → sweep marks abandoned. Target released to
error. ✓

The split-brain risk: Pod A is "actually dead" but heartbeat just fired.
Window: < 2 min from last heartbeat.

**Mitigation:** OS-2's Phase 2.6 runtime-sweep proposal closes this.

### OS-8: PocketBase admin manually clears `current_operation`

**Setup:** Operator clears the field directly.

**Behavior:** The dispatcher (if still running) will eventually call
`releaseTarget` with CAS `WHERE current_operation = our_op_id`. Match
0 rows → `[RELEASE_LOST_OWNERSHIP]` logged, no write. ✓

But the operation row stays `in_progress` — the dispatcher's
`completeOperation` also CAS-guards on `status = 'in_progress'`. It
SUCCEEDS (the op is still in_progress, the operator only touched the
target). So the operation reaches a terminal status. ✓

**Hazard:** If the operator ALSO directly updates the op row, anything
goes. This is "operator broke the invariant" — out of scope.

## Ownership integrity summary

| Scenario | Today | Phase 2.4 |
|---|---|---|
| OS-1 crash before first heartbeat | ✓ swept | unchanged |
| OS-2 crash mid-heartbeat → stuck | ⚠️ target stuck | runtime sweep in Phase 2.6 |
| OS-3 restart during long deploy | ✓ released cleanly | unchanged |
| OS-4 restart after DeployTimeout fired | ✓ already failed | unchanged |
| OS-5 multi-pod | ⚠️ status flap, doubled APIs | Phase 2.5 pod-identity |
| OS-6 heartbeat leak | ✓ no leak | unchanged |
| OS-7 sweep race vs sentinel | ⚠️ stuck-window | Phase 2.6 runtime sweep |
| OS-8 admin clears field | ✓ release CAS guard | unchanged |

## Phase 2.4 implementation in this area

None directly. The runtime-sweep proposal (closes OS-2, OS-7) lands in
Phase 2.6.

## Related
- [[../phase2.3/ownership-integrity-review]]
- [[../phase2.3/replay-safety-assessment]]
- [[../reconciler/operation-ownership]]
- [[../reconciler/sweep-and-heartbeat-semantics]]
