# ECS / Fargate Provider Design (Phase 3.1)

**Last Updated:** 2026-05-14
**Phase:** 3.1 (Provider Architecture Evolution)
**Status:** Design only; no code yet.

## Purpose

The first non-Cloud-Run provider. Implements the Phase 3.0 capability
framework against AWS ECS with Fargate launch type. Establishes the
template for ACA and any future provider.

This document is the **design contract**. Code that diverges is wrong;
either the code is wrong or this doc must be updated in the same PR.

## Scope

| In scope (Phase 3.1) | Out of scope |
|---|---|
| Deploy / Destroy / GetStatus | Rollback (Phase 3.3) |
| Capabilities() profile | Metrics, log streaming |
| Lifecycle mapping (ECS → canonical) | Drift detection |
| Destroy confirmation via `INACTIVE` status poll | LRO operation tracking |
| ValidateCredentials via STS GetCallerIdentity | Cost estimates (Phase 3.5) |
| ListRegions (EC2 DescribeRegions) | Quotas (Phase 3.5) |

## ECS Concepts Mapped

| ECS concept | AutoStack concept |
|---|---|
| Cluster | Implicit per account (one cluster per region, named per AutoStack convention) |
| Service | Deployment target |
| Task Definition | Revision (immutable per deploy) |
| Task | A running instance of a revision |
| Deployment (within service) | Current rollout |
| LoadBalancer (target group) | EndpointURL surface (when applicable) |

## Provider Module Structure

```
pkg/providers/ecs/
├── provider.go         — Provider interface impl
├── capabilities.go     — CapabilitySet (per phase3 framework)
├── lifecycle_mapping.go — ECS native state → canonical state
├── confirm.go          — INACTIVE-poll destroy-confirm helper
├── lifecycle_mapping.md — in-code reference doc per N-rule discipline
└── contract_test.go    — runtime capability-claim verification
```

No file imports `pkg/providers/cloudrun`. HC-6 (provider isolation).

## Capability Profile (proposed)

| Capability | Supported | Semantic | Notes |
|---|---|---|---|
| C-Deploy | true | `create-or-update-service` | RegisterTaskDefinition then CreateService/UpdateService |
| C-Destroy | true | `delete-after-scale-to-zero-then-inactive-poll` | DesiredCount=0, then DeleteService, then poll INACTIVE |
| C-GetStatus | true | `deployment-rollout-state-based` | Reads `services.deployments[0].rolloutState` |
| C-Rollback | false (Phase 3.1) | `not-implemented` | Phase 3.3 unlocks |
| C-TrafficSplit | false | `not-implemented` | ECS native CodeDeploy blue/green; AutoStack does not orchestrate |
| C-RevisionHistory | true | `task-definition-revisions-retained` | TaskDefinition family revisions persist on AWS side |
| C-CanaryRollout | false | `not-implemented` | Phase 3.3 may unlock via CodeDeploy integration |
| C-HealthReporting | true | `deployment-circuit-breaker-state` | ECS deployment circuit breaker provides health signal |
| C-DeploymentCancel | true | `update-service-force-new-deployment` | Cancellation = roll forward to prior task def |
| C-RevisionCleanup | false | `not-implemented` | Task def revisions auto-retained by AWS |
| C-DriftVisibility | false | `not-implemented` | Phase 3.2 |
| C-ScalingIntrospection | false | `not-implemented` | Phase 3.5+ |
| C-LogStreaming | false | `not-implemented` | Phase 3+ via CloudWatch Logs |
| C-MetricsVisibility | false | `not-implemented` | Phase 3+ via CloudWatch |
| C-DestroyConfirmation | true | `status-inactive-poll` | Poll DescribeServices until `status=INACTIVE` |
| C-IdempotentDeploy | true | `describe-then-create-or-update` | DescribeServices → Create or Update |
| C-OperationTracking | false | `not-implemented` | ECS does not expose true LRO names |
| C-EventualConsistency | true | `read-after-write-lag-typical` | UncertaintyP99 = 30s (R-8: longer than Cloud Run) |

## Lifecycle Mapping (per N-rules)

| ECS state | Canonical | Ambiguous | Source | Notes |
|---|---|---|---|---|
| `service.status=ACTIVE` + `deployments[0].rolloutState=COMPLETED` + `runningCount=desiredCount` | `running` | false | — | Standard healthy (N-3 affirmative readiness) |
| `service.status=ACTIVE` + `rolloutState=COMPLETED` + `runningCount<desiredCount` | `creating` | true | S-2 | Scale-up in progress; not failure |
| `service.status=ACTIVE` + `rolloutState=IN_PROGRESS` + prior `running` | `updating` | false | — | Standard deploy progress (N-4) |
| `service.status=ACTIVE` + `rolloutState=IN_PROGRESS` + no prior `running` | `creating` | false | — | Initial deploy |
| `service.status=ACTIVE` + `rolloutState=FAILED` | `error` after suspicion | false | — | G-6 applies; reconciler triggers transition |
| `service.status=DRAINING` + destroy-intent | `deleting` | false | — | N-5; AutoStack-initiated |
| `service.status=DRAINING` + no destroy-intent | `error` | true | S-2 | External destroy detected; drift_summary records |
| `service.status=INACTIVE` (post-DeleteService) | `deleted` | false | — | N-6; confirmation complete |
| Service NOT_FOUND | `error` with NOT_FOUND classification | false | — | P-4 |

