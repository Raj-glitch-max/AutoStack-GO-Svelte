# Retry Amplification Review — Phase 2.4

## Last Updated
2026-05-14

## Premise

Real retries + replay + sweep recovery interact. When transient
failures occur, the system must not turn a brief provider hiccup into
a cascade — API hammering, reconciliation storms, deployment
starvation, cost amplification.

## Retry surfaces in the system

| Surface | Trigger | Bound | Visibility |
|---|---|---|---|
| Reconciler tick | every 30s (default `Interval`) | `BackoffMax` (5 min) at cycle level after errors | `[RECONCILE]` per cycle |
| Per-target circuit breaker | failure count ≥ 5 | indefinite skip until operator action | `circuit open` log |
| Per-cycle backoff | last error recency | exponential up to BackoffMax | implicit (cycle skipped) |
| Dispatch | only from `pending` or `deleting` | one attempt per cycle | `[DISPATCH_CLAIM]` |
| Stale-spec re-dispatch | rollout updated during deploy | every cycle until stable | `[DEPLOY_STALE]` per attempt |
| Status-poll retry | error from GetStatus | cycle-level backoff | `[FAILURE]` |
| Heartbeat | every 60s | indefinite (within ctx) | `[HEARTBEAT_FAIL]` |
| Sweep | startup only | one-shot | `[STARTUP_SWEEP]` |
| Cloud Run waitForServiceReady poll | every 5s | bounded by parent ctx (DeployTimeout=15min) | none |

## Amplification scenarios

### A-1: API rate-limit cascade

**Setup:** Cloud Run API returns 429 quota-exceeded on GetStatus for
50 targets simultaneously (e.g., cross-target quota tied to project).

**Behavior:**
- Each target's GetStatus fails with quota error.
- `ClassifyError` → FailureQuota.
- `recordTargetFailureWithCategory` short-circuits: quota errors do NOT
  increment circuit breaker, do NOT call `recordError`.
- Cycle-level backoff is NOT engaged for quota errors.
- Next cycle (30s later): 50 GetStatus calls again. Same 429.
- Quota stays consumed by retries.

**Severity:** Medium. Quota errors should engage backoff at cycle level
to give the provider a chance to refill.

**Phase 2.4 fix considered:** Treat quota errors as cycle-level backoff
triggers (call `recordError` for quota class).

**Trade-off:** Cycle-level backoff affects ALL targets, not just the
quota-impacted ones. If only one cloud_account is rate-limited, backing
off all targets is over-broad.

**Decision:** Defer. The right answer is per-cloud_account backoff
which requires schema work. Documented as Phase 2.5 hardening.

### A-2: Auth failure cascade across all targets sharing a credential

**Setup:** A cloud_account's service account JSON is revoked. All
targets using that account fail with 401.

**Behavior:**
- Each target's GetStatus / Deploy fails with auth error.
- `ClassifyError` → FailureAuth.
- Same as quota: no circuit increment, no cycle backoff.
- 30s later: same 50 401s.

**Severity:** Medium. Auth failures hammer the provider's auth
endpoint, which providers actively monitor for abuse.

**Phase 2.4 fix considered:** Per-cloud_account circuit breaker.

**Decision:** Defer to Phase 2.5. Documented.

### A-3: Sweep-induced replay storm at startup

**Setup:** Process restarts after a long downtime (e.g., 1 hour). N
in-progress operations exist. Sweep marks all abandoned, targets →
error. Operator (or automated system) then mass-resets via respec.

**Behavior:**
- Each respec'd target → pending.
- Next tick: 30s later, N dispatches in single cycle.
- Each dispatch is a fresh Deploy → provider load spike.

**Severity:** Low. Cloud Run can absorb a batch of 50 deploys.
Higher-N scenarios are not in scope today (no production has 1000+
targets).

**Mitigation today:** None.

**Phase 2.4 fix considered:** Stagger dispatches across cycles
(dispatch only N targets per cycle, prioritize by recency).

**Decision:** Defer to Phase 2.5 (worker-pool-with-bound work).
Documented.

### A-4: Stale-spec infinite re-dispatch

Covered in [[reconciliation-convergence-assessment]] C-1.

**Phase 2.6 fix:** Track succeeded_stale count; circuit breaker after
N consecutive.

### A-5: Heartbeat-fail-storm during DB busy

**Setup:** SQLite WAL contention or backup-in-progress causes
heartbeats to fail with `SQLITE_BUSY`.

**Behavior:**
- Heartbeat goroutine logs `[HEARTBEAT_FAIL]` and continues.
- If DB busy persists for >2 min: the heartbeat's `updated_at` falls
  outside the Phase 2.3 heartbeat liveness window.
- A startup sweep at that moment would mark the live op abandoned.
- But heartbeat hasn't crashed; it just hasn't written. Dispatcher is
  still running.

**Severity:** Low. SQLite busy storms are rare in single-pod operation.

**Mitigation today:** Phase 2.3 heartbeat-aware sweep uses 2-min window
which gives margin for a single missed heartbeat. A 3+ minute window
of busy would be required to falsely abandon.

**Phase 2.4 fix:** Add escalating-retry on heartbeat with backoff.
Defer to Phase 2.7 (observability) since the symptom is observability,
not correctness.

### A-6: Reconciler tick overlap

Discussed in [[../phase2.3/incident-reconstruction-assessment]] I-10.
Go's ticker queues at most one tick. Back-to-back cycles, no
concurrency. ✓ Not an amplification source.

## Bounded retry surfaces — verified

| Bound | Bound implementation |
|---|---|
| Per-target failure count | `r.config.FailureThreshold` (default 5) |
| Per-cycle backoff | exponential, capped at `BackoffMax` (5 min) |
| DeployTimeout | 15 min hard cap |
| Status-poll ctx | 30s per target |
| waitForServiceReady | parent ctx bounded |
| Heartbeat interval | 60s (no retry beyond next tick) |
| Sweep | startup-only, one-shot |

## Phase 2.4 implementation

None directly in this phase. The Phase 2.6 succeeded_stale guard
addresses A-4. Other amplification scenarios (A-1, A-2, A-3) are
deferred to Phase 2.5 per-cloud_account backoff design.

## Summary

| Amplification | Severity | Fix | Lands |
|---|---|---|---|
| A-1 quota cascade | Medium | per-account backoff | Phase 2.5 |
| A-2 auth cascade | Medium | per-account circuit | Phase 2.5 |
| A-3 startup mass-respec storm | Low | dispatch staggering | Phase 2.5 |
| A-4 succeeded_stale loop | Low | stale-count circuit | Phase 2.6 |
| A-5 heartbeat-fail storm | Low | escalating retry | Phase 2.7 |
| A-6 ticker overlap | n/a | Go ticker queueing | already safe |

## Related
- [[reconciliation-convergence-assessment]] C-1
- [[../phase2.3/dangerous-ambiguity-inventory]] M-6
