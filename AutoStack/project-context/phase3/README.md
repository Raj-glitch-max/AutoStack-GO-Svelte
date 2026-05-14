# Phase 3 — Multi-Provider Orchestration Maturity

## Last Updated
2026-05-14

## Phase Identity

Phase 3 is the transition from **single-provider operational
correctness** (Phase 2) to **multi-provider orchestration maturity**.

Phase 3 is NOT:
- "Add more cloud logos."
- A workflow-engine project.
- A distributed orchestration project.
- A SaaS multi-tenant project.

Phase 3 IS:
- The disciplined evolution of provider contracts, lifecycle
  normalization, and operational truth-telling across a growing
  provider set, **without compromising Phase 2's correctness guarantees**.

## Phase 3 Sequencing

Phase 3 is broken into 8 sub-phases. Each builds on the prior. Each is
authored, reviewed, and frozen before the next begins. This is
deliberate: most multi-provider platforms collapse because they
implement providers before contracting capability semantics.

### Phase 3.0 — Foundations (THIS SUB-PHASE)

The conceptual scaffolding. No new code. No new providers. Just the
contract surfaces and risk frame that all of Phase 3 builds on.

| Doc | Status | Purpose |
|---|---|---|
| [phase3-architecture-evolution.md](phase3-architecture-evolution.md) | ✅ landed | Trajectory + hard constraints + non-goals |
| [multi-provider-risk-analysis.md](multi-provider-risk-analysis.md) | ✅ landed | The 12 collapse modes Phase 3 must avoid |
| [provider-capability-matrix.md](provider-capability-matrix.md) | ✅ landed | Honest capability framework + Cloud Run baseline |
| [provider-normalization-rules.md](provider-normalization-rules.md) | ✅ landed | Rules for what may be normalized vs. preserved-as-truth |
| [ambiguity-semantics-model.md](ambiguity-semantics-model.md) | ✅ landed | How truthful ambiguity propagates across providers |
| [provider-contract-evolution.md](provider-contract-evolution.md) | ✅ landed | What changes in `Provider` interface and why |
| [lifecycle-normalization-model.md](lifecycle-normalization-model.md) | ✅ landed | Lifecycle states that survive provider differences |
| [future-ha-boundary-analysis.md](future-ha-boundary-analysis.md) | ✅ landed | What HA work is in-scope and what is hard-deferred |

### Phase 3.1 — Provider Architecture Evolution

Provider work AFTER the contracts are frozen. Targeted at adding **one**
second provider (ECS/Fargate likely) under the capability matrix
constraints.

| Planned doc | Purpose |
|---|---|
| `ecs-fargate-provider-design.md` | First non-GCP provider; must satisfy capability matrix |
| `azure-aca-provider-design.md` | Second non-GCP provider; alignment review |
| `provider-isolation-boundaries.md` | What may and may NOT leak from a provider module |
| `provider-capability-negotiation.md` | How reconciler queries capabilities at runtime |

### Phase 3.2 — Provider-Normalized Lifecycle Semantics

Refining the lifecycle model under multi-provider stress.

| Planned doc | Purpose |
|---|---|
| `multi-provider-boundaries.md` | Where each provider's truth ends |
| `provider-drift-model.md` | Drift semantics across providers |
| `partial-success-semantics.md` | How partial-rollout truth is preserved |

### Phase 3.3 — Deployment Workflow Maturity

Workflow primitives — not a workflow engine.

| Planned doc | Purpose |
|---|---|
| `workflow-maturity-roadmap.md` | Sequencing for staged/canary/blue-green primitives |
| `deployment-strategy-model.md` | Pluggable strategy contract |
| `workflow-lifecycle-contracts.md` | How workflows extend (not replace) lifecycle contracts |
| `rollout-semantics.md` | Traffic-shift, pause, resume, cancel semantics |
| `rollback-semantics.md` | Provider-aware rollback contract (supersedes Phase 2 stub) |

### Phase 3.4 — Scalable Reconciliation Foundations

Foundations only — no distributed systems yet.

