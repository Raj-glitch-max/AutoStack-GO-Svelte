# Delete & Orphan-Risk Assessment — Phase 2.3

## Last Updated
2026-05-14

## The delete contract

Cloud rollouts MUST destroy provider resources before forgetting them.
A target row reaching `deleted` must mean the cloud service is gone
(or honestly unreachable, with that uncertainty surfaced).

## Today's delete pipeline

```
Operator: PATCH rollouts {id, endDate=now}
  → HandleRolloutUpdate (cloud branch, newEnd != "")
    → markCloudTargetForDestroy(rolloutID)
      → for each target in {rollout=:rid AND status NOT IN ('deleted','deleting')}:
          if current_operation != "": LOG [CLOUD_DESTROY_DEFER] and SKIP  ← ⚠️ intent loss
          else: target.status = 'deleting'

Reconciler tick (next ≤ 30s):
  → reconcileOne sees status=deleting
    → shouldDispatchDestroy → true
      → dispatchDestroy
        → createOperation('destroy')
        → CAS claim
        → Provider.Destroy
          → GetService → if NOT_FOUND: return nil (idempotent)
          → DeleteService → 200 OK = "request accepted"
        → on nil err: target → 'deleted'

Operator: DELETE rollouts/:id
  → HandleRolloutDelete refuses if any target.status != 'deleted'  ← ✓ orphan defense
  → otherwise: cascade delete (rollouts → deployment_targets → operations →
                deployment_history all cascadeDelete=true)
```

## Issues

### D-1: Destroy intent lost during in-flight deploy

**Severity:** HIGH

**Today:** `markCloudTargetForDestroy` skips targets with
`current_operation != ""` with a log message. **The destroy intent is
NOT persisted anywhere.** The dispatcher's release path doesn't re-check
for endDate; it just releases to `updating` (success) or `error`
(failure).

**Scenario:**
1. Operator triggers deploy at T0.
2. Operator decides to abort, sets `endDate` at T0+5min while deploy in
   flight.
3. `markCloudTargetForDestroy` logs `[CLOUD_DESTROY_DEFER]`, returns
   nil.
4. Dispatcher completes deploy at T0+8min, releases target to
   `updating`.
5. Next reconcile tick: target is `updating` with no in-flight op.
   `shouldDispatchDestroy` requires status=`deleting`, so no destroy
   fires. **The cloud service runs forever.**
6. Operator wonders why the rollout still seems active. Looks at
   PocketBase; sees endDate set on rollout but target is `running`.
   Confusing.

**Fix:** Add `deployment_targets.pending_destroy: bool`. Set to true
when `markCloudTargetForDestroy` defers. Dispatcher's release path
checks `pending_destroy` and flips `status = 'deleting'` instead of
`updating`/`running` on success.

**Status:** Schema change required. Two designs considered:

a. **`pending_destroy` boolean column.** Cleanest. Requires a new
   migration (1715300006).
b. **Dispatcher's release path always re-reads rollout, checks endDate.**
   Convergent without schema change but adds a read per release.

Option (a) chosen for clarity; option (b) is the fallback if migration
needs to be deferred.

**Phase 2.3 decision:** Implement option (a). Migration + dispatcher
change.

### D-2: No post-destroy NOT_FOUND confirmation

**Severity:** MEDIUM

**Today:** `Provider.Destroy` returns nil on `DeleteService` 200. The
target → `deleted`. The cloud service may continue to be listable for
10-60s due to Cloud Run eventual consistency.

**Operator-visible window:** Hitting "delete rollout" within seconds
after `deleted` is set succeeds. The cascade removes
deployment_targets, operations, history. The cloud service eventually
becomes NOT_FOUND but the AutoStack record is gone.

**Cost impact:** Cloud Run does NOT bill for a service in
deletion-pending state. So no real cost leak in this window.

**Trust impact:** The operator's mental model "I deleted it" is
slightly ahead of reality.

