# Phase 2 Finalization — Observability Integrity Assessment

**Last Updated:** 2026-05-14

## Tagged Log Surface

The following log tags are reliably emitted and can be used for forensic
reconstruction:

| Tag | Source | Meaning |
|---|---|---|
| `[RECONCILE] cycle_start` | `reconcileAll` | Start of reconciliation cycle |
| `[RECONCILE] cycle_complete` | `reconcileAll` | End of cycle with stats |
| `[RECONCILE_TARGET]` | `reconcileOne` | Target being processed |
| `[DISPATCH_CLAIM]` | `dispatchDeploy/Destroy` | Claim acquired |
| `[DISPATCH_CLAIM_SKIP]` | `dispatchDeploy` | CAS race lost |
| `[DEPLOY_START]` | `dispatchDeploy` | Provider call starts |
| `[DEPLOY_END]` | `dispatchDeploy` | Provider call ends |
| `[DEPLOY_STALE]` | `dispatchDeploy` | Stale spec detected |
| `[DISPATCH_PANIC]` | `dispatchDeploy/Destroy` | Panic in dispatcher |
| `[DISPATCH_SPEC_ERR]` | `dispatchDeploy` | Manifest parse failed |
| `[DISPATCH_OP_CREATE_ERR]` | `dispatchDeploy` | Op row creation failed |
| `[CLOUD_DESTROY_REARMED]` | `dispatchDeploy` | Destroy intent honored mid-flight |
| `[OP_COMPLETE_NOOP]` | `completeOperation` | Op already terminal (expected) |
| `[OP_ABANDONED]` | `sweep` | Startup sweep reclaimed op |
| `[OP_ABANDONED_RUNTIME]` | `runtimeSweep` | Runtime sweep reclaimed op |
| `[STARTUP_SWEEP_SKIP_LIVE]` | `sweep` | Preserved live op at startup |
| `[STATE_TRANSITION]` | `updateTargetStatus` | Status changed |
| `[TRANSITION_REFUSED]` | `updateTargetStatus` | Invalid transition blocked |
| `[STATUS_UNKNOWN]` | `updateTargetStatus` | No actionable provider condition |
| `[SUSPICION_HOLD]` | `reconcileOne` | First error observation held |
| `[FAILURE]` | `reconcileOne` | GetStatus or Dispatch error |
| `[DESTROY_CONFIRM_TIMEOUT]` | `confirmDeleted` | Delete confirmed via timeout |
| `[DESTROY_CONFIRM_POLL_ERR]` | `confirmDeleted` | Transient error during confirm poll |
| `[CIRCUIT_RESET]` | `reconcileOne` | Circuit breaker cleared on pending entry |
| `[RECONCILE_SKIP] terminal_deleted` | `reconcileOne` | Deleted target skipped |
| `[RECONCILE_SKIP] dispatch_in_flight` | `reconcileOne` | Target owned by dispatcher |
| `[CLOUD_DESTROY_AUTO_PROMOTE]` | `reconcileOne` | error+pending_destroy auto-promoted to deleting |
| `[HEARTBEAT_FAIL]` | `heartbeat` | Heartbeat tick failed |
| `[HEARTBEAT_FAIL_PERSISTENT]` | `heartbeat` | 5 consecutive heartbeat failures |
| `[PENDING_DESTROY_CLEAR_ERR]` | `clearPendingDestroy` | Flag clear failed |
| `[PENDING_DESTROY_CLEARED]` | `clearPendingDestroy` | (implicit via success path) |
| `[CYCLE_BACKED_OFF]` | `reconcileWithBackoff` | Cycle skipped due to backoff |

**Coverage assessment:** Every significant decision point in the dispatch and reconciler paths has a tagged log. Transient happy-path operations (successful GetStatus with no state change) do NOT log beyond `[RECONCILE_TARGET]` and `[RECONCILE_TARGET_COMPLETE]`, which is correct — logging every poll would create noise.

---

## Forensic Reconstruction via Logs

### Scenario: What happened to target X?

**Required search:** `grep target=X logs | grep '\|' separated tags`

**Timeline reconstruction steps:**

1. `[DISPATCH_CLAIM] target=X` → when claim was acquired
2. `[DEPLOY_START] target=X` → when provider call fired
3. `[DEPLOY_END] target=X` → when provider call returned
4. `[OP_COMPLETE]` or `[OP_COMPLETE_NOOP]` (implicit via `RELEASE_LOST_OWNERSHIP` if sweep won)
5. `[STATE_TRANSITION] target=X from=Y to=Z` → status changes
6. `[HISTORY_WRITE] target=X` → deployment_history rows written

