# Phase 2 Finalization — Deferred Phase 3 Concerns

**Last Updated:** 2026-05-14

## Purpose

Documents the full Phase 3 backlog identified during the Phase 2 finalization review. Organized by type so the Phase 3 owner understands the scope, rationale, and urgency of each item.

---

## Schema Changes (require migrations)

### SC-1: `deployed_spec` snapshot column

**What:** Add `deployment_targets.deployed_spec TEXT` or `operations.deployed_spec TEXT`. Store the canonical YAML/JSON at deploy success, before returning.

**Why:** Phase 3 drift detection requires a baseline to diff against. Without this, we cannot detect manual cloud mutations. The current "last AutoStack deploy" is the YAML the operator may have edited.

**Complexity:** Medium. Migration adds column. Provider.Deploy captures spec at success time (before release). Cloud Run provider needs a `DumpServiceSpec()` method.

**Blocked by:** None — can be done in Phase 3.

---

### SC-2: `operations.cycle_id` column

**What:** Migration to add `operations.cycle_id TEXT` column. Wire from `__cycle_id` at claim time. Remove `cycle_id` from `__cycle_id` context threading.

**Why:** Phase 3 worker pool requires durable per-operation identity so any worker can resume any op. The in-memory `__cycle_id` context key doesn't survive process boundaries.

**Complexity:** Small. Migration adds nullable string column; update `createOperation` call site.

**Blocked by:** None.

---

### SC-3: `deployment_history.target` FK with `on delete set null` / migration

**What:** Phase 2.8 deferred-followups item: FK to `deployment_history.target` should be examined. Cascade behavior for target deletion needs review in Phase 3.

**Why:** If a target is deleted, `deployment_history.target` should probably be preserved (set null or keep FK). Orphan-cleanup scanner (SC-8) will need to handle targets that have history but no provider object.

**Complexity:** Small. FK constraint review; potentially a migration.

**Blocked by:** Orphan cleanup scanner (SC-8).

---

### SC-4: `deployment_history.status` add `stale` enum value

**What:** Phase 2.8 deferred-followups item. Add `stale` to the `deployment_history.status` enum to distinguish "deploy succeeded but spec moved" from "deploy failed."

**Why:** Currently `succeeded_stale` op status is written to `operations.status`, not `deployment_history.status`. A history row with `status=failed` and `message=stale spec` is ambiguous. Adding `status=stale` makes history queries unambiguous.

**Complexity:** Small. Migration to add enum value. Update `writeHistory` call in dispatcher's stale branch.

**Blocked by:** None.

---

### SC-5: `owned_by_pod` / pod-identity stamping on operations

**What:** Phase 2.7 deferred; Phase 2.8 deferred-followups. Add `operations.owned_by_pod TEXT` column. Set at claim time via pod hostname or `HOSTNAME` env. Runtime sweep AND startup sweep must verify `owned_by_pod` before reclaiming.

**Why:** Multi-pod PocketBase support. Without this, two pods' reconcilers cannot distinguish a live op owned by peer pod A from an abandoned op in peer pod B. The heartbeat liveness window is insufficient for multi-pod safety.

**Complexity:** Medium. Schema + all sweep queries filter on `owned_by_pod`. Requires `HOSTNAME` env var to be set consistently in Kubernetes/infra.

**Blocked by:** None for the schema. Multi-pod deployment requires this before safe.

---

## Provider Work

### PR-1: Cloud Run `serving_revision` field

**What:** Extend `TargetStatus` to include `ServingRevision string`. GetService's `traffic` field contains the actual revision serving traffic (not just Ready=SUCCEEDED which means "some revision is ready"). Cloud Run's API exposes `Service.Traffic[]` with `LatestCreatedRevision`, `LatestReadyRevision`, and per-traffic-target percents.

**Why:** Phase 2 gap: a new revision can be Ready=SUCCEEDED while old revision serves 100% of traffic (Cloud Run promotes on next deploy). AutoStack reports `running` with no information about which revision is serving. Rollback depends on this lineage.

**Complexity:** Medium. Provider extension; structured diff for drift detection would also read these fields.

**Blocked by:** SC-1 (deployed_spec) — drift diff compares spec vs actual.

---

### PR-2: Cloud Run `CreateService` vs `UpdateService` race handling

**What:** Cloud Run's `UpdateService` is a PATCH, not a PUT. If two updates race (two AutoStack instances or operator concurrent edit), the last-write-wins. Determine whether GCP's `etag` concurrency control should be used.

**Why:** Phase 2.8 deferred-followups: "Cloud Run create-vs-update transient retry." A retry of `UpdateService` during a transient error could overwrite a concurrent operator change.

**Complexity:** Medium. Requires etag handling + retry-with-etag-check logic.

---

### PR-3: Region-scoped credential validation

**What:** `ValidateCredentials` currently uses `projects/-/locations/-` (all regions). Validate that the credentials + project can actually access the specific `cloud_account.region` before starting deployments.

**Why:** Phase 2.8 deferred-followups. A service account with region-restricted IAM roles would silently fail on deployments to the restricted region. Better to fail fast at credential validation.

**Complexity:** Small. Add `TestRegion(parent, credentials)` path in ValidateCredentials that runs `ListServices` scoped to `projects/P/locations/REGION`.

---

### PR-4: Real Cloud Run rollback via `Service.Traffic`

**What:** Implement `Provider.Rollback` correctly using `Service.Traffic` with a `TrafficTarget` pinning Percent: 100 to a known prior revision name. Requires persisting deployment_targets lineage: `previous_revision`, `current_revision` before the rollback attempt.

**Why:** The current Rollback stub refuses with ErrNotImplemented because the previous implementation was unsafe (posted empty service, picked wrong revision). Correct rollback requires revision history that Phase 2 doesn't store.

