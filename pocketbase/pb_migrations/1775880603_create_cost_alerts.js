migrate((db) => {
  const collection = new Collection({
    "id": "cost_alerts",
    "name": "costAlerts",
    "type": "base",
    "system": false,
    "schema": [
      {
        "id": "deployment_field",
        "name": "deployment",
        "type": "relation",
        "system": false,
        "required": true,
        "options": {
          "collectionId": "awsDeployments",
          "cascadeDelete": true,
          "minSelect": null,
          "maxSelect": 1,
          "displayFields": ["name"]
        }
      },
      {
        "id": "user_field",
        "name": "user",
        "type": "relation",
        "system": false,
        "required": true,
        "options": {
          "collectionId": "_pb_users_auth_",
          "cascadeDelete": true,
          "minSelect": null,
          "maxSelect": 1,
          "displayFields": ["name", "email"]
        }
      },
      {
        "id": "type_field",
        "name": "type",
        "type": "select",
        "system": false,
        "required": true,
        "options": {
          "maxSelect": 1,
          "values": [
            "cost_overrun",
            "budget_exceeded",
            "anomaly_detected"
          ]
        }
      },
      {
        "id": "threshold_field",
        "name": "threshold",
        "type": "number",
        "system": false,
        "required": true,
        "options": {
          "min": 0,
          "max": null
        }
      },
      {
        "id": "triggered_field",
        "name": "triggered",
        "type": "bool",
        "system": false,
        "required": true,
        "options": {}
      },
      {
        "id": "actual_cost_field",
        "name": "actualCost",
        "type": "number",
        "system": false,
        "required": true,
        "options": {
          "min": 0,
          "max": null
        }
      },
      {
        "id": "estimated_cost_field",
        "name": "estimatedCost",
        "type": "number",
        "system": false,
        "required": true,
        "options": {
          "min": 0,
          "max": null
        }
      },
      {
        "id": "variance_field",
        "name": "variance",
        "type": "number",
        "system": false,
        "required": true,
        "options": {
          "min": null,
          "max": null
        }
      },
      {
        "id": "message_field",
        "name": "message",
        "type": "text",
        "system": false,
        "required": true,
        "options": {
          "min": null,
          "max": null,
          "pattern": ""
        }
      },
      {
        "id": "sent_at_field",
        "name": "sentAt",
        "type": "date",
        "system": false,
        "required": false,
        "options": {
          "min": "",
          "max": ""
        }
      },
      {
        "id": "acknowledged_field",
        "name": "acknowledged",
        "type": "bool",
        "system": false,
        "required": true,
        "options": {}
      },
      {
        "id": "acknowledged_at_field",
        "name": "acknowledgedAt",
        "type": "date",
        "system": false,
        "required": false,
        "options": {
          "min": "",
          "max": ""
        }
      }
    ],
    "indexes": [
      "CREATE INDEX idx_cost_alerts_deployment ON costAlerts (deployment)",
      "CREATE INDEX idx_cost_alerts_user ON costAlerts (user)",
      "CREATE INDEX idx_cost_alerts_type ON costAlerts (type)",
      "CREATE INDEX idx_cost_alerts_triggered ON costAlerts (triggered)",
      "CREATE INDEX idx_cost_alerts_acknowledged ON costAlerts (acknowledged)",
      "CREATE INDEX idx_cost_alerts_sent_at ON costAlerts (sentAt)"
    ],
    "listRule": "@request.auth.id != '' && user = @request.auth.id",
    "viewRule": "@request.auth.id != '' && user = @request.auth.id",
    "createRule": "@request.auth.id != ''",
    "updateRule": "@request.auth.id != '' && user = @request.auth.id",
    "deleteRule": "@request.auth.id != '' && user = @request.auth.id",
    "options": {}
  });

  return Dao(db).saveCollection(collection);
}, (db) => {
  const dao = new Dao(db);
  const collection = dao.findCollectionByNameOrId("cost_alerts");
  return dao.deleteCollection(collection);
})