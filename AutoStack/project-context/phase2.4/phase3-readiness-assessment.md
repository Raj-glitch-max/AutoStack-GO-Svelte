# Phase 3 Readiness Assessment — Phase 2.4

## Last Updated
2026-05-14

## Question

Does the current architecture trajectory remain SAFE for Phase 3
multi-provider evolution (AWS ECS, Azure ACA, HA control plane,
KMS-backed encryption, SSO/SAML)?

## Architecture-trajectory check

### Provider neutrality

| Concern | Status |
|---|---|
| `providers.Provider` interface | Stable across Phase 1.9–2.4. 14 methods, well-defined. |
| Provider registry singleton | Stateless; safe for concurrent use post Phase 1.9 fix. |
| `cloud_accounts.provider` triplicate enum | Still inconsistent across collections — Phase 2.5 work. |
| Provider-specific config in `target_config` map | Generic JSON; safe to extend per provider. |
| Provider error classification | Substring matching is GCP-flavored; AWS / Azure errors not yet tested. |

**Verdict:** Phase 3 can add ECS / ACA providers as new packages
under `pkg/providers/`. No `if provider == "gcp"` conditionals in core
paths — CLAUDE.md mandate held.

### Lifecycle semantic preservation

| Concern | Status |
|---|---|
| Status enum (`deployment_targets.status`) | Provider-neutral; same vocabulary works for ECS tasks and ACA revisions. |
| Operation kind enum | `deploy`, `rollback`, `destroy` are universal. |
| Stale-spec detection | Provider-neutral (uses `rollouts.updated`). |
| Suspicion counter | Provider-neutral (operates on status transitions). |
| Transition guard | Provider-neutral. |

**Verdict:** Lifecycle semantics carry forward unchanged for new
providers.

### Truthful-state preservation

| Concern | Status |
|---|---|
| ErrNotImplemented refusal pattern | Established (Rollback, GetOperation, GetMetrics, CheckQuotas). New providers can refuse honestly. |
| Status "unknown" non-persistence | Provider-neutral. |
| Single-observation regression refusal | Provider-neutral. |

**Verdict:** Solid.

### Rollback semantics

| Concern | Status |
|---|---|
| Rollback design | Documented per provider needs (Cloud Run via Traffic, ECS via service-update, ACA via revisions). No code shared, but lineage/CAS pattern is provider-neutral. |

**Verdict:** Ready for Phase 3 implementation.

### Replay semantics

| Concern | Status |
|---|---|
| CAS-claim pattern | SQLite/Postgres-portable. |
| Sweep with heartbeat-aware policy | Provider-neutral. |
| `pending_destroy` re-arm | Provider-neutral. |

**Verdict:** Safe.

### Ownership integrity

| Concern | Status |
|---|---|
| `current_operation` CAS | Portable. |
| Heartbeat | Provider-neutral. |
| Multi-pod safety | NOT READY. Requires pod-identity stamping. |

**Verdict:** Multi-pod blocked until Phase 2.5 / Phase 3 work lands.
Single-pod Phase 3 deployments (one AutoStack pod managing multiple
providers) ARE safe.

## Architectural dead-end risks

Re-evaluating from Phase 2.3 [[../phase2.3/maintainability-review]]:

| Risk | Status |
|---|---|
| DE-1 PocketBase tight coupling | Acceptable. |
| DE-2 SQLite single-writer | Postgres migration path documented; Phase 3 work. |
| DE-3 Provider singleton | Stateless; ✓. |
| DE-4 Heartbeat as only liveness primitive | Forward-fit with pod-identity stamping. |
| DE-5 Reconciler tight-coupling | Forward-fit. |
| DE-6 Status enum migration cost | Manageable. |

No new dead-ends introduced in Phase 2.4.

## Premature abstraction risks

The codebase HAS NOT introduced:
- Generic "operation engine" abstractions
- Pluggable workflow / state-machine frameworks
- Multi-provider feature flags
- Provider capability matrices

This is **good**. Phase 3 should add providers concretely (one at a
time), not via speculative abstraction.

## What Phase 3 should land BEFORE adding providers

1. Pod-identity stamping (operations.owned_by_pod).
2. Runtime-sweep with peer-pod awareness.
3. Postgres-compatible CAS predicates (normalize empty-string vs NULL
   semantics).
4. Per-cloud-account backoff / circuit-breaker.
5. KMS-backed key management (encryption work).
6. SSO/SAML (if multi-tenant).
7. Stable provider error classification (AWS / Azure-aware mappings).

## Phase 3 risk inventory

| Risk | Mitigation |
|---|---|
| AWS ECS uses synchronous DescribeServices but eventually consistent task lists | Provider implementation responsibility |
| Azure ACA uses LRO patterns (200 vs 202) | Provider implementation responsibility |
| Concurrent reconciliation across providers in one process | Worker pool work |
| Provider-specific rate limits | Per-account / per-provider rate limiters |
| Different revision/version naming conventions | `current_revision` is provider-opaque text — already neutral ✓ |

## Phase 2.4 implementation in this area

None. Phase 3 readiness is a continuous assessment; this doc is the
current state of trust.

## Verdict

The architecture is **Phase 3 ready** for:
- Adding ECS / ACA providers as new packages.
- Implementing Rollback per provider.
- Implementing live metrics / quota / cost APIs.
- Adding new status enum values (with migrations).
- Adding new operation kinds (with migrations).

The architecture is **NOT Phase 3 ready** for:
- Multi-pod HA control planes (Phase 2.5 work blocks this).
- Production-grade compliance workloads (Phase 3 audit log work).
- Tenant isolation (Phase 3 work).

## Related
- [[../phase2.3/maintainability-review]]
- [[../known-issues/deferred-operational-hardening]]