**Complexity:** High. Requires revisions tracking, traffic targeting, and the schema work first.

---

### PR-5: `GetOperation` for LRO tracking

**What:** Return the actual operation name from Deploy/Destroy and poll `OperationsAPI` to track in-flight long-running operations.

**Why:** Phase 2 notes that the previous GetOperation was "inferred state masquerading as LRO tracking." A correct implementation returns the operation name from the CreateService/CreateWorkload Egress API call and polls the operations endpoint.

**Complexity:** Medium. Requires Cloud Logging / Cloud Operations API access.

---

### PR-6: Live Cloud Billing API for cost estimates

**What:** `EstimateCost` currently uses static 2024 pricing. Phase 3 must call the GCP Cloud Billing API (`CloudBillingClient`) for actual regional rates.

**Why:** ADR-010 and CLAUDE.md forbid hardcoded pricing. Estimates must be live API calls per region.

**Complexity:** Medium. Requires adding Cloud Billing API scope to credentials.

---

### PR-7: `CheckQuotas` implementation

**What:** Actually query `Cloud Quotas API` (or `Service Usage API`) to check project CPUs, memory, and Cloud Run-specific quotas before a deploy.

**Why:** The previous `CheckQuotas` stub returned Available: true regardless of input, which is a lie that would cause deploy failures right after UI says "deployable."

**Complexity:** Medium. Requires quota API integration.

---

## Reconciliation Architecture

### RA-1: Worker pool for dispatch

**What:** Change from single-threaded sequential dispatch to a worker pool (e.g., `N=4` workers). Work is queued per target; workers claim via CAS.

**Why:** Phase 2.8 deferred-followups: Sequential dispatch limits throughput. With 100 targets and 30s tick, a slow `GetStatus` on target 1 delays all 99 others.

**Prerequisites:** SC-2 (cycle_id), SC-5 (owned_by_pod), operation persistence (so a worker that crashes mid-deploy has its op visible to any worker that resumes).

**Complexity:** High. Requires careful leader election or work-stealing design.

---

### RA-2: Per-target backoff

**What:** Replace the global `lastErrorTime` backoff with per-target backoff. Each target independently applies its own backoff after failure, tracked per-target in DB or in-memory with persist-on-db-write.

**Why:** One failing target delays all 50 healthy targets today. Phase 2.8 deferred-followups.

**Complexity:** Small. Phase 3 work (doesn't affect correctness, only performance).

---

### RA-3: Per-target circuit persistence

**What:** Store the circuit breaker's per-target failure count in a new table `circuit_state(target_id, failures, last_failure_at)` so it survives process restarts.

**Why:** After restart, all circuits are closed. A persistent transient failure target immediately retries despite an ongoing upstream issue.

**Complexity:** Small. New table + update `recordTargetFailureWithCategory` to write to table.

---

## Drift and Observability

### DO-1: Structured logging (log/slog)

**What:** Replace all `log.Printf` with `slog` structured JSON. Field keys: `target_id`, `operation_id`, `cycle_id`, `status`, `duration_ms`, `category`, etc.

**Why:** Phase 2.8 deferred-followups. `log/slog` adoption is Phase 2.9 decision per deferred-followups. Current free-text tag parsing limits log aggregation tool quality.

**Effort:** Medium. All ~30 call sites need conversion. Not risky — purely additive logging refactor.

---

### DO-2: Drift detection cycle

**What:** Every N minutes, for every `running` target: call `GetService`, structurally diff against `deployed_spec`, set `deployment_targets.drift_detected=true` + `drift_summary`.

**Requires:** SC-1 (deployed_spec snapshot), PR-1 (serving_revision field so diff knows actual vs. spec), diff library.

**Complexity:** High. Full diff library; semantic comparison (image tag, env vars, scale, resources — ignore timestamps, generated fields).

---

### DO-3: Metrics export (Prometheus / OpenTelemetry)

**What:** Export cycle duration, dispatch latency, success/failure rates, circuit states, queue depth as Prometheus metrics or OpenTelemetry metrics.

**Why:** No SLO monitoring without this. Phase 2 gap — operators have no quantitative visibility.

**Complexity:** Small. Add a `/metrics` endpoint and wire the reconciler/ dispatcher to emit counters/gauges.

---

## Orphan Cleanup

### OC-1: Orphan cleanup scanner

**What:** Background goroutine that periodically scans `deployment_targets` and verifies each target's external_id is still present in the provider. If NOT_FOUND: mark `error` or `deleted` (depending on intent).

**Why:** A target whose service was deleted externally (another tool, another AutoStack bug) stays in whatever status it was. No self-healing path. Phase 2.8 deferred-followups.

**Requires:** Per-provider scan is fast (ListServices with filter). Add to sweep loop.

**Complexity:** Small. Can be a pass in the existing reconciler tick.

---

## Summary

| Category | Count | Top Items by Effort |
|---|---|---|
| Schema | 5 | SC-1 (deployed_spec), SC-5 (pod stamping) |
| Provider | 7 | PR-4 (rollback), PR-1 (serving_revision) |
| Reconciliation | 3 | RA-1 (worker pool) |
| Observability | 3 | DO-2 (drift detection), DO-1 (slog) |
| Cleanup | 1 | OC-1 (orphan scan) |
| **Total** | **19** | |

---

## Related
- [[phase2.8/deferred-followups]] — this supersedes and expands the Phase 2.8 deferred list
- [[phase2.9/architectural-weaknesses]] — AW-S1 through AW-S5 are within Phase 3 scope
- [[phase2.9/safe-operational-boundaries]] — Phase 2 operational limits