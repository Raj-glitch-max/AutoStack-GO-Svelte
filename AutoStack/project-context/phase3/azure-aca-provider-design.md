# Azure Container Apps (ACA) Provider Design (Phase 3.1)

**Last Updated:** 2026-05-14
**Phase:** 3.1 (Provider Architecture Evolution)
**Status:** Design only; no code yet.

## Purpose

The second non-Cloud-Run provider. Lands after ECS so the second provider
validates the framework's portability, not just its monolithic
correctness.

## Scope

| In scope (Phase 3.1) | Out of scope |
|---|---|
| Deploy / Destroy / GetStatus | Rollback (Phase 3.3) |
| Capabilities() profile | Metrics, log streaming |
| Lifecycle mapping (ACA → canonical) | Drift detection |
| Destroy confirmation via tombstone poll | LRO tracking |
| ValidateCredentials | Cost estimates |

## ACA Concepts Mapped

| ACA concept | AutoStack concept |
|---|---|
| Container App Environment | Implicit per account (one per region/subscription) |
| Container App | Deployment target |
| Revision | Revision (provider-managed history) |
| Ingress | EndpointURL surface |
| Replica | Running task instance |

## Provider Module Structure

```
pkg/providers/aca/
├── provider.go
├── capabilities.go
├── lifecycle_mapping.go
├── confirm.go            — tombstone-poll destroy-confirm helper
├── lifecycle_mapping.md
└── contract_test.go
```

## Capability Profile (proposed)

| Capability | Supported | Semantic | Notes |
|---|---|---|---|
| C-Deploy | true | `create-or-update-app` | CreateOrUpdate via ARM REST |
| C-Destroy | true | `delete-then-tombstone-poll` | Delete; provider returns "deprovisioning" then 404 |
| C-GetStatus | true | `revision-active-state-based` | Reads latest active revision's `provisioningState` |
| C-Rollback | true (capability-only) | `revision-activate` | ACA natively supports — but AutoStack's workflow exposure is Phase 3.3 |
| C-TrafficSplit | true | `revision-traffic-weights-native` | Up to 100 revisions; percent-weighted |
| C-RevisionHistory | true | `provider-retained-revisions` | ACA retains revisions natively |
| C-CanaryRollout | false (workflow not wired) | `not-implemented` | Provider-native; Phase 3.3 workflow exposes |
| C-HealthReporting | true | `provisioning-state-plus-replica-count` | Phase 3.1 partial; full Phase 3.2 |
| C-DeploymentCancel | false | `not-implemented` | ACA revisions are immutable; no in-flight cancel |
| C-RevisionCleanup | true | `auto-deactivate-after-N-revisions` | ACA limits retained revisions |
| C-DriftVisibility | false | `not-implemented` | Phase 3.2 |
| C-ScalingIntrospection | false | `not-implemented` | Phase 3+ |
| C-LogStreaming | false | `not-implemented` | Phase 3+ via Log Analytics |
| C-MetricsVisibility | false | `not-implemented` | Phase 3+ via Azure Monitor |
| C-DestroyConfirmation | true | `tombstone-poll` | Poll until 404 |
| C-IdempotentDeploy | true | `get-then-create-or-update` | ARM idempotent by design |
| C-OperationTracking | true | `arm-async-operation-uri` | ARM provides operation URIs (LRO native) |
| C-EventualConsistency | true | `read-after-write-lag-typical` | UncertaintyP99 = 15s (between Cloud Run 5s and ECS 30s) |

**Notable:** ACA's `C-OperationTracking` is `Supported: true` —
the FIRST provider where LRO tracking is honestly implementable.
Phase 3.1 implements the capability flag and a basic implementation;
the reconciler may use it (Phase 3.4+) to replace polling with operation-URI tracking.

## Lifecycle Mapping (per N-rules)

| ACA revision state | Canonical | Ambiguous | Source | Notes |
|---|---|---|---|---|
| `Provisioning` | `creating` | false | — | Pre-ready |
| `Provisioned` + Active + Healthy replicas | `running` | false | — | N-3 affirmative readiness |
| `Provisioned` + Inactive (revision exists, not serving) | `creating` | true | S-2 | Revision exists, not yet promoted |
| `Provisioned` + replicas 0 (scaled-to-zero) | `running` | true | S-2 | ACA scale-to-zero is healthy idle |
| `Failed` | `error` after suspicion | false | — | G-6 applies |
| `Deprovisioning` + destroy-intent | `deleting` | false | — | N-5 |
| `Deprovisioning` + no destroy-intent | `error` | true | S-2 | External delete detected |
| App not found (tombstone) | `deleted` | false | — | N-6; confirmation complete |

