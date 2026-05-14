# Workflow Lifecycle Contracts (Phase 3.3)

**Last Updated:** 2026-05-14
**Phase:** 3.3 (Deployment Workflow Maturity)

## Purpose

Define **how workflows interact with the Phase 2 lifecycle contracts
without replacing them**. The lifecycle stays canonical (DC-1..DC-8
from [[../phase2.9/lifecycle-contracts]]); workflows are an additional
dimension that adds observability and operator control without weakening
lifecycle guarantees.

This doc is the **non-regression contract** for Phase 3.3 workflow work.

## The Two-Dimensional Truth

A Phase 3.3 deployment_target carries TWO state dimensions:

| Dimension | Field | Meaning | Phase |
|---|---|---|---|
| Lifecycle | `status` | Provider-observed state of the currently active revision | Phase 2 baseline |
| Workflow | `workflow_state.phase` | Multi-step strategy progress | Phase 3.3 new |

These are **orthogonal**. A target can be:

- `status=running` + `workflow.phase=observing` (canary mid-rollout)
- `status=creating` + `workflow.phase=running` (deploy step in flight)
- `status=error` + `workflow.phase=halted` (deploy step failed)
- `status=running` + `workflow.phase=completed` (strategy done)
- `status=running` + `workflow.phase=""` (no workflow active)

The UI surfaces both. The reconciler honors both. Neither replaces the
other.

## The Non-Regression Contract

Phase 3.3 work must preserve **every** Phase 2 lifecycle guarantee.

### NR-1 — DC-1.1..DC-1.5 (Deploy Lifecycle) unchanged

Deploy success, error, hard error, stale, and crash semantics are
identical to Phase 2. A workflow step that calls `Provider.Deploy`
goes through the existing dispatch path, with the existing CAS claim,
heartbeat, sweep, and lineage.

### NR-2 — DC-2.1..DC-2.3 (Destroy Lifecycle) unchanged

Destroy semantics are unchanged. A workflow does NOT call Destroy
internally; cancellation produces traffic reversion or revision discard,
not target destroy. Target-level destroy follows the Phase 2 path.

### NR-3 — DC-3 (Replay Contracts) extended

Restart with in-flight workflow step:

- The op for the in-flight provider call is reclaimed per Phase 2 sweep
  rules (G-3).
- The workflow state is read at next cycle and resumed from the same
  step number.
- If the prior op was marked `failed`, the strategy's `HaltOnFailure`
  rule applies.

**New contract:** Workflow state is **deterministic across restart**.
The workflow state row + the operation row + the deployment_history rows
together reproduce the exact strategy execution path.

### NR-4 — DC-4 (Ownership Semantics) extended

The CAS claim applies per-step. A step is one Provider call with the
existing claim mechanism. Workflow phase transitions write
`workflow_state` atomically; they are NOT a new claim mechanism.

Phase 3.3 does NOT introduce a "workflow lock" or "workflow leader."
The CAS claim per provider call remains the only ownership primitive.

### NR-5 — DC-5 (Convergence Guarantees) extended

Each step has its own convergence guarantee:

| Step type | Converges via |
|---|---|
| DeployStep | Provider.Deploy convergence (DC-1.1) |
| TrafficShiftStep | Provider.Deploy via spec.traffic; converges per DC-1.1 |
| ObserveStep | Time-bounded; deadline drives transition |
| ApprovalStep (variant of ObserveStep) | Operator action or operator-set deadline |
| DestroyStep (Staged-strategy cleanup) | Provider.Destroy convergence (DC-2.1) |

The strategy converges when its last step converges.

**Non-convergence:** A strategy halted by `HaltOnFailure=true` is
non-convergent at the strategy level; the lifecycle is `error` and
operator action is required. The strategy phase is `halted`.

### NR-6 — DC-6 (Retry Semantics) unchanged

Step retries follow Phase 2 retry semantics. A `FailureTransient`
step retries; a `FailureAuth` step does not. The strategy does NOT
introduce new retry semantics.

### NR-7 — DC-7 (Ambiguity Contracts) extended

Workflow introduces new honest-ambiguity surfaces:

| Surface | Source | Meaning |
|---|---|---|
| `workflow.phase=observing` | strategy normal flow | Step waiting for observation window |
| `workflow.phase=awaiting_approval` | strategy normal flow | Step waiting for operator decision |
| `workflow.phase=halted` | step failed | Operator intervention required |

These are surfaced via UI, not via the lifecycle ambiguity bit.
[[partial-success-semantics]] HR-1 enforces visibility.

### NR-8 — DC-8 (Lineage Contracts) extended

Workflow transitions write `deployment_history` rows:

