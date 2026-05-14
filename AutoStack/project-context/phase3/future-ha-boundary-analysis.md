# Future HA Boundary Analysis

**Last Updated:** 2026-05-14
**Phase:** 3.0 (Foundations)

## Purpose

Define **what HA (high availability) work is permitted in Phase 3**,
what is hard-deferred to Phase 4+, and **why the deferrals are
deliberate** rather than oversight.

The temptation to "just add leader election" or "just shard by
provider" is significant. This document is the explicit, recorded
refusal — and the path forward.

## The Boundary

```
Phase 3 IN-SCOPE          | Phase 4+ OUT-OF-SCOPE
--------------------------+--------------------------------
Single-pod worker pool    | Multi-pod reconciliation
Single-process queue      | Distributed queue (Kafka, NATS)
In-process backpressure   | Cross-pod coordination
Capability negotiation    | Leader election
Per-target scheduling     | Consensus protocols
Worker fairness within pod | Cross-pod fairness
```

The single-pod envelope established in Phase 2 ([[../phase2.9/safe-operational-boundaries]])
is preserved through Phase 3. Phase 4 is when the multi-pod conversation
opens.

## Why The Hard Deferral

### Reason H-1 — Phase 2 ownership semantics are single-pod safe

CAS claim works in multi-pod with caveats (SQLite WAL serializes writes
on a single file system; on a network FS or Postgres, behavior depends
on isolation level). The sweep liveness window (2 min startup, 5 min
runtime) is **NOT** safe for multi-pod: peer pod's live op may be
reclaimed.

Phase 3 SC-5 (`owned_by_pod` stamping) is the prerequisite. Until that
ships in code, multi-pod **is unsafe**, not just "not implemented."

### Reason H-2 — Multi-pod compounds Phase 3 risks

Each of R-1 through R-12 [[multi-provider-risk-analysis]] is harder
to verify under multi-pod. Capability negotiation is per-pod; if pods
disagree on capability versions during a rolling deploy, behavior
becomes nondeterministic. Lifecycle normalization under split-brain
between two reconciler pods is far harder than under single-pod.

Phase 3 needs to land multi-provider correctness first, multi-pod
correctness later. Doing both simultaneously is the textbook scenario
for "feature delivered, system broken."

### Reason H-3 — The single-pod envelope is large enough

Phase 2's stated envelope is ≤ 20 cloud targets per reconciler
instance. Phase 3.4's worker pool extends this without changing pod
count. Even at 100 targets with a 4-worker pool and 30s tick, the
system is healthy single-pod. Multi-pod is required only at scale
beyond the Phase 3 ceiling, which is Phase 4+ territory.

### Reason H-4 — Distributed consensus has a steep correctness cliff

Leader election (Raft, Etcd-style, Postgres-advisory-locks) is a
specialized correctness problem. Getting it 95% right is far worse
than not having it: rare split-brain windows cause double-deploys.
AutoStack's contract is truthful state; a 1-in-1000 split-brain incident
breaks that contract.

Phase 4 may build leader election. Phase 3 won't.

## What Phase 3 May Build

### Allowed — In-Pod Worker Pool (Phase 3.4)

A configurable-size goroutine pool inside a single reconciler process.
Workers claim targets via the existing CAS mechanism. Ownership
semantics (G-1, G-2) preserved bit-identically.

**Why allowed:** No new ownership primitives. The CAS claim is already
goroutine-safe. The worker pool is just a parallelism multiplier
around an unchanged claim mechanism.

### Allowed — In-Process Queue Abstraction (Phase 3.4)

A queue interface that, in Phase 3, is backed by an in-process Go
channel or priority queue. The interface is designed so a future
implementation could be backed by a single-process Redis or SQLite
queue — **but not** a distributed broker.

**Why allowed:** Interface-only commitment. The implementation stays
in-process; the abstraction lets Phase 4 swap implementations without
reconciler core changes.

### Allowed — `owned_by_pod` Column (Phase 3.1 or 3.4)

A column on `operations` recording which pod's hostname owns the row.
**Phase 3 enforces it always equals the local hostname** at write
time. The column has no enforcement role in Phase 3 (the reconciler
ignores it during sweeps).

**Why allowed:** Phase 4 multi-pod safety needs the column to exist
historically. Adding it in Phase 3 is forward-compatible. Phase 3
sweeps continue to use heartbeat-based liveness; Phase 4 sweeps add
`AND owned_by_pod = :local` filtering.

