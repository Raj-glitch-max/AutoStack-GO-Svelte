/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const collection = new Collection({
    "id": "recovery_attempts",
    "created": "2026-04-11 10:00:00.000Z",
    "updated": "2026-04-11 10:00:00.000Z",
    "name": "recoveryAttempts",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "deployment",
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
        "id": "error_type",
        "name": "errorType",
        "type": "text",
        "required": true,
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
        "id": "error_category",
        "name": "errorCategory",
        "type": "text",
        "required": true,
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
        "id": "severity",
        "name": "severity",
        "type": "select",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "maxSelect": 1,
          "values": [
            "low",
            "medium",
            "high",
            "critical"
          ]
        }
      },
      {
        "system": false,
        "id": "description",
        "name": "description",
        "type": "text",
        "required": true,
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
        "id": "root_cause",
        "name": "rootCause",
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
        "id": "suggested_fix",
        "name": "suggestedFix",
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
        "id": "auto_fixable",
        "name": "autoFixable",
        "type": "bool",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {}
      },
      {
        "system": false,
        "id": "fix_applied",
        "name": "fixApplied",
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
        "id": "strategy",
        "name": "strategy",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "min": null,
          "max": 50,
          "pattern": ""
        }
      },
      {
        "system": false,
        "id": "status",
        "name": "status",
        "type": "select",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "maxSelect": 1,
          "values": [
            "pending",
            "in_progress",
            "success",
            "failed"
          ]
        }
      },
      {
        "system": false,
        "id": "attempt_number",
        "name": "attemptNumber",
        "type": "number",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "min": 1,
          "max": 10,
          "noDecimal": true
        }
      },
      {
        "system": false,
        "id": "confidence",
        "name": "confidence",
        "type": "number",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "min": 0,
          "max": 1,
          "noDecimal": false
        }
      },
      {
        "system": false,
        "id": "error_logs",
        "name": "errorLogs",
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
        "id": "completed_at",
        "name": "completedAt",
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
        "id": "duration_seconds",
        "name": "durationSeconds",
        "type": "number",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "min": 0,
          "max": null,
          "noDecimal": false
        }
      }
    ],
    "indexes": [
      "CREATE INDEX idx_recovery_deployment ON recoveryAttempts(deployment)",
      "CREATE INDEX idx_recovery_status ON recoveryAttempts(status)",
      "CREATE INDEX idx_recovery_created ON recoveryAttempts(created)"
    ],
    "listRule": "@request.auth.id != \"\" && deployment.user.id = @request.auth.id",
    "viewRule": "@request.auth.id != \"\" && deployment.user.id = @request.auth.id",
    "createRule": null,
    "updateRule": null,
    "deleteRule": null,
    "options": {}
  });

  return Dao(db).saveCollection(collection);
}, (db) => {
  const dao = new Dao(db);
  const collection = dao.findCollectionByNameOrId("recovery_attempts");

  return dao.deleteCollection(collection);
});
