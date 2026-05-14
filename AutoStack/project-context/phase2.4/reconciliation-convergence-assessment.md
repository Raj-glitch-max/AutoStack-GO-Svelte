# Reconciliation Convergence Assessment — Phase 2.4

## Last Updated
2026-05-14

## Definition

A reconciliation system **converges** if, given:

- a fixed desired state (rollout manifest, target_type, cloud_account),
- a fixed provider state at time T,
- enough time to run N tick cycles,

then the persisted PocketBase state and the provider-observable state
stabilize at matching values, and the operations table contains no
live in-progress rows attributable to that target.

Anywhere convergence is not guaranteed, the system can stall, oscillate,
or accumulate stale state — all of which violate the directive.

## Convergence per lifecycle path

| Start state | Desired state | Convergence path | Stabilizes? | Notes |
|---|---|---|---|---|
| pending | running | pending→creating→updating→running (via dispatch + status-poll) | ✓ | First-deploy. Bounded by `DeployTimeout` (15 min) per attempt. |
| running | running (steady) | status-poll observes Ready=SUCCEEDED → stays running | ✓ | Steady state. |
| running | running (new spec, same image tag) | respec → flipCloudTargetsToPendingOnRespec → pending → creating → updating → running | ✓ | Two-tick convergence. |
| running | running (new image tag) | same as above | ✓ | Cloud Run revision swap, traffic shift handled by provider. |
| running | deleted | endDate set → markCloudTargetForDestroy → deleting → dispatch Destroy → deleted | ✓ | If endDate set during in-flight deploy: pending_destroy re-arm (Phase 2.3) routes to deleting on dispatcher release. |
| error | (operator clears + respec) | error → pending → creating → updating → running | ✓ | Operator-gated. Circuit breaker holds until operator action. |
| pending | error (deploy fails) | pending→creating→deploy fails→error | ✓ Bounded. Circuit breaker opens after threshold (5). |
| updating | error (transient) | suspicion counter holds first observation; second confirms | ✓ | 2-cycle convergence. |
| creating | (process crash mid-deploy) | sweep → operations failed: abandoned → target → error | ✓ Bounded. Operator-gated. |
| `succeeded_stale` deploy loop | (rollout updated continuously) | re-dispatch every cycle | ⚠️ Does NOT converge naturally. See C-1. |
| deleted | (target row stays as-is) | reconcile skips (terminal-deleted guard) | ✓ | No tick activity. |
| Cloud-side service deleted manually | (AutoStack target = running, provider = NOT_FOUND) | next status-poll returns NOT_FOUND error → updateTargetStatus → category=permanent → error | ✓ Bounded. Operator must investigate. |
| Cloud-side service in long convergence (revision propagating for minutes) | updating | suspicion counter on `updating→error`; allows transition guard refuse on `updating→creating` | ✓ | Stays at `updating` until provider stabilizes. |
| Long-running deploy past DeployTimeout (15min) | error | ctx-cancelled → result.Status=cancelled/timeout → result.Status=error branch → target → error | ✓ Bounded. |
| Two pending targets simultaneously (CAS race) | one runs, one cancels | dispatcher CAS handles | ✓ | |
| target's cloud_account credentials rotated | (cloud_accounts row updated) | next reconcile decrypt fails OR succeeds with new key | ✓/✗ | Depends on whether new key matches the encryption-key env var. If credentials are NEW but encryption key UNCHANGED, decrypt works → provider call uses new creds. If encryption KEY itself rotated without re-encrypting, decrypt fails → auth-class → target → error (forever, until operator re-encrypts). Convergence to error. ✓ |
| target with `pending_destroy=true` + deploy fails | error (pending_destroy persists) | next operator-triggered respec OR retry... wait, no auto-retry from error. **Convergence requires operator action.** | ⚠️ See C-3. |

## Convergence gaps

### C-1: Pathological `succeeded_stale` loop never converges

**Setup:** A respec is issued faster than the average deploy duration.
Each dispatch completes, observes `rollout.updated` changed, marks
`succeeded_stale`, releases to `pending`. Next cycle: new dispatch.

**Behavior:**
- No failure increment (succeeded_stale ≠ failure).
- No circuit breaker engagement.
- Each cycle consumes ~5–10 min of cloud Run quota.
- Provider's Cloud Run revision history accumulates wasted revisions.
- **Does not converge in any meaningful sense.**

**Severity:** Low for real production (no operator updates the manifest
every 8 minutes intentionally). Medium for automated systems that
patch fields frequently.