**Correlation:** All dispatch logs carry `cycle=<8hex>` (Phase 2.3 M-3). Cross-component grep works.

**Gaps:** The `[HISTORY_WRITE]` event doesn't carry `cycle_id` (it was added in Phase 2.3 to dispatch logs only). Historical correlation requires matching on `target`, `action`, and `status` across time windows.

### Scenario: Operator wants to understand a stale spec loop

**Search:** `grep 'succeeded_stale\|STALE_LOOP\|STALE'`

**Shows:** Each cycle's `[DEPLOY_STALE]`, the count, and on cycle 3: `[DEPLOY_STALE_LOOP_HOLD]`.

---

## History Row Completeness

| Event | History Row Written? | Content |
|---|---|---|
| Deploy start (claim) | ✅ | `action=created|updated, status=in_progress` |
| Deploy success | ✅ | `action=created|updated, status=success` |
| Deploy hard error | ✅ | `action=created|updated, status=failed` |
| Deploy provider error | ✅ | `action=..., status=failed` |
| Deploy stale | ✅ | `action=..., status=failed, message=stale spec` |
| Destroy start | ✅ | `action=deleted, status=in_progress` |
| Destroy success | ✅ | `action=deleted, status=success` |
| Destroy failure | ✅ | `action=deleted, status=failed` |
| Sweep abandonment | ✅ | `action=error\|deleted, status=failed, message=abandoned` |
| Dispatch CAS race lost | ✅ (Phase 2.7) | `action=..., status=failed, message=dispatch race lost` |
| Ownership lost (dispatcher returned after sweep) | ✅ (Phase 2.7) | `action=..., status=failed, message=dispatcher returned but sweep had reclaimed` |

**Coverage:** Complete for all significant lifecycle events. No orphan lifecycle moments without a history record.

---

## Observability Gaps

### OG-1: `unknown` status — no history write

When `GetStatus` returns `unknown`, `updateTargetStatus` touches `last_synced` but does NOT write a history row.

**Impact:** An extended `unknown` period is invisible in `deployment_history`.

**Severity:** Low — `unknown` is transient (typically one cycle until the service has a condition). The honest fix is that the provider should return actionable conditions, which is a Cloud Run API behavior, not an AutoStack issue.

---

### OG-2: Correlation ID not in history rows

History rows are identified by their `id` (UUID) and cross-referenced via `target` and `rollout`. There is no `correlation_id` field linking a history row to the specific cycle that produced it.

**Impact:** Correlating a history row to a specific `[RECONCILE] cycle_start` log requires fuzzy time-matching (same target id within a time window).

**Severity:** Low — the `cycle_id` in dispatch logs links the in-flight window. History is primarily a post-hoc record, not real-time correlation.

---

### OG-3: No structured logger

All logs are `log.Printf` with tagged string prefixes. There is no structured JSON logging for machine parsing.

**Impact:** Log aggregation tools (Loki, Datadog) must parse free-text tags. This is fragile for new fields and limits queryability.

**Phase 3 material:** `log/slog` adoption is listed in deferred-followups.

---

### OG-4: No per-target success/failure counters in DB

The circuit breaker state is in-memory. After restart, all circuits are closed. A process restart during a known-bad target's recovery means the target immediately retries.

**Impact:** If a target has persistent auth failures and process restarts, it will retry immediately and re-exhaust its API quota before the circuit can protect it.

**Severity:** Low — auth errors are immediate (no retry anyway). The window for transient failures is bounded to "one extra cycle." The in-memory counter is documented.

---

## Verdict

**The observability surface is comprehensive for forensic reconstruction.** All terminal paths, all significant transitions, and all non-happy-path events (sweep, stale, CAS race, panic) have tagged logs or history rows.

**The main gap is OG-3 (structured logging) — Phase 3 material.** OG-2 (correlation ID in history) is narrow but useful for Phase 3.

**Phase 2 operators can reconstruct deployment timelines from:**
- `[DEPLOY_START]` → `[DEPLOY_END]` → `[OP_COMPLETE_NOOP]` or `RELEASE_LOST_OWNERSHIP` analysis
- `[STATE_TRANSITION]` chain in logs
- `deployment_history` table (immutable, written concurrently with state writes)

---

## Related
- [[phase2.7/replay-safety-assessment]] — heartbeat-fail counter visibility
- [[phase2.9/chaos-survivability-assessment]] — observability during chaos scenarios
- [[incident-reconstruction-assessment]] (existing doc in phase2.3/)