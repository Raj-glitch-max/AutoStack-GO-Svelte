# Provider Normalization Rules

**Last Updated:** 2026-05-14
**Phase:** 3.0 (Foundations)

## Purpose

The **decision procedure** for whether a given provider difference may
be normalized away, must be surfaced as ambiguity, or must be exposed
provider-specifically. This is the operational extension of
[[lifecycle-normalization-model]] and [[provider-capability-matrix]].

Without explicit rules, every PR re-litigates the same question.

## The Three Treatments

For any cross-provider difference, exactly one of these treatments
applies:

### Treatment NORMALIZE

The difference is **semantically identical** at the operator-facing
contract level. Normalization is safe. The provider-native detail may
still be retained in `lifecycle_native_state` or `drift_summary` for
debugging, but the canonical state and operator-visible UX are
uniform.

**Example:** Cloud Run `Ready=SUCCEEDED` and ECS
`rolloutState=COMPLETED` both indicate "the provider considers the
deployment fully rolled out." Both map to canonical `running` (subject
to N-3's affirmative-readiness gate).

### Treatment AMBIGUATE

The difference is **partially semantically equivalent** but has a
gap that operators must be aware of. The canonical state may be set,
but the ambiguity bit is set and the UI surfaces the difference.

**Example:** Cloud Run `Ready=SUCCEEDED` while old revision serves
100% of traffic. The deployment is provisionally ready but not
operationally ready. Canonical state: `running`. Ambiguity bit: TRUE.
UI: "Ready, traffic shifting...".

### Treatment EXPOSE

The difference is **semantically distinct** and must NOT be
normalized. The operator must be shown the provider-native behavior
directly. Often these are capability-conditional features (rollback,
traffic split, canary).

**Example:** Cloud Run gradual-traffic-rollback vs ECS instant-cutover
rollback. These are different operations with different safety
profiles. The UI exposes the per-provider semantic via the
[[provider-capability-matrix]]; the operator chooses with full
knowledge.

## The Decision Procedure

Given a provider difference, walk this decision tree in order. Stop at
the first matching rule:

### Q1 — Does the difference affect the canonical lifecycle state?

If yes → consult [[lifecycle-normalization-model]] N-1..N-8 first.
The lifecycle rules are absolute; this doc does not override them.

### Q2 — Is the difference observable by the operator (UI, logs, history)?

If no → **NORMALIZE**. Hidden internal differences may be normalized;
they affect no contract.

### Q3 — Would normalizing the difference cause an operator to make an unsafe decision?

A decision is **unsafe** if:
- It leads to data loss or downtime that the operator did not anticipate.
- It hides a partial-success that would warrant investigation.
- It causes a rollback to behave differently than the operator expects.
- It causes a deployment to converge with different traffic semantics
  than the operator expects.

If yes → **EXPOSE**. Do not normalize.

### Q4 — Is the difference a timing/latency difference within an order of magnitude?

If yes → **NORMALIZE** with capability-driven tuning (e.g., per-provider
`UncertaintyP99` informs suspicion-counter threshold). The operator
sees uniform behavior; the system tunes itself.

### Q5 — Is the difference a partial overlap (some capability exists in one, absent in the other)?

If yes → **EXPOSE**. The capability matrix declares
`Supported: true/false`; the UI surfaces the difference. Do not
normalize "rollback" if one provider's rollback is fundamentally
unlike another's.

### Q6 — Is the difference in the failure-mode surface (error codes, transient categories)?

If yes → **AMBIGUATE** by default. Error categories map to a canonical
set (`auth`, `quota`, `permanent`, `transient`, `timeout`). Provider-
specific failure reasons are preserved in `drift_summary` and
`deployment_history.message`. The category is normalized; the message
is preserved.

### Q7 — Is the difference in the eventual-consistency lag?

If yes → **NORMALIZE** the surface (operators see `updating` then
`running`) with capability-driven tuning. Cloud Run's 5s lag and ECS's
30s lag look identical to the operator; the system adjusts internally.

### Q8 — Default

If none of Q1–Q7 match → **AMBIGUATE**. When unsure, the platform tells
the truth (R-1 mitigation). Operators can investigate. Normalization
without justification is a lie.

## The Rule Examples (audited cases)

These are the concrete cross-provider differences encountered in Phase
3 planning, with their treatment decisions recorded for future
consistency.

