# Correctness Limitations — Truthful Inventory

## Last Updated
2026-05-14 (Phase 1.9 principal review)

A control plane is only as good as its honest reporting of its own
limits. This document is the truthful version of what the system can
and cannot do as of Phase 1.9.

## What the system can do

- Create, validate, list, and surface cloud accounts (GCP only).
- Estimate cost with a clearly-flagged placeholder rate set.
- Poll status of Cloud Run services that already exist (whose
  `external_id` was somehow populated — though no AutoStack code path
  populates it).
- Destroy Cloud Run services idempotently.

## What the system cannot do today

| Capability | Why |
|---|---|
| Trigger a Cloud Run deploy from AutoStack | No code path calls `Provider.Deploy`. |
| Rollback any Cloud Run service | Refused with `ErrNotImplemented` — previous body was destructive. |
| Track in-flight provider operations | No `operations` collection; `GetOperation` refuses. |
| Record any deployment action history | `deployment_history` table exists; no writer. |
| Report real metrics | Refused with `ErrNotImplemented`; UI must show "unavailable." |
| Validate quotas before deploy | Refused with `ErrNotImplemented`. |
| Encrypt cloud credentials at rest | Column is named `credentials_encrypted` but stores plaintext. |
| Detect stuck deployments | No timer; no `last_state_change_at` column. |
| Detect drift | `drift_detected` is permanently `false`. |
| Run highly available (multi-pod) | No leader election; no row versioning. |
| Survive a mid-Deploy crash | No operation persistence. (Moot today because no Deploy is triggered.) |
| Confirm a delete actually propagated provider-side | No post-delete confirmation poll. |

## Lies the system used to tell — corrected in Phase 1.9

- `GetMetrics` returned zeros with `nil` error → now refuses.
- `CheckQuotas` returned `Available: true` always → now refuses.
- `Rollback` returned phantom-success on an empty-spec Service update
  → now refuses.
- `GetOperation` returned synthesized operation records based on
  service conditions → now refuses.
- `GetStatus` defaulted to `"pending"` when conditions were
  unrecognizable → now returns `"unknown"`, which the reconciler
  refuses to persist.
- Reconciler wrote `rollouts.status` / `last_deployed` to columns that
  don't exist (PocketBase silently dropped the writes) → now removed.
- `Destroy` predicate used `strings.Contains(uid, "")` (always true)
  → now `existing.Uid != ""`.
- HTTP responses leaked the `credentials_encrypted` column → now
  scrubbed via `sanitizeCloudAccountResponse`.

## What "truthful state reporting" means here

The directive's rule: never report "healthy" when degraded, never report
"deployed" before ready, never report "success" before convergence.

This system enforces that rule by:

- Refusing to claim capabilities not implemented (return
  `ErrNotImplemented`; UI must surface "unavailable").
- Refusing to persist status that the provider gave us ambiguously
  (return `"unknown"`; the reconciler logs and skips persistence).
- Refusing transitions that contradict a healthy prior observation
  (transition guard in `updateTargetStatus`).
- Logging refusals so they are not invisible (`[STATUS_UNKNOWN]`,
  `[TRANSITION_REFUSED]`).

## Open questions for the next phase

- Should `GetStatus` retry once on `"unknown"` within the same cycle
  before persisting? (Would smooth flap; risks doubling API calls.)
- Should refused observations increment a "suspicion" counter that
  eventually forces a re-poll out of cycle? (Operational complexity vs.
  faster detection of true regressions.)
- Should the deploy dispatch path be a synchronous HTTP handler call or
  asynchronous via an operation record? (Synchronous is simpler now;
  async is required for crash survivability.)

## Related
- [[provider-limitations]]
- [[lifecycle-assumptions]]
- [[deferred-operational-hardening]]
- [[control-plane-paranoia-findings]]
