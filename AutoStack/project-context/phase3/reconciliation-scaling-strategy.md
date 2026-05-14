# Reconciliation Scaling Strategy (Phase 3.4)

**Last Updated:** 2026-05-14
**Phase:** 3.4 (Scalable Reconciliation Foundations)

## Purpose

Detail-level companion to [[reconciliation-scaling-foundations]]. This
doc covers **scheduling**, **fairness**, **prioritization**, **batching**,
and **per-provider concurrency isolation** under the Phase 3.4
worker-pool model.

## Scheduling: When Does A Target Get Picked Up?

The scheduling pipeline:

```
1. Tick fires (every 30s).
2. SelectTargets() returns the eligible target set.
3. PrioritizeTargets() orders the set.
4. Push to queue.
5. Workers Pop and process.
```

### SelectTargets

Reads `deployment_targets` joined with `rollouts` and filters:

- `status IN ('pending', 'creating', 'updating', 'running', 'deleting')`
- `cloud_account.status = 'active'`
- `circuit_state.open = false` OR `circuit_state.next_retry_at <= now()`
- `target_backoff.next_retry_at <= now()`

A target excluded from selection is skipped this cycle, included next.

### PrioritizeTargets

Phase 3.4 ships a simple priority function. Future Phase 3.7+ may
refine.

```go
func priority(target *Target, now time.Time) int {
    // Lower is higher priority.
    base := 100

    // 1. Operator-visible states first (operator is waiting).
    if target.Status == "pending" || target.Status == "deleting" {
        base -= 50
    }

    // 2. In-flight workflows (Phase 3.3) — observation deadlines drive priority.
    if target.WorkflowState != nil && target.WorkflowState.StepDeadlineAt != nil {
        timeToDeadline := target.WorkflowState.StepDeadlineAt.Sub(now)
        if timeToDeadline < 60*time.Second {
            base -= 30
        }
    }

    // 3. Targets in `running` are stable; lower priority.
    if target.Status == "running" {
        base += 10
    }

    return base
}
```

Sort ascending; push to queue. Phase 3.4 priority is a hint; workers
pop in queue order regardless (channel is FIFO). To honor priority
strictly, Phase 3.5+ may switch to a priority queue.

## Fairness: Per-Provider Concurrency

Without isolation, one slow provider can monopolize all workers. Phase
3.4 enforces per-provider in-flight caps:

```go
type ProviderConcurrencyLimit struct {
    Provider string
    MaxConcurrent int
}

var providerConcurrencyLimits = map[string]ProviderConcurrencyLimit{
    "gcp-cloudrun": {Provider: "gcp-cloudrun", MaxConcurrent: 2},
    "aws-ecs":       {Provider: "aws-ecs",      MaxConcurrent: 2},
    "azure-aca":     {Provider: "azure-aca",    MaxConcurrent: 2},
}
```

A worker that pops a target whose provider is at cap pushes the target
back to the queue (with a small delay) and tries the next. This keeps
healthy providers moving when one is slow.

The cap is `MaxConcurrent: 2` (half of MaxWorkers: 4). One slow
provider can claim at most half the pool.

## Prioritization Across Providers

Per-provider fairness pairs with global priority. Implementation:

```go
// Worker pop loop (simplified):
for {
    select {
    case work := <-queue:
        if !providerLimiter[work.Provider].Acquire() {
            // Push back; try again on next cycle
            requeue(work)
            continue
        }
        process(work)
        providerLimiter[work.Provider].Release()
    case <-ctx.Done():
        return
    }
}
```

## Batching: When And When Not

