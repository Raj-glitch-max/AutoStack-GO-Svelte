# Operational Entropy Assessment — Phase 2.4

## Last Updated
2026-05-14

## Premise

A long-running deployment platform accumulates "operational entropy":
stale records, dead lineage, abandoned operations, unresolved retry
markers. None is a correctness bug today; together they degrade
operability over time.

## Inventory of entropy sources

### E-1: `operations` table grows forever

Already covered in [[operation-retention-ttl-proposal]]. Phase 2.5
implements TTL.

### E-2: `deployment_history` table grows forever

Same as E-1. Phase 2.5 retention policy applies.

### E-3: Orphan cloud resources (post-cascade-delete or post-error)

**Setup:** A rollout is deleted via admin (bypassing
HandleRolloutDelete). Targets cascade-delete. Cloud Run services
persist.

**Setup B:** A destroy operation fails. Target → error. Operator
forgets. Cloud Run service persists indefinitely.

**Entropy effect:**
- Cost accumulation.
- `autostack-managed=true` labeled resources without corresponding
  AutoStack records.

**Fix:** Orphan-cleanup scanner. Phase 2.5 work.

### E-4: Stale failure-count state in `Reconciler.failures` map

**Setup:** A target trips the circuit breaker. Operator clears the
target via respec. Status → pending. `flipCloudTargetsToPendingOnRespec`
sets status but does NOT clear the in-memory failure count.

**Behavior:**
- Reconciler reads failure count = 5 (circuit open).
- shouldDispatchDeploy returns true (status=pending, no in-flight op).
- reconcileOne enters the "circuit open, skipping" branch BEFORE
  shouldDispatchDeploy is consulted.

Wait — re-reading the code:
```go
if failureCount >= r.config.FailureThreshold {
    log.Printf("Target %s: circuit open ...", targetID, ...)
    return reconcileSkipped
}
```

This is checked early in reconcileOne, before any dispatch branch.
**So a respec'd target with a residual failure count would be skipped
forever (until process restart).**

**Severity:** Medium. Operator respec is the documented recovery path
from `error`. If it doesn't actually clear the circuit, recovery is
broken.

**Phase 2.4 fix:** `flipCloudTargetsToPendingOnRespec` should call
`r.clearTargetFailure(targetID)`. But it doesn't have access to the
reconciler instance (it's in the controllers package, the reconciler
is a separate package).

**Decision:** Add a clearing pass to the reconciler itself: when a
target transitions from `error → pending` (observed at the SELECT
point), clear in-memory state for it. **Or** simpler: reset
`Reconciler.failures[targetID]` when the reconciler's dispatch path
sees status=pending with no current_operation — since this is the
"healthy retry entry" state.

**Actually the cleanest fix:** the dispatcher's success branch already
calls `r.clearTargetFailure(targetID)`. A failed deploy stays at error
and isn't dispatched. So failure-clearing happens on the next successful
deploy. Good.

But the **circuit-open skip happens BEFORE dispatch is attempted**. So
a respec'd target with circuit=open never gets to the dispatch path.

**The real fix:** In `reconcileOne`, after the circuit check, also
check: if the target's `status` is `pending` AND `current_operation` is
empty, this is a fresh deploy intent. Clear the circuit and proceed.

**Phase 2.4 implementation:** Add a clarification: clear the per-target
failure count when the target enters a `pending` dispatch-eligible
state.

Wait — I should re-verify against actual code behavior. Let me check
the in-memory state lifecycle.

`Reconciler.failures` is in-memory. Process restart clears it. So an
operator can also "fix" the issue by restarting AutoStack. Acceptable
recovery path, but not great UX.

**Decision:** Land in Phase 2.4 implementation. Trivial change in
cloud.go.

### E-5: Stale suspicion counter

**Setup:** A target's `updating → error` triggered one suspicion-hold.
Operator respec's; target goes pending. The suspicion counter for that
target is still set to 1 in memory.

**Behavior:** Next time the target reaches `updating` and observes an
error, the counter increments to 2 immediately, persisting `error`
without the intended 2-observation tolerance.

**Severity:** Low. Operator-driven respec is rare; the suspicion
counter is per-deploy-cycle in spirit.

**Phase 2.4 fix:** Same pattern as E-4 — clear suspicion when target
enters dispatch-eligible pending.

**Decision:** Land in Phase 2.4 implementation.

### E-6: Stale `last_synced` on terminal targets

**Setup:** A target reaches `deleted`. Reconciler's terminal-skip
guard prevents further GetStatus polls. `last_synced` is frozen at the
time of the final poll.

**Entropy effect:** UI showing "last synced 6 months ago" for deleted
targets is misleading; but the target IS deleted, so it shouldn't be
on the UI dashboard at all.

**Severity:** None (operator-facing UX issue, not a control-plane
concern).

### E-7: Stale rollout records with `endDate` set but stale targets

**Setup:** Rollout has `endDate` set. All targets reached `deleted`.
Rollout row remains.

**Entropy effect:** Inactive rollouts accumulate. If 1000 rollouts have
ended, the rollouts table has 1000 rows.

**Phase 2.5 work:** Rollout retention policy (delete rollouts older
than N days with endDate set). Cascade-deletes targets, ops, history.
But would need to PRESERVE history — see retention proposal.

### E-8: Stale revisions in Cloud Run

Cloud Run's own retention applies (default: keep most recent N).
AutoStack doesn't manage this. No entropy contribution from AutoStack.

### E-9: Stale `pending_destroy=true` flag after manual recovery

**Setup:** Operator sets endDate during in-flight deploy →
pending_destroy=true. Deploy fails → target=error, pending_destroy
persists. Operator manually clears endDate AND status to recover the
rollout.

**Behavior:**
- Phase 2.4 C-3 fix promotes `error+pending_destroy → deleting`.
- This would interfere with operator's manual recovery.

**Mitigation:** The fix should only promote when `endDate` is still
set on the rollout. **Check this in the implementation.**

### E-10: In-memory state on long-running processes

`Reconciler.failures`, `Reconciler.suspicions`, lastErrorTime — all
in-memory, all cleared on restart. No persistent entropy.

**No fix needed.**

## Entropy summary

| Source | Persistence | Phase 2.4 fix |
|---|---|---|
| E-1 ops table grows | persistent | Phase 2.5 TTL |
| E-2 history grows | persistent | Phase 2.5 TTL |
| E-3 orphan cloud resources | persistent (cloud-side) | Phase 2.5 scanner |
| E-4 stale failure count | in-memory | **Land in Phase 2.4: clear on pending dispatch entry** |
| E-5 stale suspicion | in-memory | **Land in Phase 2.4: clear on pending dispatch entry** |
| E-6 last_synced on deleted | persistent (cosmetic) | none needed |
| E-7 stale rollouts | persistent | Phase 2.5 retention |
| E-8 cloud revisions | provider-managed | none needed |
| E-9 pending_destroy + operator manual recovery | persistent + UX | **Land in Phase 2.4: check endDate before auto-promotion** |
| E-10 reconciler in-memory | resets on restart | none needed |

## Phase 2.4 implementation in this area

- E-4 + E-5: Clear in-memory failures/suspicions for a target when its
  next observation is `status=pending && current_operation=""` (the
  fresh-dispatch entry state).
- E-9: The Phase 2.4 C-3 auto-promotion fix verifies `rollout.endDate
  != ""` before promoting target to deleting.

## Related
- [[reconciliation-convergence-assessment]]
- [[operation-retention-ttl-proposal]]
