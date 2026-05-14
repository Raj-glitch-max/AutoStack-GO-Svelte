# Phase 2.6 — Chaos Survivability & Failure Modeling

## Last Updated
2026-05-14

## Goal

Survive operational chaos without corrupting lifecycle truth. Phase 2.3
and Phase 2.4 reviewed survivability; Phase 2.6 closes the remaining
implementation gaps that fall in scope for chaos hardening.

## Documents

- [Chaos scenarios catalog](chaos-scenarios-catalog.md) — extends Phase 2.3's incident reconstruction with chaos-specific cases
- [Succeeded-stale loop guard design](succeeded-stale-guard.md)
- [Runtime sweep design](runtime-sweep-design.md)
- [Deferred Phase 2.6 follow-ups](deferred-followups.md)

## Implementation landing in Phase 2.6

1. **Succeeded-stale circuit-breaker integration.** `succeeded_stale`
   outcomes now increment a per-target stale-count. After 3
   consecutive stale outcomes, the target is held in `error` until
   operator action. Closes the pathological respec loop hazard
   ([[../phase2.4/reconciliation-convergence-assessment]] C-1).
2. **Runtime sweep goroutine.** A periodic goroutine ticks every 5
   minutes and reclaims ops whose `updated_at` is older than 2 ×
   heartbeat liveness window. Closes the post-first-heartbeat-death
   stuck-state hazard ([[../phase2.4/ownership-integrity-review]] OS-2,
   OS-7).

## What does NOT land in 2.6

- Pod-identity stamping (Phase 2.7).
- Orphan-cleanup scanner (Phase 2.7).
- Per-cloud-account circuit-breaker (Phase 2.7).
- Graceful-shutdown for in-flight ops (Phase 2.8 — coordinated with
  PocketBase lifecycle integration).
- Per-rollout DeployTimeout (Phase 3 — operator UX).

## Hard rules preserved

- Kubernetes untouched.
- Cloud changes additive.
- Provider-neutral semantics.
- Truthful state over optimistic UX.
- No new providers / engines / queues.

## Related
- [[../phase2.3/replay-safety-assessment]]
- [[../phase2.4/ownership-integrity-review]]
- [[../phase2.4/reconciliation-convergence-assessment]]
