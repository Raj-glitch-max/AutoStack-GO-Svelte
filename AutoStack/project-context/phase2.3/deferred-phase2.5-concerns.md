# Deferred Phase 2.5 Concerns

## Last Updated
2026-05-14

## What this document is

The Phase 2.3 audit identified many improvements that are NOT landing
this phase. This is the consolidated, prioritized backlog for Phase 2.5
and beyond. Each item links to its discovering audit doc.

## Phase 2.5 — required before next correctness milestone

These should land before any operational chaos validation (chaos
testing, multi-pod pilot, large-customer-target-count pilot).

### Reliability

| # | Title | Source |
|---|---|---|
| 1 | Post-destroy NOT_FOUND confirmation poll | [[delete-orphan-risk-assessment]] D-2 |
| 2 | Pod-identity stamping (`operations.owned_by_pod`) for multi-pod safety | [[ownership-integrity-review]] O-1 |
| 3 | Runtime sweep goroutine using heartbeat-age threshold (current sweep is startup-only) | [[../reconciler/sweep-and-heartbeat-semantics]] |
| 4 | `succeeded_stale` circuit-breaker integration to prevent infinite stale loops | [[replay-safety-assessment]] §3 |
| 5 | Cloud Run create/update transient-retry on GetService | [[eventual-consistency-hazards]] E-4 |
| 6 | One-shot migration to re-encrypt all legacy-plaintext credential rows + remove plaintext fallback in `Decrypt` | [[encryption-integrity-assessment]] EI-3 |

### Lineage / Truthfulness

| # | Title | Source |
|---|---|---|
| 7 | `deployment_history.operation` foreign key | [[lineage-integrity-review]] L-6 |
| 8 | `deployment_history.target` change cascadeDelete: false | [[lineage-integrity-review]] L-4 |
| 9 | Add `stale` to `deployment_history.status` enum so `succeeded_stale` ops can be honestly recorded | [[lineage-integrity-review]] L-5 |
| 10 | History row on `[RELEASE_LOST_OWNERSHIP]` so sweep-overridden dispatchers leave a trace | [[lineage-integrity-review]] L-3 |
| 11 | History row on `[DISPATCH_CLAIM_SKIP]` (CAS race) so cancelled ops have a record | [[lineage-integrity-review]] L-3 |
| 12 | Provider returns DeployResult on CreateService error path so external_id is preserved for orphan correlation | [[lineage-integrity-review]] L-2 |

### Observability

| # | Title | Source |
|---|---|---|
| 13 | Adopt `log/slog` for structured logging | [[observability-integrity]] O-4 |
| 14 | Emit Prometheus-style metrics: `autostack_target_status{status}`, `autostack_dispatch_duration_seconds`, `autostack_operation_in_flight`, `autostack_circuit_open{target}` | [[observability-integrity]] O-7 |
| 15 | `operations.cycle_id` column for on-disk cycle correlation | [[observability-integrity]] O-2 |
| 16 | Separate `[OP_COMPLETE_SWEEP_CONFLICT]` vs `[OP_COMPLETE_REENTRY]` | [[observability-integrity]] O-6 |
| 17 | `[HEARTBEAT_FAIL_PERSISTENT]` escalation after 5 consecutive failures | [[observability-integrity]] O-9 |
| 18 | `[CYCLE_BACKED_OFF]` debug emission | [[observability-integrity]] O-8 |
| 19 | `deployment_targets.last_observation_kind` (succeeded/refused/unknown) | [[truthful-state-assessment]] T-4 |

### Operational tooling

| # | Title | Source |
|---|---|---|
| 20 | Orphan-cleanup scanner (provider-side scan for autostack-managed=true resources without backing target row) | [[../known-issues/deferred-operational-hardening]] §4/§7 |
| 21 | Operation TTL/archival (drop terminal ops > N days old) | [[../known-issues/phase2.2-assessment]] §4 |
| 22 | Stuck-state detector (`creating` > 10min → provider query) | [[../known-issues/deferred-operational-hardening]] §6 |
| 23 | `HandleCloudAccountDelete` refusal if any target rows reference it with status != deleted | [[delete-orphan-risk-assessment]] D-3 |
| 24 | Operator-facing "force re-destroy" endpoint | [[delete-orphan-risk-assessment]] D-5 |
| 25 | Region-scoped credential validation (locations/-region not locations/-) | [[../providers/eventual-consistency-assumptions]] |
| 26 | Per-rollout configurable DeployTimeout in target_config | [[lro-survivability-review]] S-1 |

### Schema hygiene

| # | Title | Source |
|---|---|---|
| 27 | Single canonical `provider` enum across collections | [[../known-issues/deferred-operational-hardening]] §15 |
| 28 | Cross-validate `cloud_accounts.provider` × `deployment_targets.provider` × `rollouts.target_type` at controller boundary | same |

### Provider correctness

| # | Title | Source |
|---|---|---|
| 29 | Real Cloud Run Rollback via Service.Traffic, with lineage columns | [[rollback-integrity-assessment]] |
| 30 | Real Cloud Run GetOperation (LRO polling) | [[../providers/cloudrun-status]] |
| 31 | Real Cloud Run GetMetrics (Cloud Monitoring) | same |
| 32 | Real Cloud Run CheckQuotas | same |
| 33 | Live cost APIs (replace static placeholder) | ADR-010 |
| 34 | GCP Secret Manager integration (replace plaintext env-var secrets) | [[../known-issues/phase2.2-assessment]] §3 |
| 35 | Cloud Run UpdateService with etag/optimistic-concurrency | (new audit finding) |
| 36 | `waitForServiceReady` internal deadline reduced to 20 min | [[../known-issues/phase2.2-assessment]] §3 |

### Maintainability

| # | Title | Source |
|---|---|---|
| 37 | Worker pool for per-target dispatch | [[maintainability-review]] |
| 38 | Per-account provider client pool | [[maintainability-review]] |
| 39 | Per-provider rate limiter | [[maintainability-review]] |
| 40 | Graceful shutdown for in-flight ops on SIGTERM | [[lro-survivability-review]] S-6 |

## Phase 3 — HA / production-grade

| # | Title |
|---|---|
| 41 | KMS-backed key management with rotation |
| 42 | Per-tenant key derivation |
| 43 | At-rest SQLite encryption |
| 44 | Encrypted backups |
| 45 | Leader election / row-versioning for true multi-pod |
| 46 | SSO/SAML |
| 47 | VPC management |
| 48 | Audit log (separate from deployment_history; immutable WORM-style) |

## Phase 4+

- AI-assisted incident explainer
- Right-sizing recommendations
- Multi-tenant isolation
- SOC2 compliance

## Tracking

This list is the source of truth for the post-Phase-2.3 backlog. As
Phase 2.5 picks up items, mark them with their landing phase.

## Related
- [[dangerous-ambiguity-inventory]]
- [[remaining-operational-blockers]]
- [[../known-issues/deferred-operational-hardening]]
