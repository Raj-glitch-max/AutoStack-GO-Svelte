# Workflow Maturity Roadmap (Phase 3.3)

**Last Updated:** 2026-05-14
**Phase:** 3.3 (Deployment Workflow Maturity)

## Purpose

Roadmap for evolving AutoStack from **single-step deployment execution**
into **workflow-capable orchestration**, WITHOUT becoming a workflow
engine.

R-10 (workflow sprawl) is the failure mode this roadmap exists to
prevent. The strict ceiling: if a workflow needs ≥ 5 sequential steps,
it goes through architectural review.

## What "Workflow Maturity" Means In Phase 3

The Phase 2 deployment model:

```
DeploySpec → Provider.Deploy → result
```

The Phase 3.3 deployment model:

```
DeploySpec + Strategy → workflow plan (1..N steps) → executed step-by-step
                                                  ↓
                                          each step is a Provider call
                                          managed by the existing reconciler
```

The reconciler is unchanged. Strategies decompose a deploy into a
sequence of provider calls. The reconciler dispatches each call using
existing CAS semantics.

## The Four Strategies

Phase 3.3 lands four deployment strategies. Each is a composition of
existing provider primitives — not a new orchestration layer.

### S-Direct (Phase 2 baseline)

The status quo. One `Deploy` call; success when provider converges.

```
Step 1: Provider.Deploy(...) → wait for Ready
```

Capabilities required: `CapDeploy`. Available on every provider.

### S-BlueGreen

Provision new (green) alongside old (blue), then switch traffic.

```
Step 1: Provider.Deploy(spec_green) → wait for Ready
Step 2: Provider.UpdateTraffic(green=100%, blue=0%) → wait for converge
Step 3: Provider.DeleteRevision(blue) [optional / capability-gated]
```

Capabilities required: `CapDeploy`, `CapTrafficSplit`, `CapRevisionHistory`.

Refused if any required capability is absent (e.g., ECS without
CodeDeploy → `ErrCapabilityUnavailable`).

### S-Canary

Provision new alongside old; gradually shift traffic; observation
windows; promote-or-rollback.

```
Step 1: Provider.Deploy(spec_canary) → wait for Ready
Step 2: Provider.UpdateTraffic(canary=5%, old=95%) → observe N minutes
Step 3: Operator approval / auto-promote → UpdateTraffic(canary=50%, old=50%) → observe
Step 4: Operator approval / auto-promote → UpdateTraffic(canary=100%, old=0%)
Step 5: Provider.DeleteRevision(old) [optional]
```

Capabilities required: `CapDeploy`, `CapTrafficSplit`, `CapCanaryRollout`.

Step 3 evaluation is the **explicit human decision point** by default.
Auto-promote based on metrics is Phase 3.7+ (requires
`CapMetricsVisibility`).

### S-Staged

Deploy to N regions or N targets sequentially with halting on failure.

```
Step 1: Deploy to target_1 → wait for Ready
Step 2: Deploy to target_2 → wait for Ready
   ... (halts if any step fails)
```

This is a multi-target strategy; not provider-specific. Implemented in
reconciler workflow layer, not provider.

## The Strategy Storage Model

A new column `deployment_targets.workflow_state JSON` stores
strategy execution state:

```json
{
  "strategy": "canary",
  "phase": "observing",
  "step_number": 2,
  "step_started_at": "2026-05-14T10:00:00Z",
  "step_deadline_at": "2026-05-14T10:15:00Z",
  "traffic_split": {"canary": 5, "old": 95},
  "next_action": "auto-promote OR operator-approval",
  "step_history": [
    {"step": 1, "phase": "deployed", "duration_ms": 45000},
    {"step": 2, "phase": "observing", ...}
  ]
}
```

This field is **read-mostly during a step** and **updated by the
reconciler on step transitions**. The reconciler's CAS claim mechanism
(F-2) protects against double-execution; the workflow layer rides on
top.

## The Strategy Dispatch

The reconciler's dispatch path gains a strategy-aware layer:

```go
// pkg/reconciler/strategy.go (Phase 3.3)
func executeStrategyStep(ctx, target *Target, account *Account) {
    state := target.WorkflowState
    strategy := strategies[state.Strategy]
    step := strategy.Steps[state.StepNumber]

    if !step.Eligible(ctx, target, state) {
        return  // not ready (e.g., observation window not elapsed)
    }

    result := step.Execute(ctx, target, account)

    if result.Failed && step.HaltOnFailure {
        target.WorkflowState.Phase = "halted"
        target.Status = "error"
        return
    }

    if result.Succeeded {
        state.StepNumber++
        if state.StepNumber >= len(strategy.Steps) {
            state.Phase = "completed"
        } else {
            state.Phase = "running"
        }
    }
    persistWorkflowState(target, state)
}
```

