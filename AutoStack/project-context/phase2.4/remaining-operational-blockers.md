# Remaining Operational Blockers — Phase 2.4

## Last Updated
2026-05-14

## Format

Phase 2.3 introduced "blockers by deployment context". Phase 2.4
maintains the same lens, updated with the new C-3/M-2/M-3 fixes
landing in this phase.

## Blockers by deployment context

### Single-pod, single-region, single-tenant

After Phase 2.4 lands, this context is **fully supported** for the
following workflows:

- First-time deploy
- Re-deploy on respec
- Destroy via endDate
- Destroy intent set during in-flight deploy (Phase 2.3 + Phase 2.4 fix)
- Recovery from `error` via respec (Phase 2.4 in-memory state clearing)
- Status visibility for steady-state running services
- Rolling restart of the single replica (Phase 2.3 heartbeat-aware sweep)
- Mid-deploy crash recovery

Known limitations within this context (documented, accepted):

- Rollback not implemented.
- Live metrics not implemented (UI must show "unavailable").
- Cost estimates static.
- No spec-vs-actual drift detection (Phase 2.8 work).
- No stuck-state detection (Phase 2.6 work).
- Pathological `succeeded_stale` loop has no circuit (Phase 2.6 work);
  no production evidence of this pattern yet.
- Operations + history grow forever (Phase 2.5 TTL work).

### Multi-pod (rolling restart)

After Phase 2.4 (unchanged from Phase 2.3):
- Single-replica rolling restart is safe.
- Heartbeat-aware sweep preserves in-flight ops of the outgoing pod.

### Multi-pod (concurrent replicas)

**BLOCKED.** Pod-identity stamping (Phase 2.5 work) is required for:
- Safe sweep behavior across peer pods.
- Eliminating doubled API calls for status-poll.
- Eliminating status-write races.

Mitigation guidance: **operate as 1 replica.**

### Postgres backend

**BLOCKED** until Phase 2.5/3 work normalizes:
- Empty-string vs NULL CAS predicates.
- Primary-read enforcement on `rolloutMovedSince`.
- Test coverage for Postgres-specific edge cases.

### Production HA hosting

**BLOCKED** by:
- Multi-pod blocker (Phase 2.5).
- KMS key management (Phase 3).
- SSO/SAML (Phase 3+).
- Audit log (Phase 3).
- Stuck-state detector (Phase 2.6).
- Orphan-cleanup scanner (Phase 2.5).
- Drift detection (Phase 2.8).
- Operation/history TTL (Phase 2.5).

This context cannot be supported today. Phase 2.9 will define the
production boundary explicitly.

## Blockers by use case

### Operator wants to roll back a bad deploy

**BLOCKED.** Phase 2.5 work.

### Operator wants to delete a cloud rollout safely

**Works** end-to-end after Phase 2.4:
- Set endDate while running → `markCloudTargetForDestroy` flips
  `deleting` → reconciler dispatches Destroy → `deleted`.
- Set endDate while in-flight deploy → `pending_destroy=true` flag → if
  deploy succeeds, dispatcher routes to `deleting` (Phase 2.3) → if
  deploy fails, reconciler auto-promotes `error+pending_destroy+endDate`
  to `deleting` (Phase 2.4 H-1 fix) → reconciler dispatches Destroy.

### Operator wants to update a running cloud rollout

**Works.** Phase 2.3 + Phase 2.4 in-memory state clearing ensures the
recovery from `error` via respec is clean.

### Operator wants real-time metrics

**BLOCKED.** Phase 2.5+ work.

### Operator wants live cost data

**BLOCKED.** Phase 2.5+ work.

### Operator wants drift detection on a running service

**BLOCKED.** Phase 2.8 work.

### Operator wants to debug an incident

**Adequate.** Phase 2.3 + Phase 2.4 give:
- Cycle-correlated logs.
- Intent-boundary history rows.
- Operations table with timestamps.

Insufficient surfaces:
- No timeline endpoint (Phase 3 frontend).
- No incident snapshot tool (Phase 3).
- Cleanup activity visibility once Phase 2.5 lands (must include
  during 2.5 implementation).

## What Phase 2.4 unblocks

| Capability | Before 2.4 | After 2.4 |
|---|---|---|
| Mid-deploy endDate-set → eventual destroy ON FAILED deploy | ✗ stuck at error+pending_destroy | ✓ auto-promotes to deleting |
| Operator respec recovery from `error` | ⚠️ in-memory failures persist; circuit holds until restart | ✓ failures + suspicions cleared |
| `pending_destroy` interfering with manual recovery | ⚠️ would promote even after operator cleared endDate | ✓ verifies endDate still set |

## Related
- [[dangerous-ambiguity-inventory]]
- [[deferred-phase2.5-concerns]]
- [[../phase2.3/remaining-operational-blockers]]
