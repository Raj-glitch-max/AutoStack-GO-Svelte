# Deferred Phase 2.5+ Concerns — Phase 2.4 Update

## Last Updated
2026-05-14

This document is the Phase 2.4 update to the running Phase 2.5+ backlog.
Phase 2.3 introduced a similar list; this expands it with Phase 2.4
findings.

## Phase 2.5 — Operational Hygiene (next milestone)

| # | Title | Source |
|---|---|---|
| 2.5-1 | Operation TTL janitor (succeeded:30d / failed:90d / cancelled:7d) | [[operation-retention-ttl-proposal]] |
| 2.5-2 | Deployment-history TTL (success:90d / failed:365d) | same |
| 2.5-3 | `[CLEANUP_OPS]`/`[CLEANUP_HISTORY]` log emissions | same |
| 2.5-4 | Orphan-cleanup scanner (provider-side, `autostack-managed=true` tag) | [[../phase2.3/deferred-phase2.5-concerns]] §20 |
| 2.5-5 | Per-cloud-account backoff / circuit-breaker | [[retry-amplification-review]] A-1, A-2 |
| 2.5-6 | Worker-pool with bounded concurrency for dispatch staggering | [[retry-amplification-review]] A-3 |
| 2.5-7 | `operations.cycle_id` column for on-disk correlation | [[incident-reconstruction-maturity-review]] IR-3 |
| 2.5-8 | `deployment_history.operation` FK | [[../phase2.3/lineage-integrity-review]] L-6 |
| 2.5-9 | `deployment_history.target` cascadeDelete=false (preserve history beyond target lifetime) | [[../phase2.3/lineage-integrity-review]] L-4 |
| 2.5-10 | `deployment_history.status` enum extension: add `stale` | [[../phase2.3/lineage-integrity-review]] L-5 |
| 2.5-11 | Rollout retention (delete ended rollouts older than N days; preserve history) | [[operational-entropy-assessment]] E-7 |
| 2.5-12 | `HandleCloudAccountDelete` refusal if live targets reference | [[../phase2.3/delete-orphan-risk-assessment]] D-3 |
| 2.5-13 | One-shot migration: re-encrypt legacy plaintext + remove plaintext fallback in `Decrypt` | [[../phase2.3/encryption-integrity-assessment]] EI-3 |
| 2.5-14 | Pod-identity stamping on operations (`owned_by_pod`) | [[../phase2.3/ownership-integrity-review]] O-1 |
| 2.5-15 | Cloud Run failed-revision: surface `serving_revision` distinct from `current_revision` | [[drift-persistence-assessment]] D-7 |
| 2.5-16 | Cloud Run create-vs-update transient-retry on GetService | [[../phase2.3/eventual-consistency-hazards]] E-4 |
| 2.5-17 | Cloud_account region change refusal if live targets | [[drift-persistence-assessment]] D-4 |

## Phase 2.6 — Chaos Survivability

| # | Title | Source |
|---|---|---|
| 2.6-1 | Runtime sweep goroutine (closes post-first-heartbeat-death stuck-state) | [[ownership-integrity-review]] OS-2, OS-7 |
| 2.6-2 | `succeeded_stale` count → circuit breaker integration | [[reconciliation-convergence-assessment]] C-1 |
| 2.6-3 | `last_observation_kind` column (succeeded/refused/unknown) | [[../phase2.3/truthful-state-assessment]] T-4 |
| 2.6-4 | Stuck-state detector (`creating` > 10min → provider query) | [[../known-issues/deferred-operational-hardening]] §6 |
| 2.6-5 | Graceful-shutdown for in-flight ops on SIGTERM | [[../phase2.3/lro-survivability-review]] S-6 |
| 2.6-6 | Per-rollout configurable DeployTimeout | [[../phase2.3/lro-survivability-review]] S-1 |

## Phase 2.7 — Observability Integrity

| # | Title | Source |
|---|---|---|
| 2.7-1 | Adopt `log/slog` structured logging | [[../phase2.3/observability-integrity]] O-4 |
| 2.7-2 | Prometheus-style metrics emission | [[../phase2.3/observability-integrity]] O-7 |
| 2.7-3 | History row on `[RELEASE_LOST_OWNERSHIP]` | [[incident-reconstruction-maturity-review]] IR-7 |
| 2.7-4 | History row on CAS-race cancel | [[incident-reconstruction-maturity-review]] IR-8 |
| 2.7-5 | `[OP_COMPLETE_SWEEP_CONFLICT]` vs `[OP_COMPLETE_REENTRY]` separation | [[../phase2.3/observability-integrity]] O-6 |
| 2.7-6 | `[HEARTBEAT_FAIL_PERSISTENT]` escalation | [[../phase2.3/observability-integrity]] O-9 |
| 2.7-7 | `[CYCLE_BACKED_OFF]` debug emission | [[../phase2.3/observability-integrity]] O-8 |
| 2.7-8 | Operator-facing CLI: `autostack timeline <target>` | [[incident-reconstruction-maturity-review]] IR-1 |

## Phase 2.8 — Drift / Divergence

| # | Title | Source |
|---|---|---|
| 2.8-1 | Post-destroy NOT_FOUND confirmation poll | [[../phase2.3/delete-orphan-risk-assessment]] D-2 |
| 2.8-2 | Spec-vs-actual drift detection scan | [[drift-persistence-assessment]] D-1 |
| 2.8-3 | Cloud Run `serving_revision` field | [[drift-persistence-assessment]] D-7 |
| 2.8-4 | Region-scoped credential validation (locations/-region) | [[../providers/eventual-consistency-assumptions]] |
| 2.8-5 | Provider returns partial DeployResult on CreateService error path | [[lifecycle-closure-integrity-review]] LC-9 / [[../phase2.3/lineage-integrity-review]] L-2 |

## Phase 2.9 — Production-readiness gate

| # | Title | Source |
|---|---|---|
| 2.9-1 | Full audit pass against all Phase 2.3–2.8 deliverables | this directive |
| 2.9-2 | Production boundary definition document | this directive |
| 2.9-3 | Trustworthiness verdict | this directive |

## Phase 3+ — HA / production-grade

Carried over from [[../phase2.3/deferred-phase2.5-concerns]]:
- KMS-backed key management
- Per-tenant key derivation
- At-rest SQLite encryption / encrypted backups
- Leader election or row-versioning for true HA
- SSO/SAML
- VPC management
- Audit log (separate WORM-style)
- Multi-tenant isolation
- SOC2 compliance
- AI features (incident explainer, right-sizer)

## Related
- [[dangerous-ambiguity-inventory]]
- [[remaining-operational-blockers]]
- [[../phase2.3/deferred-phase2.5-concerns]]
