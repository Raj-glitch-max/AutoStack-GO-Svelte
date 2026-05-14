# Rollback Survivability Assessment — Phase 2.4

## Last Updated
2026-05-14

## Current state

`Provider.Rollback` for Cloud Run returns `ErrNotImplemented`. No
dispatch path exists. `operations.kind="rollback"` is a valid enum
value but no caller emits one. **Rollback does not survive anything
because rollback does not exist.**

## What this assessment is

Phase 2.3 already documented the design prerequisites for a future
rollback implementation in
[[../phase2.3/rollback-integrity-assessment]]. This Phase 2.4 doc
extends that with **survivability** properties — what the design must
ensure under chaos.

## Survivability requirements for a future rollback

### R-S1: Rollback interrupted by restart

**Requirement:** A rollback in flight when the process restarts must
either complete on the next process startup OR be marked abandoned
with truthful state.

**Design:**
- Open `operations` row with `kind="rollback"` before the provider call,
  same CAS pattern as Deploy.
- Heartbeat sidecar refreshes `updated_at`.
- Startup sweep with heartbeat-aware policy (Phase 2.3) preserves a
  recently-heartbeated rollback op.
- A truly abandoned rollback (>2× heartbeat window since last heartbeat)
  is marked failed, target → error, operator manually inspects
  provider-side traffic state.

### R-S2: Rollback replay after provider lag

**Requirement:** A re-dispatched rollback after restart must not corrupt
the traffic configuration.

**Design:**
- Pre-check: read current `Service.Traffic`. If 100% already on the
  target revision, the rollback is a no-op success.
- If not, post the same traffic shift again. Cloud Run's
  UpdateService is idempotent for traffic-percent semantics.

### R-S3: Rollback retry semantics

**Requirement:** A failed rollback must be retryable without
double-shifting traffic.

**Design:** Same idempotency as R-S2. Pre-check on every attempt.

### R-S4: Rollback lineage correctness

**Requirement:** Every rollback must record `from_revision`
(current active) and `to_revision` (target revision), with
`status` reflecting the actual outcome.

**Design:**
- History row at intent boundary: `action=rolled_back`,
  `status=in_progress`, `from_revision=X`, `to_revision=Y`.
- Outcome row at dispatcher completion: same action, `status=success`
  or `failed`, message includes traffic-shift verification result.

### R-S5: Rollback during stale provider state

**Requirement:** A rollback to a revision that Cloud Run has GC'd
must fail clearly.

**Design:**
- Pre-check: ListRevisions, verify target revision exists.
- If not, fail with `revision_not_found` (operator can roll back to a
  different revision).

### R-S6: Rollback convergence truth

**Requirement:** A rollback that "succeeded" at the API level must not
report success before Cloud Run actually shifted 100% of traffic.

**Design:**
- Poll `Service.Traffic` after UpdateService.
- Loop until all `TrafficTarget` with `Type=REVISION` have
  `Revision=target` and `Percent=100`. Bounded by 60s timeout.
- Only mark `succeeded` after this convergence check passes.

### R-S7: Rollback idempotency

**Requirement:** Multiple operator-triggered rollbacks for the same
target_revision must not flip back and forth.

**Design:**
- The dispatcher's pre-check returns "no-op success" if traffic is
  already where it should be.
- The reconciler's CAS prevents concurrent rollbacks.

### R-S8: Rollback during in-flight deploy

**Requirement:** Operator requests rollback while a deploy is mid-flight.

**Design:**
- CAS claim refuses (target has `current_operation` set).
- Operator gets a 409 "operation in progress; retry after completion".
- Rollback intent could be persisted as `pending_rollback` flag,
  consumed by the dispatcher's release path (same pattern as Phase 2.3
  `pending_destroy`). But this is over-engineering for a phase-2.5
  feature; defer.

### R-S9: Rollback during destroy

**Requirement:** A target in `deleting` should not accept rollback
requests.

**Design:**
- Rollback's CAS predicate: `status IN ('running', 'updating')`. A
  `deleting` target is not eligible.
- Operator gets a clear refusal.

### R-S10: Rollback after partial deploy success

**Requirement:** A deploy that landed a new revision but the revision
failed Ready: the active traffic is on the previous (good) revision.
Operator wants to "roll back the failed deploy" — what does that mean?

**Interpretation A:** Roll back to the revision currently serving
traffic (the previous good revision). This is a no-op since traffic is
already there.

**Interpretation B:** Delete the failed revision so it doesn't appear
in the revision list. Cloud Run supports `DeleteRevision`. This is not
a rollback — it's a revision GC.

**Design:** Rollback API accepts a target_revision parameter; operator
specifies. The dispatcher uses the documented semantics. No magic
"undo last deploy" UX.

## Operational survivability score

Across the 10 survivability requirements:

- R-S1 through R-S9: have clear designs that follow Phase 2.3 patterns.
- R-S10: requires explicit UX-level decision; deferred.

A future rollback implementation is **achievable** within the
established control-plane patterns. No fundamental architectural
change required.

## Verdict for Phase 2.4

**Rollback remains `ErrNotImplemented`.** This is the right state for
Phase 2.4. Implementing rollback requires:
- Schema additions (`previous_revision` on target row, possibly a
  `revision_history` json column).
- Operator-facing HTTP endpoint(s).
- Provider implementation (Cloud Run + future ECS/ACA).
- Frontend UX.

That is Phase 2.5 / Phase 3 work. The design here documents the
survivability properties so the future implementation can be checked
against them.

## Related
- [[../phase2.3/rollback-integrity-assessment]]
- [[../providers/rollback-semantics]]
