# Phase 2 Finalization — Safe Operational Boundaries

**Last Updated:** 2026-05-14

## Intent

Defines the operational envelope within which the system behaves
deterministically and honestly, and explicitly states what lies outside
that envelope.

---

## Single-Pod Envelope

**This is the only supported configuration for Phase 2.**

The reconciler is a single-threaded, in-process loop. SQLite WAL-mode
serializes writes. Multi-pod PocketBase is not supported for cloud
reconciliation until Phase 3 (requires pod-identity stamping + leader
election or row-versioning).

**Explicit boundary:**
```
SAFE: 1 PocketBase pod + 0 or N Kubernetes pods
SAFE: 1 PocketBase pod + 1 Cloud Run reconciler
NOT SAFE: 2+ PocketBase pods (same PocketBase binary, no HA layer)
```

---

## Concurrency Envelope

**Per-cycle:** Single goroutine processes all rows sequentially. No parallel dispatch within a cycle.

**Per-target:** At most one in-flight operation per target enforced by CAS claim. If two reconciliation ticks fire simultaneously, `claimTarget` serializes at the DB level — one wins, one cancels its op.

**Multi-target:** Sequential processing. A slow `GetStatus` on target 1 delays target 2 through N for that cycle only.

**Operational limit:** With 30s tick interval and 100 targets × 500ms poll latency, cycle duration = 50s > 30s tick. With 5 targets × 500ms = 2.5s. **Recommended limit for Phase 2: ≤ 20 cloud targets per reconciler instance.**

---

## State Assumptions Envelope

**Assumption 1:** The operator does not manually edit PocketBase rows for active cloud targets. If they do, no guarantees apply. (Documented in [[operation-ownership]].)

**Assumption 2:** `cloud_account.region` does not change on active accounts. Changes cause orphaned resources (see [[phase2.9/drift-survivability-assessment]] D-6).

**Assumption 3:** Cloud Run service is not manually mutated outside AutoStack. If it is, AutoStack will overwrite it on next deploy with no drift warning (Phase 3 feature). Compensation: IAM policy restricts non-AutoStack service edits.

**Assumption 4:** No two operators target the same `rollout` to different cloud accounts simultaneously. The CAS dispatch is per-target, not per-rollout, so concurrent rollouts to the same target would race the CAS.

---

## Lifecycle Completeness Envelope

**Deploy lifecycle:** Complete for happy path and error path. Replay after crash safe via sweep. Panic recovery ensures target is never stranded with in-flight op and no status.

**Destroy lifecycle:** Complete for happy path. `confirmDeleted` ensures NOT_FOUND confirmation. Crash gap exists for confirm loop (Phase 2.9 fix identified).

**Rollback lifecycle:** Not implemented. `Provider.Rollback` returns `ErrNotImplemented`. Attempting rollback has no effect. Phase 3.

**Orphan cleanup:** Not implemented. A target whose service was deleted externally via another tool stays `error` forever unless operator clears `current_operation`. Phase 3.

---

## Observability Envelope

**Logs:** All significant events tagged. Free-text `log.Printf` format. No structured JSON. Aggregation tools parse tag prefix + key=value tail.

**History:** Immutable append-only. Every terminal dispatch event written. Sweep events written. No TTL.

**Metrics:** None exported to Prometheus/DataDog. Not a Phase 2 deliverable.

**Real-time:** No WebSocket state push to frontend for cloud targets. This limits live dashboard updates to manual refresh.

---

## Known Failure Modes with Documented Workarounds

| Failure Mode | Symptom | Workaround |
|---|---|---|
| `running` + `endDate` set | Target stays `running`; destroy never fires | Clear target status manually to `deleting`, or restart reconciler (Phase 2.9 fix) |
| `creating` + terminal op (crash gap) | Target stuck `creating` | Operator clears `current_operation` to `''` in PocketBase admin |
| `deleting` + confirm loop crash | Target stuck `deleting` | Check GCP console; if service gone, manually set status to `deleted` |
| Respec-flapping rollout (3+ cycles) | Target → `error` + "pathological stale loop" | Respec rollout (new revision clears stale count); investigate why |
| Region change mid-account | Old-region service persists, orphaned | Create new cloud_account; don't reuse old one |
| Auth error on target | Circuit opens; no retries | Fix credentials; operator clears `current_operation` to re-enable dispatch |

---

## Operational Checkpoints

**Pre-production launch:**
1. Verify all cloud targets use the same PocketBase pod (no multi-pod HA without Phase 3 work).
2. Set up `cloud_account.region` change freeze — changes mid-life create orphaned services.
3. Configure IAM on Cloud Run to restrict service edits to AutoStack service account + break-glass users.
4. Train operators on `deployment_history` reconstruction for incident analysis.
5. Document the compensations for UA-1, UA-2, and drift invisibility for operators.

**Ongoing operational rules:**
1. Do not manually edit `deployment_targets` or `operations` rows for active cloud targets.
2. Do not change `cloud_account.region` on active accounts.
3. Restart PocketBase only during low-traffic windows (in-memory circuit state resets, all targets will retry).
4. Monitor `[OP_ABANDONED]`, `[DESTROY_CONFIRM_TIMEOUT]`, `[RELEASE_LOST_OWNERSHIP]` in logs.
5. All targets stuck in non-terminal state > 15 minutes = operator intervention required.

---

## Related
- [[phase2.9/production-readiness-gate]] — full readiness gate
- [[phase2.9/operational-ambiguity-inventory]] — UA-1, UA-2 details
- [[phase2.9/lifecycle-closure-assessment]] — closure gap detail
- [[phase2.8/deferred-followups]] — Phase 3 scope