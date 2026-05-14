# Phase 2.2 Assessment — Deployment Validation, Recovery & Operational Hardening

## Last Updated
2026-05-14

## Scope

This document covers findings from end-to-end validation of the Phase 2.1
cloud deployment system (reconciler, dispenser, provider, sweep, encryption).
Kubernetes system is untouched and out of scope.

---

## 1. Deployment Validation Summary

### End-to-End Flow

```
Rollout POST (handleRolloutCreate)
  → createPendingDeploymentTarget() inserts deployment_targets row [status=pending]
  → Reconciler.Start() [calls SweepAbandonedOperations first]
  → reconcileAll() [every 30s]
    → reconcileOne() per target
      → shouldDispatchDeploy? yes if status=pending + no current_operation
        → dispatchDeploy()
          → buildDeploySpec (yaml → DeploySpec)
          → createOperation (opens operations row with opID)
          → claimTarget (CAS: current_operation=null → opID, status: pending→creating)
          → heartbeat goroutine starts (refreshes ops.updated_at every 60s)
          → Provider.Deploy()
            → provider builds GCP CreateServiceRequest
            → waitForServiceReady polls until Ready=SUCCEEDED
          → stale = rolloutMovedSince(rolloutID, rolloutRevision)?
            → no: completeOperation(succeeded), releaseTarget(updating), writeHistory(success)
            → yes: completeOperation(succeeded_stale), releaseTarget(pending)
          → heartbeat goroutine exits (deployCtx cancelled)
      ← Deployment continues via status polling on subsequent ticks
```

### Correctness Findings

**✅ CAS claim ownership is correct.** Only one operation can claim a given
target at a time. The WHERE clause `(current_operation = '' OR current_operation IS NULL)`
combined with the SQLite WAL ensures serialization. Release-CAS uses
`WHERE current_operation = :opID` so the dispatcher's release is a no-op
if the sweep moved the row.

**✅ Status transition guard is correct.** `isAllowedTransition` in cloud.go
refuses: deleted→*, running→pending, running→creating, updating→pending,
updating→creating. This prevents a single NOT_FOUND or transient flap from
regressing a healthy target.

**✅ `unknown` status handled gracefully.** When GetStatus returns "unknown"
(no actionable condition), updateTargetStatus updates `last_synced` without
writing a status enum, preventing corruption of the status field.

**✅ Release-CAS prevents sweep-conflict overwrites.** When the dispatcher
returns after the sweep has already moved a target to `error`, the release
UPDATE matches 0 rows (current_operation no longer points at our opID) and
logs `[RELEASE_LOST_OWNERSHIP]`.

**✅ Panic recovery in dispatchDeploy** catches panics from provider calls
and ensures both `completeOperation` and `releaseTarget` fire, preventing
orphaned in-flight state. dispatchDestroy has the same pattern.

**✅ Stale-spec detection** stamps the rollout's `updated` timestamp at claim
time. If it changes while Deploy is in flight, the dispatcher releases back
to `pending` (not `running`) so the next cycle re-deploys cleanly.

**✅ Suspicion tolerance for `updating` targets.** Two consecutive error
observations from an `updating` target are required before `error` is
persisted. This prevents a Cloud Run Ready=FAILED transient during revision
ramp from corrupting the on-disk status.

---

## 2. Replay & Restart Safety Assessment

### Startup Sweep Correctness

The sweep (sweep.go `SweepAbandonedOperations`) runs synchronously inside
`Reconciler.Start()` under startMu, BEFORE the ticker goroutine launches.
This invariant is enforced in code, not just in documentation.

Every `in_progress` operation is marked `failed` with message
"abandoned: process restart while in flight". The corresponding
`deployment_targets` row is set to `status=error` and `current_operation`
is cleared (only if it still points at the sweep's op — avoiding
collisions with a dispatcher that started before the sweep).

**Limitation:** The sweep marks ALL in_progress ops regardless of age.
A multi-pod deployment where pod A's op is still alive when pod B restarts
would mark pod A's op as abandoned. This is deferred to Phase 3.0
pod-identity stamping. Phase 2.1 documents this hazard.

### Restart Replay Behavior

After restart, the sweep neutralizes all in-flight operations. The
reconciler then processes targets as if they were fresh:

- Targets in `pending` → dispatchDeploy fires on next tick
- Targets in `error` → circuit breaker holds, requires operator reset
- Targets in `updating` → status-poll resumes, GetStatus resolves actual state

**No duplicate deploys occur** because the sweep clears current_operation
before the ticker starts. The CAS in `claimTarget` correctly prevents two
dispatchers from claiming the same target.

