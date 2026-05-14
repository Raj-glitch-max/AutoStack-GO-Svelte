# AutoStack — Current State (Phase 3.1)

## Last Updated
2026-05-14 (Phase 3.1 — ECS/Fargate provider implemented)

## Active Phase

**Phase 3.1** — first non-Cloud-Run provider (ECS/Fargate) implemented
under Phase 3.0 capability contracts.

See [phase3/README.md](phase3/README.md) for the Phase 3 sub-phase plan.

---

## Phase 3.1 Work Landed (2026-05-14)

### ECS/Fargate Provider Implementation

New package: `pkg/providers/ecs/`

| File | Purpose |
|---|---|
| `provider.go` | `Provider` interface implementation; all 12 methods |
| `capabilities.go` | Phase 3.1 baseline capability profile (9 supported) |
| `lifecycle_mapping.go` | ECS native state → canonical state (N-rule discipline) |
| `confirm.go` | `status-inactive-poll` destroy-confirm (120s window) |
| `contract_test.go` | 13 capability + lifecycle mapping tests; all pass |

Provider registered in `pkg/reconciler/cloud.go` alongside Cloud Run.

### Dispatch Tables (Reconciler)

New file: `pkg/reconciler/dispatch_tables.go`

| Function | Purpose |
|---|---|
| `cacheCapabilities()` | Stores provider Capabilities() at Start() |
| `destroyConfirmDispatch()` | Phase 3 Change 5: routes destroy-confirm by semantic |
| `suspicionThreshold()` | Phase 3 Change 6: per-provider suspicion tuning |
| `supportsCapability()` | Concise capability gate for reconciler branches |
| `capabilityNotes()` | Surfaces Notes for ErrCapabilityUnavailable |

### ErrCapabilityUnavailable

Added to `pkg/providers/provider.go`. Structured refusal type used
when a caller requests an action on a provider that declares
Supported=false. Carries CapabilityKey, provider name, and Notes.

### AWS SDK Dependencies

Added to `go.mod`:
- `github.com/aws/aws-sdk-go-v2/service/ecs`
- `github.com/aws/aws-sdk-go-v2/service/ec2`

HC-6 verified: no aws-sdk imports outside `pkg/providers/ecs/`.

---

## Phase 3.0 Work (previously landed, docs only — now both docs and code)

### Implementation (landed 2026-05-14)

1. `Capabilities()` added to `Provider` interface — `pkg/providers/provider.go`
2. `CapabilitySet`, `Capability`, `CapabilityKey` types defined
3. `AllCapabilityKeys` canonical list (18 keys)
4. Cloud Run `capabilities.go` — Phase 2 baseline profile
5. `ServingRevision string` added to `TargetStatus`
6. Cloud Run `contract_test.go` — 5 tests, all pass
7. Migration `1715300007_updated_deployment_targets_phase3.js` — adds
   `lifecycle_ambiguous`, `lifecycle_native_state`, `lifecycle_ambiguity_source`,
   `lifecycle_ambiguity_detail`, `lifecycle_ambiguity_deadline` columns

### Foundation docs (8, all landed)

| Doc | Purpose |
|---|---|
| [phase3/phase3-architecture-evolution.md](phase3/phase3-architecture-evolution.md) | Trajectory + HC-1..HC-8 hard constraints |
| [phase3/multi-provider-risk-analysis.md](phase3/multi-provider-risk-analysis.md) | R-1..R-12 collapse modes |
| [phase3/provider-capability-matrix.md](phase3/provider-capability-matrix.md) | C-* capability framework |
| [phase3/provider-normalization-rules.md](phase3/provider-normalization-rules.md) | NORMALIZE / AMBIGUATE / EXPOSE rules |
| [phase3/ambiguity-semantics-model.md](phase3/ambiguity-semantics-model.md) | S-1..S-5 ambiguity sources; P-1..P-6 propagation |
| [phase3/provider-contract-evolution.md](phase3/provider-contract-evolution.md) | Provider interface change plan |
| [phase3/lifecycle-normalization-model.md](phase3/lifecycle-normalization-model.md) | N-1..N-8 lifecycle mapping rules |
| [phase3/future-ha-boundary-analysis.md](phase3/future-ha-boundary-analysis.md) | HA scope + Phase 4+ deferral |

