# Lifecycle Closure Integrity Review — Phase 2.4

## Last Updated
2026-05-14

## Premise

Every lifecycle path must reach a **truthful terminal state** with
ownership released, operations marked terminal, history written, and
no residual in-flight markers.

## Terminal states inventory

### `deployment_targets.status`

| Terminal value | Reachable from | Stable? |
|---|---|---|
| `running` | updating, error (with operator), stopped | ✓ — but it's NOT a hard terminal; status-poll keeps observing |
| `error` | many | ✓ — operator-gated recovery |
| `deleted` | deleting | ✓ — hard terminal, reconciler skips entirely |
| `stopped` | running (scale=0) | ✓ — not used today; no scale-to-zero path |

### `operations.status`

| Terminal value | Reached by | Stable? |
|---|---|---|
| `succeeded` | dispatcher success branch (CAS on in_progress) | ✓ |
| `succeeded_stale` | dispatcher stale-spec branch | ✓ |
| `failed` | dispatcher hard-error, sweep abandonment, panic | ✓ |
| `cancelled` | dispatcher CAS-race-loss | ✓ |

### `deployment_history.status`

| Terminal value | Reached by | Stable? |
|---|---|---|
| `success` | dispatcher success outcome | ✓ |
| `failed` | dispatcher error/stale/panic outcome; sweep abandonment | ✓ |
| `in_progress` | dispatch start; intent boundaries (Phase 2.3) | not terminal but supersedeable |

## Closure paths — verification

### LC-1: Deploy success closure

```
pending
  → CAS claim → creating (target) + in_progress (operation) + in_progress (history)
  → Deploy succeeds
  → succeeded (operation, CAS-guarded on in_progress)
  → success (history)
  → updating (target, CAS-guarded on current_operation match)
  → cleared current_operation
  → status-poll → running (if Ready=SUCCEEDED observed)
```

**Closure properties:**
- Operation reaches `succeeded` exactly once. ✓
- History has matching `in_progress` → `success` pair. ✓
- Target releases `current_operation` and reaches `running` via poll. ✓
- All writes CAS-guarded against sweep conflicts. ✓

### LC-2: Deploy failure closure

```
pending → claim → creating + in_progress
  → Deploy errors
  → failed (operation)
  → failed (history)
  → error (target)
  → circuit breaker increments
```

**Closure properties:**
- All writes happen. ✓
- Circuit breaker prevents immediate retry storm. ✓
- Target stays in error until operator action.
- **Lineage gap**: if `pending_destroy=true` was set during the failed
  deploy, the flag persists but no path consumes it (see Phase 2.4
  convergence gap C-3, fix landing in this phase).

### LC-3: Stale-spec closure

```
pending → claim → creating + in_progress
  → Deploy succeeds
  → rolloutMovedSince → true
  → succeeded_stale (operation)
  → failed (history, message="stale spec")
  → pending (target, NOT updating)
```

**Closure properties:**
- Operation reaches a terminal status (`succeeded_stale`). ✓
- Target re-enters dispatchable state (pending). ✓
- Next cycle re-dispatches with new spec. ✓
- **Convergence concern**: see C-1 in convergence assessment.

### LC-4: Destroy success closure

```
deleting → claim → deleting + in_progress
  → Provider.Destroy returns nil (Cloud Run 200 OK)
  → succeeded (operation)
  → success (history)
  → deleted (target)
```

**Closure properties:**
- All writes. ✓
- Target reaches `deleted`, reconciler skips it forever after. ✓
- **Truthfulness window**: Cloud Run may still list the service for
  10-60s post-API-200. Phase 2.8 work: post-destroy NOT_FOUND poll.

### LC-5: Destroy failure closure

```
deleting → claim → deleting + in_progress
  → Provider.Destroy errors
  → failed (operation)
  → failed (history)
  → error (target)
```

**Closure properties:**
- All writes. ✓
- Target in `error` with the destroy attempt visible.
- **Recovery**: operator must re-flip status to `deleting` via admin
  (no auto-retry from error). Or operator could clear pending_destroy
  and respec; not a destroy convergence path.

### LC-6: Dispatcher panic closure

```
... → defer recovery fires
  → completeOperation(opID, "failed", "dispatcher panic") (CAS-guarded)
  → releaseTarget(opID, ..., "creating"|"deleting", "error", "dispatcher panic") (CAS-guarded)
  → writeHistory(action, "failed", "", externalID, "dispatcher panic", targetProvider)
```

**Closure properties:**
- All writes happen. ✓
- Operation marked failed; target → error. ✓
- **Concern:** the dispatcher's defer recovery does NOT pass external_id
  from the in-flight Deploy result (the panic may have happened before
  the result was set). So orphan correlation via history is broken in
  this case. See LC-9 below.

