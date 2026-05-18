# Operational Risks and Limitations

This document is honest about what AutoStack does not guarantee. Read it before deploying.

---

## Storage

| Risk | Severity | Mitigation |
|------|----------|------------|
| `InMemoryKVStore` loses all state on process restart | Critical | Use `SQLiteKVStore` or `PocketBaseKVStore` in production |
| SQLite WAL is single-writer | Medium | Single-node deployment; horizontal scaling requires external coordination |
| SQLite WAL is not replicated | High | Mount on a replicated volume (EBS, GCE PD, Azure Disk) or use an external database |
| Large archive PVCs may exceed storage quotas | Medium | Monitor `autostack-archive-pvc` utilization |

## Replay

| Risk | Severity | Mitigation |
|------|----------|------------|
| Divergence detection may produce false-positives | Low | Caused by different event ordering policies; investigate before acting |
| Partial replay may miss events before checkpoint | Low | By design; documented in `replayLimitations()` |
| Clock skew between nodes can break monotonicity | Medium | Use NTP; monitor `ClockMonotonic` field in audit results |

## Workers

| Risk | Severity | Mitigation |
|------|----------|------------|
| Abandoned task detection requires live heartbeats | High | Workers must emit heartbeats on the configured interval |
| Deterministic assignment breaks if worker list changes mid-execution | Medium | Worker roster changes during an execution can reassign tasks; use stable worker sets |
| Quarantined workers are excluded from assignment | Low | By design; operators must un-quarantine or replace workers |

## Governance

| Risk | Severity | Mitigation |
|------|----------|------------|
| Policy bypass attempts are logged but not blocked at network layer | Medium | Add API auth middleware before policy evaluation |
| Approval hierarchy with no approvers blocks execution indefinitely | Medium | Always configure at least one approver per hierarchy level |

## Provider Operations

| Risk | Severity | Mitigation |
|------|----------|------------|
| Contradiction detection does not cover all provider error modes | Medium | 4 types are detected; novel provider behavior may go undetected |
| Provider API rate limits can cause false timeouts | Low | Classify provider errors correctly in `NormalizedProviderResult` |
| `ResultStateAmbiguous` states are not auto-resolved | High | Operators must investigate and classify; no auto-correction |

## Platform Certification

| Risk | Severity | Mitigation |
|------|----------|------------|
| Certification is point-in-time | Medium | Runtime state changes after certification; recertify on significant changes |
| A certified platform with `health=degraded` may be partially impaired | High | Degraded health means the platform is functioning but below normal capacity |
| Replay certification does not predict future divergences | Medium | It certifies the recorded event set only |

---

## Unsupported Claims

AutoStack does not claim:

- Byzantine fault tolerance
- Distributed consensus
- Infinite horizontal scalability
- Self-healing or autonomous recovery
- AI-driven orchestration decisions
- Zero-downtime rolling upgrades of the runtime itself

Any marketing or documentation that contradicts this list is incorrect.
