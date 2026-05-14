# Multi-Provider Risk Analysis

**Last Updated:** 2026-05-14
**Phase:** 3.0 (Foundations)

## Purpose

A catalogued inventory of **how multi-provider orchestration platforms
collapse** and the specific mitigation each Phase 3 design owes the
reader. This document is the **adversarial check** that every Phase 3
PR is reviewed against.

Risks are graded by **severity** (probability × consequence) and
**proximity** (how soon Phase 3 must address it).

---

## R-1 (Critical, Phase 3.0) — Abstraction Lies

**Pattern:** The Provider interface declares `Rollback(...)`. The Cloud
Run implementation does revision-traffic-targeting (real). The ECS
implementation does `update-service --task-definition <old>` (degraded:
no canary, no traffic split). The interface hides the difference. An
operator triggers rollback on an ECS service expecting Cloud Run-style
traffic gradualism and gets an instant cutover, surprising them at 3 AM.

**Why it kills platforms:** Operators stop trusting any
`Provider.X(...)` call. They start reading provider docs in parallel
with AutoStack docs — at which point AutoStack stops adding value.

**Phase 3 mitigation:**
- [[provider-capability-matrix]] declares each capability with a
  truthful semantic profile, not just a yes/no flag.
- Capability declarations are **code-verified** — runtime checks ensure
  the implementation does what the matrix claims.
- UI surfaces the capability semantic, not just availability.

---

## R-2 (Critical, Phase 3.0) — Fake Normalization

**Pattern:** Cloud Run's `Ready=SUCCEEDED` means "revision ready, not
necessarily serving traffic." ECS's `RUNNING` means "task is up and
healthy per the load balancer." ACA's `Running` means yet another
thing. The reconciler maps all three to AutoStack's `running` status.
Operators see `running`; ECS service is failing health checks; AutoStack
shows green.

**Why it kills platforms:** The lifecycle state is the operator's
primary signal. If it's wrong, every downstream decision (rollback,
scale, alerts) is wrong.

**Phase 3 mitigation:**
- [[lifecycle-normalization-model]] defines a **lossless** mapping
  with explicit ambiguity states (`running-not-yet-serving`,
  `running-degraded`, etc.) where providers genuinely differ.
- The reconciler refuses to flatten differences that the model marks
  as semantically distinct.
- [[ambiguity-semantics-model]] propagates uncertainty to the UI
  rather than swallowing it.

---

## R-3 (Critical, Phase 3.0/3.1) — Lowest-Common-Denominator Architecture

**Pattern:** To make providers "consistent," the platform omits Cloud
Run's traffic-targeting, ECS's deployment circuit breakers, and ACA's
revision modes. Operators lose access to provider-native power.
Eventually the platform's value-add is "we abstract clouds" but the
abstraction is so impoverished that everyone bypasses it.

**Why it kills platforms:** Users go directly to provider tooling.
AutoStack becomes a thin proxy with no leverage.

**Phase 3 mitigation:**
- **Capability extension**, not capability subtraction:
  [[provider-contract-evolution]] adds capability-conditional methods
  that providers may implement; absence is honest, not crippling.
- The deployment-strategy model (Phase 3.3) lets operators opt into
  provider-native power via strategy selection, not by giving up
  AutoStack.

---

## R-4 (High, Phase 3.1) — Hidden Provider Coupling

**Pattern:** Core code grows `if provider == "ecs"` branches. Each
branch is small. Within 6 months the reconciler is a switch statement
over providers. The Provider interface is irrelevant: real behavior
lives in the switches.

**Why it kills platforms:** The cost of adding a provider goes from
"implement an interface" to "find and update every switch." Each
addition risks regressing every existing provider.

**Phase 3 mitigation:**
- HC-6 (provider-isolation): no provider imports outside
  `pkg/providers/<provider>/`.
- A Phase 3.4 lint check enforces this at CI.
- Capability negotiation (Phase 3.1) is the **only** sanctioned place
  for provider-conditional reconciler behavior, and it routes by
  capability flag, not provider name.

---

## R-5 (High, Phase 3.0) — Replay Inconsistency

