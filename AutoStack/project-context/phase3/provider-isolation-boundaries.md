# Provider Isolation Boundaries (Phase 3.1)

**Last Updated:** 2026-05-14
**Phase:** 3.1 (Provider Architecture Evolution)

## Purpose

Define **exactly** what may leak across the provider boundary, what
must remain inside `pkg/providers/<provider>/`, and how the boundary is
enforced. This is the operational instantiation of HC-6 (provider
isolation) from [[phase3-architecture-evolution]].

R-4 (hidden provider coupling) is the failure mode this doc exists to
prevent. Every Phase 3 PR is reviewed against the boundary rules below.

## The Boundary

```
ALLOWED OUTSIDE pkg/providers/<X>/:
  - Imports of pkg/providers (interface package only)
  - Use of providers.CapabilityKey constants
  - Use of providers.* type definitions
  - Capability-flag-driven branching:
      if provider.Capabilities()[CapX].Semantic == "..." { ... }

FORBIDDEN OUTSIDE pkg/providers/<X>/:
  - Imports of pkg/providers/cloudrun, pkg/providers/ecs, pkg/providers/aca
  - Imports of provider-vendor SDKs (cloud.google.com/go, aws-sdk-go, azidentity)
  - Branching on provider NAME:
      if account.Provider == "aws-ecs" { ... }   // FORBIDDEN
  - Provider-specific error strings in core logic:
      if strings.Contains(err.Error(), "AccessDenied") { ... }   // FORBIDDEN
  - Provider-specific timing assumptions hard-coded
```

## The Three Boundary Layers

### Layer 1 — Interface Package (`pkg/providers/`)

Contains:
- Provider interface definition.
- All shared types (CloudAccount, DeploySpec, DeployResult, TargetStatus, …).
- Capability framework (CapabilityKey, Capability, CapabilitySet, AllCapabilityKeys).
- Provider registry (RegisterProvider, GetProvider).
- ErrNotImplemented, ProviderError, ErrProviderNotFound.

Does NOT contain:
- Provider-specific logic.
- SDK imports.

Importers: **any package**. This is the public seam.

### Layer 2 — Provider Implementation Packages (`pkg/providers/<X>/`)

Contains:
- Provider interface implementation.
- Provider-specific SDK calls.
- Provider-specific error classification.
- Provider-specific lifecycle mapping.
- Provider-specific confirmation helpers.
- Provider-specific contract tests.

Importers: **only** registration in `cmd/` (the main entrypoint) and
the package's own test suite.

### Layer 3 — Consumers (`pkg/reconciler/`, `pkg/controller/`, frontend)

Contains:
- All consumer code that uses `Provider`.

Allowed imports: `pkg/providers` only. Allowed branching: capability-
semantic strings. Forbidden: provider-name branching, SDK imports,
implementation package imports.

## The Allowed Cross-Boundary Communication

The reconciler and controller can communicate with providers via:

1. **Method calls on the Provider interface** — the standard seam.
2. **Capability queries** — `caps := p.Capabilities()` then branch on
   `caps[CapX].Semantic`.
3. **Error categorization** — `category := p.ClassifyError(err)` then
   branch on category (Phase 3 Change 2). The category enum is in
   `pkg/providers`; provider-specific error types are NOT.
4. **TargetStatus / DeployResult fields** — additive Form-B fields
   convey provider truth uniformly.

These are the **only** sanctioned channels.

## The Forbidden Patterns (with examples)

### Pattern F-1: Provider-Name Branching

```go
// FORBIDDEN
if account.Provider == "gcp-cloudrun" {
    timeout = 60 * time.Second
} else if account.Provider == "aws-ecs" {
    timeout = 120 * time.Second
}
```

**Why forbidden:** Each new provider adds an else-if. R-4.

**Replacement:** Capability-driven.

```go
// ALLOWED
caps := provider.Capabilities()
timeout = caps[providers.CapDestroyConfirmation].UncertaintyP99
```

### Pattern F-2: SDK Imports in Core

```go
// FORBIDDEN (in pkg/reconciler/)
import "cloud.google.com/go/run/apiv2"
```

**Replacement:** The reconciler never imports SDKs. Provider methods
abstract them.

### Pattern F-3: Provider-Specific Error Strings

```go
// FORBIDDEN
if strings.Contains(err.Error(), "AccessDenied") {
    return FailureAuth
}
```

**Replacement:** Provider's `ClassifyError` handles this.

```go
// ALLOWED
category := provider.ClassifyError(err)
if category == FailureAuth {
    // ...
}
```

### Pattern F-4: Implementation Package Imports