**No state corruption on fast restart** because the sweep runs synchronously
before the ticker can fire. There is no window where a dispatcher's in-flight
op survives a process restart without being marked abandoned.

### CAS Claim Race Safety

The SQL CAS in `claimTarget`:
```sql
UPDATE deployment_targets
   SET current_operation = {:op}, ...
   WHERE id = {:id}
     AND (current_operation = '' OR current_operation IS NULL)
     AND status IN ('pending', 'deleting')
```
Under SQLite WAL (single-writer), concurrent writers serialize at this
UPDATE. Only one writer gets `RowsAffected() == 1`. This is the correct
single-node CAS pattern and is forward-compatible with Postgres.

---

## 3. Cloud Run Correctness Assessment

### Deploy — CreateServiceRequest [FIXED this session]

**Issue fixed:** The `CreateServiceRequest` was being populated with both
`Service.Name = "projects/P/locations/R/services/N"` (fully-qualified path)
AND `ServiceId = "N"` (short service name). GCP internal validation rejects
this as conflicting.

**Fix:** Drop `ServiceId` from `CreateServiceRequest`. GCP derives the
service ID from the trailing segment of `Service.Name`.

Affected file: `pkg/providers/cloudrun/provider.go` line ~185.

### Deploy — target_config [FIXED this session]

**Issue fixed:** `target_config` from PocketBase is a JSON field that returns
as `map[string]interface{}` on dbx.Select. The original type-assertion to
`string` always failed, leaving `targetConfig` nil/empty.

**Fix:** Handle all three PocketBase return types (map, string, []byte)
with a type switch. Also thread `targetConfig` through `dispatchDeploy` into
`DeploySpec.TargetConfig` so Cloud Run's `min_instances` scaling works.

Affected files: `pkg/reconciler/cloud.go` (parse), `pkg/reconciler/dispatch.go`
(signature + thread-through).

### Deploy — BuildDeploySpec Missing Fields

**Issue documented, not fixed:** `BuildDeploySpec` constructs `EnvVar` from
`spec.secrets[]` entries, but:
1. The manifest `spec.secrets` format (`Name`, `Value` plain strings) does
   not carry the secret SOURCE (Kubernetes secretRef, GCP Secret Manager,
   AWS Secrets Manager, etc.)
2. All values are passed as plaintext env vars to the Cloud Run container
3. GCP Cloud Logging may capture these in plaintext in container logs
4. No mechanism exists to use GCP Secret Manager for secret injection

**Severity:** Medium. All values are exposed as env vars. Any user who can
edit the rollout manifest can read any other user's env var values stored
as rollout secrets.

**Fix:** Extend `spec.secrets` and `spec.env` to carry a `source` field
(e.g., `"gcp-secret-manager"`, `"plaintext"`). Map `"gcp-secret-manager"`
to Cloud Run's `env.projectNumRefs` etc. Deferred to Phase 2.5.

### GetStatus — Condition Precedence Logic

The condition precedence in GetStatus handles the key cases:
- `Ready=SUCCEEDED` → `running` (correct)
- `Ready=FAILED` → `error` (correct)
- No Ready condition + `ConfigurationsReady=RECONCILING` → `creating` (correct)

The fallback to `unknown` when no condition matches is honest — it doesn't
produce a misleading `pending` that could be misinterpreted.

### Destroy — Idempotency

`Destroy` calls `GetService` first and only proceeds if the service
exists with a non-empty UID. The NOT_FOUND case returns nil (already deleted)
which is correct idempotent behavior.

**Edge case:** If the service exists but UID is empty (which shouldn't happen
for a real Cloud Run service), the destroy is silently skipped. This is
unlikely in practice.

### WaitForServiceReady — Timeout Mismatch

`waitForServiceReady` is passed a 1-hour timeout by Deploy, but the enclosing
`deployCtx` has a 15-minute `DeployTimeout`. The parent context will cancel
long before the 1-hour internal deadline is reached. This is actually correct
behavior (parent-budget-wins) but the internal 1 hour is misleading in code.

**Assessment:** Acceptable but worth reducing to something closer to
`DeployTimeout` + headroom (20 minutes) to keep the function self-documenting.

---

## 4. Operation Cleanup Assessment

### Operations Never Expire

Operations in `succeeded`, `failed`, `cancelled`, `succeeded_stale` status
are NEVER cleaned up. They accumulate forever.

- A busy deployment target with 1 deploy per day generates ~365 operation
  rows per year per target.
- At Phase 2.1 there is ~1 target per rollout, but multi-target rollouts
  would multiply this.
