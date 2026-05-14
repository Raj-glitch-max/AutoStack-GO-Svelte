# Provider Capability Matrix

**Last Updated:** 2026-05-14
**Phase:** 3.0 (Foundations)

## Purpose

Define the **capability framework** that lets AutoStack truthfully
describe what each provider can and cannot do, **without normalizing
differences into lies**. This is the foundational defense against R-1
(abstraction lies) and R-5 (replay inconsistency).

## The Core Idea

A capability is **not a boolean**. It is a **semantic profile** with
fields that describe HOW the capability behaves, not just IF it
exists. A provider that says "I support rollback" without specifying
the rollback shape is lying by omission.

```go
// Phase 3 capability declaration shape (concept; see
// provider-contract-evolution.md for the precise Go API)
type Capability struct {
    Supported    bool
    Semantic     string       // e.g., "instant-cutover", "gradual-shift"
    Constraints  []string     // e.g., "requires-revision-history"
    UncertaintyP99 time.Duration  // e.g., expected lag
    Notes        string       // honest free-text caveats
}
```

The system NEVER renders `Supported: true` alone. The UI surfaces the
semantic profile alongside.

## The Capability Set

These are the capabilities AutoStack reasons about. They are the
minimal set required for truthful multi-provider operation; **adding to
this list requires architectural review**.

| Cap | Question | Why it matters |
|---|---|---|
| `C-Deploy` | Can the provider create/update a deployment? | Required for any provider |
| `C-Destroy` | Can it delete? | Required |
| `C-GetStatus` | Can it report current state? | Required |
| `C-Rollback` | Can it return to a prior revision/version? | Operator-critical |
| `C-TrafficSplit` | Can it serve N revisions at percentage weights? | Canary, blue-green |
| `C-RevisionHistory` | Does it retain prior revisions for rollback? | Bounds rollback target set |
| `C-CanaryRollout` | Can it gradually shift traffic to a new revision? | Workflow primitive |
| `C-HealthReporting` | Does it expose health beyond "Ready"? | Truthful drift |
| `C-DeploymentCancel` | Can an in-flight deploy be cancelled? | Workflow primitive |
| `C-RevisionCleanup` | Can old revisions be garbage-collected? | Cost management |
| `C-DriftVisibility` | Can spec-vs-actual be queried? | Drift detection |
| `C-ScalingIntrospection` | Can current scale state be observed? | Operational truth |
| `C-LogStreaming` | Can logs be streamed in real time? | Debugging |
| `C-MetricsVisibility` | Can CPU/memory/request-rate be queried? | Operational truth |
| `C-DestroyConfirmation` | What signals "deletion confirmed"? | Replay correctness (R-5) |
| `C-IdempotentDeploy` | Is `Deploy` safe to call twice? | Required (P-1) |
| `C-OperationTracking` | Does the provider expose LRO IDs? | Phase 3 LRO support |
| `C-EventualConsistency` | What's the typical truth-lag? | Suspicion-counter tuning |

## Capability Profile Examples (Cloud Run baseline)

These profiles are the **honest Phase 2 baseline** for Cloud Run. Phase
3 adds providers that fill in their own; the matrix is then compared.

### Cloud Run — C-Rollback

```yaml
supported: false      # Phase 2: refused, ErrNotImplemented
semantic: "not-implemented"
constraints:
  - "requires-revision-history-tracking-not-yet-built"
  - "requires-traffic-targeting-implementation"
notes: |
  Phase 2 explicitly refuses Rollback with ErrNotImplemented.
  Cloud Run natively supports gradual-traffic-shift via
  Service.Traffic, but AutoStack does not yet persist revision
  lineage. Phase 3.3 PR-4 makes this `supported: true` with
  semantic: "gradual-traffic-shift".
```

### Cloud Run — C-DestroyConfirmation

```yaml
supported: true
semantic: "not-found-poll"
constraints:
  - "post-DeleteService-GetService-returns-NOT_FOUND-eventually"
uncertainty_p99: 60s
notes: |
  Phase 2.8 `confirmDeleted` polls every 5s until NOT_FOUND or
  60s timeout. Records `[DESTROY_CONFIRM_TIMEOUT]` if the
  window expires — target is marked deleted truthfully with
  caveat surfaced.
```

### Cloud Run — C-TrafficSplit

```yaml
supported: true                          # provider-native
semantic: "gradual-percentage-weighted"
constraints:
  - "via-Service.Traffic-array"
  - "per-revision-percent-must-sum-to-100"
notes: |
  Cloud Run natively supports up to 5 traffic targets per service.
  AutoStack does NOT yet wire this through the workflow layer
  (Phase 3.3). Provider exposes it; AutoStack does not consume.
```

### Cloud Run — C-EventualConsistency

```yaml
supported: true
semantic: "read-after-write-lag-typical"
uncertainty_p99: 5s
notes: |
  GetService after a deploy/destroy may return stale state for
  up to ~5s. Suspicion counter (G-6) tolerates first error
  observation from `updating`.
```

