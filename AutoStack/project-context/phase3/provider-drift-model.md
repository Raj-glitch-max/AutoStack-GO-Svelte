# Provider Drift Model (Phase 3.2)

**Last Updated:** 2026-05-14
**Phase:** 3.2 (Provider-Normalized Lifecycle Semantics)

## Purpose

Define **what drift is**, how it is detected, how it is surfaced, and
how it is differentiated across providers. Drift is the most operationally
sensitive concept in a multi-provider control plane — getting it wrong
means operators stop trusting either the platform or the provider.

This doc supersedes the Phase 2.8 drift-handling-maturity doc on the
multi-provider dimension; Phase 2.8 remains valid for the single-provider
case.

## The Six Kinds Of Drift

Drift is not one phenomenon. Phase 3 distinguishes six kinds. Each has
distinct detection, surfacing, and remediation paths.

### Drift D-1 — Manual external mutation

The operator (or another tool) modifies the provider resource directly,
bypassing AutoStack. Example: scaling a Cloud Run service via gcloud
CLI.

**Detection:** Spec-vs-actual diff after Phase 3 SC-1 (`deployed_spec`)
lands. Today (Phase 2): invisible (acknowledged limitation).

**Surfacing:** `drift_detected=true`, `drift_summary` describes the
delta.

**Remediation:** Operator chooses — respec to absorb, revert provider
to spec, or accept divergence (turn off drift alerts for this target).

### Drift D-2 — External deletion

The resource is deleted externally. Provider returns NOT_FOUND.

**Detection:** `GetStatus` returns NOT_FOUND, classified `FailurePermanent`.

**Surfacing:** Target transitions to `error` (after suspicion). UI
shows "external deletion detected." Lineage records the event.

**Remediation:** Operator either respecs (recreates via AutoStack) or
clears the target.

### Drift D-3 — Capability-conditional drift

A capability that was previously supported becomes unsupported (rare;
typically only via provider deprecation announcements). Example: GCP
deprecates an API surface AutoStack relies on.

**Detection:** Provider's method begins returning errors that
DefaultClassifier marks `FailurePermanent`.

**Surfacing:** Circuit breaker opens; UI surfaces "provider capability
unavailable." Operator action: update AutoStack to a new provider
version.

**Remediation:** Code update — out of operator scope.

### Drift D-4 — Eventual-consistency drift

The provider's state has settled but AutoStack hasn't observed the
settled state yet. Not true drift; resolves on next poll.

**Detection:** Suspicion counter logic (G-6) absorbs.

**Surfacing:** Not surfaced externally during the suspicion window.

**Remediation:** None needed; resolves automatically.

### Drift D-5 — Replay drift

AutoStack restarted during a deploy; the operation row is now `failed`
(or live, if heartbeat fired). The provider may have completed the
deploy successfully despite AutoStack's perception.

**Detection:** Sweep marks op `failed`. Next `GetStatus` reports
`running` from the provider.

**Surfacing:** Target transitions through normal lifecycle. The
deployment_history row from the sweep is the audit trail. Provisional
`error` may resolve to `running` on subsequent observation.

**Remediation:** None automatic; the system converges.

### Drift D-6 — Cross-provider lifecycle drift

A target migrates from one provider to another (Phase 3 may eventually
support this; not in 3.2). The old provider's resource still exists;
the new provider's resource exists. AutoStack thinks one is "active."

**Phase 3 stance:** REFUSE to support cross-provider migration. The
target's `provider` field is immutable. Operators wishing to migrate
create a new target with the new provider and destroy the old one.
D-6 is documented to be explicit; not implemented.

## The Drift Model In One Diagram

```
                    +---------------------+
                    |  provider truth     |
                    |  (resource state)   |
                    +----------+----------+
                               |
            +------------------+------------------+
            |                  |                  |
    +-------v------+   +-------v------+   +-------v------+
    | match spec   |   | mutated      |   | absent        |
    | (no drift)   |   | externally   |   | (deleted ext) |
    +--------------+   +------+-------+   +-------+-------+
                              |                   |
                              | D-1               | D-2
                              v                   v
                       drift_detected=true   transition to error
                       drift_summary=...     with NOT_FOUND
```

## The Detection Surface

Drift detection lives in two places:

### Surface 1 — `reconcileOne` (Phase 2 + Phase 3)

After `GetStatus`:

