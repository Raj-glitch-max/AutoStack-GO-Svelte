# Dangerous Edge Cases — Control-Plane Paranoia Inventory

## Last Updated
2026-05-14 (Phase 2.0 — deploy execution wired)

These are concrete failure modes the system can produce. Each is paired
with current behavior and current mitigation. Items marked 🔴 remain
unmitigated as of Phase 1.9.

## Silent corruption / phantom state

### 🟢 (mitigated) Phantom rollout-status writes
Reconciler previously called `rollout.Set("status", ...)` against a
collection with no `status` column. PocketBase silently dropped the
write. Logs claimed a transition; the row never changed.
**Mitigation (1.9):** Writes removed. To restore: add columns via migration.

### 🟢 (mitigated) Phantom rollback success
`Rollback` returned a `DeployResult` reporting success regardless of
whether the (empty) Service update actually changed anything.
**Mitigation (1.9):** Body replaced with `ErrNotImplemented`.

### 🟢 (mitigated, Phase 2.0) Phantom cloud deploy
`HandleRolloutCreate` previously called `k8s.CreateOrUpdateRollout`
unconditionally for cloud target types. The UI saw "rollout created"
but Cloud Run was never touched.
**Mitigation (2.0):** Handler now branches on `target_type`. Cloud
rollouts create a `deployment_targets` row in `pending`; the reconciler
dispatcher claims it via CAS and calls `Provider.Deploy`. See
[[deploy-dispatch-design]].

### 🟢 (mitigated) Status oscillation / regression on flap
`GetStatus` previously defaulted to `"pending"`; combined with the absence
of a transition guard, a healthy `running` target could regress to
`pending` on a single inconclusive observation.
**Mitigation (1.9):** Provider returns `"unknown"`; reconciler refuses
`running → pending|creating` single-step.

### 🟢 (mitigated) Circuit-breaker reset on unrelated panic
Panic in `reconcileAll` cleared the entire `failures` map, re-arming
retries for known-broken targets.
**Mitigation (1.9):** Only `reconcileOne` increments its own target on
panic; `reconcileAll` does not clear.

## Security

### 🟢 (mitigated, Phase 2.0) `credentials_encrypted` is plaintext
Previously the column stored plaintext under a misleading name.
**Mitigation (2.0):** `pkg/secrets` AES-256-GCM with env-var key.
Write boundary in `HandleCloudAccountCreate` encrypts; use boundaries
in reconciler and validate handlers decrypt. Legacy plaintext rows
re-encrypt on next validate. Process refuses to start
`[ENCRYPTION_NOT_CONFIGURED]` if the key is missing. See
[[encryption-design]]. Remaining gap: KMS-grade key management
(deferred to Phase 3).

## Concurrency

### 🟢 (mitigated) Failure-map data race
`getFailureCount(r.failures)` iterated the map while holding
`lastErrorMu` (the wrong lock). Per the Go memory model this is a race.
**Mitigation (1.9):** `backoffDuration()` reads `failures` under
`failureMu`; `getFailureCount` removed.

### 🟢 (mitigated) Provider singleton mutable fields
`cloudrun.Provider` carried `projectID` and `region` that were mutated
per call. The registered singleton would race under any future concurrent
reconcile.
**Mitigation (1.9):** Struct is empty; locals only.

### 🔴 Multi-pod reconciler race
Two backend pods → two reconcilers writing to the same `deployment_targets`
rows. SQLite WAL mode serializes writes; Postgres path would not.
**Mitigation:** None. Requires leader election or row versioning.

## Backoff and circuit breaker

### 🟢 (mitigated) Per-target failures never triggered backoff
`reconcileWithBackoff` checks `lastErrorTime`, set only by
`recordError()`. Per-target failures called
`recordTargetFailureWithCategory` which never touched `lastErrorTime`.
**Mitigation (1.9):** `recordTargetFailureWithCategory` now calls
`recordError()` for transient/timeout/permanent failures.

## State and lifecycle

