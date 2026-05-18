/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)

  // Phase 3.9.7 — Provider Truth Reconciliation.
  //
  // Two new collections:
  //
  //   provider_observations — every provider state observation is a
  //     first-class forensic record. Operators can reconstruct what the
  //     provider claimed, when, whether it was trusted, and whether it
  //     contradicted runtime belief.
  //
  //   provider_drifts — tracks when provider truth diverges from runtime
  //     expectation across multiple observations. Drift records survive
  //     restart; operators can see when drift began, how persistent it was,
  //     and when (if) it resolved.
  //
  // ─────────────────────────────────────────────────────────────────────────
  // DESIGN INVARIANTS
  // ─────────────────────────────────────────────────────────────────────────
  //
  //   1. provider_observations rows are APPEND-ONLY. Never mutated.
  //   2. provider_drifts rows are OPEN (resolved_at NULL) or CLOSED.
  //      A closed drift is never reopened; a new observation creates a new row.
  //   3. Both collections survive restart — all state is in the DB, none
  //      in process memory.
  //   4. Neither collection is auto-repaired. Contradiction + drift records
  //      are forensic evidence. The operator must investigate and resolve.
  //   5. Retention: provider_observations defer to Phase 3.9.8+ because
  //      contradiction rows have indefinite forensic value. Resolved drifts
  //      follow the same 90-day retention as resolved incidents.

  // ── provider_observations ──────────────────────────────────────────────

  const obsCollection = new Collection({
    "id": "provider_observations",
    "name": "provider_observations",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "po_target",
        "name": "target",
        "type": "text",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        "system": false,
        "id": "po_operation_id",
        "name": "operation_id",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 15, "pattern": "" }
      },
      {
        "system": false,
        "id": "po_cycle_id",
        "name": "cycle_id",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 32, "pattern": "" }
      },
      {
        "system": false,
        "id": "po_pod_id",
        "name": "pod_id",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 64, "pattern": "" }
      },
      {
        // observed_at — RFC3339 UTC, set by the runtime clock (NOT the
        // provider clock). Provider timestamps are in evidence_json.
        "system": false,
        "id": "po_observed_at",
        "name": "observed_at",
        "type": "date",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": { "min": "", "max": "" }
      },
      {
        // provider — e.g., "aws_ecs", "gcp_run", "azure_aca".
        "system": false,
        "id": "po_provider",
        "name": "provider",
        "type": "text",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        // observation_source — canonical enum: poll | deploy-result |
        //   rollback-result | reconciliation-read | startup-rehydrate
        "system": false,
        "id": "po_source",
        "name": "observation_source",
        "type": "text",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": { "min": 1, "max": 32, "pattern": "" }
      },
      {
        // provider_state — what the provider claimed at observation time.
        // Normalized to the canonical state vocabulary (running, missing,
        // deleted, updating, error, etc.).
        "system": false,
        "id": "po_provider_state",
        "name": "provider_state",
        "type": "text",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        // runtime_expected_state — what the runtime believed should be
        // true at observation time. Empty if unknown/unset.
        "system": false,
        "id": "po_expected_state",
        "name": "runtime_expected_state",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 64, "pattern": "" }
      },
      {
        // observation_trust — canonical enum: authoritative | inferred |
        //   stale | ambiguous | contradicted.
        // Set by the caller based on how the observation was obtained.
        "system": false,
        "id": "po_trust",
        "name": "observation_trust",
        "type": "text",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": { "min": 1, "max": 16, "pattern": "" }
      },
      {
        // evidence_json — raw provider response excerpt (sanitized — no
        // credentials). Used for forensic replay.
        "system": false,
        "id": "po_evidence",
        "name": "evidence_json",
        "type": "json",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {}
      },
      {
        // contradiction_detected — true when provider_state explicitly
        // contradicts runtime_expected_state (e.g., provider says missing
        // but runtime expects running).
        "system": false,
        "id": "po_contradiction",
        "name": "contradiction_detected",
        "type": "bool",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {}
      },
      {
        // drift_detected — true when this observation contributed to a
        // provider drift record. May be true without contradiction_detected
        // (e.g., prolonged stale observations trigger drift escalation).
        "system": false,
        "id": "po_drift",
        "name": "drift_detected",
        "type": "bool",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {}
      },
      {
        // drift_reason — human-readable description of the divergence.
        "system": false,
        "id": "po_drift_reason",
        "name": "drift_reason",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 500, "pattern": "" }
      }
    ],
    "indexes": [
      "CREATE INDEX idx_po_target ON provider_observations (target)",
      "CREATE INDEX idx_po_target_observed ON provider_observations (target, observed_at)",
      "CREATE INDEX idx_po_contradiction ON provider_observations (target, contradiction_detected)",
      "CREATE INDEX idx_po_provider ON provider_observations (provider)",
      "CREATE INDEX idx_po_trust ON provider_observations (observation_trust)"
    ],
    "listRule": null,
    "viewRule": null,
    "createRule": null,
    "updateRule": null,
    "deleteRule": null,
    "options": {}
  })

  // ── provider_drifts ────────────────────────────────────────────────────

  const driftCollection = new Collection({
    "id": "provider_drifts",
    "name": "provider_drifts",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "pd_target",
        "name": "target",
        "type": "text",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        // provider — which provider's observation caused this drift.
        "system": false,
        "id": "pd_provider",
        "name": "provider",
        "type": "text",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        // first_seen_at — when the first contradicting observation was recorded.
        "system": false,
        "id": "pd_first_seen",
        "name": "first_seen_at",
        "type": "date",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": { "min": "", "max": "" }
      },
      {
        // last_seen_at — most recent update (new observation still contradicting).
        "system": false,
        "id": "pd_last_seen",
        "name": "last_seen_at",
        "type": "date",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": { "min": "", "max": "" }
      },
      {
        // drift_state — the provider state that caused the contradiction.
        "system": false,
        "id": "pd_drift_state",
        "name": "drift_state",
        "type": "text",
        "required": false,
        "presentable": true,
        "unique": false,
        "options": { "min": null, "max": 64, "pattern": "" }
      },
      {
        // drift_count — number of observations that fed this drift record.
        "system": false,
        "id": "pd_drift_count",
        "name": "drift_count",
        "type": "number",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": { "min": 1, "max": null, "noDecimal": true }
      },
      {
        // resolved_at — when the drift was closed. NULL = still active.
        "system": false,
        "id": "pd_resolved_at",
        "name": "resolved_at",
        "type": "date",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": "", "max": "" }
      },
      {
        // resolution_reason — how drift ended (e.g., "provider state
        // converged with runtime expectation", "operator investigated").
        "system": false,
        "id": "pd_resolution_reason",
        "name": "resolution_reason",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 500, "pattern": "" }
      }
    ],
    "indexes": [
      "CREATE INDEX idx_pd_target ON provider_drifts (target)",
      "CREATE INDEX idx_pd_target_provider ON provider_drifts (target, provider)",
      "CREATE INDEX idx_pd_active ON provider_drifts (target, resolved_at)",
      "CREATE INDEX idx_pd_first_seen ON provider_drifts (first_seen_at)"
    ],
    "listRule": null,
    "viewRule": null,
    "createRule": null,
    "updateRule": null,
    "deleteRule": null,
    "options": {}
  })

  dao.saveCollection(obsCollection)
  return dao.saveCollection(driftCollection)

}, (db) => {
  const dao = new Dao(db)
  const obs = dao.findCollectionByNameOrId("provider_observations")
  if (obs) { dao.deleteCollection(obs) }
  const drifts = dao.findCollectionByNameOrId("provider_drifts")
  if (drifts) { dao.deleteCollection(drifts) }
})
