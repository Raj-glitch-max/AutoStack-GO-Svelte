/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("aws_deployments_001")

  // Add costAlertThreshold field to awsDeployments collection
  // Validates: AC-4.4 (User can set custom alert thresholds per deployment)
  collection.schema.addField(new SchemaField({
    "system": false,
    "id": "aws_dep_threshold",
    "name": "costAlertThreshold",
    "type": "number",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "min": 0,
      "max": null,
      "noDecimal": false
    }
  }))

  return dao.saveCollection(collection)
}, (db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("aws_deployments_001")

  // Remove costAlertThreshold field
  collection.schema.removeField("aws_dep_threshold")

  return dao.saveCollection(collection)
})