- NOT_FOUND → D-2 (existing Phase 2 behavior + Phase 3 lineage extension)
- Hard provider error → D-3 (after circuit-open accumulation)
- Stale-spec convergence loop → already handled (G-7)

### Surface 2 — Drift Cycle (Phase 3.2 NEW)

A second reconciler pass dedicated to drift:

```go
// Phase 3.2 — drift reconcile pass
// Runs every N minutes (configurable; default 5min).
// Iterates all `running` targets whose capability profile supports drift.

for _, target := range targets where status == "running" {
    p, _ := providers.GetProvider(target.Provider)
    if !p.Capabilities()[CapDriftVisibility].Supported {
        continue  // honest skip
    }
    actual, err := p.DumpServiceSpec(ctx, account, target)  // new method (Phase 3.2)
    if err != nil { ... }
    delta := diffSpec(target.DeployedSpec, actual)
    if delta != "" {
        target.DriftDetected = true
        target.DriftSummary = delta
        // ... persist + history row
    }
}
```

`DumpServiceSpec` is a new optional Provider method (Phase 3.2 Change).
Providers that support it declare `CapDriftVisibility = Supported: true`.
Cloud Run, ECS, and ACA all CAN implement it; the implementation work
is Phase 3.2.

## The Diff Algorithm

Drift detection requires a stable, semantic diff. The diff library lives
in `pkg/reconciler/drift/`:

```go
type DriftDelta struct {
    Field      string  // e.g., "spec.scale.minReplicas"
    Expected   string  // from deployed_spec
    Actual     string  // from provider
    Severity   string  // "info", "warn", "critical"
}

func diffSpec(deployed, actual string) []DriftDelta
```

Severity rules:

- `image.tag` mismatch → critical.
- `scale.{min,max}Replicas` mismatch → warn.
- `env[].value` mismatch → critical (likely sensitive).
- `network.interfaces[].ingressHost` mismatch → warn.
- Provider-internal fields (timestamps, generated IDs) → ignored.

Per-field severity rules live in `drift/severity.go`. Reviewers
augment them based on operational experience.

## The Surfacing Contract

When drift is detected:

1. `deployment_targets.drift_detected = true`.
2. `deployment_targets.drift_summary = <JSON list of DriftDelta>`.
3. `deployment_history` row written:
   `status=drift_detected, message=<count> fields drifted`.
4. UI shows drift badge with severity color (Phase 3.5).
5. **No automatic remediation.** The operator decides.

The "no auto-remediation" rule is deliberate. Auto-remediation creates
oscillation (provider reverts → AutoStack reverts → provider reverts).
Phase 3 surfaces; Phase 4+ may add opt-in remediation policies.

## The Provider-Specific Drift Profile

Each provider's drift visibility varies by what their API exposes:

| Provider | Spec fields visible | Provider fields hidden |
|---|---|---|
| Cloud Run | image, env, scale, resources, traffic | revision metadata, IAM bindings |
| ECS | task def fields, service config, capacity providers | task IAM role assumptions |
| ACA | app config, scale rules, ingress | dapr config (separate API) |

Phase 3.2 documents per-provider limitations; the diff library marks
unobservable fields with `Severity: "unknown"` rather than skipping
them silently.

## Phase 3.2 Closure Criteria

For this doc + corresponding code:

1. `DumpServiceSpec` method added to Provider interface (Form A, gated
   by capability).
2. Cloud Run, ECS, ACA implementations of `DumpServiceSpec` (provider
   work).
3. `drift/diff.go` + `drift/severity.go` exist.
4. Drift reconcile cycle implemented in `pkg/reconciler/drift.go`.
5. Migration adds `deployment_targets.deployed_spec TEXT` column.
6. UI surfaces drift badge.

## What Phase 3 Refuses To Do

- **Refuse:** Auto-remediate drift. Operators decide.
- **Refuse:** Drift detection for unsupported providers — capability
  matrix governs.
- **Refuse:** Drift alerts via email/Slack without operator opt-in
  (Phase 3.7 observability work).
- **Refuse:** Drift snapshots without `deployed_spec`. No baseline = no
  truth = no drift detection.

## Related

- [[multi-provider-boundaries]] — manual-mutation = boundary violation
- [[provider-capability-matrix]] — CapDriftVisibility
- [[ambiguity-semantics-model]] — drift produces ambiguity if provider
  reports unexpected state
- [[../phase2.8/drift-handling-maturity]] — Phase 2 baseline (single-provider)
- [[../phase2.8/manual-cloud-mutation-policy]] — operator-side rules
