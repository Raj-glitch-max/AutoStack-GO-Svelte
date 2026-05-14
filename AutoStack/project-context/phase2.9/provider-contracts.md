# Provider Contracts — Phase 2 Final

**Last Updated:** 2026-05-14

## Purpose

Defines EXACTLY what the Provider interface contracts require of any
implementation (currently: Cloud Run), what is optional/best-effort,
and what AutoStack explicitly refuses to assume.

This document prevents abstraction lies — a provider implementation that
violates a REQUIRED term is a correctness bug, not a feature difference.

---

## Interface Overview

```go
type Provider interface {
    Deploy(ctx, account, spec) (*DeployResult, error)
    Destroy(ctx, account, target) error
    GetStatus(ctx, account, target) (*TargetStatus, error)
    GetMetrics(ctx, account, target) (*TargetMetrics, error)
    GetOperation(ctx, account, opID) (*Operation, error)
    Rollback(ctx, account, target, revision) (*DeployResult, error)
    StreamLogs(ctx, account, target, writer) error
    EstimateCost(ctx, account, spec) (*CostEstimate, error)
    GetActualCost(ctx, account, target, start, end) (*ActualCost, error)
    ValidateCredentials(ctx, account) error
    ListRegions(ctx, account) ([]string, error)
    CheckQuotas(ctx, account, spec) (*QuotaCheck, error)
}
```

---

## REQUIRED Semantics (must hold or it is a bug)

### P-1: Deploy — Idempotent on Existing Service

**Requirement:** Calling `Deploy` with a RolloutID whose service already
exists in the provider MUST update the existing service, not fail with a
conflict, not create a duplicate.

**Why:** The `reconcileAll` → `shouldDispatchDeploy` dispatch path re-dispatches
on `pending` targets. If `Deploy` returned an error on existing services,
every re-dispatch of an existing target would fail.

**Cloud Run implementation:** `GetService` first; if service exists, call
`UpdateService`. If UpdateService is needed, proceed. This satisfies
idempotency — calling Deploy twice with the same rollout converges.

**Not guaranteed:** Whether UpdateService is a full spec rewrite or a
patch-merge. Cloud Run's `UpdateService` is partial. Future Phase 3 may
add etag support for optimistic concurrency.

### P-2: Destroy — Idempotent on NOT_FOUND

**Requirement:** `Destroy` MUST return `nil` (success) when the target
service does not exist in the provider. It MUST NOT return an error that
accumulates in the circuit breaker.

**Why:** The destroy path is dispatched when `status = deleting`. If the
service was already deleted externally, `Destroy` must not produce an error
that blocks reconciliation. Phase 2.8 `confirmDeleted` poll provides
truthful deletion confirmation, but idempotency on NOT_FOUND is independent
of the confirm loop.

**Cloud Run implementation:** Checks `GetService` first; if NOT_FOUND
returns `nil` immediately. After `DeleteService`, also triggers
`confirmDeleted` polling.

### P-3: GetStatus — Return Value When Service Exists

**Requirement:** When the target service EXISTS in the provider, `GetStatus`
MUST return a non-nil `*TargetStatus` with a meaningful `Status` field
(`creating`, `running`, `error`, etc.). It MUST NOT return `nil, nil`
(the zero value — no `error` but no `TargetStatus` either).

**Why:** `reconcileOne` branches on `err != nil` to detect provider failures.
If `GetStatus` returns `nil, nil`, the code would NPE on accessing
`status.Status`. The provider must always return a non-nil status or a
non-nil error.

### P-4: GetStatus — NOT_FOUND Semantics

**Requirement:** When the target service DOES NOT exist in the provider,
`GetStatus` MUST return an error. The error message SHOULD contain
"NOT_FOUND" or "not found" to allow the caller to classify it as a
permanent failure.

**Why:** `reconcileOne` uses `ClassifyError` to categorize errors. NOT_FOUND
is classified as `FailurePermanent` (no retry). The error message must be
parseable by `strings.Contains(err.Error(), "NOT_FOUND")` or equivalent.

### P-5: ValidateCredentials — Returns Only on Valid Credentials

**Requirement:** `ValidateCredentials` MUST return `nil` exactly when the
provided credentials are valid and authorized to perform operations in the
specified project. Return a non-nil error for any auth failure.

**Why:** Credential validation runs at account creation and at rotation.
If it returned `nil` for invalid credentials, the system could queue
deploys with bad credentials and waste API quota.

**Cloud Run implementation:** Calls `ListServices` with `projects/-/locations/-`.
Returns `nil` on success, error on any auth/API failure.

---

## REQUIRED Timeout and Cancellation Behavior

### P-6: All Methods Honor Context Cancellation

**Requirement:** Every Provider method MUST check `ctx.Done()` and return
early with `context.Canceled` or `context.DeadlineExceeded` when the
caller cancels the context. Long-running operations (Deploy, Destroy) MUST
respect the deadline even if the underlying API call is pending.

**Why:** The dispatcher's `deployCtx`/`destroyCtx` is derived from `ctx`
with a `DeployTimeout` deadline. If the operation exceeds the deadline,
the reconciler must reclaim the target and not wait indefinitely.

### P-7: Deploy/Destroy Have Upper Bounds

**Requirement:** `Deploy` and `Destroy` MUST NOT wait indefinitely. They
MUST return within a reasonable multiple of `DeployTimeout` (15 min) even
if the provider API is unresponsive.

