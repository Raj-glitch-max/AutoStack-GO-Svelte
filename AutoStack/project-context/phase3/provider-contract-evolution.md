# Provider Contract Evolution

**Last Updated:** 2026-05-14
**Phase:** 3.0 (Foundations)

## Purpose

Define exactly **how the Provider interface evolves** in Phase 3.
This document is the authoritative source for what changes, what stays,
and what is forbidden to change.

It supersedes nothing in [[../phase2.9/provider-contracts]]: P-1..P-15
remain the contract baseline. Phase 3 **extends** the contract; it does
not weaken any clause.

## The Three Forms of Evolution

### Form A — Additive Method

A new method is added to the `Provider` interface. **Existing providers
must implement it** (returning `ErrNotImplemented` is acceptable;
returning fabricated success is not).

### Form B — Additive Return-Field

An existing method's return struct gains a new field. **Existing callers
ignore the field** until they're updated. Providers that cannot supply
the field set it to its zero value with a `Note` in the capability
matrix.

### Form C — Capability-Aware Branch

The reconciler queries `Provider.Capabilities()` and selects a code
path based on the semantic string. No interface change; behavior
change is in the reconciler routing logic.

**Forms not permitted in Phase 3:**

- **Form X — Method signature change.** Breaking any existing method
  signature. Use Form B if a new field is needed.
- **Form Y — Method removal.** Hard-deferred to Phase 4 at earliest.
- **Form Z — Method semantic change without capability annotation.**
  If a method's behavior changes for any provider, the matrix
  semantic MUST change in the same PR.

## Phase 3 Provider Interface Changes

### Change 1 — Add `Capabilities()` (Form A)

```go
// New method, required for all providers.
type Provider interface {
    // ... existing 12 methods ...

    // Capabilities returns a snapshot of this provider's capability profile.
    // The result is read-mostly: callers may cache the result for the
    // lifetime of the process. Providers MUST return a complete set —
    // every defined Capability key must be present (with Supported=false
    // and explanatory Notes if not implemented).
    Capabilities() CapabilitySet
}

type CapabilityKey string

const (
    CapDeploy                CapabilityKey = "C-Deploy"
    CapDestroy               CapabilityKey = "C-Destroy"
    CapGetStatus             CapabilityKey = "C-GetStatus"
    CapRollback              CapabilityKey = "C-Rollback"
    CapTrafficSplit          CapabilityKey = "C-TrafficSplit"
    CapRevisionHistory       CapabilityKey = "C-RevisionHistory"
    CapCanaryRollout         CapabilityKey = "C-CanaryRollout"
    CapHealthReporting       CapabilityKey = "C-HealthReporting"
    CapDeploymentCancel      CapabilityKey = "C-DeploymentCancel"
    CapRevisionCleanup       CapabilityKey = "C-RevisionCleanup"
    CapDriftVisibility       CapabilityKey = "C-DriftVisibility"
    CapScalingIntrospection  CapabilityKey = "C-ScalingIntrospection"
    CapLogStreaming          CapabilityKey = "C-LogStreaming"
    CapMetricsVisibility     CapabilityKey = "C-MetricsVisibility"
    CapDestroyConfirmation   CapabilityKey = "C-DestroyConfirmation"
    CapIdempotentDeploy      CapabilityKey = "C-IdempotentDeploy"
    CapOperationTracking     CapabilityKey = "C-OperationTracking"
    CapEventualConsistency   CapabilityKey = "C-EventualConsistency"
)

type Capability struct {
    Supported       bool
    Semantic        string        // e.g. "gradual-traffic-shift"
    Constraints     []string
    UncertaintyP99  time.Duration
    Notes           string
}

type CapabilitySet map[CapabilityKey]Capability
```

**Why required:** Without `Capabilities()`, the reconciler must
hard-code provider-specific behavior. R-4 (hidden provider coupling)
becomes inevitable.

**Migration:** Cloud Run gets a `capabilities.go` file that returns
the Phase 2 baseline profile (see
[[provider-capability-matrix]]). The reconciler's existing
hard-coded assumptions become explicit capability queries.

### Change 2 — Add `ClassifyError()` (Form A, optional via capability)

```go
type Provider interface {
    // ... Capabilities() and existing methods ...

    // ClassifyError categorizes a provider-emitted error for retry,
    // circuit-breaker, and alert purposes. Providers MAY implement
    // this to override the default classifier; absence means the
    // default classifier is used.
    ClassifyError(err error) FailureCategory
}
```

**Why:** Phase 2's `ClassifyError` lives in `pkg/reconciler` and grows
provider-specific cases. Moving it into the provider satisfies HC-6.
Optional because the default classifier remains correct for Cloud
Run; providers add overrides only where they have genuinely
provider-specific error shapes.

**Migration:** Phase 2's classifier remains as `DefaultClassifier()`
in `pkg/reconciler`. Provider implementations call it for common
cases and override only their unique cases.

### Change 3 — `TargetStatus.ServingRevision` (Form B)

```go
type TargetStatus struct {
    // ... existing fields ...

    // ServingRevision is the revision currently receiving traffic.
    // Empty string if the provider cannot report it (capability
    // C-HealthReporting must be false in that case).
    ServingRevision string
}
```

**Why:** Phase 2 declined to assume `Ready=SUCCEEDED` means traffic.
Phase 3 makes the distinction first-class. Required for honest rollback
and canary status.

