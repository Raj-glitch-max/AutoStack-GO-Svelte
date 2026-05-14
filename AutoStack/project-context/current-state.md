# AutoStack — Current State (Phase 3.0)

## Last Updated
2026-05-14 (Phase 3.0 — Phase 3 foundation contracts authored)

## Active Phase

**Phase 3.0** — multi-provider orchestration foundation contracts.
Code: unchanged from Phase 2.9 closure. Documentation: the 8 Phase 3.0
foundation docs are landed. Next: Phase 3.0 implementation work
(Capability framework in code), then Phase 3.1 (first non-Cloud-Run
provider under capability contracts).

See [phase3/README.md](phase3/README.md) for the Phase 3 sub-phase plan.

## Phase 3.0 foundation contracts (landed in docs, not yet in code)

| Doc | Purpose |
|---|---|
| [phase3/phase3-architecture-evolution.md](phase3/phase3-architecture-evolution.md) | Trajectory, HC-1..HC-8 hard constraints, non-goals |
| [phase3/multi-provider-risk-analysis.md](phase3/multi-provider-risk-analysis.md) | R-1..R-12 collapse modes + mitigations |
| [phase3/provider-capability-matrix.md](phase3/provider-capability-matrix.md) | C-* capability framework + Cloud Run baseline |
| [phase3/provider-normalization-rules.md](phase3/provider-normalization-rules.md) | NORMALIZE / AMBIGUATE / EXPOSE decision procedure |
| [phase3/ambiguity-semantics-model.md](phase3/ambiguity-semantics-model.md) | S-1..S-5 ambiguity sources; P-1..P-6 propagation rules |
| [phase3/provider-contract-evolution.md](phase3/provider-contract-evolution.md) | Provider interface changes (Capabilities, ServingRevision, etc.) |
| [phase3/lifecycle-normalization-model.md](phase3/lifecycle-normalization-model.md) | N-1..N-8 lifecycle mapping rules |
| [phase3/future-ha-boundary-analysis.md](phase3/future-ha-boundary-analysis.md) | What HA is in-scope and what is Phase 4+ |

## Phase 3.0 implementation work (NOT yet started)

The foundation docs are contracts. Phase 3.0 also requires (when ready):

1. Add `Capabilities()` to `Provider` interface in `pkg/providers/provider.go`.
2. Define `CapabilitySet`, `Capability`, and `CapabilityKey` types.
3. Implement Cloud Run's `capabilities.go` with the Phase 2 baseline profile.
4. Add `lifecycle_ambiguous` and `lifecycle_native_state` columns (migration).
5. Add `ServingRevision string` to `TargetStatus` (Form B return-field).
6. Ensure build + vet remain clean.
7. Add `contract_test.go` for Cloud Run asserting capability-claim correctness.

No code has been touched yet. The Phase 2 codebase is intact.

## Phase 2.9 summary

Phase 2.9 closed Phase 2. The three blocking correctness fixes
identified by the Phase 2 finalization review are landed, contracts are
frozen, and the trustworthiness verdict is signed.

`go build ./...` and `go vet ./...` are clean. Only the `secrets`
package has tests today and they pass.

### What landed in Phase 2.9

1. **AW-C1: `running + pending_destroy` auto-promote** —
   `pkg/reconciler/cloud.go:542`. The H-1 auto-promote condition now
   matches `previousStatus == "running"` in addition to `"error"`.
   Operators setting `endDate` on a `running` target route through the
   destroy lifecycle within one cycle.

2. **AW-C2: Panic recovery verified** — the existing panic defer in
   both `dispatchDeploy` and `dispatchDestroy` already calls
   `completeOperation` and `releaseTarget` synchronously before stack
   unwind. No code change. The narrow hard-crash gap between
   `completeOperation` and `releaseTargetWithExternal` is accepted for
   Phase 2; Phase 3 atomic-release closes it.

3. **AW-C3: Heartbeat outlives provider return** —
   `pkg/reconciler/dispatch.go:203` (deploy) and `:363` (destroy). The
   heartbeat goroutine is now scoped to the outer `ctx`, not the inner
   `deployCtx`/`destroyCtx`. The heartbeat persists across the
   `confirmDeleted` poll loop and through release. Sweep cannot reclaim
   a live op during the confirm window.

### Frozen contracts (Phase 2 final)

