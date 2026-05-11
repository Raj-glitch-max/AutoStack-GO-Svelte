/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const collection = new Collection({
    "id": "gcp_credentials_col",
    "name": "gcpCredentials",
    "type": "base",
    "system": false,
    "schema": [
      {
        "id": "gcp_user",
        "name": "user",
        "type": "relation",
        "required": true,
        "options": { "collectionId": "_pb_users_auth_", "cascadeDelete": true, "maxSelect": 1 }
      },
      {
        "id": "gcp_project_id",
        "name": "projectId",
        "type": "text",
        "required": true,
        "options": { "min": 1, "max": 100 }
      },
      {
        "id": "gcp_service_account_key",
        "name": "serviceAccountKey",
        "type": "text",
        "required": true,
        "options": { "min": null, "max": null }
      },
      {
        "id": "gcp_region",
        "name": "region",
        "type": "text",
        "required": false,
        "options": { "min": null, "max": 50 }
      },
      {
        "id": "gcp_validated",
        "name": "validated",
        "type": "bool",
        "required": false,
        "options": {}
      },
      {
        "id": "gcp_last_validated",
        "name": "lastValidated",
        "type": "date",
        "required": false,
        "options": {}
      },
      {
        "id": "gcp_state_bucket",
        "name": "stateBucketName",
        "type": "text",
        "required": false,
        "options": { "min": null, "max": 200 }
      }
    ],
    "indexes": ["CREATE UNIQUE INDEX idx_gcp_creds_user ON gcpCredentials (user)"],
    "listRule": "@request.auth.id != '' && user = @request.auth.id",
    "viewRule": "@request.auth.id != '' && user = @request.auth.id",
    "createRule": "@request.auth.id != ''",
    "updateRule": "@request.auth.id != '' && user = @request.auth.id",
    "deleteRule": "@request.auth.id != '' && user = @request.auth.id"
  });
  return Dao(db).saveCollection(collection);
}, (db) => {
  const dao = new Dao(db);
  const col = dao.findCollectionByNameOrId("gcp_credentials_col");
  return dao.deleteCollection(col);
});
