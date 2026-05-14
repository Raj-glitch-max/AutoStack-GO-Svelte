# Reconciliation Determinism Review — Phase 2.3

## Last Updated
2026-05-14

## What "deterministic" means here

Given:
- the same rows in `deployment_targets`, `rollouts`, `cloud_accounts`,
  `operations`, `deployment_history`,
- the same provider-observable state,
- the same time-bounded execution window (we accept that wall-clock
  affects backoff and circuit windows),

then two reconciler runs MUST converge on the same persisted state.
Anywhere this is not true is a divergence risk.

## Sources of nondeterminism, inventoried

| Source | Determinism risk | Today's defense | Verdict |
|---|---|---|---|
| `newCycleID` random | None — cycle_id is logging only | n/a | ✓ |
| Per-target iteration order | `SELECT ... All(&rows)` — driver-dependent ordering | Acceptable: per-target work is independent. | ✓ |
| `Reconciler.failures` (in-memory) | Restart clears it | Acceptable: circuit breaker is a transient throttle, not durable state. | ✓ |
| `Reconciler.suspicions` (in-memory) | Restart clears it; a single transient error post-restart can persist `error` after one observation instead of two. | Documented in cloud.go comments. | ✓ Acceptable. |
| Wall-clock time stamps | `last_synced`, `last_state_change_at` — read by humans only, not by reconciliation logic | n/a | ✓ |
| `recordError` / `clearError` timing | Backoff is a function of failures count, not wall-time, except for the gate "since last error". Two restarts could pick different windows. | Acceptable: backoff is a throttle, not a correctness primitive. | ✓ |
| `rolloutMovedSince` read-after-write | If the dispatcher's read races a respec write, `rolloutMovedSince` may return false even though the spec moved. SQLite WAL serializes this so it's safe in practice. Under Postgres replicas, replica lag could lie. | SQLite-only today. | ✓ for SQLite; ⚠️ for any future replica setup. |
| `claimTarget` CAS | SQLite WAL serializes; correct CAS. | ✓ | ✓ |
| `releaseTarget` release-CAS | conditional on `current_operation = :opID`; refuses to write if sweep moved the row. | ✓ | ✓ |
| `completeOperation` complete-CAS | conditional on `status = 'in_progress'`; refuses to flip a sweep-marked `failed` back to `succeeded`. | ✓ | ✓ |
| Cloud Run `CreateService` vs `UpdateService` branching | Determined by `GetService` returning err or not. A transient permission error masquerades as "doesn't exist" → CreateService → 409 conflict on next call. | None today. | ⚠️ Hazard. |
| Cloud Run `waitForServiceReady` first-observation latch | First `Ready=SUCCEEDED` returns immediately; no debounce. | Dispatcher persists `updating` (not `running`); status-poller re-validates. | ✓ |
| `GetStatus` condition precedence | Ready > ConfigurationsReady, fixed in Phase 1.9. | ✓ | ✓ |
| `isAllowedTransition` | Pure function of (previous, next). | ✓ | ✓ |
| `ClassifyError` substring matching | Substring patterns are order-dependent (auth checked before quota, etc.). Two errors containing both "401" and "quota" map deterministically because auth checks first. | ✓ | ✓ |
| `sanitizeError` truncation | Fixed 200-char break on first sensitive-pattern match; deterministic. | ✓ | ✓ |
| `flipCloudTargetsToPendingOnRespec` | Skips in-flight; flips others to pending. Two simultaneous PATCHes to the same rollout would each fire the handler, but the second is a no-op (status already pending). | ✓ | ✓ |
| `markCloudTargetForDestroy` | Skips in-flight (silently). **This is the destroy-intent-loss bug**, not a determinism issue per se — both runs converge on "didn't flip to deleting", but the intent is dropped. | None today. | 🔴 Lineage hazard, not a determinism hazard. |
| Cycle-level panic clearing failures | Phase 1.9 fix: `reconcileAll` recovery does NOT clear failures. | ✓ | ✓ |
| `dispatchDeploy` defer-panic path | If a panic fires mid-flight, recovery marks op `failed`, releases target → `error`, writes a failed history row. Subsequent ticks see `error` and respect the circuit. | ✓ | ✓ |
| `succeeded_stale` outcome | Does not increment failures; does not clear them. A perpetually-respec'd rollout never trips circuit. | None today. | ⚠️ Mild divergence — see [[replay-safety-assessment]] §3. |

## Anti-determinism subroutines: the "create vs update" gamble

`Provider.Deploy` decides between `CreateService` and `UpdateService`
based on whether `GetService` returns existing. The transient cases:

| GetService outcome | Code behavior | Hazard |
|---|---|---|
| 200 OK, existing populated | enters Update path with `service.Name = existing.Name`. | ✓ Safe. |
| 404 NOT_FOUND | enters Create path. | ✓ Safe. |
| 403 PERMISSION_DENIED transiently | `err != nil`, enters Create path. Subsequent CreateService either succeeds (creating a duplicate? — no, service-name unique) or fails (409 conflict if it actually exists). | ⚠️ If the service DOES exist and the perm check just failed transiently, Create will 409 → returned as deploy error → target → error. Operator must investigate. **Convergence:** next dispatch (after operator clears) calls GetService again, ideally with creds restored, enters Update. **Determinism:** the failure path is reproducible. **Truthful state:** error status is honest. |
| 503 UNAVAILABLE transiently | err != nil, enters Create path → CreateService either succeeds against a non-existent name or 409s. | Same analysis as above. |

**Verdict:** The Create-vs-Update branch is technically nondeterministic
under transient API errors, but the failure mode is loud (409 conflict
shows up as an error), self-correcting on next dispatch, and never
silently corrupts state. Acceptable. **Phase 2.5 work** could harden
this by retrying GetService once on transient error before deciding the
branch — narrow improvement, not a Phase 2.3 blocker.

## Replica-lag hazard (forward-looking)

`rolloutMovedSince` does a `FindRecordById("rollouts", ...)` read after
the dispatcher returns. Under SQLite, this read sees the post-write
state of any respec that completed during deploy. Under Postgres with
read replicas, the dispatcher could read a stale revision and miss the
stale-spec detection.

Today: SQLite only. Documented as a Postgres-migration-prereq in
`[[../known-issues/deferred-operational-hardening]]`.

## Phase 2.3 implementation in this area

None directly — the determinism story is already strong. The fixes
landing this phase (history at intents, cycle-ID propagation,
heartbeat-aware sweep) improve observability, not determinism.

## Related
- [[replay-safety-assessment]]
- [[truthful-state-assessment]]
- [[../reconciler/reconciliation-guarantees]]
