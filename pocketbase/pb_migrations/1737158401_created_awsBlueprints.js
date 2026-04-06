/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const collection = new Collection({
    "id": "aws_blueprints_001",
    "created": "2026-03-17 12:00:01.000Z",
    "updated": "2026-03-17 12:00:01.000Z",
    "name": "awsBlueprints",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "aws_bp_name",
        "name": "name",
        "type": "text",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": {
          "min": 1,
          "max": 100,
          "pattern": ""
        }
      },
      {
        "system": false,
        "id": "aws_bp_desc",
        "name": "description",
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
        "id": "aws_bp_category",
        "name": "category",
        "type": "select",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "maxSelect": 1,
          "values": [
            "web-app",
            "full-stack",
            "static-site",
            "serverless",
            "database",
            "custom"
          ]
        }
      },
      {
        "system": false,
        "id": "aws_bp_template",
        "name": "terraformTemplate",
        "type": "text",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "min": 1,
          "max": 50000,
          "pattern": ""
        }
      },
      {
        "system": false,
        "id": "aws_bp_schema",
        "name": "configSchema",
        "type": "json",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "maxSize": 10000
        }
      },
      {
        "system": false,
        "id": "aws_bp_regions",
        "name": "supportedRegions",
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
        "id": "aws_bp_cost",
        "name": "estimatedCost",
        "type": "json",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "maxSize": 2000
        }
      },
      {
        "system": false,
        "id": "aws_bp_logo",
        "name": "logo",
        "type": "file",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "maxSelect": 1,
          "maxSize": 2097152,
          "mimeTypes": [
            "image/png",
            "image/jpeg",
            "image/svg+xml"
          ],
          "thumbs": [
            "100x100"
          ],
          "protected": false
        }
      },
      {
        "system": false,
        "id": "aws_bp_owner",
        "name": "owner",
        "type": "relation",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "collectionId": "_pb_users_auth_",
          "cascadeDelete": false,
          "minSelect": null,
          "maxSelect": 1,
          "displayFields": ["email"]
        }
      },
      {
        "system": false,
        "id": "aws_bp_public",
        "name": "public",
        "type": "bool",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {}
      }
    ],
    "indexes": [
      "CREATE INDEX idx_aws_blueprints_owner ON awsBlueprints (owner)",
      "CREATE INDEX idx_aws_blueprints_category ON awsBlueprints (category)",
      "CREATE INDEX idx_aws_blueprints_public ON awsBlueprints (public)"
    ],
    "listRule": "public = true || owner = @request.auth.id",
    "viewRule": "public = true || owner = @request.auth.id",
    "createRule": "@request.auth.id != \"\" && owner = @request.auth.id",
    "updateRule": "@request.auth.id != \"\" && owner = @request.auth.id",
    "deleteRule": "@request.auth.id != \"\" && owner = @request.auth.id",
    "options": {}
  });

  return Dao(db).saveCollection(collection);
}, (db) => {
  const dao = new Dao(db);
  const collection = dao.findCollectionByNameOrId("aws_blueprints_001");

  return dao.deleteCollection(collection);
})