# Partial Success Semantics (Phase 3.2)

**Last Updated:** 2026-05-14
**Phase:** 3.2 (Provider-Normalized Lifecycle Semantics)

## Purpose

Define how AutoStack handles **partial success** — the operational
state where a deployment is neither fully succeeded nor failed, but
some subset of expected outcomes are realized.

This is the hardest truthfulness problem in multi-provider orchestration.
Phase 2 had a simple model (deploy either converges or errors). Phase 3
introduces deployment workflows (canary, blue-green, staged) where
partial success is the normal intermediate state.

## What Counts As Partial Success

Six concrete scenarios:

### PS-1 — Provider reports ready, traffic not yet shifted

Cloud Run revision `Ready=SUCCEEDED`, but `Service.Traffic` still
routes 100% to old revision. This is normal during gradual rollout but
ambiguous if AutoStack doesn't know whether traffic shift is in
progress or stopped.

**Lifecycle position:** `running` with `ServingRevision != latest`.

**Treatment:** AMBIGUATE (S-2). Canonical `running` + ambiguity bit +
`lifecycle_native_state="ready-not-serving"`.

### PS-2 — Some replicas healthy, some unhealthy

ECS service: `desiredCount=4`, `runningCount=3`, one task crash-looping.

**Lifecycle position:** `running` with degraded capacity.

**Treatment:** AMBIGUATE (S-2). `lifecycle_native_state=
"degraded-3-of-4"`.

### PS-3 — Canary deployed but not promoted

Phase 3.3 workflow: canary at 5% traffic; metrics under observation; no
promotion decision yet.

**Lifecycle position:** `running` (canary serving) with workflow state
indicating "canary pending promotion."

**Treatment:** Workflow primitives expose; lifecycle stays `running`.
The workflow state is separate from lifecycle state.

### PS-4 — Old revision draining

During blue-green cutover: traffic shifting from blue to green; blue
still active but draining.

**Lifecycle position:** `running` (green serving) with drift bit if
the operator expected blue to be gone.

**Treatment:** Workflow tracks the cutover; lifecycle reflects green's
status.

### PS-5 — Multi-region deploy, one region failed

(Future scope — Phase 3.5+ when multi-region deployment groups exist.)

**Lifecycle position:** the deployment_target represents one region;
partial multi-region is multi-target. Each target reflects its region
truthfully.

### PS-6 — Destroy initiated, resource not yet gone

The standard Phase 2.8 case: `confirmDeleted` window in progress.

**Lifecycle position:** `deleting` until confirmation.

**Treatment:** Phase 2.8 confirm logic (now capability-aware in Phase 3
via destroy-confirm dispatch table).

## The Core Principle

**Partial success is a state, not a transition.** AutoStack must:

1. Persist canonical lifecycle state per Phase 3 N-rules.
2. Record the partial-success dimension(s) via:
   - `lifecycle_ambiguous=true` (where applicable)
   - `lifecycle_native_state` (provider-readable form)
   - `workflow_state` (Phase 3.3 column, when workflows are present)
   - `drift_summary` (when drift is involved)
3. Refuse to flatten any of these into a single "everything-is-fine"
   signal.

## The Workflow State Field (Phase 3.3 preview)

Phase 3.3 introduces `deployment_targets.workflow_state JSON` to track
multi-step deployment strategies. PS-3 (canary pending promotion) is
expressed there, not in the canonical lifecycle state.

```json
{
  "strategy": "canary",
  "phase": "observing",
  "traffic_split": {"new": 5, "old": 95},
  "started_at": "2026-05-14T10:00:00Z",
  "next_evaluation_at": "2026-05-14T10:15:00Z"
}
```

The lifecycle field is `running`; the workflow field carries the
partial-success nuance. UI joins both for the operator.

## The Honesty Rules

### Rule HR-1 — Partial Success Surfaces, Not Hides

Any partial-success state MUST be surfaced to the operator. The UI
SHALL NOT render a target as "fully operational" if any of the
following is true:

