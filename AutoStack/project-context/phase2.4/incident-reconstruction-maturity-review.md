# Incident Reconstruction Maturity Review — Phase 2.4

## Last Updated
2026-05-14

## Premise

Phase 2.3 walked 10 incident scenarios and identified reconstruction
quality. Phase 2.4 evaluates **maturity** — can an operator at 3 AM
actually use what's there, or do they hit grep archaeology?

## The maturity rubric

| Maturity level | Operator must... |
|---|---|
| 0 — none | read source code |
| 1 — basic | grep logs by target + timestamp |
| 2 — correlated | grep by cycle_id; cross-reference history + operations |
| 3 — structured | run a query against history; trace a timeline |
| 4 — observable | dashboard panel showing per-target lifecycle |
| 5 — automated | runbook tooling that reconstructs timelines |

## Current maturity per incident type

| Incident | Today's level | Bottleneck |
|---|---|---|
| Failed deploy | 2 | Manual cycle_id grep across components; history + operations join required |
| Stale-spec replay | 2 | Two events per cycle visible in logs; history records "stale spec" but no replay count |
| Rollback interruption | n/a | Rollback not implemented |
| Sweep reclaim event | 2 | `[STARTUP_SWEEP]`, `[OP_ABANDONED]` clear; cycle_id missing (sweep is not in a cycle) |
| Provider timeout | 2 | `[DEPLOY_END]` shows duration; classify by err message; failure category logged |
| Delete replay | 2 | History rows give timeline; cycle_id propagated (Phase 2.3) |
| Reconciliation storm | 1 | Quota/auth cascade: same error N times across cycles; manual aggregation required |
| Stale-spec loop | 1 | Each cycle produces a stale event; manual counting required |

**Gap:** Today's median maturity is 2. Production-readiness wants 3+
for common incidents.

## Specific reconstruction gaps

### IR-1: No per-target deploy-history summary

Operators must `SELECT * FROM deployment_history WHERE target = ?
ORDER BY created`. No CLI tool, no dashboard panel, no
PocketBase-API-side aggregate.

**Severity:** Medium. The data is there; the access is manual.

**Phase 2.4 fix considered:** A GET
`/api/v1/targets/:id/timeline` endpoint that returns aggregated
history. Defer to frontend / API work in Phase 3.

### IR-2: No operation-lifetime visibility

`operations.started_at`, `updated_at` give lifetime but no derived
"how long has this been live?" or "what was the heartbeat cadence?"

**Phase 2.4 fix considered:** Same endpoint as IR-1 could derive this.
Defer.

### IR-3: Cycle_id not on disk

Phase 2.3 threaded cycle_id through dispatch logs, but the
`operations` row doesn't carry it. Reconstructing from operations
alone requires correlating timestamps with log emissions.

**Phase 2.4 fix considered:** Add `operations.cycle_id` column (Phase
2.3 deferred this; Phase 2.5 will land it during retention/cleanup
schema work).

### IR-4: No "incident snapshot" tool

For chaos events (storm, replay cascade), no tool dumps a coherent
snapshot of:
- All in-progress ops
- All targets in transient states
- All recent failures
- All sweep events

**Phase 2.4 fix considered:** An operator-facing CLI subcommand or
HTTP endpoint that returns the snapshot. Defer to Phase 2.7 / 3.

### IR-5: Heartbeat-fail-storms invisible

`[HEARTBEAT_FAIL]` is logged at default level. A 3-min DB-busy storm
produces 3 heartbeat fail logs across all live ops, but no
"persistent failures" signal.

**Phase 2.7 fix:** `[HEARTBEAT_FAIL_PERSISTENT]` after 5 consecutive.

### IR-6: No structured event for `[SUSPICION_HOLD]`

Mentioned in Phase 2.3 O-4. WARN-level emission without structured
fields. Hard to alert on.

**Phase 2.7 fix:** slog adoption.

### IR-7: Release-lost-ownership leaves no history row

Covered in [[lifecycle-closure-integrity-review]] LC-10. The
dispatcher's actual provider outcome is lost from the audit trail.

**Phase 2.7 fix:** Write a history row on `[RELEASE_LOST_OWNERSHIP]`.

### IR-8: CAS-race-loss leaves no history row

Covered in [[lifecycle-closure-integrity-review]] LC-8.

**Phase 2.7 fix:** History row on cancel.

### IR-9: Cleanup activity (Phase 2.5) needs visibility

When Phase 2.5 cleanup lands, deletions of ops/history must be
auditable. Otherwise operators investigating "where did my history
go" hit a void.

**Phase 2.5 design:** `[CLEANUP_OPS]` / `[CLEANUP_HISTORY]` log
emissions per pass; optionally write a meta-record to a "cleanup_log"
table. Documented in retention proposal.

### IR-10: Drift detection has zero forensic surface

`deployment_targets.drift_detected` is permanently false. No drift
log emissions. Operators cannot tell whether a target has manually-
mutated provider state.

**Phase 2.8 fix:** Drift visibility improvements.

## Maturity gaps prioritized

| Gap | Severity | Lands |
|---|---|---|
| IR-1 timeline endpoint | Medium | Phase 3 (frontend) |
| IR-2 operation-lifetime view | Medium | Phase 3 (frontend) |
| IR-3 cycle_id on operations | Medium | Phase 2.5 |
| IR-4 incident snapshot tool | Low | Phase 3 |
| IR-5 heartbeat-fail escalation | Low | Phase 2.7 |
| IR-6 structured suspicion event | Low | Phase 2.7 (slog) |
| IR-7 release-lost-ownership history | Medium | Phase 2.7 |
| IR-8 cancel history | Low | Phase 2.7 |
| IR-9 cleanup visibility | Medium (when cleanup lands) | Phase 2.5 |
| IR-10 drift forensic surface | Medium | Phase 2.8 |

## Phase 2.4 implementation in this area

None directly. Each gap is addressed in 2.5-2.8 phases.

## Related
- [[../phase2.3/incident-reconstruction-assessment]]
- [[../phase2.3/observability-integrity]]
