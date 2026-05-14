# Lifecycle Normalization Model

**Last Updated:** 2026-05-14
**Phase:** 3.0 (Foundations)

## Purpose

Define **what lifecycle states are canonical** in AutoStack, what
provider-specific states map onto them, and where the mapping is
**deliberately lossy with ambiguity surfaced** rather than silently
flattened. This is the primary defense against R-2 (fake normalization)
and R-6 (lifecycle fragmentation).

## The Anchor: Phase 2 State Set

Phase 2 deploys to a single provider, so the state set is small and
well-defined. These remain the **canonical** lifecycle states. Phase 3
adds nothing here.

| State | Meaning | Terminal? |
|---|---|---|
| `pending` | Operator intent recorded; not yet dispatched | No |
| `creating` | Dispatched; provider acknowledged; not yet ready | No |
| `running` | Provider reports the deployment is operational | No (can transition out) |
| `updating` | A respec or transient state observed; dispatcher may re-dispatch | No |
| `deleting` | Destroy dispatched; awaiting confirmation | No |
| `deleted` | Confirmed gone | YES |
| `error` | Failed; operator intervention required | No (can be respecced) |

These states are **provider-agnostic by design**. They describe the
*control-plane lifecycle of the AutoStack target*, not the
*provider-internal lifecycle of the underlying resource*.

## The Two Lifecycle Layers

Phase 3 distinguishes:

| Layer | Owner | States | Persistence |
|---|---|---|---|
| **AutoStack lifecycle** | `pkg/reconciler` | The 7 canonical states above | `deployment_targets.status` |
| **Provider lifecycle** | `pkg/providers/<X>` | Provider-native states (e.g., Cloud Run conditions, ECS deployment events) | Provider API responses; surfaced via `TargetStatus.Message` and `drift_summary` |

The two layers are **mapped, not merged**. The provider's lifecycle is
the **source of evidence**; AutoStack's lifecycle is the **operator's
contract**.

## The Mapping Function

Each provider implements:

```go
// MapProviderState translates the provider's native lifecycle state
// into the canonical AutoStack state, with explicit ambiguity flags.
//
// Returns:
//   - canonical: the AutoStack state to persist
//   - ambiguous: true if the provider-native state has lossy mapping
//   - detail: human-readable explanation of the provider state,
//             preserved in TargetStatus.Message and lineage
func MapProviderState(providerState string) (canonical string, ambiguous bool, detail string)
```

The mapping is documented per provider in
`pkg/providers/<provider>/lifecycle_mapping.md` (an in-code reference
doc, not a project-context doc). Each Phase 3 provider PR includes
this mapping doc.

## The Canonical Mapping Rules

These rules govern how provider-native states map onto canonical
AutoStack states. **A provider PR violating any rule is rejected.**

### Rule N-1 — `pending` is operator-only

`pending` is set by the controller, never by a provider mapping. No
provider lifecycle state may map to `pending`. If a provider reports a
"queued" or "pre-deploying" intermediate state, it maps to `creating`,
not `pending`.

### Rule N-2 — `creating` may be entered from any pre-ready provider state

Provider states like `CREATE_IN_PROGRESS`, `Pending`, `Updating` (when
no prior `running`), and `Building` all map to `creating`. Detail field
preserves the native name.

### Rule N-3 — `running` requires provider-affirmative readiness

`running` may only be persisted when the provider's mapping
explicitly returns `canonical=running`. The provider's mapping function
is the gate. A vague "the resource exists" provider state does NOT
map to `running` — it maps to `creating` until the provider confirms
operational health (whatever that means per provider).

This is the heart of R-2 mitigation: the platform refuses to lie about
readiness.

### Rule N-4 — `updating` is for confirmed-prior-`running` respecs

Once a target has been `running`, a spec change leading to a new deploy
moves it to `updating`. Provider-native respec states map to
`updating` only if the target was previously `running`. If the target
was `creating` and the operator respecced, the new deploy stays
`creating`.

This preserves Phase 2 transition guard (G-5) semantics across providers.

### Rule N-5 — `deleting` requires destroy-intent

Provider states like ECS `DRAINING` map to `deleting` ONLY when
AutoStack initiated the destroy. If an operator manually destroys
externally and the provider reports `DRAINING`, the target is
`error` with `drift_summary` describing the external mutation (Phase
3.2 drift visibility).

### Rule N-6 — `deleted` requires capability-aware confirmation

`deleted` is persisted only after the per-provider
`DestroyConfirmation` capability's semantic confirms (see [[provider-contract-evolution]]
Change 5). A 200 from `Destroy` is not sufficient. Phase 2.8's
`confirmDeleted` is the Cloud Run instantiation.

### Rule N-7 — `error` is a transition, not a provider state

`error` is set by the reconciler/dispatcher, not by a provider
mapping. Providers report failure conditions; the reconciler decides
whether to escalate to `error` (often after suspicion-counter
confirmation per G-6).

### Rule N-8 — Ambiguous mappings persist as `creating` or `updating` with `ambiguous=true`

If a provider reports a state with no clean canonical mapping (e.g.,
ECS `ROLLBACK_IN_PROGRESS` during a failed deploy), the canonical state
is `updating` (or `creating` if no prior `running`), `ambiguous=true`,
and `detail` carries the native name. The UI surfaces ambiguity.

