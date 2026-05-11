/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const collection = new Collection({
    "id": "webhooks_collection",
    "name": "webhooks",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "webhook_url",
        "name": "url",
        "type": "url",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "exceptDomains": []
        }
      },
      {
        "system": false,
        "id": "webhook_name",
        "name": "name",
        "type": "text",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": {
          "min": 1,
          "max": 100
        }
      },
      {
        "system": false,
        "id": "webhook_events",
        "name": "events",
        "type": "json",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "maxSize": 1000
        }
      },
      {
        "system": false,
        "id": "webhook_user",
        "name": "user",
        "type": "relation",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "collectionId": "_pb_users_auth_",
          "cascadeDelete": true,
          "minSelect": null,
          "maxSelect": 1,
          "displayFields": []
        }
      },
      {
        "system": false,
        "id": "webhook_enabled",
        "name": "enabled",
        "type": "bool",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {}
      },
      {
        "system": false,
        "id": "webhook_secret",
        "name": "secret",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "min": null,
          "max": 100
        }
      },
      {
        "system": false,
        "id": "webhook_last_triggered",
        "name": "lastTriggered",
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
        "id": "webhook_failure_count",
        "name": "failureCount",
        "type": "number",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "min": 0,
          "max": null
        }
      }
    ],
    "indexes": [
      "CREATE INDEX idx_webhooks_user ON webhooks (user)",
      "CREATE INDEX idx_webhooks_enabled ON webhooks (enabled)"
    ],
    "listRule": "@request.auth.id != '' && user = @request.auth.id",
    "viewRule": "@request.auth.id != '' && user = @request.auth.id",
    "createRule": "@request.auth.id != ''",
    "updateRule": "@request.auth.id != '' && user = @request.auth.id",
    "deleteRule": "@request.auth.id != '' && user = @request.auth.id"
  });

  return Dao(db).saveCollection(collection);
}, (db) => {
  const dao = new Dao(db);
  const collection = dao.findCollectionByNameOrId("webhooks_collection");

  return dao.deleteCollection(collection);
});
