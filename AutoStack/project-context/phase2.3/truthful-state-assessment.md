# Truthful-State Assessment — Phase 2.3

## Last Updated
2026-05-14

## The truthfulness contract

The control plane MUST prefer:

> truthful uncertainty over false confidence.

Every status surface — `deployment_targets.status`,
`operations.status`, `deployment_history.status`, log emissions — must
report what is observed, not what was hoped.

## Truth-claims the system makes

| Claim | Where | Truthful today? |
|---|---|---|
| "Cloud Run service is `running`" | deployment_targets.status=running | ✓ Only set by a status-poll observing Ready=SUCCEEDED. Dispatcher post-success deliberately persists `updating` first; promotion to `running` requires a separate observation. |
| "Cloud Run service is `creating`" | deployment_targets.status=creating | ✓ Set by CAS claim. Means "we own this and are deploying right now." Once dispatcher returns successfully, status moves to `updating` (not `creating`) → eventually `running`. |
| "Cloud Run service is `error`" | deployment_targets.status=error | ✓ Set by hard provider error, deploy failure result, or status-poll observing Ready=FAILED twice (suspicion counter). Single transient FAILED from `updating` is held; second confirms. |
| "Cloud Run service is `deleted`" | deployment_targets.status=deleted | ⚠️ Set after Provider.Destroy returns nil. Cloud Run's `DeleteService` returns asynchronously — provider may still list the service for tens of seconds. We mark `deleted` based on the API return, not on observed disappearance. **Minor lie window** — see EC hazard E-3 in [[eventual-consistency-hazards]]. |
| "Operation is `succeeded`" | operations.status=succeeded | ✓ Only on dispatcher's clean success branch. Sweep cannot upgrade `in_progress → succeeded`; only the owning dispatcher can. |
| "Operation is `succeeded_stale`" | operations.status=succeeded_stale | ✓ Honest middle-ground: "Deploy returned successfully but the spec moved during the call." |
| "Operation is `failed: abandoned: process restart while in flight`" | operations.status=failed (sweep) | ✓ Sweep never claims success. Always marks abandoned ops as failed. |
| "Deployment history success" | deployment_history.status=success | ✓ Only written on dispatcher's clean success branch. |
| "Deployment is reconciling on schedule" | log `[RECONCILE] cycle_complete` | ✓ Includes target count + duration. |

## Truth-claims the system does NOT make (correctly refused)

| Claim | Refused via |
|---|---|
| Rollback succeeded | Rollback returns `ErrNotImplemented` |
| Real metrics | GetMetrics returns `ErrNotImplemented` |
| Real LRO tracking | GetOperation returns `ErrNotImplemented` |
| Quota available | CheckQuotas returns `ErrNotImplemented` |
| GetActualCost | returns "not yet implemented" |
| log streaming | StreamLogs returns "not yet implemented" |
| `unknown` provider status mapped to a concrete enum | reconciler skips persistence, touches `last_synced` only |
| Single-observation regression of running/updating | transition guard refuses `running → pending`, `running → creating`, `updating → pending`, `updating → creating` |
| Single-observation flap of `updating → error` | suspicion counter requires 2 consecutive errors before persistence |

## Truth gaps identified

### T-1: `deleted` claimed before provider confirms

**Scenario:** Dispatcher calls `Provider.Destroy`, which returns nil
once `DeleteService` returns 200. Cloud Run's API contract is "request
accepted, deletion in progress." The service may continue to be listed
for 10-60 seconds.

**Current truth claim:** `deployment_targets.status = deleted`,
`deployment_history.status = success`, op `succeeded`.

**Actual provider state:** Possibly still `running` from the API's
perspective for tens of seconds.

**Severity:** Low. The only consumer of `deleted` status is the orphan
check in `HandleRolloutDelete`, which now allows rollout-delete only
after target reaches `deleted`. An operator hitting delete immediately
after `deleted` flips could (in principle) succeed at deleting the
rollout while the service still exists provider-side — but the
rollout-delete cascade would remove deployment_history but not affect
the service. Eventual consistency closes the gap within seconds.

**Mitigation:** Post-destroy NOT_FOUND confirmation poll. Deferred to
Phase 2.5 (see [[../known-issues/orphan-defense-policy]]).

### T-2: `deployment_history.status=success` written before provider readiness convergence

**Scenario:** Dispatcher's success branch fires when `Provider.Deploy`
returns with a result and no error. For Cloud Run, this means
`waitForServiceReady` saw `Ready=SUCCEEDED`. But Cloud Run can
**briefly** report `Ready=SUCCEEDED` for an old revision before traffic
shifts to a new one — the "transient SUCCEEDED flap" documented in
[[../providers/eventual-consistency-assumptions]].