### Cloud Run — C-MetricsVisibility

```yaml
supported: false                         # Phase 2: ErrNotImplemented
semantic: "not-implemented"
notes: |
  Cloud Monitoring API path exists but is not wired.
  UI MUST surface "metrics unavailable" rather than rendering 0.
  Phase 3 deferred.
```

## Capability Discovery — How Reconciliation Uses This

Reconciliation queries capabilities at runtime via a new
`Provider.Capabilities() CapabilitySet` method (Phase 3.0; see
[[provider-contract-evolution]]).

```go
// Conceptual flow — actual API in provider-contract-evolution.md
caps := provider.Capabilities()

// Suspicion counter threshold tuned per provider:
suspicionThreshold := caps[C-EventualConsistency].UncertaintyP99 / pollInterval + 1

// Destroy confirm shape per provider:
switch caps[C-DestroyConfirmation].Semantic {
case "not-found-poll":     // Cloud Run path (existing)
case "status-deleted-poll":// ECS path (Phase 3.1)
case "tombstone-poll":     // ACA path (Phase 3.1)
}
```

The reconciler **never** branches on `provider == "..."`. It branches
only on capability semantic strings. New providers add new semantic
strings; the reconciler handles them via lookup table.

## What Capability Flags Do NOT Replace

Capability flags are an honesty contract, not a replacement for:

- **The Provider interface itself.** Required methods remain required;
  capability flags annotate them.
- **`ErrNotImplemented`.** A method that declares
  `Supported: false` MUST still return `ErrNotImplemented` (or the
  documented equivalent) at runtime. Capability flag absence and
  runtime refusal are layered defenses.
- **Lifecycle contracts (DC-1..DC-8).** Capability flags annotate
  lifecycle steps; they don't override the lifecycle.
- **Operational guarantees (G-1..G-19).** A capability flag does not
  exempt a provider from claim-CAS, sweep honesty, or transition
  refusal.

## The Anti-Patterns This Matrix Forbids

1. **Boolean-only capability flags.** A `supports_rollback: true` flag
   without semantic is a lie waiting to happen.
2. **Inferring capabilities from provider name.** No code path may say
   "Cloud Run supports X" by checking `provider == "gcp-cloudrun"`.
   Always query `caps[X]`.
3. **Defaulting capabilities to a permissive value.** If a provider
   doesn't declare a capability, it's `Supported: false`. Absence is
   honest.
4. **UI rendering availability without semantic.** Phase 3 UI MUST
   render the semantic, not the boolean.

## The Verification Gate

Phase 3 introduces a per-provider verification test:

```
TestProviderCapabilityClaims:
  For each capability the provider claims `Supported: true`:
    - Invoke the corresponding method under a test scenario
    - Assert it behaves consistently with the declared semantic
    - Assert it does NOT return ErrNotImplemented
```

A provider that **claims** a capability it does not deliver fails this
test. This is the runtime defense against R-1.

## Phase 3 Sub-Phase Wiring

| Sub-phase | Capability action |
|---|---|
| 3.0 (this) | Define the framework, baseline Cloud Run profile |
| 3.1 | ECS profile, ACA profile, capability negotiation in reconciler |
| 3.2 | EventualConsistency-aware suspicion tuning, DestroyConfirmation-aware sweep |
| 3.3 | Rollback semantic profiles drive deployment-strategy UI |
| 3.4 | (no capability changes; scaling concerns are infra, not provider) |
| 3.5 | UI renders the matrix as operator-facing capability table |
| 3.6 | (GitOps respects capability matrix for rollback target validity) |
| 3.7 | Capability-aware health scoring |
| 3.8 | Per-tenant capability views (future) |

## Maintenance Discipline

When Phase 3 wants to **add** a capability:

1. Add an entry to the capability set table in this doc with the
   question and motivation.
2. Update [[provider-contract-evolution]] with the type definition.
3. Implement the capability in **every** existing provider (Cloud Run
   first); the profile may be `Supported: false`, but the field must
   be present.
4. Add a verification test stub.
5. Update [[provider-normalization-rules]] if the capability informs
   normalization.

When Phase 3 wants to **change** a capability semantic for an existing
provider:

1. Update the profile in this doc.
2. Update the provider implementation.
3. Update the verification test.
4. Update any reconciler lookup-table entries.

When Phase 3 wants to **remove** a capability:

1. Don't, in Phase 3. Removal requires Phase 4 architectural review.

## Related

- [[phase3/provider-contract-evolution]] — Capability API in Go
- [[phase3/provider-normalization-rules]] — How capabilities inform normalization
- [[phase3/ambiguity-semantics-model]] — How uncertainty is surfaced
- [[phase3/multi-provider-risk-analysis]] — R-1, R-5 mitigation
- [[../phase2.9/provider-contracts]] — P-1..P-15 baseline
