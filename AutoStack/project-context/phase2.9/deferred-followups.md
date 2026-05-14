# Phase 2.9 Deferred Follow-ups

**Last Updated:** 2026-05-14

## Items REQUIRED before Phase 2 close

These are correctness gaps found during Phase 2 finalization. All three are
single-file changes; none require architectural work.

| # | Item | File | Change | Reason |
|---|---|---|---|---|
| D-1 | `running + pending_destroy` auto-promote | `pkg/reconciler/cloud.go` | Add `previousStatus == "running"` to H-1 condition in `reconcileOne` | Destroy intent silently ignored when target is `running` |
| D-2 | Panic recovery extended to releaseTarget | `pkg/reconciler/dispatch.go` | In defer block, call `releaseTargetWithExternal` with CAS guard after `completeOperation` | Panic after completeOperation but before release leaves target stuck `creating` |
| D-3 | `confirmDeleted` heartbeat scoping | `pkg/reconciler/dispatch.go` + `pkg/providers/cloudrun/provider.go` | Move confirm loop into a heartbeat-scoped goroutine, or restructure so heartbeat outlives the Destroy() call | Process crash during confirm loop leaves target stuck `deleting` |

## Items moved to Phase 3

| Original | Phase 2.9 Decision | Reason |
|---|---|---|
| Spec-vs-actual drift detection | Phase 3 | Requires deployed_spec column + diff library + UI |
| `serving_revision` field | Phase 3 | Provider extension; rollback depends on it |
| Cloud Run create-vs-update transient retry | Phase 3 | Requires etag handling |
| Region-scoped credential validation | Phase 3 | Provider work |
| Per-cloud-account backoff / circuit | Phase 3 | Schema + refactor |
| `operations.cycle_id` column | Phase 3 | Migration; worker pool prerequisite |
| `deployment_history.target` FK cascade review | Phase 3 | Migration |
| `deployment_history.status` enum add `stale` | Phase 3 | Migration |
| Worker pool for dispatch | Phase 3 | Requires operations persistence + pod stamping |
| `log/slog` adoption | Phase 3 | Refactor; no correctness impact |
| Pod-identity stamping on operations | Phase 3 | Multi-pod safety prerequisite |
| Orphan cleanup scanner | Phase 3 | Background sweep extension |
| Real Cloud Run rollback via Traffic | Phase 3 | Requires revision lineage; stub in place |
| Live GCP Billing API estimates | Phase 3 | ADR-010; requires billing API scope |
| CheckQuotas live implementation | Phase 3 | Stub was honest (returned error); Phase 3 implementation |
| GetOperation LRO tracking | Phase 3 | Return op name from CreateService; poll operations API |
| Drift detection cycle | Phase 3 | Requires SC-1 + PR-1 |

## What Phase 2.9 DOES land

The Phase 2 finalization review confirmed the following are already correctly implemented
and require no further work:

- Post-destroy NOT_FOUND confirmation poll (Phase 2.8, confirmed implemented)
- CAS claim + release-CAS for dispatch exclusivity (Phase 2.1)
- Heartbeat per-operation to prevent sweep false-positives (Phase 2.3)
- Panic recovery in dispatcher that calls completeOperation + releaseTarget (Phase 2.1)
- Succeeded-stale loop guard at threshold 3 (Phase 2.6)
- Suspicion counter for `updating` error tolerance (Phase 2.4)
- Transition guard preventing regressions (Phase 1.9+2.3)
- Auth/quota fast-circuit (no retries) (Phase 2.5)
- Destroy idempotent on NOT_FOUND (Phase 1.9)
- Deployment history completeness (all terminal paths + sweep + CAS race) (Phase 2.3)
- Cycle-ID threaded through all dispatch logs (Phase 2.3)
- `[RELEASE_LOST_OWNERSHIP]` forensic history when sweep claims mid-flight (Phase 2.7)
- Destroy intent re-arm on in-flight deploy (Phase 2.3)
- Error hygiene: sanitized messages, no credential leakage in logs (Phase 1.9+2.5)
- Cloud Run ValidateCredentials implementation (Phase 2.3)

## Phase 2.9 verification checklist

Before considering Phase 2 closed:

- [ ] D-1 fix: test `running` + `pending_destroy` + `endDate` set → auto-promotes to `deleting`
- [ ] D-2 fix: panic injected between completeOperation and releaseTarget → target released to `error`, not stuck
- [ ] D-3 fix: process killed during `confirmDeleted` → sweep doesn't reclaim the destroy op, or sweep and release handle the `deleting` + terminal op case

---

## Related
- [[phase2.9/reconciliation-convergence-assessment]] — D-1 root cause analysis
- [[phase2.9/lifecycle-closure-assessment]] — D-2, D-3 root cause analysis
- [[phase2.9/replay-determinism-assessment]] — confirmDeleted analysis
- [[phase2.9/trustworthiness-verdict]] — overall Phase 2 verdict