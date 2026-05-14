# Phase 2.5 — Operational Hygiene, Retention & Entropy Control

## Last Updated
2026-05-14

## Goal

Prevent long-running operational decay. Implement TTL-based cleanup of
operations and deployment_history while preserving replay safety,
incident reconstruction capability, and forensic lineage.

## Documents

- [Operation cleanup design](operation-cleanup-design.md)
- [Retention policy](retention-policy.md)
- [Cleanup-safety analysis](cleanup-safety-analysis.md)
- [Deferred Phase 2.5 follow-ups](deferred-followups.md)

## Implementation

A new `pkg/reconciler/cleanup.go` runs as a separate goroutine
alongside the reconciler. Daily cadence. Configurable retention via
env vars (defaults documented in retention-policy.md).

## Hard rules preserved

- Cleanup never destroys replay safety: the startup sweep only queries
  in-progress ops, never affected by cleanup.
- Cleanup never destroys live ownership: the FK-guard predicate
  prevents deleting ops referenced by `current_operation`.
- Cleanup is best-effort: failures logged, do not propagate.
- Cleanup is auditable: every pass logs `[CLEANUP_OPS]` /
  `[CLEANUP_HISTORY]` with deleted counts.
- Cleanup is disable-able: `AUTOSTACK_CLEANUP_ENABLED=false` skips
  cleanup entirely (compliance-hold use case).

## Related
- [[../phase2.4/operation-retention-ttl-proposal]]
- [[../phase2.4/operational-entropy-assessment]]
