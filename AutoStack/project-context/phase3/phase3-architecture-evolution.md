# Phase 3 Architecture Evolution

**Last Updated:** 2026-05-14
**Phase:** 3.0 (Foundations)

## Why This Document Exists

Most platforms that attempt the single-provider → multi-provider
transition collapse architecturally because they make at least one of
these errors:

1. They normalize provider differences into fake consistency.
2. They overgeneralize the Provider interface and lose semantic fidelity.
3. They couple the reconciler to provider-specific behavior.
4. They try to build a workflow engine before the provider contract is stable.
5. They build distributed orchestration before sequential orchestration is correct.

This document records the **trajectory and constraints** Phase 3 follows
to avoid those collapse modes. Every Phase 3 sub-phase is anchored here.

## The Direction of Evolution

Phase 2 left AutoStack as a **truthful single-provider control plane**:
deterministic reconciliation, CAS-based dispatch ownership, sweep-based
replay safety, lineage completeness, and refused abstractions for
unimplemented capabilities.

Phase 3 evolves AutoStack into a **truthful multi-provider orchestration
platform**. The defining property is preserved: AutoStack never lies
about state. Phase 3 extends "no lying" from one provider's truth into
**a comparative framework that does not flatten provider differences
into a false common denominator**.

```
Phase 2: "Cloud Run is honest."
Phase 3: "Cloud Run, ECS, and ACA are each honest, and AutoStack tells
         operators when they differ."
```

## The Evolution Axes

Phase 3 evolves AutoStack along eight axes simultaneously. They are
sequenced (see [README.md](README.md)) but interdependent: a change on
one axis must be checked against all others.

| Axis | Phase 2 state | Phase 3 trajectory | Risk if rushed |
|---|---|---|---|
| Provider count | 1 (Cloud Run) | +1 then +1 (ECS, then ACA) | Capability matrix becomes a lie |
| Provider contract | 13-method interface | Capability-aware interface | Hidden coupling, abstraction explosion |
| Lifecycle normalization | Provider-anchored states | Provider-normalized + truthful divergence | Lifecycle fragmentation |
| Workflow capability | Single-step deploy | Staged primitives | Workflow-engine sprawl |
| Reconciliation scale | Single-thread loop | Worker-pool foundations | Race conditions, ownership corruption |
| Operational UX | PocketBase admin + frontend basics | Real operator dashboards | UI hiding provider truth |
| GitOps integration | None | Bounded integration | Lifecycle authority conflicts |
| Tenant isolation | Single tenant assumed | Foundations only | Premature enterprise complexity |

## The Hard Constraints

These are **load-bearing for correctness**. Phase 3 work that violates
any of them is a regression and must be rejected at review.

### HC-1 — Kubernetes path untouched

The k8s operator/CRD/controller is the anchor of trust for AutoStack.
Phase 3 adds nothing to that path. New providers live exclusively in
`pkg/providers/<provider>/`.

### HC-2 — Reconciliation core not rewritten

`reconcileAll → reconcileOne → dispatch*` is the single-threaded
ownership backbone (Phase 2 F-1, F-2, F-3). Phase 3 may add hooks,
prioritization metadata, and queue interfaces around it. It may NOT
replace it.

### HC-3 — Ownership semantics preserved

CAS claim + release-CAS is the load-bearing invariant. No Phase 3
change may bypass either.

### HC-4 — Truthful-state philosophy preserved

`ErrNotImplemented` is honest. Synthesized zero-values are lies. Phase 3
extends this with capability-flag absence (see
[provider-capability-matrix.md](provider-capability-matrix.md)).

### HC-5 — Replay safety preserved

Sweep honesty (G-3), heartbeat scoping (G-16), and release-CAS (G-4)
are intact. Any Phase 3 change to operations lifecycle must extend
sweep predicates explicitly.

### HC-6 — Provider isolation enforced

No `if provider == "aws"` in `pkg/reconciler/`, `pkg/controller/`, or
the frontend. All provider-specific logic lives in
`pkg/providers/<provider>/`.

### HC-7 — Additive architecture

Phase 3 adds files, methods, and tables. It removes none in the core
reconciler or in the existing Provider interface (new methods may be
added; existing methods may evolve only via additive return-value fields
or context-key conventions).

### HC-8 — Lineage completeness

Every Phase 3 lifecycle event (workflow steps, rollback checkpoints,
capability negotiation outcomes) writes a `deployment_history` row.
DC-8 / G-12 are extended, not replaced.

