# Deployment Strategy Model (Phase 3.3)

**Last Updated:** 2026-05-14
**Phase:** 3.3 (Deployment Workflow Maturity)

## Purpose

The detailed type and contract specification for Phase 3.3 deployment
strategies. Companion to [[workflow-maturity-roadmap]] which carries
the *what*; this doc carries the *how*.

## Strategy Interface

```go
// Strategy describes a multi-step deployment plan. Strategies are
// registered in the strategies registry (parallel to providers); the
// reconciler dispatches a target's current step via the registered
// strategy's step at WorkflowState.StepNumber.
type Strategy interface {
    Name() string                            // "direct", "blue-green", "canary", "staged"
    Steps() []Step                           // immutable per-strategy step list
    RequiredCapabilities() []CapabilityKey   // ALL must be supported by target's provider
    Description() string                     // operator-facing
}
```

## Step Interface

```go
// Step is one atomic action in a Strategy. Each step:
//   - Calls at most one Provider method
//   - Has a deterministic outcome (Eligible + Execute + result)
//   - Writes a deployment_history row for its outcome
type Step interface {
    Name() string

    // Eligible returns true when this step is ready to run. Examples:
    //   - Wait for prior provider call to converge (Ready=SUCCEEDED).
    //   - Wait for an observation window to elapse.
    //   - Wait for operator approval (returns false until approval row exists).
    // Eligible MUST be a pure function of (target, state, now); no side effects.
    Eligible(ctx context.Context, target *Target, state WorkflowState, now time.Time) bool

    // Execute runs the step. It calls a single Provider method and
    // interprets the result. It may NOT call multiple Provider methods.
    // (If you need multiple, that's two steps.)
    Execute(ctx context.Context, target *Target, account *CloudAccount, p Provider) StepResult

    // HaltOnFailure controls whether a failed step halts the strategy
    // (true) or continues (false). Most steps halt; cleanup steps may
    // continue.
    HaltOnFailure() bool

    // Description is the operator-facing single-line summary.
    Description() string
}

type StepResult struct {
    Succeeded     bool
    Message       string
    AmbiguousNote string   // optional, if step produces ambiguity
    NextDeadline  *time.Time  // for observation windows
}
```

## WorkflowState Type

```go
type WorkflowState struct {
    Strategy        string
    Phase           string  // "pending", "running", "observing", "awaiting_approval", "completed", "halted", "rolled_back"
    StepNumber      int
    StepStartedAt   time.Time
    StepDeadlineAt  *time.Time
    TrafficSplit    map[string]int  // revision name → percent; sums to 100
    StepHistory     []StepHistoryEntry
    NextAction      string  // "auto-promote", "operator-approval", "halted-investigate", ""
    StartedBy       string  // user ID
}

type StepHistoryEntry struct {
    StepNumber  int
    StepName    string
    StartedAt   time.Time
    EndedAt     time.Time
    Succeeded   bool
    Message     string
}
```

This serializes to JSON in `deployment_targets.workflow_state`. The
column is read-mostly during a step; updated atomically by the
reconciler when transitions occur.

## The Concrete Strategies

### Direct (Strategy `S-Direct`)

```go
Steps: [
  DeployStep{},  // Provider.Deploy → wait Ready
]
RequiredCapabilities: [CapDeploy]
```

The Phase 2 path expressed as a one-step strategy. Eligible only on a
`pending` target with `workflow_state` empty or `strategy="direct"`.

### Blue-Green (Strategy `S-BlueGreen`)

```go
Steps: [
  DeployStep{},                    // Deploy green revision
  ObserveStep{Duration: 60s},      // Optional brief observation
  TrafficShiftStep{Green: 100, Blue: 0},  // Single cutover
  // Optional: DeleteRevisionStep{Target: blue} — capability-gated
]
RequiredCapabilities: [CapDeploy, CapTrafficSplit]
```

The brief observation window catches obvious post-deploy failures
before traffic shift. Operator-tunable; default 60s.

### Canary (Strategy `S-Canary`)

```go
Steps: [
  DeployStep{},                                                // Deploy canary
  TrafficShiftStep{Canary: 5, Old: 95},
  ObserveStep{Duration: 15min, RequireOperatorApproval: true},
  TrafficShiftStep{Canary: 50, Old: 50},
  ObserveStep{Duration: 15min, RequireOperatorApproval: true},
  TrafficShiftStep{Canary: 100, Old: 0},
]
RequiredCapabilities: [CapDeploy, CapTrafficSplit, CapCanaryRollout]
```

Two operator-approval gates: 5% → 50% and 50% → 100%. Auto-promote
(skipping approval) is Phase 3.7+ work; Phase 3.3 always requires
operator approval.

### Staged (Strategy `S-Staged`)