```go
// FORBIDDEN (in pkg/reconciler/)
import "github.com/janlauber/one-click/pkg/providers/cloudrun"
```

**Replacement:** Use the registry.

```go
// ALLOWED
p, err := providers.GetProvider(account.Provider)
```

### Pattern F-5: Timing Hard-Code

```go
// FORBIDDEN (in pkg/reconciler/)
const cloudRunSuspicionThreshold = 2
```

**Replacement:** Compute from capability.

```go
// ALLOWED
uncertainty := caps[providers.CapEventualConsistency].UncertaintyP99
threshold := max(2, int(uncertainty/pollInterval)+1)
```

### Pattern F-6: Lifecycle Mapping in Core

```go
// FORBIDDEN (in pkg/reconciler/)
switch provider {
case "gcp-cloudrun":
    if ready == "SUCCEEDED" { return "running" }
case "aws-ecs":
    if rolloutState == "COMPLETED" { return "running" }
}
```

**Replacement:** Provider's `MapProviderState` (or equivalent) handles
mapping. Core calls it.

## Enforcement Mechanisms

### Mechanism E-1 — Code Review (Phase 3.1–3.3)

Every PR is reviewed against this doc's forbidden-pattern list. PRs
violating any pattern are blocked.

### Mechanism E-2 — `go vet` Analyzer (Phase 3.4)

A custom analyzer in `tools/providercoupling/`:

```
PASS 1 — Import audit:
  For each .go file under pkg/reconciler/, pkg/controller/:
    For each import:
      If import path starts with pkg/providers/cloudrun, ecs, aca, etc:
        REPORT "provider implementation imported in consumer code"
      If import path is a known cloud SDK:
        REPORT "cloud SDK imported in consumer code"

PASS 2 — String literal audit:
  For each string literal in pkg/reconciler/, pkg/controller/:
    If literal matches "aws-ecs|gcp-cloudrun|azure-aca|kubernetes":
      If used in if/switch comparison:
        REPORT "provider-name branching detected"
```

The analyzer runs in CI; violations fail the build.

### Mechanism E-3 — Capability-Profile Diff (Phase 3.5)

A test that compares each provider's `Capabilities()` output against
the documented profile in this `phase3/` directory. Drift in either
direction (code added without doc, doc added without code) fails CI.

## What This Doc Allows That Looks Like Coupling But Isn't

### Allowed A-1 — Capability-Conditional Logic

The reconciler branches based on capability:

```go
caps := p.Capabilities()
switch caps[CapDestroyConfirmation].Semantic {
case "not-found-poll":
    waitForNotFound(...)
case "status-inactive-poll":
    waitForInactive(...)
case "tombstone-poll":
    waitForTombstone(...)
}
```

This is **NOT** provider coupling. It is capability-semantic
dispatch. The reconciler does not know which provider declared which
semantic — it only knows the semantic and the helper function to call.

The helper functions live in `pkg/reconciler/confirm.go` and are
provider-agnostic — they take a Provider and a predicate function.

### Allowed A-2 — Provider-Aware UI Annotations

The UI displays a capability table per provider. This is operator-
facing truth (R-1 mitigation), not coupling.

### Allowed A-3 — Registry-Based Routing

`providers.GetProvider(account.Provider)` is the canonical lookup.
The string "aws-ecs" appears here as a key; this is not branching, it
is identification.

## The Reverse Boundary: What Providers Cannot Import

Provider modules must NOT import:

- `pkg/reconciler/` — providers are leaves; the reconciler depends on
  them, not vice versa.
- `pkg/controller/` — same.
- Another provider package — providers must not depend on each other.

Provider modules MAY import:

- `pkg/providers` — the interface package.
- `pkg/secrets` — credential handling (shared utility).
- Cloud SDK packages — encapsulated within the provider module.
- `time`, `context`, standard library.

## Phase 3.1 Closure Criteria

For this doc:

1. The boundary table is sealed.
2. Forbidden patterns F-1..F-6 are sealed.
3. Allowed patterns A-1..A-3 are recorded.
4. Enforcement mechanism E-1 is in active use.
5. E-2 (analyzer) is added to Phase 3.4 deliverables.
6. E-3 (capability-diff test) is added to Phase 3.5 deliverables.

## Related

- [[phase3-architecture-evolution]] — HC-6
- [[provider-capability-matrix]] — capability framework
- [[provider-contract-evolution]] — sanctioned interface seams
- [[multi-provider-risk-analysis]] — R-4 mitigation
- [[ecs-fargate-provider-design]] — first non-Cloud-Run provider
- [[azure-aca-provider-design]] — second non-Cloud-Run provider
