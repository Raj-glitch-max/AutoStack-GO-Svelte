# Reconciliation Scaling Foundations (Phase 3.4)

**Last Updated:** 2026-05-14
**Phase:** 3.4 (Scalable Reconciliation Foundations)

## Purpose

Prepare AutoStack's reconciliation architecture for higher target
counts and provider concurrency WITHOUT introducing distributed systems
complexity. Phase 3 ships **single-process worker-pool foundations**;
multi-pod is hard-deferred per [[future-ha-boundary-analysis]].

R-11 (premature distribution) is the primary failure mode this phase
must avoid.

## Phase 3.4 Deliverables (Single-Process Only)

| Deliverable | Purpose | Risk if rushed |
|---|---|---|
| Worker pool | Parallel dispatch within a single reconciler process | Ownership corruption |
| Per-provider rate limiting | Honor cloud API limits | Throttling cascades |
| Per-target backoff | Replace global lastErrorTime | One target affects all (E-1) |
| Persistent circuit state | Survive process restart | Loss of circuit knowledge (E-2) |
| Queue abstraction | Future-extensible interface | Premature distributed primitives |
| Backpressure primitives | Bound in-flight work | Resource exhaustion |
| `operations.cycle_id` column | Per-operation correlation | Cross-pod identity (H-1 prerequisite) |
| `operations.owned_by_pod` column | Forward-compat for Phase 4 | Multi-pod safety prerequisite |

## The Worker-Pool Design

### Overview

```
                  +---------------------+
                  |   Reconciler Tick   |
                  | (every 30s; sweep   |
                  |  before first tick) |
                  +----------+----------+
                             |
                  selectTargets()
                             |
                             v
              +--------------+--------------+
              |   Target Work Queue (Go     |
              |   channel; FIFO; bounded)   |
              +--------------+--------------+
                             |
       +---------+-----------+-----------+---------+
       |         |           |           |         |
   +---v---+ +---v---+   +---v---+   +---v---+ +---v---+
   |Worker | |Worker |   |Worker |   |Worker | |Worker | (N workers, configurable; default 4)
   +-------+ +-------+   +-------+   +-------+ +-------+
       |         |           |           |         |
       +---------+-----------+-----------+---------+
                             |
                  CAS claim per target (unchanged from Phase 2)
                             |
                             v
                    Provider.X(...) per target
```

### The Critical Invariant

**The CAS claim is the single source of ownership.** Workers race the
CAS on a target; one wins; one proceeds. This is bit-identical to
Phase 2's reconcile-loop semantics, just parallelized.

```go
// pkg/reconciler/worker_pool.go (Phase 3.4)
func (w *Worker) Run(ctx, queue chan TargetWork) {
    for work := range queue {
        // Worker picks up work
        target := work.Target
        // Existing claim mechanism — no change
        claimed, err := w.reconciler.claimTarget(target.ID, work.OpKind)
        if err != nil || !claimed {
            continue  // another worker won, or not eligible
        }
        // Existing dispatch — no change
        w.reconciler.dispatchTarget(ctx, work)
    }
}
```

The reconciler's existing CAS-based dispatch is reused. The worker pool
is a parallelism multiplier; ownership semantics are unchanged.

### Worker-Pool Sizing

| Setting | Default | Notes |
|---|---|---|
| `MaxWorkers` | 4 | Configurable via env var `AUTOSTACK_MAX_WORKERS` |
| `QueueDepth` | 64 | Bounded buffer between tick and workers |
| `Per-provider concurrency cap` | 2 | Honors typical cloud API rate limits |

Sizing rationale:

- 4 workers is enough for typical Phase 3 envelope (~100 targets).
- Larger pools risk cloud API throttling (next section).
- Per-provider cap prevents one provider's load from monopolizing the pool.

### Worker-Pool Lifecycle

- Workers start at process boot (after sweep, before first tick — Phase 2's
  Reconciler.Start() invariant preserved).
- Workers stop on process shutdown via `ctx.Done()`.
- A worker that panics is recovered (per Phase 2 panic defer); the pool
  continues with remaining workers; a replacement is NOT spawned in
  Phase 3.4 (the next tick brings work to the remaining workers).

## Per-Provider Rate Limiting

Each provider has natural API rate limits. Phase 3.4 adds:

```go
// pkg/reconciler/ratelimit.go
type ProviderRateLimit struct {
    Provider string
    PerSecond int   // tokens added per second
    Burst     int   // max tokens accumulated
}

var providerLimits = map[string]ProviderRateLimit{
    "gcp-cloudrun":  {Provider: "gcp-cloudrun", PerSecond: 5,  Burst: 10},
    "aws-ecs":       {Provider: "aws-ecs",      PerSecond: 5,  Burst: 10},
    "azure-aca":     {Provider: "azure-aca",    PerSecond: 5,  Burst: 10},
}
```

Workers acquire a token before making a provider call; block if the
bucket is empty. This is process-local backpressure that **never breaks**
correctness — it just slows.

The capability matrix may extend with `CapRateLimit` in Phase 3.5+ to
let providers declare their own limits.

## Per-Target Backoff (Replaces E-1)

Phase 2's `Reconciler.lastErrorTime` is a single global. Phase 3.4
replaces with per-target:

