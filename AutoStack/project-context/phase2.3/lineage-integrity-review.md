# Deployment Lineage Integrity Review — Phase 2.3

## Last Updated
2026-05-14

## The lineage contract

The `deployment_history` collection is the **append-only operator-facing
record of what happened on a target**. An operator with read access
should be able to reconstruct, from history alone:

1. When was a target's lifecycle initiated? By whom? Against what
   manifest?
2. What deploy attempts occurred, in what order, with what outcome?
3. Was there a rollback? Was it requested? Did it succeed?
4. Was there a destroy? Did it complete or fail?
5. Was the lifecycle interrupted (crash, sweep, panic)? When did it
   recover?

If the answer to any of these is "you also need to read the application
logs," lineage has failed.

## What gets written today

| Writer | Trigger | Action / Status | Notes |
|---|---|---|---|
| `dispatchDeploy` claim | CAS won | action=`created`/`updated`, status=`in_progress` | provider=`account.Provider` (which is `gcp`, not `gcp-cloudrun` — see Issue L-1 below) |
| `dispatchDeploy` outcome (success) | deployErr=nil, !stale | action=`created`/`updated`, status=`success` | external_id captured into `to_revision` |
| `dispatchDeploy` outcome (provider error result) | deployErr=nil, result.Status="error" | action=`created`/`updated`, status=`failed` | message=sanitized |
| `dispatchDeploy` outcome (hard error) | deployErr!=nil | action=`created`/`updated`, status=`failed` | external_id is NOT recorded — see Issue L-2 |
| `dispatchDeploy` outcome (stale) | rollout moved during deploy | action=`created`/`updated`, status=`failed`, msg="stale spec" | |
| `dispatchDeploy` panic recovery | panic during provider call | action=`created`/`updated`, status=`failed`, msg="dispatcher panic" | external_id not recorded |
| `dispatchDestroy` claim | CAS won | action=`deleted`, status=`in_progress` | |
| `dispatchDestroy` outcome (success) | Destroy returned nil | action=`deleted`, status=`success` | |
| `dispatchDestroy` outcome (error) | Destroy returned err | action=`deleted`, status=`failed` | |
| `dispatchDestroy` panic recovery | panic during Destroy | action=`deleted`, status=`failed`, msg="dispatcher panic" | |
| `SweepAbandonedOperations.writeAbandonHistory` | startup sweep finds in-progress op | action=`error`/`deleted`/`rolled_back` (mapped from op kind), status=`failed`, msg="abandoned" | |

## What does NOT get written today

| Event | Why it matters |
|---|---|
| Rollout create → `createPendingDeploymentTarget` | Operators have no record of WHEN the target was provisioned. First history row is the dispatcher's `in_progress`, written ~30s later. |
| Rollout respec → `flipCloudTargetsToPendingOnRespec` | If the operator updates the manifest, history shows "deploy in_progress" but not "user intent: re-deploy because spec changed". |
| Rollout endDate → `markCloudTargetForDestroy` | Same gap on the destroy side — history shows "deleted in_progress" but not "user intent: destroy because endDate was set". |
| CAS-race-loss | The losing dispatcher cancels its operation but does NOT write history. An operator looking at history would see one in_progress→success pair and no record that another goroutine briefly considered the same work. Acceptable but invisible. |
| Release-lost-ownership | When the dispatcher's release-CAS finds 0 rows affected (sweep moved on), we log `[RELEASE_LOST_OWNERSHIP]` but write no history row. The dispatcher's actual provider-call outcome (success? failure?) is lost from history. **This is a real lineage gap during sweep–dispatcher conflicts.** |

## Issues

### L-1: Inconsistent `provider` value in history rows

`writeHistory` is called with `account.Provider` (`gcp`, `aws`, `azure`)
but `deployment_targets.provider` is the canonical
(`gcp-cloudrun`, `aws-ecs`, `azure-aca`). Querying history by provider
returns mismatched values across collections.

**Fix:** Pass the target row's `provider` (or the canonicalized provider
name) into `writeHistory`. **Landing in Phase 2.3.**

### L-2: Lost `external_id` on hard error

