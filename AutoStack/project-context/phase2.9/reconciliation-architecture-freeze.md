# Phase 2 Architecture Freeze Review

**Last Updated:** 2026-05-14

## Purpose

This document records which parts of the AutoStack reconciliation
architecture are STABLE (Phase-2 frozen), EXPERIMENTAL (known issues,
acceptable for Phase 2), DEFERRED (Phase 3), or UNSAFE (requires
redesign before multi-pod/production scale).

---

## STABLE — No Changes Required in Phase 3

### F-1: Single-Threaded Reconciler Loop

**Component:** `reconcileAll` → `reconcileOne` (cloud.go)

**Why stable:** The sequential per-cycle iteration makes determinism
trivial to reason about. `go Vet`/`go build` clean. No race conditions
within a cycle.

**Future constraint:** A Phase 3 worker pool MUST preserve the property
that two goroutines never operate on the same target simultaneously. The
worker pool design must use the same CAS claim mechanism. The single-thread
semantic is an implementation detail, not a requirement — but the
serialism of dispatch per-target IS a requirement.

### F-2: CAS-Based Dispatch Claim

**Component:** `claimTarget`, `releaseTargetWithExternal` (dispatch.go, owns.go)

**Why stable:** `UPDATE ... WHERE current_operation = '' AND status IN ('pending', 'deleting')`
with `rows_affected == 1` is the standard SQL CAS idiom. Works identically
on SQLite (WAL) and PostgreSQL.

**Future constraint:** The release CAS `WHERE current_operation = :opID` is
load-bearing. Any Phase 3 code that modifies `current_operation` MUST
respect this CAS guard.

### F-3: Operation Lifecycle (Create → In_Progress → Terminal)

**Component:** `createOperation`, `completeOperation` (dispatch.go)

**Why stable:** `createOperation` always inserts `status = 'in_progress'`.
`completeOperation` uses `WHERE status = 'in_progress'` as a CAS guard.
The ownership transfer from dispatcher to sweep is clean.

**Future constraint:** New operation types (e.g., rollback, scale) must
follow the same create → in_progress → terminal schema. Sweep logic depends
on `status = 'in_progress'` being the only non-terminal state.

### F-4: Heartbeat + Sweep Liveness

**Component:** `heartbeat`, `SweepAbandonedOperations`, `RuntimeSweep` (sweep.go)

**Why stable:** Heartbeat fires every 60s and the startup sweep uses
`2 × heartbeatInterval = 2 min` as the liveness cutoff. First-heartbeat
guard catches ops that never fired a heartbeat. These three properties
together make false sweep reclamation unlikely.

**Known:** The runtime sweep at 5 min stale threshold is safe for
single-pod. Multi-pod requires `owned_by_pod` stamping (Phase 3). This is
documented, not a regression risk.

### F-5: Status Transition Guard

**Component:** `isAllowedTransition` (cloud.go, line 774)

**Why stable:** Four rules only: empty→any, same→any, deleted→none,
running+updating→pending+creating blocked. Simple to audit, no edge cases.

**Future constraint:** Any new status values (Phase 3: `drifted`, `stale`)
must be added to `isAllowedTransition` explicitly. The guard will refuse
unexpected transitions by default.

### F-6: Suspicion Counter for Transient Flaps

**Component:** `noteSuspectError`, `clearSuspect` (cloud.go)

**Why stable:** Requires two consecutive error observations before
persisting `error` from `updating` state. Single flap resets on any
non-error observation. The 2-observation threshold is conservative
(avoids false error) and bounded (worst case: 1 extra cycle delay).

### F-7: Panic Defer in Dispatchers

**Component:** `dispatchDeploy`/`dispatchDestroy` defer blocks

**Why stable:** Both dispatchers call `completeOperation` + `releaseTarget`
in defer. No path exists where a dispatcher panic leaves the op in
`in_progress` without at least attempting to mark it terminal.

### F-8: Error Sanitization

**Component:** `sanitizeError` (cloud.go)

**Why stable:** Blocklists credential keywords and truncates long messages.
Does not rely on allowlisting, which would break on new error formats.

### F-9: Lineage Completeness

**Component:** `writeHistory` calls throughout dispatch + sweep

**Why stable:** Every dispatch branch writes history. The sweep writes
history for abandoned ops. `writeOwnershipLostHistory` covers the race case.

---

## EXPERIMENTAL — Known Issues, Accepted for Phase 2

### E-1: Global `lastErrorTime` Backoff

**Issue:** A single failing target causes all targets' cycles to back off.

**Why acceptable:** At Phase 2 scale (≤20 cloud targets), the impact is
negligible. The backoff is capped at 5 min. A one-cycle delay on 19 healthy
targets during one target's transient failure is not operationally
significant.

**Phase 3:** Per-target backoff state.

### E-2: In-Memory Circuit State Reset on Restart

