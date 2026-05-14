/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const collection = new Collection({
    "id": "y9j3kqc5pl0rdma",
    "created": "2024-05-10 00:00:01.000Z",
    "updated": "2024-05-10 00:00:01.000Z",
    "name": "deployment_targets",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "dt_rollout",
        "name": "rollout",
        "type": "relation",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "collectionId": "22k6ts6gvnp46mc",
          "cascadeDelete": true,
          "minSelect": null,
          "maxSelect": 1,
          "displayFields": ["name"]
        }
      },
      {
        "system": false,
        "id": "dt_cloud_account",
        "name": "cloud_account",
        "type": "relation",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "collectionId": "x8h2mnb4pk9qclz",
          "cascadeDelete": false,
          "minSelect": null,
          "maxSelect": 1,
          "displayFields": ["name"]
        }
      },
      {
        "system": false,
        "id": "dt_provider",
        "name": "provider",
        "type": "select",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": {
          "maxSelect": 1,
          "values": ["aws-ecs", "gcp-cloudrun", "azure-aca"]
        }
      },
      {
        "system": false,
        "id": "dt_region",
        "name": "region",
        "type": "text",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": {
          "min": null,
          "max": 50,
          "pattern": ""
        }
      },
      {
        "system": false,
        "id": "dt_external_id",
        "name": "external_id",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "min": null,
          "max": 500,
          "pattern": ""
        }
      },
      {
        "system": false,
        "id": "dt_status",
        "name": "status",
        "type": "select",
        "required": false,
        "presentable": true,
        "unique": false,
        "options": {
          "maxSelect": 1,
          "values": ["pending", "creating", "running", "updating", "stopped", "error", "deleting", "deleted"]
        }
      },
      {
        "system": false,
        "id": "dt_endpoint_url",
        "name": "endpoint_url",
        "type": "url",
        "required": false,
        "presentable": true,
        "unique": false,
        "options": {
          "exceptDomains": null,
          "onlyDomains": null
        }
      },
      {
        "system": false,
        "id": "dt_last_synced",
        "name": "last_synced",
        "type": "date",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "min": "",
          "max": ""
        }
      },
      {
        "system": false,
        "id": "dt_drift_detected",
        "name": "drift_detected",
        "type": "bool",
        "required": false,
        "presentable": true,
        "unique": false,
        "options": {}
      },
      {
        "system": false,
        "id": "dt_drift_summary",
        "name": "drift_summary",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "min": null,
          "max": 500,
          "pattern": ""
        }
      }
    ],
    "indexes": [],
    "listRule": "@request.auth.id != ''",
    "viewRule": "@request.auth.id != ''",
    "createRule": "@request.auth.id != ''",
    "updateRule": "@request.auth.id != ''",
    "deleteRule": "@request.auth.id != ''",
    "options": {}
  });

  return Dao(db).saveCollection(collection);
}, (db) => {
  const dao = new Dao(db);
  const collection = dao.findCollectionByNameOrId("y9j3kqc5pl0rdma");

  return dao.deleteCollection(collection);
})