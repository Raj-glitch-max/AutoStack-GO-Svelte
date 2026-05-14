/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("operations")

  // Phase 3 SC-2: add operations.cycle_id column.
  //
  // Rationale: see project-context/phase2.9/deferred-Phase3-concerns.md §SC-2.
  //
  // Purpose: durable per-operation identity for future worker pool (RA-1).
  // When multiple workers are dispatching, any worker must be able to resume
  // any op. The in-memory __cycle_id context key does not survive worker
  // boundaries. This column stores the cycle_id at claim time so the
  // lineage is visible without process-context threading.
  //
  // Nullable: existing rows left null (they belonged to Phase 2 single-worker
  // world where in-memory cycle_id was sufficient). Phase 3 workers populate
  // this at createOperation time.

  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "ops_cycle_id",
    "name": "cycle_id",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "max": 32
    }
  }))

  return dao.saveCollection(collection)
}, (db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("operations")
  collection.schema.removeField("ops_cycle_id")
  return dao.saveCollection(collection)
})
