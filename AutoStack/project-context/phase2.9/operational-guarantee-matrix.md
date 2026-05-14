# Operational Guarantee Matrix — Phase 2 Final

**Last Updated:** 2026-05-14

## Purpose

A single structured reference for **every operational guarantee AutoStack
makes in Phase 2**, the evidence backing each guarantee, and the contract
boundary that holds it. Read this when:

- An operator asks "can the system promise X?"
- A reviewer asks "where is X proven?"
- A Phase 3 change is proposed and you need to confirm it does not
  weaken a Phase 2 guarantee.

This document does not introduce new contracts. It consolidates the
guarantees defined across [[phase2.9/lifecycle-contracts]],
[[phase2.9/provider-contracts]],
[[phase2.9/reconciliation-architecture-freeze]], and
[[phase2.9/safe-operational-boundaries]] into a single matrix.

---

## How to Read This Matrix

| Column | Meaning |
|---|---|
| **Guarantee** | The operational property the system commits to. |
| **Scope** | The operating envelope inside which the guarantee holds. |
| **Mechanism** | The code/runtime artifact that produces the guarantee. |
| **Evidence** | The file + symbol that can be inspected to verify. |
| **Failure mode if violated** | What the operator observes if the mechanism were ever bypassed (i.e., what to look for in incident analysis). |
| **Contract anchor** | The contract clause this guarantee maps to. |

---

## G-1: Dispatch Exclusivity

| Field | Value |
|---|---|
| Guarantee | At most one reconciler goroutine executes `Provider.Deploy` or `Provider.Destroy` per target at any moment. |
| Scope | Single-pod envelope ([[phase2.9/safe-operational-boundaries]] §Single-Pod). |
| Mechanism | CAS claim — `UPDATE deployment_targets SET current_operation=? WHERE id=? AND current_operation='' AND status IN ('pending','deleting')` with `rows_affected == 1` check. |
| Evidence | `pkg/reconciler/dispatch.go` `claimTarget`; release-CAS in `releaseTargetWithExternal`. |
| Failure mode | Two `[DEPLOY_START]` log lines with the same `target=`, different `cycle=`, overlapping in time. |
| Contract anchor | DC-4.1 ([[phase2.9/lifecycle-contracts]]); F-2 ([[phase2.9/reconciliation-architecture-freeze]]). |

---

## G-2: Operation Singleton

| Field | Value |
|---|---|
| Guarantee | A target has at most one `operations` row in `status='in_progress'` at any time. |
| Scope | Single-pod envelope. |
| Mechanism | `createOperation` always inserts `status='in_progress'`; `completeOperation` uses `WHERE status='in_progress'` CAS; sweep transitions to `failed`. |
| Evidence | `pkg/reconciler/dispatch.go` `createOperation`, `completeOperation`; `pkg/reconciler/sweep.go`. |
| Failure mode | Query `SELECT count(*) FROM operations WHERE target=? AND status='in_progress'` ever returns `>1`. |
| Contract anchor | DC-4.2; F-3. |

---

## G-3: Sweep Honesty

| Field | Value |
|---|---|
| Guarantee | The sweep never infers that a mid-deploy crash means "deploy succeeded." Abandoned ops become `failed`, never `succeeded`. |
| Scope | Startup sweep and runtime sweep, single-pod. |
| Mechanism | Sweep marks `failed` unless the heartbeat fired at least once AND is within `2 × heartbeatInterval` (startup) or `5 min` (runtime). |
| Evidence | `pkg/reconciler/sweep.go` `SweepAbandonedOperations`, `RuntimeSweep`. |
| Failure mode | An op row transitions `in_progress → succeeded` without the dispatcher's `completeOperation` call (no `[DEPLOY_END]` log line). |
| Contract anchor | DC-1.5; F-4. |

---

## G-4: Release-CAS Ownership

| Field | Value |
|---|---|
| Guarantee | A dispatcher's release does not overwrite a sweep's state if the sweep already reclaimed the op. |
| Scope | Always — single-pod or multi-pod (the CAS predicate is universal). |
| Mechanism | `releaseTargetWithExternal` uses `WHERE current_operation = :opID`. If sweep cleared it first, 0 rows affected; `writeOwnershipLostHistory` preserves forensic context. |
| Evidence | `pkg/reconciler/dispatch.go` release path; log tag `[RELEASE_LOST_OWNERSHIP]`. |
| Failure mode | A target that the sweep set to `error` is silently restored to `running`/`updating` by a returning dispatcher. |
| Contract anchor | DC-4.3. |

