# Rollback Semantics (Phase 3.3)

**Last Updated:** 2026-05-14
**Phase:** 3.3 (Deployment Workflow Maturity)

## Purpose

Define **rollback** correctly and provider-aware. Supersedes the Phase 2
stub (`Provider.Rollback` returning `ErrNotImplemented`) and the
Phase 2.9 declination (P-11).

Rollback is the operationally most dangerous action. Inconsistency
across providers causes incidents during incidents. R-7 (rollback
inconsistency) is the primary failure mode this doc prevents.

## What "Rollback" Means

A rollback is **the act of returning a deployment target to a known
prior state**. There are TWO operationally distinct rollback shapes:

### Shape RB-1 — Revision Rollback

Restore the target to serve a specific prior revision. The operator
chooses which revision.

**Provider mechanism varies:**

- Cloud Run: TrafficShift to prior revision (revision still retained).
- ECS: Update service with prior task definition revision.
- ACA: Activate prior revision; deactivate current.

### Shape RB-2 — Workflow Cancellation

Reverse an in-flight workflow's partial effects, returning to the state
before the workflow started.

**Provider mechanism varies by strategy:**

- BlueGreen mid-flight: keep blue, discard green.
- Canary mid-flight: TrafficShift to 0%, discard canary.
- Staged mid-flight: halt at current step (deployed steps remain).

These are **different operations**. The UI distinguishes them.

## The Rollback Capability Profile

| Provider | CapRollback (Phase 3.3) | Semantic |
|---|---|---|
| Cloud Run | true | `traffic-shift-to-revision` |
| ECS (without CodeDeploy) | true | `update-service-with-prior-task-def` |
| ECS (with CodeDeploy) | true | `codedeploy-rollback-deployment` (Phase 3.3 stretch) |
| ACA | true | `revision-activate` |

All three Phase 3 providers can implement rollback. The semantic
differs; the capability flag declares it.

## The Revision Rollback Contract

### Inputs

```go
type RollbackRequest struct {
    TargetID       string
    TargetRevision string  // empty = "previous", else explicit revision name/ID
    Strategy       string  // "instant" or "gradual" (Phase 3.3.1)
}
```

### Outputs

```go
type RollbackResult struct {
    Succeeded         bool
    PriorRevision     string  // the revision before rollback
    NowServing        string  // the revision now serving (the rollback target)
    Message           string
    OperationID       string  // for tracking
}
```

### The Lifecycle During Rollback

Rollback follows the deploy lifecycle (DC-1.1) but with a known target
revision instead of a new one:

```
running → claim → updating → [Provider.Rollback / Provider.Deploy with prior-rev spec]
       → wait for ready → success → release(updating, running)
       → next poll → ServingRevision reflects target
```

The lifecycle transitions are exactly the standard deploy flow. The
**semantic** difference is that no new revision is created — the
provider's existing revision is selected.

### The Capability-Aware Dispatch

```go
// pkg/reconciler/rollback.go (Phase 3.3)
func executeRollback(ctx, target, req RollbackRequest) error {
    p, _ := providers.GetProvider(target.Provider)
    caps := p.Capabilities()
    if !caps[CapRollback].Supported {
        return ErrCapabilityUnavailable{
            Cap:      CapRollback,
            Provider: target.Provider,
            Notes:    caps[CapRollback].Notes,
        }
    }

    // Provider.Rollback is the canonical entry point.
    result, err := p.Rollback(ctx, account, target, req.TargetRevision)
    // ... standard claim/heartbeat/release/lineage
}
```

## The Per-Provider Rollback Implementation

### Cloud Run (semantic: `traffic-shift-to-revision`)

```go
// pkg/providers/cloudrun/rollback.go (Phase 3.3 replaces the ErrNotImplemented stub)
func (p *Provider) Rollback(ctx, account, target, targetRevision string) (*DeployResult, error) {
    if targetRevision == "" {
        targetRevision = p.findPreviousRevision(ctx, target)
    }
    // Use Service.Traffic to route 100% to targetRevision
    // ... API call
}
```

Cloud Run rollback is **gradual** (technically supports percent-weighted
shifts, but Phase 3.3 default is 100% cutover). Phase 3.3.1 may add
gradual rollback via a `Strategy: "gradual"` parameter.

### ECS (semantic: `update-service-with-prior-task-def`)

```go
// pkg/providers/ecs/rollback.go
func (p *Provider) Rollback(ctx, account, target, targetRevision string) (*DeployResult, error) {
    if targetRevision == "" {
        targetRevision = p.findPreviousTaskDefRevision(ctx, target)
    }
    // UpdateService with the prior task definition ARN
    // ... API call
}
```

ECS rollback is **cutover** (immediate replacement, no traffic split
without CodeDeploy). UI surfaces this as "instant cutover" rollback.

### ACA (semantic: `revision-activate`)

```go
// pkg/providers/aca/rollback.go
func (p *Provider) Rollback(ctx, account, target, targetRevision string) (*DeployResult, error) {
    if targetRevision == "" {
        targetRevision = p.findPreviousRevision(ctx, target)
    }
    // Activate prior revision, deactivate current. ACA-native operation.
}
```

ACA rollback is **gradual via traffic weight**; defaults to 100% cutover
to prior revision.

## The Revision Lineage Requirement

