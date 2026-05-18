/// <reference path="../pb_data/types.d.ts" />
// Phase PX-2: add observability fields to runtime_deployments.
//
//   * started_at            — when the container first became certified
//   * last_health_check_at  — most recent reconciler health probe time
//   * restart_count         — how many times the engine has recreated the
//                             container (rollback or recovery from drift)
//   * uptime_seconds        — computed at read time; not stored
//
// These are populated by the reconciler from REAL observations.  Nothing is
// synthesized.  If the field is empty, the engine hasn't observed enough yet.
migrate((db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("rt_deployments_001")
  if (!collection) {
    throw new Error("rt_deployments_001 not found")
  }

  collection.schema.addField(new SchemaField({
    system: false, id: "rtd_started", name: "started_at", type: "text",
    required: false, options: { min: null, max: 64, pattern: "" },
  }))
  collection.schema.addField(new SchemaField({
    system: false, id: "rtd_lhc", name: "last_health_check_at", type: "text",
    required: false, options: { min: null, max: 64, pattern: "" },
  }))
  collection.schema.addField(new SchemaField({
    system: false, id: "rtd_rcount", name: "restart_count", type: "number",
    required: false, options: { min: 0, max: null, noDecimal: true },
  }))

  return dao.saveCollection(collection)
}, (db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("rt_deployments_001")
  if (!collection) return null
  collection.schema.removeField("rtd_started")
  collection.schema.removeField("rtd_lhc")
  collection.schema.removeField("rtd_rcount")
  return dao.saveCollection(collection)
})