---

## G-5: Status Transition Refusal

| Field | Value |
|---|---|
| Guarantee | Disallowed transitions (e.g. `deleted → running`, `running → pending` via poll) are refused and logged. |
| Scope | All `updateTargetStatus` calls. |
| Mechanism | `isAllowedTransition` predicate; refusal logs `[TRANSITION_REFUSED]`. |
| Evidence | `pkg/reconciler/cloud.go` `isAllowedTransition`, `updateTargetStatus`. |
| Failure mode | A target visibly regresses from `deleted` or skips lifecycle stages without a dispatcher event. |
| Contract anchor | F-5. |

---

## G-6: Suspicion Tolerance for Transient Flaps

| Field | Value |
|---|---|
| Guarantee | A single `updating → error` observation does not persist `error`. Two consecutive observations required. |
| Scope | `updating` → `error` transitions only. |
| Mechanism | `noteSuspectError` in-memory counter; cleared on any non-error observation. |
| Evidence | `pkg/reconciler/cloud.go` `noteSuspectError`, `clearSuspect`; log tag `[SUSPICION_HOLD]`. |
| Failure mode | A transient Cloud Run flap during deploy convergence flips the target to `error` on first sight. |
| Contract anchor | F-6; DC-7.1 ("suspicion_hold"). |

---

## G-7: Stale-Spec Loop Bound

| Field | Value |
|---|---|
| Guarantee | A respec-flapping rollout is bounded to at most 3 automatic re-dispatches before halting with `error` and explicit message. |
| Scope | Deploy path only. |
| Mechanism | In-memory `staleCount` map; threshold 3 → `releaseTargetWithExternal(creating, error, "pathological stale-spec loop")`. |
| Evidence | `pkg/reconciler/dispatch.go` stale handling in `dispatchDeploy`. |
| Failure mode | A rollout whose manifest moves every cycle drives unbounded `Provider.Deploy` calls. |
| Contract anchor | DC-1.4. |

---

## G-8: Destroy NOT_FOUND Confirmation

| Field | Value |
|---|---|
| Guarantee | A target reports `deleted` only after observing NOT_FOUND from the provider or after the bounded confirm window elapses (with `[DESTROY_CONFIRM_TIMEOUT]` recorded). |
| Scope | Cloud Run provider (the only provider implementing Destroy in Phase 2). |
| Mechanism | Phase 2.8 `confirmDeleted` poll loop after `DeleteService` returns 200. |
| Evidence | `pkg/providers/cloudrun/provider.go` `Destroy` + confirm loop; log tag `[DESTROY_CONFIRM_TIMEOUT]`. |
| Failure mode | A target shows `deleted` while Cloud Run still lists the service. |
| Contract anchor | DC-2.1; P-15. |

---

## G-9: Idempotent Deploy

| Field | Value |
|---|---|
| Guarantee | A second `Deploy` call against an existing service updates rather than fails. |
| Scope | All providers (REQUIRED contract). |
| Mechanism | `GetService` precheck → `UpdateService` path in the provider; reconciler may re-dispatch `pending` targets without divergence. |
| Evidence | `pkg/providers/cloudrun/provider.go` `Deploy`. |
| Failure mode | Repeated `Deploy` on the same target produces "already exists" errors that accumulate in the circuit. |
| Contract anchor | P-1. |

---

## G-10: Idempotent Destroy

| Field | Value |
|---|---|
| Guarantee | `Destroy` on an already-deleted target returns success (no circuit-accumulating error). |
| Scope | All providers (REQUIRED contract). |
| Mechanism | `GetService` NOT_FOUND check inside `Destroy`. |
| Evidence | `pkg/providers/cloudrun/provider.go` `Destroy`. |
| Failure mode | Re-dispatching a `deleting` target after external deletion causes a permanent circuit-open. |
| Contract anchor | P-2. |

---

## G-11: Honest Provider Capability

