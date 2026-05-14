/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("operations")

  // Phase 3.2 — exec_stage and checkpoint_data columns on operations.
  //
  // exec_stage: the fine-grained stage of an in-flight operation.
  // Values: queued, dispatching, provisioning, configuring, verifying,
  //         ready, partial, failed, rolling_back, rolled_back,
  //         deleting, deleted, ambiguous.
  //
  // Rationale: operations.status ("in_progress/succeeded/failed") is too
  // coarse for operator-visible diagnostics. After a process crash, an
  // operator cannot tell whether the provider had started resource creation
  // (provisioning) or was still waiting for CAS (dispatching). exec_stage
  // makes the stage visible without changing the status semantics.
  //
  // See pkg/reconciler/exec_stage.go for the authoritative model.
  //
  // Existing rows: null (they were Phase 2 operations that predated exec_stage).
  // Phase 3 operations populate this at each stage transition.

  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "ops_exec_stage",
    "name": "exec_stage",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "max": 32
    }
  }))

  // checkpoint_data: provider-specific state captured at each exec stage.
  // JSON blob. Examples:
  //   ECS provisioning: {"task_def_arn":"arn:...", "cluster":"autostack-us-east-1"}
  //   Cloud Run provisioning: {"service_name":"my-svc", "region":"us-central1"}
  //
  // Purpose: makes partial resource creation visible to operators and
  // provides the foundation for Phase 4 smart continuation (resuming
  // from checkpoint instead of re-running full Deploy()).
  //
  // Phase 3.2 behaviour: checkpoint_data is WRITTEN by the dispatcher but
  // NOT READ for continuation — restart still uses the mark-failed-redispatch
  // pattern. The data is for operator observability and future Phase 4 use.
  //
  // Null for operations that predated this migration or where checkpoint
  // was not applicable (destroy operations, cancelled operations).

  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "ops_checkpoint_data",
    "name": "checkpoint_data",
    "type": "json",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {}
  }))

  return dao.saveCollection(collection)
}, (db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("operations")
  collection.schema.removeField("ops_exec_stage")
  collection.schema.removeField("ops_checkpoint_data")
  return dao.saveCollection(collection)
})
