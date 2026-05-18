/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)
  const targets = dao.findCollectionByNameOrId("deployment_targets")

  // Phase 3.9.3 Runtime Integration — target-side retry scheduling.
  //
  // The retry engine (pkg/reconciler/retry.go) computes WHEN the next
  // retry attempt may run. That decision MUST drive the reconciler
  // dispatch loop deterministically, not just be persisted as forensic
  // data. To do that, the target row needs two new columns:
  //
  //   next_retry_at         — absolute UTC time at which the target
  //                            becomes dispatch-eligible after a failure
  //   consecutive_failures  — number of consecutive failures on this
  //                            target (across operations); used to enforce
  //                            the retry budget at the target level even
  //                            after the original operation row is
  //                            compacted by retention
  //
  // Both are nullable: pre-3.9.3 targets and freshly-created targets have
  // neither set.
  //
  // The new status "retry_pending" is NOT a DB enum (status is free-form
  // text); it's enforced by the lifecycle_guards.go transition table.

  targets.schema.addField(new SchemaField({
    "system": false,
    "id": "dt_next_retry_at",
    "name": "next_retry_at",
    "type": "date",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "min": "",
      "max": ""
    }
  }))

  targets.schema.addField(new SchemaField({
    "system": false,
    "id": "dt_consecutive_failures",
    "name": "consecutive_failures",
    "type": "number",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "min": 0,
      "max": null,
      "noDecimal": true
    }
  }))

  return dao.saveCollection(targets)
}, (db) => {
  const dao = new Dao(db)
  const targets = dao.findCollectionByNameOrId("deployment_targets")
  targets.schema.removeField("dt_next_retry_at")
  targets.schema.removeField("dt_consecutive_failures")
  return dao.saveCollection(targets)
})
