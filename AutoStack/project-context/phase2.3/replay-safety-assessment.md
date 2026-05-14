# Replay Safety Assessment — Phase 2.3

## Last Updated
2026-05-14

## Premise

If the same desired state is presented to the reconciler twice (after
restart, after a sweep, after a CAS-race retry), the resulting provider
state and persisted PocketBase state MUST be the same. Anywhere this is
not true is a replay-corruption risk.

## What the system replays today

| Replay path | Trigger | Outcome | Verdict |
|---|---|---|---|
| Startup → in_progress ops swept | process restart | ALL in_progress ops → `failed`; targets pointing at them → `error`. New ticks re-process from `error` (circuit breaker holds until operator reset). | Safe single-pod. **Wrong** multi-pod (sweeps live ops). |
| Startup → `pending` targets | first tick after restart | dispatcher CAS-claims, calls Deploy. | Safe IF the previous process didn't already partially deploy — if it did, we hit the partial-deploy convergence path below. |
| Startup → `creating` targets | sweep cleared `current_operation` and moved status to `error` | reconciler sees `error` + no in-flight op + circuit breaker holds (failure count 0 after restart, so first tick polls GetStatus); status-poll reflects actual provider state. | Safe — but suspicion counter is reset, so a transient flap from `error → error` may be persisted again. Acceptable. |
| Startup → `updating` targets | sweep does not touch these (no in-flight op when sweep runs, by invariant — release fired before crash) | status-poll path resolves actual provider state. | Safe. |
| CAS race lost | two ticks reach the same target simultaneously (only possible with multi-pod or with a tick-overlap edge case) | losing dispatcher marks its op `cancelled`, returns `reconcileSkipped`. Winning dispatcher proceeds. | Safe. |
| Stale-spec replay | rollout updated during deploy | dispatcher marks op `succeeded_stale`, releases target → `pending`, writes failed history with msg "stale spec". Next cycle re-dispatches with new manifest. | Safe — provider sees an update-via-revision-swap, which is convergent. |
| Reconcile re-entry mid-deploy | reconciler ticker fires while dispatcher is mid-Deploy | `currentOp != ""` skip-guard fires → `[DISPATCH_IN_FLIGHT]` logged → reconcileSkipped. | Safe (Phase 2.1 fix). |
| `succeeded_stale` re-dispatch loop | rollout keeps updating during deploys | each cycle observes stale, re-dispatches; CPU/quota cost but no corruption. | Safe but wasteful. **Hazard:** quota-class failures will eventually open circuit, but the rollout could theoretically chase its own tail indefinitely. |
| Replay against partially-created Cloud Run service | mid-Deploy crash before `waitForServiceReady` returned, restart, target → error after sweep, operator clears error → new dispatch | Cloud Run's `GetService` will return existing → Deploy enters Update path → posts the same spec → idempotent | Safe under same spec. |
| Replay against partially-deleted Cloud Run service | crash during `DeleteService`, restart, target → error → operator sets endDate again | dispatcher re-dispatches Destroy → provider GetService returns NOT_FOUND (already deleted) → Destroy returns nil → target → deleted | Safe (idempotent NOT_FOUND). |
| Replay of rollback | not applicable — Rollback is `ErrNotImplemented` | n/a | n/a; refusal is the right answer |

## Where replay can corrupt today

### 1. Multi-pod startup sweep (documented, deferred)

A second pod boot sweeps the **first pod's live in-progress ops**. The
first pod's dispatcher will eventually return, find its op marked
`failed` by the second pod's sweep, and try to `releaseTarget`. The
release-CAS guard prevents the persisted status from flipping back to
`updating`, but:

- Provider-side: the first pod may have created a real Cloud Run service.
- PocketBase-side: target shows `error` with message "abandoned: process
  restart".
- **Operator-visible truth gap:** the service exists; AutoStack denies it.

This is the dominant correctness gap for any multi-pod deployment of
AutoStack itself. **Phase 2.3 cannot fix this** without pod-identity
stamping on operations. Mitigation today: ship single-pod only.
Phase 2.5 work: add `operations.owned_by_pod` and refuse to sweep ops
owned by a peer pod.

