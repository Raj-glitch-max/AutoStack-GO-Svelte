# AutoStack Architecture

**AutoStack is a replay-certified orchestration operating platform.**

It is not a DevOps SaaS. It is not an AI agent. It is not a self-healing system.

It is a deterministic substrate that records every state transition as append-only, hash-verified evidence — and certifies that any execution can be replayed from that evidence to produce the same result.

---

## Platform Rings

The architecture is organized as concentric rings. Each ring depends only on rings interior to it.

```
┌─────────────────────────────────────────────────────┐
│  Certification Ring  (pkg/certification)            │
│  ┌───────────────────────────────────────────────┐  │
│  │  Operational Ring  (pkg/platformapi,          │  │
│  │                     pkg/operational)          │  │
│  │  ┌─────────────────────────────────────────┐  │  │
│  │  │  Governance Ring  (pkg/governance,      │  │
│  │  │                   pkg/policy, pkg/audit)│  │
│  │  │  ┌───────────────────────────────────┐  │  │  │
│  │  │  │  Execution Ring                  │  │  │  │
│  │  │  │  (pkg/executor, pkg/workflow,    │  │  │  │
│  │  │  │   pkg/workers, pkg/providerops)  │  │  │  │
│  │  │  │  ┌─────────────────────────────┐ │  │  │  │
│  │  │  │  │  Evidence Ring              │ │  │  │  │
│  │  │  │  │  (pkg/events, pkg/archive,  │ │  │  │  │
│  │  │  │  │   pkg/replay)               │ │  │  │  │
│  │  │  │  │  ┌───────────────────────┐  │ │  │  │  │
│  │  │  │  │  │  Persistence Ring     │  │ │  │  │  │
│  │  │  │  │  │  (pkg/persistence,    │  │ │  │  │  │
│  │  │  │  │  │   pkg/storage)        │  │ │  │  │  │
│  │  │  │  │  └───────────────────────┘  │ │  │  │  │
│  │  │  │  └─────────────────────────────┘ │  │  │  │
│  │  │  └───────────────────────────────────┘  │  │  │
│  │  └─────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────┘
```

---

## Ring Responsibilities

### Persistence Ring
- `pkg/persistence` — `DurableKVStore` interface; `InMemoryKVStore` (dev), `SQLiteKVStore` (WAL-backed production), `PocketBaseKVStore` (existing deployment)
- `pkg/storage` — file-backed WAL, integrity verification, corruption detection, startup recovery
- **Guarantee**: append-only; duplicate Put returns error; deep-copy on Get; hash verified at write time

### Evidence Ring
- `pkg/events` — `PlatformEvent` with content-identity hash; `EventStream` append-only
- `pkg/archive` — `ArchiveEntry` with payload hash; `ForensicExportManifest`; retention policy
- `pkg/replay` — read-only replay from ordered event sequence; partial replay from checkpoints; divergence detection
- **Guarantee**: nothing in this ring mutates any event, stream, or execution state

### Execution Ring
- `pkg/executor` — `ExecutionTask` FSM (12 states, explicit allowed-transitions map); dispatch; retry policy; checkpoint
- `pkg/workflow` — `WorkflowGraph` + `WorkflowExecution`; deterministic node traversal; rollback edges
- `pkg/workers` — `Worker` FSM; capability matching; deterministic assignment (SHA-256 of taskID + sorted worker list)
- `pkg/providerops` — `ProviderOperation` FSM; contradiction detection (4 types); rollback records
- **Guarantee**: same inputs always produce same state transitions; contradictions are detected and reported, never auto-resolved

### Governance Ring
- `pkg/governance` — RBAC; policy hierarchy; approval workflows; tamper-evident audit chain
- `pkg/policy` — `EvaluatePolicy` default-deny; confidence thresholds; escalation
- `pkg/audit` — `RunProductionTruthAudit` (9 dimensions); `CertifyReplay` (4 checks); operator safety assessment
- **Guarantee**: no action bypasses policy evaluation; denials and approvals are recorded in append-only audit chain

### Operational Ring
- `pkg/platformapi` — read-only query projections for all subsystems (7 modules)
- `pkg/operational` — `BuildOperationalSurface()` assembles derived live view; never persisted as source-of-truth
- `pkg/observability` — metrics, health assessment, timeline, contradiction summary; all derived-only
- `pkg/streaming` — `ExecutionStream` append-only event sequence; subscription manager

### Certification Ring
- `pkg/certification` — `PlatformCertificationReport` (5 gates); `CertifiedPlatformSnapshot`
- `PlatformCertified = true` only when all 5 gates pass: production-truth-audit, operator-safety-hash, replay-certification, platform-health, audit-hash-integrity
- **Guarantee**: certification is evidence only; it triggers nothing

---

## Determinism Invariants

The following properties hold across all packages:

1. **Same inputs → same hash** — all content-identity hashes exclude timestamps (`*At` fields). Hash functions are deterministic over content fields only.
2. **Append-only evidence** — `Put` on an existing key always returns an error. No delete path exists in any package.
3. **Explicit FSM transitions** — every state machine has a declared `allowedTransitions` map. Transitions not in the map are rejected.
4. **Evidence over automation** — contradictions, abandoned tasks, and safety violations are detected and reported. Operators decide. No silent auto-healing.
5. **Value semantics** — all state transition functions return new values. Original values are never mutated.
6. **Default-deny policy** — `EvaluatePolicy` denies unless explicitly allowed.

---

## What AutoStack Is Not

- **Not a self-healing system** — it detects failures and provides evidence; it does not auto-correct
- **Not an AI orchestration platform** — no LLM integration in the core runtime
- **Not a distributed consensus system** — no Raft, no Paxos, no Byzantine fault tolerance claims
- **Not infinitely scalable** — single-node SQLite WAL is the production storage; horizontal scaling requires external coordination not yet implemented
- **Not a replacement for Kubernetes** — the Kubernetes operator is the existing deployment mechanism; AutoStack is additive