```go
// pkg/reconciler/backoff.go
type TargetBackoff struct {
    LastErrorAt   time.Time
    Consecutive   int
    NextRetryAt   time.Time
}

func (r *Reconciler) shouldBackoff(targetID string) bool {
    b := r.backoffs[targetID]
    return time.Now().Before(b.NextRetryAt)
}

func (r *Reconciler) recordTargetBackoff(targetID string, category FailureCategory) {
    b := r.backoffs[targetID]
    b.Consecutive++
    b.LastErrorAt = time.Now()
    b.NextRetryAt = time.Now().Add(exponentialBackoff(b.Consecutive))
    r.backoffs[targetID] = b
}
```

One failing target no longer delays 19 healthy targets. AW-S1 closed.

## Persistent Circuit State (Replaces E-2)

A new table:

```sql
CREATE TABLE circuit_state (
    target_id TEXT PRIMARY KEY,
    failures INT NOT NULL DEFAULT 0,
    last_failure_at DATETIME,
    last_category TEXT,
    open BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

Loaded into memory at startup; written on every transition. After
restart, persistent transient-failure targets do NOT immediately retry
(AW-S2 closed).

## Queue Abstraction

A minimal interface so future implementations can differ:

```go
// pkg/reconciler/queue/queue.go
type Queue interface {
    Push(ctx context.Context, work TargetWork) error
    Pop(ctx context.Context) (TargetWork, error)
    Depth() int
    Close() error
}

// Phase 3.4 ships ONE implementation: in-process bounded channel.
// Phase 4+ may add: SQLite-backed durable, Redis-backed shared, etc.
type ChannelQueue struct {
    ch chan TargetWork
}
```

The interface lets Phase 4 swap implementations without changing
worker code. Phase 3.4 does NOT ship more than the channel
implementation.

## Backpressure Primitives

```go
// pkg/reconciler/backpressure.go
type Limiter struct {
    inflight  *atomic.Int64
    maxInflight int64
}

func (l *Limiter) Acquire(ctx context.Context) error {
    for {
        if l.inflight.Load() < l.maxInflight {
            l.inflight.Add(1)
            return nil
        }
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(100 * time.Millisecond):
        }
    }
}

func (l *Limiter) Release() {
    l.inflight.Add(-1)
}
```

Applied per-provider (e.g., max 4 concurrent ECS API calls). Workers
acquire before provider call, release after.

## Schema Changes

### Migration: `operations.cycle_id`

```sql
ALTER TABLE operations ADD COLUMN cycle_id TEXT;
```

Phase 2 already includes cycle_id in logs (G-13). Phase 3 persists it on
the operation row for cross-cycle forensics and as a prerequisite for
SC-5 multi-pod work.

### Migration: `operations.owned_by_pod`

```sql
ALTER TABLE operations ADD COLUMN owned_by_pod TEXT;
```

Set to `os.Hostname()` at claim time. Phase 3.4 reads it back but does
NOT use it for sweep filtering. Phase 4 multi-pod work activates it.

### Migration: `circuit_state` table

Already shown above.

## The Single-Process Limits

Phase 3.4's worker pool extends the single-pod envelope:

| Setting | Phase 2 limit | Phase 3.4 limit | Phase 4+ limit |
|---|---|---|---|
| Targets per pod | ≤ 20 | ≤ 100 | unbounded with HA |
| Cycle duration | ~10s | ~10s (parallelized) | n/a |
| Tick interval | 30s | 30s (unchanged) | configurable |
| Concurrent ops per pod | 1 | 4 (MaxWorkers) | per-pod × pod count |

These are validated via load test in Phase 3.4 closure.

## Phase 3.4 Refuses

- **Refuse:** Distributed locks, leases, leader election.
- **Refuse:** External brokers (Kafka, NATS).
- **Refuse:** Cross-pod cache coherence.
- **Refuse:** Auto-scaling of the reconciler itself (Phase 4+).
- **Refuse:** Multi-pod-safe runtime sweep (Phase 4+ work).

These are documented in [[future-ha-boundary-analysis]].

## Phase 3.4 Closure Criteria

1. Worker pool implemented, default 4 workers, configurable.
2. Per-provider rate limit via token bucket.
3. Per-target backoff replaces global lastErrorTime.
4. `circuit_state` table + persistence layer.
5. Queue abstraction with in-process channel implementation.
6. Backpressure limiter per provider.
7. `operations.cycle_id` migration.
8. `operations.owned_by_pod` migration (set, not enforced).
9. Load test: 100 targets across 3 providers, 4 workers — assert
   cycle duration < tick interval.
10. Build + vet + tests clean.
11. All Phase 2 G-1..G-19 guarantees preserved (regression test).

## Related

- [[reconciliation-scaling-strategy]] — detailed scheduling and fairness
- [[queue-migration-strategy]] — path to future queue implementations
- [[future-ha-boundary-analysis]] — what Phase 4 unlocks
- [[provider-isolation-boundaries]] — per-provider primitives stay local
- [[../phase2.9/reconciliation-architecture-freeze]] — F-1..F-9 invariants
- [[multi-provider-risk-analysis]] — R-11 mitigation
