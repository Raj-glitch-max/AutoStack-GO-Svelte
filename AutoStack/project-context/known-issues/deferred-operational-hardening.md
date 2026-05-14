# Deferred Operational Hardening

## Last Updated
2026-05-14 (Phase 2.0 — deploy execution wired; updated tier status)

Work known to be required for production behavioral integrity, but
intentionally deferred. Each item names the consequence of leaving it.

## Tier 1 — closed in Phase 2.0 ✅

### 1. Cloud Deploy dispatch — DONE
Reconciler dispatcher claims `pending` deployment_targets via atomic
CAS, opens an `operations` row, calls Provider.Deploy. See
[[deploy-dispatch-design]].

### 2. Operations collection — DONE
Migration `1715300004_created_operations.js`. Statuses include
`succeeded_stale` for the stale-spec case.

### 3. `deployment_history` writer — DONE
Dispatcher writes two rows per action (dispatch + outcome). Immutability
rules from the migration unchanged.

### 4. Foundational credential encryption — DONE
AES-256-GCM with env-var key, versioned ciphertext, legacy-plaintext
lazy migration. See [[encryption-design]]. KMS-backed key management
remains Tier 5.

## New Tier 1 (post-2.0) — needed before next correctness milestone

### A. Cloud rollout spec-update propagation
Phase 2.0 logs `[CLOUD_UPDATE_DEFERRED]` and refuses to re-deploy on
spec change. Required: detect a meaningful spec diff (image / env /
scale) on rollout update, flip `deployment_targets.status` → `pending`,
let dispatcher redeploy.

### B. Operation lease / heartbeat
Long-running deploys past the 20-min sweep threshold race with the
sweep. Required: refresh `operations.updated_at` during a long deploy;
sweep ignores ops with recent heartbeat.

### C. Release-only-if-still-owner guard
A dispatcher whose op was swept must refuse to overwrite the sweep's
terminal status when it eventually returns. Required: CAS in
`releaseTarget` that only updates if `current_operation` still equals
the dispatcher's op ID.

### D. Orphan-cleanup scanner
Hard-deleting a cloud rollout currently orphans the provider resource.
Required: periodic background scan that lists provider resources tagged
`autostack-managed=true` and destroys those without a corresponding
deployment_targets row.

## Tier 2 — required before HA

### 5. Leader election or row versioning
**Gap:** `Reconciler.started` is process-local. Two pods race writes.
**Required:** Either a single-leader pattern (advisory lock in PG /
distributed lock for SQLite-less deployments) OR optimistic concurrency
on `deployment_targets.updated`.

### 6. Stuck-state detector
**Gap:** state-model.md describes thresholds; no implementation.
**Required:** `last_state_change_at` column + periodic scan that, after
threshold, queries the provider directly and updates status or marks
`error`.

### 7. Orphan cleanup
**Gap:** If `Destroy` fails and the target row is deleted, the cloud
service is orphaned (ongoing cost).
**Required:** Periodic provider-side scan for resources tagged
`autostack-managed=true` whose corresponding target row no longer exists.

## Tier 3 — observability

### 8. Correlation IDs
**Gap:** Cannot grep all log lines for one target in one cycle.
**Required:** `cycle_id` assigned in `reconcileAll`, threaded into every
per-target log emission. Per-attempt ID where retries exist.

### 9. Structured logger
**Gap:** `log.Printf` produces unparseable strings. CLAUDE.md already
forbids `fmt.Println`; `log.Printf` is the same class of problem.
**Required:** Adopt a structured logger (slog is in stdlib since 1.21);
emit `level`, `subsystem`, `cycle_id`, `target_id` as fields.

### 10. Per-target observability columns
**Gap:** Only `last_synced` is recorded; it updates on success and on
refused observations alike.
**Required:** Add `last_success_at`, `last_failure_at`,
`last_failure_category`, `consecutive_failures` — derivable from
existing in-memory state.

## Tier 4 — provider correctness

### 11. Real Cloud Run rollback via `Service.Traffic`
See [[rollback-semantics]]. Requires lineage columns first.

### 12. Live cost APIs
ADR-010 promise. Replace static rates with GCP Cloud Billing API calls.

### 13. Real metrics / quota
Replace `ErrNotImplemented` (Phase 1.9) with Cloud Monitoring / quota
service integrations. UI must continue to surface "unavailable" until
both land.

### 14. Region-scoped credential validation
`ValidateCredentials` should attempt a region-targeted no-op
(GetService against a non-existent name) to surface region-level
permission gaps at validation time.

## Tier 5 — schema / contract hygiene

### 15. Single canonical `provider` enum
Three enums describe the same concept today
(`cloud_accounts.provider`, `deployment_targets.provider`,
`rollouts.target_type`). Reconciler should cross-validate and refuse
mismatched combinations.

### 16. Rollout `status` / `last_deployed` columns (only if needed)
Adding them lets the reconciler publish a rollout-level status. Don't
add until the cloud-Deploy dispatch path lands and the value would
reflect reality.

## What is intentionally NOT here

- ECS / Azure ACA provider implementation. Out of scope for 1.9.
- AI features. Out of scope.
- SSO/SAML. Phase 4.
- VPC management. Phase 3.

## Related
- [[lifecycle-assumptions]]
- [[reconciliation-guarantees]]
- [[restart-behavior]]
- [[provider-limitations]]
- [[dangerous-edge-cases]]
- [[correctness-limitations]]