Batching is **not** a Phase 3.4 deliverable. Each Provider call is one
target, one operation. Multi-target batching is provider-specific (some
provider APIs support it, others don't) and adds complexity that the
single-target model doesn't need at this scale.

Phase 3.5+ may add per-provider batched operations where they exist
(e.g., AWS CloudFormation StackSet for multi-region) — but only with
explicit capability declaration.

## Deployment Batching (Different Concept — Phase 3.3 Staged Strategy)

The Staged strategy (Phase 3.3) sequences deploys across N targets.
This is **not** API batching — it is sequential dispatch per target,
using existing single-target Provider calls. The Staged strategy
controls the ordering; the worker pool processes each step in turn.

## Cycle-Level Operations: SweepAbandoned, RuntimeSweep

These remain single-goroutine operations, run from the reconciler's
main loop, NOT from workers:

```go
func (r *Reconciler) Tick(ctx) {
    r.RuntimeSweep(ctx)           // Single-threaded
    selected := r.SelectTargets(ctx)
    prioritized := r.PrioritizeTargets(selected)
    for _, t := range prioritized {
        r.queue.Push(ctx, t)      // Workers pop
    }
}
```

Sweep operations are global; running them in workers would risk
double-claim of abandoned ops. Phase 2 invariant preserved.

## Provider Rate-Limit Calibration

Per-provider rate limits (token-bucket) and concurrency caps:

| Provider | Tokens/sec | Burst | MaxConcurrent | Notes |
|---|---|---|---|---|
| Cloud Run | 5 | 10 | 2 | Cloud Run API: 1000 req/min global, but per-region varies |
| ECS | 5 | 10 | 2 | DescribeServices is throttled at 100 RPS account-wide |
| ACA | 5 | 10 | 2 | ARM read APIs limited; CreateOrUpdate cheaper |

These defaults are conservative. Operators can override via
environment variables; Phase 3.7+ may expose via capability matrix.

## Backpressure Cascading

When the queue fills (default depth 64), `Push` blocks. This is the
escape valve — if work piles up beyond the queue's depth, the next
tick's PrioritizeTargets returns less work (less time to process the
buffered set).

If queue fills consistently → operator alert via Phase 3.7
observability. Indicates Phase 4+ scale-out is needed, OR a slow
provider is the bottleneck.

## Cycle Duration Targets

| Targets | Workers | Expected cycle | Phase budget |
|---|---|---|---|
| 20 | 4 | < 5s | Phase 2 baseline |
| 50 | 4 | < 8s | Phase 3.4 baseline |
| 100 | 4 | < 15s | Phase 3.4 envelope |
| 200 | 4 | > 30s (exceeds tick) | Out of Phase 3 envelope; needs Phase 4 |
| 100 | 8 | < 10s | Phase 3.4 with operator-tuned MaxWorkers |

Phase 3.4 closure includes a load test at 100 targets to validate the
envelope.

## Worker Crash Handling

A worker that panics is recovered by the Phase 2 panic defer in the
dispatch path. The defer's effect:

1. `completeOperation(failed, "dispatcher panic")`.
2. `releaseTarget(creating/deleting, error, "dispatcher panic")`.
3. Worker goroutine returns; pool has N-1 workers until next tick
   re-spawns? No — Phase 3.4 ships a static pool. The next tick's work
   is processed by remaining workers. If all workers panic
   simultaneously (catastrophic), the pool stops; reconciler restart
   recovers.

Phase 3.5+ may add worker resurrection — but only if real evidence of
need.

## Phase 3.4 Scheduling Test Matrix

For each of these scenarios, a test asserts correct scheduling:

- One target stuck in CAS claim race — only one worker wins.
- One slow provider monopolizing — others still progress.
- Workflow observation deadline — target gets priority on next cycle.
- Circuit-open target — not selected this cycle.
- Per-target backoff active — not selected.

## Phase 3.4 Closure Criteria For This Doc

1. Scheduling pipeline (Tick → Select → Prioritize → Push → Workers)
   implemented.
2. Priority function as documented (Phase 3.4 simple form).
3. Per-provider concurrency limiter operational.
4. Per-provider rate limiter operational.
5. Cycle-duration load test asserting 100 targets in < 15s.
6. Worker-crash recovery validated.
7. Sweep operations remain single-threaded; documented invariant
   preserved.

## Related

- [[reconciliation-scaling-foundations]] — overall deliverables
- [[queue-migration-strategy]] — queue interface
- [[provider-capability-matrix]] — per-provider rate-limit declaration
  (Phase 3.7+ extension)
- [[future-ha-boundary-analysis]] — Phase 4 multi-pod
- [[multi-provider-risk-analysis]] — R-11 mitigation
- [[../phase2.9/reconciliation-architecture-freeze]] — F-1..F-9