## The Hard Non-Goals

These are EXPLICITLY OUT of scope. Phase 3 work in this direction is
out-of-bounds:

| Non-goal | Reason |
|---|---|
| Distributed reconciliation clusters | Phase 2 single-pod envelope; HA is Phase 4+ |
| Leader election | Premature; sequential loop + CAS still adequate |
| Kafka/NATS orchestration | No scale evidence yet |
| Temporal/Cadence-style workflow engines | Sprawl; primitives suffice |
| Full GitOps reconciliation (Argo/Flux) | Authority-boundary conflicts |
| Multi-tenant SaaS | Foundations only; no tenant infra |
| Enterprise RBAC matrices | Foundations only |
| Drift remediation automation | Drift visibility first; remediation last |
| Cross-provider migration tooling | Each provider's lineage stays its own |

## The Three Trustworthiness Tests Every Phase 3 Change Must Pass

Before any Phase 3 PR merges:

1. **Does the change introduce abstraction without provider semantic
   fidelity?** If two providers behave differently and the change
   pretends they don't — reject.

2. **Does the change weaken Phase 2 guarantees G-1 through G-19?** If
   yes — reject. (See
   [operational-guarantee-matrix.md](../phase2.9/operational-guarantee-matrix.md).)

3. **Does the change require an `if provider == "X"` branch in core
   reconciliation, controller, or frontend code?** If yes — the
   abstraction is wrong. Move the difference into the provider module
   or expose it via the capability matrix.

## Provider-Coupling Detection

A simple rule prevents the most common collapse mode:

> If the reconciler, controller, or frontend imports a provider package
> directly (e.g. `pkg/providers/cloudrun`), it is **coupled**.
> If it only imports `pkg/providers` (the interface package), it is
> **decoupled**.

Phase 2 already obeys this rule. Phase 3 keeps it. A lint or compile-time
check (`go vet` + custom analyzer) may be added in Phase 3.4 to enforce.

## Surface Area Growth Budget

The Provider interface today has 12 methods. Phase 3 should not double
that. Soft budget:

| Provider interface methods | Phase 2 final | Phase 3 budget | Phase 3 hard ceiling |
|---|---|---|---|
| Required methods | 5 | +1 (Capabilities()) | +2 |
| Optional methods (`ErrNotImplemented`-allowed) | 7 | +2 | +4 |

If a Phase 3 design wants 6 new methods, **first** consolidate via
capability-conditional methods or extension structs in
`pkg/providers`. Method-count is a signal — exceeding it indicates the
abstraction is bending under provider difference instead of containing it.

## The Three Risk Categories Phase 3 Introduces

| Risk category | Examples | Mitigation |
|---|---|---|
| **Abstraction lies** | Capability matrix says "supports rollback" while the impl is broken | Capability flags must be code-verified at runtime + tested |
| **Lifecycle fragmentation** | ECS "PENDING" mapped onto Cloud Run "creating" without verifying semantic equivalence | `lifecycle-normalization-model.md` defines the lossless mapping rules |
| **Premature complexity** | Worker pool, queue, leader election all built before need | Sub-phase sequencing + this doc's non-goals list |

## What "Done" Looks Like for Phase 3

Phase 3 closes when:

1. ≥ 2 providers (Cloud Run + ECS, ideally + ACA) implement the
   capability-aware contract.
2. Lifecycle normalization rules are enforced in code and tested.
3. Reconciliation worker-pool foundations exist (single-pod still),
   without weakening Phase 2 guarantees.
4. Operational dashboard surfaces capability matrix and ambiguity states
   truthfully.
5. The trustworthiness-verdict successor doc signs Phase 3 closure with
   ≥ 8.5/10 weighted score.

## Related

- [[phase3/multi-provider-risk-analysis]] — 12 collapse modes
- [[phase3/provider-capability-matrix]] — capability framework
- [[phase3/provider-normalization-rules]] — abstraction rules
- [[phase3/ambiguity-semantics-model]] — truth propagation
- [[phase3/provider-contract-evolution]] — interface changes
- [[phase3/lifecycle-normalization-model]] — multi-provider lifecycle
- [[phase3/future-ha-boundary-analysis]] — what HA is and isn't
- [[../phase2.9/reconciliation-architecture-freeze]] — Phase 2 invariants
- [[../phase2.9/trustworthiness-verdict]] — Phase 2 closure
