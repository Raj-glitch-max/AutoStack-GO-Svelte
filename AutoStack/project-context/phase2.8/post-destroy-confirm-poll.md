# Post-Destroy NOT_FOUND Confirmation Poll — Phase 2.8

## Last Updated
2026-05-14

## Problem

`Provider.Destroy` returns nil once Cloud Run's `DeleteService` returns
200 OK. Cloud Run's deletion is asynchronous server-side; `GetService`
may continue to return the service for 10-60s after.

The dispatcher's success branch marks `deployment_targets.status =
'deleted'` immediately. The operator-facing truth says "deleted" while
the provider may still list the service briefly.

## Fix

After `DeleteService` returns 200, the provider polls `GetService`
every 5s until either:
- NOT_FOUND is observed (truthful "gone"), OR
- 60s timeout fires (provider may still be cleaning up; we mark the
  delete provisionally-successful with an explicit message indicating
  the timeout).

## Why poll vs. just trust the 200

- Cloud Run's deletion is best-effort eventually-consistent. A 200
  means "request accepted" not "resource gone."
- Operators relying on "deleted" status to trigger rollout-delete
  (cascade) would succeed before provider-side cleanup completes.
- Compliance / cost-tracking contexts need truthful "gone" semantics.

## Why 60s timeout

- Typical Cloud Run delete propagation: under 10s in normal operation.
- 60s gives a 6× margin without indefinitely blocking the dispatcher.
- The dispatcher's parent `DeployTimeout` is 15 min, so 60s confirm
  fits comfortably within budget.

## Outcome semantics

| Outcome | dispatcher branch | target status |
|---|---|---|
| `DeleteService` 200 + `GetService` NOT_FOUND within 60s | success | deleted |
| `DeleteService` 200 + 60s timeout (provider still lists) | success-with-warning | deleted (with `drift_summary` noting "delete confirmed via API; provider visibility lag exceeded confirm window — verify via console") |
| `DeleteService` returns error | failure | error |
| `DeleteService` returns NOT_FOUND (idempotent — already gone) | success | deleted |

## Implementation

Modify `cloudrun.Provider.Destroy`:

1. After successful `DeleteService`, loop `GetService` every 5s.
2. On NOT_FOUND: return nil.
3. On other error from `GetService`: log, return nil (we already
   committed to delete; transient errors during confirmation are
   acceptable).
4. On 60s timeout: return nil with no error but log a
   `[DESTROY_CONFIRM_TIMEOUT]` warning.

The dispatcher's `dispatchDestroy` success branch doesn't need to
change — `Destroy` returning nil still means "we're done."

## Hazards considered

### Pole-timeout false-positive

If the operator's GCP project has propagation delay > 60s, every
destroy confirms slowly. **Mitigation:** the 60s timeout is configurable
via `AUTOSTACK_CR_DESTROY_CONFIRM_TIMEOUT_SECONDS` env var (default
60). The target is marked `deleted` either way; only the message
changes.

### Confirm-loop blocks dispatcher

The dispatcher's 15-min DeployTimeout is much larger than the 60s
confirm. No risk of dispatcher exceeding its budget for a destroy.

### Idempotency on retry

If destroy is re-dispatched (operator clears error, sets endDate
again), the next call:
- `GetService` returns NOT_FOUND → idempotent success → no poll needed.

✓ Safe.

## Verdict

Lands in Phase 2.8.

## Related
- [[../phase2.3/eventual-consistency-hazards]] E-3
- [[../phase2.3/delete-orphan-risk-assessment]] D-2
