# Cleanup Safety Analysis — Phase 2.5

## Last Updated
2026-05-14

## Replay safety

The Phase 2.3 startup sweep reads:
```sql
SELECT id, target, kind, started_at, updated_at
FROM operations
WHERE status = 'in_progress'
```

Cleanup never touches `in_progress` rows. **Sweep is unaffected.** ✓

The reconciler's main SELECT reads `deployment_targets` joined with
`rollouts` and `cloud_accounts`. It does NOT read `operations` for
behavior — `operations` is audit-only. **Reconciler is unaffected.** ✓

The dispatcher reads `operations` only via `completeOperation` and
`heartbeat`, both of which use `WHERE id = :op AND status = 'in_progress'`.
Cleanup of terminal rows cannot affect either query. **Dispatcher
unaffected.** ✓

## FK-guard correctness

The cleanup query excludes any op currently referenced by a target's
`current_operation`:

```sql
DELETE FROM operations
WHERE status IN ('succeeded', ...) AND updated_at < :cutoff
  AND id NOT IN (
    SELECT current_operation FROM deployment_targets
    WHERE current_operation IS NOT NULL AND current_operation != ''
  )
```

Why this matters: a target row's `current_operation` should always
point at an `in_progress` op (post-release would have cleared it). But
edge cases:

- Dispatcher panic before `releaseTarget` ran: `current_operation` set,
  op marked `failed` by panic-recovery. Target's `current_operation`
  references a `failed` op. Cleanup of that op would dangle the FK.
  **FK-guard prevents this.** ✓
- Sweep marked op `failed` but couldn't clear target's
  `current_operation` (target.find error or save error logged at
  sweep time): same as above. **FK-guard prevents this.** ✓

## Lineage preservation

History rows survive operations cleanup by separate policy. A target's
"first deploy" history row (in_progress) and "first deploy succeeded"
history row both retain at the configured retention (90d for success).
After 90d, the success rows are cleaned; failed/in_progress rows
remain for 365d.

After cleanup, the joined view "show me this target's deploys" looks
like:
- Most recent 30 days: full ops + history.
- 30-90 days: history only (success/failed) + no ops.
- 90-365 days: failed history only.
- > 365d: empty.

**Operators can still answer "what was the most recent deploy of this
target" within the 90d window.** ✓

## Race conditions

### R-1: Cleanup runs while a deploy completes

**Timeline:**
- T+0: dispatcher's `completeOperation` writes `status=succeeded` on op X.
- T+1ms: cleanup goroutine's DELETE pass starts. It reads ops with
  `updated_at < cutoff` (cutoff = now - 30d).
- T+10ms: cleanup's DELETE runs.

Op X has `updated_at = T+0` and would not be included in the DELETE
(cutoff is 30 days ago). ✓

### R-2: Cleanup runs while a stale-op is being swept

The Phase 2.3 sweep only runs at startup, not concurrent with the
cleanup goroutine. But suppose the runtime sweep (Phase 2.6) runs:

- Runtime sweep marks op `failed` due to stale heartbeat.
- Cleanup's NEXT pass (24h later) would find this op `failed` and
  potentially eligible if older than 90d. But the failed → cleanup
  cycle requires 90 days minimum. No race. ✓

### R-3: Cleanup runs while operator manually edits ops

If an admin modifies an op row (e.g., via PocketBase admin UI) and
cleanup happens to delete that row in the same tick — last writer
wins. The admin would see "row vanished". Acceptable: admin manual
edits are out-of-band.

## Failure isolation

Cleanup failures (DB busy, disk full) are logged with
`[CLEANUP_FAILED]` and the goroutine continues. The next tick (24h
later) retries. A persistent failure does NOT block reconciliation.

## Disable safety

Setting `AUTOSTACK_CLEANUP_ENABLED=false`:
- Cleanup goroutine never starts.
- All other behavior unchanged.
- No retention applied — data grows.

Operator must remember to re-enable, OR external systems must monitor
growth. This is documented in retention-policy.md.

## Risks NOT mitigated

- **No backup-before-delete.** Cleanup is destructive. Compliance
  contexts requiring "always have a recoverable copy" must externally
  back up before cleanup runs.
- **No partial-result preservation.** If a single batch DELETE removes
  100 rows including a forensically-important one, that row is gone.
  Mitigation: extend retention or disable.
- **No tenant-aware retention.** All tenants share the same retention
  policy. Phase 3 multi-tenancy work needs per-tenant overrides.

## Related
- [[operation-cleanup-design]]
- [[retention-policy]]