- [Lifecycle contracts (DC-1..DC-8)](phase2.9/lifecycle-contracts.md)
- [Provider contracts (P-1..P-15)](phase2.9/provider-contracts.md)
- [Reconciliation architecture freeze (F-1..F-9, E-1..E-4, U-1..U-3)](phase2.9/reconciliation-architecture-freeze.md)
- [Operational guarantee matrix (G-1..G-19)](phase2.9/operational-guarantee-matrix.md)
- [Safe operational boundaries](phase2.9/safe-operational-boundaries.md)

### Verdict and assessments

- [Trustworthiness verdict](phase2.9/trustworthiness-verdict.md)
- [Production readiness gate](phase2.9/production-readiness-gate.md)
- [Architectural weaknesses](phase2.9/architectural-weaknesses.md)
- [Mandatory fixes implementation](phase2.9/mandatory-fixes-implementation.md)
- 13 supporting audit/assessment docs in
  [phase2.9/](phase2.9/README.md)

### Operating envelope

- Single PocketBase pod.
- ≤ 20 cloud targets per reconciler instance.
- Cloud Run is the only Phase 2 provider.
- SQLite WAL mode.

See [phase2.9/safe-operational-boundaries.md](phase2.9/safe-operational-boundaries.md) for full envelope.

## Phase 3 readiness

The Phase 3 backlog is fully catalogued in
[phase2.9/deferred-Phase3-concerns.md](phase2.9/deferred-Phase3-concerns.md):
19 items across Schema (SC-1..SC-5), Provider (PR-1..PR-7),
Reconciliation Architecture (RA-1..RA-3), Drift/Observability
(DO-1..DO-3), and Orphan Cleanup (OC-1).

No Phase 3 implementation has begun. Safe extension points are
documented in
[reconciliation-architecture-freeze.md](phase2.9/reconciliation-architecture-freeze.md)
("Phase 3 Extension Points").

The architectural invariants in
[reconciliation-architecture-freeze.md](phase2.9/reconciliation-architecture-freeze.md)
must not be violated by Phase 3 work.

## Earlier Phase summaries

- **Phase 2.8** — Drift handling, post-destroy NOT_FOUND confirmation
  poll, manual cloud mutation policy. See
  [phase2.8/README.md](phase2.8/README.md).
- **Phase 2.7** — Forensic completeness, structured logging proposal,
  release-lost-ownership history. See [phase2.7/README.md](phase2.7/README.md).
- **Phase 2.6** — Chaos scenarios catalog, runtime sweep design,
  succeeded-stale guard. See [phase2.6/README.md](phase2.6/README.md).
- **Phase 2.5** — Operation cleanup design, retention policy, cleanup
  safety. See [phase2.5/README.md](phase2.5/README.md).
- **Phase 2.4** — Drift persistence, lifecycle closure integrity,
  retry amplification, reconciliation convergence assessment. See
  [phase2.4/README.md](phase2.4/README.md).
- **Phase 2.3** — Control-plane integrity audit + 5 narrow safety
  fixes (Destroy-intent re-arm, Heartbeat-aware startup sweep,
  Canonical provider in history, Intent-boundary history writes,
  Cycle-ID in dispatch logs). See [phase2.3/README.md](phase2.3/README.md).
- **Phase 2.2** — End-to-end validation. See
  [known-issues/phase2.2-assessment.md](known-issues/phase2.2-assessment.md).
- **Phase 2.0 / 2.1** — CAS dispatcher, heartbeat infrastructure,
  release-CAS, status-poll skip-guards, suspicion counter, cycle_id
  correlation.
- **Phase 1.9** — Code-level behavioral-integrity review. See
  [architecture/control-plane-paranoia-findings.md](architecture/control-plane-paranoia-findings.md).

## Kubernetes path status

**UNCHANGED** through Phase 2.9.

- No modifications to k8s package.
- No modifications to rollout controller.
- No changes to operator.
- No changes to CRD schema.

## Related

- [phase2.9/README.md](phase2.9/README.md) — Phase 2.9 index
- [phase2.9/trustworthiness-verdict.md](phase2.9/trustworthiness-verdict.md)
- [phase2.9/deferred-Phase3-concerns.md](phase2.9/deferred-Phase3-concerns.md)
- [README.md](README.md) — project-context index