The `Step` interface is:

```go
type Step interface {
    Name() string
    Eligible(ctx, target, state) bool
    Execute(ctx, target, account) StepResult
    HaltOnFailure() bool
    Description() string  // for lineage
}
```

Steps compose existing Provider methods. No new provider methods are
introduced by the workflow layer.

## The Capability-Strategy Matrix

Which strategies are available on which providers, derived from
capability profiles:

| Strategy | Cloud Run | ECS | ACA | Required capabilities |
|---|---|---|---|---|
| Direct | ✅ | ✅ | ✅ | CapDeploy |
| BlueGreen | ⚠️ (3.3) | ⚠️ (3.3, via CodeDeploy) | ✅ (native traffic) | CapDeploy + CapTrafficSplit |
| Canary | ⚠️ (3.3) | ⚠️ (3.3) | ✅ | CapDeploy + CapTrafficSplit + CapCanaryRollout |
| Staged | ✅ | ✅ | ✅ | CapDeploy (multi-target only) |

**Important:** A strategy unavailable on a provider is REFUSED at
selection time with `ErrCapabilityUnavailable`, not silently degraded.
The operator gets a clear "this provider doesn't support canary"
message, not a fake-canary that's actually a direct deploy.

## The Phase 3.3 Implementation Order

Strategies land sequentially to manage R-10 risk:

1. **S-Direct + S-Staged** — straight-line implementations, no new
   provider capabilities needed. Validate workflow_state column and
   step dispatcher.
2. **S-BlueGreen** — requires the strategy-step dispatch to handle
   traffic transitions. Cloud Run first (provider-native traffic);
   then ACA.
3. **S-Canary** — adds operator approval gates and observation windows.

ECS Canary/BlueGreen requires CodeDeploy integration and is deferred
to Phase 3.3 stretch goal.

## The Operator UX

Strategy is selected at deploy time:

```
[Deploy panel]
Image: myapp:v1.2.3
Strategy: [Direct ▼]
          ├─ Direct (always available)
          ├─ Blue-Green (Cloud Run ✓ / ECS ✗ — capability not supported)
          ├─ Canary (Cloud Run ✓ / ECS ✗)
          └─ Staged (always available, multi-target)
```

Strategy availability is queried from the capability matrix. Unsupported
strategies are disabled with explanatory tooltip.

## The Anti-Patterns This Doc Forbids

1. **Workflow Engine Resurrection.** Don't build a DAG executor with
   conditional branches, parallel steps, and dependency graphs.
   Strategies are linear, bounded, and explicit. If a use case
   requires DAGs, the use case is out of scope for Phase 3.
2. **Hidden Step Compression.** A strategy with 12 steps is wrong.
   Either reduce to ≤ 4 steps, decompose into multiple smaller
   strategies, or escalate to architectural review.
3. **Cross-Strategy State.** A strategy's `workflow_state` is private
   to that target. No "global workflow" notion. No cross-target
   coordination beyond Staged's explicit sequencing.
4. **Workflow As Lifecycle.** The workflow phase is NOT a lifecycle
   state. The lifecycle stays canonical (`pending`, `creating`,
   `running`, ...). The workflow phase is a separate dimension.

## Phase 3.3 Closure Criteria

1. `deployment_targets.workflow_state` column migration.
2. `pkg/reconciler/strategy.go` exists with `Step` interface +
   dispatcher.
3. S-Direct (regression-test the Phase 2 path under the new dispatcher).
4. S-BlueGreen on Cloud Run.
5. S-Canary on Cloud Run with operator approval gate.
6. S-Staged across multiple targets.
7. Strategy availability matrix surfaced in UI.
8. Lineage records each step transition.

## Related

- [[deployment-strategy-model]] — Step / Strategy types in detail
- [[workflow-lifecycle-contracts]] — workflow vs lifecycle separation
- [[rollout-semantics]] — traffic-shift mechanics
- [[rollback-semantics]] — Phase 3.3 rollback (supersedes Phase 2 stub)
- [[partial-success-semantics]] — workflow phase as partial-success dimension
- [[provider-capability-matrix]] — capability gating
- [[multi-provider-risk-analysis]] — R-10 mitigation
