# Provider Capability Negotiation (Phase 3.1)

**Last Updated:** 2026-05-14
**Phase:** 3.1 (Provider Architecture Evolution)

## Purpose

Define **how the reconciler and controller query and act on provider
capabilities at runtime**. The capability matrix is the contract; this
doc is the **operational procedure** for using it.

R-4 (hidden provider coupling) and R-1 (abstraction lies) are mitigated
when capability negotiation is uniformly disciplined.

## The Three Negotiation Surfaces

Capability negotiation happens at three points in the system:

| Surface | Who reads | When | Cache lifetime |
|---|---|---|---|
| Reconciler dispatch | `pkg/reconciler` | Per-cycle | Process |
| Controller request handling | `pkg/controller` | Per-request | Process |
| Frontend display | SvelteKit via API | On-load | Session (revalidate on focus) |

The capability profile is **read-mostly** (a provider's
`Capabilities()` is deterministic and stable for the process
lifetime). Callers may cache aggressively.

## The Canonical Negotiation Procedure

```go
// In reconciler / controller:
provider, err := providers.GetProvider(account.Provider)
if err != nil {
    return err  // unknown provider — already handled by registry
}

caps := provider.Capabilities()

// Capability checks BEFORE invoking the corresponding method.
// This is the runtime defense against R-1 (abstraction lies):
// methods that have Supported=false return ErrNotImplemented at
// runtime as well, but checking the flag first lets the caller
// surface "unavailable" cleanly rather than encountering an error.

if !caps[providers.CapRollback].Supported {
    return ErrCapabilityUnavailable{
        Cap:      providers.CapRollback,
        Provider: account.Provider,
        Notes:    caps[providers.CapRollback].Notes,
    }
}

result, err := provider.Rollback(ctx, account, target, revision)
```

## The Defensive Pattern

For every capability-driven action, callers SHOULD:

1. Query `caps[CapX]`.
2. If `Supported=false`: return a structured error that the UI can
   render as "this provider does not support X" with the `Notes` field
   as explanation. **Do not** call the method — calling
   `Rollback` on an unsupported provider works (returns
   `ErrNotImplemented`) but burns API quota for no benefit.
3. If `Supported=true`: branch on `Semantic` if multiple semantics
   exist for this capability (e.g., destroy confirmation).
4. Honor `UncertaintyP99` for any timing-sensitive logic.
5. Surface `Constraints` and `Notes` to the operator on the relevant
   UI surface.

## Capability-Driven Dispatch Tables

The reconciler maintains lookup tables for capabilities with multiple
semantics. These tables live in `pkg/reconciler/dispatch_tables.go`
(Phase 3.1 file).

### Destroy Confirmation Dispatch

```go
type confirmFn func(ctx context.Context, p Provider, account *providers.CloudAccount, target *providers.DeploymentTarget) error

var destroyConfirmDispatch = map[string]confirmFn{
    "not-found-poll":       confirmNotFound,        // Cloud Run
    "status-inactive-poll": confirmInactiveStatus,  // ECS
    "tombstone-poll":       confirmTombstone,       // ACA
}

func dispatchDestroyConfirm(p Provider, account *providers.CloudAccount, target *providers.DeploymentTarget) error {
    caps := p.Capabilities()
    sem := caps[providers.CapDestroyConfirmation].Semantic
    fn, ok := destroyConfirmDispatch[sem]
    if !ok {
        log.Printf("[CONFIRM_UNKNOWN_SEMANTIC] provider=%s semantic=%q", account.Provider, sem)
        return ErrUnknownCapabilitySemantic{Cap: providers.CapDestroyConfirmation, Semantic: sem}
    }
    return fn(ctx, p, account, target)
}
```

A new provider with `tombstone-poll` semantic does NOT require core
changes. It maps to the existing helper. A new provider with a novel
semantic (e.g., `event-driven-confirm`) is rejected by the dispatch
table until `dispatch_tables.go` adds an entry — which is the
sanctioned moment to review whether a new helper is needed.

### Suspicion Threshold Computation

```go
func computeSuspicionThreshold(caps providers.CapabilitySet, pollInterval time.Duration) int {
    uncertainty := caps[providers.CapEventualConsistency].UncertaintyP99
    if uncertainty == 0 {
        return 2  // Phase 2 baseline
    }
    return max(2, int(uncertainty/pollInterval)+1)
}
```

