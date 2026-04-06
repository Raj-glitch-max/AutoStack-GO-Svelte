/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const collection = new Collection({
    "id": "aws_rollouts_001",
    "created": "2026-03-17 12:00:02.000Z",
    "updated": "2026-03-17 12:00:02.000Z",
    "name": "awsRollouts",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "aws_ro_deployment",
        "name": "deployment",
        "type": "relation",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "collectionId": "awsDeployments",
          "cascadeDelete": true,
          "minSelect": null,
          "maxSelect": 1,
          "displayFields": ["name"]
        }
      },
      {
        "system": false,
        "id": "aws_ro_project",
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
        "id": "aws_ro_user",
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
        "id": "aws_ro_tf_config",
        "name": "terraformConfig",
        "type": "text",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "min": 1,
          "max": 100000,
          "pattern": ""
        }
      },
      {
        "system": false,
        "id": "aws_ro_state_ver",
        "name": "stateVersion",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "min": null,
          "max": 100,
          "pattern": ""
        }
      },
      {
        "system": false,
        "id": "aws_ro_start",
        "name": "startDate",
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
        "id": "aws_ro_end",
        "name": "endDate",
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
        "id": "aws_ro_status",
        "name": "status",
        "type": "select",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "maxSelect": 1,
          "values": [
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
        "id": "aws_ro_plan",
        "name": "planOutput",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "min": null,
          "max": 50000,
          "pattern": ""
        }
      },
      {
        "system": false,
        "id": "aws_ro_logs",
        "name": "executionLogs",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "min": null,
          "max": 100000,
          "pattern": ""
        }
      }
    ],
    "indexes": [
      "CREATE INDEX idx_aws_rollouts_deployment ON awsRollouts (deployment)",
      "CREATE INDEX idx_aws_rollouts_project ON awsRollouts (project)",
      "CREATE INDEX idx_aws_rollouts_user ON awsRollouts (user)",
      "CREATE INDEX idx_aws_rollouts_status ON awsRollouts (status)",
      "CREATE INDEX idx_aws_rollouts_start_date ON awsRollouts (startDate)",
      "CREATE INDEX idx_aws_rollouts_active ON awsRollouts (deployment, endDate) WHERE endDate IS NULL"
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
  const collection = dao.findCollectionByNameOrId("aws_rollouts_001");

  return dao.deleteCollection(collection);
})