**This is the most important rule.** Provider differences that don't
fit the canonical model **become visible as ambiguity**, not as
synthesized confidence.

## Provider Mapping Tables (sketch)

These are the *intended* mappings for each provider. The detailed
form lives in the per-provider lifecycle doc.

### Cloud Run mapping (Phase 2 baseline)

| Cloud Run condition | Canonical | Ambiguous? | Notes |
|---|---|---|---|
| `Ready=UNKNOWN` | `creating` | No | Standard pre-ready |
| `Ready=SUCCEEDED` + first observation | `running` | No | Promoted via `updating → running` |
| `Ready=FAILED` | `error` after suspicion | No | G-6 applies |
| `Ready=SUCCEEDED` while traffic still on old revision | `running` | YES | `ServingRevision != latest` triggers ambiguity flag (Phase 3 Change 3) |

### ECS mapping (Phase 3.1 plan)

| ECS service state | Canonical | Ambiguous? | Notes |
|---|---|---|---|
| `ACTIVE` + `runningCount == desiredCount` | `running` | No | Standard healthy |
| `ACTIVE` + `runningCount < desiredCount` | `creating` | YES | Scale-up in progress; not failure |
| `ACTIVE` + `deployments[0].rolloutState=COMPLETED` | `running` | No | Confirmed rollout |
| `ACTIVE` + `deployments[0].rolloutState=IN_PROGRESS` | `updating` (if prior `running`) or `creating` | No | Standard deploy progress |
| `ACTIVE` + `deployments[0].rolloutState=FAILED` | `error` after suspicion | No | G-6 applies |
| `DRAINING` | `deleting` (if AutoStack-initiated) or `error` (if external) | No | N-5 applies |
| `INACTIVE` | `deleted` (after confirmation) | No | Required for N-6 |

### ACA mapping (Phase 3.1 plan)

| ACA revision state | Canonical | Ambiguous? | Notes |
|---|---|---|---|
| `Provisioning` | `creating` | No | Pre-ready |
| `Provisioned` + active | `running` | No | Standard healthy |
| `Provisioned` + inactive | `creating` (waiting for traffic) | YES | Revision exists, not serving |
| `Failed` | `error` after suspicion | No | G-6 applies |
| `Deprovisioning` | `deleting` (if AutoStack-initiated) | No | N-5 applies |

## Transition Guard Extension

Phase 2's `isAllowedTransition` (F-5) enumerates allowed transitions.
Phase 3 extends it with capability-aware refusal:

```go
func isAllowedTransition(from, to string, caps CapabilitySet) bool {
    // Phase 2 rules retained verbatim.

    // Phase 3 additions:
    // - to=running requires the provider to explicitly map there (N-3).
    //   Enforced upstream in updateTargetStatus's mapping check.
    // - to=deleted requires DestroyConfirmation capability supported.
    //   Enforced upstream in confirm logic.
    // - No Phase 3 transition is broader than Phase 2 transitions.
}
```

The signature change is additive (new `caps` parameter). Callers in
Phase 3.0 pass an empty `CapabilitySet`; behavior is bit-identical.
Phase 3.1+ callers pass real capabilities; new refusal paths activate.

## The Ambiguity-Bit Storage

Phase 3 adds a column:

```sql
ALTER TABLE deployment_targets ADD COLUMN lifecycle_ambiguous BOOLEAN DEFAULT FALSE;
ALTER TABLE deployment_targets ADD COLUMN lifecycle_native_state TEXT;
```

These columns capture the ambiguity bit and the provider-native state
name. UI surfaces them. Lineage queries can filter on ambiguity to
investigate drift or unusual behavior.

(Migration is Phase 3.0/3.1 scope; not yet applied.)

## What Phase 3 Refuses To Do

- **Refuse:** Add `running-degraded`, `running-not-yet-serving`,
  `running-canary-in-progress` as top-level states. R-6 (lifecycle
  fragmentation). Use ambiguity bit + native-state column instead.
- **Refuse:** Allow a provider to bypass the mapping function and
  write canonical states directly. The mapping function is the
  single seam.
- **Refuse:** Hide ambiguity at the UI layer. The UI surfaces
  ambiguity; operators choose how to interpret it.

## Phase 3.0 Closure Criteria

Phase 3.0 is closed for this doc when:

1. The canonical state set is sealed (this doc).
2. The N-1..N-8 mapping rules are sealed.
3. The Cloud Run mapping table reflects the current Phase 2 baseline
   (sketch in this doc is canonical until Phase 3.0 implementation).
4. The migration plan for `lifecycle_ambiguous` and
   `lifecycle_native_state` columns is recorded in
   `phase2.9/deferred-Phase3-concerns.md` (or supersedes SC-1
   wording).

## Related

- [[phase3/provider-capability-matrix]] — capabilities that gate states
- [[phase3/provider-contract-evolution]] — `MapProviderState` interface
- [[phase3/ambiguity-semantics-model]] — how ambiguity surfaces
- [[phase3/multi-provider-risk-analysis]] — R-2, R-6 mitigation
- [[../phase2.9/lifecycle-contracts]] — DC-1..DC-8 baseline
- [[../phase2.9/reconciliation-architecture-freeze]] — F-5 transition guard
