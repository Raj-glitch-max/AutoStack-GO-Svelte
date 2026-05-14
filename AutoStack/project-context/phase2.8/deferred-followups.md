# Deferred Phase 2.8 Follow-ups

## Last Updated
2026-05-14

## Carried forward to Phase 2.9 / Phase 3

| # | Title | Target | Rationale |
|---|---|---|---|
| Spec-vs-actual drift detection | Phase 3 | Large feature: snapshot + diff + UX. |
| Cloud Run `serving_revision` field | Phase 3 | Provider extension + schema. |
| Cloud Run create-vs-update transient retry | Phase 3 | Provider work. |
| Region-scoped credential validation | Phase 3 | Provider work. |
| Per-cloud-account backoff / circuit | Phase 3 | Schema + refactor. |
| HandleCloudAccountDelete refusal | Phase 2.9 | Controller work. |
| Cloud_account region change refusal | Phase 2.9 | Controller work. |
| Encrypt-everything migration | Phase 2.9 | Security + careful migration plan. |
| Stuck-state detector goroutine | Phase 2.9 | Goroutine architecture. |
| Pod-identity stamping | Phase 2.9 | Multi-pod blocker. |
| `operations.cycle_id` column | Phase 2.9 | Migration. |
| `deployment_history.operation` FK | Phase 2.9 | Migration. |
| `deployment_history.target` cascadeDelete=false | Phase 2.9 | Migration. |
| `deployment_history.status` enum add `stale` | Phase 2.9 | Migration. |
| Worker pool for dispatch | Phase 3 | Not blocking trust. |
| `log/slog` adoption | Phase 2.9 decision | Refactor. |

## What Phase 2.8 DOES land

- Post-destroy NOT_FOUND confirmation poll in Cloud Run provider.

## Related
- [[../phase2.7/deferred-followups]]
