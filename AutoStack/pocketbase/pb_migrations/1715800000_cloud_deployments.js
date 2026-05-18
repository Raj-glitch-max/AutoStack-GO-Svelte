/// <reference path="../pb_data/types.d.ts" />
// Phase PX-2: cloud_deployments — minimal collection backing the cloud
// deploy endpoint.  One row per user-triggered cloud deployment; the
// pkg/providers code is real (uses aws-sdk-go-v2 + cloud.google.com/go),
// but provisioning is only as real as the credentials supplied to the
// owning cloud_accounts row.
//
// Status values mirror the provider.TargetStatus contract:
//   pending → provisioning → running | error → updating | rolling_back
//   → rolled_back | destroying → destroyed
migrate((db) => {
  const dao = new Dao(db)

  const cd = new Collection({
    id: "cloud_dep_001",
    name: "cloud_deployments",
    type: "base",
    system: false,
    schema: [
      { system: false, id: "cd_name",     name: "name",     type: "text",     required: true,  options: { min: 1, max: 100, pattern: "" } },
      { system: false, id: "cd_user",     name: "user",     type: "relation", required: true,  options: { collectionId: "_pb_users_auth_", cascadeDelete: false, minSelect: null, maxSelect: 1, displayFields: null } },
      { system: false, id: "cd_account",  name: "account",  type: "relation", required: true,  options: { collectionId: "7yu2gjzlb5wp4xa", cascadeDelete: false, minSelect: null, maxSelect: 1, displayFields: null } },
      { system: false, id: "cd_provider", name: "provider", type: "text",     required: true,  options: { min: 1, max: 32, pattern: "" } },
      { system: false, id: "cd_region",   name: "region",   type: "text",     required: false, options: { min: null, max: 32, pattern: "" } },
      { system: false, id: "cd_image",    name: "image",    type: "text",     required: true,  options: { min: 1, max: 200, pattern: "" } },
      { system: false, id: "cd_cpu",      name: "cpu_vcpu", type: "number",   required: false, options: { min: 0, max: 64, noDecimal: false } },
      { system: false, id: "cd_mem",      name: "memory_mb",type: "number",   required: false, options: { min: 0, max: null, noDecimal: true } },
      { system: false, id: "cd_replicas", name: "replicas", type: "number",   required: false, options: { min: 0, max: null, noDecimal: true } },
      { system: false, id: "cd_status",   name: "status",   type: "text",     required: true,  options: { min: 1, max: 32, pattern: "" } },
      { system: false, id: "cd_external", name: "external_id",  type: "text", required: false, options: { min: null, max: 500, pattern: "" } },
      { system: false, id: "cd_endpoint", name: "endpoint_url", type: "text", required: false, options: { min: null, max: 500, pattern: "" } },
      { system: false, id: "cd_opid",     name: "operation_id", type: "text", required: false, options: { min: null, max: 200, pattern: "" } },
      { system: false, id: "cd_lerr",     name: "last_error",   type: "text", required: false, options: { min: null, max: 2000, pattern: "" } },
      { system: false, id: "cd_deployed", name: "deployed_at",  type: "text", required: false, options: { min: null, max: 64, pattern: "" } },
    ],
    indexes: [
      "CREATE INDEX IF NOT EXISTS idx_cd_user_account ON cloud_deployments (user, account)",
    ],
    // The cloud_accounts collection id from the existing schema (the migration
    // 1715400000 didn't change cloud_accounts; it predates this work).  The
    // hardcoded id above (7yu2gjzlb5wp4xa) is the live id seen via the admin API.
    // If that id doesn't match in your env, run `pocketbase admin` and read
    // /api/collections/cloud_accounts to get the actual id, then update.
    listRule:   "user = @request.auth.id",
    viewRule:   "user = @request.auth.id",
    createRule: null, // server-only writes
    updateRule: null, // server-only writes
    deleteRule: "user = @request.auth.id",
  })

  // Look up cloud_accounts to get the real id rather than hardcode.
  const accounts = dao.findCollectionByNameOrId("cloud_accounts")
  if (accounts) {
    cd.schema.getFieldById("cd_account").options.collectionId = accounts.id
  }

  dao.saveCollection(cd)

  return null
}, (db) => {
  const dao = new Dao(db)
  try { dao.deleteCollection(dao.findCollectionByNameOrId("cloud_deployments")) } catch (e) {}
  return null
})
