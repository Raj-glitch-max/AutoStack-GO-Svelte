migrate((db) => {
  const dao = new Dao(db);
  
  // Create a system user for blueprints
  let systemUser;
  try {
    systemUser = dao.findFirstRecordByData("users", "email", "system@autostack.local");
  } catch (error) {
    // Create system user
    const usersCollection = dao.findCollectionByNameOrId("users");
    systemUser = new Record(usersCollection, {
      "id": "system_user_001",
      "email": "system@autostack.local",
      "username": "system",
      "name": "System",
      "verified": true,
      "emailVisibility": false
    });
    // Set a dummy password
    systemUser.setPassword("system123");
    dao.saveRecord(systemUser);
  }
  
  // Create or update blueprints collection
  let collection;
  try {
    collection = dao.findCollectionByNameOrId("blueprints");
  } catch (error) {
    // Create blueprints collection
    collection = new Collection({
      "id": "blueprints_collection",
      "name": "blueprints",
      "type": "base",
      "system": false,
      "schema": [
        {
          "id": "name_field",
          "name": "name",
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
          "id": "description_field", 
          "name": "description",
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
          "id": "manifest_field",
          "name": "manifest",
          "type": "json",
          "system": false,
          "required": true,
          "options": {}
        },
        {
          "id": "owner_field",
          "name": "owner",
          "type": "relation",
          "system": false,
          "required": true,
          "options": {
            "collectionId": "_pb_users_auth_",
            "cascadeDelete": false,
            "minSelect": null,
            "maxSelect": 1,
            "displayFields": ["name", "username"]
          }
        },
        {
          "id": "public_field",
          "name": "public",
          "type": "bool",
          "system": false,
          "required": false,
          "options": {}
        },
        {
          "id": "logo_field",
          "name": "logo",
          "type": "file",
          "system": false,
          "required": false,
          "options": {
            "maxSelect": 1,
            "maxSize": 5242880,
            "mimeTypes": [
              "image/jpeg",
              "image/png",
              "image/svg+xml",
              "image/gif",
              "image/webp"
            ],
            "thumbs": null,
            "protected": false
          }
        }
      ],
      "indexes": [],
      "listRule": "@request.auth.id != '' || public = true",
      "viewRule": "@request.auth.id != '' || public = true", 
      "createRule": "@request.auth.id != ''",
      "updateRule": "@request.auth.id != '' && owner = @request.auth.id",
      "deleteRule": "@request.auth.id != '' && owner = @request.auth.id",
      "options": {}
    });
    dao.saveCollection(collection);
  }
  
  // Create sample Kubernetes blueprints with system user as owner
  const nginxBlueprint = new Record(collection, {
    "id": "k8s_nginx_system",
    "name": "NGINX Web Server",
    "description": "Simple NGINX web server deployment with service and ingress",
    "manifest": {
      "apiVersion": "v1",
      "kind": "List",
      "items": [
        {
          "apiVersion": "apps/v1",
          "kind": "Deployment",
          "metadata": {
            "name": "nginx-deployment",
            "labels": {
              "app": "nginx"
            }
          },
          "spec": {
            "replicas": 2,
            "selector": {
              "matchLabels": {
                "app": "nginx"
              }
            },
            "template": {
              "metadata": {
                "labels": {
                  "app": "nginx"
                }
              },
              "spec": {
                "containers": [
                  {
                    "name": "nginx",
                    "image": "nginx:1.21-alpine",
                    "ports": [
                      {
                        "containerPort": 80
                      }
                    ],
                    "resources": {
                      "requests": {
                        "memory": "64Mi",
                        "cpu": "50m"
                      },
                      "limits": {
                        "memory": "128Mi", 
                        "cpu": "100m"
                      }
                    }
                  }
                ]
              }
            }
          }
        },
        {
          "apiVersion": "v1",
          "kind": "Service",
          "metadata": {
            "name": "nginx-service"
          },
          "spec": {
            "selector": {
              "app": "nginx"
            },
            "ports": [
              {
                "protocol": "TCP",
                "port": 80,
                "targetPort": 80
              }
            ],
            "type": "ClusterIP"
          }
        }
      ]
    },
    "owner": systemUser.id,
    "public": true
  });
  
  const nodeBlueprint = new Record(collection, {
    "id": "k8s_node_system", 
    "name": "Node.js Application",
    "description": "Node.js application with deployment, service, and configmap",
    "manifest": {
      "apiVersion": "v1",
      "kind": "List",
      "items": [
        {
          "apiVersion": "v1",
          "kind": "ConfigMap",
          "metadata": {
            "name": "app-config"
          },
          "data": {
            "NODE_ENV": "production",
            "PORT": "3000"
          }
        },
        {
          "apiVersion": "apps/v1",
          "kind": "Deployment",
          "metadata": {
            "name": "nodejs-app",
            "labels": {
              "app": "nodejs"
            }
          },
          "spec": {
            "replicas": 2,
            "selector": {
              "matchLabels": {
                "app": "nodejs"
              }
            },
            "template": {
              "metadata": {
                "labels": {
                  "app": "nodejs"
                }
              },
              "spec": {
                "containers": [
                  {
                    "name": "nodejs",
                    "image": "node:18-alpine",
                    "ports": [
                      {
                        "containerPort": 3000
                      }
                    ],
                    "envFrom": [
                      {
                        "configMapRef": {
                          "name": "app-config"
                        }
                      }
                    ],
                    "resources": {
                      "requests": {
                        "memory": "128Mi",
                        "cpu": "100m"
                      },
                      "limits": {
                        "memory": "256Mi",
                        "cpu": "200m"
                      }
                    }
                  }
                ]
              }
            }
          }
        },
        {
          "apiVersion": "v1",
          "kind": "Service",
          "metadata": {
            "name": "nodejs-service"
          },
          "spec": {
            "selector": {
              "app": "nodejs"
            },
            "ports": [
              {
                "protocol": "TCP",
                "port": 80,
                "targetPort": 3000
              }
            ],
            "type": "LoadBalancer"
          }
        }
      ]
    },
    "owner": systemUser.id,
    "public": true
  });
  
  dao.saveRecord(nginxBlueprint);
  dao.saveRecord(nodeBlueprint);
  
}, (db) => {
  const dao = new Dao(db);
  
  // Delete the seeded blueprints
  const blueprintIds = ["k8s_nginx_system", "k8s_node_system"];
  
  blueprintIds.forEach(id => {
    try {
      const record = dao.findRecordById("blueprints", id);
      dao.deleteRecord(record);
    } catch (error) {
      console.log(`Blueprint ${id} not found, skipping deletion`);
    }
  });
  
  // Delete system user
  try {
    const systemUser = dao.findRecordById("users", "system_user_001");
    dao.deleteRecord(systemUser);
  } catch (error) {
    console.log("System user not found, skipping deletion");
  }
})