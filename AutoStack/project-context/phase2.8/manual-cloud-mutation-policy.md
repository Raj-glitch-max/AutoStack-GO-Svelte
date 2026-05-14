# Manual Cloud Mutation Policy — Phase 2.8

## Last Updated
2026-05-14

## Statement

AutoStack does NOT detect manual cloud-side mutations to managed
resources today. This is a known limitation. Operators must understand
that:

- Direct edits via `gcloud run services update`, the GCP console, or
  Cloud Run API direct calls are invisible to AutoStack.
- AutoStack's `deployment_targets` row reflects the **last AutoStack-
  driven deploy**, not the **current provider state**.
- The next AutoStack deploy will OVERWRITE the manual changes.

## What detection requires

Phase 3 work:

1. **Spec snapshot at deploy:** Capture the canonical spec sent to the
   provider at each successful Deploy. Store in
   `deployment_targets.deployed_spec` (JSON) or in `operations.deployed_spec`.
2. **Drift comparison cycle:** Periodically (e.g., every 5 min) read
   `GetService` for every running target and structurally diff against
   `deployed_spec`. Surface mismatches via
   `deployment_targets.drift_detected = true` +
   `deployment_targets.drift_summary`.
3. **Diff classification:** Distinguish material (image, env, scale)
   from cosmetic (label additions). Different policies per class.
4. **UI surface:** Frontend should expose drift state per target with
   a "remediate" affordance (re-deploy from manifest).

## Why deferred to Phase 3

- Substantial schema work (snapshot column or new table).
- Provider extension to dump canonical config.
- Diff library / semantic comparison.
- UX design for drift reports.
- Per-tenant tolerance policies.

## Phase 2.8 acknowledgments

- `deployment_targets.drift_detected` remains permanently `false`.
- `deployment_targets.drift_summary` is used only for transition
  reasons and error messages today.
- Operators are explicitly warned in CLAUDE.md and runbook (TBD) that
  manual mutation is invisible.

## Compensating controls today

- IAM policy: restrict who can edit Cloud Run services to AutoStack's
  service account + emergency-break-glass users.
- Cloud Run audit logs: external systems can monitor for non-AutoStack
  `UpdateService` calls.
- Periodic operator review of high-value rollouts.

## Related
- [[drift-handling-maturity]]
- [[../phase2.4/drift-persistence-assessment]] D-1
