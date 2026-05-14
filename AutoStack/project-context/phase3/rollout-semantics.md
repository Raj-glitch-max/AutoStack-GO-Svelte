# Rollout Semantics (Phase 3.3)

**Last Updated:** 2026-05-14
**Phase:** 3.3 (Deployment Workflow Maturity)

## Purpose

Define what **traffic-shift**, **pause**, **resume**, and **cancel**
mean concretely in AutoStack — across providers — including the
honest acknowledgment of where providers differ.

Rollout actions are the most semantically loaded operations in a
deployment platform. R-7 (rollback inconsistency) is the failure mode
this doc exists to prevent.

## The Four Rollout Actions

| Action | Operator intent | What AutoStack does |
|---|---|---|
| Traffic shift | "Move N% of traffic to new revision" | Provider.Deploy with TrafficSpec, or Provider.UpdateTraffic if supported |
| Pause | "Halt the rollout where it is; do not promote further" | Workflow phase → `paused`; no provider call |
| Resume | "Continue the rollout from current position" | Workflow phase → `running`; next eligible step proceeds |
| Cancel | "Reverse the rollout; restore prior state" | Strategy-specific reversal — see [[rollback-semantics]] |

These actions live at the **workflow layer**, not the lifecycle layer.
The lifecycle remains the canonical state of the currently active
revision.

## Traffic Shift Semantics

### Across Providers

| Provider | Mechanism | Semantic |
|---|---|---|
| Cloud Run | `Service.Traffic[]` with revision name + percent | gradual-percentage-weighted |
| ECS | CodeDeploy deployment with traffic shifting (when configured) | gradual-or-cutover-by-CodeDeploy-config |
| ACA | `Revision.TrafficWeight` array | gradual-percentage-weighted-native |

Per [[provider-normalization-rules]] decision tree:

- Q5 (capability overlap) — different traffic mechanisms.
- Treatment: **EXPOSE**. Cloud Run and ACA can shift gradually
  natively; ECS requires CodeDeploy infrastructure. The capability
  matrix declares this.

### The Honest Surface

A canary at 5% traffic is **identical operationally** across providers
that support it. But:

- **Cloud Run:** Traffic shift is atomic from the provider's
  perspective (Traffic array update is a single API call). The 5%
  shift happens immediately on the next request.
- **ACA:** Same — atomic per-revision traffic weight update.
- **ECS:** Requires CodeDeploy. AutoStack does NOT initiate
  CodeDeploy in Phase 3.3 baseline; capability declared
  `Supported: false` for ECS.

Operators see "Canary 5%" on Cloud Run and ACA. On ECS, the canary
strategy is refused at deploy time.

## TrafficShiftStep Implementation

```go
// pkg/reconciler/strategies/traffic_shift_step.go
type TrafficShiftStep struct {
    Shifts map[string]int  // revision_name → percent (sums to 100)
}

func (s TrafficShiftStep) Execute(ctx, target, account, p Provider) StepResult {
    caps := p.Capabilities()
    if !caps[CapTrafficSplit].Supported {
        return StepResult{
            Succeeded: false,
            Message: "traffic split not supported; should have been caught at strategy validation",
        }
    }

    // Construct a DeploySpec that requests the traffic state
    spec := &DeploySpec{
        RolloutID: target.Rollout.ID,
        ...
        Traffic: convertShifts(s.Shifts),  // new Phase 3.3 field in DeploySpec
    }

    result, err := p.Deploy(ctx, account, spec)
    if err != nil {
        return StepResult{Succeeded: false, Message: err.Error()}
    }

    // Update workflow state with new traffic split
    target.WorkflowState.TrafficSplit = s.Shifts
    return StepResult{Succeeded: true}
}
```

A new field `DeploySpec.Traffic` (Form B additive) carries the desired
traffic state. Phase 2 callers leave it nil; Phase 3 callers populate
it. Providers without `CapTrafficSplit` ignore it (and the strategy
that produced it would have been refused at selection).

## Pause Semantics

Pause is an operator action against an in-flight workflow:

```
POST /api/v1/targets/:id/workflow/pause
```

Effect:

