# Ambiguity Semantics Model

**Last Updated:** 2026-05-14
**Phase:** 3.0 (Foundations)

## Purpose

Define how AutoStack **represents, propagates, and surfaces ambiguity**
when reality is genuinely uncertain — across providers, across replay,
across eventual consistency.

This is the operational instantiation of the truthful-state philosophy
established in Phase 2 ("the system refuses to lie"). Phase 3 extends
it from single-provider truth into multi-provider truth.

## The Foundational Principle

```
Certainty and uncertainty are both first-class.
```

A control plane that surfaces only certainty is silently lying every
time it doesn't know. AutoStack distinguishes:

| Type | Meaning |
|---|---|
| **Certain affirmative** | "The deployment is running and serving traffic." |
| **Certain negative** | "The deployment is in error; specific reason X." |
| **Bounded ambiguity** | "The deployment is provisionally running; serving revision may not match latest. Investigating." |
| **Unbounded ambiguity** | "The provider has not reported in N minutes; state unknown." |

Phase 2 introduced bounded ambiguity (the `[SUSPICION_HOLD]`,
`[DESTROY_CONFIRM_TIMEOUT]`, `[STATUS_UNKNOWN]` flags). Phase 3
formalizes the model.

## The Five Sources of Ambiguity

Ambiguity in a multi-provider system comes from five distinct
sources. Each is handled differently.

### S-1 — Eventual consistency

The provider's API returns a stale view of state. Latency is bounded
(see `C-EventualConsistency.UncertaintyP99` in
[[provider-capability-matrix]]).

**Resolution policy:** Wait. The suspicion counter (G-6) absorbs first
observations; the second confirms. Window is tuned by capability.

**Surface:** `[SUSPICION_HOLD]` log; `lifecycle_ambiguous=false`
(internal state; not operator-visible during the window).

### S-2 — Provider-native state with no canonical mapping

The provider reports a state the canonical mapping doesn't cleanly
absorb (see [[lifecycle-normalization-model]] N-8).

**Resolution policy:** Map to the closest canonical state with
`ambiguous=true` and preserve the native state in
`lifecycle_native_state`.

**Surface:** UI shows "(provider-native: `XYZ`)" badge alongside the
canonical state.

### S-3 — Capability gap

A provider does not support a capability that another provider does.
The operator is comparing across providers.

**Resolution policy:** Capability matrix declares the gap honestly.
UI surfaces "(not supported by this provider)" for affected actions.

**Surface:** Disabled UI controls with explanatory tooltip; capability
table view.

### S-4 — Confirmation timeout

A long-running operation's confirmation window elapses without a
definitive answer. Example: `confirmDeleted` 60s window expires;
service may still exist.

**Resolution policy:** Persist the canonical state truthfully
(`deleted` per the operator's intent + provider 200) AND record the
ambiguity ([`[DESTROY_CONFIRM_TIMEOUT]`] log + history row).

**Surface:** Status badge: "deleted (unconfirmed)"; lineage row
flags the gap.

### S-5 — Provider silence (extended)

The provider's API is unreachable, returns errors, or stops responding
for an extended period beyond eventual-consistency window.

**Resolution policy:** Suspicion counter exhausts → `error` with
sanitized reason. Circuit breaker opens. Last-known canonical state
preserved; UI shows "stale" badge if the last successful poll is older
than 2× tick interval.

**Surface:** Stale badge + last-poll timestamp. `[CIRCUIT_OPEN]` log.

## The Ambiguity Type Hierarchy

In code (Phase 3.0 conceptual; type lives in `pkg/reconciler`):

```go
type Ambiguity struct {
    Source        AmbiguitySource   // S-1..S-5
    Bounded       bool              // true if a deadline exists for resolution
    DeadlineAt    *time.Time        // set if Bounded
    NativeDetail  string            // provider-native state name or message
    Reason        string            // human-readable
}

type AmbiguitySource string

const (
    AmbiguityEventualConsistency AmbiguitySource = "S-1"
    AmbiguityProviderNative      AmbiguitySource = "S-2"
    AmbiguityCapabilityGap       AmbiguitySource = "S-3"
    AmbiguityConfirmTimeout      AmbiguitySource = "S-4"
    AmbiguityProviderSilence     AmbiguitySource = "S-5"
)
```

## The Storage Model

Phase 3 adds:

```sql
ALTER TABLE deployment_targets
  ADD COLUMN lifecycle_ambiguous BOOLEAN DEFAULT FALSE,
  ADD COLUMN lifecycle_ambiguity_source TEXT,         -- S-1..S-5
  ADD COLUMN lifecycle_ambiguity_detail TEXT,         -- native state / reason
  ADD COLUMN lifecycle_ambiguity_deadline DATETIME;   -- when ambiguity self-resolves, if bounded
```

These columns are operationally critical, not optional. Migration
lands in Phase 3.0 or 3.1.

## The Propagation Rules

### P-1 — Ambiguity propagates from provider to canonical state, not vice versa

If the provider mapping returns `ambiguous=true`, the canonical state
is set AND the ambiguity flag is set. The canonical state itself is
never altered by ambiguity.

### P-2 — Bounded ambiguity has a deadline

S-1 and S-4 ambiguities are bounded. Their deadline is recorded. The
reconciler clears the ambiguity bit on the next confirming
observation OR on deadline expiry (with an `[AMBIGUITY_TIMEOUT]` log
and a history row).

### P-3 — Unbounded ambiguity persists until acted on

S-2, S-3, S-5 ambiguities persist until either:
- Provider behavior changes (mapping resolves it).
- Operator action resolves it (respec, manual investigation).
- Capability matrix updates (S-3 only).

### P-4 — Lineage records every ambiguity transition

Every entry into and exit from ambiguity writes a `deployment_history`
row with `status = "ambiguity_entered"` / `"ambiguity_cleared"` and a
`message` describing the source and detail. G-12 (lineage
completeness) extends to ambiguity.

### P-5 — The UI MUST NOT hide ambiguity

Frontend rendering MUST surface the ambiguity bit. The form of
surfacing (badge, color, modal) is a design choice; the surfacing
itself is contractual. This is the operator-facing R-1 defense.

### P-6 — Ambiguity does not block reconciliation

A target with `lifecycle_ambiguous=true` continues normal
reconciliation. Ambiguity is information, not a control-flow state.
The reconciler may schedule additional polls (Phase 3.4) to attempt
resolution, but the lifecycle path is unchanged.

## The Capability-Aware Tuning

Phase 3's per-provider suspicion threshold (see [[provider-contract-evolution]]
Change 6) is the first ambiguity-tuning hook. It computes:

