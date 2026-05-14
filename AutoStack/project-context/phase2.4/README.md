# Phase 2.4 — Operational Convergence, Lifecycle Closure & Failure-Tolerant Reconciliation

## Last Updated
2026-05-14

## Premise

Phase 2.3 closed the control-plane integrity audit and landed the
narrow safety fixes (pending_destroy re-arm, heartbeat-aware sweep,
intent-boundary history, canonical provider in lineage, cycle_id in
dispatch logs).

Phase 2.4 asks the next question:

> Does the control plane reliably **converge** toward truthful desired
> state under operational chaos — delayed providers, retries, replays,
> restarts, partial failures, stale operations, long-running deploys,
> interrupted deletes, transient instability?

This is no longer "does it execute?" or "is state truthful at one
moment?" It is "does it stay correct over time, under load and
failure, without operational decay?"

## The 13 deliverables

1. [Reconciliation convergence](reconciliation-convergence-assessment.md) — does every lifecycle reach a stable terminal/steady state?
2. [Retry amplification](retry-amplification-review.md) — can transient failures cascade?
3. [Lifecycle closure](lifecycle-closure-integrity-review.md) — terminal states are truthful and durable?
4. [Drift persistence](drift-persistence-assessment.md) — can divergence accumulate?
5. [Operation retention/TTL](operation-retention-ttl-proposal.md) — design for cleanup
6. [Rollback survivability](rollback-survivability-assessment.md) — current refusal + future fitness
7. [Ownership integrity (chaos-aware)](ownership-integrity-review.md) — extends Phase 2.3 with chaos lens
8. [Operational entropy](operational-entropy-assessment.md) — long-running decay
9. [Incident reconstruction maturity](incident-reconstruction-maturity-review.md) — can operators debug today?
10. [Phase 3 readiness](phase3-readiness-assessment.md) — architecture-trajectory check
11. [Dangerous ambiguity inventory](dangerous-ambiguity-inventory.md) — prioritized Phase 2.4 findings
12. [Deferred Phase 2.5+ concerns](deferred-phase2.5-concerns.md)
13. [Remaining operational blockers](remaining-operational-blockers.md)

## Phase 2.4 implementation

Five additional narrow fixes land in Phase 2.4 itself (covered by
Phase 2.5/2.6/2.8 work that follows):

- **Operation TTL janitor** (Phase 2.5 work) — periodic cleanup of
  terminal operations older than a retention window, while preserving
  `deployment_history` lineage.
- **Succeeded-stale circuit-breaker integration** (Phase 2.6) — prevents
  pathological respec loops from burning quota indefinitely.
- **Post-destroy NOT_FOUND confirmation poll** (Phase 2.8) — Cloud Run
  destroy success no longer trusts the 200 OK alone.
- **`[RELEASE_LOST_OWNERSHIP]` writes a history row** (Phase 2.7) —
  sweep-vs-dispatcher conflicts now leave a forensic trace.
- **Stuck-state detector** (Phase 2.8) — passive scan flags targets
  stuck in transitional states past threshold.

Each is documented in the relevant phase folder. This phase's
README focuses on the audit.

## Hard rules preserved

- Kubernetes path untouched.
- Cloud changes additive only.
- No new providers, no workflow engines, no distributed queues.
- Truthful state over optimistic UX remains the tie-breaker.

## Related
- [[../phase2.3/README]]
- [[../current-state]]
