/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("deployment_history")

  // Phase 3 SC-4: add `stale` enum value to deployment_history.status.
  //
  // Rationale: see project-context/phase2.9/deferred-Phase3-concerns.md §SC-4.
  //
  // A history row with status=failed and message="stale spec" is ambiguous —
  // did the deploy FAIL or did it succeed with an immediately-outdated spec?
  // Adding status=stale makes history queries unambiguous.
  //
  // This column stores a select field. PocketBase select fields support
  // arbitrary string values; we add `stale` to the options list.
  //
  // Existing rows with status=succeeded_stale in the operations table will
  // write status=stale into deployment_history from this migration forward.
  // The dispatcher's stale branch (dispatch.go) still writes
  // operations.status=succeeded_stale; the history row gets status=stale.
  //
  // Implementation note: PocketBase select fields are stored as JSON arrays;
  // adding a value here extends the allowed set without breaking existing rows.

  const historyStatusField = collection.schema.getFieldByName("status")
  if (historyStatusField) {
    // Extend options to include stale.
    const opts = historyStatusField.options || {}
    const existing = opts.values || []
    if (!existing.includes("stale")) {
      existing.push("stale")
      historyStatusField.options = { values: existing }
    }
    return dao.saveCollection(collection)
  }

  // If no status field exists (schema variant), add it with full enum.
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "dh_status",
    "name": "status",
    "type": "select",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "maxSelect": 1,
      "values": ["in_progress", "success", "failed", "stale"]
    }
  }))

  return dao.saveCollection(collection)
}, (db) => {
  // Down: removal of the stale value from options is a no-op for existing
  // rows (they would just have an "invalid" value which PocketBase tolerates
  // for select fields at read time). The safer down is to leave the value in
  // place; the field cannot be narrowed without risking data loss.
  return null
})
