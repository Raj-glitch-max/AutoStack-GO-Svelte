# Operation Ownership — Phase 2.0

## Last Updated
2026-05-14

## Single-ownership invariant

The goroutine that **opens an operations row** owns it for that row's
entire lifetime, until that row reaches a terminal status (`succeeded`,
`succeeded_stale`, `failed`, `cancelled`).

No other goroutine writes to a row owned by someone else, with **one**
explicit exception: the crash-recovery sweep, which runs once at startup
before the reconciler ticker starts, and only against operations whose
`updated_at` heartbeat has stalled past `abandonedOpThreshold`.

## Lifecycle states

```
pending      → (currently unused — operations are created in_progress)
in_progress  → succeeded | succeeded_stale | failed | cancelled
succeeded    → terminal
succeeded_stale → terminal (deploy completed; desired state moved during run)
failed       → terminal
cancelled    → terminal
```

Terminal transitions never reverse. The DAO has no rule preventing it,
but no code path writes a terminal → in_progress transition. If you
need to retry a failed operation, create a new operation row.

## Ownership boundaries

| Concern | Owner | Notes |
|---|---|---|
| Operation creation | `Reconciler.createOperation` | Single call per dispatch attempt |
| CAS claim | `Reconciler.claimTarget` | Operation ID is the lock token |
| Provider.Deploy invocation | `Reconciler.dispatchDeploy` | Inside the dispatcher goroutine only |
| Status transitions during run | `Reconciler.completeOperation` | Same goroutine |
| Heartbeat (updated_at refresh) | Not yet implemented | Phase 2.5 — see hazards |
| Crash-recovery sweep | `Reconciler.SweepAbandonedOperations` | Startup-only; runs synchronously |
| Cancellation on CAS race | `Reconciler.cancelOperation` | Same goroutine that lost the race |
| Operation expiry / GC | Not implemented | operations is append-only; archival is a future concern |

## Why we forbid auto-retry from `error`

Phase 2.0 does NOT auto-retry a target stuck in status `error`. The
circuit breaker keeps the failure count; reaching the threshold causes
the target to be skipped entirely.

To re-deploy after an error:
1. Operator inspects `deployment_targets.drift_summary` and `operations`
   history for the failure reason.
2. Operator fixes the root cause (credentials, manifest, quota).
3. Operator creates a fresh rollout. The new rollout creates a new
   `deployment_targets` row in `pending`, and the dispatcher picks it
   up cleanly.

Auto-retry from `error` is rejected for Phase 2.0 because:
- We cannot distinguish "transient" from "structural" failures
  reliably from row state.
- A retry loop on a quota / auth failure burns provider quota for
  nothing.
- Truthful-state reporting demands that an operator-visible `error`
  status mean *operator action required*, not *we'll retry quietly*.

## Hazards under single-pod operation

1. **Stale dispatcher beyond sweep threshold.** A deploy that genuinely
   takes longer than 20 minutes will trigger the sweep, marking its op
   `failed` while the goroutine is still running. The goroutine will
   then call `releaseTarget` (or `completeOperation`) on a row the
   sweep already updated. The release will overwrite the sweep's state.
   This is a known race, accepted for Phase 2.0 — current Cloud Run
   deploys are bounded at 15 minutes by `DeployTimeout`. Phase 2.5
   should add a "release-only-if-still-owner" predicate.

2. **PocketBase admin manual edit.** An operator who edits the
   `operations` table directly through PocketBase admin can break the
   ownership invariant. Document; do not defend against it
   programmatically.

## Hazards under multi-pod operation (currently unsupported)

3. **Two reconcilers race the CAS claim.** Safe — one wins, one cancels
   its own operation.

4. **Two reconcilers race the sweep.** A pod restarting while another
   pod's reconciler is running could run the sweep against a still-live
   operation. The threshold is the only defense. Phase 2.5 should add
   pod-identity stamping on operations to refuse sweep across pods.

5. **Lost lease.** If a pod loses connectivity to PocketBase mid-deploy
   and another pod's sweep marks the op abandoned, the original pod
   can still complete its provider call. The provider state and
   AutoStack state diverge. Phase 2.5 lease/heartbeat work addresses
   this.

## Related
- [[deploy-dispatch-design]]
- [[restart-behavior]]
- [[reconciliation-guarantees]]
- [[deferred-operational-hardening]]
