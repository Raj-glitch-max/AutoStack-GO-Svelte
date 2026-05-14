# Multi-Provider Boundaries (Phase 3.2)

**Last Updated:** 2026-05-14
**Phase:** 3.2 (Provider-Normalized Lifecycle Semantics)

## Purpose

Define **where each provider's truth ends** in the AutoStack platform.
For every operational concern, this doc declares: which side of the
boundary owns it, what crosses the boundary, and what does NOT.

Without this discipline, multi-provider systems devolve into circular
ownership: AutoStack waits for the provider to settle; the provider
waits for the operator; the operator waits for AutoStack. Phase 3
defines the boundary explicitly.

## The Concept of Provider Truth

A provider's truth is what its API returns when queried. Provider truth
is **authoritative for the resource state** (the service exists, is
running, is healthy). It is **not authoritative for**:

- The operator's intent (Phase 3 owns this via `pending_destroy`, `endDate`).
- Lineage (Phase 2/3 owns this via `deployment_history`).
- Cross-provider comparison (Phase 3 owns this via capability matrix).
- Ambiguity attribution (Phase 3 owns this via `lifecycle_ambiguity_*`).

## The Boundary Table

| Concern | Provider truth | AutoStack truth | Border policy |
|---|---|---|---|
| Resource existence | YES | mirrors via GetStatus | Provider wins on conflict |
| Resource health | YES | mirrors via GetStatus + suspicion | Provider wins with G-6 tolerance |
| Operator intent | NO | YES | AutoStack writes; provider does not see |
| Deployment lineage | NO | YES | `deployment_history` immutable |
| Cost | YES (live API) | mirrors via EstimateCost | Provider wins; AutoStack caches |
| Manual external mutations | YES | drift_summary records | Provider wins; AutoStack surfaces as drift |
| Resource cleanup ordering | YES (provider drains, etc.) | AutoStack initiates | Provider controls; AutoStack waits with capability-driven timeout |
| Authentication state | YES (creds work or don't) | mirrors via ValidateCredentials | Provider wins |
| Revisions | YES (provider retains) | references via ExternalID | Provider wins; AutoStack cannot delete revisions |
| Traffic distribution | YES (provider applies splits) | requests via Deploy | Provider wins; AutoStack reads back |
| Scale | YES (provider scales) | requests via DeploySpec | Provider wins; AutoStack reads back |

## The Five Authority Rules

### Authority A-1 — Provider state is read-only from AutoStack

AutoStack writes deployment requests via `Deploy`/`Destroy` and reads
state via `GetStatus`. AutoStack does NOT update provider state directly
via SDK calls outside `pkg/providers/<X>/`. The provider boundary
guards this: a `pkg/reconciler/` file calling an SDK directly is a
violation.

### Authority A-2 — AutoStack state is the operator contract

`deployment_targets.status`, `deployment_history`, `operations` —
these are the operator's truth. The reconciler maintains them; the
provider does not write to them directly. Provider state changes flow
through the reconciler.

### Authority A-3 — Conflicts resolve toward the provider

When provider state contradicts AutoStack state, the provider is
authoritative for the resource:

- Provider says NOT_FOUND, AutoStack says `running` → AutoStack
  transitions to `error` (post-suspicion) with drift_summary.
- Provider says `Ready=FAILED`, AutoStack says `running` → suspicion
  counter increments; after second observation, `error`.
- Provider says `Ready=SUCCEEDED`, AutoStack says `error` → AutoStack
  remains `error` (operator must respec); the provider's "fixed"
  state does NOT auto-clear `error`.

The third case is the asymmetry: failure flows up; recovery requires
operator action.

### Authority A-4 — Operator intent stays in AutoStack

`endDate` set, `pending_destroy` flag, respec'd manifest — these are
AutoStack-owned. They never push to the provider directly (no
"forward operator intent to AWS" path). The reconciler translates
intent into provider calls in its own time.

### Authority A-5 — Lineage is AutoStack-only

The provider does not see `deployment_history`. AutoStack writes
history rows for every dispatch attempt, every sweep, every
ambiguity transition. The provider does not read or contribute.
History is **persistent** in PocketBase; provider history (e.g.,
CloudWatch Events) is independent and consultable but not joined.

## What Crosses The Boundary

Inbound to AutoStack (provider → AutoStack):

- `TargetStatus` from `GetStatus`.
- `DeployResult` from `Deploy`.
- Capability profile from `Capabilities()`.
- Error categorization from `ClassifyError`.
- Region list from `ListRegions`.

Outbound to provider (AutoStack → provider):

- `DeploySpec` to `Deploy`.
- `DeploymentTarget` reference to `Destroy`, `GetStatus`, etc.
- `CloudAccount` (with decrypted credentials) to all methods.

Nothing else crosses. Operator intent does not cross. Lineage does not
cross. Cross-provider comparison does not cross.

## What Does NOT Cross The Boundary

These items are deliberately one-sided:

| Item | Lives on | Reason |
|---|---|---|
| Operator's `endDate` | AutoStack | Provider has no concept of operator intent |
| Suspicion counter state | AutoStack | Per-target tolerance; provider unaware |
| Ambiguity bit | AutoStack | Cross-provider construct |
| Circuit breaker state | AutoStack | Local backoff |
| Cycle ID correlation | AutoStack | Reconciler-only |
| AutoStack's notion of `error` | AutoStack | Provider may say "Ready"; AutoStack remembers prior failure until respec |
| Provider's internal events log | Provider | Not retrieved; operator consults provider console |
| Provider's IAM/RBAC | Provider | AutoStack has no opinion |

## The Per-Provider Boundary Annotations

Each provider design doc records its specific boundary positions for
fields where the boundary is non-obvious:

### Cloud Run (Phase 2 baseline)

- **Traffic distribution:** Provider owns Service.Traffic array. AutoStack
  does not yet read it back into TargetStatus (Phase 3.1 work).
- **Revisions:** Provider retains; AutoStack does NOT delete provider
  revisions. C-RevisionCleanup=false.
- **Internal LRO IDs:** Not exposed; AutoStack does not track.
  C-OperationTracking=false.

### ECS (Phase 3.1)

- **Cluster:** Operator pre-provisions; AutoStack does not manage.
  Cluster name is convention (`autostack-<region>`).
- **Task Definition revisions:** Provider retains. AutoStack does NOT
  deregister old revisions.
- **ALB target group:** Operator pre-provisions; AutoStack references
  via `TargetConfig.target_group_arn`. AutoStack does NOT create or
  destroy ALBs.

### ACA (Phase 3.1)

- **Container App Environment:** Operator pre-provisions. AutoStack
  does NOT manage the Environment.
- **Revision retention:** Provider auto-deactivates old revisions
  beyond a configured count. AutoStack does NOT control this.
- **Operation URIs:** Provider exposes; AutoStack MAY consume in Phase
  3.4 worker pool.

## The Operator-Side Boundary

The operator also has a boundary. Operators must NOT:

- Manually mutate provider resources that AutoStack manages
  (`deployment_targets.external_id`-pointed services). This breaks
  drift visibility and reproducibility.
- Manually edit PocketBase rows for active targets. This breaks
  Phase 2 ownership and replay guarantees (A-3 violation).
- Assume capability X exists on provider Y without consulting the
  capability matrix.

These are documented in
[[../phase2.9/safe-operational-boundaries]] and reaffirmed here.

## Boundary Violations As Drift

When the boundary is violated externally (operator clicks "delete" in
GCP console while AutoStack is managing the target), AutoStack:

1. Observes via `GetStatus` returning NOT_FOUND.
2. Classifies as `FailurePermanent` (P-4).
3. Transitions to `error` (after suspicion).
4. Records the divergence in `drift_summary`.
5. Surfaces as drift in UI (Phase 3.2 partial; Phase 3.5 full).

Phase 3 does NOT auto-recreate the resource. The operator must
acknowledge the external mutation and either respec or accept the
deletion.

## Phase 3.2 Closure Criteria

For this doc:

1. The boundary table is sealed.
2. Authority A-1..A-5 are sealed.
3. Per-provider boundary annotations are recorded for Cloud Run, ECS,
   ACA.
4. The operator-side boundary is documented in
   [[../phase2.9/safe-operational-boundaries]] (existing) and any
   gaps closed.

## Related

- [[provider-isolation-boundaries]] — code boundary
- [[provider-drift-model]] — drift handling (this doc's flip side)
- [[partial-success-semantics]] — when truth is incomplete
- [[../phase2.9/safe-operational-boundaries]] — operator boundary
- [[../phase2.9/operational-guarantee-matrix]] — G-12 lineage extension