### 🔴 Stuck-state detection unimplemented
state-model.md §"Stuck State Detection" promises action after N minutes
in `creating`/`updating`/`deleting`. No `last_state_change_at` column,
no timer, no implementation. A target stuck in `creating` will stay
there until manually removed.

### 🔴 Drift detection unimplemented
`deployment_targets.drift_detected` is permanently `false`. No spec-vs.-
actual comparison is ever performed.

### 🔴 `provider` enum is three different alphabets
- `cloud_accounts.provider` ∈ `{aws, gcp, azure}`
- `deployment_targets.provider` ∈ `{aws-ecs, gcp-cloudrun, azure-aca}`
- `rollouts.target_type` ∈ `{kubernetes, ecs, cloudrun, aca}`

No cross-validation. A `gcp` account paired with an `aws-ecs` target
paired with a `cloudrun` rollout would parse and (notionally) execute.

## Visibility

### 🟢 (mitigated) Placeholder `targets_queued=...` log
`reconcileAll` previously logged `cycle_start targets_queued=...` with
the literal three dots.
**Mitigation (1.9):** Replaced by `cycle_start target_count=N` after the
query returns.

### 🔴 No correlation IDs
No `cycle_id`, no per-attempt ID. Cannot grep all log lines for one
target in one cycle. Required for 3 AM debugging.

### 🟢 (mitigated, Phase 2.0) `deployment_history` write-never
**Mitigation (2.0):** The dispatcher writes two rows per action:
one at dispatch (`status=in_progress`) and one at outcome
(`status=success|failed`). Records remain immutable per the migration's
rule set.

## Phase 2.0 — new hazards introduced by real deploy execution

### 🟠 Orphan provider resource on hard rollout-delete
Hard-deleting a cloud rollout cascade-deletes its `deployment_targets`
row, but the dispatcher does NOT run Destroy first. A previously-deployed
Cloud Run service is left orphaned (ongoing cost).
**Mitigation today:** Logged as `[CLOUD_ORPHAN_RISK]`. Operators should
set `endDate` first (which triggers destroy via the
`status=deleting` path) before deleting the rollout record.
**Required next:** Orphan-cleanup scanner (Tier-2 in
[[deferred-operational-hardening]]).

### 🟠 Cloud rollout spec-update propagation deferred
A cloud rollout's image / env / scale changes after first deploy are
NOT pushed to the cloud target. Logged as `[CLOUD_UPDATE_DEFERRED]`.
**Mitigation today:** None. Operator must end the rollout and create a
fresh one.
**Required next:** Spec-change detection that flips
`deployment_targets.status` back to `pending` for re-dispatch.

### 🟠 Long-running deploy past 20-minute sweep threshold
The crash-recovery sweep marks any `in_progress` op older than 20
minutes as `failed`. A genuine deploy still running at that point will
have its op overwritten by the sweep; the dispatcher will then write
conflicting outcome status when it eventually returns.
**Mitigation today:** `DeployTimeout=15 min` caps the dispatcher.
Sweep threshold is 20 min, giving a 5-min margin.
**Required next:** Operation lease/heartbeat — refresh `updated_at`
during a long deploy; sweep ignores ops with recent heartbeat. Phase 2.5.

### 🟠 Stale-spec window
Between CAS claim (T0) and Deploy return (T1, up to 15 min later), a
rollout can be updated. The dispatcher checks `rollout.updated` at T1
and marks the op `succeeded_stale` if it advanced. There is still a
window where the provider's state reflects the spec at T0 until the
next dispatch redeploys (one tick, ~30s).
**Mitigation today:** `[DEPLOY_STALE]` logged; target released to
`pending` for fresh dispatch. History row written with `status=failed`
and message "stale spec" so it's auditable.
**Why we don't try harder:** A read-and-recheck-before-update CAS at
Deploy *time* requires provider-side optimistic concurrency, which
Cloud Run doesn't expose for service updates.

## Related
- [[lifecycle-assumptions]]
- [[reconciliation-guarantees]]
- [[restart-behavior]]
- [[control-plane-paranoia-findings]]
- [[deploy-dispatch-design]]
- [[operation-ownership]]
- [[encryption-design]]
- [[deferred-operational-hardening]]
