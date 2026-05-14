# Deferred Phase 2.6 Follow-ups

## Last Updated
2026-05-14

## Carried forward to Phase 2.7

| # | Title | Rationale |
|---|---|---|
| 2.5-4 | Orphan-cleanup scanner | Phase 2.7 |
| 2.5-5 | Per-cloud-account backoff | Phase 2.7 |
| 2.5-6 | Worker pool for dispatch | Phase 2.7 (or Phase 3) |
| 2.5-7 | `operations.cycle_id` column | Phase 2.7 |
| 2.5-8 to 2.5-11 | Schema migrations (history FK, cascade, enum) | Phase 2.7 |
| 2.5-12 | HandleCloudAccountDelete refusal | Phase 2.7 |
| 2.5-13 | Encrypt-everything migration | Phase 2.8 |
| 2.5-14 | Pod-identity stamping | Phase 2.7 — unblocks multi-pod runtime sweep |
| 2.5-15 | Cloud Run serving_revision | Phase 2.8 (drift) |
| 2.5-16 | Cloud Run create-vs-update transient-retry | Phase 2.8 |
| 2.5-17 | Cloud_account region change refusal | Phase 2.7 |

## What Phase 2.6 DOES land

- Succeeded-stale loop circuit (threshold=3).
- Runtime sweep goroutine (every 5 min, 5-min stale-age cutoff).
- Stale-count clearing on success/failure/operator-recovery.

## Related
- [[../phase2.5/deferred-followups]]
- [[succeeded-stale-guard]]
- [[runtime-sweep-design]]
