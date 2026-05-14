# Phase 2 Finalization — Operational Maintainability Assessment

**Last Updated:** 2026-05-14

## Architecture Trajectory

### What scales cleanly

| Concern | Current capacity | Scaling headroom |
|---|---|---|
| Single-pod cloud reconciliation | 1 pod, 30s tick | Limited by in-memory state + single-threaded dispatch |
| CAS claim dispatch | SQLite WAL serialization | Works; Postgres also supports conditional UPDATE |
| Deployment history | Append-only, no eviction | Scales; consider archival after N months |
| Heartbeat-per-op | In-memory, O(N ops) | Manageable until N > ~50 concurrent ops |
| Circuit breaker | In-memory, O(N targets) | Scales; circuit-open targets are skipped |

**Conclusion:** Current architecture handles "a handful of cloud targets" with headroom. Does not yet handle "hundreds of concurrent cloud targets + 5-min reconcile ticks" without profiling.

---

### What does NOT scale cleanly

**1. Global backoff (`lastErrorTime`)**

The entire reconciliation cycle backs off when ANY failure occurs. With 50 targets and one transient failure, all 49 healthy targets are delayed by `BackoffBase × 2^N`. This was documented in Phase 2.7 as a known issue.

**Fix:** Per-target backoff or circuit-level throttling. Phase 3 material.

**2. In-memory state resets on restart**

- `failures`, `suspicions`, `staleCount`, `heartbeatFails` all reset.
- After restart: all circuits closed, all suspicion counters zero.
- A target with auth failures immediately retries after restart.

**Impact:** If an auth-failure target has its credentials rotated during a process restart, it won't retry until the next cycle (which is fine). A target with transient failures immediately retries with no circuit protection.

**Fix:** Persist circuit/transition state to DB. Phase 3 material.

**3. No operation TTL or GC**

`operations` collection is append-only. Long-running deployments + restarts accumulate rows.

**Impact:** None for months of operation with typical deployment counts. Becomes relevant at scale.

**Fix:** TTL expiry or archival. Phase 3.

**4. Single reconciliation goroutine**

The reconciler polls sequentially. With 100 targets that all need polling, each cycle is N × poll_latency. No parallelism within a cycle.

**Impact:** 100 targets × 500ms poll latency = 50s cycle. With 30s tick interval, cycle never completes before next tick fires. Backlog accumulates.

**Fix:** Worker pool with per-target parallelism. Phase 3.

---

## Maintainability Today

### Code complexity assessment

The reconciler code is approximately 900 lines. Key concerns:

| Module | Complexity | Notes |
|---|---|---|
| `cloud.go` reconcileAll/reconcileOne | Moderate | Main entry; panic boundary at cycle level |
| `dispatch.go` dispatchDeploy/dispatchDestroy | Medium-High | Heartbeat + panic-recovery; many branches |
| `sweep.go` startup + runtime sweep | Low | Clear, linear logic |
| `owns.go` claimTarget/releaseTarget | Low-Medium | CAS idiom; clearly documented |

**The code is readable and defensible** — the dispatch paths have dense comments explaining the why of each branch. The goroutine safety model (panic recovery, heartbeat lifecycle) is transparent.

### Adding a new provider (AWS ECS, Azure ACA)

**What is required:**
1. Implement `Provider` interface in `pkg/providers/<name>/provider.go`.
2. Register in `providers.RegisterProvider` (called from `Reconciler.Start`).
3. Add to `providerToProviderName` switch in `cloud.go`.
4. Handle `CheckQuotas`, `Rollback`, `GetMetrics`, `GetOperation` as `ErrNotImplemented` until Phase 3+.
5. Implement `Deploy`, `Destroy`, `GetStatus`, `EstimateCost` correctly.

**What is NOT required:** Changes to the reconciler, dispatch, sweep, or history infrastructure. The provider interface isolates all cloud-specific logic.

**This is the intended additive architecture** — CLAUDE.md mandate preserved.

---

## Extensibility for Phase 3

### Adding a worker pool

The design in `phase2.8/deferred-followups.md` lists "Worker pool for dispatch" as Phase 3. The current single-threaded dispatch is a prerequisite: a worker pool requires:

- **Operation persistence** (so workers can find their assigned ops after restart) — migrations for `operations.cycle_id` column, `owned_by_pod`, etc. are Phase 2.9 material.
- **Dispatch idempotency** — `claimTarget` is already idempotent-safe.
- **Heartbeat scoping** — the current global heartbeat-per-dispatcher won't map to workers without pod-identity stamping.

**Architecture can accommodate:** The current in-process per-op heartbeat already uses the op ID as a scope. Moving to per-worker heartbeats requires pod-identity on operations, but the pattern is the same.

### Adding drift detection

Requires:
- Capturing `deployed_spec` at deploy success (Phase 3 schema change).
- Per-cycle `GetService` structural diff (Cloud Run provider extension).
- Surfacng `drift_detected = true` + `drift_summary` in UI.

**The UI hook exists** (`drift_detected` column already in schema). The data infrastructure needs Phase 3 work.

---

## Operational Entropy Controls

| Entropy type | Controlled by | Adequate? |
|---|---|---|
| In-memory state loss on restart | Acceptable — circuits reset, targets retransparent | ✅ Acceptable |
| Operation row accumulation | No TTL, accepted for Phase 2 | ⚠️ Phase 3 cleanup needed |
| Stale count reset on restart | Acceptable — wasted quota for 3 cycles | ✅ Acceptable |
| Suspicion count reset on restart | Acceptable — first error held after restart | ✅ Acceptable |
| Circuit reset on restart | ⚠️ Could cause retry storm | ✅ Auth errors retry-safe; transient limited to 1 extra cycle |
| History row accumulation | Append-only; archival TBD | ⚠️ Phase 3 archival needed |

---

## Verdict

**Current architecture is maintainable at Phase 2 scale.** Code is readable, provider isolation is clean, and the goroutine model is understandable.

**Phase 3 scaling gaps are identified and deferred** — global backoff, in-memory circuit state, sequential polling, and operation GC are all documented.

**Adding new providers is low friction** — the interface boundary holds.

**The main maintainability risk** is the global `lastErrorTime` backoff causing cascade delays when one target fails. Acceptable for Phase 2; Phase 3 per-target backoff closes it.

---

## Related
- [[phase2.8/deferred-followups]] — worker pool and Phase 3 items
- [[reconciliation-guarantees]] — single-threaded design documented
- [[current-state]] — Phase 2.3 architecture summary