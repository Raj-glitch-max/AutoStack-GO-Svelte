# Eventual-Consistency Hazard Inventory — Phase 2.3

## Last Updated
2026-05-14

## Premise

Cloud Run is eventually consistent. The following assumptions, made by
AutoStack today, MUST be evaluated against this premise:

- Provider state can be **read without lag** after a write.
- A 200 OK from the provider implies the resource is **fully in the
  intended state**.
- A NOT_FOUND from the provider implies the resource **does not exist
  anywhere** in the provider.
- Two consecutive reads will return **the same state**.
- A revision marked `Ready=SUCCEEDED` is **stable for the rest of its
  lifetime**.

The system assumes some of these. Below is the inventory.

## Hazards

### E-1: `Deploy` → `waitForServiceReady` first-observation latch

**Today:** `waitForServiceReady` returns on the first `Ready=SUCCEEDED`
observation. No "stable for N seconds" debounce.

**EC reality:** Cloud Run can flip `Ready=SUCCEEDED → Ready=RECONCILING
→ Ready=SUCCEEDED` during revision traffic shift, especially when
`min_instances > 0` and a new revision is rolling.

**Risk if violated:** Dispatcher claims success, persists `updating`
(intentionally, to allow GetStatus to re-validate), history says
success. Next status-poll may see RECONCILING transiently. The
suspicion counter handles `updating → error` flaps but not
`updating → creating`. If the provider reports RECONCILING as
`creating`, the transition guard refuses `updating → creating`, target
stays at `updating`. Eventually returns `running`. **Convergent.**

**Verdict:** Acceptable. Mitigation by dispatcher persisting `updating`
not `running` + transition guard + suspicion counter is sufficient.

### E-2: `GetStatus` single-observation classifier

**Today:** Single `GetService` call. Maps Ready conditions to status
strings.

**EC reality:** Two `GetService` calls 10 seconds apart can show
different `Ready` state during a revision swap.

**Risk:** Per-cycle status flap. Persisted target oscillates within
allowed transitions.

**Mitigation today:** Transition guard refuses
`running → pending|creating`; suspicion counter holds
`updating → error` until second observation. Both partial.

**Phase 2.5 work:** Stable-for-N-cycles requirement before persisting
state transitions, especially `updating → running`. Currently
`updating → running` requires one positive observation. A 2-of-3
observation requirement would tolerate the EC flap better. Deferred.

### E-3: `Destroy` post-API-200 visibility lag

**Today:** `Destroy` returns nil once `DeleteService` returns 200.
Dispatcher persists `status=deleted`.

**EC reality:** Cloud Run may continue listing the service for 10-60
seconds after `DeleteService` accepts. A `GetService` during this
window returns the service.

**Risk if reconciler polled again:** The terminal-`deleted` skip-guard
prevents this (target with status=deleted is never polled). So
post-destroy AutoStack reports `deleted` immediately; provider may
briefly disagree.

**Operator impact:** Operator who hits delete-rollout immediately after
target reaches `deleted` succeeds. The actual cloud cleanup may still
be in progress. Cost-wise negligible (already-deleting service is not
charged). Trust-wise: a one-shot small lie.

**Mitigation today:** None.

**Phase 2.5 work:** Post-destroy NOT_FOUND confirmation poll. The
dispatcher's success path would loop `GetService` until NOT_FOUND or
timeout, then mark `deleted`. Adds 10-60s to destroy duration; closes
the truthfulness gap.

### E-4: `Deploy` create-vs-update branch on transient GetService error

**Today:** `GetService` returns err → branch to Create. `GetService`
returns 200 + existing → branch to Update.

**EC reality:** A transient 503 / 504 / 429 on `GetService` would cause
the dispatcher to take the Create branch against a service that
exists. CreateService would 409 conflict. Returned as deploy error.

**Risk:** False error; the service is fine. Target → `error`. Next
dispatch (after operator clears error and respec triggers redispatch)
re-reads GetService, which may succeed → Update branch → convergent.

**Mitigation today:** None.

**Phase 2.5 work:** Retry GetService once on transient error before
deciding create/update branch. Narrow, low-risk improvement. Not a
Phase 2.3 priority because the failure mode is loud, not silent.

### E-5: Stale-spec detection via `rollout.updated` read after Deploy

**Today:** `rolloutMovedSince` reads `rollouts.updated` after Deploy
returns. SQLite WAL makes this a fresh read; safe.

**EC reality (Postgres future):** Read replicas could lag the primary's
last-write. The dispatcher reads a stale `updated` and concludes the
rollout did NOT move, even though it did. Result: claim success against
a stale spec.

**Mitigation today:** N/A (SQLite only).

**Phase 2.5 work:** When PocketBase migrates to Postgres, ensure
`rolloutMovedSince` reads from primary, not replica. Documented as a
migration prerequisite.

### E-6: Cloud Run revision name reuse across recreate

**Today:** AutoStack uses `serviceName = "autostack-" + rolloutID` as
the Cloud Run service name. If a rollout is deleted and a new rollout
with the same `rolloutID` is created (operator action; not the default
generated-id path, but possible), the existing-or-deleted Cloud Run
service may still be cleaning up → CreateService 409s or worse, the
service "comes back" from Cloud Run's tombstone window.

**EC reality:** Cloud Run's reuse semantics for recently-deleted
service names is unspecified. We have not tested this.

**Mitigation today:** rollouts.id is generated by `util.GenerateId(15)`
random. Practical collision probability is negligible. The hazard only
applies to operator-set IDs.

**Verdict:** Defer; documented; unlikely.

### E-7: `Operations.updated_at` heartbeat write under DB busy

**Today:** Heartbeat `UPDATE operations SET updated_at` may fail with
`SQLITE_BUSY` if the DB is under write pressure. Heartbeat logs
`[HEARTBEAT_FAIL]` and continues. Sweep (with Phase 2.3
heartbeat-aware change) uses `updated_at` to decide liveness — a
heartbeat-failing op may be incorrectly classified as abandoned.

**EC reality:** SQLite WAL serializes; transient busy is rare but
possible.

**Mitigation today:** Single missed heartbeat is not fatal; the next
heartbeat catches up. The 2× heartbeatInterval threshold gives 1
missed heartbeat of margin.

**Phase 2.5 work:** Heartbeat retry on busy; defer.

### E-8: `last_state_change_at` and `last_synced` clock skew

**Today:** Wall-clock timestamps on dispatcher / reconciler / sweep
writes. Single-pod: same clock. Multi-pod: possible skew.

**Risk:** Stuck-state detection (deferred) would rely on this timestamp.
If clocks skew, "stuck for 10 min" reads vary across pods.

**Mitigation today:** Single-pod only.

**Phase 2.5 work:** Use DB time (`CURRENT_TIMESTAMP`) instead of
process time. Schema-side fix.

## Cross-component summary

The system makes **two strong EC assumptions** today that warrant
Phase 2.5 attention:

1. **`Destroy` truthfulness:** The post-API-200 visibility lag is the
   single largest "we lie briefly" gap. Confirmation poll fixes it.
2. **`GetService` create/update branching:** Transient errors can
   silently corrupt the dispatcher's branch decision. Single-retry
   fixes it.

Everything else has documented mitigations or is acceptably honest
under EC.

## Phase 2.3 implementation in this area

None — every fix here is a defer-to-2.5 item with a clear design.

## Related
- [[truthful-state-assessment]]
- [[delete-orphan-risk-assessment]]
- [[../providers/eventual-consistency-assumptions]]