## Destroy Confirmation Design

ECS deletion is two-step:

1. `UpdateService(desiredCount=0)` — drain running tasks.
2. After tasks reach zero, `DeleteService` — transitions to `DRAINING`.
3. Polling `DescribeServices` eventually returns `status=INACTIVE`.
4. Eventually `DescribeServices` returns `ServiceNotFoundException`.

The confirm semantic is `status-inactive-poll`:

- Poll every 5s.
- Window = 120s (longer than Cloud Run's 60s; ECS draining is slower).
- Success when `status=INACTIVE` observed.
- Timeout: log `[DESTROY_CONFIRM_TIMEOUT]`, mark target `deleted`
  with ambiguity bit set (S-4 bounded → unbounded after timeout).

Implementation lives in `pkg/providers/ecs/confirm.go`. The dispatcher's
Phase 3 Change 5 routing reads `CapDestroyConfirmation.Semantic ==
"status-inactive-poll"` and calls this confirm.

## Error Classification (Phase 3 Change 2)

The ECS provider implements `ClassifyError(err) FailureCategory` with:

| AWS error | Category | Notes |
|---|---|---|
| `AccessDeniedException`, `UnauthorizedException` | `FailureAuth` | IAM role insufficient |
| `ServiceQuotaExceededException` | `FailureQuota` | Service quota hit |
| `ServiceNotFoundException` | `FailurePermanent` | NOT_FOUND analog (P-4) |
| `ClusterNotFoundException` | `FailurePermanent` | Account misconfigured |
| `ResourceInUseException` (during delete) | `FailureTransient` | Retry |
| `ThrottlingException`, `RequestLimitExceeded` | `FailureTransient` | Back off, retry |
| `InvalidParameterException` | `FailurePermanent` | Operator must fix spec |
| Network errors | `FailureTimeout` | Retry |
| Default | Delegate to `DefaultClassifier` | Phase 2 baseline |

## Authentication

AWS credentials are stored in `CloudAccount.CredentialsEncrypted` per
the existing encryption pattern (Phase 2 envelope). Decrypted at runtime
into a temporary `*aws.Config`. The provider MUST NOT cache decrypted
credentials beyond the lifetime of a single Provider call.

**Validate via STS:** `sts:GetCallerIdentity` confirms the credentials
are usable. Region-scoped validation (PR-3 from Phase 2 deferred list)
is a Phase 3.5 enhancement.

## Network Surface

ECS deployments may use an Application Load Balancer (ALB) for ingress
(`ServiceType: LoadBalancer`) or be internal-only. Phase 3.1 supports:

- ALB integration if a `target_group_arn` is provided in
  `DeploySpec.TargetConfig` (provider-specific override).
- Otherwise, `EndpointURL` is left empty with capability note.

The ALB lifecycle is **owned by the operator**, NOT by AutoStack.
AutoStack creates the service with a reference; it does not provision
or destroy the ALB. This is documented in
`provider-isolation-boundaries.md`.

## Region & Cluster Convention

- Cluster name: `autostack-<region>` (created externally; AutoStack
  validates existence in `ValidateCredentials`).
- Service name: `<rolloutID>` (matches the AutoStack convention).
- Task definition family: `<rolloutID>-fargate`.

These conventions are documented and enforced; the operator does not
override them in Phase 3.1.

## Phase 3.1 Closure Criteria

ECS provider is closed for Phase 3.1 when:

1. `pkg/providers/ecs/{provider.go,capabilities.go,lifecycle_mapping.go,confirm.go,contract_test.go}` exist.
2. `contract_test.go` passes; verifies all `AllCapabilityKeys` present;
   asserts profile matches this doc.
3. Provider registered in `pkg/providers/provider.go` (`ProviderAWSECS` constant already exists).
4. Reconciler-level integration test: deploy → running → destroy →
   deleted, end-to-end against a real AWS account in test mode.
5. Build + vet + tests clean.
6. No code path outside `pkg/providers/ecs/` imports
   `github.com/aws/aws-sdk-go-v2/...`. HC-6.

## What This Doc Does NOT Cover

- AWS SDK version selection — pick whatever aligns with current
  go.mod conventions; document in DECISIONS.md when implemented.
- CodeDeploy integration for canary/blue-green (Phase 3.3).
- Spot capacity / Fargate Spot — Phase 3.5 enhancement.
- VPC / subnet / security-group plumbing — assume operator pre-provisions and supplies via TargetConfig.

## Related

- [[provider-capability-matrix]] — capability framework
- [[provider-contract-evolution]] — Provider interface changes
- [[lifecycle-normalization-model]] — N-rules this doc obeys
- [[provider-normalization-rules]] — NORMALIZE/AMBIGUATE/EXPOSE decisions
- [[ambiguity-semantics-model]] — S-2, S-4 sources
- [[provider-isolation-boundaries]] — HC-6 enforcement
- [[multi-provider-risk-analysis]] — R-1, R-4, R-5, R-7 mitigations
