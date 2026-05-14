# Remaining Operational Blockers — Phase 2.3

## Last Updated
2026-05-14

## Definition

An **operational blocker** is something that prevents AutoStack from
being safely operated in a specific deployment context. This is
narrower than "deferred concerns" — these are the items that, if
unaddressed, will cause an operator to lose data, leak cloud cost, or
miscommunicate state.

## Blockers by deployment context

### Single-pod, single-region, single-tenant (today's default)

After Phase 2.3 lands, this context has **no remaining blockers** for
non-rollback workflows.

Known limitations within this context (documented, accepted):

- Rollback not implemented; operator must end + re-create rollout to
  recover from a bad deploy.
- Live metrics not implemented; UI must show "metrics unavailable".
- Cost estimates are static placeholders, not live API.
- Stuck-state detection unimplemented; operator must use PocketBase
  admin to investigate stuck `creating`/`updating` targets after 10+ min.
- Drift detection unimplemented.

### Multi-pod (rolling-restart of the AutoStack pod itself)

After Phase 2.3 lands, rolling restart of the SAME single replica is
**safe** (heartbeat-aware sweep protects the in-flight op of the
outgoing pod from the incoming pod's sweep).

### Multi-pod (two-or-more concurrent replicas)

**BLOCKED.** Cannot be deployed safely until Phase 2.5 pod-identity
stamping lands. Specifically:

- Pod B's startup sweep will mark Pod A's live ops as abandoned.
- Targets pointing at live ops will be reset to `error`.
- Provider-side resources may be in unknown state.

Deployment guidance: **operate as 1 pod with restart-strategy
`Recreate` or `RollingUpdate maxSurge=0`**. Anything more is unsafe.

### Postgres backend

**BLOCKED.** Cannot be migrated until:

- `current_operation = '' OR IS NULL` predicate is normalized to strict
  NULL semantics (Phase 2.5 migration).
- Read-from-primary on `rolloutMovedSince` is enforced (no replica
  reads during dispatcher's stale-spec check).
- WAL-serialized-write-lock assumption is replaced with row-level CAS
  validation (already in place, but needs Postgres-specific test).

These are not blockers for keeping AutoStack on SQLite, which is the
current default.

### Production AutoStack hosting (HA-required)

**BLOCKED** by all of:
- Multi-pod blocker (above).
- KMS key management (Phase 3).
- SSO/SAML (Phase 4).
- Audit log (Phase 3).
- Stuck-state detector (Phase 2.5).
- Orphan-cleanup scanner (Phase 2.5).

This context cannot be supported today.

## Blockers by use case

### Operator wants to roll back a bad deploy

**BLOCKED.** Rollback is `ErrNotImplemented`. Workaround: end the
current rollout, create a new rollout with the previous-known-good
manifest. Slow and operator-heavy but functional.

### Operator wants to delete a cloud rollout safely

**Works.** `HandleRolloutDelete` refuses cascade-delete when targets are
live; operator must end (set endDate) first, wait for `deleted`, then
hard-delete the rollout.

After Phase 2.3 H-1 fix, setting endDate during in-flight deploy no
longer silently drops the destroy intent. ✓

### Operator wants to update the spec of a running cloud rollout

**Works.** `flipCloudTargetsToPendingOnRespec` flips
`running`/`updating`/`error` → `pending` on manifest change. Reconciler
re-dispatches. Cloud Run's revision-update mechanism handles the
swap.

In-flight respec: dispatcher's stale-spec check catches divergence,
releases to `pending`, next cycle re-deploys.

### Operator wants to know what happened during incident X

**Improved by Phase 2.3.** With intent-boundary history rows + cycle-id
in dispatch logs, post-mortem reconstruction is significantly improved.

Remaining gaps:
- `RELEASE_LOST_OWNERSHIP` events leave no history row (Phase 2.5 fix).
- CAS-race cancellations leave no history row (Phase 2.5 fix).
- Hard-deletes of rollouts cascade-delete history (operator must
  preserve history before deletion; Phase 2.5 schema fix).

### Operator wants real-time metrics on a Cloud Run service

**BLOCKED.** `Provider.GetMetrics` returns `ErrNotImplemented`. UI must
honestly report "metrics unavailable for cloud deployments". Phase 2.5
work.

### Operator wants live cost data

**BLOCKED.** `Provider.GetActualCost` returns "not implemented".
`EstimateCost` returns a static placeholder.

### Operator wants to set up high-frequency auto-updates of an image

**Works for Kubernetes path** (existing cron job in main.go calls
`controller.AutoUpdateController`). **Not validated for cloud path.**
The cron handler does not invoke any cloud-side update — it operates on
the Kubernetes operator. Cloud rollouts that have auto-updates
configured would silently not auto-update. Phase 2.5 work: cloud
path for auto-update.

### Operator wants to deploy to AWS or Azure

**BLOCKED.** Provider implementations for ECS / ACA are not started.
Phase 2 scope per CLAUDE.md.

## What Phase 2.3 unblocks

| Capability | Status before 2.3 | Status after 2.3 |
|---|---|---|
| Single-pod safe operation | ✓ | ✓ (preserved) |
| Rolling restart of single-replica pod | ⚠️ aggressive sweep | ✓ heartbeat-aware sweep |
| endDate during in-flight deploy → eventual destroy | ✗ intent lost | ✓ pending_destroy re-arm |
| History timeline includes operator intent | ✗ starts at dispatch | ✓ starts at intent |
| Cross-component log correlation via cycle_id | ✗ reconciler-only | ✓ reconciler + dispatcher |
| `deployment_history.provider` consistent with `deployment_targets.provider` | ✗ inconsistent | ✓ canonical |

## Related
- [[dangerous-ambiguity-inventory]]
- [[deferred-phase2.5-concerns]]