---

## Phase 3.1 Docs Landed (design-only, code now added)

| Doc | Purpose |
|---|---|
| [phase3/ecs-fargate-provider-design.md](phase3/ecs-fargate-provider-design.md) | ECS design contract |
| [phase3/azure-aca-provider-design.md](phase3/azure-aca-provider-design.md) | ACA design contract (code pending) |
| [phase3/provider-isolation-boundaries.md](phase3/provider-isolation-boundaries.md) | HC-6 enforcement |
| [phase3/provider-capability-negotiation.md](phase3/provider-capability-negotiation.md) | Negotiation procedure + dispatch tables |

---

## Phase 3.2–3.5 Docs Landed (design-only, no code)

| Phase | Docs | Status |
|---|---|---|
| 3.2 | multi-provider-boundaries.md, provider-drift-model.md, partial-success-semantics.md | Design-only |
| 3.3 | workflow-maturity-roadmap.md, deployment-strategy-model.md, workflow-lifecycle-contracts.md, rollout-semantics.md, rollback-semantics.md | Design-only |
| 3.4 | reconciliation-scaling-foundations.md, reconciliation-scaling-strategy.md, queue-migration-strategy.md | Design-only |
| 3.5 | operational-platform-maturity-roadmap.md, operational-taxonomy.md, deployment-lineage-model.md | Design-only; incident-reconstruction-guide.md missing |

---

## Phase 3.1 Remaining Gaps

| Item | Status |
|---|---|
| Azure ACA provider (`pkg/providers/aca/`) | Design only; no code |
| Reconciler capability-aware destroy routing wired end-to-end | dispatch_tables.go added; not yet called from dispatch.go Destroy path |
| `suspicionThreshold()` used in reconciler suspicion counter | dispatch_tables.go added; not yet replacing hardcoded suspicion logic |
| SC-1: `deployed_spec` migration | Not yet done |
| SC-2: `operations.cycle_id` migration | Not yet done |
| SC-4: `deployment_history.status=stale` | Not yet done |
| DO-1: `log/slog` structured logging | Not yet done |

---

## Phase 2.9 Summary (frozen)

Phase 2.9 closed Phase 2. Three blocking correctness fixes (AW-C1, AW-C2,
AW-C3) are landed. Contracts frozen. Trustworthiness verdict signed.

`go build ./...` and `go vet ./...` clean. Secrets + Cloud Run + ECS tests pass.

### Frozen contracts

- [Lifecycle contracts (DC-1..DC-8)](phase2.9/lifecycle-contracts.md)
- [Provider contracts (P-1..P-15)](phase2.9/provider-contracts.md)
- [Reconciliation architecture freeze (F-1..F-9, E-1..E-4, U-1..U-3)](phase2.9/reconciliation-architecture-freeze.md)
- [Operational guarantee matrix (G-1..G-19)](phase2.9/operational-guarantee-matrix.md)
- [Safe operational boundaries](phase2.9/safe-operational-boundaries.md)

### Operating envelope

- Single PocketBase pod.
- ≤ 20 cloud targets per reconciler instance.
- Cloud Run and ECS/Fargate are Phase 3.1 providers.
- SQLite WAL mode.

---

## Kubernetes path status

**UNCHANGED** through Phase 3.1.

- No modifications to k8s package.
- No modifications to rollout controller.
- No changes to operator.
- No changes to CRD schema.

---

## Related

- [phase3/README.md](phase3/README.md) — Phase 3 sub-phase plan
- [phase2.9/trustworthiness-verdict.md](phase2.9/trustworthiness-verdict.md)
- [phase2.9/deferred-Phase3-concerns.md](phase2.9/deferred-Phase3-concerns.md)
- [README.md](README.md) — project-context index
