/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const collection = new Collection({
    "id": "azure_credentials_col",
    "name": "azureCredentials",
    "type": "base",
    "system": false,
    "schema": [
      {
        "id": "az_user",
        "name": "user",
        "type": "relation",
        "required": true,
        "options": { "collectionId": "_pb_users_auth_", "cascadeDelete": true, "maxSelect": 1 }
      },
      {
        "id": "az_client_id",
        "name": "clientId",
        "type": "text",
        "required": true,
        "options": { "min": 1, "max": 200 }
      },
      {
        "id": "az_client_secret",
        "name": "clientSecret",
        "type": "text",
        "required": true,
        "options": { "min": 1, "max": 500 }
      },
      {
        "id": "az_tenant_id",
        "name": "tenantId",
        "type": "text",
        "required": true,
        "options": { "min": 1, "max": 200 }
      },
      {
        "id": "az_subscription_id",
        "name": "subscriptionId",
        "type": "text",
        "required": true,
        "options": { "min": 1, "max": 200 }
      },
      {
        "id": "az_region",
        "name": "region",
        "type": "text",
        "required": false,
        "options": { "min": null, "max": 50 }
      },
      {
        "id": "az_validated",
        "name": "validated",
        "type": "bool",
        "required": false,
        "options": {}
      },
      {
        "id": "az_last_validated",
        "name": "lastValidated",
        "type": "date",
        "required": false,
        "options": {}
      },
      {
        "id": "az_state_container",
        "name": "stateContainerName",
        "type": "text",
        "required": false,
        "options": { "min": null, "max": 200 }
      },
      {
        "id": "az_state_storage",
        "name": "stateStorageAccount",
        "type": "text",
        "required": false,
        "options": { "min": null, "max": 200 }
      }
    ],
    "indexes": ["CREATE UNIQUE INDEX idx_azure_creds_user ON azureCredentials (user)"],
    "listRule": "@request.auth.id != '' && user = @request.auth.id",
    "viewRule": "@request.auth.id != '' && user = @request.auth.id",
    "createRule": "@request.auth.id != ''",
    "updateRule": "@request.auth.id != '' && user = @request.auth.id",
    "deleteRule": "@request.auth.id != '' && user = @request.auth.id"
  });
  return Dao(db).saveCollection(collection);
}, (db) => {
  const dao = new Dao(db);
  const col = dao.findCollectionByNameOrId("azure_credentials_col");
  return dao.deleteCollection(col);
});
