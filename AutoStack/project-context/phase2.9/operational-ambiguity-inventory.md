# Phase 2 Finalization — Operational Ambiguity Inventory

**Last Updated:** 2026-05-14

## What This Document Is

Catalogs every operational state where the system holds truthful ambiguity
(intentional or unavoidable) rather than false certainty. The goal is
visibility, not resolution — an operator who encounters any of these
should understand what they're looking at.

---

## Intentional Truthful Ambiguity

These states are accepted because hiding them would be a lie.

### A-1: Post-deploy `updating` not `running`

**State:** Target shows `updating` for up to one poll cycle (~30s) after a successful deploy.

**Why:** `dispatchDeploy` releases `creating → updating` (not `running`). The poll path promotes to `running` on the next GetService call.

**Ambiguity:** Is this deploy still in-flight or has it converged?

**Visibility:** `[STATE_TRANSITION] target=X from=creating to=updating` at deploy success. `[STATE_TRANSITION] target=X from=updating to=running` at next poll.

**Operator action:** None. This is correct transient state.

---

### A-2: `creating` vs `updating` — both mean "in-flight deploy"

**State:** `creating` comes from initial pending dispatch; `updating` comes from post-success release.

**Ambiguity:** Operators may not know these are semantically equivalent in Phase 2.8.

**Visibility:** Both are `creating` in state-model.md. The UI should treat them identically or collapse them.

**Phase 3:** [[phase2.8/manual-cloud-mutation-policy.md]] §Phase 2.8 acknowledgments notes `drift_detected` remains permanently false.

---

### A-3: `unknown` status not persisted

**State:** Provider returns no actionable condition → reconciler does not write `unknown` to the status field (not in enum). Logs `[STATUS_UNKNOWN]` and touches `last_synced`.

**Ambiguity:** What IS the target's state if the provider says nothing?

**Visibility:** `deployment_targets.status` retains its previous value. `last_synced` and `drift_summary` show the last provider response. Operator can see the timestamp of the last poll.

**Correct interpretation:** "Last known state is X; provider gave no new information."

---

### A-4: Destroy 200 + timeout (confirmDeleted)

**State:** `DeleteService` → 200, but `GetService` still returns the service after 60s confirm window.

**Ambiguity:** Is the service gone or not?

**Visibility:** `[DESTROY_CONFIRM_TIMEOUT]` log. `drift_summary` notes the timeout. Target marked `deleted`.

**Correct interpretation:** "AutoStack believes the service is deleted; GCP may still be cleaning up. Verify via console." The target is terminal `deleted` either way.

**Operator action:** Verify via GCP console if there is cost/accounting sensitivity.

---

### A-5: `error` from `updating` — first observation held

**State:** Provider returns Ready=FAILED once; AutoStack holds `updating` (does not flip to `error`).

**Ambiguity:** Is this a real failure or a Cloud Run convergence flap?

**Visibility:** `[SUSPICION_HOLD]` log on first observation. On second: `[STATE_TRANSITION] ... to=error`.

**Correct interpretation:** "Two consecutive provider errors required before declaring failure." This prevents transient flaps from corrupting state.

---

### A-6: `succeeded_stale` — deploy succeeded but rollout moved

**State:** Deploy completes (provider returned running), but the rollout was updated during the call.

**Ambiguity:** Was the right version deployed?

**Visibility:** `succeeded_stale` operation status. `drift_summary = stale spec`. History row shows `status=failed, message=stale spec`.

**Correct interpretation:** "The deploy technically succeeded, but AutoStack does not trust it because the desired state changed during execution. The next cycle will re-dispatch."

---

## Unintentional Ambiguity (Correctable)

### UA-1: `running` + `pending_destroy` + `endDate` set — stuck destroy intent

**State:** Operator sets `endDate` on a `running` target; `pending_destroy=true` is set, but target stays `running`.

**This is a bug**, not intentional ambiguity. Documented in [[phase2.9/reconciliation-convergence-assessment]] C-1.

**Visibility:** Target looks healthy (`running`). `pending_destroy` flag is invisible in standard UIs.

**Should appear as:** `running` with a visible destroy-pending badge, or AutoStack should auto-transition to `deleting`.

---

### UA-2: `creating` with a terminal operation row

**State:** Dispatcher completes `Deploy` but crashes between `completeOperation` (terminal) and `releaseTarget` (clears `current_operation`).

**Visibility:** Target is `creating`. Operations row is `failed` (sweep's classification of this abandoned op). `current_operation` still points at the terminal op.

**Next cycle:** `shouldDispatchDeploy` returns false. `shouldDispatchDestroy` returns false. Poll path: target has `current_operation != ''`, so skip poll. Target is stuck `creating`.

**Severity:** Structural. Phase 2.9 fix: sweep should also clear `current_operation` when reclaiming a `creating` target — sweep already does this (line 145: clears `current_operation` + sets `status=error`). But if the release happened AFTER completeOperation, the release itself may overwrite the sweep's `error` → `updating`. **This is the `releaseStillOwner` guard's purpose — the dispatcher detects when the sweep took over mid-flight.** However, if the dispatcher's release wins (releases to `creating` with op=failed), the next cycle sees `creating` + `current_operation=failed_op` → stuck.

**Fix:** `releaseTarget` should reject releasing to a non-terminal state when the op is already terminal. Or sweep should clear `current_operation` regardless of op status.

---

## Truthful States That Should Be More Visible

| State | What's missing from operators |
|---|---|
| `updating` | Explanation that "updating" = "converging, poll will promote to running" |
| `creating` | Same — `creating` = "provider accepted the deploy, waiting for Ready" |
| `error` + circuit open | Explicit note that circuit is open; "X failures occurred" |
| `deleted` after destroy-timeout | Note that confirm window exceeded, "verify via console" |
| `succeeded_stale` | Explicit "rollout moved during deploy; re-dispatching" |

Phase 3 UI work should surface these explanations.

---

## Verdict

**A-1 through A-6 are intentional, visible, and survivable.** The operator can reconstruct what happened from logs + history + status.

**UA-1 (running+pending_destroy stuck) blocks Phase 2 trust claims** — it allows a target to appear `running` while an operator's destroy intent silently goes unconsumed.

**UA-2 (creating+terminal-op) is a gap**, but gated on a precise crash timing that makes it rare in practice.

**Both UA-1 and UA-2 require Phase 2.9 fixes.** The intentional ambiguity matrix is otherwise complete.

---

## Related
- [[phase2.9/reconciliation-convergence-assessment]] — UA-1 analysis
- [[phase2.9/lifecycle-closure-assessment]] — UA-2 analysis
- [[drift-handling-maturity]]