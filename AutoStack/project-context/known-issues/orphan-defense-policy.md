# Cloud Orphan Defense Policy — Phase 2.1

## Last Updated
2026-05-14

## What an orphan is

A "cloud orphan" is a provider-side resource (e.g., a Cloud Run service)
that AutoStack created but no longer tracks in PocketBase. Orphans cost
money, leak credentials, and corrupt audit history.

The Phase 2.0 `cascadeDelete: true` on `deployment_targets.rollout` is
the underlying mechanism: deleting a rollout row cascades to its targets,
removing the AutoStack-side record without touching the provider.

## Phase 2.1 defense

### Hard-delete refusal

`HandleRolloutDelete` REFUSES to delete a cloud rollout that has any
`deployment_targets` row in a non-`deleted` status. Returns HTTP 409:

```
cloud rollout has a live deployment_target. End the rollout
(set endDate) so the reconciler can destroy it, then delete the
rollout after the target reaches status=deleted.
```

The OnRecordBeforeDeleteRequest hook returning an error blocks the
delete at the PocketBase layer, so cascade-delete never fires.

Logged: `[CLOUD_DELETE_REFUSED]` (refused), `[CLOUD_DELETE_ALLOWED]`
(safe).

### Operator workflow for cloud rollout removal

1. PATCH the rollout to set `endDate = <now>`.
2. The rollout-update handler calls `markCloudTargetForDestroy`, which
   sets `deployment_targets.status = "deleting"`.
3. Next reconcile tick: dispatcher CAS-claims and runs Provider.Destroy.
4. Target row reaches `status = "deleted"`.
5. Operator can now DELETE the rollout. Hard-delete refusal no longer
   triggers (no live targets remain).

If step 3 fails (provider error), the target reaches `status = "error"`.
The operator must investigate and either retry by re-setting `endDate`
(if it was somehow cleared) or manually clean up provider-side.

### What this does NOT cover

| Scenario | Today |
|---|---|
| Operator manually deletes the service in GCP console | AutoStack target row stays `running`; reconciler will eventually observe NOT_FOUND, transition to `error`. Operator must then either retry or remove the rollout. |
| Rollout `endDate` set while a deploy is in flight | `markCloudTargetForDestroy` defers (logged `[CLOUD_DESTROY_DEFER]`). Currently the intent CAN be lost if the deploy completes and nothing re-marks `deleting`. **Phase 2.1 gap — see [[deferred-operational-hardening]] Tier 1-post-2.1.** |
| Network partition between AutoStack and Cloud Run during destroy | Destroy returns error; target → `error`. Operator retries via `endDate` change. |
| Provider deletes the service via TTL or owner-deletes the GCP project | AutoStack will keep seeing NOT_FOUND; transition guard refuses `deleted → error`; circuit breaker eventually opens. Acceptable noise; documented. |

## Deferred (Phase 2.5+)

- **Orphan-cleanup scanner.** Periodically list provider resources with
  `autostack-managed=true` label and destroy any that lack a backing
  `deployment_targets` row.
- **Defer-and-rearm for `markCloudTargetForDestroy`.** A
  `deployment_targets.pending_destroy: bool` column would let the
  dispatcher's release path detect that an end-intent was deferred and
  flip status to `deleting` automatically.
- **Pre-destroy confirmation poll.** Provider.Destroy returns once API
  call returns. Service may remain listable for ~30s. A "confirm gone"
  check would tighten the truthfulness window.

## Related
- [[deploy-dispatch-design]]
- [[lifecycle-assumptions]]
- [[deferred-operational-hardening]]
- [[dangerous-edge-cases]]
