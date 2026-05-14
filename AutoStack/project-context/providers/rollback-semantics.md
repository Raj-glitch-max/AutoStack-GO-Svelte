# Rollback Semantics — What Works, What Doesn't

## Last Updated
2026-05-13 (Phase 1.9 principal review)

## Current state: rollback is REFUSED

`Provider.Rollback` for Cloud Run now returns `providers.ErrNotImplemented`.
Earlier implementations are removed. Calling Rollback today produces an
honest "not implemented" error, never a destructive partial change.

## Why the previous implementation was unsafe

Located at `pkg/providers/cloudrun/provider.go` (now removed). The
previous body:

1. Constructed `rollbackService := &runpb.Service{Template:
   &runpb.RevisionTemplate{}}` — an EMPTY service with no Name, no
   Containers, no Traffic.
2. Posted that empty service via `UpdateService`. If accepted, the
   service would be wiped to defaults.
3. Picked `revisions[1].Name` from `ListRevisions` as the "previous"
   revision. `ListRevisions` order is provider-defined; index 1 is not
   "previous."
4. Computed `revisionToUse` and never used it in the update call.
5. Reported the un-rolled-back state as a successful rollback by
   threading `waitForServiceReady`'s status into the result.
6. Contained a duplicate `if len(revisions) < 2` block, signaling
   the file was edited without a re-read.

## What a correct Cloud Run rollback requires

Cloud Run v2 rollback is **traffic-target manipulation**, not a service
overwrite:

```go
service.Traffic = []*runpb.TrafficTarget{
  {
    Type:     runpb.TrafficTargetAllocationType_TRAFFIC_TARGET_ALLOCATION_TYPE_REVISION,
    Revision: previousRevisionName,
    Percent:  100,
  },
}
client.UpdateService(ctx, &runpb.UpdateServiceRequest{Service: service})
```

This requires that AutoStack persist revision lineage on
`deployment_targets` before invoking it:

- `current_revision: string`
- `previous_revision: string`
- (optionally) `revision_history: json`

None of these columns exist today.

## What gets recorded today

Nothing. `deployment_history` migration adds the collection but no code
writes to it. A rollback that worked would be invisible to operators.

## Rollback hazards a future implementation must address

1. **Idempotency.** Calling Rollback twice should not flip back-and-forth.
   Persist the intended `target_revision` first; if subsequent call sees
   the service already pointing at that revision, return no-op success.
2. **Concurrency with active Deploy.** A rollback during an in-flight
   Deploy must either fail fast or coordinate via an operation record.
   No operations collection exists today.
3. **Lineage integrity.** Persist both `from_revision` and `to_revision`
   in `deployment_history` so the history is auditable.
4. **Eventual consistency.** Traffic shifts can take 30-60 seconds to
   propagate. Returning "success" from the API call ≠ traffic actually
   shifted.

## Related
- [[lifecycle-assumptions]]
- [[eventual-consistency-assumptions]]
- [[provider-limitations]]
- [[deferred-operational-hardening]]