When `Provider.Deploy` returns `(nil, err)` (typically pre-wait
CreateService/UpdateService failure), the dispatcher writes a history
row with empty `to_revision`. If the failure left a partial Cloud Run
service behind (GCP's CreateService is async — it can return error
after server-side resource was already allocated), the orphan link is
lost from lineage.

**Why this matters:** Orphan cleanup later needs to correlate
`autostack-managed=true` resources with history. Without the
externalID-on-error, the link is one-directional only.

**Fix:** None for Phase 2.3 — would require Cloud Run provider to
return a partial-result DeployResult on its CreateService error path.
Deferred to Phase 2.5; documented here.

### L-3: Missing intent-boundary history rows

Listed above. The dispatcher writes lineage starting at its own claim,
not at the operator's intent.

**Fix:** Add explicit history writes at:
- `createPendingDeploymentTarget` (action=`created`, status=`in_progress`,
  msg="target created, awaiting reconciler dispatch")
- `flipCloudTargetsToPendingOnRespec` (action=`updated`,
  status=`in_progress`, msg="spec changed; redispatch pending")
- `markCloudTargetForDestroy` (action=`deleted`, status=`in_progress`,
  msg="destroy intent recorded; reconciler will dispatch")

**Status:** Landing in Phase 2.3. Risk: PocketBase write failure here
would normally propagate — we wire history writes to be best-effort
(log on failure, do not block the controller).

### L-4: Cascade delete wipes history

`deployment_history.target` has `cascadeDelete: true`. When a target row
is deleted (which only happens through PocketBase admin, since
HandleRolloutDelete refuses delete-with-live-targets), all its history
is removed.

**Why this matters:** Post-mortem reconstruction across rollout
lifecycles relies on history persisting beyond the target's lifetime.

**Fix:** Schema change — set `cascadeDelete: false` on
`deployment_history.target`. Operators would see "orphan" history rows
(target relation broken), but that's the correct trade — history is
immutable, target is not.

**Status:** Deferred to Phase 2.5. Cannot change schema without migration
+ touching existing rows. Documented as deferred.

### L-5: `succeeded_stale` history shows `status=failed`

The dispatcher writes a stale-spec history row with `status=failed` and
message="stale spec". The corresponding operations row has
`status=succeeded_stale`. An operator reading history alone sees
"failed" — not "completed but for an outdated spec".

**Decision:** Keep `status=failed` in history. `succeeded_stale` is not
a valid `deployment_history.status` enum value (the enum is
`{success, failed, in_progress}`). Extending the enum is a schema
change. The honest interpretation of "failed because of stale spec" is
acceptable for operators, and the message disambiguates.

**Long-term:** Add `stale` as a history status value when the schema is
next migrated. Deferred.

### L-6: No operation_id on history rows

`deployment_history` has no `operation` column. Reconstructing
"which history rows correspond to which operation" requires correlating
by (target, action, timestamp). Brittle.

**Fix:** Schema change to add `deployment_history.operation` (relation).
Deferred to Phase 2.5 because every new history-write call site would
have to be updated.

### L-7: `writeHistory` failures are silent

```go
if err := r.app.Dao().SaveRecord(rec); err != nil {
    log.Printf("[HISTORY_WRITE_ERR] save: %v", err)
    return
}
```

A PocketBase transient write failure here permanently loses an outcome
row. The actual deploy / destroy outcome is correct on disk
(deployment_targets state is the source of truth), but lineage is
incomplete.

**Mitigation today:** Tolerate. History is for forensics, not for
correctness; a missed row is a degraded forensic surface, not a wrong
control-plane decision.

**Phase 2.5 work:** Persistent retry queue for history writes, OR
make history writes transactional with the state change.

## Phase 2.3 implementation in this area

- Fix L-1 (provider value consistency).
- Fix L-3 (intent-boundary history writes).
- Document L-2, L-4, L-5, L-6, L-7 as known limitations.

## Related
- [[truthful-state-assessment]]
- [[incident-reconstruction-assessment]]
- [[ownership-integrity-review]]
- [[../reconciler/deploy-dispatch-design]]
