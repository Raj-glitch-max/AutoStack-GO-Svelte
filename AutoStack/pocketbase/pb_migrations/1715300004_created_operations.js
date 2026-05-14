/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const collection = new Collection({
    "id": "a2m5nse7qp2tfng",
    "created": "2024-05-14 00:00:00.000Z",
    "updated": "2024-05-14 00:00:00.000Z",
    "name": "operations",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "op_target",
        "name": "target",
        "type": "relation",
        "required": true,
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
        "id": "op_kind",
        "name": "kind",
        "type": "select",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": {
          "maxSelect": 1,
          "values": ["deploy", "rollback", "destroy"]
        }
      },
      {
        "system": false,
        "id": "op_status",
        "name": "status",
        "type": "select",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": {
          "maxSelect": 1,
          "values": ["pending", "in_progress", "succeeded", "succeeded_stale", "failed", "cancelled"]
        }
      },
      {
        "system": false,
        "id": "op_started_at",
        "name": "started_at",
        "type": "date",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": { "min": "", "max": "" }
      },
      {
        "system": false,
        "id": "op_updated_at",
        "name": "updated_at",
        "type": "date",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": { "min": "", "max": "" }
      },
      {
        "system": false,
        "id": "op_message",
        "name": "message",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 1000, "pattern": "" }
      },
      {
        "system": false,
        "id": "op_provider_op_id",
        "name": "provider_operation_id",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 500, "pattern": "" }
      },
      {
        "system": false,
        "id": "op_rollout_revision",
        "name": "rollout_revision",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": { "min": null, "max": 50, "pattern": "" }
      }
    ],
    "indexes": [
      "CREATE INDEX op_target_idx ON operations (target)",
      "CREATE INDEX op_status_idx ON operations (status)",
      "CREATE INDEX op_started_idx ON operations (started_at)"
    ],
    "listRule": "@request.auth.id != ''",
    "viewRule": "@request.auth.id != ''",
    "createRule": "@request.auth.id != ''",
    "updateRule": "@request.auth.id != ''",
    "deleteRule": null,
    "options": {}
  });

  return Dao(db).saveCollection(collection);
}, (db) => {
  const dao = new Dao(db);
  const collection = dao.findCollectionByNameOrId("a2m5nse7qp2tfng");
  return dao.deleteCollection(collection);
})