### LC-7: Sweep-abandoned operation closure

```
process restart
  → sweep finds in_progress op (filter applied: not within heartbeat liveness)
  → operations.status = failed, message = "abandoned: ..."
  → writeAbandonHistory: deployment_history action=error/deleted/rolled_back, status=failed
  → if target.current_operation == op.id: target.current_operation = "", status = error
```

**Closure properties:**
- Operation reaches `failed`. ✓
- History row written. ✓
- Target released to `error`. ✓
- The released-original dispatcher's later return is silently absorbed
  by the release-CAS / complete-CAS guards.

### LC-8: CAS-race-loss closure

```
two dispatchers race claim
  → loser: cancelOperation(opID, "another reconciler won the claim")
  → completeOperation(opID, "cancelled", reason)
```

**Closure properties:**
- Operation marked cancelled. ✓
- **No history row written** for the cancelled attempt.
- The winner's dispatch proceeds normally.

**Severity:** Low. CAS race losses are rare (single-pod, ticker
overlap). History row would clarify the timeline.

**Phase 2.4 fix considered:** Write a history row on cancel.
**Decision:** Defer to Phase 2.7 lineage hardening. The closure itself
is correct; only the forensic visibility is lacking.

### LC-9: External_id loss on dispatcher panic / hard error

**Setup:** Cloud Run provider's CreateService panics or returns error
before waitForServiceReady. The service may have been partially
created server-side (GCP CreateService is async).

**Behavior:**
- Dispatcher's error/panic recovery path writes history with empty
  `to_revision`.
- Target → error with no external_id reference.
- The (potentially) orphaned service is invisible to AutoStack's
  history.

**Severity:** Medium. Orphan correlation via history is broken in this
edge case.

**Phase 2.4 fix:** Modify Cloud Run Deploy to return a partial
`DeployResult` containing `ExternalID = serviceName` even on
CreateService/UpdateService error, so the dispatcher can record it.

**Decision:** Defer to Phase 2.8 (provider-side improvement, packaged
with NOT_FOUND-poll work).

### LC-10: Closure under release-lost-ownership

**Setup:** Phase 2.3 fix: heartbeat-aware sweep, so this should be rare.
But if it does happen (e.g., DB busy storm > 2 min), the dispatcher
returns and its release/complete-CAS both find 0 rows affected.

**Behavior:**
- `[OP_COMPLETE_NOOP]` logged.
- `[RELEASE_LOST_OWNERSHIP]` logged.
- **No history row written** recording what the dispatcher actually
  observed (success? failure?).
- The sweep's `failed: abandoned` history row stands as the timeline
  entry.

**Severity:** Medium. Forensic visibility lost — operator cannot tell
whether the dispatcher's Deploy actually succeeded provider-side.

**Phase 2.4 fix:** When `releaseTargetWithExternal` matches 0 rows,
emit a `deployment_history` row capturing the dispatcher's outcome
(success/failure) with a special status or message tag (e.g.,
`status=failed` + `message="dispatcher returned but sweep had reclaimed
ownership; observed=success/failed"`).

**Decision:** Land in Phase 2.7 (observability). Documented now;
implementation in 2.7.

## Closure integrity summary

| Path | Closure OK | Forensic Visibility |
|---|---|---|
| LC-1 Deploy success | ✓ | ✓ |
| LC-2 Deploy failure | ✓ (after Phase 2.4 C-3 fix for pending_destroy) | ✓ |
| LC-3 Stale-spec | ✓ | ✓ but message-only |
| LC-4 Destroy success | ✓ | ✓ but provider truthfulness window |
| LC-5 Destroy failure | ✓ | ✓ |
| LC-6 Dispatcher panic | ✓ | ⚠️ external_id lost |
| LC-7 Sweep abandon | ✓ | ✓ |
| LC-8 CAS race loss | ✓ | ⚠️ no history |
| LC-9 External_id on hard error | ⚠️ orphan correlation broken | Phase 2.8 fix |
| LC-10 Release-lost-ownership | ✓ closure via CAS | ⚠️ no dispatcher-outcome history |

## Phase 2.4 implementation in this area

- **C-3** auto-promotion from `error+pending_destroy` to `deleting`.

## Deferred

- LC-8: history on cancel (Phase 2.7).
- LC-9: external_id on hard error (Phase 2.8).
- LC-10: history on release-lost-ownership (Phase 2.7).

## Related
- [[reconciliation-convergence-assessment]]
- [[../phase2.3/lineage-integrity-review]]
- [[ownership-integrity-review]]