- The `operations` table has no TTL column, no archival, no retention policy.

**Operational impact:** Query performance on `SELECT ... FROM operations WHERE status = 'in_progress'`
will degrade with table growth, though SQLite handles tens of thousands of
rows without issue.

**Recommended fix:** Add a `reconciler_delete_ended_operations` goroutine
that runs daily and deletes terminal operations older than N days.
Low priority.

### Abandoned Operation Recovery

At startup, ALL `in_progress` ops are marked failed. This is aggressive but
correct for single-pod (the only process that could own the op died).

The sweep writes `abandoned: process restart while in flight` in both the
operation row and a deployment_history row.

### dispatchDestroy Dispatch Safety

dispatchDestroy is gated by `shouldDispatchDestroy` which only matches
`status = 'deleting'`. A target that somehow reached `deleted` before the
dispatch fires would not be dispatched.

The CAS in `claimTarget` re-validates status at claim time, so concurrent
state changes are safe.

---

## 5. Deployment History Assessment

### History Writes Are Correct

`writeHistory` is called at every dispatch claim and at every outcome:
- Claim: action=`created`/`updated` (first deploy vs. re-deploy), status=`in_progress`
- Success: status=`success`, no rollbacks
- Failure: status=`failed`
- Stale spec: status=`failed`, message="stale spec"
- Abandoned by sweep: action=`error`, status=`failed`, message=`abandoned:...`

History is append-only (`SaveRecord` on a new record). No UPDATE or DELETE
paths modify history.

### Missing History Events

**No history on rollout create:** `HandleRolloutCreate` does not write a
history row when it creates the deployment_targets row. The first history
row appears only when the reconciler dispatches Deploy.

**No history on spec-update redispatch:** When a manifest changes and
`flipCloudTargetsToPendingOnRespec` resets targets to `pending`, no history
row is written describing the intent. The subsequent dispatch creates its
own history row, but the "why pending again" context is invisible.

**No history on cloud destroy initiation:** `markCloudTargetForDestroy` sets
`status=deleting` on the target but writes no history. The dispatcher's
subsequent `writeHistory(..., "deleted", "in_progress", ...)` appears
confusingly late.

**Assessment:** These gaps are acceptable for Phase 2.1. The history is
useful enough to trace what happened; it doesn't need to capture every
intentional state transition. Deferred improvement.

---

## 6. Encryption Safety Assessment

### `EnsureConfigured()` Called at Boot

cloud.go does NOT call `secrets.EnsureConfigured()` at boot. If the key is
missing or malformed, the first credential read silently uses the empty
credential (decrypt fails → `CRED_DECRYPT_FAIL` logged → target set to error).
The error path is correct (refuse to operate), but the silent fallback
during Encrypt would have already been blocked by the controller.

### Secrets Package Test Coverage

The test suite (secrets_test.go) covers:
- Round-trip encrypt/decrypt
- Empty plaintext passthrough
- Legacy plaintext fallback (no prefix → returned as-is)
- Missing key (ErrKeyMissing)
- Malformed key (ErrKeyMissing)
- Wrong key rejects ciphertext (ErrCiphertextCorrupt)
- Concurrent encryption (no race conditions)

**Assessment:** Good coverage. The critical adversarial cases (tamper,
wrong key, missing key) are all tested.

### Credential Scoping

`secrets.Decrypt` is called in cloud.go's `reconcileOne` for each target
per cycle. The result (`credentialsJSON` string) is passed to the provider
and discarded after use. Credentials are not cached across cycles.

**Potential improvement:** The decrypt-in-each-cycle is wasteful for steady-
state status polling. Could cache credentials per cloud_account_id for the
cycle duration. Low priority.

---

## 7. Observability Improvements Summary

### Phase 2.1 Observability Is Good

The following structured logs provide strong traceability:
- `[RECONCILE]` / `[RECONCILE_TARGET]` / `[RECONCILE_TARGET_COMPLETE]` — cycle
  boundaries and per-target outcomes
- `[DISPATCH_CLAIM]` with `rollout_revision` — traces which revision claims a target
- `[DEPLOY_START]` / `[DEPLOY_END]` with duration_ms — quantifies deploy duration
- `[DISPATCH_IN_FLIGHT]` — confirms mid-flight skip guards work
- `[STATE_TRANSITION]` — explicit transition events
- `[RELEASE_LOST_OWNERSHIP]` — sweep-conflict visibility
- `[CLOUD_DELETE_REFUSED]` / `[CLOUD_DELETE_ALLOWED]` — delete safety decisions
- `[CLOUD_TARGET_CREATED]` — target creation trace
- `[CLOUD_RESPEC_REDISPATCH]` — manifest change → redispatch
- `[OP_ABANDONED]` — sweep action with op/target correlation
- `cycle_id` on every log — narrow grep to single cycle