| Field | Value |
|---|---|
| Guarantee | A provider method that is not implemented returns `ErrNotImplemented` rather than `nil, nil` or fabricated success. |
| Scope | All Phase 2 providers. |
| Mechanism | Explicit `return nil, ErrNotImplemented` returns in `Rollback`, `GetOperation`, `GetMetrics`, `CheckQuotas`, `StreamLogs`, `GetActualCost`. |
| Evidence | `pkg/providers/cloudrun/provider.go` stubs. |
| Failure mode | UI/operator sees a "0% CPU" or "rollback succeeded" message for an unimplemented capability. |
| Contract anchor | P-10, P-11, P-12. |

---

## G-12: Lineage Completeness

| Field | Value |
|---|---|
| Guarantee | Every dispatch attempt, every terminal outcome, every sweep abandonment, and every ownership-loss race produces at least one `deployment_history` row. |
| Scope | All lifecycle events. |
| Mechanism | `writeHistory` calls at intent boundaries, dispatch terminal paths, sweep paths, and `writeOwnershipLostHistory` for the CAS-race forensic case. |
| Evidence | `pkg/reconciler/dispatch.go`, `pkg/reconciler/sweep.go`, `pkg/reconciler/cloud.go` intent boundaries. |
| Failure mode | An operator-visible lifecycle event has no row in `deployment_history`. |
| Contract anchor | DC-8; F-9. |

---

## G-13: Cycle Correlation in Logs

| Field | Value |
|---|---|
| Guarantee | Every dispatcher and reconciler log line carries `cycle=<8-hex>` so cross-component grep correlates a unit of work end-to-end. |
| Scope | All dispatch and reconcile log emissions. |
| Mechanism | `__cycle_id` context key threaded through `reconcileAll → reconcileOne → dispatchDeploy/dispatchDestroy`. |
| Evidence | `pkg/reconciler/cloud.go`, `pkg/reconciler/dispatch.go` — every `log.Printf` includes `cycle=`. |
| Failure mode | A failing target's logs across components cannot be joined into one timeline. |
| Contract anchor | (Forensic; see `phase2.7/structured-logging-proposal`.) |

---

## G-14: Truthful "Unknown" Handling

| Field | Value |
|---|---|
| Guarantee | When the provider returns no status condition, AutoStack does NOT persist a synthesized status. It logs `[STATUS_UNKNOWN]` and waits for the next cycle. |
| Scope | `GetStatus` returning a `*TargetStatus` with empty `Status` field. |
| Mechanism | Early return in `updateTargetStatus` when `status == ""`. |
| Evidence | `pkg/reconciler/cloud.go` `updateTargetStatus`. |
| Failure mode | A target is persistently `running` when the provider has gone silent. |
| Contract anchor | DC-7.1. |

---

## G-15: Destroy Intent Re-Arm

| Field | Value |
|---|---|
| Guarantee | An operator setting `endDate` on a `running` OR `error` target with `pending_destroy=true` auto-promotes to `deleting` within one reconcile cycle. |
| Scope | Targets whose rollout has a non-empty `endDate`. |
| Mechanism | Phase 2.9 AW-C1 fix — `previousStatus` matches `error` OR `running` in `reconcileOne` H-1 block. |
| Evidence | `pkg/reconciler/cloud.go:542`. |
| Failure mode | Operator sets `endDate` on a `running` target; target stays `running` indefinitely. |
| Contract anchor | DC-7.2 (false-ambiguity, fixed); [[phase2.9/mandatory-fixes-implementation]] AW-C1. |

---

## G-16: Heartbeat Survives Provider Return

| Field | Value |
|---|---|
| Guarantee | The heartbeat goroutine for a dispatched operation persists until `completeOperation` transitions the op terminal, including across the `confirmDeleted` poll loop. |
| Scope | Both deploy and destroy dispatch paths. |
| Mechanism | Phase 2.9 AW-C3 fix — heartbeat scoped to outer `ctx`, not the inner `deployCtx`/`destroyCtx`. |
| Evidence | `pkg/reconciler/dispatch.go:203` (deploy), `pkg/reconciler/dispatch.go:363` (destroy). |
| Failure mode | A process crash during `confirmDeleted` allows the sweep to reclaim a still-alive op. |
| Contract anchor | DC-2.1; [[phase2.9/mandatory-fixes-implementation]] AW-C3. |

