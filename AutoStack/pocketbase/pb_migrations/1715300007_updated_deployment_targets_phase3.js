/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("y9j3kqc5pl0rdma")

  // Phase 3.0 — Ambiguity columns.
  //
  // See project-context/phase3/ambiguity-semantics-model.md and
  // project-context/phase3/lifecycle-normalization-model.md.
  //
  // These columns are additive. Phase 2 readers ignore them; Phase 3
  // readers surface ambiguity honestly. The reconciler must NEVER
  // present a target as certain when these columns indicate otherwise
  // (P-5: UI surfaces ambiguity).

  // lifecycle_ambiguous: true when the provider-native state has lossy
  // mapping (N-8) OR when a bounded confirmation window expired (S-4)
  // OR when the provider has gone silent beyond the eventual-consistency
  // window (S-5). UI MUST surface this.
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "dt_lifecycle_ambiguous",
    "name": "lifecycle_ambiguous",
    "type": "bool",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {}
  }))

  // lifecycle_native_state: the provider's native state name preserved
  // verbatim. Required when lifecycle_ambiguous=true so the operator
  // can investigate. Free-form string; provider-specific format.
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "dt_lifecycle_native_state",
    "name": "lifecycle_native_state",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "max": 200
    }
  }))

  // lifecycle_ambiguity_source: which of S-1..S-5 produced the ambiguity.
  // Required when lifecycle_ambiguous=true so the reconciler and UI can
  // route handling correctly.
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "dt_lifecycle_ambiguity_source",
    "name": "lifecycle_ambiguity_source",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "max": 16
    }
  }))

  // lifecycle_ambiguity_detail: free-text reason. Surfaces in UI tooltip
  // and in lineage history rows.
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "dt_lifecycle_ambiguity_detail",
    "name": "lifecycle_ambiguity_detail",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "max": 500
    }
  }))

  // lifecycle_ambiguity_deadline: when a bounded ambiguity (S-1, S-4)
  // self-resolves. Null for unbounded ambiguities (S-2, S-3, S-5). The
  // reconciler clears the ambiguity bit on the next confirming poll OR
  // on deadline expiry (with an [AMBIGUITY_TIMEOUT] log + history row).
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "dt_lifecycle_ambiguity_deadline",
    "name": "lifecycle_ambiguity_deadline",
    "type": "date",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "min": "",
      "max": ""
    }
  }))

  return dao.saveCollection(collection)
}, (db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("y9j3kqc5pl0rdma")

  collection.schema.removeField("dt_lifecycle_ambiguous")
  collection.schema.removeField("dt_lifecycle_native_state")
  collection.schema.removeField("dt_lifecycle_ambiguity_source")
  collection.schema.removeField("dt_lifecycle_ambiguity_detail")
  collection.schema.removeField("dt_lifecycle_ambiguity_deadline")

  return dao.saveCollection(collection)
})
