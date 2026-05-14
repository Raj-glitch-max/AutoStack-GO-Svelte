# Dangerous Ambiguity Inventory — Phase 2.4

## Last Updated
2026-05-14

## Severity rubric

- **CRITICAL** — silent corruption or permanent operator misdirection.
- **HIGH** — transient lie or silent intent loss not easily detectable.
- **MEDIUM** — operator-visible degradation; manual recovery; state on
  disk correct.
- **LOW** — observable rough edges; no correctness or trust impact.

## Findings ranked

### CRITICAL

None open.

### HIGH

| # | Concern | Doc | Phase 2.4 action |
|---|---|---|---|
| H-1 | `error + pending_destroy=true` is a stuck state. The flag was set during in-flight deploy; deploy then failed; no path consumes the flag. Destroy intent lost. | [[reconciliation-convergence-assessment]] C-3 | **Fix: reconciler auto-promotes `error + pending_destroy=true + endDate still set` to `deleting`.** |
| H-2 | Stuck-window in heartbeat-aware sweep: process crashes after first heartbeat but before next; sweep classifies live; target stuck. | [[ownership-integrity-review]] OS-2 | **Defer to Phase 2.6 (runtime sweep).** Documented. |

### MEDIUM

| # | Concern | Doc | Phase 2.4 action |
|---|---|---|---|
| M-1 | `succeeded_stale` pathological loop — no circuit-breaker bound. | [[reconciliation-convergence-assessment]] C-1 / [[retry-amplification-review]] A-4 | Defer to Phase 2.6. |
| M-2 | In-memory `Reconciler.failures` not cleared on respec recovery → target stays "circuit open" until process restart. | [[operational-entropy-assessment]] E-4 | **Fix: clear in-memory failures + suspicion on dispatch-eligible pending entry.** |
| M-3 | In-memory `Reconciler.suspicions` not cleared on respec recovery. | [[operational-entropy-assessment]] E-5 | Same fix as M-2. |
| M-4 | Phase 2.4 H-1 fix could interfere with operator manual recovery if not checked against rollout.endDate. | [[operational-entropy-assessment]] E-9 | **Verify endDate != "" before auto-promotion.** |
| M-5 | Manual cloud mutation drift undetected. | [[drift-persistence-assessment]] D-1 | Defer to Phase 2.8. |
| M-6 | Operations + history grow forever. | [[operational-entropy-assessment]] E-1, E-2 | Defer to Phase 2.5 TTL. |
| M-7 | Orphan cloud resources after admin-delete. | [[operational-entropy-assessment]] E-3 | Defer to Phase 2.5 scanner. |
| M-8 | Per-cloud-account quota cascade / auth cascade hammers provider. | [[retry-amplification-review]] A-1, A-2 | Defer to Phase 2.5 per-account backoff. |
| M-9 | `dispatcher panic` outcome loses external_id from history. | [[lifecycle-closure-integrity-review]] LC-9 | Defer to Phase 2.8 (provider work). |
| M-10 | `[RELEASE_LOST_OWNERSHIP]` writes no history row. | [[lifecycle-closure-integrity-review]] LC-10 / [[incident-reconstruction-maturity-review]] IR-7 | Defer to Phase 2.7. |
| M-11 | Cloud Run failed-revision serving the previous revision: AutoStack reports `error`, doesn't expose `serving_revision`. | [[drift-persistence-assessment]] D-7 | Defer to Phase 2.5. |
| M-12 | Stale `last_state_change_at` written but never consumed (stuck-state detection unimplemented). | (existing known issue) | Defer to Phase 2.8 (stuck-state detector). |

### LOW

| # | Concern | Doc | Phase 2.4 action |
|---|---|---|---|
| L-1 | Startup mass-respec storm: N targets dispatched in single cycle, provider load spike. | [[retry-amplification-review]] A-3 | Defer to Phase 2.5 (worker pool / dispatch staggering). |
| L-2 | Heartbeat-fail-storm during long DB busy windows; no escalation. | [[retry-amplification-review]] A-5 / [[incident-reconstruction-maturity-review]] IR-5 | Defer to Phase 2.7. |
| L-3 | CAS-race-loss writes no history row. | [[lifecycle-closure-integrity-review]] LC-8 / [[incident-reconstruction-maturity-review]] IR-8 | Defer to Phase 2.7. |
| L-4 | Cloud Run create-vs-update branch on transient GetService error. | (Phase 2.3 E-4) | Defer to Phase 2.5. |
| L-5 | Cleanup activity visibility (when Phase 2.5 lands). | [[incident-reconstruction-maturity-review]] IR-9 | Land alongside Phase 2.5 cleanup. |

## What lands in Phase 2.4

Three narrow fixes, all in the reconciler:

1. **H-1**: Auto-promote `error + pending_destroy + endDate-still-set`
   to `deleting`.
2. **M-2 + M-3**: Clear in-memory failures and suspicions when target
   enters fresh dispatch-eligible state (pending + no current_operation).
3. **M-4**: Phase 2.3's `pending_destroy` re-arm now verifies the
   rollout's `endDate` is still set before honoring the flag on the
   success path. Symmetric with H-1.

## What is deferred

Everything else (M-5 through M-12, L-1 through L-5) is deferred to
Phase 2.5–2.8 with clear designs documented in this directory and
[[deferred-phase2.5-concerns]].

## Related
- [[deferred-phase2.5-concerns]]
- [[remaining-operational-blockers]]
- [[../phase2.3/dangerous-ambiguity-inventory]]
