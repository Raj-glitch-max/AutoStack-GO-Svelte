# Operational Maintainability Review — Phase 2.3

## Last Updated
2026-05-14

## Question

Does the current control-plane architecture allow future hardening
(multi-pod, queue-based reconciliation, higher target counts,
rate-limiting, prioritization, backpressure) **without massive rewrite**?
The point is to avoid architectural dead-ends, not to implement those
systems now.

## Architectural shape today

- Single goroutine reconciliation loop in the PocketBase server process.
- Reconciler tick → SELECT → per-target sequential processing.
- Per-target dispatch happens inside the same goroutine; in-flight
  status-poll skip prevents re-entry.
- `operations` collection persists in-flight state; `current_operation`
  on targets is the lock.

## Forward-compatibility analysis

### Multi-worker reconciliation

**Today's blocker:** Single goroutine processing.

**Forward-fit:** Move per-target dispatch into a worker pool. The CAS
on `current_operation` already serializes per-target. The reconciler
already separates "find dispatchable targets" from "do dispatch", so a
pool reading from a channel is a straightforward refactor.

**Verdict:** ✓ Future-fit. No schema change needed.

### Queue-based reconciliation

**Today:** Time-driven (30s ticker) plus event-driven via PocketBase
hooks (rollout create/update/delete → status flip).

**Forward-fit:** Move the ticker into a "scan for work" producer that
emits target IDs onto a channel; workers consume. The 30s tick remains
the heartbeat for "find drift"; PocketBase hooks emit "immediate"
events.

**Verdict:** ✓ Future-fit. The state shape supports a queue without
schema change.

### Higher deployment counts

**Today:** Each cycle SELECTs all cloud deployment_targets. For N
targets, this is one query, N status-polls (or fewer due to skip
guards). Scaling to 10K targets means 10K provider API calls per 30s →
333 RPS. Cloud Run quota typically allows ~1000 RPS read for
GetService — fits.

**Forward-fit blockers:**
- Per-target provider client construction per call — each call to
  `Provider.GetStatus` builds a new `run.ServicesClient`. Wasteful at
  scale. Future fix: per-account client pool (low effort).
- Sequential per-cycle processing — at scale, cycle duration grows
  linearly with target count. Worker pool fixes this.

**Verdict:** ✓ Future-fit. No schema change. Client pooling is a clean
optimization.

### Provider rate limiting

**Today:** None. Each tick hits provider for every target.

**Forward-fit:** Per-provider rate-limiter (token bucket). Lives in
provider package; transparent to the reconciler.

**Verdict:** ✓ Future-fit.

### Reconciliation prioritization

**Today:** First-in-first-out per target order from SQL.

**Forward-fit:** Add `priority` column or `next_eligible_at` to
deployment_targets; worker pool reads by priority order.

**Verdict:** ✓ Future-fit; trivial schema add.

### Operation backpressure

**Today:** No backpressure. Reconciler picks up all eligible targets
each cycle.

**Forward-fit:** Worker pool with bounded concurrency naturally
provides backpressure. CAS claim re-tries on conflict.

**Verdict:** ✓ Future-fit.

### Safer cleanup semantics

**Today:** Sweep is startup-only. No periodic runtime sweep, no
operation TTL/archival.

**Forward-fit:** Add a "janitor" goroutine running periodically (e.g.,
hourly). Runtime sweep: ops with stale heartbeat + pod_id mismatch.
Archival: ops in terminal status older than N days → archive table or
delete.

**Verdict:** ✓ Future-fit. Phase 2.5 work.

## Dead-end risks (architectural decisions that would block future
hardening)

### DE-1: PocketBase server process tight-coupling

The reconciler runs in the same process as the HTTP server. Splitting
into a separate process would require:
- Externalizing the DAO (currently `app.Dao()` only works in-process).
- Replicating the env initialization.

**Risk:** Tight coupling. **Mitigation:** Accept for now; revisit when
HA is required. The PocketBase project has an embedded-DB philosophy
that this matches.

### DE-2: SQLite as source of truth

SQLite is a single-writer database. Multi-pod writes serialize at the
WAL lock. This works at small scale but eventually becomes a
contention bottleneck.

**Migration path:** PocketBase supports Postgres. The CAS patterns used
here (UPDATE...WHERE) are portable. The `current_operation` predicate
on null-or-empty-string is the only quirk; Postgres can be made to
treat empty-string-as-null with a custom check or migration to
strictly-null semantics.

**Risk:** Medium. **Mitigation:** Documented in
[[../known-issues/deferred-operational-hardening]]. Test plan for
Postgres migration deferred.

### DE-3: Provider singleton registry

`providers.RegisterProvider(name, p)` is a process-global map. Two
reconciler instances in the same process would share the same provider
singletons. This is correct only because the providers are stateless
(Phase 1.9 fix).

**Risk:** Low. **Mitigation:** Phase 1.9 already de-statified the
Cloud Run provider.

### DE-4: Heartbeat as the only liveness primitive

If multi-pod expansion adds pod-identity stamping, the heartbeat-based
liveness check needs to be aware of pod-identity expiry. A pod-id that
hasn't heartbeated in N minutes is "dead"; ops it owns can be swept.

**Forward-fit:** Add a `pod_heartbeat` table or similar. Heartbeat
goroutine writes its own pod row; sweep reads it.

**Verdict:** ✓ Future-fit, with one additional schema. Documented.

### DE-5: Tight coupling between status-poll path and reconciler tick

Status-poll is currently inside the reconciler. A future "status
service" (separate poll cadence per target, batched provider reads)
would extract this. Not blocked; just not yet abstracted.

**Verdict:** ✓ Future-fit.

### DE-6: `deployment_targets.status` enum is the public surface

The status enum is hard-coded in PocketBase migration. Adding new
values (`rolling_back`, `stale`, etc.) requires a migration.

**Risk:** Low. **Mitigation:** Migrations are cheap; just discipline.

## Anti-patterns that are NOT in the code today

These are anti-patterns the codebase has **avoided** so far, which is
worth recording so future work doesn't introduce them:

- **No "wrap the provider in a state machine the provider also has"** —
  AutoStack does not duplicate Cloud Run's Revision/Service state
  machine in PocketBase. Status is an abstraction layer, not a mirror.
- **No "implement features by reading provider state without a
  source-of-truth"** — Every status persistence is gated by transition
  guard, sweep, dispatcher ownership. Operator intent in PocketBase is
  always the desired state.
- **No "background goroutine per target"** — single reconcile loop
  iterates all. Avoids goroutine leak at scale.
- **No "callbacks from provider into AutoStack"** — provider calls are
  request/response; no webhook reverse path.
- **No "soft-coupled config that requires reading multiple env vars"**
  — `AUTOSTACK_ENCRYPTION_KEY` is the only operational env var. Future
  config goes in `env.Config`.

## Phase 2.3 implementation in this area

None. Maintainability is good. The audits above identify Phase 2.5
work; the architecture supports them without rewrite.

## Related
- [[lro-survivability-review]]
- [[../known-issues/deferred-operational-hardening]]
- [[../architecture/current-architecture]]
