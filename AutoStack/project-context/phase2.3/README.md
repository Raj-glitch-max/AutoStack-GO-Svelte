# Phase 2.3 — Control-Plane Integrity Audit

## Last Updated
2026-05-14

## Purpose

Phase 2.3 is **not** a feature phase. It is an integrity audit of the cloud
control plane that landed in Phase 2.0/2.1/2.2:

- Deploy/Destroy dispatch via CAS-claimed operations.
- Heartbeat sidecar + startup sweep.
- Deployment history (two-row dispatch + outcome).
- AES-256-GCM at-rest credential encryption.
- Stale-spec detection + suspicion-counted convergence flap tolerance.
- Cloud Run provider (Deploy, GetStatus, Destroy implemented honestly;
  Rollback / GetMetrics / GetOperation / CheckQuotas refused).

The audit asks one question, twelve times:
**Can the control plane lie about state under operational chaos?**

The directive's instruction was: *document the concern first, propose a
fix, decide whether immediate action is warranted — only then implement.*
The assessment documents in this directory are the "document the concern"
step. Implementation work is gated on the inventory in
`dangerous-ambiguity-inventory.md`.

## Index

1. [Replay safety](replay-safety-assessment.md) — restart/replay/abandoned-op
2. [Reconciliation determinism](reconciliation-determinism-review.md) — same input → same output
3. [Deployment lineage integrity](lineage-integrity-review.md) — history truthfulness
4. [Truthful-state](truthful-state-assessment.md) — never claim success before convergence
5. [Eventual-consistency hazards](eventual-consistency-hazards.md) — Cloud Run propagation lag
6. [Operation ownership integrity](ownership-integrity-review.md) — CAS / sweep / heartbeat
7. [Long-running operation survivability](lro-survivability-review.md) — slow deploys past sweep threshold
8. [Rollback integrity](rollback-integrity-assessment.md) — current state is refusal, future-fitness
9. [Delete & orphan risk](delete-orphan-risk-assessment.md) — destroy replay & cascade-delete hazards
10. [Incident reconstruction](incident-reconstruction-assessment.md) — 10 scripted incident walks
11. [Operational maintainability](maintainability-review.md) — future workers / queues / scale
12. [Observability integrity](observability-integrity.md) — correlation IDs / lifetimes
13. [Encryption integrity](encryption-integrity-assessment.md) — fail-closed semantics
14. [Dangerous ambiguity inventory](dangerous-ambiguity-inventory.md) — prioritized findings
15. [Deferred Phase 2.5 concerns](deferred-phase2.5-concerns.md) — what moves out of scope
16. [Remaining operational blockers](remaining-operational-blockers.md) — what still must land

## Phase 2.3 implementation outcome

After auditing, the following narrow, additive safety fixes are landing
in Phase 2.3 itself (everything else is deferred — see #15):

- **History write at intent boundaries.** `createPendingDeploymentTarget`,
  `markCloudTargetForDestroy`, and `flipCloudTargetsToPendingOnRespec`
  now write a `deployment_history` row so lineage starts at the
  operator's intent, not at the dispatcher's claim.
- **Cycle-ID propagation into dispatch logs.** The reconciler's `cycle_id`
  is threaded into `[DISPATCH_CLAIM]`, `[DEPLOY_START]`, `[DEPLOY_END]`,
  `[DISPATCH_PANIC]`, `[RELEASE_LOST_OWNERSHIP]`, `[HISTORY_WRITE]`, and
  `[OP_ABANDONED]`. Cross-component grep becomes possible.
- **Heartbeat-aware startup sweep.** The sweep now ignores ops whose
  `updated_at` heartbeat is fresher than `2 × heartbeatInterval`. This
  closes the long-deploy-vs-restart race for single-pod restarts where
  the dying process didn't actually die — it restarted under a hot
  reload window. Pod-identity stamping is still deferred for true
  multi-pod safety.
- **`writeHistory` provider value consistency.** The dispatcher's
  history rows now record `deployment_targets.provider` (`gcp-cloudrun`)
  instead of `cloud_accounts.provider` (`gcp`), matching the target row.
- **Cycle correlation field on `operations`.** When opening an op, the
  dispatcher stamps `cycle_id` into the operation row so the on-disk
  operation always references its dispatching cycle.

Everything else this audit identified — pending-destroy re-arm, post-destroy
confirmation poll, multi-pod pod-identity stamping, runtime-stale-op sweep,
operation TTL/archival, rollback lineage, structured logger — is documented
in `deferred-phase2.5-concerns.md` and held until Phase 2.5+.

## Hard rules followed

- The Kubernetes path was not touched. `pkg/k8s/`, the operator, and the
  CRD schema were not opened.
- Every cloud change is additive (new code, new branches inside existing
  cloud-only paths). No existing cloud behavior was renamed, removed, or
  refactored.
- No new providers, no workflow engines, no distributed queues, no fake
  enterprise abstractions.
- The directive's bar — truthful state over optimistic UX — was the
  tie-breaker for every "should we ship this or defer" decision.

## Related

- [[../current-state]]
- [[../known-issues/phase2.2-assessment]]
- [[../known-issues/dangerous-edge-cases]]
- [[../known-issues/deferred-operational-hardening]]
