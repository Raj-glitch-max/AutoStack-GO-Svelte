/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)

  // Phase 4.3 — Real Provider Execution + Durable Control Plane.
  //
  // Creates five collections that provide the durable persistence layer for
  // the operator-authoritative execution control plane.
  //
  // Design principles:
  //   - All records are append-only or update-once (resolution/expiry).
  //   - No record is ever deleted — audit history is permanent.
  //   - Typed outcome fields replace freeform string sentinel values.
  //   - The FSM state is reconstructed from transitions, not stored directly.
  //
  // Collections:
  //   execution_journals      — per-node action audit log entries
  //   execution_transitions   — FSM state transitions (persist BEFORE applying)
  //   execution_approvals     — operator gate decisions (approve/reject)
  //   execution_leases        — single-holder execution leases
  //   execution_ambiguities   — first-class ambiguity records requiring operator review

  // ────────────────────────────────────────────────────────────────────────
  // 1. execution_journals
  // ────────────────────────────────────────────────────────────────────────
  dao.saveCollection(new Collection({
    "id": "execution_journals",
    "name": "execution_journals",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "ej_execution_id",
        "name": "execution_id",
        "type": "text",
        "required": true,
        "presentable": true,
        "options": { "min": 1, "max": 128, "pattern": "" }
      },
      {
        "system": false,
        "id": "ej_stage_id",
        "name": "stage_id",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 128, "pattern": "" }
      },
      {
        "system": false,
        "id": "ej_node_id",
        "name": "node_id",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 128, "pattern": "" }
      },
      {
        "system": false,
        "id": "ej_intended_action",
        "name": "intended_action",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 32, "pattern": "" }
      },
      {
        "system": false,
        "id": "ej_timestamp",
        "name": "timestamp",
        "type": "text",
        "required": true,
        "presentable": false,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        // outcome — typed enum: "", "started", "success", "ambiguous", "failed", "cancelled"
        // Replay logic uses this field EXCLUSIVELY — never the note field.
        "system": false,
        "id": "ej_outcome",
        "name": "outcome",
        "type": "text",
        "required": false,
        "presentable": true,
        "options": { "min": 0, "max": 32, "pattern": "" }
      },
      {
        "system": false,
        "id": "ej_failure_mode",
        "name": "failure_mode",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 64, "pattern": "" }
      },
      {
        "system": false,
        "id": "ej_note",
        "name": "note",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 1024, "pattern": "" }
      },
      {
        "system": false,
        "id": "ej_plan_hash",
        "name": "plan_hash",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 128, "pattern": "" }
      },
      {
        "system": false,
        "id": "ej_execution_plan_hash",
        "name": "execution_plan_hash",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 128, "pattern": "" }
      },
      {
        "system": false,
        "id": "ej_expected_outcome",
        "name": "expected_outcome",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 256, "pattern": "" }
      },
      {
        "system": false,
        "id": "ej_rollback_reference",
        "name": "rollback_reference",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 128, "pattern": "" }
      },
      {
        "system": false,
        "id": "ej_compensation_reference",
        "name": "compensation_reference",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 128, "pattern": "" }
      },
      {
        "system": false,
        "id": "ej_human_approval_ref",
        "name": "human_approval_ref",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 128, "pattern": "" }
      }
    ],
    "indexes": [
      "CREATE INDEX idx_ej_execution_id ON execution_journals (execution_id)",
      "CREATE INDEX idx_ej_timestamp ON execution_journals (timestamp)",
      "CREATE INDEX idx_ej_outcome ON execution_journals (execution_id, outcome)"
    ],
    "listRule": null,
    "viewRule": null,
    "createRule": null,
    "updateRule": null,
    "deleteRule": null,
    "options": {}
  }))

  // ────────────────────────────────────────────────────────────────────────
  // 2. execution_transitions
  // ────────────────────────────────────────────────────────────────────────
  dao.saveCollection(new Collection({
    "id": "execution_transitions",
    "name": "execution_transitions",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "et_execution_id",
        "name": "execution_id",
        "type": "text",
        "required": true,
        "presentable": true,
        "options": { "min": 1, "max": 128, "pattern": "" }
      },
      {
        "system": false,
        "id": "et_from_state",
        "name": "from_state",
        "type": "text",
        "required": true,
        "presentable": false,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        "system": false,
        "id": "et_to_state",
        "name": "to_state",
        "type": "text",
        "required": true,
        "presentable": true,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        "system": false,
        "id": "et_reason",
        "name": "reason",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 512, "pattern": "" }
      },
      {
        "system": false,
        "id": "et_timestamp",
        "name": "timestamp",
        "type": "text",
        "required": true,
        "presentable": false,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        "system": false,
        "id": "et_actor_id",
        "name": "actor_id",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 128, "pattern": "" }
      }
    ],
    "indexes": [
      "CREATE INDEX idx_et_execution_id ON execution_transitions (execution_id)",
      "CREATE INDEX idx_et_timestamp ON execution_transitions (execution_id, timestamp)"
    ],
    "listRule": null,
    "viewRule": null,
    "createRule": null,
    "updateRule": null,
    "deleteRule": null,
    "options": {}
  }))

  // ────────────────────────────────────────────────────────────────────────
  // 3. execution_approvals
  // ────────────────────────────────────────────────────────────────────────
  dao.saveCollection(new Collection({
    "id": "execution_approvals",
    "name": "execution_approvals",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "ea_execution_id",
        "name": "execution_id",
        "type": "text",
        "required": true,
        "presentable": true,
        "options": { "min": 1, "max": 128, "pattern": "" }
      },
      {
        "system": false,
        "id": "ea_gate_id",
        "name": "gate_id",
        "type": "text",
        "required": true,
        "presentable": true,
        "options": { "min": 1, "max": 128, "pattern": "" }
      },
      {
        "system": false,
        "id": "ea_stage_id",
        "name": "stage_id",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 128, "pattern": "" }
      },
      {
        "system": false,
        "id": "ea_approved",
        "name": "approved",
        "type": "bool",
        "required": false,
        "presentable": true,
        "options": {}
      },
      {
        "system": false,
        "id": "ea_operator_id",
        "name": "operator_id",
        "type": "text",
        "required": true,
        "presentable": false,
        "options": { "min": 1, "max": 128, "pattern": "" }
      },
      {
        "system": false,
        "id": "ea_operator_role",
        "name": "operator_role",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 64, "pattern": "" }
      },
      {
        "system": false,
        "id": "ea_reason",
        "name": "reason",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 1024, "pattern": "" }
      },
      {
        "system": false,
        "id": "ea_timestamp",
        "name": "timestamp",
        "type": "text",
        "required": true,
        "presentable": false,
        "options": { "min": 1, "max": 64, "pattern": "" }
      }
    ],
    "indexes": [
      "CREATE INDEX idx_ea_execution_gate ON execution_approvals (execution_id, gate_id)",
      "CREATE INDEX idx_ea_timestamp ON execution_approvals (execution_id, timestamp)"
    ],
    "listRule": null,
    "viewRule": null,
    "createRule": null,
    "updateRule": null,
    "deleteRule": null,
    "options": {}
  }))

  // ────────────────────────────────────────────────────────────────────────
  // 4. execution_leases
  // ────────────────────────────────────────────────────────────────────────
  dao.saveCollection(new Collection({
    "id": "execution_leases",
    "name": "execution_leases",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "el_execution_id",
        "name": "execution_id",
        "type": "text",
        "required": true,
        "presentable": true,
        "options": { "min": 1, "max": 128, "pattern": "" }
      },
      {
        "system": false,
        "id": "el_holder_id",
        "name": "holder_id",
        "type": "text",
        "required": true,
        "presentable": false,
        "options": { "min": 1, "max": 128, "pattern": "" }
      },
      {
        "system": false,
        "id": "el_acquired_at",
        "name": "acquired_at",
        "type": "text",
        "required": true,
        "presentable": false,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        "system": false,
        "id": "el_expires_at",
        "name": "expires_at",
        "type": "text",
        "required": true,
        "presentable": false,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        "system": false,
        "id": "el_renewed",
        "name": "renewed",
        "type": "number",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": null, "noDecimal": true }
      },
      {
        "system": false,
        "id": "el_expired",
        "name": "expired",
        "type": "bool",
        "required": false,
        "presentable": true,
        "options": {}
      }
    ],
    "indexes": [
      "CREATE UNIQUE INDEX idx_el_execution_id ON execution_leases (execution_id)",
      "CREATE INDEX idx_el_expires_at ON execution_leases (expires_at)"
    ],
    "listRule": null,
    "viewRule": null,
    "createRule": null,
    "updateRule": null,
    "deleteRule": null,
    "options": {}
  }))

  // ────────────────────────────────────────────────────────────────────────
  // 5. execution_ambiguities
  // ────────────────────────────────────────────────────────────────────────
  dao.saveCollection(new Collection({
    "id": "execution_ambiguities",
    "name": "execution_ambiguities",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "eamb_ambiguity_id",
        "name": "ambiguity_id",
        "type": "text",
        "required": true,
        "presentable": true,
        "options": { "min": 1, "max": 256, "pattern": "" }
      },
      {
        "system": false,
        "id": "eamb_execution_id",
        "name": "execution_id",
        "type": "text",
        "required": true,
        "presentable": true,
        "options": { "min": 1, "max": 128, "pattern": "" }
      },
      {
        "system": false,
        "id": "eamb_stage_id",
        "name": "stage_id",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 128, "pattern": "" }
      },
      {
        "system": false,
        "id": "eamb_node_id",
        "name": "node_id",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 128, "pattern": "" }
      },
      {
        // kind: "timeout"|"partial_apply"|"unverifiable"|"conflicting_state"|"verify_error"
        "system": false,
        "id": "eamb_kind",
        "name": "kind",
        "type": "text",
        "required": true,
        "presentable": true,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        "system": false,
        "id": "eamb_description",
        "name": "description",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 1024, "pattern": "" }
      },
      {
        "system": false,
        "id": "eamb_provider_state",
        "name": "provider_state",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 512, "pattern": "" }
      },
      {
        "system": false,
        "id": "eamb_detected_at",
        "name": "detected_at",
        "type": "text",
        "required": true,
        "presentable": false,
        "options": { "min": 1, "max": 64, "pattern": "" }
      },
      {
        "system": false,
        "id": "eamb_resolved",
        "name": "resolved",
        "type": "bool",
        "required": false,
        "presentable": true,
        "options": {}
      },
      {
        "system": false,
        "id": "eamb_resolution",
        "name": "resolution",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 1024, "pattern": "" }
      },
      {
        "system": false,
        "id": "eamb_resolved_at",
        "name": "resolved_at",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 64, "pattern": "" }
      },
      {
        "system": false,
        "id": "eamb_operator_id",
        "name": "operator_id",
        "type": "text",
        "required": false,
        "presentable": false,
        "options": { "min": 0, "max": 128, "pattern": "" }
      }
    ],
    "indexes": [
      "CREATE UNIQUE INDEX idx_eamb_ambiguity_id ON execution_ambiguities (ambiguity_id)",
      "CREATE INDEX idx_eamb_execution_id ON execution_ambiguities (execution_id)",
      "CREATE INDEX idx_eamb_unresolved ON execution_ambiguities (execution_id, resolved)"
    ],
    "listRule": null,
    "viewRule": null,
    "createRule": null,
    "updateRule": null,
    "deleteRule": null,
    "options": {}
  }))

}, (db) => {
  const dao = new Dao(db)
  for (const name of [
    "execution_ambiguities",
    "execution_leases",
    "execution_approvals",
    "execution_transitions",
    "execution_journals",
  ]) {
    try {
      const col = dao.findCollectionByNameOrId(name)
      dao.deleteCollection(col)
    } catch (_) {}
  }
})
