/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("22k6ts6gvnp46mc")

  // add target_type field
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "rollout_target_type",
    "name": "target_type",
    "type": "select",
    "required": false,
    "presentable": true,
    "unique": false,
    "options": {
      "maxSelect": 1,
      "values": ["kubernetes", "ecs", "cloudrun", "aca"]
    }
  }))

  // add target_config field
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "rollout_target_config",
    "name": "target_config",
    "type": "editor",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {}
  }))

  // add cloud_account relation
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "rollout_cloud_account",
    "name": "cloud_account",
    "type": "relation",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "collectionId": "x8h2mnb4pk9qclz",
      "cascadeDelete": false,
      "minSelect": null,
      "maxSelect": 1,
      "displayFields": ["name"]
    }
  }))

  // add endpoint_url field
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "rollout_endpoint_url",
    "name": "endpoint_url",
    "type": "url",
    "required": false,
    "presentable": true,
    "unique": false,
    "options": {
      "exceptDomains": null,
      "onlyDomains": null
    }
  }))

  return dao.saveCollection(collection)
}, (db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("22k6ts6gvnp46mc")

  // remove
  collection.schema.removeField("rollout_target_type")
  collection.schema.removeField("rollout_target_config")
  collection.schema.removeField("rollout_cloud_account")
  collection.schema.removeField("rollout_endpoint_url")

  return dao.saveCollection(collection)
})