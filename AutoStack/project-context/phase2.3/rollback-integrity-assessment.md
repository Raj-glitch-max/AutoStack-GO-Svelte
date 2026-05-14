# Rollback Integrity Assessment — Phase 2.3

## Last Updated
2026-05-14

## Current state

`Provider.Rollback` for Cloud Run returns `providers.ErrNotImplemented`.
No caller invokes it. `operations.kind` allows `"rollback"` but no
dispatch path emits one. `deployment_history.action` allows
`"rolled_back"` but only the sweep writes that value (when reconstructing
an abandoned rollback op's history, which itself can never have been
written today).

**Rollback is honestly refused.** This is the correct posture per the
directive ("truthfulness over optimistic UX") — better than a phantom
rollback path that pretends to work.

## What the system would need before a real rollback

### Required schema additions

- `deployment_targets.previous_revision` (text) — revision name to roll
  back TO. Set on every successful deploy = old `current_revision`.
- `deployment_targets.revision_history` (json, optional) — last N
  revisions for multi-step rollback. Or store ordered history in
  `deployment_history` only.
- `operations.kind = 'rollback'` is already in the enum.

### Required code paths

- `Reconciler.dispatchRollback`: CAS-claim, open op, call
  `Provider.Rollback(ctx, account, target, target.previous_revision)`,
  handle outcome.
- `HandleRolloutRollback` (HTTP): operator endpoint POST
  `/api/v1/rollouts/:id/rollback` that flips
  `deployment_targets.status = 'rolling_back'` and the dispatcher picks
  it up. Or a `rollback_requested: bool` flag.
- `Provider.Rollback` (Cloud Run): use
  `Service.Traffic = [{Revision: previous, Percent: 100}]` posted via
  `UpdateService`. Wait for the revision swap to complete (poll until
  100% of traffic is on the target revision).

### Required state-model additions

- New `deployment_targets.status` value: `rolling_back`. Behaves like
  `updating` for transition-guard purposes.
- Transition rules: `running → rolling_back → running` on success,
  `→ error` on failure.

### Required lineage additions

- History row at intent (`action=rolled_back`, `status=in_progress`,
  `from_revision=current_revision`,
  `to_revision=previous_revision`).
- History row at outcome (`status=success`/`failed`).

## Truthfulness hazards a future rollback implementation must avoid

### R-1: Phantom success

The pre-1.9 implementation reported success regardless of the actual
result. A correct rollback MUST:

- Verify the API call accepted the request.
- Poll until traffic has actually shifted (Cloud Run Traffic
  reconciliation can take 30-60 seconds).
- Only report success when 100% of traffic is on the target revision.

### R-2: Rollback to a missing revision

Cloud Run automatically garbage-collects unused revisions after a
configurable retention. A rollback to a revision that's been GC'd will
fail with NOT_FOUND. The dispatcher must:

- Read `ListRevisions` and confirm the target revision exists.
- Fail loudly with operator-friendly message if not.

### R-3: Idempotency

Calling Rollback twice with the same target_revision should not flip
traffic back and forth.

- Pre-check: if current traffic is already 100% on target_revision,
  return success no-op.

### R-4: Concurrency with in-flight Deploy

If a Deploy is in-flight and a Rollback is requested:

- The CAS claim refuses (target already has `current_operation`).
- Operator gets a clear error.

But what about the reverse: Deploy requested while Rollback in flight?
Same CAS protection.

### R-5: Cascading rollback to a previously-rolled-back revision

If operator rolls back A→B, then rolls back B→A, the operator has
effectively re-deployed A. History must record both as rollbacks, not
as deploys. Action enum `rolled_back` distinguishes.

### R-6: Replay safety

A rollback interrupted by restart:
- Sweep marks op `failed: abandoned`.
- Target → error.
- Operator must observe whether the traffic shift completed
  provider-side (via Cloud Run console or AutoStack's GetStatus +
  manual revision check).

This is the same replay-safety pattern as Deploy. ✓

### R-7: Rollback during destroy

A target in `deleting` should refuse rollback requests. The CAS
predicate `status IN ('pending', 'deleting')` does NOT include
`running` for the deploy path. A new rollback path would need its own
predicate (`status IN ('running', 'updating')`).

## Integrity properties a future rollback MUST satisfy

| Property | Validation |
|---|---|
| `to_revision` exists provider-side at rollback start | Provider lists revisions before posting traffic shift |
| Operation row created before provider call | Same CAS pattern as Deploy |
| Heartbeat during traffic-shift poll | Same goroutine pattern as Deploy |
| Truthful "success" only after 100% traffic on target | Poll loop with bounded timeout |
| History records both `from_revision` and `to_revision` | Mandatory fields on rollback action |
| `current_revision` updated on success | Atomic with status transition |
| `previous_revision` updated to the rolled-FROM value | So a subsequent rollback re-flips correctly |

## Phase 2.3 verdict

Rollback remains **`ErrNotImplemented` and honestly refused**. No code
change in this area in Phase 2.3.

The design above is the prerequisite for a Phase 2.5 or Phase 3
implementation. Documenting it here so the design isn't re-derived
when work resumes.

## Related
- [[truthful-state-assessment]]
- [[lineage-integrity-review]]
- [[../providers/rollback-semantics]]
