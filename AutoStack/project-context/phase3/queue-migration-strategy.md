# Queue Migration Strategy (Phase 3.4)

**Last Updated:** 2026-05-14
**Phase:** 3.4 (Scalable Reconciliation Foundations)

## Purpose

Define the **migration path** from Phase 3.4's in-process channel queue
to future queue implementations, WITHOUT building those implementations
prematurely. The Queue interface is the contract; this doc records what
the interface allows and what it forbids.

R-11 (premature distribution) is the failure mode this doc preempts.

## The Queue Interface (Phase 3.4)

```go
type Queue interface {
    Push(ctx context.Context, work TargetWork) error
    Pop(ctx context.Context) (TargetWork, error)
    Depth() int
    Close() error
}

type TargetWork struct {
    TargetID  string
    Provider  string
    OpKind    string  // "deploy", "destroy", "poll"
    Priority  int
    EnqueuedAt time.Time
    CycleID   string
}
```

**Constraints on this interface:**

- `Push` is non-blocking up to queue depth; blocks beyond.
- `Pop` blocks until work available or `ctx` cancelled.
- `Depth` is informational, may be approximate for distributed
  implementations.
- `Close` is idempotent.
- `TargetWork` is minimal — IDs, not full target rows. Workers re-read
  the target from DB at process time (guarantees fresh state per cycle).

## The In-Process Implementation (Phase 3.4 only)

```go
// pkg/reconciler/queue/channel_queue.go
type ChannelQueue struct {
    ch       chan TargetWork
    closed   bool
    closeMu  sync.Mutex
}

func NewChannelQueue(depth int) *ChannelQueue {
    return &ChannelQueue{ch: make(chan TargetWork, depth)}
}

func (q *ChannelQueue) Push(ctx context.Context, work TargetWork) error {
    select {
    case q.ch <- work:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (q *ChannelQueue) Pop(ctx context.Context) (TargetWork, error) {
    select {
    case w, ok := <-q.ch:
        if !ok {
            return TargetWork{}, ErrQueueClosed
        }
        return w, nil
    case <-ctx.Done():
        return TargetWork{}, ctx.Err()
    }
}

// Depth, Close: standard.
```

This is the **only implementation** Phase 3.4 ships. It satisfies the
interface, runs in-process, has zero infrastructure dependencies.

## What The Interface Permits (Future Implementations)

| Implementation | Properties | Phase | Notes |
|---|---|---|---|
| ChannelQueue (Phase 3.4) | In-process, FIFO, ephemeral | 3.4 | Current |
| SQLiteQueue | In-process, durable, FIFO | 3.5+ | Survives crash; same DB as PocketBase |
| PriorityChannelQueue | In-process, priority-ordered | 3.5+ | Refines priority enforcement |
| PostgresQueue | Cross-pod, durable, FIFO with row-level locking | 4+ | When Postgres path is enabled |
| RedisQueue | Cross-pod, durable, FIFO with Redis Streams | 4+ | If Redis is an acceptable dependency |
| KafkaQueue | Cross-pod, durable, partitioned | NEVER (out of scope) | Kafka is rejected per [[future-ha-boundary-analysis]] |

The interface accommodates the first four (3.4 through 4+). Kafka is
out-of-scope at the architecture level, not just at the implementation
level.

## Migration Triggers

When does AutoStack switch implementations? Phase 3.4 does NOT switch.
Future migrations are triggered by:

| Trigger | Move to | Reason |
|---|---|---|
| Process crash loses inflight work | SQLiteQueue | Durability becomes operational requirement |
| Priority-strict ordering needed | PriorityChannelQueue | Workflow deadlines demand strict ordering |
| Single-pod no longer sufficient | PostgresQueue or RedisQueue | Multi-pod work (Phase 4) |

Each migration is its own architectural review. Phase 3.4 does not
predetermine the choice.

## What The Interface Does NOT Cover

Deliberate omissions:

### Dead-letter queues

The interface does not expose a dead-letter pattern. The reconciler
handles failure via existing circuit-state and operator action. A
target with permanent failure is `error` with `current_operation=''`;
not re-queued.

Future Phase 4+ may add a `DeadLetter(work TargetWork)` method if
analytics or replay-from-dead-letter becomes valuable. Phase 3.4
declines.

### Visibility timeouts

The interface does not expose per-work visibility timeouts. The CAS
claim mechanism in the worker handles the equivalent: a claimed target
is owned for the dispatch's lifetime; heartbeat keeps liveness; sweep
reclaims abandoned ops.

If a distributed queue implementation needs visibility timeouts (e.g.,
RedisQueue with claim-then-ack semantics), it implements them
**internally** — the Queue interface still doesn't expose them.

### Priority-as-first-class

The interface accepts a `Priority` field on TargetWork but does NOT
guarantee priority ordering. The current ChannelQueue is FIFO; priority
is a hint. PriorityChannelQueue would change semantics; callers should
NOT assume strict priority ordering.

This is recorded in Go doc comments so future implementers know.

### Batching

The interface is single-work-per-Push, single-work-per-Pop. No batch
operations. Future implementations may add helpers, but the core
interface stays simple.

### Reordering / cancellation

Once pushed, work cannot be cancelled from the queue. If a target is
respec'd while in-queue, the worker will pop it, re-read target state,
and handle the new state (this is the FIFO + fresh-read pattern). No
queue-side cancel API.

## The Forward-Compatibility Tests

Phase 3.4 ships tests that exercise the Queue interface in ways that
future implementations must also pass:

```
TestQueue_PushPopFIFO:
  Push N works; Pop N works; assert order preserved.

TestQueue_ConcurrentPushPop:
  N concurrent producers, M concurrent consumers; assert all works
  are popped exactly once.

TestQueue_BlockingPop:
  Pop on empty queue blocks until Push.

TestQueue_BlockingPush:
  Push beyond depth blocks until Pop.

TestQueue_ContextCancellation:
  Both Push and Pop respect ctx.Done() within reasonable time.

TestQueue_CloseIdempotent:
  Multiple Close() calls don't panic.

TestQueue_DepthEstimation:
  Depth() is non-negative; consistent with Push/Pop counts.
```

These tests run against `ChannelQueue` in Phase 3.4. Any future
implementation must pass the same suite.

## Where The Queue Lives In The Code

```
pkg/reconciler/queue/
├── queue.go         — Interface + TargetWork type
├── channel_queue.go — Phase 3.4 implementation
├── queue_test.go    — Forward-compat tests
└── errors.go        — ErrQueueClosed, ErrQueueFull
```

The reconciler imports `pkg/reconciler/queue` and uses the interface.
The concrete `NewChannelQueue` is constructed in `cmd/` (the main
entrypoint) and injected.

## Phase 3.4 Closure Criteria

For this doc:

1. The Queue interface is sealed (above).
2. ChannelQueue passes the forward-compat test suite.
3. Reconciler uses the interface, not the concrete type.
4. Future-implementation table is recorded.
5. Omissions (DLQ, visibility timeouts, strict priority, batching,
   cancellation) are documented with rationale.

## The Final Discipline

**Do not build implementations until they are needed.** Phase 3.4 ships
one. Phase 3.5+ might ship a second. Phase 4+ might ship a third.

Premature implementations cost design freedom AND maintenance burden.
The interface is the durable contract; implementations are replaceable.

## Related

- [[reconciliation-scaling-foundations]] — overall scaling
- [[reconciliation-scaling-strategy]] — scheduling and worker pools
- [[future-ha-boundary-analysis]] — distributed queue context
- [[multi-provider-risk-analysis]] — R-11 mitigation
