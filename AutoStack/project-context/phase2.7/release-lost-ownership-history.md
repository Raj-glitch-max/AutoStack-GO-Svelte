# Release-Lost-Ownership History — Phase 2.7

## Last Updated
2026-05-14

## Problem

When the dispatcher's release-CAS finds 0 rows affected (sweep
reclaimed the op while the dispatcher was mid-flight), today's
behavior is:

- `[RELEASE_LOST_OWNERSHIP]` log emitted.
- No `deployment_history` row written by the dispatcher.

The sweep's `writeAbandonHistory` did record a failed history row at
reclaim. But the **dispatcher's actual provider observation** —
whether the Deploy succeeded provider-side, whether it failed, what
the external_id was — is lost from history.

## Fix

When `releaseTargetWithExternal` matches 0 rows, write a forensic
history row capturing the dispatcher's outcome with an explicit tag
indicating ownership was lost.

### Schema use

- `action`: matches the dispatcher's intent (created/updated/deleted).
- `status`: `failed` (the dispatcher's intent never landed on disk).
- `message`: explicit — "dispatcher returned but sweep had reclaimed
  ownership; observed_outcome=<success|failed>; external_id=<id>".
- `to_revision`: the external_id, if the dispatcher had one.

### Forensic semantics

Two history rows result from this sequence:
1. The sweep's row: action=error/deleted, status=failed,
   message="abandoned: heartbeat went stale".
2. The dispatcher's row (post-Phase-2.7): action=created/etc.,
   status=failed, message="dispatcher returned ... observed_outcome=success
   external_id=foo".

Operators see BOTH rows and can deduce:
- The sweep classified the op as abandoned.
- The dispatcher actually completed the provider call.
- The provider state may not match AutoStack's record.
- Manual verification (GCP console) is needed.

## Implementation

`releaseTargetWithExternal` already detects the 0-rows case and logs.
Extend it to call a new `writeOwnershipLostHistory` helper. The helper
needs the operation's kind, the dispatcher's outcome (success vs
failed), the external_id, and the rollout_id.

Pass these into the release function. Easiest: change
`releaseTargetWithExternal` to accept `(rolloutID, action, outcome,
externalID)` for the history-write path.

## Hazards considered

### H-1: Writing history for an op the sweep already wrote history for

Both rows are valid forensic data. They tell different stories:
- Sweep row: "I observed this op was stale".
- Dispatcher row: "Here's what I actually saw provider-side".

Operators reading both get more truth, not less. ✓

### H-2: History-write failure during this rare path

Logged at `[HISTORY_WRITE_ERR]`. The release-CAS already failed; we
can't even update last_synced. The op is in a known terminal state
via the sweep. Forensic gap is acceptable for the rare-rare case.

### H-3: External_id may be empty if release fired before Deploy
returned

Possible if a panic occurred before the result was captured. The
history row would have empty external_id. Acceptable — operators
correlate via cycle_id from logs.

## Related
- [[../phase2.4/lifecycle-closure-integrity-review]] LC-10
- [[../phase2.3/lineage-integrity-review]] (Phase 2.5 will add
  history.operation FK)
