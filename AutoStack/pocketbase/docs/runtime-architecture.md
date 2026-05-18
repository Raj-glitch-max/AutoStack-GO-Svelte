# AutoStack Runtime Architecture

**Schema Version**: 7.0.0 | **Status**: Production-ready

---

## Overview

AutoStack is a deterministic orchestration platform. Its runtime is structured around three invariants:

1. **Append-only evidence** — no execution evidence is ever deleted or mutated
2. **Replay determinism** — the same inputs always produce the same manifest hash
3. **Operator authority** — no automated path bypasses governance; operators remain in control

---

## Runtime Layers

```
┌─────────────────────────────────────────────────────┐
│                  Operator Surface                   │
│  (Platform UI: Timeline, Forensics, Governance,     │
│   Dashboard, Drill Mode)                            │
└─────────────────┬───────────────────────────────────┘
                  │ read-only query
┌─────────────────▼───────────────────────────────────┐
│              Platform API (pkg/platform/api)        │
│  GET /overview · /timeline · /forensics · /drills   │
│  GET /governance · /replay · /topology · /scheduler │
└─────────────────┬───────────────────────────────────┘
                  │ snapshots
┌─────────────────▼───────────────────────────────────┐
│               Execution Runtime                     │
│  pkg/executor  — task lifecycle                     │
│  pkg/scheduler — priority queue, fairness           │
│  pkg/workers   — worker pool, quarantine            │
│  pkg/verifier  — verification FSM                   │
│  pkg/workflow  — deterministic workflow graph       │
└─────────┬──────────────┬──────────────┬─────────────┘
          │              │              │
┌─────────▼──────┐ ┌─────▼──────┐ ┌────▼───────────────┐
│  Governance    │ │  Replay    │ │  Evidence Store    │
│  pkg/governance│ │  pkg/replay│ │  pkg/audit         │
│  pkg/policy    │ │  pkg/events│ │  pkg/security      │
│                │ │            │ │  (SQLite WAL)      │
└────────────────┘ └────────────┘ └────────────────────┘
```

---

## Key Packages

| Package | Role |
|---|---|
| `pkg/executor` | Task creation, state machine, logical clock |
| `pkg/scheduler` | Priority queue, fairness, replay events |
| `pkg/workers` | Worker pool, assignment, quarantine |
| `pkg/verifier` | Verification FSM (pending → verified / contradicted) |
| `pkg/governance` | Policy decisions, approval hierarchies, overrides |
| `pkg/policy` | Human approval gate, autonomous action rules |
| `pkg/replay` | Replay manifest, hash verification |
| `pkg/events` | WAL-backed event substrate |
| `pkg/audit` | Hash-chained audit trail |
| `pkg/security` | Auth, RBAC, revocation, durable audit store |
| `pkg/persistence` | SQLite KV store (WAL mode) |
| `pkg/planner` | Deterministic planning engine |
| `pkg/topology` | Execution graph, dependency analysis |
| `pkg/observability` | Health, contradiction metrics |
| `pkg/opcert` | Operational readiness certification (9 gates) |
| `pkg/prodcert` | Production + product certification (10+11 gates) |

---

## Execution Lifecycle

```
Plan → Schedule → Assign → Execute → Verify → Archive
         │                              │
         │ (blocked by governance)      │ (contradiction detected)
         ▼                              ▼
    RequiresApproval             VerificationContradicted
         │                              │
    Operator approves             Operator investigates
         │                         (never auto-resolved)
         ▼
    Execution continues
```

A task moves through: `Pending → Running → Completed | Failed | RolledBack`

---

## Restart Behavior

On process restart:
1. `DurableAuditRecorder.Restore()` loads persisted audit entries
2. `PersistentRevocationStore` reopens — all revocations survive
3. Replay manifest hashes are re-verified from store
4. Governance snapshots reload; no decisions are re-played automatically
5. Scheduler queue rebuilds from persisted task state

No automatic recovery, no auto-healing, no silent state mutation on restart.

---

## Single-Node Constraints

AutoStack is authoritative on a single node. The following are **not** provided:

- Distributed consensus (no Raft, no Paxos)
- Cross-replica audit chain continuity
- Distributed rate limiting
- Multi-node revocation propagation

These constraints are documented honestly in `docs/distributed-boundaries.md`.
