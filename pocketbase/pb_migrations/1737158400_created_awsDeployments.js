/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const collection = new Collection({
    "id": "aws_deployments_001",
    "created": "2026-03-17 12:00:00.000Z",
    "updated": "2026-03-17 12:00:00.000Z",
    "name": "awsDeployments",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "aws_dep_name",
        "name": "name",
        "type": "text",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": {
          "min": 1,
          "max": 100,
          "pattern": "^[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]$"
        }
      },
      {
        "system": false,
        "id": "aws_dep_project",
        "name": "project",
        "type": "relation",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "collectionId": "projects",
          "cascadeDelete": true,
          "minSelect": null,
          "maxSelect": 1,
          "displayFields": ["name"]
        }
      },
      {
        "system": false,
        "id": "aws_dep_user",
        "name": "user",
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
        "id": "aws_dep_blueprint",
        "name": "blueprint",
        "type": "relation",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "collectionId": "awsBlueprints",
          "cascadeDelete": false,
          "minSelect": null,
          "maxSelect": 1,
          "displayFields": ["name"]
        }
      },
      {
        "system": false,
        "id": "aws_dep_region",
        "name": "region",
        "type": "select",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "maxSelect": 1,
          "values": [
            "us-east-1",
            "us-east-2", 
            "us-west-1",
            "us-west-2",
            "eu-west-1",
            "eu-west-2",
            "eu-central-1",
            "ap-southeast-1",
            "ap-southeast-2",
            "ap-northeast-1"
          ]
        }
      },
      {
        "system": false,
        "id": "aws_dep_status",
        "name": "status",
        "type": "select",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "maxSelect": 1,
          "values": [
            "pending",
            "planning",
            "applying",
            "active",
            "failed",
            "destroying",
            "destroyed"
          ]
        }
      },
      {
        "system": false,
        "id": "aws_dep_config",
        "name": "configuration",
        "type": "json",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "maxSize": 10000
        }
      },
      {
        "system": false,
        "id": "aws_dep_outputs",
        "name": "outputs",
        "type": "json",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "maxSize": 5000
        }
      }
    ],
    "indexes": [
      "CREATE INDEX idx_aws_deployments_user ON awsDeployments (user)",
      "CREATE INDEX idx_aws_deployments_project ON awsDeployments (project)",
      "CREATE INDEX idx_aws_deployments_status ON awsDeployments (status)",
      "CREATE INDEX idx_aws_deployments_region ON awsDeployments (region)"
    ],
    "listRule": "@request.auth.id != \"\" && user = @request.auth.id",
    "viewRule": "@request.auth.id != \"\" && user = @request.auth.id",
    "createRule": "@request.auth.id != \"\" && user = @request.auth.id",
    "updateRule": "@request.auth.id != \"\" && user = @request.auth.id",
    "deleteRule": "@request.auth.id != \"\" && user = @request.auth.id",
    "options": {}
  });

  return Dao(db).saveCollection(collection);
}, (db) => {
  const dao = new Dao(db);
  const collection = dao.findCollectionByNameOrId("aws_deployments_001");

  return dao.deleteCollection(collection);
})