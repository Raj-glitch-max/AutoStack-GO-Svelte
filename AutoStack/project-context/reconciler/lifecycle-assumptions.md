# Deployment Lifecycle — Runtime Assumptions

## Last Updated
2026-05-13 (Phase 1.9 principal review)

## Purpose
state-model.md describes intent. This document records what the runtime
actually does, where it diverges, and which divergences are deliberate.

## What the runtime does today

| Subsystem | Behavior |
|---|---|
| `HandleRolloutCreate` (`pkg/controller/rollouts.go:64`) | Calls `k8s.CreateOrUpdateRollout` unconditionally — for ALL rollouts including `target_type ∈ {ecs, cloudrun, aca}`. There is no cloud-Deploy dispatch path. |
| Reconciler `reconcileAll` | Polls `GetStatus` for every `deployment_targets` row whose joined `rollouts.target_type != 'kubernetes'`. |
| Reconciler `reconcileOne` | Writes the provider-reported status to `deployment_targets.status`, gated by `isAllowedTransition`. Does NOT call Deploy, Rollback, or Destroy. |
| Provider `Deploy` | Defined; **never called**. Cloud deployments are not initiated by any code path. |
| Provider `Rollback` | Returns `ErrNotImplemented` (Phase 1.9 fix). |
| Provider `GetOperation` | Returns `ErrNotImplemented` (Phase 1.9 fix). |
| Provider `GetMetrics`, `CheckQuotas` | Return `ErrNotImplemented` (Phase 1.9 fix). |
| Provider `Destroy` | Implemented; idempotent on `NOT_FOUND`. |

## Divergences from state-model.md

1. **`pending → creating`** edge is unreachable. No code calls Deploy.
2. **`creating → running` / `creating → error`** edges are reachable only via
   `GetStatus` poll observation, not via Deploy completion.
3. **`updating`** is documented but no code produces it. `GetStatus`
   collapses revision-in-progress into `creating`.
4. **Stuck-state detection** (state-model.md §"Stuck State Detection") is not
   implemented. No `last_state_change_at` column exists.
5. **Drift detection** (`deployment_targets.drift_detected`) is not
   implemented. Reconciler never compares spec vs. actual provider state.

## Transition guard (Phase 1.9, enforced in code)

`isAllowedTransition(previous, next)` in `pkg/reconciler/cloud.go`:

- Empty previous → permit any. (First observation.)
- `previous == next` → permit. (No-op.)
- `previous == "deleted"` → refuse. (Terminal.)
- `previous ∈ {running, updating}` and `next ∈ {pending, creating}` → refuse.
  (Treat a single inconclusive observation as a flap, not a regression.)
- All other transitions → permit.

Refused transitions still touch `last_synced` so operators can see the
reconciler ran. They are logged with the tag `[TRANSITION_REFUSED]`.

This is a tripwire, not a full FSM. Refining it is Phase 2 work — see
[[deferred-operational-hardening]].

## Rollout-level status field

`updateRolloutStatus` was REMOVED in Phase 1.9 because the `rollouts`
collection has no `status` or `last_deployed` column (no migration adds them),
and PocketBase silently drops `Set` calls for unknown fields. The previous
code wrote a phantom-success signal: logs claimed a transition, but the
persisted record never changed.

To restore rollout-level status, add the columns via a migration first.
Until then, the only authoritative status surface is `deployment_targets`.

## "Unknown" status semantics (Phase 1.9)

Cloud Run `GetStatus` returns `"unknown"` when the service has no `Ready`
condition and no `ConfigurationsReady = RECONCILING`. The reconciler does
NOT persist `"unknown"` (it is not in the select enum). It logs
`[STATUS_UNKNOWN]` and touches `last_synced`. This replaces the previous
behavior of defaulting silently to `"pending"`, which combined with the
transition guard could mislead operators or get refused without an
explanation.

## Related
- [[reconciliation-guarantees]]
- [[rollback-semantics]]
- [[dangerous-edge-cases]]
- [[deferred-operational-hardening]]