This is **the** R-8 (cross-provider ambiguity) mitigation. Different
providers settle at different latencies; the reconciler reads the
capability and tunes itself. The operator sees uniform behavior.

## Capability Versioning (deferred — Phase 4 territory, NOT Phase 3)

Phase 3 assumes capability profiles are static per process. If a
provider's profile changes (a code update), restart picks up the new
profile.

Phase 4 may need capability **versions** for safe rolling deploys of
the reconciler itself — but that work is out of scope for Phase 3
(see [[future-ha-boundary-analysis]] H-2).

## Error Types

The capability-negotiation error set:

```go
// ErrCapabilityUnavailable is returned when a caller queries a capability
// that the provider does not support. Distinct from ErrNotImplemented:
// ErrCapabilityUnavailable is the polite refusal at the negotiation
// surface; ErrNotImplemented is the fallback when negotiation is bypassed.
type ErrCapabilityUnavailable struct {
    Cap      providers.CapabilityKey
    Provider string
    Notes    string
}

// ErrUnknownCapabilitySemantic is returned when a dispatch table does
// not have an entry for the semantic a provider declares. Indicates a
// new semantic was introduced without updating the dispatch table.
type ErrUnknownCapabilitySemantic struct {
    Cap      providers.CapabilityKey
    Semantic string
}
```

Both are logged with `[CAPABILITY_REFUSED]` and `[CAPABILITY_UNKNOWN]`
tags respectively, and produce a `deployment_history` row when they
arise during a lifecycle event (G-12 / DC-8 extension).

## API Surface (controller → frontend)

A new `GET /api/v1/providers/:name/capabilities` endpoint returns the
provider's profile as JSON. Frontend renders the capability table on
the cloud account detail page.

```json
{
  "provider": "gcp-cloudrun",
  "capabilities": {
    "C-Rollback": {
      "supported": false,
      "semantic": "not-implemented",
      "notes": "Phase 2 declined per P-11. Phase 3.3 (PR-4) makes this..."
    },
    "C-DestroyConfirmation": {
      "supported": true,
      "semantic": "not-found-poll",
      "uncertainty_p99": "60s",
      "notes": "Phase 2.8 confirmDeleted..."
    }
  }
}
```

The endpoint is read-only and unauthenticated for the user's own
account (RBAC details defer to Phase 3.8).

## Audit Trail

When a capability-driven decision changes operational behavior (e.g.,
suspicion threshold differs from Phase 2 baseline), the reconciler logs:

```
[CAPABILITY_TUNED] cycle=<id> target=<id> provider=<name> capability=<cap> applied=<value>
```

This makes the negotiation auditable from logs.

## The Anti-Patterns This Doc Forbids

1. **Bypassing the capability flag.** Calling a method whose capability
   is `Supported=false`. Surfaces ErrNotImplemented but burns API quota.
2. **Caching the capability set across providers.** Each provider gets
   its own cache. Caches do not survive `RegisterProvider` re-invocation.
3. **Deriving capability from provider name.** F-1 of
   [[provider-isolation-boundaries]]. Always read via
   `Capabilities()`.
4. **Promoting `Notes` field to machine-readable.** `Notes` is for
   humans. Machine logic reads `Supported`, `Semantic`, `Constraints`,
   `UncertaintyP99`. Do NOT parse `Notes` programmatically.

## Phase 3.1 Closure Criteria

For this doc:

1. `dispatch_tables.go` exists with destroy-confirm table.
2. `computeSuspicionThreshold` exists and is used by `noteSuspectError`.
3. `ErrCapabilityUnavailable` / `ErrUnknownCapabilitySemantic` types
   defined.
4. `GET /api/v1/providers/:name/capabilities` endpoint exists and
   tested.
5. Frontend renders capability table per provider on the cloud account
   detail page.

## Related

- [[provider-capability-matrix]] — capability framework
- [[provider-contract-evolution]] — Phase 3 Change 5 + Change 6
- [[provider-isolation-boundaries]] — what coupling is forbidden
- [[multi-provider-risk-analysis]] — R-1, R-4, R-8 mitigations
- [[../phase2.9/operational-guarantee-matrix]] — G-12, G-13, G-14
