/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)
  const targetCollection = dao.findCollectionByNameOrId("deployment_targets")

  // Phase 3.4 — Truthful Drift & Reconciliation Foundations.
  //
  // This migration adds:
  //   1. Drift state columns on deployment_targets (drift detection foundation)
  //   2. last_deployed_revision on deployment_targets (reconciliation baseline)
  //   3. Ambiguity escalation counter on deployment_targets
  //
  // These columns are ADDITIVE. Phase 3.0–3.3 code that ignores them continues
  // to function. Phase 3.4+ code reads them to drive truthful drift detection
  // and controlled ambiguity escalation.

  // ── Drift columns ─────────────────────────────────────────────────────────

  // drift_state: classification of the relationship between desired spec and
  // deployed state. Values defined in pkg/reconciler/drift.go:
  //   unknown             — no deployed_spec available yet (no prior deploy)
  //   no_drift            — desired revision matches last deployed revision
  //   suspected_drift     — single-tick mismatch; may be eventual consistency lag
  //   confirmed_drift     — spec changed and not yet re-deployed
  //   stale_provider_state — provider observation conflicts with known deployed state
  //
  // The reconciler writes this on every GetStatus tick. The UI MUST surface
  // non-"no_drift" values as a warning, not an error. Drift is NOT a failure;
  // it is a truthful statement about the gap between desired and deployed state.
  targetCollection.schema.addField(new SchemaField({
    "system": false,
    "id": "dt_drift_state",
    "name": "drift_state",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "min": null,
      "max": 32,
      "pattern": ""
    }
  }))

  // drift_detected_at: when the current drift state was first observed.
  // Cleared when drift_state returns to no_drift or unknown.
  // Useful for "how long has this target been drifted?" operator queries.
  targetCollection.schema.addField(new SchemaField({
    "system": false,
    "id": "dt_drift_detected_at",
    "name": "drift_detected_at",
    "type": "date",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "min": "",
      "max": ""
    }
  }))

  // drift_detail: operator-readable description of the drift evidence.
  // Example: "desired revision 2026-05-14T10:00:00Z differs from deployed
  // revision 2026-05-13T18:00:00Z". Cleared when drift resolves.
  targetCollection.schema.addField(new SchemaField({
    "system": false,
    "id": "dt_drift_detail",
    "name": "drift_detail",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "min": null,
      "max": 1000,
      "pattern": ""
    }
  }))

  // last_deployed_revision: the rollout.updated_at timestamp string at the
  // time the last SUCCESSFUL deploy operation completed. This is the
  // reconciliation baseline for drift detection.
  //
  // When the reconciler computes GetStatus:
  //   if rollout.updated_at == last_deployed_revision → no spec drift
  //   if rollout.updated_at != last_deployed_revision → spec was updated since last deploy
  //
  // Written by dispatchDeploy on the success path. Null for targets that
  // predate Phase 3.4 or have never had a successful deploy.
  //
  // This is NOT the same as rollout_revision on the operations row (which
  // is the revision AT DISPATCH TIME). last_deployed_revision is the
  // confirmed-success baseline on the target itself.
  targetCollection.schema.addField(new SchemaField({
    "system": false,
    "id": "dt_last_deployed_revision",
    "name": "last_deployed_revision",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "min": null,
      "max": 64,
      "pattern": ""
    }
  }))

  // ── Ambiguity escalation counter ──────────────────────────────────────────

  // lifecycle_ambiguity_consecutive_timeouts: number of consecutive
  // GetStatus ticks in which the ambiguity deadline has already passed
  // and the provider is STILL reporting ambiguous state.
  //
  // Phase 3.3 wrote a forensic history row on each timeout but did not
  // escalate. Phase 3.4 escalates to `error` after this counter reaches
  // ambiguityEscalationThreshold (3) — see pkg/reconciler/drift.go.
  //
  // The counter is incremented in applyAmbiguityToTarget on each
  // [AMBIGUITY_TIMEOUT] observation. It is cleared when ambiguity
  // resolves (provider observation becomes non-ambiguous) or when
  // escalation fires (target promoted to error, ambiguity cleared).
  targetCollection.schema.addField(new SchemaField({
    "system": false,
    "id": "dt_ambiguity_consecutive_timeouts",
    "name": "lifecycle_ambiguity_consecutive_timeouts",
    "type": "number",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "min": null,
      "max": null,
      "noDecimal": true
    }
  }))

  return dao.saveCollection(targetCollection)
}, (db) => {
  const dao = new Dao(db)
  const targetCollection = dao.findCollectionByNameOrId("deployment_targets")

  targetCollection.schema.removeField("dt_drift_state")
  targetCollection.schema.removeField("dt_drift_detected_at")
  targetCollection.schema.removeField("dt_drift_detail")
  targetCollection.schema.removeField("dt_last_deployed_revision")
  targetCollection.schema.removeField("dt_ambiguity_consecutive_timeouts")

  return dao.saveCollection(targetCollection)
})