1. Workflow phase transitions from `running`/`observing` to `paused`.
2. No provider call. The current traffic state is preserved.
3. A history row records `[WORKFLOW_PAUSED]`.
4. The reconciler does NOT advance to next step.

Pause is **idempotent**: pausing an already-paused workflow is a no-op.

## Resume Semantics

```
POST /api/v1/targets/:id/workflow/resume
```

Effect:

1. Workflow phase transitions from `paused` to `running`.
2. Reconciler evaluates the current step's `Eligible` predicate on
   the next cycle.
3. If eligible, the step executes.
4. History row records `[WORKFLOW_RESUMED]`.

Resume from a paused observation window: the deadline is **recomputed**
from now (paused duration doesn't count toward observation time).

## Cancel Semantics

Cancel triggers strategy-specific reversal. Detailed in [[rollback-semantics]].

Brief summary:

- Direct: no cancel (atomic deploy).
- BlueGreen: TrafficShift(blue=100, green=0) → discard green revision.
- Canary: TrafficShift(canary=0, old=100) → discard canary.
- Staged: halt at current step; do not roll back already-deployed.

## Auto-Promote vs Operator-Approve

Phase 3.3 ships **operator-approve only**. Auto-promote requires:

- `CapMetricsVisibility=Supported: true` (Phase 3.7+).
- Promotion criteria (error rate, p99 latency, etc.) — Phase 3.7.

Until then, every canary phase transition requires explicit operator
approval. The UI clearly distinguishes:

```
[Canary at 5% traffic]
Observation period ending in 12:34
[ Approve and continue to 50% ]
[ Reject and roll back ]
[ Pause ]
```

This is operator-centric on purpose. Auto-promotion of canaries based on
incomplete metric signals is a class of failure that has caused real
incidents at many platforms.

## Provider-Side Traffic Shift Caveats

### Cloud Run

- Traffic array allows up to 5 entries. AutoStack workflows produce at
  most 2 (canary + old). Safe.
- Cloud Run can route by revision **tag** (e.g., `canary` tag). Phase
  3.3 uses revision **name** only. Tag-based routing is Phase 3.5+.
- Revision becomes inactive when traffic=0 for a configured period.
  Operators wanting to preserve old revision indefinitely must NOT
  use full canary-to-100 cutover; they should retain at least 1%
  traffic to keep it warm. AutoStack's BlueGreen strategy informs the
  operator.

### ACA

- Revision retention is limited by environment config (default ~100
  revisions retained, configurable). Old revisions are auto-deactivated.
- Traffic weights persist across deploys; AutoStack must always re-send
  the full traffic state in each Deploy.

### ECS (when CapTrafficSplit becomes true via CodeDeploy)

- Linear and canary deployments via CodeDeploy require pre-provisioned
  CodeDeploy app and deployment group. Phase 3.3 baseline declines.
- Phase 3.3 stretch: implement.

## The Phase 3.3 Rollout Test Matrix

For each (strategy, provider) combination that's supposed to work:

```
TestRollout_<Strategy>_<Provider>:
  1. Deploy initial revision → assert status=running
  2. Start strategy with new revision
  3. For each step:
     a. Assert workflow.phase reflects step
     b. Wait for Eligible
     c. Apply operator-approval if required
  4. Assert final state: lifecycle=running, workflow.phase=completed
  5. Issue cancel mid-flight → assert correct reversion
```

These tests run against real provider accounts in test mode. Phase 3.3
landing without these tests is incomplete.

## Phase 3.3 Closure Criteria for This Doc

1. The four rollout actions (shift, pause, resume, cancel) are
   contracted.
2. `DeploySpec.Traffic` field is added (Form B).
3. TrafficShiftStep is implemented and tested.
4. Pause/resume endpoints exist.
5. Per-provider traffic caveats are surfaced in UI tooltips.

## Related

- [[deployment-strategy-model]] — Step / Strategy types
- [[workflow-maturity-roadmap]] — strategy list
- [[workflow-lifecycle-contracts]] — non-regression contracts
- [[rollback-semantics]] — cancel detail
- [[provider-capability-matrix]] — CapTrafficSplit, CapCanaryRollout
- [[provider-normalization-rules]] — EXPOSE treatment for traffic mechanisms
- [[multi-provider-risk-analysis]] — R-7 mitigation
