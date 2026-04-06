/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const collection = new Collection({
    "id": "terraform_executions_001",
    "created": "2026-03-17 12:00:04.000Z",
    "updated": "2026-03-17 12:00:04.000Z",
    "name": "terraformExecutions",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "tf_exec_rollout",
        "name": "rollout",
        "type": "relation",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "collectionId": "awsRollouts",
          "cascadeDelete": true,
          "minSelect": null,
          "maxSelect": 1,
          "displayFields": null
        }
      },
      {
        "system": false,
        "id": "tf_exec_operation",
        "name": "operation",
        "type": "select",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "maxSelect": 1,
          "values": [
            "init",
            "plan",
            "apply",
            "destroy",
            "validate"
          ]
        }
      },
      {
        "system": false,
        "id": "tf_exec_status",
        "name": "status",
        "type": "select",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "maxSelect": 1,
          "values": [
            "running",
            "completed",
            "failed",
            "cancelled"
          ]
        }
      },
      {
        "system": false,
        "id": "tf_exec_output",
        "name": "output",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "min": null,
          "max": 100000,
          "pattern": ""
        }
      },
      {
        "system": false,
        "id": "tf_exec_exit_code",
        "name": "exitCode",
        "type": "number",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "min": null,
          "max": null,
          "noDecimal": true
        }
      },
      {
        "system": false,
        "id": "tf_exec_start_time",
        "name": "startTime",
        "type": "date",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "min": "",
          "max": ""
        }
      },
      {
        "system": false,
        "id": "tf_exec_end_time",
        "name": "endTime",
        "type": "date",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "min": "",
          "max": ""
        }
      }
    ],
    "indexes": [
      "CREATE INDEX idx_terraform_executions_rollout ON terraformExecutions (rollout)",
      "CREATE INDEX idx_terraform_executions_operation ON terraformExecutions (operation)",
      "CREATE INDEX idx_terraform_executions_status ON terraformExecutions (status)",
      "CREATE INDEX idx_terraform_executions_start_time ON terraformExecutions (startTime)"
    ],
    "listRule": "@request.auth.id != \"\" && rollout.user = @request.auth.id",
    "viewRule": "@request.auth.id != \"\" && rollout.user = @request.auth.id",
    "createRule": "@request.auth.id != \"\" && rollout.user = @request.auth.id",
    "updateRule": "@request.auth.id != \"\" && rollout.user = @request.auth.id",
    "deleteRule": "@request.auth.id != \"\" && rollout.user = @request.auth.id",
    "options": {}
  });

  return Dao(db).saveCollection(collection);
}, (db) => {
  const dao = new Dao(db);
  const collection = dao.findCollectionByNameOrId("terraform_executions_001");

  return dao.deleteCollection(collection);
})