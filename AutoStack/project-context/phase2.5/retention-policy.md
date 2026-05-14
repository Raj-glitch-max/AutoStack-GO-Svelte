# Retention Policy — Phase 2.5

## Last Updated
2026-05-14

## Defaults

| Collection | Status | Retention | Rationale |
|---|---|---|---|
| operations | succeeded / succeeded_stale | 30d | Forensic value drops fast; ops with success outcome have a matching history success row that retains 90d |
| operations | failed | 90d | More forensic value (post-mortem) |
| operations | cancelled | 7d | Race-loss artifacts |
| operations | in_progress | indefinite | Caught by startup sweep; never aged out |
| deployment_history | success | 90d | Operator-visible deploy log |
| deployment_history | failed | 365d | Compliance / incident review |
| deployment_history | in_progress (orphaned) | 365d | Mid-state that survived cleanup of its paired terminal row; preserve same as failed |

## Operator overrides

All defaults are env-var configurable. Common overrides:

- **Compliance hold:** set `AUTOSTACK_CLEANUP_ENABLED=false`. Nothing is
  deleted; data grows. Operator must externally archive.
- **Aggressive cleanup (dev):** set retention env vars to small values.
- **Long forensic windows:** double the defaults.

## Forensic guarantees

After cleanup runs against the defaults:
- The most recent 30 days of every target's deploy history is intact
  (operations + history rows).
- Every failed operation older than 30d is gone; its corresponding
  failed-history row remains for up to a year.
- 90+ days back, only failed/in_progress history rows survive.
- 365+ days back, nothing remains except indefinite-retention
  in_progress ops (which shouldn't exist post-sweep).

## What the policy does NOT do

- It does not preserve "incident snapshots" — there's no concept of
  pinning a chunk of history during investigation. Operators in an
  investigation should disable cleanup or extend retention.
- It does not preserve cloud-provider revision history — Cloud Run
  manages its own revision GC.
- It does not preserve provider audit logs — those live in GCP / AWS /
  Azure-side audit services.

## Validation

After implementation:
- Run with default retention.
- Verify a target's lineage 6 months later still has at least one
  history row.
- Verify operations table size stays bounded.

## Related
- [[operation-cleanup-design]]
- [[cleanup-safety-analysis]]