Rollback requires AutoStack to **know** which revisions exist. Phase 3
adds:

```sql
CREATE TABLE deployment_revisions (
    id TEXT PRIMARY KEY,
    target_id TEXT NOT NULL,
    revision_name TEXT NOT NULL,        -- provider-native name
    deployed_at DATETIME NOT NULL,
    deployed_spec TEXT,                  -- snapshot for drift comparison (links to SC-1)
    rolled_out_to_100_at DATETIME,       -- when this revision became fully traffic-receiving
    deactivated_at DATETIME,             -- when revision was no longer serving
    UNIQUE (target_id, revision_name)
);
```

Every successful deploy writes a row. Rollback queries this table to
present options to the operator:

```
[Rollback panel]
Current: myapp-rev-v1.3.2 (deployed 2026-05-14T09:00)
Roll back to:
  ○ myapp-rev-v1.3.1 (deployed 2026-05-13T15:00)
  ○ myapp-rev-v1.3.0 (deployed 2026-05-10T11:00)
  ○ Specific revision: [           ]
```

## The Rollback History Contract

Rollback writes specific lineage rows:

```
status=rollback_started, message=<target_revision>
status=rollback_succeeded, message=now_serving=<rev>
status=rollback_failed, message=<reason>
```

These are distinct from deploy events. Forensic reconstruction
distinguishes "this was a rollback" from "this was a normal deploy."

## Workflow Cancellation Rollback (Shape RB-2)

For active workflows, cancellation is a **separate code path** from
revision rollback. The workflow's `Cancel()` method on the Strategy:

```go
type Strategy interface {
    // ... existing
    Cancel(ctx context.Context, target *Target, state WorkflowState) ([]Step, error)
}
```

Returns a sequence of steps that produce the cancellation. Per strategy:

- Direct: returns `[]` (no cancel; deploy is atomic).
- BlueGreen: returns `[TrafficShiftStep{blue=100, green=0}]`.
- Canary: returns `[TrafficShiftStep{canary=0, old=100}]`.
- Staged: returns `[]` (halt where you are).

The reconciler executes these steps in the same CAS-claimed manner as
forward steps. Lineage records:

```
status=workflow_cancel_started
status=workflow_cancel_step_started
status=workflow_cancel_step_succeeded
status=workflow_cancelled
```

## The Honesty Contract

### Honesty H-1 — Rollback Confirms Target Revision Exists

Before executing rollback, AutoStack queries the provider OR the
`deployment_revisions` table to confirm the target revision exists.
Rollback to a non-existent revision returns
`ErrInvalidRollbackTarget` BEFORE any provider mutation.

### Honesty H-2 — Rollback Surfaces The Operational Profile

The UI rollback panel shows the operational profile of the rollback:

- Cloud Run: "Gradual via traffic targeting"
- ECS: "Instant cutover" (or "Gradual via CodeDeploy" if configured)
- ACA: "Gradual via revision traffic weights"

The operator picks rollback knowing the operational shape (R-7
mitigation).

### Honesty H-3 — Rollback Does Not Recreate Deleted Revisions

If the target revision has been auto-garbage-collected by the provider
(e.g., ACA dropped it past retention), rollback REFUSES with a clear
error. AutoStack does NOT attempt to recreate the revision from
`deployed_spec` (that's a forward deploy, not a rollback, and the
operator should know).

Phase 3 deferred: a "redeploy from saved spec" feature that explicitly
forwards-deploys a saved spec when a revision is no longer available.
That is distinct from rollback.

### Honesty H-4 — Rollback Is Audit-Logged

Every rollback action writes `deployment_history` rows AND a
structured log:

```
[ROLLBACK_INITIATED] cycle=<id> target=<id> from=<current_rev> to=<target_rev> initiator=<user>
[ROLLBACK_COMPLETED] cycle=<id> target=<id> now_serving=<rev>
[ROLLBACK_FAILED] cycle=<id> target=<id> reason=<sanitized>
```

These are forensically reconstructible.

## Phase 3.3 Closure Criteria

1. `Provider.Rollback` implemented in Cloud Run, ECS, ACA — no more
   `ErrNotImplemented` for these providers.
2. `deployment_revisions` table migration.
3. `pkg/reconciler/rollback.go` with capability-aware dispatch.
4. UI rollback panel listing prior revisions with operational profile.
5. Cancellation path implemented for each strategy.
6. Lineage rows for rollback events.
7. Tests: revision rollback per provider, workflow cancellation per
   strategy, lineage assertion.

## What Phase 3.3 Refuses

- **Refuse:** Cross-provider rollback (rollback a target while changing
  its provider). Not a thing.
- **Refuse:** Rollback to a revision not in `deployment_revisions`
  (per H-3).
- **Refuse:** Auto-rollback based on health metrics. Operator-initiated
  only.
- **Refuse:** Rollback during an active workflow. Workflow must be
  cancelled first (or completed); then rollback as a separate action.

## Related

- [[deployment-strategy-model]] — Cancel() method on Strategy
- [[workflow-maturity-roadmap]] — strategy inventory
- [[rollout-semantics]] — traffic shifts
- [[provider-capability-matrix]] — CapRollback semantic profile
- [[multi-provider-risk-analysis]] — R-7 mitigation
- [[../phase2.9/provider-contracts]] — P-11 (declined in Phase 2, now lifted)
