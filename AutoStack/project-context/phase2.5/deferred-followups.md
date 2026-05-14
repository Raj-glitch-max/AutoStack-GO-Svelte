# Deferred Phase 2.5 Follow-ups

## Last Updated
2026-05-14

## Items deferred FROM Phase 2.5 (originally Phase 2.4 backlog)

These were scoped to Phase 2.5 in the Phase 2.4 deferred list but are
NOT landing in 2.5. They move to Phase 2.6 or later.

| # | Title | New target phase | Rationale |
|---|---|---|---|
| 2.5-4 | Orphan-cleanup scanner | Phase 2.6 | Requires provider-side tag scan; needs design for safe-vs-unsafe orphan classification. |
| 2.5-5 | Per-cloud-account backoff | Phase 2.6 | Schema work + circuit-breaker refactor; not blocking 2.5 hygiene. |
| 2.5-6 | Worker pool for dispatch | Phase 2.6 | Substantial refactor; 2.5 cleanup gives the immediate operational hygiene win. |
| 2.5-7 | `operations.cycle_id` column | Phase 2.6 | Schema migration; doesn't block 2.5 cleanup. |
| 2.5-8 | `deployment_history.operation` FK | Phase 2.6 | Schema migration. |
| 2.5-9 | `deployment_history.target` cascadeDelete=false | Phase 2.6 | Schema migration; needs migration plan for existing rows. |
| 2.5-10 | `deployment_history.status` enum add `stale` | Phase 2.6 | Schema migration. |
| 2.5-11 | Rollout retention | Phase 2.6 | Compound work with history cascade fix (2.5-9). |
| 2.5-12 | HandleCloudAccountDelete refusal | Phase 2.6 | Controller work; not in cleanup scope. |
| 2.5-13 | Encrypt-everything migration + remove plaintext fallback | Phase 2.7 | Security work; needs careful migration plan. |
| 2.5-14 | Pod-identity stamping | Phase 2.7 | Multi-pod work; not in 2.5 scope. |
| 2.5-15 | Cloud Run serving_revision | Phase 2.7 | Provider work. |
| 2.5-16 | Cloud Run create-vs-update transient-retry | Phase 2.7 | Provider work. |
| 2.5-17 | Cloud_account region change refusal | Phase 2.7 | Controller work. |

## What Phase 2.5 DOES land

- Operation TTL janitor.
- Deployment-history TTL.
- `[CLEANUP_OPS]`/`[CLEANUP_HISTORY]` log emissions.
- Configurable retention windows via env vars.
- `AUTOSTACK_CLEANUP_ENABLED` master switch.

## Forward-fit

The cleanup architecture (separate goroutine, daily cadence, FK-guard
predicate) is forward-fit for:
- Orphan-cleanup scanner (2.6-X): adds a new pass at the same cadence.
- Rollout retention (2.6-X): adds another pass.
- Per-tenant retention (Phase 3): subclassed CleanupConfig per tenant.

## Related
- [[../phase2.4/deferred-phase2.5-concerns]]
- [[operation-cleanup-design]]