| Planned doc | Purpose |
|---|---|
| `reconciliation-scaling-foundations.md` | The scaling primitives roadmap |
| `reconciliation-scaling-strategy.md` | Worker pool, queue, fairness contracts |
| `queue-migration-strategy.md` | Path from sequential loop to queue-backed |

### Phase 3.5 — Operational Platform Maturity

UX + tooling — informed by Phase 2 forensic primitives.

| Planned doc | Purpose |
|---|---|
| `operational-platform-maturity-roadmap.md` | UX roadmap sequencing |
| `operational-taxonomy.md` | What operators see vs. what's internal |
| `deployment-lineage-model.md` | How lineage surfaces in UI |
| `incident-reconstruction-guide.md` | Operator-facing runbook |

### Phase 3.6 — GitOps & Repository Integration

Treated carefully. GitOps adds lifecycle complexity.

| Planned doc | Purpose |
|---|---|
| `gitops-boundary-analysis.md` | What we will and will NOT do GitOps-wise |

### Phase 3.7 — Observability & Operational Intelligence

| Planned doc | Purpose |
|---|---|
| `observability-evolution-roadmap.md` | slog → tracing → analytics sequencing |
| `replay-guarantees.md` | Formalized replay contract, supersedes ad-hoc statements |

### Phase 3.8 — Security & Multi-Tenant Foundations

Foundation only — no SaaS yet.

| Planned doc | Purpose |
|---|---|
| `security-multi-tenant-foundations.md` | RBAC, org boundaries, credential segmentation roadmap |
| `tenant-isolation-future.md` | Hard deferral notes |

### Cross-cutting updates (after each sub-phase)

| Doc | When updated |
|---|---|
| Updated [operational-guarantee-matrix.md](../phase2.9/operational-guarantee-matrix.md) | After each sub-phase, with new G-N entries appended |
| Updated [production-readiness-gate.md](../phase2.9/production-readiness-gate.md) → `production-boundaries.md` | After 3.1, 3.4, 3.8 |
| Updated `technical-debt-priority-matrix.md` | After each sub-phase |
| Updated ADRs in [decisions/](../decisions/) | Each architectural decision |
| Updated [trustworthiness-verdict.md](../phase2.9/trustworthiness-verdict.md) → Phase 3 successor | After 3.4 |

## Hard Rules (carried from Phase 2, do not break)

1. Kubernetes path untouched.
2. Reconciliation core not rewritten.
3. CAS-based ownership semantics preserved.
4. Truthful-state philosophy preserved (`ErrNotImplemented` over fake success).
5. Replay safety preserved (sweep honesty, heartbeat scoping, CAS release).
6. Provider isolation enforced (no `if provider == "aws"` in core).
7. Additive architecture only.
8. Lineage completeness preserved.

## Hard Non-Goals for Phase 3

- Temporal-style workflow engines.
- Distributed reconciliation clusters.
- Leader election protocols.
- Kafka-style orchestration infrastructure.
- Multi-pod HA reconciliation (deferred — see `future-ha-boundary-analysis.md`).
- Enterprise BPM features.
- Full GitOps reconciliation engines (Argo/Flux-style).
- Multi-tenant SaaS.

## Phase 3 Trustworthiness Test

The Phase 3 platform is trustworthy when, for **any** supported
provider, an operator can answer:

1. "Did my deploy succeed?" — truthfully, including partial-success states.
2. "What is currently serving traffic?" — truthfully, even during gradual rollouts.
3. "If I roll back, what version will I land on?" — deterministically.
4. "What does the provider know that AutoStack doesn't?" — visibly, via drift surfacing.
5. "What guarantees apply to this provider?" — via capability matrix.
6. "Why did reconciliation skip this cycle?" — via cycle correlation and lineage.

If any of these answers requires the operator to read provider-specific
docs, the abstraction has failed truthfully — and that failure must be
**surfaced**, not papered over.

## Related

- [[../phase2.9/trustworthiness-verdict]] — Phase 2 closure
- [[../phase2.9/deferred-Phase3-concerns]] — original Phase 3 backlog
- [[../phase2.9/operational-guarantee-matrix]] — Phase 2 guarantees that extend into Phase 3
- [[../current-state]] — top-level state