```go
suspicionThreshold = max(2, int(uncertaintyP99 / pollInterval) + 1)
```

This means:

- Cloud Run (5s lag, 30s poll) → threshold 2 (Phase 2 default).
- ECS (30s lag, 30s poll) → threshold 3.
- A future provider with 90s lag → threshold 4.

The operator-visible behavior is uniform: a target flapping in and out
of error doesn't persist as `error` until confirmed. The internal
tuning preserves correctness across providers.

## The Forbidden Patterns

1. **Silent ambiguity clearance.** The reconciler never clears
   `lifecycle_ambiguous` without writing a lineage row. P-4 is
   absolute.
2. **Ambiguity without source attribution.** Setting `ambiguous=true`
   without setting `ambiguity_source` is a partial lie. The operator
   needs to know which kind.
3. **Ambiguity as a status alias.** Don't introduce `running-ambiguous`
   as a canonical state. The ambiguity bit is the modifier; the
   canonical state stands.
4. **Ambiguity inferred from operator action.** Operators initiating
   destroys, respec, or rollback do not create ambiguity. Provider
   behavior creates ambiguity.

## Phase 3 Sub-Phase Integration

| Sub-phase | Ambiguity work |
|---|---|
| 3.0 (this) | Model + storage schema + propagation rules |
| 3.1 | Provider mapping functions return `ambiguous` per N-8 |
| 3.2 | Drift visibility uses S-2 (provider-native detail) |
| 3.3 | Workflow primitives respect ambiguity (no canary promotion during S-1/S-4) |
| 3.4 | Reconciliation prioritization may de-prioritize ambiguous targets |
| 3.5 | UI surfaces all five sources with operator-readable explanations |
| 3.6 | GitOps integration must NOT fight ambiguity (no auto-respec if ambiguous) |
| 3.7 | Ambiguity analytics — which providers, which targets, how often |
| 3.8 | Tenant-aware ambiguity visibility (foundations only) |

## Phase 3.0 Closure Criteria

For this doc:

1. The five ambiguity sources S-1..S-5 are sealed.
2. The Ambiguity type sketch is the API spec for Phase 3.0
   implementation.
3. The storage schema is recorded; migration is Phase 3.0/3.1.
4. Propagation rules P-1..P-6 are sealed.
5. Capability-aware tuning rule is sealed.

## Related

- [[phase3/provider-capability-matrix]] — `UncertaintyP99`, semantic profiles
- [[phase3/lifecycle-normalization-model]] — N-8 (canonical mapping ambiguity)
- [[phase3/provider-normalization-rules]] — AMBIGUATE treatment
- [[phase3/provider-contract-evolution]] — `MapProviderState` returns `ambiguous`
- [[phase3/multi-provider-risk-analysis]] — R-1, R-2, R-8 mitigation
- [[../phase2.9/operational-guarantee-matrix]] — G-14 (truthful unknown), G-6 (suspicion)
