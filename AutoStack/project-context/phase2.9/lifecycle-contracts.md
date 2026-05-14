# Lifecycle Contracts — Phase 2 Final

**Last Updated:** 2026-05-14

## Contract Language

This document defines WHAT the control plane guarantees, WHAT it
declines to guarantee, and WHY each choice was made. Contract violations
are bugs. Accepted limitations are documented gaps.

---

## DC-1: Deploy Lifecycle

### DC-1.1 Normal Success

```
pending → [claim] → creating → [Provider.Deploy] → waitForServiceReady
  → [success] → completeOperation(succeeded) → releaseTarget(creating, updating)
  → [next poll] → GetService(Ready=SUCCEEDED) → updateTargetStatus(updating, running)
```

**Guarantees:**
- Exactly one Provider.Deploy call per dispatch (CAS claim guard)
- `deployment_history` row written for "in_progress" and "success"
- `last_state_change_at` updated to claim time
- `_external_id` and `current_revision` populated from provider response
- `pending_destroy` consumed if set (routes to `deleting` instead of `updating`)

**Not guaranteed:**
- Exact timing of `waitForServiceReady` convergence
- That `updating → running` promotion happens in the next immediate poll
  (poll interval governs; could be up to 30s after convergence)

### DC-1.2 Provider Error (result.Status = "error")

```
pending → claim → creating → [Provider.Deploy] → result.Status="error"
  → completeOperation(failed) → releaseTarget(creating, error)
  → recordTargetFailure(category=permanent|transient)
```

**Guarantees:**
- Target → `error` with sanitized message in `drift_summary`
- Circuit breaker incremented
- History row written with failure reason

### DC-1.3 Hard Provider Error (Deploy returns error)

Same as DC-1.2 but `deployErr != nil` on the same branch.

### DC-1.4 Stale Spec During Deploy

```
pending → claim → creating → [Provider.Deploy succeeds]
  → rolloutMovedSince=true → noteStaleSucceeded++
  → if staleCount < 3: releaseTarget(creating, pending)
  → if staleCount >= 3: releaseTargetWithExternal(creating, error, "stale loop")
```

**Guarantees:**
- Deploy result is NOT trusted when rollout moved
- At most 3 automatic re-dispatches before halting
- `error` status with "pathological stale-spec loop" message at threshold
- Operator must respec to clear staleCount

### DC-1.5 Crash During Deploy

```
dispatchDeploy running → process dies
  → SweepAbandonedOperations at restart:
    - if updated_at == started_at (never heartbeated): op = failed
    - if heartbeat live (within 2 min) AND fired at least once: op preserved
  → If op failed: target → error; if op preserved: target stuck creating
```

**Guarantees:**
- Sweep never infers that a mid-deploy crash means "deploy succeeded"
- Op either preserved as live or marked `failed`; never marked `succeeded`
- `deployment_history` row written for the sweep action

---

## DC-2: Destroy Lifecycle

### DC-2.1 Normal Success

```
deleting → [claim] → [Provider.Destroy]
  → DeleteService API (200)
  → confirmDeleted poll loop: GetService every 5s until NOT_FOUND or 60s timeout
  → completeOperation(succeeded)
  → releaseTarget(deleting, deleted)
  → clearPendingDestroy(targetID)
```

**Guarantees:**
- Exactly one `DeleteService` call (CAS claim)
- NOT_FOUND confirmation before reporting `deleted` (Phase 2.8 fix)
- Target is `deleted` either way (truthful even on confirm timeout)
- `pending_destroy` flag cleared on success
- History row for every destroy attempt

### DC-2.2 Idempotent Destroy (already gone)

```
deleting → GetService → NOT_FOUND immediately
  → completeOperation(succeeded)
  → releaseTarget(deleting, deleted)
```

Same as DC-2.1 but no poll loop needed. NOT_FOUND is treated as success.

### DC-2.3 Destroy Error

```
deleting → Destroy returns error
  → completeOperation(failed)
  → releaseTarget(deleting, error)
```

---

## DC-3: Replay Contracts

### DC-3.1 Restart with In-Flight Op

| Op state at crash | Sweep action | Target result | Replay path |
|---|---|---|---|
| Never heartbeated | Mark `failed` | `error` | Operator respecs |
| Heartbeated + fresh (< 2 min) | Preserve | `creating` + `current_operation=opID` | Next cycle skips poll/dispatch; stuck until runtime sweep at 5+ min |
| Heartbeated + stale (> 2 min startup, > 5 min runtime) | Mark `failed` | `error` | Operator respecs |

### DC-3.2 Identical State = Identical Behavior

