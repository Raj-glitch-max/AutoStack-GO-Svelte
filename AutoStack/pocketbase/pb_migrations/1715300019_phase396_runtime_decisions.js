/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)

  // Phase 3.9.6 — Runtime Decisions persistence.
  //
  // Every meaningful runtime decision (retry scheduled, retry exhausted,
  // lease abort, reconcile blocked, escalation, contradiction detected, etc.)
  // writes a row here so an operator can reconstruct WHY the runtime acted
  // from persisted DB state alone — no log archaeology required.
  //
  // This is the "decision evidence" table that turns the runtime from
  // "correct" into "operationally trustworthy" per the Phase 3.9.6 mandate.
  //
  // ─────────────────────────────────────────────────────────────────────────
  // DESIGN INVARIANTS
  // ─────────────────────────────────────────────────────────────────────────
  //
  //   1. NEVER MUTATED after creation. Decisions are append-only forensic
  //      evidence. Updating one rewrites operational history.
  //   2. NO retention purge in this phase. Decisions are small (no large
  //      blobs); operational forensic value is permanent. If growth becomes
  //      a concern, Phase 3.9.7+ may add retention rules that respect FK
  //      protection (operations referenced by recent decisions stay alive).
  //   3. FK references (operation_id, evidence_refs) are PLAIN TEXT, not
  //      PocketBase relations. This avoids cascade-delete surprises when
  //      retention compacts operations; the cleanup sweep in cleanup.go
  //      adds an explicit subquery to keep operations referenced by recent
  //      decisions.
  //   4. policy_name + policy_version snapshot the decision-making contract
  //      at the time of the decision. A future Phase 4 policy registry can
  //      be backfilled against these strings.

  const collection = new Collection({
    "id": "runtime_decisions",
    "name": "runtime_decisions",
    "type": "base",
    "system": false,
    "schema": [
      {
        // target — the deployment_target this decision pertains to.
        // Text not relation; mirrors incidents.target rationale.
        "system": false,
        "id": "rd_target",
        "name": "target",
        "type": "text",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        // rollout — snapshot of the target's rollout at decision time.
        "system": false,
        "id": "rd_rollout",
        "name": "rollout",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 64, "pattern": "" }
      },
      {
        // operation_id — links to operations.id. NULL for pre-claim decisions
        // (lease-acquire-deny, CAS-claim-lost) where no operation exists yet.
        "system": false,
        "id": "rd_operation_id",
        "name": "operation_id",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 15, "pattern": "" }
      },
      {
        // reconcile_cycle_id — the cycleID that triggered this decision.
        // The operations.cycle_id column has existed since 1715300008 but
        // was never written; Phase 3.9.6 finally wires it on both tables.
        "system": false,
        "id": "rd_cycle_id",
        "name": "reconcile_cycle_id",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 32, "pattern": "" }
      },
      {
        // pod_id — currentPodIdentity() at decision time. Enables
        // multi-pod forensic separation when SC-5 work expands.
        "system": false,
        "id": "rd_pod_id",
        "name": "pod_id",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 64, "pattern": "" }
      },
      {
        // decision_type — canonical enum string (pkg/reconciler/decisions.go).
        // Stable strings; renaming requires a migration.
        "system": false,
        "id": "rd_decision_type",
        "name": "decision_type",
        "type": "text",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        // decision_reason — stable short code (e.g., "budget_exhausted",
        // "forward_progress_hold"). NOT free-form prose; this is the
        // subtype within decision_type for filterable queries.
        "system": false,
        "id": "rd_decision_reason",
        "name": "decision_reason",
        "type": "text",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": { "min": 1, "max": 500, "pattern": "" }
      },
      {
        // decision_inputs — JSON snapshot of values that fed the decision.
        // Examples: retry_class, attempt_count, lease_valid, previous_status.
        "system": false,
        "id": "rd_inputs",
        "name": "decision_inputs",
        "type": "json",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {}
      },
      {
        // evidence_refs — JSON array of {collection, id} refs to related
        // rows in operations/incidents/deployment_history/runtime_decisions.
        "system": false,
        "id": "rd_evidence_refs",
        "name": "evidence_refs",
        "type": "json",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {}
      },
      {
        // policy_name — e.g. "retry_engine", "lease_engine", "invariant_audit".
        "system": false,
        "id": "rd_policy_name",
        "name": "policy_name",
        "type": "text",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        // policy_version — semver-style. Bumps when policy semantics change.
        "system": false,
        "id": "rd_policy_version",
        "name": "policy_version",
        "type": "text",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": { "min": 1, "max": 16, "pattern": "" }
      },
      {
        // runtime_confidence — "high" | "medium" | "low".
        "system": false,
        "id": "rd_confidence",
        "name": "runtime_confidence",
        "type": "text",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": { "min": 1, "max": 8, "pattern": "" }
      },
      {
        // confidence_source — why this confidence level (enum).
        "system": false,
        "id": "rd_confidence_source",
        "name": "confidence_source",
        "type": "text",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": { "min": 1, "max": 32, "pattern": "" }
      },
      {
        // decision_timestamp — RFC3339 UTC. The sort key for narrative
        // reconstruction.
        "system": false,
        "id": "rd_decision_at",
        "name": "decision_timestamp",
        "type": "date",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": { "min": "", "max": "" }
      },
      {
        // human_summary — single-line operator-readable description.
        // Always populated by RecordDecision; this is what appears in
        // narrative streams and UI tables.
        "system": false,
        "id": "rd_human_summary",
        "name": "human_summary",
        "type": "text",
        "required": false,
        "presentable": true,
        "unique": false,
        "options": { "min": null, "max": 1000, "pattern": "" }
      }
    ],
    "indexes": [
      "CREATE INDEX idx_rd_target ON runtime_decisions (target)",
      "CREATE INDEX idx_rd_target_decided ON runtime_decisions (target, decision_timestamp)",
      "CREATE INDEX idx_rd_decision_type ON runtime_decisions (decision_type)",
      "CREATE INDEX idx_rd_cycle_id ON runtime_decisions (reconcile_cycle_id)",
      "CREATE INDEX idx_rd_operation_id ON runtime_decisions (operation_id)"
    ],
    "listRule": null,
    "viewRule": null,
    "createRule": null,
    "updateRule": null,
    "deleteRule": null,
    "options": {}
  })

  return Dao(db).saveCollection(collection)
}, (db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("runtime_decisions")
  return dao.deleteCollection(collection)
})
