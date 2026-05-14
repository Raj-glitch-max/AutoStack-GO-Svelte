# Phase 2 Finalization — Drift & Divergence Survivability Assessment

**Last Updated:** 2026-05-14

## Scope

This document covers whether the system survives long-running divergence
between AutoStack's desired state and actual provider state, and whether
it surfaces divergence honestly.

---

## Drift Categories and Survivability

### D-1: Provider-side manual config change

**Scenario:** Operator edits a Cloud Run service via `gcloud run services update`.

| Property | Outcome |
|---|---|
| Detection | None. `drift_detected` permanently false. No structural diff. |
| AutoStack believes | The service matches the rollout spec (last deployed manifest) |
| What gets overwritten | Next AutoStack deploy — full spec rewrite |
| Visibility | `drift_summary` is only for error messages; no drift surface |
| Survivability | **Overwrite-on-next-deploy** — functional, but manual changes are invisible and get clobbered without warning |

**Verdict:** Accepted limitation, documented in [[phase2.8/manual-cloud-mutation-policy.md]]. Phase 3 material (spec snapshot + diff).

---

### D-2: External rename of service

**Scenario:** Operator renames Cloud Run service via GCP console.

- `GetService` returns NOT_FOUND.
- `GetStatus` error → `recordTargetFailure` + circuit breaker.
- After 5 failures → circuit open → no more polling.
- Target stays at whatever status it was before the rename.

**Visibility:** `deployment_targets.status` is stale. `drift_summary` set on the error. Log shows `[FAILURE] category=permanent` (NOT_FOUND is classified permanent).

**Survivability:** Correct — the target is now orphaned and visible as a failure. But **there is no automatic cleanup path**: the target stays `error` and never gets deleted. The operator must manually clear the target.

**Gap:** There's no "service gone but target exists — should we delete the target?" logic.

---

### D-3: Cloud Run revision GC of old revisions

**Scenario:** Cloud Run GCs a revision that AutoStack previously recorded in `current_revision`.

- `current_revision` is informational only. No code path reads it back.
- Revision GC does not trigger API errors.
- Rollback is not implemented, so no code path depends on the revision field.

**Visibility:** `deployment_targets.current_revision` refers to a revision that no longer exists in GCP.

**Survivability:** ✅ No impact. The field is inert today.

---

### D-4: Post-destroy lag (Cloud Run 200 → still listable)

**Case:** `DeleteService` returns 200; `GetService` returns the service for up to 60s.

| Outcome | Handled? |
|---|---|
| NOT_FOUND within 60s | ✅ Phase 2.8 confirmDeleted polls and returns nil |
| Timeout (60s) | ✅ Returns nil + `[DESTROY_CONFIRM_TIMEOUT]` log |
| NOT_FOUND on first call (already deleted externally) | ✅ Idempotent nil |

**Verdict:** ✅ Closed by Phase 2.8 confirm loop.

---

### D-5: Cloud Run failed revision while old serves traffic

**Scenario:** New revision fails to start; Cloud Run keeps serving old revision.

- `GetService` for the service shows Ready=SUCCEEDED (the old revision is fine).
- The failing revision is invisible to AutoStack (we call `GetService`, not `ListRevisions`).
- AutoStack reports `running`.
- Operator sees healthy service but a failed revision in GCP console.

**Visibility:** No visibility into per-revision health. The `drift_summary` on `running` targets may say "healthy" but doesn't distinguish serving vs. transitioning revision.

**Gap:** Deferred to Phase 3 (`serving_revision` field — the service's traffic target, not just Ready=SUCCEEDED condition).

---

### D-6: Region change on cloud_account

**Scenario:** Admin changes `cloud_account.region` from `us-central1` to `us-east1`.

- `reconcileOne` re-reads `cloud_accounts.region` via the SQL join every cycle.
- All subsequent `GetService`, `Deploy`, `Destroy` calls use the new region.
- Old-region `GetService` → NOT_FOUND → permanent error → circuit opens → `error`.
- Destroy tries `DeleteService` in new region → NOT_FOUND → success (idempotent).
- Old service persists in old region, no longer managed.

**Verdict:** Documented in [[phase2.8/deferred-followups]], Phase 3 scope. The old managed resource becomes orphaned.

**Mitigation:** Don't change `cloud_account.region` on active accounts. Create a new account.

---

### D-7: Provider-side delete observed (NOT_FOUND on GetStatus)

**Scenario:** Another AutoStack instance or admin deletes the Cloud Run service.

```go
GetService → NOT_FOUND → err != nil
  → recordTargetFailureWithCategory(targetID, FailurePermanent)
  → circuit breaker increments
  → if prevStatus=updating and noteSuspectError: hold (first observation)
  → else: updateTargetStatus(previous, error)
```

**Visibility:** After 5 cycles → circuit open. Target stuck `error`. History row shows `status=failed` with message = "not found" or similar.

**Survivability:** ✅ Correctly detected and surfaced as a persistent failure. Operators see the NOT_FOUND.

---

## Drift Truthfulness Matrix

| Drift type | Detected? | Surface honest? | Degrades gracefully? |
|---|---|---|---|
| Manual cloud mutation | No | No — `drift_detected` false | ⚠️ Next deploy clobbers silently |
| Provider delete | ✅ | ✅ | ✅ Circuit open + error |
| Service rename | ✅ | ✅ | ⚠️ Orphan persists; no auto-cleanup |
| Post-destroy lag | ✅ | ⚠️ `[DESTROY_CONFIRM_TIMEOUT]` on timeout | ✅ Target still marked `deleted` |
| Revision GC | ✅ (inert, no impact) | N/A | ✅ No impact |
| Region change | ✅ | ✅ | ⚠️ Orphan in old region |
| Failed revision (old serves) | No | ⚠️ Reports `running` | ⚠️ Invisible |
| Cloud Run outage (all services degraded) | Partial | ✅ | ✅ Stale healthy; `last_synced` visible |

---

## Phase 2.8 Closures

The Phase 2.8 post-destroy confirmation poll closes D-4. All other drift
types remain as documented limitations (Phase 3 material for the structural
ones, accepted limitations for the rest).

---

## Verdict

**Manual drift (D-1) is invisible and will be overwritten.** Acknowledged and documented.

**Provider-side delete (D-7) is correctly detected and surfaced.** Circuit breaker prevents retry storms.

**Post-destroy lag (D-4) is fixed by Phase 2.8.**

**Failed revision visibility (D-5) requires Phase 3 `serving_revision` work.** Not safety-critical — cloud run reports the service itself as healthy.

**D-2 and D-6 (orphaned resources) remain Phase 3 scope.** No data corruption — just unmanaged resource persistence.

---

## Related
- [[phase2.8/drift-handling-maturity]]
- [[phase2.8/manual-cloud-mutation-policy]]
- [[phase2.8/post-destroy-confirm-poll]]
- [[phase2.8/deferred-followups]]