### 2. Heartbeat infrastructure exists but startup sweep ignores it

`heartbeat()` refreshes `operations.updated_at` every 60s during a live
deploy. The sweep, however, ignores the heartbeat and marks **every**
in-progress op `failed` regardless of recency.

**The scenario that exposes this:**
- Process A starts a Deploy at T0.
- Process A heartbeats at T0+60s, T0+120s, T0+180s.
- Process A receives a SIGTERM at T0+200s; process B starts at T0+201s.
- Process B's sweep sees the op `updated_at = T0+180s` (live within the
  last 21s) and marks it `failed`.
- Process A is gone, but if a sidecar or graceful-shutdown path had given
  it a chance to finish, the op would have actually succeeded.

For the single-pod case where a restart is genuine, this is correct
(the dying process cannot come back). For the rolling-restart case (a
common k8s deploy pattern for the AutoStack pod itself), this is wrong —
the new pod assumes the old pod is dead before it actually is.

**Phase 2.3 fix landing:** Sweep ignores ops whose `updated_at` is
within `2 × heartbeatInterval` (2 min). This is a low-cost change that
preserves the "abandoned" semantics for actually-stale ops while
giving live ops a brief window to complete or self-mark abandoned.

### 3. `succeeded_stale` infinite loop

A rollout updated continuously (e.g., automation that flips a field
every 25s while deploys take 30s) will produce a perpetual stale
re-dispatch loop. The circuit breaker only fires on failures, and
`succeeded_stale` increments `failures` via `recordTargetFailureWithCategory`?

Looking at the code path: the stale case returns `reconcileFailed` but
calls neither `recordTargetFailureWithCategory` nor `clearTargetFailure`.
So the circuit breaker does NOT engage. **A pathological updater
loop would burn quota indefinitely.** Documented; defer.

### 4. Hard-deleted rollout mid-deploy → orphan replay

If the rollout is hard-deleted while Deploy is in flight,
`HandleRolloutDelete`'s cascade refusal blocks the delete — UNLESS the
operator goes through PocketBase admin and bypasses the controller hook.
That is an explicit "operator broke the invariant" case; we accept it.

The non-admin path is gated by `[CLOUD_DELETE_REFUSED]` which is the
correct refusal. ✓

### 5. Spec-update during dispatcher panic recovery

If `dispatchDeploy`'s `defer` panic-recovery fires (provider call
panicked), the recovery path:
- marks op `failed`
- releases target → `error`
- writes a history row with action=updated/created and status=failed

But it does NOT re-check stale-spec. If a respec arrived between
`[DEPLOY_START]` and the panic, the target is now `error` with no
indication that the spec moved. The respec handler's
`flipCloudTargetsToPendingOnRespec` only flips targets out of
`{running, updating, error}` to `pending` on a spec change… wait, let
me re-read.

Re-read: `flipCloudTargetsToPendingOnRespec` skips targets in
`{deleted, deleting, pending}`. So an `error` target IS flipped to
`pending`. Good — the respec recovery is convergent.

But if the panic occurred BEFORE the respec arrived, the target is
`error` with no respec yet. Operator must clear `error` (no auto-retry
from error). Acceptable for single-pod.

## Phase 2.3 implementation in this area

1. **Heartbeat-aware sweep** (above). Reduces false-abandon rate during
   rolling restarts.
2. **Cycle-ID stamped onto operations.** Lets incident reconstruction
   reconnect a sweep-abandoned op to the cycle that dispatched it.

## Non-fixes (deferred)

1. **Pod-identity stamping** — required for multi-pod safety.
2. **`succeeded_stale` circuit-breaker integration** — wait until we see
   a real instance of the pathology before adding logic.
3. **Replay determinism under provider-side partial-create** — Cloud
   Run's CreateService is asynchronous; we cannot tell from the API
   whether the service is "partially created" or "fully created and
   awaiting Ready". Convergent re-Deploy via Update path is the only
   honest answer.

## Related
- [[reconciliation-determinism-review]]
- [[ownership-integrity-review]]
- [[lro-survivability-review]]
- [[../reconciler/sweep-and-heartbeat-semantics]]
- [[../reconciler/restart-behavior]]
