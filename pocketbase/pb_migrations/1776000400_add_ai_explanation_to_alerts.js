/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("costAlerts")

  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "ai_explanation",
    "name": "aiExplanation",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": { "min": null, "max": 2000 }
  }))

  return dao.saveCollection(collection)
}, (db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("costAlerts")
  collection.schema.removeField("ai_explanation")
  return dao.saveCollection(collection)
})