**Fix:** Dispatch destroy success branch polls `GetService` until
NOT_FOUND or timeout (60s) before flipping target to `deleted`. Adds
predictable latency.

**Status:** Deferred to Phase 2.5 — the trust gap is small, the cost
gap is nil. Documented.

### D-3: Cascade delete on `deployment_targets.cloud_account = false`

**Today:** `cloud_accounts → deployment_targets` is **not** cascade
delete (Phase 1.9 migration sets `cascadeDelete: false`). Deleting a
cloud_account leaves orphaned deployment_targets pointing at a missing
relation.

**Severity:** Low. PocketBase doesn't break on this; the relation is
just dangling. The reconciler will fail to JOIN cloud_accounts and
those targets are silently dropped from the reconcile query.

**Risk:** Cloud Run services still running, AutoStack no longer
manages them. Orphan.

**Operator workflow today:** Delete the rollout (which destroys its
targets via cascade) BEFORE deleting the cloud_account. The
`HandleRolloutDelete` orphan refusal helps: rollout-delete cascades
ONLY when targets are in `deleted` state, which means provider
resources are gone.

**Phase 2.5 work:** `HandleCloudAccountDelete` refusal if any target
rows reference this account with status != 'deleted'. Same defense
pattern.

### D-4: Hard-delete in PocketBase admin bypasses HandleRolloutDelete

**Today:** PocketBase admin UI can delete a rollout directly. The hook
runs `OnRecordBeforeDeleteRequest` (request scope), so admin UI's
"direct" delete goes through the same hook. ✓

**Verdict:** Defended.

But if an admin uses the underlying SQLite `DELETE FROM rollouts WHERE
id=...` query, the hook does NOT run. This is documented as
out-of-band; no defense.

### D-5: Provider.Destroy panic during DeleteService

**Today:** Dispatcher panic-recovery defer handles it. Target → error.
Cloud Run service state unknown. Operator must investigate manually
(check GCP console) and either re-trigger destroy (set endDate again on
a re-created rollout? No — endDate is one-way). Operator must use
PocketBase admin to flip status to `deleting` again.

**Severity:** Low. Panics in provider code are rare.

**Phase 2.5 work:** Add an operator-facing "force re-destroy" endpoint
that resets status to `deleting` without requiring rollout re-creation.
Deferred.

### D-6: Orphan-cleanup scanner not implemented

**Today:** No periodic background scan of provider resources tagged
`autostack-managed=true` that lack a backing target row.

**Severity:** Medium. Operator-error or process-crash during destroy
can produce orphans, especially before D-1 is fixed.

**Phase 2.5 work:** Scanner. Listed in [[../known-issues/deferred-operational-hardening]] Tier 1.

## Orphan-risk summary table

| Source of orphan | Today's defense | Phase 2.3 change |
|---|---|---|
| Hard-delete rollout with live target | `HandleRolloutDelete` 409 refusal | unchanged |
| Hard-delete cloud_account | None | unchanged |
| Mid-deploy endDate set | **`markCloudTargetForDestroy` silently defers — orphan risk** | **D-1 fix: pending_destroy column + re-arm on release** |
| Mid-destroy crash | sweep + target→error; operator must re-trigger | unchanged |
| Provider.Destroy succeeds API but cleanup fails server-side | None — AutoStack trusts the 200 | unchanged (D-2 deferred) |
| Operator deletes via SQLite directly | None | unchanged (out-of-band) |
| Cascade from cloud_account delete | None | unchanged |

## Phase 2.3 implementation in this area

- D-1: Implement `pending_destroy` column + dispatcher re-arm.
  This is the single highest-value safety fix in Phase 2.3.

## Deferred

- D-2 (post-destroy poll)
- D-3 (cloud_account delete refusal)
- D-5 (force re-destroy)
- D-6 (orphan scanner)

## Related
- [[ownership-integrity-review]]
- [[../known-issues/orphan-defense-policy]]
- [[../known-issues/deferred-operational-hardening]]