**Pattern:** Cloud Run's sweep behavior depends on the assumption that
`GetService` after `DeleteService` eventually returns NOT_FOUND. ECS
deletion is "DELETED" status, not absence; ACA returns a tombstone.
Phase 2's `confirmDeleted` poll loop, hard-coded to NOT_FOUND, lies
when applied to ECS/ACA.

**Why it kills platforms:** Replay safety (G-3, G-8, G-16) is the
spine of Phase 2's correctness. Cross-provider replay drift means the
control plane can no longer truthfully say "deleted" or recover from a
crash.

**Phase 3 mitigation:**
- Provider capability matrix includes `DestroyConfirmationMode`
  (NOT_FOUND, status-DELETED, tombstone) so the dispatcher's confirm
  logic is provider-aware.
- The sweep predicates are extended (not replaced) to account for new
  modes. Phase 2 sweep behavior remains intact for Cloud Run.

---

## R-6 (High, Phase 3.2) — Lifecycle Fragmentation

**Pattern:** Each provider adds 2–3 lifecycle states unique to its
platform. The state machine grows from 8 states to 30. Transitions
between provider-specific states are undefined or unsafe. Status
transition guard (F-5) becomes unmaintainable.

**Why it kills platforms:** The lifecycle state machine is supposed to
be a small, auditable invariant. Once it crosses ~12 states, operators
cannot reason about it.

**Phase 3 mitigation:**
- [[lifecycle-normalization-model]] defines the **canonical** state
  set. Provider-specific lifecycle details are captured in extension
  fields, not in new top-level states.
- Strict review gate: adding a top-level state requires updating
  every transition rule in `isAllowedTransition` + a justification in
  this doc.

---

## R-7 (High, Phase 3.1/3.2) — Rollback Inconsistency

**Pattern:** "Rollback" means revision-traffic-targeting on Cloud Run,
prior-task-def cutover on ECS, prior-revision activation on ACA.
Operator semantics — "land on the previous version" — looks the same
externally but is fundamentally different in safety profile.

**Why it kills platforms:** Rollback is the most operationally
sensitive action. Inconsistency here causes incidents during incidents.

**Phase 3 mitigation:**
- [[lifecycle-normalization-model]] separates `RollbackAtomicity`
  (instant-cutover vs gradual-traffic-shift vs revision-swap) and
  `RollbackTrafficShape` (binary vs gradual).
- UI surfaces the rollback profile *before* the operator commits.
- Phase 3.3 `rollback-semantics.md` formalizes per-provider semantics.

---

## R-8 (Medium, Phase 3.2) — Operational Ambiguity Across Providers

**Pattern:** Cloud Run's "eventually consistent" lag is ~5s. ECS's lag
is ~30s. ACA's is variable. The reconciler treats all three
identically; operators see misleading "settling" times.

**Why it kills platforms:** Time-to-truthful-state varies by provider.
Operators learn (the hard way) that some providers settle quickly and
others don't.

**Phase 3 mitigation:**
- Provider capability matrix declares
  `EventualConsistencyLagP99 time.Duration`. UI surfaces it.
- Reconciler uses it to set suspicion-counter thresholds per provider
  (Phase 2 used a single global; Phase 3 makes it per-provider).

---

## R-9 (Medium, Phase 3.1) — Provider-Specific Chaos Leaking Everywhere

**Pattern:** AWS quota errors are AWS-specific. GCP eventual consistency
quirks are GCP-specific. Each provider has 5–10 "weird" failure modes.
The error classifier (`ClassifyError`) accumulates all of them into one
function. The function becomes a 500-line switch statement.

**Why it kills platforms:** Error classification quality determines
retry behavior, circuit behavior, and alert noise. A monolithic
classifier is fragile and provider-coupled.

**Phase 3 mitigation:**
- Error classification moves **into** the provider module:
  `Provider.ClassifyError(err) FailureCategory`.
- The reconciler calls the provider's classifier; the core has only the
  category enum.
- Phase 2 categorization is the baseline; providers override only
  where they have provider-specific cases.

---

## R-10 (Medium, Phase 3.3) — Workflow Sprawl