- `lifecycle_ambiguous=true`
- `drift_detected=true`
- `workflow_state.phase != "completed"`

This is the operational instantiation of P-5 (UI surfaces ambiguity).

### Rule HR-2 — Partial Success Does NOT Block Reconciliation

A target in partial-success state continues to receive cycles. The
reconciler does not freeze waiting for the partial state to resolve.
Phase 2's P-6 (ambiguity does not block reconciliation) extends to
all partial-success kinds.

### Rule HR-3 — Lineage Records Each Partial Transition

Every entry into and exit from a partial state writes a
`deployment_history` row. Example sequence for PS-3:

```
canary_started     → traffic_split={5,95}
canary_observing   → metrics ok
canary_promoting   → traffic_split={50,50}
canary_promoted    → traffic_split={100,0}
```

G-12 (lineage completeness) extends through these states.

### Rule HR-4 — Partial Success Is Not "Almost-Success"

The lifecycle `error` state is reserved for **operator-action-required**.
Partial success — even "stuck-but-survivable" — is NOT `error`.
Example: a canary that has been observing for 24h without promotion is
NOT `error`. It is `running` with `workflow_state.phase="observing"`
and a warning surfaced.

### Rule HR-5 — The Operator Can Cancel Partial States

Phase 3.3 workflow primitives include cancel paths for partial states:

- Cancel canary → roll back to 100% old revision.
- Cancel blue-green → keep blue, discard green.
- Cancel staged → finish current stage, stop further stages.

Cancellation produces a deterministic terminal state (back to prior
revision or new revision, depending on workflow). These paths are
documented in [[workflow-lifecycle-contracts]].

## The Partial-Success Display Contract (UI)

The operator-facing display contract:

| State | Badge | Color | Tooltip |
|---|---|---|---|
| `running` + no ambiguity, no drift | "Running" | green | none |
| `running` + ambiguous | "Running (ambiguous)" | yellow | renders `lifecycle_native_state` + `lifecycle_ambiguity_detail` |
| `running` + drift | "Running (drift)" | yellow | renders `drift_summary` count + severity |
| `running` + workflow.phase != "completed" | "Running (workflow)" | blue | renders workflow phase + traffic split |
| `running` + multiple flags | "Running (multiple flags)" | red-orange | all relevant tooltips |
| `error` | "Error" | red | renders `message` |
| `deleting` + confirm pending | "Deleting (confirming)" | gray | renders confirm deadline |
| `deleted` + ambiguous (S-4 timeout) | "Deleted (unconfirmed)" | gray | renders timeout reason |

UI never collapses these into "Healthy" alone.

## The Three Anti-Patterns

1. **The All-Green Lie.** Rendering partial-success states as fully
   green to look operational. Forbidden.
2. **The Promotion Auto-Heal.** Automatically promoting a stuck canary
   because "long enough." Forbidden. Operators decide.
3. **The Composite Status.** Defining a new lifecycle state
   (`running-degraded`) to collapse multiple partial flags into one.
   Forbidden per R-6 (lifecycle fragmentation). Use the ambiguity bit
   + workflow state.

## Phase 3.2 Closure Criteria

For this doc:

1. PS-1..PS-6 are catalogued.
2. The honesty rules HR-1..HR-5 are sealed.
3. The display contract table is sealed (handed off to Phase 3.5 UI).
4. The three anti-patterns are flagged for review discipline.

## Related

- [[ambiguity-semantics-model]] — S-1..S-5 ambiguity sources
- [[lifecycle-normalization-model]] — N-1..N-8 mapping rules
- [[provider-drift-model]] — D-1..D-6 drift kinds
- [[workflow-maturity-roadmap]] — Phase 3.3 workflow context
- [[workflow-lifecycle-contracts]] — workflow state vs lifecycle state
- [[../phase2.9/operational-guarantee-matrix]] — G-12, G-14
