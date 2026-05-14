# Forensic Completeness Assessment — Phase 2.7

## Last Updated
2026-05-14

## Question

After Phase 2.7 lands, can an operator reconstruct any incident from
`deployment_history` + `operations` + tagged log lines alone, without
reading source code?

## Test cases

### F-1: Failed deploy

- History: in_progress row (Phase 2.3 intent boundary) + dispatcher's
  in_progress row + failure outcome row.
- Operations: failed row with category-classified message.
- Logs: `[DISPATCH_CLAIM]`, `[DEPLOY_START]`, `[DEPLOY_END]`,
  `[FAILURE]` with cycle_id.

**Verdict:** ✓ Reconstruction possible.

### F-2: Stale-spec replay (multiple cycles)

- History: one failed-stale-spec row per cycle.
- Operations: succeeded_stale row per cycle.
- Logs: `[DEPLOY_STALE]` per cycle with stale_count incremented (Phase
  2.6).

**Verdict:** ✓ Reconstruction possible; stale_count makes cadence
visible.

### F-3: Stale loop hold at threshold

- History: failed row with explicit "pathological stale-spec loop"
  message.
- Operations: succeeded_stale row with the same explicit message.
- Logs: `[DEPLOY_STALE_LOOP_HOLD]`.

**Verdict:** ✓ Clear forensic surface.

### F-4: Sweep abandonment at startup

- History: `writeAbandonHistory` row per abandoned op.
- Operations: failed with "abandoned: process restart" message.
- Logs: `[STARTUP_SWEEP]`, `[OP_ABANDONED]`.

**Verdict:** ✓ Reconstruction possible.

### F-5: Runtime sweep reclaim

- History: `writeAbandonHistory` row per reclaimed op.
- Operations: failed with "heartbeat went stale" message.
- Logs: `[RUNTIME_SWEEP]`, `[OP_ABANDONED_RUNTIME]`.

**Verdict:** ✓ New in Phase 2.6 — works.

### F-6: Release-lost-ownership

**Before Phase 2.7:** Dispatcher returns post-sweep. CAS guards fail.
Log emits `[RELEASE_LOST_OWNERSHIP]`. **No history row.**

**After Phase 2.7:** Dispatcher returns post-sweep. CAS guards fail.
Log emits `[RELEASE_LOST_OWNERSHIP]`. **History row with dispatcher's
observed outcome is written.**

**Verdict:** ✓ after Phase 2.7.

### F-7: CAS race loss

**Before:** Dispatcher cancels its op. No history row.

**After Phase 2.7:** Brief history row written: action=created/updated,
status=failed, message="dispatch race lost; operation cancelled".

**Verdict:** ✓ after Phase 2.7.

### F-8: Pending_destroy auto-promotion (Phase 2.4 H-1)

- Logs: `[CLOUD_DESTROY_AUTO_PROMOTE]`.
- No history row written at the promotion itself.

**Concern:** The promotion changes target state but doesn't write
history. Subsequent dispatch writes "deleted in_progress" history.
Operator can trace via the logs.

**Phase 2.7 fix:** Write a forensic history row at promotion. Land in
Phase 2.7.

### F-9: Heartbeat-fail storm

**Before:** Per-tick `[HEARTBEAT_FAIL]`.

**After Phase 2.7:** Per-tick `[HEARTBEAT_FAIL]` PLUS
`[HEARTBEAT_FAIL_PERSISTENT]` after 5 consecutive.

**Verdict:** Easier alerting after 2.7.

### F-10: Cycle backed off

**Before:** Silent. Cycle skipped due to backoff, no signal.

**After Phase 2.7:** `[CYCLE_BACKED_OFF] sinceLastError=Xs
backoffDuration=Ys` emitted.

**Verdict:** Visibility into backoff state.

## Forensic completeness scorecard

| Incident class | Pre-2.7 | Post-2.7 |
|---|---|---|
| Deploy success | ✓ | ✓ |
| Deploy failure | ✓ | ✓ |
| Stale-spec loop | ✓ (Phase 2.6 added counter) | ✓ |
| Stale loop hold | ✓ (Phase 2.6) | ✓ |
| Startup sweep abandonment | ✓ | ✓ |
| Runtime sweep abandonment | ✓ (Phase 2.6) | ✓ |
| Release-lost-ownership | ✗ | ✓ |
| CAS race loss | ✗ | ✓ |
| Pending_destroy auto-promotion | partial | ✓ |
| Heartbeat-fail storms | partial | ✓ |
| Cycle backoff | ✗ | ✓ |

## Phase 2.7 closes 5 forensic surface gaps

All small, all additive, all log-or-history only (no schema work
required).

## Related
- [[../phase2.3/incident-reconstruction-assessment]]
- [[../phase2.4/incident-reconstruction-maturity-review]]
