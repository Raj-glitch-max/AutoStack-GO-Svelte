/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("y9j3kqc5pl0rdma")

  // pending_destroy: true when an `endDate` was set while a deploy was in
  // flight. The dispatcher's release path checks this on completion and
  // flips the target to `deleting` so the next reconcile cycle dispatches
  // Destroy.
  //
  // Phase 2.3: closes the "destroy intent silently dropped during
  // in-flight deploy" hazard (H-1 in phase2.3/dangerous-ambiguity-inventory.md).
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "dt_pending_destroy",
    "name": "pending_destroy",
    "type": "bool",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {}
  }))

  return dao.saveCollection(collection)
}, (db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("y9j3kqc5pl0rdma")

  collection.schema.removeField("dt_pending_destroy")

  return dao.saveCollection(collection)
})
