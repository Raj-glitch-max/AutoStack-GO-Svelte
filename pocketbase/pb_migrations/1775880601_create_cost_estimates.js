migrate((db) => {
  const collection = new Collection({
    "id": "cost_estimates",
    "name": "costEstimates",
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
        "id": "blueprint_field",
        "name": "blueprint",
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
        "id": "region_field",
        "name": "region",
        "type": "text",
        "system": false,
        "required": true,
        "options": {
          "min": null,
          "max": null,
          "pattern": "^[a-z]{2}-[a-z]+-[0-9]$"
        }
      },
      {
        "id": "compute_monthly_field",
        "name": "computeMonthly",
        "type": "number",
        "system": false,
        "required": true,
        "options": {
          "min": 0,
          "max": null
        }
      },
      {
        "id": "networking_monthly_field",
        "name": "networkingMonthly",
        "type": "number",
        "system": false,
        "required": true,
        "options": {
          "min": 0,
          "max": null
        }
      },
      {
        "id": "storage_monthly_field",
        "name": "storageMonthly",
        "type": "number",
        "system": false,
        "required": true,
        "options": {
          "min": 0,
          "max": null
        }
      },
      {
        "id": "transfer_monthly_field",
        "name": "transferMonthly",
        "type": "number",
        "system": false,
        "required": true,
        "options": {
          "min": 0,
          "max": null
        }
      },
      {
        "id": "total_estimate_field",
        "name": "totalEstimate",
        "type": "number",
        "system": false,
        "required": true,
        "options": {
          "min": 0,
          "max": null
        }
      },
      {
        "id": "range_min_field",
        "name": "rangeMin",
        "type": "number",
        "system": false,
        "required": true,
        "options": {
          "min": 0,
          "max": null
        }
      },
      {
        "id": "range_max_field",
        "name": "rangeMax",
        "type": "number",
        "system": false,
        "required": true,
        "options": {
          "min": 0,
          "max": null
        }
      },
      {
        "id": "assumptions_field",
        "name": "assumptions",
        "type": "json",
        "system": false,
        "required": false,
        "options": {}
      },
      {
        "id": "disclaimer_field",
        "name": "disclaimer",
        "type": "text",
        "system": false,
        "required": false,
        "options": {
          "min": null,
          "max": null,
          "pattern": ""
        }
      },
      {
        "id": "pricing_version_field",
        "name": "pricingVersion",
        "type": "text",
        "system": false,
        "required": true,
        "options": {
          "min": null,
          "max": null,
          "pattern": ""
        }
      }
    ],
    "indexes": [
      "CREATE INDEX idx_cost_estimates_deployment ON costEstimates (deployment)",
      "CREATE INDEX idx_cost_estimates_blueprint ON costEstimates (blueprint)",
      "CREATE INDEX idx_cost_estimates_region ON costEstimates (region)",
      "CREATE INDEX idx_cost_estimates_created ON costEstimates (created)"
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
  const collection = dao.findCollectionByNameOrId("cost_estimates");
  return dao.deleteCollection(collection);
})