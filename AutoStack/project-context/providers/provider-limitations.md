# Provider Limitations — Honest Inventory

## Last Updated
2026-05-14 (Phase 1.9 principal review)

This file enumerates what each provider does, what it pretends to do, and
what it refuses to do. The directive is to never let the system claim a
capability it does not have.

## Cloud Run (GCP)

| Method | Status | Notes |
|---|---|---|
| `ValidateCredentials` | Implemented | Calls `ListServices(locations/-)`. Region-scoped permission gaps are NOT caught. |
| `Deploy` | Implemented | Idempotent on existing service (Update path). **No code path calls it** — see [[lifecycle-assumptions]]. No idempotency key; concurrent callers can race. |
| `GetStatus` | Implemented | Returns `"unknown"` (not `"pending"`) when no Ready/ConfigurationsReady condition is present (Phase 1.9). `Ready` outranks `ConfigurationsReady`. |
| `GetOperation` | **Refused** (`ErrNotImplemented`) | Cloud Run v2 has no per-service operation polling for services; LRO support requires returning operation names from Deploy/Rollback/Destroy and polling separately. |
| `Rollback` | **Refused** (`ErrNotImplemented`) | Previous body was destructive — see [[rollback-semantics]]. |
| `GetMetrics` | **Refused** (`ErrNotImplemented`) | Previous body returned `{0, 0, 0}, nil`, which renders as "0% CPU" in the UI. Honest refusal is better than a zero-success lie. |
| `StreamLogs` | Returns error | Not implemented; Cloud Logging integration needed. Honest. |
| `EstimateCost` | Placeholder | Hard-coded rates. `UncertaintyNote` populated. Per ADR-010, must be replaced with live GCP Billing API call. |
| `GetActualCost` | Returns error | Honest. |
| `Destroy` | Implemented | Idempotent on `NOT_FOUND`. Predicate `existing.Uid != ""` (Phase 1.9 fix — was the trivially-true `strings.Contains(uid, "")`). No post-delete confirmation poll. |
| `ListRegions` | Hard-coded | Static list. No live API call. May drift as GCP adds regions. |
| `CheckQuotas` | **Refused** (`ErrNotImplemented`) | Previous body always returned `Available: true`, telling the UI "deployable" right before deploys would fail. |

## AWS ECS

Not implemented. `providerToProviderName("aws") → ProviderAWSECS`, but no
provider with that name is registered. `GetProvider` returns
`ErrProviderNotFound`. The reconciler logs and skips.

## Azure ACA

Not implemented. Same situation as ECS.

## Provider singleton safety (Phase 1.9)

The Cloud Run provider was previously stateful: `projectID` and `region`
fields were mutated on each `Deploy`/`Rollback` call. A single
`*Provider` is registered for all cloud accounts. Concurrent
reconciliations against different accounts would race those fields.

Phase 1.9 makes the struct empty (`type Provider struct{}`) and derives
project/region from `CloudAccount` on each call. The singleton is now
safe under any future concurrent caller.

## "Unknown" status — provider/reconciler contract

When a provider cannot classify a service state from a single
observation, it returns `Status: "unknown"`. The reconciler:

- Does not persist `"unknown"` (not in the `deployment_targets.status`
  enum).
- Logs `[STATUS_UNKNOWN]`.
- Updates `last_synced` so operators see the poll happened.

This contract is observable from both sides; do not silently default
provider state to a real enum value.

## Related
- [[eventual-consistency-assumptions]]
- [[rollback-semantics]]
- [[lifecycle-assumptions]]
- [[dangerous-edge-cases]]
- [[correctness-limitations]]