**Issue:** After restart, all circuit breakers are closed.

**Why acceptable:** Auth errors never retry. Transient failure targets get
at most one extra cycle of retries. Auth failures require external action
regardless.

**Phase 3:** Persist circuit state to DB.

### E-3: Sequential Status Polling

**Issue:** A slow `GetStatus` on target 1 delays targets 2..N within the
same cycle.

**Why acceptable:** ≤20 targets × ~500ms latency = ~10s cycle duration.
With 30s tick interval, still healthy. At 100 targets, the design needs a
worker pool.

**Phase 3:** Worker pool (documented).

### E-4: Stale Count In-Memory Reset

**Issue:** Process restart resets `staleCount`. A respec-flapping rollout
could burn 3 more provider calls before the stale guard engages post-restart.

**Why acceptable:** 3 extra cycles × 30s = 90s wasted quota maximum. Not
a correctness failure.

---

## DEFERRED — Not Yet Implemented, Phase 3

| Item | Description | Blocker |
|---|---|---|
| Worker pool | Parallel dispatch across targets | Operations persistence + pod stamping |
| Drift detection | Spec-vs-actual diff on periodic cycle | deployed_spec column + diff library |
| Structured logging | slog replacing log.Printf | log/slog migration; no correctness impact |
| Metrics export | Prometheus/OpenTelemetry | No SLO monitoring in Phase 2 |
| Pod-identity stamping | `owned_by_pod` on operations | Multi-pod unsafe in Phase 2 |
| Real rollback | Cloud Run Traffic targeting | Revision lineage not persisted |
| Live cost estimates | GCP Billing API | ADR-010; Phase 3 only |
| CheckQuotas live | Cloud Quotas API | Phase 3; ErrNotImplemented stub honest |

---

## UNSAFE — Requires Redesign Before Use

### U-1: Multi-Pod Reconciliation

**Unsafe because:** Two pods running the reconciler simultaneously cannot
distinguish a peer's live operation from an abandoned one. The startup
sweep will not falsely reclaim a beathing peer op (2-min window), but the
runtime sweep (5-min threshold) can't distinguish peer-live from stale.

**Required for safe multi-pod:** `operations.owned_by_pod VARCHAR` column,
runtime sweep filtering `WHERE owned_by_pod = :my_pod_id OR heartbeat_stale`.
Phase 3 must implement this before multi-pod PocketBase is supported.

### U-2: HA Database Without Row Versioning

**Unsafe because:** Two pods racing `UPDATE deployment_targets` with no
row version check causes lost updates. SQLite's WAL serializes writes,
which masks the problem — but Postgres doesn't.

**Required for safe Postgres:** Either (a) optimistic concurrency (row
version column checked on UPDATE) or (b) leader election (raft or similar)
ensuring only one pod is the active reconciler.

### U-3: Concurrent Rollouts to Same Target

**Unsafe because:** The CAS claim is per-target, not per-rollout. Two
rollout-update events targeting the same `deployment_targets` row
simultaneously would race the `shouldDispatchDeploy` check and both see
`status = pending`. CAS resolves one winner; the loser gets lost intent
with no notification to the operator.

**Required for safe concurrent rollouts:** Queue-based dispatch or
optimistic locking with explicit conflict resolution.

---

## Architectural Invariants (Must Not Break in Phase 3)

These properties are load-bearing for correctness. Any Phase 3 change
MUST preserve them:

| Invariant | Why Critical |
|---|---|
| At most one in-flight op per target | Prevents double-dispatch |
| Op must be terminal before target can dispatch new op | CAS claim ensures this |
| Sweep can reclaim any in_progress op regardless of age | Crash recovery correctness |
| Deployment history written on all terminal paths | Forensic auditability |
| Release uses CAS on current_operation | Sweep can't be overwritten by dispatcher |
| Error status means operator action required | Truthful state contract with operators |
| NOT_FOUND = permanent failure | Circuit breaker correctness |
| `deleted` is terminal; no transition out | State model anchor |
| `unknown` not persisted | Honest ambiguity signal |

---

## Phase 3 Extension Points (Safe)

These are additive; they do NOT compromise existing invariants:

- New field in `DeploySpec` (passed through to provider)
- New provider implementation (AWS ECS, Azure ACA)
- New status values in `TargetStatus` with corresponding transitions in `isAllowedTransition`
- New operation types (`kind = "scale"`, `kind = "restart"`)
- Per-target persistent failure state in a `circuit_state` table
- Background goroutine for orphan scanning (reads only)
- Structured log fields (additive to existing tags)

---

## Related
- [[phase2.9/lifecycle-contracts]]
- [[phase2.9/provider-contracts]]
- [[phase2.9/deferred-Phase3-concerns]] — Phase 3 full backlog
- [[phase2.9/trustworthiness-verdict]]