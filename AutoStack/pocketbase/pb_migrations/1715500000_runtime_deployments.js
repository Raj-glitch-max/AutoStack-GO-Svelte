/// <reference path="../pb_data/types.d.ts" />
// Runtime deployment lifecycle collections.
// runtime_deployments       — current state per deployment (mutable; engine writes here)
// runtime_deployment_events — append-only event log (audit + replay trail)
//
// Auth model: per-user. listRule + viewRule limit each operator to their own
// deployments. The engine writes via admin-context Dao (no rule applies),
// so the events collection has a permissive create rule for the same reason
// other engine-written collections do (operations, incidents, etc.).
migrate((db) => {
  const dao = new Dao(db)

  // ── runtime_deployments ────────────────────────────────────────────────
  const deployments = new Collection({
    id: "rt_deployments_001",
    name: "runtime_deployments",
    type: "base",
    system: false,
    schema: [
      {
        system: false, id: "rtd_name", name: "name", type: "text", required: true,
        options: { min: 1, max: 100, pattern: "" },
      },
      {
        system: false, id: "rtd_user", name: "user", type: "relation", required: true,
        options: { collectionId: "_pb_users_auth_", cascadeDelete: false, minSelect: null, maxSelect: 1, displayFields: null },
      },
      {
        system: false, id: "rtd_image", name: "image", type: "text", required: true,
        options: { min: 1, max: 200, pattern: "" },
      },
      {
        system: false, id: "rtd_host_port", name: "host_port", type: "number", required: false,
        options: { min: 0, max: 65535, noDecimal: true },
      },
      {
        system: false, id: "rtd_cont_port", name: "container_port", type: "number", required: false,
        options: { min: 0, max: 65535, noDecimal: true },
      },
      {
        system: false, id: "rtd_state", name: "state", type: "select", required: true,
        options: {
          maxSelect: 1,
          values: [
            "planned", "validating", "provisioning", "deploying", "verifying",
            "stabilized", "certified", "contradicted",
            "rollback_pending", "rolling_back", "rolled_back",
            "failed", "destroyed",
          ],
        },
      },
      { system: false, id: "rtd_prev_img", name: "previous_image", type: "text", required: false, options: { min: null, max: 200, pattern: "" } },
      { system: false, id: "rtd_cid",      name: "container_id",   type: "text", required: false, options: { min: null, max: 100, pattern: "" } },
      { system: false, id: "rtd_cname",    name: "container_name", type: "text", required: false, options: { min: null, max: 100, pattern: "" } },
      { system: false, id: "rtd_health",   name: "health_url",     type: "url",  required: false, options: { exceptDomains: null, onlyDomains: null } },
      { system: false, id: "rtd_lerr",     name: "last_error",     type: "text", required: false, options: { min: null, max: 2000, pattern: "" } },
    ],
    indexes: [
      "CREATE INDEX IF NOT EXISTS idx_rtd_user_state ON runtime_deployments (user, state)",
    ],
    listRule:   "user = @request.auth.id",
    viewRule:   "user = @request.auth.id",
    createRule: "@request.auth.id != \"\"",
    updateRule: "user = @request.auth.id",
    deleteRule: "user = @request.auth.id",
  })
  dao.saveCollection(deployments)

  // ── runtime_deployment_events ──────────────────────────────────────────
  const events = new Collection({
    id: "rt_dep_events_001",
    name: "runtime_deployment_events",
    type: "base",
    system: false,
    schema: [
      {
        system: false, id: "rde_dep", name: "deployment", type: "relation", required: true,
        options: { collectionId: "rt_deployments_001", cascadeDelete: true, minSelect: null, maxSelect: 1, displayFields: null },
      },
      { system: false, id: "rde_seq",        name: "sequence",   type: "number", required: true,  options: { min: 1, max: null, noDecimal: true } },
      { system: false, id: "rde_from",       name: "from_state", type: "text",   required: false, options: { min: null, max: 50, pattern: "" } },
      { system: false, id: "rde_to",         name: "to_state",   type: "text",   required: false, options: { min: null, max: 50, pattern: "" } },
      { system: false, id: "rde_etype",      name: "event_type", type: "text",   required: true,  options: { min: 1, max: 100, pattern: "" } },
      { system: false, id: "rde_detail",     name: "detail",     type: "text",   required: false, options: { min: null, max: 5000, pattern: "" } },
      { system: false, id: "rde_error",      name: "error",      type: "text",   required: false, options: { min: null, max: 5000, pattern: "" } },
      { system: false, id: "rde_occurred",   name: "occurred_at",type: "date",   required: true,  options: { min: "", max: "" } },
    ],
    indexes: [
      "CREATE INDEX IF NOT EXISTS idx_rde_dep_seq ON runtime_deployment_events (deployment, sequence)",
    ],
    // The events are inspectable by the deployment owner. The engine writes
    // events via the admin Dao (rules do not apply to Dao.SaveRecord), so we
    // can keep the create rule strict.
    listRule:   "deployment.user = @request.auth.id",
    viewRule:   "deployment.user = @request.auth.id",
    createRule: null, // server-only
    updateRule: null, // append-only — events are never updated
    deleteRule: null, // append-only — events are never deleted
  })
  dao.saveCollection(events)

  return null
}, (db) => {
  const dao = new Dao(db)
  try {
    const ev = dao.findCollectionByNameOrId("runtime_deployment_events")
    dao.deleteCollection(ev)
  } catch (e) {}
  try {
    const d = dao.findCollectionByNameOrId("runtime_deployments")
    dao.deleteCollection(d)
  } catch (e) {}
  return null
})
