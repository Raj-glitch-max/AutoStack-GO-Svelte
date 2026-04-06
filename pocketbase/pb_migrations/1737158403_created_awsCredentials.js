/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const collection = new Collection({
    "id": "aws_credentials_001",
    "created": "2026-03-17 12:00:03.000Z",
    "updated": "2026-03-17 12:00:03.000Z",
    "name": "awsCredentials",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "aws_cred_user",
        "name": "user",
        "type": "relation",
        "required": true,
        "presentable": false,
        "unique": true,
        "options": {
          "collectionId": "_pb_users_auth_",
          "cascadeDelete": true,
          "minSelect": null,
          "maxSelect": 1,
          "displayFields": ["email"]
        }
      },
      {
        "system": false,
        "id": "aws_cred_access_key",
        "name": "accessKeyId",
        "type": "text",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "min": 16,
          "max": 128,
          "pattern": ""
        }
      },
      {
        "system": false,
        "id": "aws_cred_secret_key",
        "name": "secretAccessKey",
        "type": "text",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "min": 32,
          "max": 256,
          "pattern": ""
        }
      },
      {
        "system": false,
        "id": "aws_cred_session_token",
        "name": "sessionToken",
        "type": "text",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {
          "min": null,
          "max": 2048,
          "pattern": ""
        }
      },
      {
        "system": false,
        "id": "aws_cred_region",
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
        "id": "aws_cred_validated",
        "name": "validated",
        "type": "bool",
        "required": false,
        "presentable": false,
        "unique": false,
        "options": {}
      },
      {
        "system": false,
        "id": "aws_cred_last_validated",
        "name": "lastValidated",
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
      "CREATE UNIQUE INDEX idx_aws_credentials_user ON awsCredentials (user)",
      "CREATE INDEX idx_aws_credentials_validated ON awsCredentials (validated)"
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
  const collection = dao.findCollectionByNameOrId("aws_credentials_001");

  return dao.deleteCollection(collection);
})