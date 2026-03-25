migrate((db) => {
  const collection = new Collection({
    "id": "actual_costs",
    "name": "actualCosts",
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
        "id": "cost_to_date_field",
        "name": "costToDate",
        "type": "number",
        "system": false,
        "required": true,
        "options": {
          "min": 0,
          "max": null
        }
      },
      {
        "id": "projected_monthly_field",
        "name": "projectedMonthly",
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
        "id": "breakdown_field",
        "name": "breakdown",
        "type": "json",
        "system": false,
        "required": true,
        "options": {}
      },
      {
        "id": "period_start_field",
        "name": "periodStart",
        "type": "date",
        "system": false,
        "required": true,
        "options": {
          "min": "",
          "max": ""
        }
      },
      {
        "id": "period_end_field",
        "name": "periodEnd",
        "type": "date",
        "system": false,
        "required": true,
        "options": {
          "min": "",
          "max": ""
        }
      },
      {
        "id": "fetched_at_field",
        "name": "fetchedAt",
        "type": "date",
        "system": false,
        "required": true,
        "options": {
          "min": "",
          "max": ""
        }
      }
    ],
    "indexes": [
      "CREATE INDEX idx_actual_costs_deployment ON actualCosts (deployment)",
      "CREATE INDEX idx_actual_costs_period ON actualCosts (periodStart, periodEnd)",
      "CREATE INDEX idx_actual_costs_fetched ON actualCosts (fetchedAt)",
      "CREATE UNIQUE INDEX idx_actual_costs_deployment_period ON actualCosts (deployment, periodStart, periodEnd)"
    ],
    "listRule": null,
    "viewRule": null,
    "createRule": null,
    "updateRule": null,
    "deleteRule": null,
    "options": {}
  });

  return Dao(db).saveCollection(collection);
}, (db) => {
  const dao = new Dao(db);
  const collection = dao.findCollectionByNameOrId("actual_costs");
  return dao.deleteCollection(collection);
})