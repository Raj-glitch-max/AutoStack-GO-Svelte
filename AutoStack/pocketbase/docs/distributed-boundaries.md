# AutoStack Distributed Boundaries

This document explicitly defines what AutoStack guarantees and does not guarantee across distributed deployments. These are honest engineering boundaries, not marketing limitations.

---

## What Is Single-Node Authoritative

The following are **single-node authoritative** in the current AutoStack implementation. They require no distributed coordination:

| Subsystem | Guarantee | Scope |
|---|---|---|
| Append-only KV store | Duplicate key rejected; no delete path | Single SQLite WAL instance |
| Audit trail | Hash-chained; tamper-detectable | Single `PersistentAuditStore` instance |
| Rate limiter | Per-key request counting | Single process; not shared |
| Replay manifest | Deterministic: same inputs → same hash | Single-node computation |
| Certification | Hash-stable; tamper-detectable | Single-node evaluation |
| Execution task hash | Deterministic content identity | Single-node |
| Logical clock | Monotonically increasing | Single-node counter |

---

## What Is Eventually Consistent

The following **do not provide strong consistency guarantees** across replicas:

| Subsystem | Behavior | Implication |
|---|---|---|
| Federation nodes | State diverges if partitioned; no auto-merge | Contradiction surfaced; operator resolves |
| Rate limiting across replicas | Each replica has its own bucket | A client can exceed the rate limit by hitting different replicas |
| Worker lease expiry | Based on heartbeat timeout; clock skew affects timing | Quarantine may be delayed or early under clock skew |
| Audit trail across replicas | Each recorder chains independently | Cross-replica chain integrity is not verified |
| PocketBase KV across nodes | SQLite is not a replicated store | Replication requires external tooling (Litestream, etc.) |

---

## Unsupported Multi-Region Guarantees

AutoStack **does not provide** the following across multiple regions:

- **Cross-region linearizability**: No global ordering of writes across nodes.
- **Cross-region read-your-writes**: A write on replica A may not be visible on replica B immediately.
- **Global rate limiting**: Rate limits are per-process; a distributed rate limit requires a shared store (Redis, etc.).
- **Cross-region audit continuity**: Audit chain continuity is not maintained across independently operating nodes.
- **Multi-region replay certification**: Replay certification covers a single execution graph; cross-region replay is not supported.
- **Multi-region governance approval**: Approvals are recorded locally; cross-replica approval deduplication is not implemented.

---

## Unsupported Consensus Guarantees

AutoStack **does not implement**:

- Raft or Paxos consensus
- Two-phase commit across nodes
- Distributed transactions (ACID across multiple stores)
- Leader election (beyond PocketBase's single-node model)
- Byzantine fault tolerance
- Automatic conflict resolution across replicas

---

## Tenant Isolation Assumptions

Tenant isolation is enforced by:

1. **Tenant-scoped keys**: All KV keys are namespaced by `tenantID`
2. **Isolation hash**: `SHA-256("isolation:" + tenantID)` detects accidental cross-tenant access in-process
3. **RBAC**: All API operations require a validated `TenantContext`
4. **Rate limiting**: Rate limit buckets are keyed by `tenantID`

**Assumptions that must hold for isolation to be valid:**
- `tenantID` is correctly extracted from a validated JWT or API token at the API boundary
- No tenant ID is passed as a raw user-supplied string without prior `ValidateTenantID()` validation
- The underlying SQLite store is not shared between tenants (separate files per tenant, or namespace enforcement)

**What isolation does NOT guarantee:**
- Cryptographic separation of data at rest (data is not encrypted per-tenant in SQLite)
- Isolation from a malicious process with filesystem access (requires OS-level isolation)
- Cross-process isolation (rate limiter buckets are per-process)

---

## Replica Divergence Behavior

When two AutoStack nodes diverge (network partition, independent writes):

1. **Evidence is preserved locally on both nodes** (append-only stores never delete)
2. **Contradictions are raised** when divergence is detected at federation boundary
3. **Operators are notified** via the operational surface
4. **No automatic merge** occurs — merging diverged append-only evidence is a manual operator decision
5. **Replay is not re-run** automatically — replay is an operator-initiated action

**What AutoStack WILL NOT guarantee:**
- Automatic resolution of diverged node state
- Causal ordering across nodes
- A single consistent view of execution history across replicas during a partition
- Silent convergence after a partition heals

---

## Summary: What AutoStack WILL NOT Guarantee

```
AutoStack does NOT guarantee:
- Distributed linearizability
- Cross-replica audit continuity
- Global rate limiting
- Automatic conflict resolution
- Multi-region consensus
- Byzantine fault tolerance
- Cross-region replay certification
- Distributed transaction atomicity
- Encryption at rest (per-tenant)
- Process-level tenant isolation
```

These limitations are honest. They reflect the current implementation boundary. Future work may address specific items (e.g., Litestream for replication, Redis for distributed rate limiting), but those will be explicitly implemented and documented — not assumed.

---

*Last updated: Phase PR-2. Schema version: 7.0.0.*
