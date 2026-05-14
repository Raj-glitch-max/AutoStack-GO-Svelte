# Deploy Dispatch — Phase 2.0 Design

## Last Updated
2026-05-14

## What this document is

The design rationale for the Phase 2.0 cloud deploy execution path. The
high-level diagram from the directive was:

```
PocketBase Desired State
  → Reconciler
    → Provider Dispatch
      → Provider Operation
        → Operation Tracking
          → Status Normalization
            → Deployment History
              → WebSocket Events
```

This document records how AutoStack realizes the first six layers. The
seventh (WebSocket events for cloud lifecycle) is out of scope for Phase 2.0.

## Decision: reconciler dispatches via CAS-claimed operations

Three shapes were considered:

| Shape | Verdict |
|---|---|
| Sync in HTTP handler | Rejected: 5-10 min request blocking; mid-deploy crash unrecoverable. |
| **Reconciler dispatches via operations table + CAS** | **Chosen.** Restart-safe; multi-pod safe; ≤30s dispatch latency. |
| Separate dispatcher goroutine | Rejected: two loops with two failure modes, no advantage for current scale. |

## Schema additions

### `operations` collection (`pb_migrations/1715300004_created_operations.js`)

| Field | Type | Notes |
|---|---|---|
| `id` | string PK | PocketBase-generated |
| `target` | relation → deployment_targets | required; cascade delete |
| `kind` | select | deploy / rollback / destroy |
| `status` | select | pending / in_progress / succeeded / **succeeded_stale** / failed / cancelled |
| `started_at` | date | when CAS won |
| `updated_at` | date | drives crash-recovery sweep threshold |
| `message` | text | sanitized error or success note |
| `provider_operation_id` | text | reserved for future real LRO support |
| `rollout_revision` | text | rollout `updated` timestamp at claim; used for stale-spec detection |

`succeeded_stale` is a deliberate addition: it lets the dispatcher record
"the Deploy completed but the desired state moved underneath us" without
either lying (claiming success of stale work) or failing (the deploy
itself was technically successful).

`deleteRule` is `null` — operations are append-only and immutable from
the client API. The reconciler updates `status`/`updated_at` via the DAO,
which bypasses the rule check.

### `deployment_targets` additions (`pb_migrations/1715300005_*`)

| Field | Type | Notes |
|---|---|---|
| `current_operation` | relation → operations | NULL when idle; populated by CAS claim |
| `current_revision` | text | provider-side revision id of last successful deploy |
| `last_state_change_at` | date | timestamp of most recent status transition |

## The CAS claim

The load-bearing safety check. SQL form:

```sql
UPDATE deployment_targets
   SET current_operation = ?,
       last_state_change_at = ?,
       status = CASE WHEN status = 'pending' THEN 'creating'
                     WHEN status = 'deleting' THEN 'deleting'
                     ELSE status END
 WHERE id = ?
   AND (current_operation = '' OR current_operation IS NULL)
   AND status IN ('pending', 'deleting');
```

If `RowsAffected() = 0`, another reconciler won. The losing dispatcher
marks its operation as `cancelled` and returns `reconcileSkipped`.

Correctness under SQLite: the WAL write lock serializes statements, so
this is a real CAS, not just a "fast read then write." Under a future
Postgres migration the same idiom holds — UPDATE...WHERE with predicate
constraints is row-level atomic.

## Dispatch flow

```
reconciler tick
  → SELECT row (status=pending, current_operation NULL)
  → buildDeploySpec(manifest) — permanent failure path if YAML invalid
  → createOperation(target, "deploy", rollout_revision)   // opens op row
  → claimTarget(target, opID) — atomic CAS
      ├─ race lost → cancelOperation; reconcileSkipped
      └─ won → log [DISPATCH_CLAIM]; writeHistory(action=created, status=in_progress)
  → ctx = WithTimeout(15min)
  → log [DEPLOY_START]
  → Provider.Deploy(ctx, account, spec)
  → log [DEPLOY_END]
  → stale = rolloutMovedSince(rolloutID, rollout_revision)
  → branch outcome:
      ├─ err != nil      → op=failed,         history=failed,  release(pending,error)
      ├─ result.Status="error" → op=failed,   history=failed,  release(pending,error)
      ├─ stale           → op=succeeded_stale, history=failed, release(pending,pending)
      └─ ok              → op=succeeded,      history=success, release(pending,updating)
```

The success path persists `updating` (not `running`). The regular
GetStatus poller then promotes `updating → running` on the next tick,
defending against Cloud Run's transient `Ready=SUCCEEDED` flap.

## Destroy dispatch

Symmetric to deploy:

- Trigger: target.status = `deleting` and current_operation NULL.
- Source: `controller.markCloudTargetForDestroy` (called when rollout
  `endDate` is set on a cloud-target rollout).
- Idempotent under provider NOT_FOUND (see provider Destroy semantics).
- Success → target.status = `deleted`.
- Failure → target.status = `error`.

## Stale-spec detection

Every operation is stamped with `rollout_revision = rollout.updated` at
claim time. After Deploy returns, the dispatcher re-reads
`rollout.updated`. If it advanced, the deploy ran against a now-stale
spec.

Two reasons to refuse to claim success:
1. The newer spec might differ materially.
2. The provider state we reached is not what the user requested.

The honest move: mark the op `succeeded_stale`, release the target back
to `pending`, write a failed history entry. Next cycle re-dispatches.

## Crash recovery

Implemented in `sweep.go`:

- Runs synchronously in `OnAfterBootstrap`, BEFORE the reconciler ticker
  starts.
- Finds all `operations` rows with `status=in_progress` AND
  `updated_at < now - 20min` (threshold > DeployTimeout).
- For each: mark op `failed` with message "abandoned: process restart
  while in flight"; if the target's `current_operation` still points at
  this op, release it to `status=error`.

We never *infer* success from row state. An abandoned operation could
have left a partial cloud-side resource; the operator must take
explicit recovery action.

## Hazards explicitly accepted (Phase 2.0)

1. **Mid-Deploy rollout-delete:** PocketBase cascade-deletes the target
   row while the provider call is still running. The provider may
   create a service we no longer track. Orphan. Tier-2 cleanup deferred.
   Logged as `[CLOUD_ORPHAN_RISK]` from `HandleRolloutDelete`.

2. **Rollout-spec update propagation:** Phase 2.0 does NOT propagate
   image / env / scale changes from a rollout update onto an
   already-deployed cloud target. Logged as `[CLOUD_UPDATE_DEFERRED]`.
   Operator must end the rollout and re-create.

3. **Single-region credential validation:** Validation uses
   `locations/-` so region-scoped permission gaps surface only at
   dispatch time (failure category becomes `auth`, target → `error`,
   no retry). Phase 2.5 should add region-targeted validation.

4. **Stale dispatcher process:** if a stale process keeps a long deploy
   running past the 20-min sweep threshold, the sweep will mark its op
   abandoned. The stale process's later release will then write a
   conflicting status. Acceptable for single-pod operation; Phase 2.5
   needs a "release-only-if-still-owner" guard.

## Related
- [[operation-ownership]]
- [[encryption-design]]
- [[lifecycle-assumptions]]
- [[restart-behavior]]
- [[rollback-semantics]]
- [[deferred-operational-hardening]]