**Pattern:** Workflow primitives (canary, blue-green, staged) start as
small additions. Each takes hooks in the reconciler. Within months the
reconciler is a workflow engine. The thing Phase 3 explicitly said not
to build.

**Why it kills platforms:** Workflow engines are notoriously hard to
keep correct. Worse, they pull all lifecycle decisions through their
state machine, which becomes incompatible with the Phase 2 reconciler.

**Phase 3 mitigation:**
- Phase 3.3 ships **strategies**, not **workflows**. A strategy is a
  composable provider-aware deployment plan stored on the target.
- The reconciler dispatches the strategy step-by-step using existing
  CAS semantics. No long-running workflow state lives outside the
  target row.
- Strict ceiling: if a strategy needs ≥ 5 sequential steps, it goes
  through architectural review.

---

## R-11 (Medium, Phase 3.4) — Premature Distributed Complexity

**Pattern:** Worker pool. Then queue. Then leader election. Then
sharding by provider. Then Kafka. AutoStack becomes an orchestration
infrastructure project, not a deployment platform.

**Why it kills platforms:** Distributed primitives have steep
correctness cliffs (consensus, exactly-once, ordering). Each new layer
is a chance to weaken Phase 2 ownership and replay guarantees.

**Phase 3 mitigation:**
- Phase 3.4 ships **single-process worker pool only**. No distributed
  coordination.
- Multi-pod HA is hard-deferred to Phase 4+
  ([[future-ha-boundary-analysis]]).
- Queue interfaces are designed so a future single-binary queue (e.g.
  internal channel pool) can satisfy them — no broker required.

---

## R-12 (Medium, Phase 3.6) — GitOps Authority Conflict

**Pattern:** GitOps says "the repo is the source of truth." AutoStack
says "PocketBase is the source of truth." Both reconcile. Both have
distinct lifecycle semantics. The result: deploy churn, oscillation,
mysterious overwrites.

**Why it kills platforms:** Authority ambiguity is the worst failure
mode in declarative systems. Operators stop being able to predict what
will happen.

**Phase 3 mitigation:**
- Phase 3.6 `gitops-boundary-analysis.md` declares **PocketBase remains
  the source of truth**. GitOps integration is event-driven (commit →
  trigger), not pull-based reconciliation.
- No GitOps reconciler watches the repo independently.

---

## Risk Summary Matrix

| ID | Risk | Severity | Phase | Mitigation owner doc |
|---|---|---|---|---|
| R-1 | Abstraction lies | Critical | 3.0 | provider-capability-matrix |
| R-2 | Fake normalization | Critical | 3.0 | lifecycle-normalization-model |
| R-3 | Lowest-common-denominator | Critical | 3.0/3.1 | provider-contract-evolution |
| R-4 | Hidden provider coupling | High | 3.1 | This doc HC-6 + lint |
| R-5 | Replay inconsistency | High | 3.0 | provider-capability-matrix |
| R-6 | Lifecycle fragmentation | High | 3.2 | lifecycle-normalization-model |
| R-7 | Rollback inconsistency | High | 3.1/3.2 | rollback-semantics (3.3) |
| R-8 | Cross-provider ambiguity | Medium | 3.2 | ambiguity-semantics-model |
| R-9 | Provider chaos leaking | Medium | 3.1 | provider-contract-evolution |
| R-10 | Workflow sprawl | Medium | 3.3 | workflow-maturity-roadmap |
| R-11 | Premature distribution | Medium | 3.4 | reconciliation-scaling-foundations |
| R-12 | GitOps authority conflict | Medium | 3.6 | gitops-boundary-analysis |

## Review Discipline

Every Phase 3 PR should be reviewed against R-1 through R-12 explicitly.
A PR description that does not mention which risks it could activate
**is not ready for review**.

## Related

- [[phase3/phase3-architecture-evolution]] — overall trajectory
- [[phase3/provider-capability-matrix]] — R-1, R-5 mitigation
- [[phase3/lifecycle-normalization-model]] — R-2, R-6 mitigation
- [[phase3/provider-contract-evolution]] — R-3, R-9 mitigation
- [[phase3/ambiguity-semantics-model]] — R-8 mitigation
- [[phase3/future-ha-boundary-analysis]] — R-11 hard-deferral
