# Dangerous Ambiguity Inventory — Phase 2.3

## Last Updated
2026-05-14

## Purpose

The Phase 2.3 directive: identify every place the control plane can be
ambiguous, lie silently, or corrupt lineage. Then prioritize: which
must land this phase, which is deferred, which is an accepted risk.

This is the master prioritization list. Other documents in this phase
deep-dive each category; this one ranks them.

## Severity rubric

- **CRITICAL** — system can silently corrupt provider state or
  permanently mislead operators.
- **HIGH** — system can transiently lie or silently lose intent in a
  way operators cannot easily detect.
- **MEDIUM** — operator-visible degradation; recovery requires manual
  steps but the state is correct on disk.
- **LOW** — observable rough edges; no correctness or trust impact.

## The inventory, ranked

### CRITICAL — none open in Phase 2.3

The pre-Phase-2.0 critical gaps (phantom Deploy, phantom Rollback,
plaintext-credentials-named-encrypted) are all closed. No CRITICAL
items remain open today.

### HIGH

| # | Concern | Doc | Phase 2.3 action |
|---|---|---|---|
| H-1 | Destroy intent silently lost when endDate set during in-flight deploy. Cloud Run service runs forever. | [[delete-orphan-risk-assessment]] D-1 | **Fix: pending_destroy column + dispatcher re-arm.** |
| H-2 | Multi-pod startup sweep clobbers peer pod's live ops. AutoStack denies a real service. | [[ownership-integrity-review]] O-1 | Document; defer (no multi-pod today). Phase 2.5 work: pod-identity stamping. |
| H-3 | Rolling-restart on AutoStack pod aggressively abandons the previous pod's in-flight deploy. | [[ownership-integrity-review]] O-2, [[incident-reconstruction-assessment]] I-4 | **Fix: heartbeat-aware sweep with first-heartbeat guard.** |

### MEDIUM

