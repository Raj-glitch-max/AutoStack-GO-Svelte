/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const collection = new Collection({
    "id": "webhooks_col",
    "name": "webhooks",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "webhook_project_rel",
        "name": "project",
        "type": "relation",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "collectionId": "7kff2zw80a7rmbu",
          "cascadeDelete": true,
          "minSelect": null,
          "maxSelect": 1,
          "displayFields": null
        }
      },
      {
        "system": false,
        "id": "webhook_url_text",
        "name": "url",
        "type": "url",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "exceptDomains": null,
          "onlyDomains": null
        }
      },
      {
        "system": false,
        "id": "webhook_provider_select",
        "name": "provider",
        "type": "select",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "maxSelect": 1,
          "values": [
            "slack",
            "discord",
            "generic"
          ]
        }
      },
      {
        "system": false,
        "id": "webhook_events_json",
        "name": "events",
        "type": "json",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {}
      },
      {
        "system": false,
        "id": "webhook_active_bool",
        "name": "active",
        "type": "bool",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {}
      }
    ],
    "indexes": [],
    "listRule": "@request.auth.id != '' && @collection.project_members.project = project && @collection.project_members.user = @request.auth.id",
    "viewRule": "@request.auth.id != '' && @collection.project_members.project = project && @collection.project_members.user = @request.auth.id",
    "createRule": "@request.auth.id != '' && @collection.project_members.project = project && @collection.project_members.user = @request.auth.id && @collection.project_members.role = 'owner'",
    "updateRule": "@request.auth.id != '' && @collection.project_members.project = project && @collection.project_members.user = @request.auth.id && @collection.project_members.role = 'owner'",
    "deleteRule": "@request.auth.id != '' && @collection.project_members.project = project && @collection.project_members.user = @request.auth.id && @collection.project_members.role = 'owner'",
    "options": {}
  });

  return Dao(db).saveCollection(collection);
}, (db) => {
  const dao = new Dao(db);
  const collection = dao.findCollectionByNameOrId("webhooks_col");
  return dao.deleteCollection(collection);
})
