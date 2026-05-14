# Phase 2 Finalization — Remaining Architectural Weaknesses

**Last Updated:** 2026-05-14

## Severity: Critical (Phase 2.9 fix required)

### AW-C1: `running + pending_destroy` destroy intent not consumed

**Location:** `cloud.go` `reconcileOne` — only `previousStatus == "error"` triggers auto-promote to `deleting`. `running` is not handled.

**Impact:** Operator sets `endDate` on a `running` target — destroy intent silently ignored. Target stays `running`. Next deploy would route to `deleting` (pending_destroy) but the target is already deployed.

**Fix:** 5-line addition in reconcileOne's H-1 block:
```go
if previousStatus == "error" || previousStatus == "running" {
    r.promoteToDeleting(targetID)
    return reconcileSkipped
}
```

**Effort:** Trivial. Risk: Low. Must land in Phase 2.9.

---

### AW-C2: `releaseTarget` panic leaves target with terminal op + non-terminal status

**Location:** `dispatch.go` `dispatchDeploy` — panic recovery calls `releaseTarget` (line 187), but if the panic occurs after `completeOperation` but before `releaseTargetWithExternal` at line 293, the target is left `creating` with a terminal operation.

**Impact:** Target stuck `creating`. No path to re-dispatch (dispatch checks `current_operation != ''`). Operator intervention required to clear `current_operation`.

**Fix:** Refactor panic defer to call `releaseTargetWithExternal` specifically (it has a CAS guard so it won't overwrite sweep's state). Or make `completeOperation` + `releaseTarget` one atomic call.

**Effort:** Small. Risk: Low. Phase 2.9.

---

### AW-C3: `confirmDeleted` crash leaves `deleting` target unreachable

**Location:** `provider.go` `Destroy` — confirm loop is co-routine-scoped; process crash during confirm loses heartbeat.

**Impact:** Sweep marks op `failed`. Target is `deleting` (from release before crash). Next cycle: `shouldDispatchDestroy(false)` (status=`deleting`, but current_operation=`failed_op_id`). No dispatch, no poll. **Target stuck `deleting` forever.**

**Fix:** Spawn confirmDeleted as a detached goroutine tied to the operation row's heartbeat rather than the dispatcher's ctx + goroutine. This requires extending heartbeat scope past the Destroy() call completion.

**Effort:** Medium. Risk: Medium. Phase 2.9.

---

## Severity: Significant (Phase 3 material)

### AW-S1: Global `lastErrorTime` backoff — cascade delay

**Location:** `cloud.go` `reconcileWithBackoff` — one failing target delays all targets' next cycle.

**Impact:** With 20 targets and one transient auth error, 19 healthy targets experience global 30s backoff until the next cycle. Not a correctness failure, but a performance degradation.

**Fix:** Per-target backoff state; circuit breaker already handles skip, but we still run the cycle (and call `GetStatus` on open circuits). Phase 3.

---

### AW-S2: In-memory circuit state resets on restart

**Location:** `cloud.go` `Reconciler.failures` — in-memory, process-private.

**Impact:** After restart, all circuits are closed. A target with persistent transient failures (e.g., quota exhaustion during a GCP incident) will immediately retry after restart, potentially burning quota while the upstream issue persists.

**Fix:** Persist per-target failure count to `deployment_targets` or a dedicated `circuit_state` table. Phase 3.

---

### AW-S3: No structured logger

**Location:** All reconciler and provider code uses `log.Printf`.

**Impact:** Log aggregation tools cannot query structured fields. Adding new structured fields requires format convention changes. Limits observability tooling quality.

**Fix:** `log/slog` adoption (deferred in [[phase2.8/deferred-followups]]). Phase 3.

---

### AW-S4: No per-revision lineage in history

**Location:** `writeHistory` doesn't accept or store a revision field (beyond `rollout_revision` on operations).

**Impact:** In a `succeeded_stale` event, the history row says "stale spec" but does not explicitly record which revision was actually deployed. Cross-referenceable via `operations.rollout_revision` but non-obvious.

**Fix:** Add `revision` column to `deployment_history`. Phase 3 (migration required).

---

### AW-S5: No operational metrics export

**Location:** Observability layer.

**Impact:** No Prometheus/DataDog/OpenTelemetry metrics for cycle duration, success/failure rates, dispatch latency. Limits SLO monitoring and alerting.

**Fix:** Add structured metrics emitter; wire to OpenTelemetry. Phase 3.

---

## Severity: Accepted Limitation (documented, no fix in Phase 2)

| Limitation | Impact | Compensation |
|---|---|---|
| Multi-pod unsafe | Two PocketBase pods race writes | Single-pod enforced; Phase 3 HA |
| Manual cloud mutation invisible | Drift not detected | IAM restrictions; console audit |
| Drift detection not implemented | Spec-vs-actual not compared | Documented in [[phase2.8/manual-cloud-mutation-policy.md]] |
| Rollback not implemented | Cloud Run rollback uses incorrect mechanism | Refused with ErrNotImplemented; Phase 3 correct implementation |
| Region change orphans service | Old-region service persists after account region change | Create new account; freeze region after creation |
| No orphan scan | Deleted external service leaves orphaned target row | Manual cleanup via PocketBase admin |
| Worker pool not implemented | Sequential dispatch; scale limited | Phase 3 worker pool work |
| Cloud Run LRO not tracked | GetOperation returns ErrNotImplemented | Phase 3 LRO support |
| Cost estimate uses static pricing | Estimate not live API | ADR-010; GCP Billing API; Phase 3 |

---

## Verdict

**AW-C1, AW-C2, AW-C3 are Phase 2.9 mandatory fixes** — they are correctness gaps that produce stuck states, not just quality or scalability issues.

**AW-S1 through AW-S5 are Phase 3 items** — important, but the system can operate correctly without them.

The criticality of AW-C1 and AW-C2 is low-effort to fix. AW-C3 is the most involved (requires heartbeat goroutine scoping change), but is bounded.

---

## Related
- [[phase2.9/reconciliation-convergence-assessment]] — AW-C1 detail
- [[phase2.9/lifecycle-closure-assessment]] — AW-C2, AW-C3 detail
- [[phase2.8/deferred-followups]] — full Phase 3 backlog
- [[phase2.9/production-readiness-gate]] — readiness gate using this inventory