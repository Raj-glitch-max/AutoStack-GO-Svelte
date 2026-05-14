/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("deployment_targets")

  // Phase 3.2 — add rollback, partial, and ambiguous status values.
  //
  // The Phase 3.2 lifecycle model (lifecycle_guards.go) defines 11 canonical
  // target states. The original deployment_targets schema (migration
  // 1715300001) only declared 8 values. These additions make the select enum
  // complete and consistent with the authoritative transition table:
  //
  //   rolling_back — a rollback operation is in flight (Provider.Rollback call)
  //   rolled_back  — rollback confirmed; target is at the prior revision
  //   partial      — partial resource creation; some resources up, some failed
  //   ambiguous    — provider state is unclear; reconciler is polling
  //
  // Existing rows: no migration of data values. Pre-existing rows have status
  // values from the original 8-value set. New values are only written by the
  // Phase 3.2 reconciler paths.
  //
  // See lifecycle_guards.go targetTransitions for the complete state machine.

  const statusField = collection.schema.getFieldByName("status")
  statusField.options.values = [
    "pending",
    "creating",
    "running",
    "updating",
    "stopped",
    "error",
    "deleting",
    "deleted",
    "rolling_back",
    "rolled_back",
    "partial",
    "ambiguous",
  ]

  return dao.saveCollection(collection)
}, (db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("deployment_targets")

  // Revert to Phase 2 status values. Any rows with the new status values
  // will be left inconsistent — operators must manually correct them if
  // rollback of this migration is needed.
  const statusField = collection.schema.getFieldByName("status")
  statusField.options.values = [
    "pending",
    "creating",
    "running",
    "updating",
    "stopped",
    "error",
    "deleting",
    "deleted",
  ]

  return dao.saveCollection(collection)
})