**Why:** `deployCtx` is the reconciler's upper bound. If the provider
hangs indefinitely, the target would be stuck forever.

---

## OPTIONAL Semantics (best-effort; system degrades gracefully)

### P-8: CheckQuotas — Best-Effort Availability Check

**Current Status:** `ErrNotImplemented` for Cloud Run.

**What would be ideal:** Return `Available: true` only if the quota check
passes. `Available: false` with `Violations: [...]` if insufficient quota.

**Why best-effort:** Quota APIs vary by provider. Full implementation is
Phase 3. Today the absent check means "deployable" is an assumption.

### P-9: EstimateCost — Live API Preferred, Static OK

**Current Status:** Cloud Run uses static 2024 US region pricing.

**Required:** If using static pricing, the `PricingSource` field MUST
accurately describe the source (e.g., "gcp_cloud_run_pricing_2024"). The
estimate MUST include an `UncertaintyNote` field describing what is not
reflected.

**Why not required for Phase 2:** ADR-010 mandates live pricing API; static
pricing is explicitly flagged as a known limitation in the uncertainty note.

### P-10: GetMetrics — Not Implemented; Must Return ErrNotImplemented

**Current Status:** Cloud Run returns `ErrNotImplemented`.

**Required semantics:** Any provider that returns `nil, nil` (no error, no
metrics) would be misinterpreted as "0% CPU utilization" by the UI, which is
a lie. `ErrNotImplemented` is the honest signal meaning "this provider
cannot report metrics yet." Callers MUST check for `ErrNotImplemented` and
surface "unavailable" rather than rendering zero values.

---

## DECLINED Semantics (explicitly not assumed)

### P-11: Rollback Not Implemented

**AutoStack does NOT assume any provider can Rollback.** The `Rollback`
method returns `ErrNotImplemented` for Cloud Run. Any rollbacks attempted
during Phase 2 will return an error to the operator.

**Reason:** Previous implementation was unsafe (empty service posted, wrong
revision picked, outcome misreported). Correct rollback with Cloud Run
Traffic targeting requires revision lineage that Phase 2 doesn't persist.

### P-12: GetOperation Not Implemented (no real LRO tracking)

**AutoStack does NOT assume any provider returns a real operation ID from
Deploy that can be polled.** `GetOperation` returns `ErrNotImplemented`.
The dispatcher treats a `nil` Deploy error as "provider accepted the
request"; `waitForServiceReady` polls `GetService` directly rather than
polling an operation API.

### P-13: Cloud Run Readiness Is Not Traffic Readiness

**AutoStack does NOT assume `Ready=SUCCEEDED` means the revision is
receiving traffic.** Cloud Run can promote a new revision (Ready=SUCCEEDED)
while the old revision still serves 100% of traffic during a gradual
rollout. AutoStack's status reflects the provider's readiness condition,
not the traffic state.

**Mitigation for Phase 2:** `drift_detected=false` is acknowledged as
incomplete. The Phase 3 `serving_revision` field will distinguish
Ready=SUCCEEDED (revision ready) from it actually receiving traffic.

### P-14: GetStatus Is Not Drift Detection

**AutoStack intentionally does NOT compare the deployed spec against the
provider's actual state.** `GetStatus` only answers "what is the current
provider-reported state?" Not "does it match the rollout manifest?" Drift
detection is Phase 3.

### P-15: Destroy 200 ≠ Service Deleted

**AutoStack does NOT assume `DeleteService` returning 200 means the
service is gone.** Cloud Run's deletion is async. The Phase 2.8
`confirmDeleted` poll is the compensating control: `Destroy` does not
return success until NOT_FOUND is observed or the confirm window expires.

---

## Provider Capability Matrix (Phase 2, Cloud Run)

| Capability | Status | Notes |
|---|---|---|
| Deploy (create) | ✅ Required | Idempotent on existing |
| Deploy (update) | ✅ Required | Via UpdateService path |
| Destroy | ✅ Required | Idempotent on NOT_FOUND |
| Destroy confirm | ✅ Required | Phase 2.8 NOT_FOUND poll |
| GetStatus | ✅ Required | Condition-based mapping |
| ValidateCredentials | ✅ Required | ListServices probe |
| ListRegions | ✅ Required | Static list |
| EstimateCost | ✅ Required | Static (Phase 3: live API) |
| Rollback | ❌ Declined | ErrNotImplemented |
| GetOperation | ❌ Declined | ErrNotImplemented |
| GetMetrics | ❌ Declined | ErrNotImplemented |
| CheckQuotas | ❌ Declined | ErrNotImplemented |
| StreamLogs | ❌ Declined | ErrNotImplemented |
| GetActualCost | ❌ Declined | ErrNotImplemented |

---

## Phase 3 Provider Additions

The Phase 3 provider contract adds:
- `ServingRevision string` in `TargetStatus` (truthfulness)
- `CheckQuotas` live implementation (truthfulness)
- `GetMetrics` implementation (completeness)
- Cloud Billing API for `EstimateCost` (truthfulness)

These are additive to REQUIRED semantics. Existing REQUIRED semantics
(P-1 through P-7) do not change.

---

## Related
- [[phase2.9/lifecycle-contracts]]
- [[providers/cloudrun/provider.go]] — implemented contract
- [[phase2.8/deferred-followups]] — Phase 3 provider work