# Control-Plane Paranoia Findings

## Last Updated
2026-05-14 (Phase 1.9 principal review)

A focused review of failure modes a deployment control plane MUST behave
correctly under. Each finding is paired with current behavior and the
1.9 disposition.

## Operational ownership map

| Concern | Owner | State (1.9) |
|---|---|---|
| Cloud deploy dispatch | **Nobody** | 🔴 Missing — controller calls k8s unconditionally |
| Cloud deploy lifecycle | Reconciler (status-only) | 🟠 Half-owned |
| Rollback | Provider method | 🟢 Refused honestly |
| Cleanup of failed deploys | **Nobody** | 🔴 Missing |
| Retry decision | Reconciler (per-target circuit) | 🟢 Owned |
| Stale-state cleanup | **Nobody** | 🔴 Missing |
| Operation expiry | **Nobody** | 🔴 Missing (operations don't persist) |
| Crash recovery | Implicit (next reconcile re-polls) | 🟠 Brittle |
| Observability | Reconciler `log.Printf` | 🟠 Minimal — no correlation IDs |
| Transition correctness | Reconciler (1.9 transition guard) | 🟢 Owned |
| History writes | **Nobody** | 🔴 Missing — collection exists, no writer |
| Credential encryption | **Nobody** | 🔴 Field name lies; plaintext at rest |

## Incident survivability

Walked through directive-specified scenarios:

| Incident | Current behavior | Survives? |
|---|---|---|
| Cloud Run returns stale `Ready` | Transition guard refuses single-observation regressions; status flaps within guard envelope | 🟢 Yes |
| Rollback succeeds partially | Refused entirely; cannot partial-succeed | 🟢 Yes (by refusal) |
| Provider returns inconsistent revision order | Rollback refused; never selects from list | 🟢 Yes (by refusal) |
| Service deleted out-of-band | `GetStatus` → `not found` → category `permanent` → circuit opens; target stays in last known state forever | 🟠 Degrades silently |
| Reconciler crashes mid-delete | Restart re-polls; `Destroy` is idempotent on `NOT_FOUND` | 🟢 Yes |
| Retry storm during recovery | Circuit opens at 5 failures; per-target failures now also trigger cycle backoff (1.9 fix) | 🟢 Yes |
| Stale deployment overwrites newer desired state | Possible — no version/etag check | 🔴 No |
| `deployment_history` inconsistent | Empty by construction; no inconsistency possible | N/A |
| Multi-pod reconcilers race | SQLite WAL serializes; Postgres path would not | 🟠 Depends on storage |

## Truthful state reporting audit

Three places the system previously lied — all addressed in 1.9:

1. `rollouts.status` writes to a missing column → silent no-op, log
   claimed transition. **Fixed** by removing the writes; documented in
   [[lifecycle-assumptions]].
2. `GetMetrics` / `CheckQuotas` returned successful-looking zeros.
   **Fixed** by returning `ErrNotImplemented`.
3. `Rollback` reported success when no actual revision swap occurred.
   **Fixed** by refusing entirely.

Remaining truthfulness gaps (Tier 5 in
[[deferred-operational-hardening]]):

- No way for the UI to distinguish "no Deploy was ever attempted" from
  "Deploy attempted but failed early" — because Deploy is never
  attempted.
- `last_synced` updates on success and on refused observation alike.
  An operator cannot tell from the row alone whether the last poll
  produced a real status write.

## Eventual-consistency hazards (snapshot)

See [[eventual-consistency-assumptions]] for full detail.

- Single-observation `GetStatus` → flap possible within transition-guard
  envelope.
- Post-delete confirmation lag → service briefly observed as `running`
  after `deleting`.
- Region-scoped permission validation absent.
- No readiness debounce in `waitForServiceReady`.

## Concurrency / restart hazards (snapshot)

See [[restart-behavior]] and [[reconciliation-guarantees]].

- Provider singleton now stateless (1.9 fix).
- Failure-map data race fixed (1.9 fix).
- Whole-map clear on panic removed (1.9 fix).
- Multi-pod race remains an open hazard.
- Operations collection missing — mid-Deploy crash unrecoverable when
  Deploy dispatch lands.

## Observability blind spots

An operator at 3 AM trying to answer:

| Question | Answerable today? |
|---|---|
| Which cycle did this target fail in? | 🔴 No cycle ID |
| Show me every action on rollout X. | 🔴 `deployment_history` empty |
| Did the deploy actually happen? | 🔴 No call site |
| Why is this target stuck in `creating`? | 🔴 No `last_state_change_at` |
| Was the last poll a real status write or a refused observation? | 🔴 Same `last_synced` either way |
| Is the provider rate-limited globally or per-account? | 🔴 No per-account telemetry |

## Future scale trajectory

Does the current architecture foreclose evolution?

- 🟢 The reconciler can be turned into a worker pool — single-threaded
  today but the per-target work is well-isolated.
- 🟢 An operations collection can be added without changing existing
  schema.
- 🟠 `Reconciler.started` is process-local — must be replaced before
  multi-pod is supported.
- 🟢 Provider singleton is safe under concurrent callers (1.9 fix).

## Related
- [[lifecycle-assumptions]]
- [[reconciliation-guarantees]]
- [[restart-behavior]]
- [[provider-limitations]]
- [[eventual-consistency-assumptions]]
- [[rollback-semantics]]
- [[dangerous-edge-cases]]
- [[deferred-operational-hardening]]
- [[correctness-limitations]]