Given identical `deployment_targets` row, identical `rollouts` manifest, and
identical provider `GetService` response, the reconciler produces the same
status transition on every cycle.

**Proof:** Single-threaded, stateless `GetStatus → isAllowedTransition → persist`.
No random decisions. No caching of provider responses across cycles.

---

## DC-4: Ownership Semantics

### DC-4.1 Single-Owner Invariant

At any moment, at most ONE reconciler goroutine holds the CAS claim on a
target (verified by `current_operation` atomic UPDATE + rows_affected=1 check).

### DC-4.2 Operation Lifecycle Ownership

The goroutine that creates the `operations` row owns it until the row
reaches a terminal status. Only that goroutine writes to it. Exception:
the sweep, which treats ANY `in_progress` op as potentially abandonable
(at startup: via the first-heartbeat guard; at runtime: via the stale
heartbeat threshold).

### DC-4.3 Release Ownership and Sweep

`releaseTargetWithExternal` uses `WHERE current_operation = :op` as a CAS
guard. If the sweep has already cleared `current_operation` and set
`status equals error`, the dispatcher's release updates 0 rows and logs
`[RELEASE_LOST_OWNERSHIP]`. The sweep's state wins. `writeOwnershipLostHistory`
preserves the dispatcher's observed outcome forensically.

---

## DC-5: Convergence Guarantees

### DC-5.1 Terminal Paths Converge

| Path | Converges | How |
|---|---|---|
| Deploy → success | ✅ | `updating` → next poll → `running` |
| Deploy → provider error | ✅ | `error`, circuit holds |
| Deploy → stale × 3 | ✅ | `error` + explicit message |
| Destroy → success | ✅ | `deleted` |
| Destroy → error | ✅ | `error` |
| GetStatus → running | ✅ | Persisted |
| GetStatus → error | ✅ | After 2nd consecutive observation from `updating` |

### DC-5.2 Non-Convergent Paths (documented gaps)

| Path | Status | Closure |
|---|---|---|
| `running + pending_destroy` set | Stuck `running` before AW-C1 fix | AW-C1 fix lands in Phase 2.9 |
| `creating` + terminal op (crash gap) | `creating` with `current_operation=opId` | Operator clears `current_operation`; Phase 3 atomic release closes |
| `deleting` + confirm loop crash | `deleting` with terminal op | Check GCP console, clear manually; Phase 2.9 heartbeat fix reduces likelihood |

---

## DC-6: Retry Semantics

### DC-6.1 Failure Classification

| Category | Retry? | Circuit? | Why |
|---|---|---|---|
| Auth | Never | No | External intervention required |
| Quota | Never | No | Same |
| Permanent (NOT_FOUND, invalid) | Never | Yes (up to threshold) | Provider says no, repeated calls useless |
| Timeout | Yes | Yes | Might succeed on retry |
| Transient | Yes | Yes | Might succeed on retry |

### DC-6.2 Circuit Breaker Reset

On any successful `GetStatus` call (nil error and provider returns a
status), circuit breaker is cleared. A single success resets the count.

### DC-6.3 Suspicion Counter Reset

On any non-error observation from `updating` state, suspicion counter is
cleared. A single non-error observation resets accumulated suspicion.

---

## DC-7: Ambiguity Contracts

### DC-7.1 Honest Ambiguity (intentional)

| State | Meaning | Operator Action |
|---|---|---|
| `updating` post-deploy | Deploy succeeded; poll will promote | None |
| `creating` from `pending` | Provider accepted; waiting for Ready | None |
| `succeeded_stale` | Deploy succeeded but rollout moved | None (auto-re-dispatches) |
| `DESTROY_CONFIRM_TIMEOUT` | Deleted per API, provider may lag | Verify via console if cost-sensitive |
| `suspicion_hold` | First error observation held | None (second will confirm or clear) |
| `unknown` (not persisted) | Provider gave no condition | Poll retried next cycle |

### DC-7.2 False Ambiguity (bugs, fixed)

| State | Bug | Fix |
|---|---|---|
| `running + pending_destroy` ignored | AW-C1 | Phase 2.9 fix |

---

## DC-8: Lineage Contracts

Every significant lifecycle event produces at least one `deployment_history`
row. The sweep produces rows for abandoned operations. The dispatcher
produces rows for every terminal outcome. No lifecycle moment is untracked.

---

## Related
- [[phase2.9/provider-contracts]]
- [[phase2.9/reconciliation-architecture-freeze]]
- [[phase2.9/operational-guarantee-matrix]]
- [[phase2.9/trustworthiness-verdict]]