# Phase 2.9 — Phase 2 Finalization

## Last Updated
2026-05-14

## Goal

Close Phase 2. Land the three remaining correctness fixes identified by
the Phase 2 finalization review, freeze the contracts and architecture,
and produce a single trustworthiness verdict the team can stand behind
before staging.

The audit set (Phase 2.3) found the structural gaps. The hardening phases
(2.4–2.8) closed them in code. Phase 2.9 verifies, contracts, and signs.

## Documents

### Contracts (frozen for Phase 2)

- [Lifecycle contracts](lifecycle-contracts.md) — DC-1 … DC-8: what the
  control plane guarantees and what it declines to.
- [Provider contracts](provider-contracts.md) — P-1 … P-15: REQUIRED,
  OPTIONAL, and DECLINED provider semantics.
- [Reconciliation architecture freeze](reconciliation-architecture-freeze.md)
  — F-1 … F-9 stable invariants, E-1 … E-4 accepted issues, U-1 … U-3
  unsafe-without-redesign items.
- [Operational guarantee matrix](operational-guarantee-matrix.md) — G-1
  … G-19: every guarantee, its mechanism, evidence, and failure mode.
- [Safe operational boundaries](safe-operational-boundaries.md) — the
  envelope inside which the guarantees hold.

### Verdict

- [Trustworthiness verdict](trustworthiness-verdict.md) — the
  overall Phase 2 closure statement and 9-dimension score.
- [Production readiness gate](production-readiness-gate.md) — full
  9-dimension readiness assessment.
- [Architectural weaknesses](architectural-weaknesses.md) — AW-C1/C2/C3
  (mandatory) + AW-S1 … AW-S5 (Phase 3).

### Implementation record

- [Mandatory fixes implementation](mandatory-fixes-implementation.md) —
  the actual code changes for AW-C1, AW-C2 (verified, no change), AW-C3.

### Assessments (audit inputs)

- [Reconciliation convergence assessment](reconciliation-convergence-assessment.md)
  — root cause of AW-C1.
- [Replay determinism assessment](replay-determinism-assessment.md) —
  `confirmDeleted` analysis (AW-C3 root cause).
- [Lifecycle closure assessment](lifecycle-closure-assessment.md) — root
  cause of AW-C2 and AW-C3.
- [Operational ambiguity inventory](operational-ambiguity-inventory.md)
  — UA-1, UA-2 surfaced and resolved.
- [Forensic reconstruction assessment](forensic-reconstruction-assessment.md)
  — the 7 incident-reconstruction paths.
- [Drift survivability assessment](drift-survivability-assessment.md) —
  long-term divergence behavior.
- [Chaos survivability assessment](chaos-survivability-assessment.md) —
  C-1 … C-9 chaos scenarios.
- [Observability integrity assessment](observability-integrity-assessment.md)
  — log tag coverage, OG-1 gap.
- [Operational maintainability assessment](operational-maintainability-assessment.md).

### Forward planning

- [Deferred Phase 2.9 follow-ups](deferred-followups.md) — the three
  blocking fixes and verification checklist (now landed).
- [Deferred Phase 3 concerns](deferred-Phase3-concerns.md) — the 19-item
  Phase 3 backlog: schema, provider, reconciliation, observability,
  cleanup.

## What landed in Phase 2.9

Three single-file correctness fixes:

1. **AW-C1: `running + pending_destroy` auto-promote** —
   `pkg/reconciler/cloud.go:542` adds `previousStatus == "running"` to
   the H-1 auto-promote condition. Operator setting `endDate` on a
   `running` target is no longer silently dropped.

2. **AW-C2: Panic recovery** — verified the existing panic defer in
   both dispatchers already calls `completeOperation` + `releaseTarget`
   synchronously. No code change required. The hard-crash gap between
   `completeOperation` and `releaseTargetWithExternal` is accepted for
   Phase 2 (Phase 3 atomic-release closes it).

3. **AW-C3: Heartbeat outlives provider return** —
   `pkg/reconciler/dispatch.go:203` (deploy) and `:363` (destroy)
   scope the heartbeat goroutine to the outer `ctx`, not the inner
   `deployCtx`/`destroyCtx`. The heartbeat now persists through
   `confirmDeleted` and through release. Sweep cannot reclaim a live op
   during the confirm window.

Build and `go vet` are clean.

## What did NOT land in Phase 2.9 (deferred to Phase 3)

The full 19-item Phase 3 backlog is in
[deferred-Phase3-concerns.md](deferred-Phase3-concerns.md). The most
significant items:

- `deployed_spec` snapshot column + drift detection cycle (SC-1 + DO-2).
- `owned_by_pod` stamping for multi-pod safety (SC-5).
- `serving_revision` field on `TargetStatus` (PR-1).
- Real Cloud Run rollback via `Service.Traffic` (PR-4).
- Cloud Billing API for live cost estimates (PR-6).
- `CheckQuotas` live implementation (PR-7).
- Worker pool for parallel dispatch (RA-1).
- Per-target backoff and persistent circuit state (RA-2, RA-3).
- `log/slog` adoption (DO-1).
- Metrics export (DO-3).
- Orphan cleanup scanner (OC-1).

## Hard rules preserved

- Kubernetes path untouched.
- Cloud changes additive — no `if provider == "aws"` in core code.
- Provider interface unchanged.
- CAS-based dispatch unchanged.
- Encryption design unchanged.
- Suspicion counter / transition guard / status-poll skip-guards unchanged.
- `Reconciler.Start()` invariant (sweep before ticker) preserved.

## Operating envelope after Phase 2.9

Single PocketBase pod. ≤ 20 cloud targets. Cloud Run only.
SQLite WAL mode. See
[safe-operational-boundaries.md](safe-operational-boundaries.md) for
the full envelope.

## Related

- [[../phase2.3/dangerous-ambiguity-inventory]] — the original
  hazard inventory that drove the 2.4–2.9 hardening.
- [[../phase2.8/deferred-followups]] — the Phase 2.8 closure document
  superseded by [deferred-Phase3-concerns.md](deferred-Phase3-concerns.md).
- [[../current-state]] — top-level state summary.
