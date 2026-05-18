/// <reference path="../pb_data/types.d.ts" />
// Phase 6 — runtime integrity:
//   1. UNIQUE(deployment, sequence) on runtime_deployment_events (event ordering guarantee)
//   2. runtime_observations — append-only observation log per deployment
//   3. runtime_deployment_leases — single-writer lease per deployment
//
// All three are append-only or single-writer; the reconciler is the only
// process that writes observations + lease heartbeats.
migrate((db) => {
  const dao = new Dao(db)

  // ── 1. UNIQUE(deployment, sequence) on events ──────────────────────────
  // This is enforced at the SQL level via a unique index, not via PocketBase
  // schema rules — PocketBase has no first-class composite-unique constraint.
  // Append-only collections rely on this index to refuse duplicate sequences.
  const events = dao.findCollectionByNameOrId("runtime_deployment_events")
  if (events) {
    const idxs = events.indexes || []
    const uniqueName = "ux_rde_dep_seq"
    if (!idxs.some(s => s.indexOf(uniqueName) >= 0)) {
      events.indexes = idxs.concat([
        "CREATE UNIQUE INDEX IF NOT EXISTS " + uniqueName +
        " ON runtime_deployment_events (deployment, sequence)",
      ])
      dao.saveCollection(events)
    }
  }

  // ── 2. runtime_observations ────────────────────────────────────────────
  const obs = new Collection({
    id: "rt_obs_001",
    name: "runtime_observations",
    type: "base",
    system: false,
    schema: [
      {
        system: false, id: "rto_dep", name: "deployment", type: "relation", required: true,
        options: { collectionId: "rt_deployments_001", cascadeDelete: true, minSelect: null, maxSelect: 1, displayFields: null },
      },
      // Source identifies WHO made the observation. Keeps provider observations
      // separate from operator intent and from internal engine state.
      {
        system: false, id: "rto_source", name: "source", type: "select", required: true,
        options: { maxSelect: 1, values: ["engine", "operator", "docker", "external"] },
      },
      // Trust classifies the strength of the observation. "direct" is a real
      // subprocess call result; "inferred" is a derivation; "asserted" is
      // operator-stated. The reconciler only acts on "direct".
      {
        system: false, id: "rto_trust", name: "trust", type: "select", required: true,
        options: { maxSelect: 1, values: ["direct", "inferred", "asserted"] },
      },
      // 0.0 (no confidence) to 1.0 (full confidence).
      { system: false, id: "rto_conf",     name: "confidence",   type: "number", required: true, options: { min: 0, max: 1, noDecimal: false } },
      { system: false, id: "rto_kind",     name: "kind",         type: "text",   required: true, options: { min: 1, max: 100, pattern: "" } },
      { system: false, id: "rto_evidence", name: "evidence",     type: "text",   required: false, options: { min: null, max: 8000, pattern: "" } },
      { system: false, id: "rto_observed", name: "observed_at",  type: "date",   required: true, options: { min: "", max: "" } },
    ],
    indexes: [
      "CREATE INDEX IF NOT EXISTS idx_rto_dep_observed ON runtime_observations (deployment, observed_at)",
    ],
    listRule:   "deployment.user = @request.auth.id",
    viewRule:   "deployment.user = @request.auth.id",
    createRule: null, // server-only
    updateRule: null, // append-only
    deleteRule: null, // append-only
  })
  dao.saveCollection(obs)

  // ── 3. runtime_deployment_leases ──────────────────────────────────────
  // Single-writer lease per deployment. The reconciler must hold a live
  // lease (not expired) before mutating a deployment's state.
  const leases = new Collection({
    id: "rt_dl_leases_001",
    name: "runtime_deployment_leases",
    type: "base",
    system: false,
    schema: [
      {
        system: false, id: "rtl_dep", name: "deployment", type: "relation", required: true,
        options: { collectionId: "rt_deployments_001", cascadeDelete: true, minSelect: null, maxSelect: 1, displayFields: null },
      },
      // A worker identifier. Process ID + boot time. Engines whose ID matches
      // can take over expired leases; others cannot mutate the deployment.
      { system: false, id: "rtl_worker",    name: "worker_id",     type: "text", required: true, options: { min: 1, max: 100, pattern: "" } },
      { system: false, id: "rtl_acquired",  name: "acquired_at",   type: "date", required: true, options: { min: "", max: "" } },
      { system: false, id: "rtl_heartbeat", name: "heartbeat_at",  type: "date", required: true, options: { min: "", max: "" } },
      { system: false, id: "rtl_expires",   name: "expires_at",    type: "date", required: true, options: { min: "", max: "" } },
    ],
    indexes: [
      // One live lease per deployment.
      "CREATE UNIQUE INDEX IF NOT EXISTS ux_rtl_dep ON runtime_deployment_leases (deployment)",
    ],
    listRule:   "deployment.user = @request.auth.id",
    viewRule:   "deployment.user = @request.auth.id",
    createRule: null, // server-only
    updateRule: null, // server-only
    deleteRule: null, // server-only
  })
  dao.saveCollection(leases)

  return null
}, (db) => {
  const dao = new Dao(db)
  try { dao.deleteCollection(dao.findCollectionByNameOrId("runtime_deployment_leases")) } catch (e) {}
  try { dao.deleteCollection(dao.findCollectionByNameOrId("runtime_observations")) } catch (e) {}
  return null
})
