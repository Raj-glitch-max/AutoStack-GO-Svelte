# Drift Persistence Assessment — Phase 2.4

## Last Updated
2026-05-14

## Premise

Drift = the provider state diverges from AutoStack's record of desired
state. Phase 2.3 closed the *short-term* drift sources (stale-spec
detection, transition guard, suspicion counter). Phase 2.4 asks the
**long-lived** question: can drift accumulate silently, persist
forever, or be normalized into "healthy"?

## Drift sources

### D-1: Manual cloud-side mutation

**Setup:** Operator (or another tool) modifies a Cloud Run service
directly via gcloud / console: changes image tag, env vars, scaling
config.

**Behavior:**
- AutoStack's `deployment_targets` row still reflects the original
  spec.
- `GetStatus` returns `running` if the service is healthy regardless
  of spec match.
- AutoStack reports "running" → operator-visible truth says
  "everything's fine".
- **Drift persists indefinitely.**

**Severity:** Medium. The operator's intent (the rollout manifest in
PocketBase) no longer matches reality. Future redeploys would clobber
the manual changes.

**Mitigation today:** `deployment_targets.drift_detected` column
exists but is permanently `false`. No spec-vs-actual comparison.

**Phase 2.4 fix considered:** Add a drift-detection pass to the
reconciler that compares the spec sent at last Deploy vs. the
provider-observed config.

**Decision:** Defer to Phase 2.8. The full design requires:
- Capturing the spec snapshot at deploy time (currently lost — only
  manifest YAML is preserved on the rollout).
- Pulling provider config every cycle (extra API cost).
- Defining "what counts as drift" — exact match, semantic match.

**Phase 2.4 immediate improvement:** Log a `[DRIFT_UNCHECKED]` debug
emission per cycle to make the gap operationally visible. Defer the
column's actual population to Phase 2.8.

**Decision update:** Skip even the log emission — it would be noisy and
misleading. The deferred-hardening doc already records this gap.

### D-2: Stale revision divergence

**Setup:** Cloud Run accumulates revisions on every UpdateService.
After 100 deploys, the service has 100 revisions, most idle. Cloud
Run's auto-cleanup eventually GC's them (default: retains N).

**Behavior:**
- AutoStack's `deployment_targets.current_revision` records the most
  recent deploy's revision name.
- Old revision names referenced in `deployment_history.to_revision`
  may have been GC'd provider-side.
- A future rollback to one of those revisions would NOT_FOUND.

**Severity:** Low for Phase 2.4 since rollback is not implemented.
Will become medium when Phase 2.5 lands rollback.

**Mitigation today:** None. Rollback refusal sidesteps the issue.

**Phase 2.5 work:** When implementing rollback, the operator-facing
endpoint MUST validate the target revision exists provider-side before
accepting the rollback request. Documented.

### D-3: Provider-side deletion observed

**Setup:** Service deleted manually via gcloud. AutoStack's
`deployment_targets.status` is `running`.

**Behavior:**
- Next status-poll: GetService returns NOT_FOUND.
- ClassifyError → permanent.
- updateTargetStatus(running → error, msg="NOT_FOUND...") →
  transition guard PERMITS `running → error`.
- Target → error with NOT_FOUND drift_summary.
- Operator inspects, sees "service not found", concludes manual delete.

**Severity:** Low. Convergent. ✓

### D-4: Cloud-account region changed via admin

**Setup:** Operator edits a `cloud_accounts.region` field. Existing
targets reference that account and their external_ids were created
under the old region.

**Behavior:**
- Next reconcile: account.Region = new region.
- GetStatus calls "projects/P/locations/NEW_REGION/services/EXT_ID" →
  NOT_FOUND (service is in OLD_REGION).
- Same as D-3: target → error.

**Severity:** Low. Operator error; convergent to error.

**Phase 2.5 work:** Refuse the cloud_account region edit if any live
targets reference it. Schema-level constraint via update hook.
Deferred.

### D-5: Revisions accumulated by AutoStack-driven deploys

**Setup:** 100 successful deploys over months. Cloud Run keeps the
revision history; service.RevisionTemplate is overwritten each time.

**Behavior:** No drift; the active revision matches the most recent
deploy. Old revisions are inert.

**Severity:** None. Cloud Run handles revision lifecycle.

**Concern:** If AutoStack ever lists revisions for rollback purposes
(Phase 2.5), the list could be enormous. Pagination required.

### D-6: External_id mismatch from rename

**Setup:** AutoStack's serviceName is `autostack-<rolloutID>`. If
someone manually renames the service in GCP (which Cloud Run doesn't
support, but bypasses via copy-and-recreate), AutoStack's target ID no
longer maps to a service.

**Behavior:** Same as D-3 — NOT_FOUND on next poll.

### D-7: Long-lived inconsistency from a single failed Update

**Setup:** Deploy succeeds at API level but Cloud Run revision rollout
stalls (e.g., container startup probes failing for hours). Status-poll
sees `Ready=FAILED`.

**Behavior:**
- Suspicion counter: first observation refuses, second confirms → target → error.
- Provider eventually marks the revision failed.
- AutoStack reports error.
- The OLD revision continues serving traffic (Cloud Run keeps the last
  Ready revision active).
- AutoStack's `current_revision` reflects the NEW revision (the one
  that failed).
- AutoStack reports `error`, but the SERVICE IS STILL SERVING traffic
  on the old revision.

**Severity:** Medium. AutoStack reports the deploy as failed; provider
hasn't actually shifted traffic. The operator-visible truth is
"deploy failed and service is serving the previous revision" which is
correct, but the `deployment_targets.endpoint_url` may still be
populated from the failed-revision attempt.

**Mitigation today:** None. Operator must understand the Cloud Run
traffic model.

**Phase 2.5 work:** On Ready=FAILED, query Cloud Run for active
traffic-target revision and surface it as
`deployment_targets.serving_revision` distinct from `current_revision`.
Schema work + provider extension. Deferred.

## Drift visibility today

| Drift type | Detected? | Reported? |
|---|---|---|
| D-1 manual mutation | ✗ | ✗ |
| D-2 stale revision (rollback target) | ✗ | n/a (no rollback) |
| D-3 provider-side delete | ✓ via NOT_FOUND on poll | ✓ via error status |
| D-4 region change | ✓ via NOT_FOUND | ✓ via error status |
| D-5 revision accumulation | n/a | n/a |
| D-6 external rename | ✓ via NOT_FOUND | ✓ via error status |
| D-7 failed-revision serving-revision mismatch | ✓ via error status | ⚠️ partial (no serving_revision field) |

## Phase 2.4 implementation in this area

None directly. D-1 and D-7 are the largest gaps; both require schema
work and provider extensions, deferred to Phase 2.5/2.8.

## Deferred to Phase 2.5+

- D-1: spec-vs-actual drift detection scan.
- D-2: rollback revision-exists pre-check (when rollback lands).
- D-4: cloud_account region change refusal.
- D-7: `serving_revision` schema + provider read.

## Related
- [[../phase2.3/eventual-consistency-hazards]]
- [[../phase2.3/truthful-state-assessment]]
- [[../known-issues/dangerous-edge-cases]]
