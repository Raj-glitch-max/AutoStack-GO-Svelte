# AutoStack Platform SLOs

This document defines the Service Level Objectives for AutoStack's deterministic orchestration platform. These are honest targets based on observed in-process performance — not marketing promises.

---

## 1. Replay Verification Latency

**Definition:** Time from replay request submission to completion of `VerifyReplayManifest()` with a valid/invalid determination.

| Tier         | Target       | Notes                                                              |
|--------------|--------------|--------------------------------------------------------------------|
| p50          | < 5 ms       | Typical single-execution replay over in-memory store              |
| p95          | < 50 ms      | Under moderate task count (<1000 tasks per execution)              |
| p99          | < 200 ms     | Under heavy task count or SQLite store I/O                         |

**Limitations:**
- Replay latency scales with task count. Executions with >10,000 tasks may exceed p99.
- SQLite WAL mode is used; cold-page reads increase latency vs. warm-cache reads.
- Distributed replay (federated nodes) adds network round-trip time not modeled here.

---

## 2. Contradiction Scan Latency

**Definition:** Time for `DetectProviderContradiction()` to evaluate one provider observation against desired state.

| Tier         | Target       | Notes                                                              |
|--------------|--------------|--------------------------------------------------------------------|
| p50          | < 1 ms       | Single-observation in-process evaluation                           |
| p95          | < 10 ms      | Batch of 100 observations                                          |
| p99          | < 50 ms      | Batch of 1000 observations under concurrent access                 |

**Limitations:**
- Contradiction detection is observation-by-observation. Scanning all providers across all tenants is O(n × m).
- No cross-provider deduplication — the same contradiction may be detected multiple times before resolution.

---

## 3. Certification Generation Latency

**Definition:** Time for `RunPlatformReadinessAudit()` to produce a `PlatformCertificationReport` with all 5 gates evaluated.

| Tier         | Target       | Notes                                                              |
|--------------|--------------|--------------------------------------------------------------------|
| p50          | < 10 ms      | In-process, warm data                                              |
| p95          | < 100 ms     | Including hash computation and gate serialization                  |
| p99          | < 500 ms     | Under concurrent audit requests                                    |

**Limitations:**
- Certification is a read-only projection — it never modifies state. Latency is bounded by hash computation speed.
- External storage backends (SQLite, PocketBase) add I/O latency not modeled here.

---

## 4. Recovery Objectives

**RTO (Recovery Time Objective):** The maximum tolerable time to restore operational state from a validated backup.

| Scenario                         | RTO Target    | Notes                                                            |
|----------------------------------|---------------|------------------------------------------------------------------|
| In-process restart (no data loss) | < 30 seconds  | `RecoverPersistentRuntime()` from in-memory checkpoint           |
| SQLite WAL restore               | < 5 minutes   | Depends on database size and I/O throughput                      |
| Full backup restore              | < 30 minutes  | Requires payload retrieval + hash verification + state replay    |

**RPO (Recovery Point Objective):** The maximum tolerable data loss window.

| Scenario                         | RPO Target    | Notes                                                            |
|----------------------------------|---------------|------------------------------------------------------------------|
| Checkpoint-based recovery        | ≤ last checkpoint | Checkpoint frequency is operator-configured                  |
| Archive-based recovery           | ≤ last archive flush | Archive flush interval is operator-configured             |
| Full backup recovery             | ≤ last backup creation | Backup frequency is operator-configured                  |

---

## 5. SLO Monitoring

These SLOs are measured via `pkg/metrics`:

- `autostack_replay_latency_ms` — Histogram (p50, p95, p99)
- `autostack_certification_runs_total` and duration via Histogram
- `autostack_contradictions_detected_total` — rate over time
- `autostack_tasks_failed_total / autostack_tasks_total` — failure rate

**Alerting thresholds (recommended):**
- Replay latency p99 > 500 ms → page on-call
- Certification failure gate count > 0 → page on-call
- Auth failure rate > 10/min → security alert
- Quarantined workers > 0 → operator alert

---

## 6. What These SLOs Do Not Cover

- Network egress latency to cloud provider APIs
- PocketBase query latency under PostgreSQL migration
- Kubernetes operator reconciliation loop latency
- Frontend WebSocket streaming latency
- Third-party notification delivery (Novu)

These are measured separately and are not part of the core orchestration platform SLOs.
