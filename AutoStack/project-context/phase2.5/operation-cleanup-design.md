# Operation Cleanup Design — Phase 2.5

## Last Updated
2026-05-14

## Design

A single goroutine, `runCleanup`, ticks at `AUTOSTACK_CLEANUP_INTERVAL_HOURS`
(default 24h). Each tick:

1. Build cutoff timestamps per status × retention policy.
2. For `operations`:
   - Find terminal rows older than cutoff AND not referenced by any
     target's `current_operation`.
   - DELETE in batches (1000 rows max per pass to keep transactions
     short).
3. For `deployment_history`:
   - Find rows older than the per-status retention.
   - DELETE in batches.
4. Log `[CLEANUP_OPS]` and `[CLEANUP_HISTORY]` with counts.

## SQL

### Operations cleanup

```sql
DELETE FROM operations
WHERE status IN ('succeeded', 'succeeded_stale')
  AND updated_at < :cutoff_success
  AND id NOT IN (
    SELECT current_operation FROM deployment_targets
    WHERE current_operation IS NOT NULL AND current_operation != ''
  )
```

Same predicate template for `failed` and `cancelled` with their own
cutoffs.

### History cleanup

```sql
DELETE FROM deployment_history
WHERE status = 'success' AND created < :cutoff_success;

DELETE FROM deployment_history
WHERE status = 'failed' AND created < :cutoff_failed;

DELETE FROM deployment_history
WHERE status = 'in_progress' AND created < :cutoff_in_progress;
```

`in_progress` history rows that survived their corresponding terminal
row's cleanup (orphaned mid-state) are cleaned with a longer retention
(same as `failed`, 365d default).

## Concurrency

- Cleanup is a single goroutine. No locking required against itself.
- Reconciler dispatcher writes to operations/history concurrently;
  SQLite WAL serializes writes. DELETE statements may block briefly on
  the WAL lock but don't conflict semantically.
- The FK-guard subquery prevents deleting an op being claimed; the
  subquery's result is consistent within the DELETE statement under WAL.

## Idempotency

DELETE is naturally idempotent. Re-running cleanup against an
already-cleaned set deletes zero rows.

## Configuration

| Env var | Default | Meaning |
|---|---|---|
| `AUTOSTACK_OPS_RETENTION_SUCCESS_DAYS` | 30 | TTL for succeeded/succeeded_stale ops |
| `AUTOSTACK_OPS_RETENTION_FAILED_DAYS` | 90 | TTL for failed ops |
| `AUTOSTACK_OPS_RETENTION_CANCELLED_DAYS` | 7 | TTL for cancelled ops |
| `AUTOSTACK_HISTORY_RETENTION_SUCCESS_DAYS` | 90 | TTL for success history |
| `AUTOSTACK_HISTORY_RETENTION_FAILED_DAYS` | 365 | TTL for failed history |
| `AUTOSTACK_HISTORY_RETENTION_IN_PROGRESS_DAYS` | 365 | TTL for orphaned in_progress history |
| `AUTOSTACK_CLEANUP_INTERVAL_HOURS` | 24 | Cleanup cadence |
| `AUTOSTACK_CLEANUP_ENABLED` | true | Master switch |

## Failure modes

- **DB busy:** DELETE batch fails. Log warning; the next pass tries
  again. Cleanup is best-effort.
- **Process crash mid-cleanup:** The DELETE is committed per batch.
  Partial progress survives.
- **Operator pauses cleanup mid-investigation:** Set
  `AUTOSTACK_CLEANUP_ENABLED=false`, restart. No cleanup runs.

## Boot integration

`StartCleanupOnBoot(app)` is registered in `main.go` next to the
reconciler. Reads config, starts the goroutine, logs startup state
including configured retention windows and enabled flag.

## Related
- [[retention-policy]]
- [[cleanup-safety-analysis]]
