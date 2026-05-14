# Operation Retention & TTL Proposal — Phase 2.4

## Last Updated
2026-05-14

## Problem

The `operations` table grows forever. The `deployment_history` table
grows forever. There is no archival, no retention policy, no cleanup.

At 1 deploy/day per target, 365 ops/year/target. At 100 targets,
36500/year — fine in SQLite. At 10000 targets or 10 deploys/day,
hundreds of thousands per year. SQLite handles it, but query latency
on indexed scans degrades.

This is **operational entropy** — not a correctness issue today, but a
long-running-system tax.

## Proposed retention policy

### Operations table

| Status | Retention | Rationale |
|---|---|---|
| `in_progress` | indefinite (caught by sweep) | Live or abandoned; sweep handles. |
| `succeeded` | 30 days | Forensic value drops sharply after a few weeks. |
| `succeeded_stale` | 30 days | Same as succeeded. |
| `failed` | 90 days | More forensic value (post-mortem use). |
| `cancelled` | 7 days | Low forensic value (race-loss). |

### Deployment_history table

| Status | Retention | Rationale |
|---|---|---|
| `in_progress` superseded | match terminal sibling | Pair with the corresponding outcome row. |
| `success` | 90 days | Operator-visible deploy log. |
| `failed` | indefinite (or 365 days) | Compliance / incident-review value. |
| Other (rolled_back, recovered, error) | 365 days | Compliance. |

### Cleanup ordering (replay safety)

The cleanup MUST NOT:
- Remove an operation whose target is in error referencing it (operator
  investigating).
- Remove history rows that are part of an active incident
  investigation (no way to detect; we accept this trade).
- Cascade-delete child records of removed ops (operations has no
  cascading children today; deployment_history.operation FK is
  deferred to Phase 2.5, after which we'd need cascade=false).

**Order:**
1. Delete `operations` rows in terminal status older than retention.
2. **Do NOT delete deployment_history yet.** History is the lineage
   record; preserve longer than operations.
3. Delete `deployment_history` rows older than their retention window,
   in batches.

### Replay-safety considerations

The Phase 2.3 startup sweep queries `operations WHERE status =
in_progress`. Cleanup of terminal ops cannot affect the sweep (the
sweep only looks at in-progress rows). ✓

The reconciler queries `deployment_targets` (joined with rollouts and
cloud_accounts). It does NOT depend on the `operations` table for
behavior — operations is purely audit/recovery. Cleanup is safe. ✓

The `current_operation` foreign key on `deployment_targets` could point
at a deleted op row. PocketBase relations don't enforce referential
integrity; the orphaned relation appears as an unmatched ID. **Concern:**
the FK becomes a dangling reference if the op is cleaned while the
target still references it. But this only happens if cleanup deletes a
terminal op while the target's `current_operation` still points at it —
which is only possible if the dispatcher's release-CAS failed to clear
the field (sweep edge case). The startup sweep would correctly find no
in-progress op and not touch the target; the target's stale
`current_operation` reference is already broken in this corner case.

**Cleanup safety predicate:** Only delete operations where no
`deployment_targets.current_operation = ops.id` exists. Practically, a
left-join check OR a subquery.

```sql
DELETE FROM operations
WHERE status IN ('succeeded', 'succeeded_stale', 'failed', 'cancelled')
  AND updated_at < :cutoff
  AND id NOT IN (
    SELECT current_operation FROM deployment_targets
    WHERE current_operation IS NOT NULL AND current_operation != ''
  )
```

This guards against the dangling-FK edge case at cost of one more
SELECT per cleanup batch.

### Cleanup scheduling

- Periodic goroutine, separate from the reconciler tick.
- Frequency: daily (24 hour interval) — entropy is slow, no need for
  faster cadence.
- Single-pod: synchronous within the goroutine. Multi-pod: future work
  needs leader election; for now, accept that two pods could run
  cleanup simultaneously (DELETE is idempotent).

### Cleanup visibility

Every cleanup pass logs:
- `[CLEANUP_OPS] deleted=N retention=Xd cutoff=<ts>`
- `[CLEANUP_HISTORY] deleted=N retention=Xd cutoff=<ts>`
- `[CLEANUP_SKIP] reason=fk_still_referenced count=N` (rare)

Failures are logged but do not propagate. Cleanup is best-effort
maintenance.

## Configuration

| Env var | Default | Meaning |
|---|---|---|
| `AUTOSTACK_OPS_RETENTION_SUCCESS_DAYS` | 30 | TTL for succeeded ops |
| `AUTOSTACK_OPS_RETENTION_FAILED_DAYS` | 90 | TTL for failed ops |
| `AUTOSTACK_OPS_RETENTION_CANCELLED_DAYS` | 7 | TTL for cancelled ops |
| `AUTOSTACK_HISTORY_RETENTION_SUCCESS_DAYS` | 90 | TTL for success history |
| `AUTOSTACK_HISTORY_RETENTION_FAILED_DAYS` | 365 | TTL for failed history |
| `AUTOSTACK_CLEANUP_INTERVAL_HOURS` | 24 | Cleanup goroutine cadence |
| `AUTOSTACK_CLEANUP_ENABLED` | true | Master switch to disable cleanup |

Operators can disable cleanup entirely via the master switch — useful
for compliance contexts that require indefinite retention.

## What this proposal does NOT include

- **No archive table.** Cleanup is delete-and-forget. If forensic
  preservation is required, operators must periodically dump the
  tables externally.
- **No incident-investigation override.** A persistent "hold this op"
  flag could be added but is deferred. Today: extend the retention
  window globally if an investigation is active.
- **No compliance-grade audit trail.** This is a maintenance system,
  not an audit system. Compliance work (Phase 3) requires a separate
  immutable WORM-style audit log.

## Implementation plan (Phase 2.5)

1. Add a `cleanup.go` file in `pkg/reconciler/` with:
   - `StartCleanupOnBoot(app *pb.PocketBase)` registered alongside
     `StartReconcilerOnBoot`.
   - Daily ticker.
   - Batched DELETE statements with the FK-guard predicate.
   - Structured log emissions.
2. Add env vars to `pkg/env/`.
3. Document in `current-state.md` and CLAUDE.md.

## Hazards explicitly considered

1. **Operator deletes a target row that has terminal ops.** Cascade
   delete on `operations.target = ...` is `cascadeDelete: true`. So
   removing a target removes its ops automatically. Cleanup doesn't
   need to handle this. ✓
2. **Cleanup races with a live deploy.** Cleanup only deletes terminal
   ops; the dispatcher writes in_progress. No interference. ✓
3. **Cleanup deletes the most recent op.** If a target had its last
   successful op cleaned, `last_successful_op_id` (if we had it; we
   don't) becomes dangling. We do not store such a FK; the most recent
   successful op is found by joining on target+status+order-by-time.
   Cleaning recent ops would break that join. **Mitigation:** retention
   windows are conservative (30 days for success). Investigations within
   30 days have all the data. After 30 days, "most recent op" is the
   most recent post-cleanup op, which is the right answer for steady-
   state operation. ✓

## Status

Proposal accepted. Implementation lands in Phase 2.5 — see
[[../phase2.5/operation-cleanup-design.md]] (to be written) and
the cleanup goroutine in `pkg/reconciler/cleanup.go`.

## Related
- [[../phase2.3/lineage-integrity-review]]
- [[operational-entropy-assessment]]
