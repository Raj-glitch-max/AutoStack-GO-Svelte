# Phase 2 Finalization — Trustworthiness Verdict

**Last Updated:** 2026-05-14

---

## Summary

AutoStack **is trustworthy for the safe use cases defined in** [[phase2.9/safe-operational-boundaries]], **with three actionable Phase 2.9 fixes required** before the system can claim Phase 2 full closure.

The system does NOT require perfect enterprise-scale architecture. It requires deterministic, truthful, survivable operational behavior. This it delivers — within its designed envelope.

---

## The Three Blocking Fixes

**These must land in Phase 2.9 before Phase 2 can be closed.**

### Fix 1: `running + pending_destroy` auto-promote (AW-C1)

5 lines in `cloud.go reconcileOne`. Adds `previousStatus == "running"` to the H-1 auto-promote condition.

**Why blocking:** An operator who sets `endDate` on a `running` deployment expects the service to be deleted. With this gap, the destroy intent is silently lost. The operator sees `running` — the service stays up. This breaks the claim of truthful lifecycle state.

---

### Fix 2: Panic-recovery extended to call releaseTarget (AW-C2)

In `dispatchDeploy`'s panic defer block, ensure `releaseTarget` is called with the CAS guard so a panic between `completeOperation` and `releaseTargetWithExternal` does not leave the target `creating` with a terminal op.

**Why blocking:** A panic after the provider call completes but before `releaseTarget` leaves a target that cannot be re-dispatched and cannot be polled (both guarded by `current_operation != ''`). Operator must manually clear the field. This is a stuck state with no self-heal path.

---

### Fix 3: Heartbeat must span `confirmDeleted` loop (AW-C3)

The destroy confirm loop must either run in a goroutine scoped to the op's heartbeat, or be restructured so the operation stays "alive" in the sweep's view throughout the confirm window.

**Why blocking:** A process crash during confirm leaves a `deleting` target with a terminal operation. No path to re-dispatch (dispatch checks `status=deleting AND current_operation=''`). The operator must manually verify GCP console and clear the target.

---

## What Works Correctly Today

| Property | Evidence |
|---|---|
| CAS claim prevents double-dispatch | `claimTarget` + `WHERE current_operation = ''` + rows_affected check |
| Status transition guard prevents regressions | `isAllowedTransition` — confirmed by code audit |
| Sweep reclaims abandoned ops honestly | Startup sweep marks `failed`, never infers `succeeded` |
| Deployment lineage is complete | `writeHistory` called on every terminal branch + sweep + CAS race |
| Cycle correlation in logs | 8-char `cycle_id` threaded through all dispatch logs |
| Stale-spec loop guard | `staleCount` map at 3 cycles → `error` |
| Suspicion for transient Cloud Run flaps | 2 consecutive `error` from `updating` required before writing |
| Destroy idempotent on NOT_FOUND | Check in Destroy + confirmDeleted poll |
| Post-destroy confirm loop (Phase 2.8) | confirmDeleted polls until NOT_FOUND or 60s timeout |
| Auth/quota errors don't retry | `FailureAuth`, `FailureQuota` circuit-skip |
| Panic recovery in dispatcher | Deferred panic handler calls completeOperation + releaseTarget |
| Release-CAS prevents sweep overwrite | 0 rows affected → logged + writeOwnershipLostHistory |
| Heartbeat keeps ops live through dispatch | 60s tick with CAS guard on `status = 'in_progress'` |

---

## What Is Documented And Acceptable for Phase 2

These are not correctness failures — they are documented limitations with Phase 3 resolution:

| Limitation | Why acceptable today |
|---|---|
| Manual cloud mutation invisible | GCP IAM restricts writes to AutoStack SA; audit logs external |
| Multi-pod unsafe | Single-pod enforced; Phase 3 pod stamping is the path |
| Drift not detected | Phase 3 spec snapshot + diff is substantial feature |
| Rollback not implemented | ErrNotImplemented stub; Phase 3 correct implementation |
| Region change orphans service | Rare; create new account instead; documented workaround |
| No structured logger | Human-readable tags suffice for Phase 2 debugging |
| In-memory circuit resets on restart | Auth errors don't retry; transients: one extra cycle max |
| Sequential polling | ≤20 cloud targets is Phase 2 envelope |

---

## What Phase 2 Achieves

The system reliably:

1. **Claims a target exactly once** per dispatch via CAS.
2. **Emits honest lifecycle state** — every status reflects the last provider observation, with transition guards preventing regression.
3. **Writes complete lineage** — every dispatch attempt and every sweep abandonment is recorded in `deployment_history`.
4. **Survives its own crashes** — sweep reclaims in-flight ops honestly; panic recovery cleans up.
5. **Handles provider eventual consistency** — suspicion tolerance, `updating` persist before `running`, `confirmDeleted` for destroy.
6. **Refuses to lie** — `ErrNotImplemented` for Rollback; `unknown` not persisted; `succeeded_stale` distinguished from success; `[DESTROY_CONFIRM_TIMEOUT]` when the confirm window expires.
7. **Converges** — terminal paths all close ownership and write history. The non-convergent gaps (AW-C1 through C-3) are discrete and fixable without architectural change.
8. **Survives chaos** — timeout storms, transient flaps, heartbeat failure escalation, backoff, and circuit breaker all work as designed.
9. **Debugs from logs** — all 9 forensic reconstruction paths are complete using tagged logs + `deployment_history`.
10. **Is extensible** — adding a new provider requires implementing the Provider interface; no changes to reconciler, dispatch, or sweep.

---

## Final Score

```
R-1 Convergence:       9/10  (gap: running+pending_destroy)
R-2 Determinism:       8/10  (gaps: confirmDeleted crash, release panic)
R-3 Closure:           8/10  (same two gaps)
R-4 Ambiguity:         7/10  (UA-1, UA-2, no structured log)
R-5 Drift:             6/10  (manual mutation invisible; Phase 3 feature)
R-6 Chaos:             7/10  (two crash gaps + global backoff)
R-7 Observability:     8/10  (structured log missing; OG-1)
R-8 Forensics:         9/10  (FG-2 minor)
R-9 Maintainability:    7/10  (global backoff; circuit state reset)

Overall weighted:     7.7/10
With 3 Phase 2.9 fixes: ~9.0/10
```

---

## Closing Statement

AutoStack Phase 2 delivers a **truthful, survivable cloud deployment control plane** for single-pod operation with low-concurrency workloads. The system is operational and correct within its designed envelope. Three targeted Phase 2.9 fixes close the remaining structural gaps. The Phase 3 backlog is well-defined and does not require architectural redesign to execute.

**The system is ready for staging and internal production use.** Enterprise-scale multi-tenant workloads, HA distributed reconciliation, and compliance-grade drift detection are Phase 3 — that work is properly scoped and deferred.

Truthfulness over optimism.

---

## Related
- [[phase2.9/production-readiness-gate]] — full 9-dimension assessment
- [[phase2.9/safe-operational-boundaries]] — what is and isn't supported
- [[phase2.9/architectural-weaknesses]] — all gaps catalogued with fixes
- [[phase2.9/deferred-Phase3-concerns]] — 19-item Phase 3 backlog
- [[phase2.8/deferred-followups]] — Phase 2.8 closure document