**Current truth claim:** History row says `status=success`. Target
status persists `updating` (not `running`), so the target row is honest
that convergence isn't proven yet.

**Severity:** Acceptable. History is a record of "the API call
completed successfully"; the target status is the operator-facing
source of truth, and it correctly holds `updating` until the next
status-poll promotes to `running`.

**No fix needed.**

### T-3: `error` claimed on auth failure may be permanent or transient

**Scenario:** A credential rotation in GCP invalidates the service
account. Provider call returns 401. ClassifyError returns `auth`. The
target → `error` with message="auth: ..." and no retry.

**Current truth claim:** Target is in `error` requiring operator
intervention.

**Truthfulness:** Correct — this IS auth failure. But the message
doesn't distinguish "creds bad" from "creds bad in this region" — the
latter is potentially fixable by changing region rather than rotating
creds.

**Mitigation:** Region-scoped credential validation (deferred —
[[../known-issues/deferred-operational-hardening]] Tier 4).

### T-4: `STATUS_UNKNOWN` is logged but no PocketBase-visible signal

The reconciler emits `[STATUS_UNKNOWN]` when GetStatus returns
"unknown", touches `last_synced` but doesn't persist a status value.
Operators looking at the PocketBase UI see the target with the previous
status and an updated `last_synced` — no visible signal that
observation was inconclusive.

**Severity:** Low. The previous status is still the most honest one
(transition guard refuses regression). The UI silently shows
"last seen 1 minute ago" which is honest.

**Phase 2.5 work:** Add `deployment_targets.last_observation_kind`
(succeeded/refused/unknown). Deferred.

### T-5: `STATE_TRANSITION` log on first observation lies about previous

`reconcileOne` logs `[STATE_TRANSITION] from= to=running` when
`previousStatus` is empty (first poll). The "from=" field is empty,
which a reader could mis-parse as "transitioned from nothing."
Acceptable but worth a comment.

**Severity:** Negligible.

### T-6: Dispatcher panic recovery writes `status=failed` without checking provider-side state

If `dispatchDeploy`'s panic-recovery defer fires, we cannot know
whether `Provider.Deploy` had already made provider-side changes. We
mark op `failed`, target `error`, history `failed`. The provider may
have actually created the service. **The on-disk state lies in this
edge case** — until the next reconcile observes the service via
GetStatus, AutoStack reports `error` for a service that exists.

**Severity:** Low. Operator-visible signal is "error" — operator
investigates, finds the service exists, decides recovery. The
transition guard then allows `error → creating` (via re-deploy intent
through end-rollout-and-recreate workflow), but a more elegant recovery
would auto-promote `error → running` if next GetStatus observes
Ready=SUCCEEDED.

**Today:** Transition guard ALLOWS error→running, error→creating,
error→updating. So on first successful status-poll after the panic,
target promotes correctly.

**No fix needed.**

### T-7: `succeeded_stale` op + `pending` target = "deploy succeeded but we're going to do it again"

This is the truthful answer to "what happened?" but operators reading
the operations table will see "succeeded_stale" and may be confused why
the target is `pending` rather than `running`/`updating`.

**Severity:** Low. The message field on `succeeded_stale` ops says
"rollout updated during deploy; re-dispatching next cycle" — adequate
operator guidance.

### T-8: `cycle_id` invisible after Phase 2.0 dispatch entry

Phase 2.1 added `cycle_id` to reconciler logs but dispatcher logs
(`[DISPATCH_CLAIM]`, `[DEPLOY_START]`, `[DEPLOY_END]`) do NOT carry it.
A 3-AM operator greping a target's logs across components cannot
correlate "this deploy ran in cycle X" — they have to chase by
timestamp.

**Truthfulness impact:** This is observability, not state truthfulness,
but it eats into the "operators can reconstruct what happened" surface.

**Fix:** Landing in Phase 2.3 — thread cycle_id through dispatch logs.

## Phase 2.3 implementation in this area

- T-8: Thread cycle_id into dispatcher logs.
- T-3 partial mitigation: keep `auth` category honest; defer
  region-scoped validation.

## Deferred (intentional)

- T-1 (post-destroy confirm poll): Phase 2.5
- T-4 (last_observation_kind): Phase 2.5
- T-6 (panic-recovery state auto-promotion): no clear better behavior
  than what the transition guard already enables on next poll.

## Related
- [[lineage-integrity-review]]
- [[eventual-consistency-hazards]]
- [[../known-issues/dangerous-edge-cases]]
- [[../known-issues/correctness-limitations]]
