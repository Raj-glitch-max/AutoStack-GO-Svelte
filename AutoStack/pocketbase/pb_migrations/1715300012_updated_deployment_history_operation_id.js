/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("deployment_history")

  // Phase 3.3 — operation_id causality link.
  //
  // Links each deployment_history row to the operations row that caused it.
  // This makes deployment history a FORENSIC artifact rather than a UI
  // decoration: an operator can reconstruct the full execution trace from
  // deployment_history rows, then drill into operations for exec_stage
  // granularity and checkpoint_data.
  //
  // Nullable: history rows written by pre-3.3 code (or by paths outside
  // dispatch, such as the startup sweep) have no operation_id.
  //
  // Note: declared as plain text (not relation) to avoid cascade-delete
  // behaviour if an operation row is ever cleaned up. The history row
  // is immutable; the operation row may be purged after a retention window.
  //
  // See dispatch.go writeHistory() for the write path.

  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "dh_operation_id",
    "name": "operation_id",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "min": null,
      "max": 15,
      "pattern": ""
    }
  }))

  // triggered_by: human-readable cause string (e.g. "stale-spec-loop",
  // "operator-rollback", "auto-destroy", "ambiguity-expiry"). Provides
  // causal context for UI and forensic investigation without requiring
  // a full join to the triggering operation.
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "dh_triggered_by",
    "name": "triggered_by",
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

  return dao.saveCollection(collection)
}, (db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("deployment_history")
  collection.schema.removeField("dh_operation_id")
  collection.schema.removeField("dh_triggered_by")
  return dao.saveCollection(collection)
})
