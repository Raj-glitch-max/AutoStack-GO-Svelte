# Succeeded-Stale Loop Guard — Phase 2.6 Design

## Last Updated
2026-05-14

## Problem

A respec-flapping rollout perpetually re-dispatches:

```
T+0  deploy starts (rev=A)
T+5  rollout updated to rev=B (during deploy)
T+10 deploy finishes; rolloutMovedSince=true; op=succeeded_stale; target=pending
T+11 deploy starts again (rev=B)
T+15 rollout updated to rev=C (during deploy)
T+20 deploy finishes; succeeded_stale; target=pending
...
```

Each cycle burns ~10 minutes of Cloud Run quota. Circuit breaker
doesn't engage because `succeeded_stale` isn't a failure.

## Design

Track consecutive `succeeded_stale` outcomes per target. After
threshold N (3), refuse further dispatch and surface as `error` with
a clear message.

### State

In-memory map `Reconciler.staleCount: map[targetID]int`. Cleared on
non-stale outcomes (success, hard failure, restart).

### Lifecycle

- On `succeeded_stale` outcome: `staleCount[targetID]++`.
- If `staleCount[targetID] >= 3`: dispatcher releases target to
  `error` (instead of `pending`) with message "pathological stale-spec
  loop detected; operator action required". Clear staleCount.
- On any other outcome (`success`, `failed`, `error`): clear
  staleCount.

### Operator recovery

Same as any `error` target: respec the rollout. Phase 2.4 in-memory
state clearing also clears `staleCount` when target enters `pending`.

### Threshold rationale

- N=1: too aggressive; a single fast-follower respec is legitimate.
- N=3: gives operators 3 consecutive attempts (~30 min of provider work)
  before halting; sufficient to catch automation pathology.
- N=5: too lenient; 50 min of provider quota burned before catch.

## Implementation

1. Add `staleCount` field to `Reconciler` struct.
2. In `dispatchDeploy`'s `stale` branch:
   - Increment `staleCount[targetID]`.
   - If >= 3, write history with explicit message, release target to
     `error` (not `pending`), clear count.
   - Else, release to `pending` as before.
3. In success/failure branches and in the pending-clear pass (Phase
   2.4 M-2): also clear `staleCount[targetID]`.

## Behavior summary

| Outcome | Action |
|---|---|
| First stale | staleCount=1, release pending. |
| Second stale | staleCount=2, release pending. |
| Third stale | staleCount=3, **release ERROR with "stale loop detected".** |
| Operator respecs | target → pending; cleared at entry to dispatch path. |
| Any non-stale outcome | clear count. |

## Hazards explicitly accepted

- **Restart loses count.** In-memory only. Process restart resets to 0.
  After 3 stales post-restart, the guard fires. Acceptable: the cost is
  3 more cycles, not a wrong outcome.
- **Single-pod scope.** Multi-pod with separate state would lose the
  signal. Mitigated by single-pod constraint (documented).

## Related
- [[../phase2.4/reconciliation-convergence-assessment]] C-1
- [[../phase2.4/retry-amplification-review]] A-4