| Event | History row |
|---|---|
| Strategy start | `status=workflow_started, message=<strategy>` |
| Step start | `status=step_started, message=<step name>` |
| Step success | `status=step_succeeded, message=<step name>` |
| Step failure | `status=step_failed, message=<reason>` |
| Phase transition | `status=workflow_phase_<phase>` |
| Operator approval | `status=operator_approved, message=<user>` |
| Operator reject | `status=operator_rejected, message=<user>` |
| Cancellation | `status=workflow_cancelled, message=<reason>` |

G-12 (lineage completeness) extends through workflows. Any workflow
event not represented here is a contract violation.

## The Workflow-Lifecycle Interaction Rules

### Rule I-1 — Workflow Cannot Force Lifecycle State

A workflow phase change does NOT directly write to
`deployment_targets.status`. The lifecycle is provider-observed; the
workflow rides on top.

Example: `workflow.phase=completed` does NOT imply `status=running`.
The lifecycle is determined by Provider.GetStatus. The two can disagree
briefly (eventual consistency).

### Rule I-2 — Lifecycle `error` Halts Workflow

When `status` transitions to `error`, the workflow phase transitions to
`halted` automatically. The strategy does NOT continue executing steps
after lifecycle error.

This is a one-way coupling: lifecycle error halts workflow, but workflow
halt does not force lifecycle error (a step failure might be recoverable
by retry).

### Rule I-3 — Workflow Cancellation Triggers Lifecycle Transitions

Operator-initiated cancellation produces strategy-specific lifecycle
effects:

- Canary cancel → TrafficShift(canary=0) → lifecycle stays `running`
  (old revision serves) → workflow=`rolled_back`.
- Staged cancel → halts mid-step → already-deployed targets stay as is
  → not-yet-deployed targets stay `pending` → workflow=`cancelled`.

### Rule I-4 — Lifecycle Destroy Cancels Active Workflow

An operator setting `endDate` on a target with an active workflow
(`workflow.phase != "completed"`) causes:

1. Workflow phase transitions to `cancelled` with history row.
2. AW-C1 auto-promote (Phase 2.9) routes target to `deleting`.
3. Destroy dispatch follows Phase 2.8 confirm semantics.

### Rule I-5 — Replay From Workflow Step Is Deterministic

Restart during a workflow step:

- If the step's Provider call had heartbeat: op preserved per Phase 2
  G-3; workflow phase unchanged; step continues.
- If the step's Provider call never heartbeated: op marked `failed`;
  step result is "failed"; HaltOnFailure rule applies; workflow phase
  transitions accordingly.

The workflow's deterministic resume is from the same `step_number`. The
step's `Eligible` predicate may re-evaluate (e.g., observation deadline
shifted by the restart duration); this is handled per-step.

## The Provider-Workflow Interaction

Workflows USE provider capabilities; providers are unaware of workflows.
This is a deliberate separation:

- The provider does not know whether a Deploy is part of a Direct or
  Canary strategy. It just gets a `DeploySpec` and returns `DeployResult`.
- The workflow does not know provider-internal state beyond what
  `GetStatus` returns.
- Cross-provider strategies work identically because the strategy
  composes the same provider operations regardless.

## The Audit Surface

Phase 3.3 introduces these log tags:

- `[WORKFLOW_START]` — strategy started.
- `[WORKFLOW_STEP_START]` — step started.
- `[WORKFLOW_STEP_END]` — step finished.
- `[WORKFLOW_PHASE]` — phase transition.
- `[WORKFLOW_APPROVAL]` — operator approved/rejected.
- `[WORKFLOW_CANCEL]` — cancellation initiated.
- `[WORKFLOW_HALT]` — strategy halted on failure.

All carry `cycle_id` (G-13) and `workflow_step` for grep correlation.

## Phase 3.3 Closure Criteria

1. Non-regression contracts NR-1..NR-8 are pinned via regression test
   suite.
2. Workflow-lifecycle interaction rules I-1..I-5 are coded into the
   strategy dispatcher.
3. Replay test: kill the reconciler mid-step, restart, assert workflow
   resumes deterministically.
4. Audit-tag coverage test: every workflow event has a tagged log.

## Related

- [[workflow-maturity-roadmap]] — strategy inventory
- [[deployment-strategy-model]] — types and dispatchers
- [[rollout-semantics]] — traffic-shift step contract
- [[rollback-semantics]] — cancellation effects
- [[partial-success-semantics]] — workflow phase as partial-success
- [[../phase2.9/lifecycle-contracts]] — DC-1..DC-8 baseline
- [[../phase2.9/operational-guarantee-matrix]] — G-1..G-19 baseline
