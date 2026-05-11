/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("_pb_users_auth_")

  // Add alertPreferences JSON field
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "alertprefs",
    "name": "alertPreferences",
    "type": "json",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "maxSize": 2000
    }
  }))

  return dao.saveCollection(collection)
}, (db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("_pb_users_auth_")

  // Remove alertPreferences field
  collection.schema.removeField("alertprefs")

  return dao.saveCollection(collection)
})
