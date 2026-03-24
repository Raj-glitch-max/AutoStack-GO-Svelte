migrate((db) => {
  const collection = new Collection({
    "id": "aws_pricing_cache",
    "name": "awsPricingCache",
    "type": "base",
    "system": false,
    "schema": [
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
        "id": "service_field",
        "name": "service",
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
        "id": "product_family_field",
        "name": "productFamily",
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
        "id": "instance_type_field",
        "name": "instanceType",
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
        "id": "vcpu_field",
        "name": "vcpu",
        "type": "number",
        "system": false,
        "required": false,
        "options": {
          "min": 0,
          "max": null
        }
      },
      {
        "id": "memory_field",
        "name": "memory",
        "type": "number",
        "system": false,
        "required": false,
        "options": {
          "min": 0,
          "max": null
        }
      },
      {
        "id": "price_per_hour_field",
        "name": "pricePerHour",
        "type": "number",
        "system": false,
        "required": true,
        "options": {
          "min": 0,
          "max": null
        }
      },
      {
        "id": "price_per_month_field",
        "name": "pricePerMonth",
        "type": "number",
        "system": false,
        "required": true,
        "options": {
          "min": 0,
          "max": null
        }
      },
      {
        "id": "currency_field",
        "name": "currency",
        "type": "text",
        "system": false,
        "required": true,
        "options": {
          "min": null,
          "max": null,
          "pattern": "^[A-Z]{3}$"
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
      },
      {
        "id": "valid_until_field",
        "name": "validUntil",
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
      "CREATE INDEX idx_aws_pricing_region_service ON awsPricingCache (region, service)",
      "CREATE INDEX idx_aws_pricing_fetched_at ON awsPricingCache (fetchedAt)",
      "CREATE INDEX idx_aws_pricing_valid_until ON awsPricingCache (validUntil)"
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
  const collection = dao.findCollectionByNameOrId("aws_pricing_cache");
  return dao.deleteCollection(collection);
})