```go
Steps: [
  DeployStep{TargetIndex: 0},     // Deploy first target
  DeployStep{TargetIndex: 1},     // Then second
  ...                              // Up to N targets
]
RequiredCapabilities: [CapDeploy]
```

Special: this strategy operates across multiple deployment_targets, not
a single target. Phase 3.3 limits N ≤ 5 to keep step count bounded.

## Approval Mechanism

Operator approval is tracked in a new table:

```sql
CREATE TABLE workflow_approvals (
    id TEXT PRIMARY KEY,
    target_id TEXT NOT NULL,
    step_number INT NOT NULL,
    approved BOOLEAN NOT NULL,
    approved_by TEXT,
    approved_at DATETIME,
    notes TEXT,
    UNIQUE (target_id, step_number)
);
```

A canary step with `RequireOperatorApproval: true` is `Eligible: false`
until an approval row exists. The `pkg/controller/workflow.go` provides
endpoints:

- `POST /api/v1/targets/:id/workflow/approve` — current step.
- `POST /api/v1/targets/:id/workflow/reject` — current step (triggers
  rollback).

## Cancellation Mechanism

Strategies expose a cancel path that produces a deterministic terminal
state.

| Strategy | Cancel produces |
|---|---|
| Direct | (no cancel; deploy is atomic) |
| BlueGreen | Keep blue serving; discard green revision |
| Canary | TrafficShift(canary=0%, old=100%); discard canary |
| Staged | Finish current step's deploy; stop further steps |

Cancellation is operator-initiated via `POST /api/v1/targets/:id/workflow/cancel`.
The reconciler executes a strategy-specific "cancel" pseudo-step that
emits provider calls to reverse partial state.

## Strategy Registration

Strategies register at process start:

```go
// pkg/reconciler/strategies/init.go
func init() {
    Register(NewDirectStrategy())
    Register(NewBlueGreenStrategy())
    Register(NewCanaryStrategy())
    Register(NewStagedStrategy())
}
```

`Get(name string) (Strategy, error)` returns by name. Used by the
controller to validate strategy selection at deploy time.

## Provider Capability Gate at Selection

```go
// pkg/controller/workflow.go
func validateStrategy(strategy Strategy, p Provider) error {
    caps := p.Capabilities()
    for _, req := range strategy.RequiredCapabilities() {
        if !caps[req].Supported {
            return ErrCapabilityUnavailable{
                Cap:      req,
                Provider: p.Name(),
                Notes:    caps[req].Notes,
            }
        }
    }
    return nil
}
```

Called at deploy submission. Operator gets a 400 error with the
specific missing capability. No silent fallback to a different strategy.

## The State Transition Diagram

```
[ pending (no workflow) ]
        |
        | operator selects strategy
        v
[ workflow.phase = "pending" ]
        |
        | reconciler claims, executes step 1
        v
[ workflow.phase = "running" / step_number = 1 ]
        |
        +--- step succeeds & more steps ---> step_number++
        |
        +--- step succeeds & last step ---> phase = "completed"
        |
        +--- step requires observation ---> phase = "observing"
        |
        +--- step requires approval ---> phase = "awaiting_approval"
        |
        +--- step fails & HaltOnFailure ---> phase = "halted"
        |
        +--- operator cancels ---> phase = "rolled_back"
```

The lifecycle field (`deployment_targets.status`) tracks the provider
state of the **currently active** revision throughout. Workflow phase is
a separate dimension.

## Phase 3.3 Closure Criteria

1. `Strategy`, `Step`, `WorkflowState`, `StepResult` types defined.
2. Four strategies implemented: Direct, BlueGreen, Canary, Staged.
3. `workflow_approvals` table migration.
4. Strategy registration init code.
5. Controller endpoints for approve/reject/cancel.
6. Reconciler dispatcher handles workflow state per cycle.
7. Tests: each strategy executed end-to-end on Cloud Run; Direct on
   ECS/ACA.
8. Strategy availability matrix in UI.

## What Phase 3.3 Refuses

- **Parallel step execution.** Steps are strictly sequential.
- **Conditional branching within a strategy.** A step is either
  eligible or not; no branching.
- **Cross-strategy state migration.** A target on Canary cannot switch
  mid-flight to BlueGreen. Cancel and restart.
- **Strategies with > 6 steps.** Phase 3.3 enforces a step ceiling.
- **Strategies depending on external systems** (e.g., a step that
  waits for a Slack approval). External systems can call the approve
  endpoint; the strategy step is still just "wait for approval row."

## Related

- [[workflow-maturity-roadmap]] — Phase 3.3 sequencing
- [[workflow-lifecycle-contracts]] — workflow vs lifecycle
- [[rollout-semantics]] — TrafficShiftStep mechanics
- [[rollback-semantics]] — cancellation semantics
- [[partial-success-semantics]] — workflow.phase dimension
- [[provider-capability-matrix]] — capability gating
