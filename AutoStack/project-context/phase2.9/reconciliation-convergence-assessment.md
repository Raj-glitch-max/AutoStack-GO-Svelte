# Phase 2 Finalization — Reconciliation Convergence Assessment
## 1. Reconciliation Convergence Proof Pass

**Last Updated:** 2026-05-14

---

## Executive Summary

The reconciliation loop **converges for the majority of lifecycle paths**. However, two structural gaps prevent unconditional convergence: one already in the deferred-followups tracking, and one identified here.

---

## Convergence Topology

### Terminal happy paths

| Path | Converges? | How |
|---|---|---|
| `pending` → Deploy → success | ✅ | dispatcher releases `updating`, poll promotes to `running` |
| `pending` → Deploy → provider error | ✅ | target → `error`, circuit holds, operator respec required |
| `deleting` → Destroy → success | ✅ | dispatcher releases `deleted`, poll-skip guard blocks re-entry |
| `updating` → provider returns `running` | ✅ | `updateTargetStatus` persists `running` |

### Non-terminal paths that also converge

| Path | Converges? | How |
|---|---|---|
| Deploy succeeds, rollout moved, stale ≤ 2 | ✅ | releases `pending`, re-dispatches with current spec |
| Deploy succeeds, stale ≥ 3 | ✅ | releases `error` with clear loop message; circuit holds |
| `error` + `pending_destroy` + `endDate` set | ✅ | Phase 2.4 H-1: `promoteToDeleting` catches this at `error` |
| Provider transient flap (first `error` from `updating`) | ✅ | Phase 2.4 suspicion hold: requires 2 consecutive observations |

---

## Convergence Risks

### C-1: running + pending_destroy + endDate gap

**Severity: Structural — blocks Phase 2 trust claim**

When an operator sets `endDate` on a rollout whose target is `running` (not `error`), the following occurs:

1. `pending_destroy = true` is set on the target.
2. The auto-promote in `reconcileOne` only triggers when `previousStatus == "error"`.
3. The target stays `running`.
4. `shouldDispatchDeploy` returns false (status not `pending`).
5. `shouldDispatchDestroy` returns false (status not `deleting`).
6. No poll occurs (target is not `pending|deleting` and has no `current_operation`).

The target is **stuck at `running`** with no path to `deleting`.

**Impact:** The `HandleCloudAccountDelete` refinement (deferred from Phase 2.8) requires this gap to be closed for proper lifecycle closure. The Phase 2.8 deferred-followups explicitly list "Cloud_account region change refusal" and "HandleCloudAccountDelete refusal" as Phase 2.9 items — both are blocked on resolving the `running + pending_destroy` stuck state.

**Fix (Phase 2.9):** Add `running` to the auto-promote condition, or handle `pending_destroy` in the main reconcile branch:

```go
// In reconcileOne's status check, after reading pending_destroy/rolloutEndDate:
if pendingDestroy && rolloutEndDate != "" && previousStatus == "running" {
    r.promoteToDeleting(targetID)
    return reconcileSkipped
}
```

**Status: Identified here. Not yet implemented.**

---

### C-2: Region change on cloud_account diverges active targets

**Severity: Structural — in deferred-followups**

When `cloud_account.region` changes, `reconcileOne` re-reads `cloud_accounts.region` each cycle via the SQL join. All provider calls use the **new** region. This means:

- External IDs from the old region become invisible (wrong region in GetService path).
- Destroy tries to delete from wrong region → NOT_FOUND each time → idempotent success but old service persists.
- Status poll queries wrong region → NOT_FOUND → target marked `error`.

**Status: Listed in phase2.8/deferred-followups.md for Phase 3. Cloud Run `serving_revision` field also deferred to Phase 3.**

---

## Oscillation Analysis

### Does the system振荡 forever?

| Scenario | Forever? | Why |
|---|---|---|
| Respec-flapping rollout + stale ≤ 2 | No | Dispatches 3 times then holds at `error` |
| Provider flap while `updating` | No | Suspicion counter requires 2 consecutive errors |
| Dispatch race (two ticks simultaneously) | No | CAS ensures one loser bails immediately |
| Stale deploy success, rollout unchanged | No | Releases `updating`; next poll promotes to `running` |

---

## Replay-after-terminal-state behavior

If `dispatchDeploy` completes and releases `updating`, but a second reconcile tick also evaluates the same target before the first tick's release persists:

- The CAS claim on `current_operation` guards against double-dispatch.
- The status-poll skip guard (`currentOp != ""`) blocks status polling while dispatch is in-flight.
- These two guards together make double-in-flight impossible.

If the dispatcher completes and releases, but the sweep has already marked the op `failed` (pre-heartbeat era, or post-heartbeat-timeout):

- `releaseTarget`'s CAS `WHERE current_operation = :op` returns 0 rows.
- `RELEASE_LOST_OWNERSHIP` is logged.
- `writeOwnershipLostHistory` is called in the dispatcher's success branch.
- The target's final state reflects the sweep's decision (abandoned → `error`), not the dispatcher's (succeeded → `updating`).

**This is correct.** The sweep is a legitimate authority; the dispatcher's success is overridden.

---

## Honest Unknown States

The reconciler does NOT self-correct these — they require operator action:

| State | Meaning | Recovery |
|---|---|---|
| `error` + circuit open | Permanent failure (auth, quota, or ≥5 transient) | Fix root cause, respec |
| `error` + `pending_destroy=true` + `endDate` set + status≠`error` | Destroy intent not consumed | Phase 2.9 fix required |
| `creating` for > 15 min | Provider call hung | Sweep reclaims after 5 min runtime; operator respec |

---

## Verdict

**Mostly converges.** The `running + pending_destroy` gap (C-1) is a real correctness bug that blocks Phase 2 trust claims. It must be closed in Phase 2.9 before the system can be called trustworthy. Region-change divergence (C-2) is Phase 3 material per the existing deferred tracking.

The core reconciliation engine — single-threaded polling with CAS-based dispatch — is sound. The convergent paths all terminate. The remaining gaps are discrete, documented, and do not require architectural changes to fix.

**Recommended action:** Close C-1 in Phase 2.9. Document C-2 as Phase 3 scope.

---

## Related
- [[phase2.9/lifecycle-closure-assessment]] — addresses C-1 via the Phase 2.9 handle-cloud-account-delete work
- [[reconciliation-guarantees]]
- [[phase2.8/deferred-followups]] — C-2 is listed as Phase 3