### Observability Gaps

**No operation lifetime visibility.** An operator cannot see how long a
specific operation has been in flight without querying the DB directly.

**No per-target deploy history summary.** The history table is the only
trace of a target's lifecycle, but it requires DB access to query.

**No structured event for suspicion hold.** `[SUSPICION_HOLD]` is logged
at WARN level but has no structured fields for dashboards.

**Recommendation:** Add a reconciler metric (Grafana-friendly) for:
- `autostack_target_status{status="running|error|pending|..."}` gauge
- `autostack_dispatch_duration_seconds` histogram
- `autostack_operation_in_flight` gauge
- `autostack_circuit_open{target_id}` gauge

None of these require new infrastructure — just structured logger output
parsed by Prom tailer or similar.

---

## 8. Remaining Known Operational Risks

| Risk | Severity | Today | Fix |
|---|---|---|---|
| `endDate` set while deploy in-flight → destroy intent silently dropped | HIGH | `markCloudTargetForDestroy` defers, re-arm not implemented | `pending_destroy` column + dispatcher re-arm |
| Startup sweep unsafe under multi-pod | LOW | Startup sweep runs single-pod only | Pod-identity stamping on operations |
| Operations table grows forever | LOW | No retention | Daily TTL cleanup |
| Secrets in env vars are plaintext in GCP logs | MEDIUM | No GCP Secret Manager integration | Source-aware secrets in manifest |
| History not written on rollout/create, spec-update, destroy-marker | LOW | Gaps acceptable for Phase 2.1 | Add history writes at key transitions |
| `target_config` silently dropped when PocketBase returns unexpected type | MEDIUM | Just fixed — but no test | Add unit test for type-switch |
| `waitForServiceReady` 1-hour deadline misleading | LOW | DeployTimeout cancels first | Reduce to 20 min |

---

## 9. Deferred Future Hardening (Phase 2.5+)

From `deferred-operational-hardening.md`, the following remain deferred:

1. **`deployment_targets.pending_destroy` column** — A defer-and-rearm flag so
   a destroy intent set mid-flight is picked up by the release path and
   automatically re-flips to `deleting`.

2. **Pre-destroy confirmation poll** — After `Provider.Destroy` returns,
   poll GetService until NOT_FOUND is confirmed. Tightens the truthfulness
   window from "API returned 200" to "service is actually gone".

3. **Orphan-cleanup scanner** — Periodic provider-side scan of
   `autostack-managed=true` tagged resources that lack a backing
   `deployment_targets` row. Destroys orphans.

4. **Pod-identity on operations** — `owned_by_pod` column on operations
   enables safe multi-pod startup sweep.

5. **Runtime sweep** — Per-cycle sweep of operations that are in_progress
   with `updated_at` older than e.g. 5 minutes. Uses heartbeat to distinguish
   live ops from abandoned ones.

6. **Secrets Source Awareness** — `spec.secrets[].source` field to distinguish
   `plaintext` vs `gcp-secret-manager` vs `aws-secrets-manager`. Cloud Run
   provider maps to `env.projectNumRefs`.

---

## Findings Fixed This Session

| # | File | Issue | Fix |
|---|---|---|---|
| 1 | `cloudrun/provider.go` | `CreateServiceRequest` set both `Service.Name` AND `ServiceId` — GCP internal conflict, 409 error | Drop ServiceId, let GCP derive it from service.Name path |
| 2 | `cloud.go` | `target_config` type-asserted to string but PocketBase JSON columns return `map[string]interface{}` or `[]byte` | Type switch handling all three cases + error logging |
| 3 | `dispatch.go` + `cloud.go` | `targetConfig` built in cloud.go never passed to `DeploySpec.TargetConfig` | Thread targetConfig through dispatchDeploy signature into spec.TargetConfig |

---

## Related
- [[orphan-defense-policy]] — cloud hard-delete refusal
- [[sweep-and-heartbeat-semantics]] — Phase 2.1 sweep/heartbeat design
- [[dispatcher-reconciler-interaction]] — Phase 2.1 ownership fix
- [[lifecycle-assumptions]] — state model
- [[deploy-dispatch-design]] — dispatch design
- [[dangerous-edge-cases]] — known hazards
- [[deferred-operational-hardening]] — Phase 2.5+ backlog