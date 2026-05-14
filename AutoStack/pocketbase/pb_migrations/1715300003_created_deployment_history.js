/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const collection = new Collection({
    "id": "z1k4lrd6mp1semf",
    "created": "2024-05-10 00:00:03.000Z",
    "updated": "2024-05-10 00:00:03.000Z",
    "name": "deployment_history",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "dh_rollout",
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
        "id": "dh_target",
        "name": "target",
        "type": "relation",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "collectionId": "y9j3kqc5pl0rdma",
          "cascadeDelete": true,
          "minSelect": null,
          "maxSelect": 1,
          "displayFields": ["external_id"]
        }
      },
      {
        "system": false,
        "id": "dh_action",
        "name": "action",
        "type": "select",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": {
          "maxSelect": 1,
          "values": ["created", "updated", "rolled_back", "deleted", "error", "recovered"]
        }
      },
      {
        "system": false,
        "id": "dh_from_revision",
        "name": "from_revision",
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
        "id": "dh_to_revision",
        "name": "to_revision",
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
        "id": "dh_status",
        "name": "status",
        "type": "select",
        "required": false,
        "presentable": true,
        "unique": false,
        "options": {
          "maxSelect": 1,
          "values": ["success", "failed", "in_progress"]
        }
      },
      {
        "system": false,
        "id": "dh_message",
        "name": "message",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "min": null,
          "max": 1000,
          "pattern": ""
        }
      },
      {
        "system": false,
        "id": "dh_provider",
        "name": "provider",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "min": null,
          "max": 50,
          "pattern": ""
        }
      }
    ],
    "indexes": [
      "CREATE INDEX dh_rollout_idx ON deployment_history (rollout)",
      "CREATE INDEX dh_target_idx ON deployment_history (target)",
      "CREATE INDEX dh_action_idx ON deployment_history (action)",
      "CREATE INDEX dh_created_idx ON deployment_history (created)"
    ],
    // History is immutable - no update or delete rules
    "listRule": "@request.auth.id != ''",
    "viewRule": "@request.auth.id != ''",
    "createRule": "@request.auth.id != ''",
    "updateRule": null,  // Immutable - no updates
    "deleteRule": null,  // Immutable - no deletes
    "options": {}
  });

  return Dao(db).saveCollection(collection);
}, (db) => {
  const dao = new Dao(db);
  const collection = dao.findCollectionByNameOrId("z1k4lrd6mp1semf");

  return dao.deleteCollection(collection);
})