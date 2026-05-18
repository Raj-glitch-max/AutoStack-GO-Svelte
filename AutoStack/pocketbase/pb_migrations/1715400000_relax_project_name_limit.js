/// <reference path="../pb_data/types.d.ts" />
// PX-RUNTIME: raise the projects.name max length from 20 to 100 chars.
// Reason: 20 chars rejected reasonable names like "Customer Portal Production"
// with a generic "Failed to create record" toast. Frontend now surfaces the
// real validation message; this migration removes the underlying limit.
migrate((db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("7kff2zw80a7rmbu")

  // Re-add the field with the same id — addField replaces by id, preserving
  // existing column data (PocketBase migration semantics).
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "tespr1c6",
    "name": "name",
    "type": "text",
    "required": true,
    "presentable": false,
    "unique": false,
    "options": {
      "min": null,
      "max": 100,
      "pattern": ""
    }
  }))

  return dao.saveCollection(collection)
}, (db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("7kff2zw80a7rmbu")

  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "tespr1c6",
    "name": "name",
    "type": "text",
    "required": true,
    "presentable": false,
    "unique": false,
    "options": {
      "min": null,
      "max": 20,
      "pattern": ""
    }
  }))

  return dao.saveCollection(collection)
})