### Allowed — Backpressure Primitives (Phase 3.4)

In-process rate limiting and backpressure. Token buckets per provider
to respect cloud API rate limits. Worker pool sizing per provider.

**Why allowed:** No cross-pod coordination. The primitives are local
to one reconciler process.

### Allowed — Persistent Circuit State (Phase 3.4 or 3.5)

A `circuit_state` table that survives process restart. Per-target
failure count and last-failure-at.

**Why allowed:** Local read/write to the existing PocketBase database.
No cross-pod coordination. Closes E-2 from
[[../phase2.9/reconciliation-architecture-freeze]].

## What Phase 3 Refuses To Build

### Refused — Leader Election

No protocol that elects a "primary" pod among multiple reconciler pods.
This includes Raft, Etcd-style leases, Postgres advisory locks, and
custom heartbeat-based election.

**If Phase 3 needs leader election**, the design has gone wrong.
Re-derive single-pod sufficiency.

### Refused — Distributed Consensus

No Paxos, no Raft, no quorum-based decision-making across pods.

### Refused — External Broker Dependencies

No Kafka, no NATS, no RabbitMQ, no Redis-as-broker. AutoStack's
dependency surface in Phase 3 remains: PocketBase, the cloud provider
APIs, and the Kubernetes API. Brokers are Phase 4+ at earliest, and
only after a real scale-driven motivation exists.

### Refused — Cross-Pod Ownership Negotiation

No "pod A asks pod B if it owns target X." The CAS claim is sufficient
within a single pod; multi-pod ownership negotiation is consensus by
another name.

### Refused — Multi-Pod Cache Coherence

No shared cache across pods. Each pod's in-memory state
(`failures`, `staleCount`, `suspicionCounter`) is process-private and
re-derivable from DB state.

## The Phase 3 → Phase 4 Bridge

Phase 3 leaves Phase 4 the following affordances:

| Phase 3 affordance | Phase 4 use |
|---|---|
| `operations.owned_by_pod` column | Sweep filtering for safe multi-pod |
| Worker pool interface | Multi-pod worker pool with cross-pod coordination |
| Queue abstraction | Distributed-broker-backed implementation |
| Capability matrix per-process | Negotiation across pods of differing versions |
| Persistent circuit state | Shared circuit knowledge across pods |
| Single-pod-only RuntimeSweep | Phase 4 adds multi-pod-safe RuntimeSweep |

These are forward-compatible structures, not premature implementations.

## The Operational Boundary (For Operators)

The Phase 3 operational envelope is communicated as:

```
SAFE:    1 PocketBase pod + N Kubernetes pods (any size)
SAFE:    1 reconciler process per PocketBase pod
SAFE:    Up to ~100 cloud targets per reconciler (Phase 3.4 worker pool)
UNSAFE:  2+ PocketBase pods running the reconciler simultaneously
UNSAFE:  Manual multi-pod deployment via Kubernetes Deployment.replicas > 1
         on the PocketBase StatefulSet WITHOUT the Phase 4 work
```

This is documented in [[../phase2.9/safe-operational-boundaries]] and
must be re-validated at Phase 3 closure.

## The Compile-Time Defense (Phase 3.4)

A `go vet` analyzer (or similar) will scan reconciler code for:

```
- Calls to net/* packages (no networking from reconciler)
- Imports of broker SDKs (sarama, nats.go, etc.)
- Goroutine spawns that escape the current process (impossible in Go,
  but new dependencies on cluster libraries flag a review gate)
```

The point is to keep the reconciler's footprint single-process.

## Phase 3.0 Closure Criteria

For this doc:

1. The boundary table (in-scope vs. out-of-scope) is sealed.
2. Reasons H-1..H-4 are recorded.
3. The "Allowed" list bounds Phase 3.4 design.
4. The "Refused" list is the explicit non-goal.
5. The Phase 3 → Phase 4 bridge table records the affordances.

## Related

- [[phase3/phase3-architecture-evolution]] — HC constraints
- [[phase3/multi-provider-risk-analysis]] — R-11 mitigation
- [[../phase2.9/reconciliation-architecture-freeze]] — U-1, U-2, U-3 (UNSAFE items)
- [[../phase2.9/safe-operational-boundaries]] — single-pod envelope
- [[../phase2.9/deferred-Phase3-concerns]] — SC-5 pod-stamping context
