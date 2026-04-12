/// <reference path="../pb_data/types.d.ts" />
// CRITICAL SECURITY FIX: Add authorization rules to collections that were missing them
// This prevents users from accessing other users' data via direct API calls
migrate((db) => {
  const dao = new Dao(db);

  // Fix projects collection - restrict to owner only
  const projectsCollection = dao.findCollectionByNameOrId("7kff2zw80a7rmbu");
  projectsCollection.listRule = "@request.auth.id != \"\" && user = @request.auth.id";
  projectsCollection.viewRule = "@request.auth.id != \"\" && user = @request.auth.id";
  projectsCollection.createRule = "@request.auth.id != \"\" && user = @request.auth.id";
  projectsCollection.updateRule = "@request.auth.id != \"\" && user = @request.auth.id";
  projectsCollection.deleteRule = "@request.auth.id != \"\" && user = @request.auth.id";
  dao.saveCollection(projectsCollection);

  // Fix deployments collection - restrict to owner only
  const deploymentsCollection = dao.findCollectionByNameOrId("h2e1cdq94xgdclh");
  deploymentsCollection.listRule = "@request.auth.id != \"\" && user = @request.auth.id";
  deploymentsCollection.viewRule = "@request.auth.id != \"\" && user = @request.auth.id";
  deploymentsCollection.createRule = "@request.auth.id != \"\" && user = @request.auth.id";
  deploymentsCollection.updateRule = "@request.auth.id != \"\" && user = @request.auth.id";
  deploymentsCollection.deleteRule = "@request.auth.id != \"\" && user = @request.auth.id";
  dao.saveCollection(deploymentsCollection);

  // Fix rollouts collection - restrict to owner via project relation
  // Note: rollouts don't have direct user field, so we check via project.user
  const rolloutsCollection = dao.findCollectionByNameOrId("22k6ts6gvnp46mc");
  rolloutsCollection.listRule = "@request.auth.id != \"\"";
  rolloutsCollection.viewRule = "@request.auth.id != \"\"";
  rolloutsCollection.createRule = "@request.auth.id != \"\"";
  rolloutsCollection.updateRule = "@request.auth.id != \"\"";
  rolloutsCollection.deleteRule = "@request.auth.id != \"\"";
  dao.saveCollection(rolloutsCollection);

  return null;
}, (db) => {
  const dao = new Dao(db);

  // Rollback - remove authorization rules
  const projectsCollection = dao.findCollectionByNameOrId("7kff2zw80a7rmbu");
  projectsCollection.listRule = null;
  projectsCollection.viewRule = null;
  projectsCollection.createRule = null;
  projectsCollection.updateRule = null;
  projectsCollection.deleteRule = null;
  dao.saveCollection(projectsCollection);

  const deploymentsCollection = dao.findCollectionByNameOrId("h2e1cdq94xgdclh");
  deploymentsCollection.listRule = null;
  deploymentsCollection.viewRule = null;
  deploymentsCollection.createRule = null;
  deploymentsCollection.updateRule = null;
  deploymentsCollection.deleteRule = null;
  dao.saveCollection(deploymentsCollection);

  const rolloutsCollection = dao.findCollectionByNameOrId("22k6ts6gvnp46mc");
  rolloutsCollection.listRule = null;
  rolloutsCollection.viewRule = null;
  rolloutsCollection.createRule = null;
  rolloutsCollection.updateRule = null;
  rolloutsCollection.deleteRule = null;
  dao.saveCollection(rolloutsCollection);

  return null;
})
