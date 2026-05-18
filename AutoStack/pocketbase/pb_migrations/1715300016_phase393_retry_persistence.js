/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)
  const operations = dao.findCollectionByNameOrId("operations")

  // Phase 3.9.3 — Retry persistence columns.
  //
  // The canonical retry engine (pkg/reconciler/retry.go) uses these
  // columns to make replay-safe retry decisions:
  //
  //   retry_count           — attempts made so far (budget enforcement)
  //   retry_class           — canonical RetryClass string (e.g., "retry_transient")
  //   retry_backoff_until   — earliest time the next retry may run
  //   retry_native_code     — provider's native error code (forensics)
  //   retry_forensic_reason — operator-readable explanation (forensics)
  //   last_retry_at         — timestamp of the most recent retry decision
  //
  // CRITICAL design choice: these columns are PERSISTED, not in-memory.
  // A process crash must not erase the retry budget — otherwise a retry
  // storm could exceed MaxRetries silently after restart.
  //
  // retry_class is the stable canonical class name (see
  // pkg/providers/normalize.go RetryClass constants). Persisted name is
  // part of the schema contract — renaming a class requires a migration.

  operations.schema.addField(new SchemaField({
    "system": false,
    "id": "ops_retry_count",
    "name": "retry_count",
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

  operations.schema.addField(new SchemaField({
    "system": false,
    "id": "ops_retry_class",
    "name": "retry_class",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "min": null,
      "max": 32,
      "pattern": ""
    }
  }))

  operations.schema.addField(new SchemaField({
    "system": false,
    "id": "ops_retry_backoff_until",
    "name": "retry_backoff_until",
    "type": "date",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "min": "",
      "max": ""
    }
  }))

  operations.schema.addField(new SchemaField({
    "system": false,
    "id": "ops_retry_native_code",
    "name": "retry_native_code",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "min": null,
      "max": 128,
      "pattern": ""
    }
  }))

  operations.schema.addField(new SchemaField({
    "system": false,
    "id": "ops_retry_forensic_reason",
    "name": "retry_forensic_reason",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "min": null,
      "max": 500,
      "pattern": ""
    }
  }))

  operations.schema.addField(new SchemaField({
    "system": false,
    "id": "ops_last_retry_at",
    "name": "last_retry_at",
    "type": "date",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "min": "",
      "max": ""
    }
  }))

  return dao.saveCollection(operations)
}, (db) => {
  const dao = new Dao(db)
  const operations = dao.findCollectionByNameOrId("operations")
  operations.schema.removeField("ops_retry_count")
  operations.schema.removeField("ops_retry_class")
  operations.schema.removeField("ops_retry_backoff_until")
  operations.schema.removeField("ops_retry_native_code")
  operations.schema.removeField("ops_retry_forensic_reason")
  operations.schema.removeField("ops_last_retry_at")
  return dao.saveCollection(operations)
})