## Destroy Confirmation Design

ACA deletion is asynchronous:

1. `DELETE /subscriptions/.../containerApps/<name>` returns 202 Accepted.
2. Subsequent GETs return `provisioningState: Deprovisioning`.
3. Eventually GET returns 404 (tombstone observed).

Confirm semantic: `tombstone-poll`.

- Poll every 5s.
- Window = 90s (between Cloud Run's 60s and ECS's 120s).
- Success when 404 observed.
- Timeout: log `[DESTROY_CONFIRM_TIMEOUT]`, mark target `deleted`
  with ambiguity bit set.

Implementation lives in `pkg/providers/aca/confirm.go`.

## Scale-to-Zero Ambiguity (S-2)

ACA's killer feature is scale-to-zero. A `running` target with
0 replicas is **healthy**, not failed. The reconciler must not treat
0 replicas as drift or error.

Handling per [[provider-normalization-rules]] decision tree:

- Q1 (lifecycle effect): Yes — must remain `running`.
- N-3 (affirmative readiness): `Provisioned` + active satisfies.
- Q3 (operator decision impact): Yes — operator should know "idle, not failed."
- Treatment: **AMBIGUATE**. Canonical `running`, ambiguity bit set,
  `lifecycle_native_state="Provisioned-scale-to-zero-active"`,
  source S-2.

UI surfaces: "Running (idle — 0 replicas, scaled to zero)".

## Error Classification

| Azure error | Category |
|---|---|
| `AuthenticationFailed`, `InvalidAuthenticationToken` | `FailureAuth` |
| `QuotaExceeded` | `FailureQuota` |
| `ResourceNotFound` | `FailurePermanent` |
| `ResourceGroupNotFound` | `FailurePermanent` |
| `Conflict` (409 during update) | `FailureTransient` |
| `TooManyRequests` (429) | `FailureTransient` |
| `InvalidRequestContent` | `FailurePermanent` |
| Network errors | `FailureTimeout` |
| Default | DefaultClassifier |

## Authentication

Azure credentials in `CloudAccount.CredentialsEncrypted`. Service principal
(client ID + tenant ID + client secret) is the Phase 3.1 expected
shape. Managed identity comes in Phase 3.5+.

Validate via `Subscriptions.List()` — minimal call that confirms
credentials work in the subscription.

## Region & Environment Convention

- Container App Environment: `autostack-<region>` (operator pre-provisions).
- App name: `<rolloutID>`.
- AutoStack does not manage the Environment lifecycle (HC-6 isolation —
  environment is operator-managed infrastructure, not deployment target).

## Phase 3.1 Closure Criteria

ACA provider is closed for Phase 3.1 when:

1. `pkg/providers/aca/{provider.go,capabilities.go,lifecycle_mapping.go,confirm.go,contract_test.go}` exist.
2. `contract_test.go` passes.
3. Provider registered.
4. Reconciler integration test: deploy → running → destroy → deleted
   end-to-end.
5. Build + vet + tests clean.
6. No code outside `pkg/providers/aca/` imports Azure SDK.

## Cross-Provider Validation (gates Phase 3.1 closure)

After both ECS and ACA land, run `TestCrossProviderNormalizationConsistency`
([[provider-normalization-rules]]). For each NORMALIZE-treatment
difference, the test must pass on all three providers.

## Related

- [[provider-capability-matrix]] — capability framework
- [[provider-contract-evolution]] — Provider interface changes
- [[lifecycle-normalization-model]] — N-rules
- [[provider-normalization-rules]] — NORMALIZE/AMBIGUATE/EXPOSE
- [[ambiguity-semantics-model]] — S-2 scale-to-zero, S-4 destroy timeout
- [[ecs-fargate-provider-design]] — sibling provider design
- [[provider-isolation-boundaries]] — HC-6 enforcement
