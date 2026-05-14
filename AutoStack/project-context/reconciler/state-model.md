# Deployment Target State Model

## Last Updated
2025-05-13

## Status Values

The `deployment_targets.status` field supports the following values:

| Status | Description |
|--------|-------------|
| `pending` | Target created but deployment not yet initiated |
| `creating` | Deployment in progress (creating/reconciling) |
| `running` | Deployment successful, service is healthy |
| `updating` | Update/deploy in progress |
| `stopped` | Deployment stopped (scaled to zero or manually stopped) |
| `error` | Deployment failed or error state |
| `deleting` | Deletion in progress |
| `deleted` | Resources deleted (target record may be soft-deleted) |

## Valid State Transitions

```
pending → creating    (deployment initiated)
pending → deleted    (deployment cancelled before start)

creating → running    (deployment successful)
creating → error      (deployment failed)
creating → deleting   (user requested delete during creation)

running → updating     (update/deploy initiated)
running → stopped     (user scaled to zero)
running → deleting    (user requested delete)
running → error       (reconciliation detected failure)
running → pending    (drift detected, reconciliation pending action)

updating → running    (update successful)
updating → error       (update failed)
updating → deleting    (user requested delete during update)

stopped → running     (user restarted)
stopped → deleting    (user requested delete)

error → creating      (retry initiated)
error → deleting      (user requested delete)
error → running       (manual fix resolved issue)

deleting → deleted    (deletion successful)
deleting → error      (deletion failed, orphan risk)
```

## Invalid Transitions

The following transitions should NOT occur:

| From | To | Reason |
|------|-----|--------|
| deleted → * | Any transition from deleted | Target should not be reused |
| pending → running | Direct without creating | Missing deployment step |
| deleted → creating | Target recreation | Must create new target |

## State Transition Rules

### Reconciliation Behavior by State

| State | Reconciliation Action |
|-------|---------------------|
| `pending` | No action (waiting for external trigger) |
| `creating` | Check provider status, update if changed |
| `running` | Monitor status, detect drift |
| `updating` | Check provider status, update if changed |
| `stopped` | No action (intentionally stopped) |
| `error` | Log error, circuit breaker may skip |
| `deleting` | Monitor deletion progress |
| `deleted` | No action |

### State Transition Validation

The reconciler should validate transitions:

1. Log unexpected states (e.g., `deleted` appearing with active provider resources)
2. Log unexpected transitions
3. Use last known good state for recovery decisions

### Stuck State Detection

A state is considered "stuck" when:

| State | Stuck After | Action |
|-------|------------|--------|
| `creating` | > 10 minutes | Query provider, update status |
| `updating` | > 10 minutes | Query provider, update status |
| `deleting` | > 5 minutes | Retry deletion or mark error |
| `error` | > 30 minutes | No auto-action, let circuit breaker handle |

## Drift Detection States

Drift is detected but NOT automatically remediated in Phase 1:

| Drift Type | Detection | Response |
|-----------|-----------|----------|
| `manual_change` | Config mismatch | Set `drift_detected=true`, log warning |
| `config_mismatch` | Spec changed externally | Set `drift_detected=true`, log warning |
| `external_modification` | Unexpected provider change | Set `drift_detected=true`, log warning |

Auto-remediation is configurable for Phase 2.

## Operation States

In-flight operations track their own state:

| Operation State | Description |
|----------------|-------------|
| `pending` | Operation queued, not started |
| `in_progress` | Operation executing |
| `succeeded` | Operation completed successfully |
| `failed` | Operation failed |
| `cancelled` | Operation cancelled (by user or timeout) |

## Status vs Operation State Relationship

```
Status: creating + OperationStatus: in_progress → Normal creation
Status: creating + OperationStatus: failed → Error state
Status: running + OperationStatus: succeeded → Normal running
Status: error + OperationStatus: failed → Normal error state
Status: updating + OperationStatus: in_progress → Normal update
```

## Future Extensions (Phase 2+)

- Add `rollout_target` state for multi-target rollouts
- Add `paused` state for deployment pausing
- Add `cancelled` state for user-initiated cancellation
- Add transition history tracking
- Add state machine validation in PocketBase rules