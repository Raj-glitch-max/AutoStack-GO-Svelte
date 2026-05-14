# Phase 2.8 — Drift, Divergence & Truthfulness Maturity

## Last Updated
2026-05-14

## Goal

Ensure truthful state under long-term provider divergence. The platform
must never silently normalize divergence into "healthy".

## Documents

- [Drift handling maturity](drift-handling-maturity.md)
- [Post-destroy confirmation poll](post-destroy-confirm-poll.md)
- [Manual cloud mutation policy](manual-cloud-mutation-policy.md)
- [Deferred Phase 2.8 follow-ups](deferred-followups.md)

## Implementation landing in Phase 2.8

1. **Post-destroy NOT_FOUND confirmation poll.** Cloud Run provider's
   `Destroy` now polls `GetService` after `DeleteService` until
   NOT_FOUND or a bounded timeout (60s). The dispatcher trusts the
   200 OK from `DeleteService` for less time; truthfulness window
   closed. See [[post-destroy-confirm-poll]].

## NOT landing in Phase 2.8

- Spec-vs-actual drift detection (large feature; deferred to Phase 3).
- Per-cloud-account backoff / circuit-breaker.
- Stuck-state detector goroutine (deferred to Phase 2.9 — needs
  separate goroutine architecture).
- Cloud Run `serving_revision` field (provider extension; Phase 3).
- Cloud Run create-vs-update transient retry (Phase 3).
- Region-scoped credential validation (Phase 3).
- Cloud_account region/delete refusal (Phase 2.9 controller work).

## Hard rules preserved

- Kubernetes untouched.
- Cloud changes additive.
- Truthful state over optimistic UX.

## Related
- [[../phase2.3/eventual-consistency-hazards]]
- [[../phase2.4/drift-persistence-assessment]]
