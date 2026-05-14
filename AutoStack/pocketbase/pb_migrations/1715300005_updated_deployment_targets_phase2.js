/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("y9j3kqc5pl0rdma")

  // current_operation: relation to operations row currently in flight, null when idle.
  // The reconciler dispatcher uses a CAS against this column to prevent
  // duplicate Deploy invocations and to make multi-pod safe.
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "dt_current_operation",
    "name": "current_operation",
    "type": "relation",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "collectionId": "a2m5nse7qp2tfng",
      "cascadeDelete": false,
      "minSelect": null,
      "maxSelect": 1,
      "displayFields": ["kind"]
    }
  }))

  // current_revision: provider-side revision identifier of the last successful
  // deploy. Required for honest rollback semantics in the future.
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "dt_current_revision",
    "name": "current_revision",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": { "min": null, "max": 500, "pattern": "" }
  }))

  // last_state_change_at: timestamp of the most recent status transition.
  // Drives stuck-state detection (Phase 2.5+); written on every status change.
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "dt_last_state_change_at",
    "name": "last_state_change_at",
    "type": "date",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": { "min": "", "max": "" }
  }))

  return dao.saveCollection(collection)
}, (db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("y9j3kqc5pl0rdma")

  collection.schema.removeField("dt_current_operation")
  collection.schema.removeField("dt_current_revision")
  collection.schema.removeField("dt_last_state_change_at")

  return dao.saveCollection(collection)
})