**Cloud Run impl:** Read `Service.Traffic` array, find the entry with
`LatestRevision=true` or the highest percent.

**ECS/ACA impl:** Phase 3.1 work.

### Change 4 — `DeployResult.DeployedSpec` (Form B)

```go
type DeployResult struct {
    // ... existing fields ...

    // DeployedSpec is the canonical JSON the provider accepted. Used
    // for drift detection in Phase 3.2. Empty if the provider does
    // not support drift detection (capability C-DriftVisibility false).
    DeployedSpec string
}
```

**Why:** Drift detection (DO-2) requires a baseline. The deploy
result is the only correct capture point.

### Change 5 — Provider-Aware DestroyConfirm (Form C)

No interface change. The dispatcher (`dispatchDestroy` in
`pkg/reconciler/dispatch.go`) queries
`caps[C-DestroyConfirmation].Semantic` and selects:

```go
switch sem := caps[CapDestroyConfirmation].Semantic; sem {
case "not-found-poll":          // Cloud Run
    confirmNotFound(...)
case "status-deleted-poll":     // ECS
    confirmStatusDeleted(...)
case "tombstone-poll":          // ACA
    confirmTombstone(...)
case "":
    // No confirm — fail-closed; treat 200 as accepted but
    // surface DESTROY_CONFIRM_NOT_SUPPORTED in lineage.
}
```

The three confirm helpers live in `pkg/reconciler/confirm.go` (new
file). Each is provider-agnostic — they take a `Provider` and a
predicate function.

**Why:** R-5 mitigation. Cloud Run's confirm logic (Phase 2.8) is the
template; ECS and ACA get their own predicates.

### Change 6 — Per-Provider Suspicion Threshold (Form C)

The reconciler reads
`caps[CapEventualConsistency].UncertaintyP99` and computes a per-target
suspicion threshold:

```go
suspicionThreshold := max(2, int(caps[CapEventualConsistency].UncertaintyP99 / pollInterval) + 1)
```

Cloud Run's profile (5s lag, 30s poll → threshold 2) matches Phase 2
behavior. ECS's profile (30s lag) yields threshold 3. This is
**additive** to the Phase 2 suspicion counter (G-6); the counter
mechanism is unchanged.

## What Does NOT Change in Phase 3

The following Phase 2 contract clauses remain bit-identical:

- **P-1** Deploy idempotency on existing service.
- **P-2** Destroy idempotency on NOT_FOUND.
- **P-3** GetStatus must return non-nil status or non-nil error.
- **P-4** GetStatus NOT_FOUND error message convention.
- **P-5** ValidateCredentials semantics.
- **P-6** Context cancellation honored by all methods.
- **P-7** Deploy/Destroy upper-bound enforcement.
- **P-10** ErrNotImplemented over fake zero-values.
- **P-15** Destroy 200 != deletion confirmed.

The following Phase 2 declined-semantics may evolve in Phase 3:

- **P-11** Rollback was declined for all providers. Phase 3.3 unlocks
  it for providers whose `C-Rollback` capability is `Supported: true`
  with a defined semantic.
- **P-12** GetOperation was declined. Phase 3 evolves this via
  `C-OperationTracking` capability — providers that genuinely return
  LRO names may implement it; absence is honest.
- **P-13** Cloud Run Ready != Traffic remains true. Now surfaced via
  `ServingRevision` (Change 3).
- **P-14** GetStatus is not drift detection. Drift is a separate
  capability (`C-DriftVisibility`) and a separate reconciler pass
  (Phase 3.2).

## The Compile-Time Coupling Guard

Phase 3.4 introduces a `go vet` analyzer enforcing HC-6:

```
analyzer "providercoupling":
  For any file under pkg/reconciler/, pkg/controller/, or frontend/:
    - Reject imports of pkg/providers/<X>/ (any provider impl package).
    - Allow imports of pkg/providers (the interface package).
```

Until the analyzer ships, this rule is enforced via code review.

## The Contract Verification Test

Each provider implementation ships with:

```
pkg/providers/<provider>/contract_test.go:
  TestProviderContractCompliance(t *testing.T):
    For each capability declared Supported: true:
      - Invoke the corresponding method in a test scenario
      - Assert no ErrNotImplemented
      - Assert semantic matches declared profile
    For each capability declared Supported: false:
      - Invoke the method
      - Assert returns ErrNotImplemented (or documented equivalent)
```

This test is the **runtime defense** against capability lies.

## Phase 3.0 Closure Criteria

Phase 3.0 is closed when:

1. `Capabilities()` is added to the Provider interface in
   `pkg/providers/provider.go`.
2. `CapabilitySet`, `Capability`, and the `CapabilityKey` constants are
   defined.
3. Cloud Run's `capabilities.go` returns the documented Phase 2 baseline
   profile.
4. Build + vet remain clean.
5. The reconciler still passes existing behavior (no regression).
6. `contract_test.go` exists for Cloud Run and is green.

**No new providers added in Phase 3.0.** That is Phase 3.1.

## Related

- [[phase3/provider-capability-matrix]] — the semantic profiles
- [[phase3/provider-normalization-rules]] — how capabilities inform normalization
- [[phase3/lifecycle-normalization-model]] — capability-driven lifecycle
- [[phase3/multi-provider-risk-analysis]] — R-3, R-9 mitigation
- [[../phase2.9/provider-contracts]] — P-1..P-15 baseline