**Phase 2.4 fix (deferred to Phase 2.6):** Track
`succeeded_stale` count on the target row (or in-memory). After N
consecutive stale outcomes (e.g., 3), open the circuit and require
operator intervention.

**Rationale for deferral:** No production evidence of the pathology
yet. Premature implementation could mask a real convergence issue.
Phase 2.6 will add the guard.

### C-2: `error` state never auto-converges to anything

**Setup:** Any deploy failure (auth, quota, permanent, transient) ends
in `error`. Phase 2.0 chose "no auto-retry from error" deliberately.

**Behavior:** Target stays in `error` until:
- Operator clears it via admin OR
- Rollout is respec'd (`flipCloudTargetsToPendingOnRespec` flips
  error → pending) OR
- Rollout is ended (`markCloudTargetForDestroy` flips error → deleting,
  if no in-flight op).

**Severity:** Tolerable. Operator-gated recovery is the chosen policy
([[../reconciler/operation-ownership]] §"Why we forbid auto-retry from
`error`").

**Convergence verdict:** ✓ if operator acts; ⚠️ otherwise the target
sits in `error` permanently.

**Not a fix candidate.** This is by design.

### C-3: `pending_destroy=true` + deploy fails → operator must manually flip

**Setup:** Operator sets endDate while deploy is in flight (Phase 2.3
`pending_destroy` flag set). Deploy then fails (provider error). The
release path takes the `deployErr != nil` branch which calls
`releaseTarget(..., "creating", "error", msg)`. `pending_destroy`
remains `true`.

**Behavior:**
- Target in `error`.
- `pending_destroy=true` on disk.
- Reconciler's status-poll path doesn't honor `pending_destroy`.
- `shouldDispatchDestroy` requires status=`deleting`.
- **The flag is set but no path consumes it.**

**Severity:** Medium. The destroy intent is preserved but invisible
beyond the flag itself.

**Phase 2.4 fix:** The reconciler's status-poll path should check
`pending_destroy` and (if set + status=error + no in-flight op)
flip the target to `deleting` so the next dispatch destroys it. This
closes the convergence gap: an aborted+failed deploy now self-heals
toward destruction.

**Implementation:** Add a "auto-deleting promotion" check in
`reconcileOne`, run when `status=error AND pending_destroy=true AND
current_operation=""`. Flip to `deleting`; let the dispatcher pick it
up.

**Status:** Will land in Phase 2.4 implementation work.

### C-4: A target whose rollout was hard-deleted by admin still reconciles

**Setup:** Admin uses PocketBase admin UI's "delete" on a rollout,
bypassing `HandleRolloutDelete`'s OnRecordBeforeDeleteRequest hook.
The cascade-delete fires; the rollout row and deployment_targets rows
go away. The Cloud Run service remains (orphaned).

**Behavior:** The reconciler's SELECT joins
`deployment_targets → rollouts → cloud_accounts`. With no
deployment_targets row, no reconciliation runs. The service is silently
orphaned.

**Convergence verdict:** Not a convergence issue per se (the system has
no record of the target). But the provider state DIVERGES from
AutoStack's "no record" state permanently.

**Fix:** Orphan-cleanup scanner. Already deferred to Phase 2.5.

### C-5: Stale records in `operations` accumulate forever

**Setup:** Long-running deployment with frequent deploys. `operations`
table grows; terminal-status rows are never cleaned.

**Behavior:** Not a convergence issue per se (the lifecycle reaches
terminal correctly), but it's an entropy issue. Will be fixed by
Phase 2.5 TTL/cleanup.

## Convergence summary

| Property | Today | After Phase 2.4 |
|---|---|---|
| Every dispatch reaches terminal op status | ✓ | ✓ |
| `error` state has a recovery path | operator-gated | unchanged |
| `pending_destroy=true` + error has a path | ✗ stuck | ✓ auto-promote to deleting |
| `succeeded_stale` loop has a bound | ✗ infinite | unchanged in 2.4; Phase 2.6 adds bound |
| Operations table bound | ✗ grows forever | Phase 2.5 TTL |
| Orphan cloud services after admin-delete | ✗ permanent | unchanged in 2.4; Phase 2.5 scanner |
| Cloud-side manual delete observed | ✓ targets converge to error | unchanged |

Phase 2.4 closes one of the five convergence gaps (C-3); two are
deferred to Phase 2.5/2.6 implementation; one is by design (C-2); one
is operator-out-of-band (C-4).

## Related
- [[retry-amplification-review]]
- [[lifecycle-closure-integrity-review]]
- [[operation-retention-ttl-proposal]]
- [[../phase2.3/replay-safety-assessment]]