| # | Concern | Doc | Phase 2.3 action |
|---|---|---|---|
| M-1 | `deployment_history.provider` value is `gcp` (account-side enum) not `gcp-cloudrun` (target-side enum). Inconsistent across collections. | [[lineage-integrity-review]] L-1 | **Fix: pass canonical provider into writeHistory.** |
| M-2 | Intent-boundary history rows missing (createTarget, respec, destroy-mark). | [[lineage-integrity-review]] L-3 | **Fix: writeHistory at controller intent points.** |
| M-3 | `cycle_id` not propagated into dispatch logs. Cross-component correlation broken. | [[observability-integrity]] O-1, [[truthful-state-assessment]] T-8 | **Fix: thread cycle_id through dispatch.** |
| M-4 | Post-destroy NOT_FOUND confirmation poll missing. Brief "deleted but service still listable" window. | [[delete-orphan-risk-assessment]] D-2, [[eventual-consistency-hazards]] E-3 | Defer to Phase 2.5. |
| M-5 | `succeeded_stale` history written as `status=failed` (enum limitation). Operators may misread "failed". | [[lineage-integrity-review]] L-5 | Defer to Phase 2.5 (schema migration to add `stale` value). |
| M-6 | Pathological respec loop never trips circuit (succeeded_stale doesn't increment failures). | [[replay-safety-assessment]] §3 | Defer; no production evidence. |
| M-7 | Cloud Run create/update branch is wrong on transient GetService error → 409 from CreateService. Loud failure but unnecessary. | [[eventual-consistency-hazards]] E-4 | Defer to Phase 2.5. |
| M-8 | Stuck-state detection unimplemented; `last_state_change_at` written but unread. | [[../known-issues/dangerous-edge-cases]] (existing); [[../known-issues/deferred-operational-hardening]] §6 | Defer. |
| M-9 | Drift detection unimplemented. `drift_detected` permanently false. | [[../known-issues/dangerous-edge-cases]] (existing) | Defer (Phase 3). |
| M-10 | `provider` triplicate enums across `cloud_accounts`/`deployment_targets`/`rollouts`. No cross-validation. | [[../known-issues/dangerous-edge-cases]] (existing) | Defer; documented. |

### LOW

| # | Concern | Doc | Phase 2.3 action |
|---|---|---|---|
| L-1 | `operations` table never expires; grows forever. | [[../known-issues/phase2.2-assessment]] §4 | Defer (TTL cleanup). |
| L-2 | History `failed` outcome from hard error loses external_id (partial-create orphan trace). | [[lineage-integrity-review]] L-2 | Defer; provider work. |
| L-3 | `deployment_history.target` cascadeDelete wipes lineage when target is admin-deleted. | [[lineage-integrity-review]] L-4 | Defer; migration. |
| L-4 | `deployment_history` has no `operation` foreign key. Forensic correlation by timestamp only. | [[lineage-integrity-review]] L-6 | Defer; migration. |
| L-5 | `writeHistory` failures are silently logged; outcome row can be permanently missed. | [[lineage-integrity-review]] L-7 | Defer; acceptable. |
| L-6 | `RELEASE_LOST_OWNERSHIP` writes no audit history row — dispatcher's actual outcome lost from history when sweep took it. | [[lineage-integrity-review]] L-3 (related) | Defer; add a "lost ownership" history row in Phase 2.5. |
| L-7 | `CAS-race-loss` (rare; one dispatcher cancels) writes no audit. | [[lineage-integrity-review]] | Defer. |
| L-8 | Suspicion counter is in-memory, lost on restart. First-error-post-restart can incorrectly persist. | comment in `cloud.go` | Accepted; documented. |
| L-9 | EnsureConfigured logs but doesn't refuse to start. Cloud features fail closed; non-cloud features keep running. | [[encryption-integrity-assessment]] EI-1 | Accepted; documented. |
| L-10 | `waitForServiceReady`'s 1-hour internal deadline is misleading (parent ctx wins at 15 min). | [[../known-issues/phase2.2-assessment]] | Defer; cosmetic. |
| L-11 | Legacy-plaintext fallback in `Decrypt` never times out. Operators may never run validate on legacy rows. | [[encryption-integrity-assessment]] EI-3 | Defer to Phase 2.5 (one-shot migration). |
| L-12 | Reconciler creates new provider client per call. Wasteful at scale. | [[maintainability-review]] | Defer. |
| L-13 | No per-target metrics (success/fail/duration histogram). | [[observability-integrity]] O-7 | Defer (Phase 2.5 instrumentation). |
| L-14 | `[HEARTBEAT_FAIL]` doesn't escalate. | [[observability-integrity]] O-9 | Defer; trivial. |
| L-15 | `OP_COMPLETE_NOOP` doesn't distinguish sweep-vs-double. | [[observability-integrity]] O-6 | Defer; trivial. |
| L-16 | `markCloudTargetForDestroy` skips `pending` rows? Re-read: no, only `deleted/deleting`. Pending is flipped to deleting. ✓ (No issue; included to confirm.) | | n/a |

## What lands in Phase 2.3

Six items will be implemented in this phase, all narrow and additive:

1. **H-1**: `pending_destroy` column + dispatcher re-arm.
2. **H-3**: Heartbeat-aware sweep with first-heartbeat guard.
3. **M-1**: Canonical provider value in `writeHistory`.
4. **M-2**: Intent-boundary history rows.
5. **M-3**: `cycle_id` threaded into dispatch logs.

(A sixth, `operations.cycle_id` column, was reconsidered and deferred
to Phase 2.5 to avoid schema-migration churn. Log correlation via M-3
is sufficient operationally.)

## What is explicitly deferred

Everything else above is deferred — most to Phase 2.5, a few to Phase 3.
See [[deferred-phase2.5-concerns.md]] for the consolidated Phase 2.5
backlog.

## What is explicitly accepted (no fix planned)

- L-8 (in-memory suspicion counter): documented constraint.
- L-9 (EnsureConfigured logs not refuses): trade-off chosen
  intentionally.
- L-16: not an issue; confirmed clean.

## Decision audit trail

Every item above has been considered against four questions, per the
directive:

1. *What is the concern?*
2. *Why does it matter?*
3. *Is the fix temporary or structural?*
4. *Should we ship this fix now or defer?*

The five Phase 2.3 fixes were chosen because they are:
- High-value (each closes a real ambiguity).
- Narrow (no architectural change, no provider rewrites).
- Additive (no existing behavior broken).
- Independent (each can land alone if another fails review).

## Related
- [[deferred-phase2.5-concerns]]
- [[remaining-operational-blockers]]
- [[../current-state]]
