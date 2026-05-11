/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const collection = new Collection({
    "id": "project_members_col",
    "name": "project_members",
    "type": "base",
    "system": false,
    "schema": [
      {
        "system": false,
        "id": "member_user_rel",
        "name": "user",
        "type": "relation",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "collectionId": "_pb_users_auth_",
          "cascadeDelete": true,
          "minSelect": null,
          "maxSelect": 1,
          "displayFields": null
        }
      },
      {
        "system": false,
        "id": "member_project_rel",
        "name": "project",
        "type": "relation",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "collectionId": "7kff2zw80a7rmbu",
          "cascadeDelete": true,
          "minSelect": null,
          "maxSelect": 1,
          "displayFields": null
        }
      },
      {
        "system": false,
        "id": "member_role_select",
        "name": "role",
        "type": "select",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "maxSelect": 1,
          "values": [
            "owner",
            "deployer",
            "viewer"
          ]
        }
      }
    ],
    "indexes": [
      "CREATE UNIQUE INDEX `idx_user_project` ON `project_members` (`user`, `project`)"
    ],
    "listRule": "@request.auth.id != '' && (@collection.project_members.user = @request.auth.id || @collection.project_members.project.user = @request.auth.id)",
    "viewRule": "@request.auth.id != '' && (@collection.project_members.user = @request.auth.id || @collection.project_members.project.user = @request.auth.id)",
    "createRule": null,
    "updateRule": null,
    "deleteRule": null,
    "options": {}
  });

  const dao = new Dao(db);
  dao.saveCollection(collection);

  // Seed existing projects: make the current project 'user' the 'owner'
  const projects = dao.findRecordsByFilter("projects", "1=1");
  for (const project of projects) {
    const member = new Record(collection);
    member.set("user", project.get("user"));
    member.set("project", project.id);
    member.set("role", "owner");
    dao.saveRecord(member);
  }

  return null;
}, (db) => {
  const dao = new Dao(db);
  const collection = dao.findCollectionByNameOrId("project_members_col");
  return dao.deleteCollection(collection);
})
