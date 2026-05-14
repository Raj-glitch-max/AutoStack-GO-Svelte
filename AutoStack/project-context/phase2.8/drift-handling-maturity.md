# Drift Handling Maturity — Phase 2.8

## Last Updated
2026-05-14

## Question

Does the system handle long-term provider divergence truthfully?

## Maturity matrix

| Drift type | Today's behavior | Verdict |
|---|---|---|
| Provider-side delete observed | NOT_FOUND from GetStatus → target → error | ✓ truthful (Phase 2.3) |
| Provider-side manual config change | Undetected; target reports last-known status | ⚠️ silent — `drift_detected` always false |
| Cloud Run revision GC of old revisions | Inert until rollback attempts use them | ✓ no impact today (rollback not implemented) |
| External rename of service | NOT_FOUND on poll | ✓ same as deletion |
| Region change on cloud_account | NOT_FOUND on poll | ✓ converges to error |
| Cloud Run failed revision while old serves traffic | AutoStack reports error; old revision still active | ⚠️ partial (no `serving_revision` field) |
| Post-destroy lag (Cloud Run 200 OK then still listable) | Reported `deleted` immediately; provider still lists for 10-60s | ⚠️ → Phase 2.8 NOT_FOUND poll fix |

## What Phase 2.8 closes

- Post-destroy lag — confirmation poll lands.

## What Phase 2.8 does NOT close (documented)

- Manual cloud mutation drift detection. Spec-vs-actual comparison
  requires:
  - Capturing the deployed spec at success time (currently lost — only
    rollout manifest YAML survives, which the operator may have since
    edited).
  - Per-cycle GetService and structured diff vs the captured spec.
  - Defining what counts as drift (exact match, semantic match,
    operator-tolerated drift).
  - UI surface for drift reports.

  This is a substantial feature — at least 3 weeks of work — deferred to
  Phase 3.

- `serving_revision` truthfulness: provider extension. Phase 3.

## Truthful-state guarantees after Phase 2.8

| Property | Status |
|---|---|
| `deleted` claimed → service is actually gone | ✓ after Phase 2.8 (NOT_FOUND poll) |
| `running` claimed → service is healthy AND serving | ⚠️ healthy = Ready=SUCCEEDED; serving = unverified |
| `error` claimed → operator action required | ✓ |
| `unknown` not persisted | ✓ |
| No regression via single-observation | ✓ |
| Manual mutation invisible | ⚠️ accepted limitation |

## Related
- [[post-destroy-confirm-poll]]
- [[manual-cloud-mutation-policy]]