| Difference | Q-Path | Treatment | Notes |
|---|---|---|---|
| Cloud Run "Ready=SUCCEEDED" vs ECS "rolloutState=COMPLETED" | Q1→N-3 | NORMALIZE → `running` | Both are affirmative readiness |
| Cloud Run gradual traffic vs ECS instant cutover (deploy) | Q3 (operator decision) | EXPOSE | Capability `C-CanaryRollout` differs |
| Cloud Run `Ready=SUCCEEDED` with traffic on old revision | Q3 (operator decision) | AMBIGUATE | New: `ServingRevision != latest` |
| Cloud Run rollback (gradual) vs ECS rollback (cutover) | Q5 (capability overlap) | EXPOSE | `C-Rollback` semantic differs |
| Cloud Run 5s eventual lag vs ECS 30s eventual lag | Q7 (latency) | NORMALIZE | `UncertaintyP99` tunes suspicion |
| Cloud Run NOT_FOUND-confirm vs ECS status-DELETED confirm vs ACA tombstone | Q5 (capability overlap) | NORMALIZE the surface, EXPOSE the lag | `C-DestroyConfirmation` semantic drives dispatcher logic; operator sees `deleted` uniformly |
| Cloud Run scaling reads (Cloud Monitoring) vs ECS reads (CloudWatch) | Q2 (operator observable: YES; Q5 capability overlap) | EXPOSE | `C-MetricsVisibility` profile per provider |
| Cloud Run auth errors vs IAM errors (AWS) | Q6 (failure surface) | AMBIGUATE | Category=`auth`; message preserved |
| ECS task placement failure (no capacity) | Q3 (operator action: increase capacity) | EXPOSE | Maps to canonical `error` with provider-specific `drift_summary` |
| Cloud Run "max instances reached" | Q3 (operator action: scale quota) | EXPOSE | Maps to canonical `error` with provider-specific reason |

## The Operator-Facing Contract (R-1 defense)

Every Phase 3 UI element that displays cross-provider information MUST
carry one of these annotations:

- **(uniform)** — Treatment NORMALIZE was applied. Operator may assume
  equivalent semantics.
- **(provider-specific)** — Treatment EXPOSE was applied. Operator
  must consult per-provider docs.
- **(ambiguous)** — Treatment AMBIGUATE was applied. Operator should
  investigate.

The annotation is **part of the UI contract**, not a decoration.

## The Anti-Patterns This Doc Forbids

1. **Silent normalization without rule justification.** Every cross-
   provider mapping in code must trace to a Q-rule in this doc.
2. **AMBIGUATE-by-omission.** Setting the ambiguity bit without also
   populating the `lifecycle_native_state` detail. The operator needs
   to know *what* is ambiguous.
3. **EXPOSE leaking into core code.** EXPOSE means surface in
   capability matrix and per-provider UI; it does NOT mean
   `if provider == "X"` in reconciler logic. Use capability flags.
4. **NORMALIZE without runtime test coverage.** A normalization claim
   must be backed by a cross-provider test asserting the behavioral
   equivalence the rule depends on.

## Verification

Phase 3.1+ adds a cross-provider integration test:

```
TestCrossProviderNormalizationConsistency:
  For each "NORMALIZE"-treatment difference in this doc:
    - Drive the same operator action on both providers
    - Assert the resulting canonical state matches
    - Assert the resulting ambiguity bit matches
```

Failures here mean either the normalization rule is wrong, or one
provider's implementation has drifted. Either is a Phase 3 correctness
issue, not a test issue.

## Phase 3.0 Closure Criteria

For this doc:

1. Q1–Q8 decision procedure is sealed (above).
2. NORMALIZE/AMBIGUATE/EXPOSE definitions are sealed.
3. Audited-cases table is the baseline for Phase 3.1 provider work.
4. UI contract annotation rule is added to Phase 3.5 frontend planning.

## Related

- [[phase3/provider-capability-matrix]] — capability semantics
- [[phase3/lifecycle-normalization-model]] — lifecycle state mapping
- [[phase3/ambiguity-semantics-model]] — ambiguity propagation
- [[phase3/provider-contract-evolution]] — the technical seam
- [[phase3/multi-provider-risk-analysis]] — R-1, R-2, R-3 mitigation
