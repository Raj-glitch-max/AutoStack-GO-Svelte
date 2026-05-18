/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)

  // Phase 3.9.4 — Incident Truthfulness & Operational Forensics.
  //
  // An "incident" is a first-class persisted operational object that
  // captures a runtime failure or escalation event. It is NOT a derived
  // view of operations + history; it has its OWN lifecycle, state machine,
  // and retention rules.
  //
  // ─────────────────────────────────────────────────────────────────────
  // RELATIONSHIP TO EXISTING TYPES
  // ─────────────────────────────────────────────────────────────────────
  //
  //   operations          — what the dispatcher was doing
  //   deployment_history  — chronological record of state transitions
  //   incidents           — Phase 3.9.4 first-class incident object (THIS TABLE)
  //
  // Incidents reference operations (first_operation_id, last_operation_id)
  // but operations do NOT reference incidents. This avoids cascade-delete
  // surprises during retention compaction.
  //
  // ─────────────────────────────────────────────────────────────────────
  // STATE MACHINE
  // ─────────────────────────────────────────────────────────────────────
  //
  //   open        — newly created; runtime failure observed
  //   escalating  — severity increased (e.g., retry exhaustion on ambiguous)
  //   acknowledged — operator has seen and accepted the incident
  //   resolved    — operator declared resolved with explicit reason
  //   reopened    — operator reopened a previously-resolved incident
  //   suppressed  — operator suppressed (won't escalate; still preserved)
  //
  // FORBIDDEN: deleting a row before retention TTL. The retention sweep
  // in pkg/reconciler/cleanup.go MUST skip incidents whose status is not
  // 'resolved' (or whose resolved_at is within retention TTL).

  const collection = new Collection({
    "id": "incidents",
    "name": "incidents",
    "type": "base",
    "system": false,
    "schema": [
      {
        // target — deployment target the incident is about.
        // text rather than relation to avoid cascade-delete coupling.
        "system": false,
        "id": "inc_target",
        "name": "target",
        "type": "text",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        // rollout — rollout the target belongs to. Snapshot at creation time.
        "system": false,
        "id": "inc_rollout",
        "name": "rollout",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 64, "pattern": "" }
      },
      {
        // source — what triggered this incident.
        // Canonical values (pkg/reconciler/incidents.go IncidentSource):
        //   "retry-exhausted"
        //   "ambiguity-escalation"
        //   "lease-loss-mid-call"
        //   "interrupted-unknown-outcome"
        //   "rollback-failed"
        //   "reclaim-after-crash"
        //   "invariant-violation"  (Phase 3.9.5+ reserve)
        "system": false,
        "id": "inc_source",
        "name": "source",
        "type": "text",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        // severity — low / medium / high / critical.
        // Determined by source class and escalation_count.
        "system": false,
        "id": "inc_severity",
        "name": "severity",
        "type": "text",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": { "min": 1, "max": 16, "pattern": "" }
      },
      {
        // status — the incident's state-machine position.
        // Validated transitions in pkg/reconciler/incidents.go.
        "system": false,
        "id": "inc_status",
        "name": "status",
        "type": "text",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": { "min": 1, "max": 16, "pattern": "" }
      },
      {
        "system": false,
        "id": "inc_escalation_count",
        "name": "escalation_count",
        "type": "number",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": 0, "max": null, "noDecimal": true }
      },
      {
        "system": false,
        "id": "inc_first_seen_at",
        "name": "first_seen_at",
        "type": "date",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": { "min": "", "max": "" }
      },
      {
        "system": false,
        "id": "inc_last_seen_at",
        "name": "last_seen_at",
        "type": "date",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": { "min": "", "max": "" }
      },
      {
        // resolved_at — null until status transitions to resolved.
        // Retention TTL is measured from this timestamp.
        "system": false,
        "id": "inc_resolved_at",
        "name": "resolved_at",
        "type": "date",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": "", "max": "" }
      },
      {
        // resolution_reason — REQUIRED when status=resolved. Free-form
        // operator-readable text or canonical machine reason (e.g.,
        // "operator_manual_resolve", "auto_resolved_provider_healthy").
        "system": false,
        "id": "inc_resolution_reason",
        "name": "resolution_reason",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 500, "pattern": "" }
      },
      {
        // acknowledged_by — operator identifier (e.g., PocketBase user id).
        // Set when status transitions to acknowledged.
        "system": false,
        "id": "inc_acknowledged_by",
        "name": "acknowledged_by",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 64, "pattern": "" }
      },
      {
        // first_operation_id — operations.id that caused the incident.
        // Plain text (not relation) so retention compaction of the
        // operation row does not cascade-delete the incident.
        "system": false,
        "id": "inc_first_op",
        "name": "first_operation_id",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 15, "pattern": "" }
      },
      {
        // last_operation_id — most recent operations.id linked to this
        // incident. Updated on each subsequent failure on the same target
        // while the incident remains open.
        "system": false,
        "id": "inc_last_op",
        "name": "last_operation_id",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 15, "pattern": "" }
      },
      {
        // ambiguity_state, retry_state, lease_state — snapshot of runtime
        // context at the moment of escalation. JSON-serialized for
        // reconstruction. Provides "what did the runtime believe at the
        // time?" without log archaeology.
        "system": false,
        "id": "inc_ambig_state",
        "name": "ambiguity_state",
        "type": "json",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {}
      },
      {
        "system": false,
        "id": "inc_retry_state",
        "name": "retry_state",
        "type": "json",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {}
      },
      {
        "system": false,
        "id": "inc_lease_state",
        "name": "lease_state",
        "type": "json",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {}
      },
      {
        // forensic_summary — operator-readable summary in a single line.
        // Designed to be displayed in a list view without further query.
        "system": false,
        "id": "inc_forensic_summary",
        "name": "forensic_summary",
        "type": "text",
        "required": false,
        "presentable": true,
        "unique": false,
        "options": { "min": null, "max": 1000, "pattern": "" }
      }
    ],
    "indexes": [
      "CREATE INDEX idx_incidents_target ON incidents (target)",
      "CREATE INDEX idx_incidents_status ON incidents (status)",
      "CREATE INDEX idx_incidents_target_status ON incidents (target, status)",
      "CREATE INDEX idx_incidents_resolved_at ON incidents (resolved_at)"
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
  const collection = dao.findCollectionByNameOrId("incidents")
  return dao.deleteCollection(collection)
})
