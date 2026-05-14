# Phase 2 Mandatory Correctness Fixes — Implementation Record

**Last Updated:** 2026-05-14

## Summary

Three mandatory fixes identified in the Phase 2 finalization review. All three
are implemented. Build and vet pass cleanly.

---

## AW-C1: `running + pending_destroy` Destroy Intent Auto-Promote

### Root Cause

`cloud.go` `reconcileOne` only auto-promoted to `deleting` when
`previousStatus == "error"`. Operators who set `endDate` on a `running`
target had their destroy intent silently ignored because:

- `pending_destroy = true` was set on the target
- H-1's `promoteToDeleting` check only matched `error`
- `shouldDispatchDeploy` returns false (status not `pending`)
- `shouldDispatchDestroy` returns false (status not `deleting`)
- Status poll skips (target is `running` with no `current_operation`)
- Target stuck `running` with no path to destroy

### Operational Impact

Target appears healthy (`running`) while the operator's destroy intent is
never consumed. Service continues running past the operator's endDate.

### Fix Location

`pkg/reconciler/cloud.go` lines 526-543.

### Change

```go
// Before:
if previousStatus == "error" && pendingDestroyRaw && rolloutEndDate != "" {

// After:
if (previousStatus == "error" || previousStatus == "running") && pendingDestroyRaw && rolloutEndDate != "" {
```

The `updating` case is intentionally excluded — dispatchDeploy's
`pendingDestroy` path already handles routing `updating → deleting` on
successful deploy completion.

### Replay/Lifecycle Impact

- After fix: `running + pending_destroy + endDate` → `deleting` in one
  cycle → `shouldDispatchDestroy` fires next cycle
- No regression: `error + pending_destroy` behavior unchanged
- No lifecycle regression: `pending` targets dispatch through deploy path,
  not destroy, regardless of `pending_destroy`

### Ownership Correctness

`promoteToDeleting` writes directly to the DB record with a separate
`FindRecordById` + `SaveRecord`. No CAS claim is involved — at this point
`current_operation == ""` (Honesty guard #2 verified that before dispatch
branching). Safe.

---

## AW-C2: Panic Recovery — ReleaseTarget Always Called

### Root Cause

Previous analysis flagged a concern: if a panic fires AFTER
`completeOperation` but BEFORE `releaseTargetWithExternal` in the
dispatchDeploy success path, the target might be left `creating` with a
terminal operation row. After re-examining the panic defer block:

```go
defer func() {
    if rec := recover(); rec != nil {
        r.completeOperation(opID, "failed", "dispatcher panic")
        r.releaseTarget(opID, targetID, "creating", "error", "dispatcher panic")
        ...
    }
}()
```

The panic defer calls BOTH `completeOperation` AND `releaseTarget`. Go
defer executes synchronously before stack unwinding — there is no window
where sweep could run between them. The concern was unfounded for the panic
case.

### Actual Gap (hard crash, not panic)

A `SIGKILL` or hard process exit BETWEEN `completeOperation` and
`releaseTargetWithExternal` (lines 291-293) would leave the target
`creating` with a terminal operation. This is NOT fixable without making
those two calls atomic (Phase 3 territory). It is already handled by the
sweep at next startup if the heartbeat had fired at least once.

### Fix Applied

Verified: the panic defer in both `dispatchDeploy` and `dispatchDestroy`
correctly calls `completeOperation` + `releaseTarget`. Both dispatchers
already handle the panic case correctly. The AW-C2 concern was misfiled.

**Phase 2.9 assessment:** No additional code change required. The panic
defer already achieves the stated goal. The hard-crash gap is accepted
for Phase 2 (requires Phase 3 atomic transaction pattern to close).

### Operational Impact

None — the existing code is correct. The issue was a false positive from
a misread of the dispatch flow.

---

## AW-C3: `confirmDeleted` Heartbeat Scoping

### Root Cause

In `dispatchDestroy`, `heartbeatCtx` was scoped to `destroyCtx` (the
inner timeout derived from `ctx`):

```go
destroyCtx, cancel := context.WithTimeout(ctx, DeployTimeout)
defer cancel()
go r.heartbeat(destroyCtx, opID)  // ← WRONG: dies when destroyCtx is cancelled
```

When `p.Destroy(destroyCtx, account, target)` returns (including the
`confirmDeleted` poll loop completing within 60s), `destroyCtx` is
cancelled. The heartbeat goroutine stops. If a sweep fires while the
dispatcher is still running post-Destroy return (e.g., while it executes
`completeOperation` and `releaseTarget`), the op has no fresh heartbeat.

This gap is narrow: the heartbeat fires every 60s. A sweep only fires every
5 min. So the op needs to have been running for > 5 min for the sweep to
even consider reclaiming it. In practice, a `Destroy` + `confirmDeleted`
takes at most 60s. The window for the sweep to reclaim a live op is very
small. But it is non-zero.

### Operational Impact

A process crash during confirmDeleted, followed by sweep running within
5 min of the last heartbeat tick (which would have been stopped when
destroyCtx was cancelled) could cause the sweep to reclaim the op. The
target would be `deleting` with `current_operation=''` and an `error`
operations row. The target is stuck `deleting` until operator clears it.

### Fix Location

`pkg/reconciler/dispatch.go` lines 193-203 (dispatchDeploy heartbeart)
and lines 351-358 (dispatchDestroy heartbeat).

Both heartbeat goroutines now use the outer `ctx` (not inner `*Ctx`):

```go
// dispatchDeploy: deployCtx cancelled on Provider.Deploy return
go r.heartbeat(ctx, opID)  // ctx = outer deadline, outlives deployCtx

// dispatchDestroy: destroyCtx cancelled when Destroy() returns
go r.heartbeat(ctx, opID)   // ctx = outer deadline, outlives destroyCtx
```

The heartbeat's own UPDATE uses `WHERE status = 'in_progress'`, so once
`completeOperation` transitions the row to `succeeded/failed`, subsequent
heartbeat ticks are no-ops automatically.

### Replay/Lifecycle Impact

- After fix: heartbeat persists through `confirmDeleted` loop. Sweep cannot
  reclaim the op during the confirm window or any subsequent execution.
- No lifecycle regression — heartbeat naturally stops when op leaves
  `in_progress`.
- `releaseTargetWithExternal` CAS still guards against sweep race when
  dispatcher is actually returning.

### Ownership Correctness

The heartbeat's `WHERE status = 'in_progress'` condition means it
self-limits: once `completeOperation` fires, heartbeat ticks match zero
rows and stop. No cleanup of the heartbeat map needed on success
(the `defer` in `heartbeat()` already does `delete(heartbeatFails, opID)`).

---

## Build Verification

```
go build ./...   ✓ clean
go vet ./...     ✓ clean
```

---

## Phase 2.9 Fix Checklist

- [x] AW-C1: `running + pending_destroy` auto-promote — implemented
- [x] AW-C2: Panic recovery correctness — verified correct (no code change)
- [x] AW-C3: Heartbeat scopes to outer ctx — implemented for both
        dispatchDeploy and dispatchDestroy

---

## Related
- [[phase2.9/trustworthiness-verdict]] — overall Phase 2 verdict
- [[phase2.9/reconciliation-convergence-assessment]] — AW-C1 root cause
- [[phase2.9/lifecycle-closure-assessment]] — AW-C3 root cause
- [[phase2.9/replay-determinism-assessment]] — confirmDeleted analysis