---

## G-17: Panic Cleanup

| Field | Value |
|---|---|
| Guarantee | A panic anywhere inside `dispatchDeploy` or `dispatchDestroy` results in BOTH `completeOperation(failed)` AND `releaseTarget(..., error, "dispatcher panic")` being called before stack unwind. |
| Scope | All dispatcher panic paths. |
| Mechanism | `defer func() { if rec := recover(); ... }()` block at the top of each dispatcher. |
| Evidence | `pkg/reconciler/dispatch.go` panic defers in both dispatchers; verified in [[phase2.9/mandatory-fixes-implementation]] AW-C2. |
| Failure mode | Target left `creating`/`deleting` with a terminal op and no path forward. |
| Contract anchor | F-7. |

---

## G-18: Error Hygiene in Logs and History

| Field | Value |
|---|---|
| Guarantee | Sanitized error messages — credential keywords blocklisted, long messages truncated — appear in logs and `deployment_history.message`. |
| Scope | All provider error emissions. |
| Mechanism | `sanitizeError` called on every error message before persistence. |
| Evidence | `pkg/reconciler/cloud.go` `sanitizeError`. |
| Failure mode | Cloud credentials, tokens, or secrets visible in `deployment_history` or logs. |
| Contract anchor | F-8; CLAUDE.md security mandate. |

---

## G-19: Refusal of Multi-Pod Use

| Field | Value |
|---|---|
| Guarantee | The operator-facing documentation refuses to support multi-pod PocketBase reconcilers. The runtime does not detect this; the boundary is documentary. |
| Scope | Operational deployment posture. |
| Mechanism | Documented in [[phase2.9/safe-operational-boundaries]] §Single-Pod; deferred to Phase 3 SC-5 (`owned_by_pod`). |
| Evidence | [[phase2.9/safe-operational-boundaries]]; [[phase2.9/deferred-Phase3-concerns]] SC-5. |
| Failure mode | Two pods run the reconciler; the runtime sweep at 5-min threshold reclaims a live peer-pod op. |
| Contract anchor | U-1 (UNSAFE). |

---

## Summary by Category

| Category | Guarantees |
|---|---|
| Concurrency / claim correctness | G-1, G-2, G-4 |
| Replay / sweep honesty | G-3, G-4, G-16, G-17 |
| Truthful state | G-5, G-6, G-8, G-11, G-14, G-15 |
| Bounded retries | G-7, G-15 |
| Idempotency | G-9, G-10 |
| Lineage / forensics | G-12, G-13 |
| Hygiene / security | G-18 |
| Operational boundaries | G-19 |

---

## What This Matrix Does NOT Cover

These are **explicitly outside** the Phase 2 guarantee envelope. They are
documented elsewhere; they are not promises:

- Drift detection (Phase 3 — [[phase2.9/deferred-Phase3-concerns]] DO-2)
- Real rollback (Phase 3 — PR-4)
- Live cost pricing (Phase 3 — PR-6)
- Live quota checks (Phase 3 — PR-7)
- LRO operation tracking (Phase 3 — PR-5)
- Spec-vs-actual drift (Phase 3 — PR-1 + SC-1)
- Multi-pod safety (Phase 3 — SC-5)
- Per-target backoff (Phase 3 — RA-2)
- Persistent circuit state (Phase 3 — RA-3)
- Structured logging (Phase 3 — DO-1)
- Metrics export (Phase 3 — DO-3)
- Orphan cleanup scanner (Phase 3 — OC-1)

---

## Related

- [[phase2.9/lifecycle-contracts]] — DC-1 through DC-8 contract clauses
- [[phase2.9/provider-contracts]] — P-1 through P-15 provider clauses
- [[phase2.9/reconciliation-architecture-freeze]] — F-1 through F-9 architectural invariants
- [[phase2.9/safe-operational-boundaries]] — operational envelope
- [[phase2.9/mandatory-fixes-implementation]] — AW-C1/C2/C3 fix records
- [[phase2.9/trustworthiness-verdict]] — overall Phase 2 verdict
