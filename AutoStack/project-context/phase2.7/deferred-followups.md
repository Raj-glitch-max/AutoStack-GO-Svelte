# Deferred Phase 2.7 Follow-ups

## Last Updated
2026-05-14

## Items carried to Phase 2.8

| # | Title | Rationale |
|---|---|---|
| Orphan-cleanup scanner | Phase 2.8 (drift work) | Conceptually a drift-class problem; bundled there. |
| Per-cloud-account backoff | Phase 2.8 | Schema work bundled with provider-validation hardening. |
| Worker pool for dispatch | Phase 2.9 / Phase 3 | Not blocking trust; performance optimization. |
| `operations.cycle_id` column | Phase 2.9 (bundled with pod-identity stamping) | Schema migration churn deferred. |
| `deployment_history.operation` FK | Phase 2.9 | Schema migration. |
| `deployment_history.target` cascadeDelete=false | Phase 2.9 | Migration. |
| `deployment_history.status` enum add `stale` | Phase 2.9 | Migration. |
| Pod-identity stamping | Phase 2.9 (or Phase 3) | Multi-pod blocker; sized as 2.9 work. |
| HandleCloudAccountDelete refusal | Phase 2.8 | Controller work. |
| Cloud_account region change refusal | Phase 2.8 | Controller work. |
| Encrypt-everything migration | Phase 2.8 | Security work. |

## What Phase 2.7 DOES land

- History row on `[RELEASE_LOST_OWNERSHIP]` (forensic completeness).
- History row on CAS-race cancel (forensic completeness).
- `[HEARTBEAT_FAIL_PERSISTENT]` escalation after 5 consecutive failures.
- `[CYCLE_BACKED_OFF]` debug emission.

## What's documented but deferred

- `log/slog` adoption proposal — see [[structured-logging-proposal]].
  Deferred to Phase 2.9 / Phase 3 audit decision.

## Related
- [[../phase2.6/deferred-followups]]
