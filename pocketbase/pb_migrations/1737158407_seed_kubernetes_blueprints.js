migrate((db) => {
  const dao = new Dao(db);
  
  // Create blueprints collection if it doesn't exist
  let collection;
  try {
    collection = dao.findCollectionByNameOrId("blueprints");
  } catch (error) {
    // Create blueprints collection
    collection = new Collection({
      "id": "blueprints_001",
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
      "listRule": null,
      "viewRule": null,
      "createRule": null,
      "updateRule": null,
      "deleteRule": null,
      "options": {}
    });
    dao.saveCollection(collection);
  }
  
  // Create sample Kubernetes blueprints
  const nginxBlueprint = new Record(collection, {
    "id": "k8s_nginx_001",
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
                    "image": "nginx:latest",
                    "ports": [
                      {
                        "containerPort": 80
                      }
                    ]
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
    "owner": "system",
    "public": true
  });
  
  const nodeBlueprint = new Record(collection, {
    "id": "k8s_node_001", 
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
            "replicas": 3,
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
                    ]
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
    "owner": "system",
    "public": true
  });
  
  dao.saveRecord(nginxBlueprint);
  dao.saveRecord(nodeBlueprint);
  
}, (db) => {
  const dao = new Dao(db);
  
  // Delete the seeded blueprints
  const blueprintIds = ["k8s_nginx_001", "k8s_node_001"];
  
  blueprintIds.forEach(id => {
    try {
      const record = dao.findRecordById("blueprints", id);
      dao.deleteRecord(record);
    } catch (error) {
      console.log(`Blueprint ${id} not found, skipping deletion`);
    }
  });
})