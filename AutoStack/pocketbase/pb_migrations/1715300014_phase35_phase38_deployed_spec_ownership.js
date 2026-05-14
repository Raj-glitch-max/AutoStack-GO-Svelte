/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)
  const operations = dao.findCollectionByNameOrId("operations")

  // Phase 3.5 + Phase 3.8 — Operational Maturity + Ownership Fencing Foundations.
  //
  // Two columns added to `operations`:
  //   1. deployed_spec (json)  — Phase 3.5 full normalized spec snapshot at deploy time
  //   2. owned_by_pod (text)   — Phase 3.8 SC-5 ownership identity stamp
  //
  // Both are ADDITIVE. Phase 3.0–3.4 code that ignores them continues to work.

  // ── Phase 3.5: deployed_spec JSON snapshot ───────────────────────────────
  //
  // The full normalized DeploySpec serialized at the moment the deploy
  // dispatcher hands off to Provider.Deploy(). This is the immutable
  // deploy-time truth baseline.
  //
  // Phase 3.4 introduced last_deployed_revision (rollout.updated_at) on
  // deployment_targets — enough to detect REVISION drift. But revision
  // alone cannot distinguish "spec updated and re-deployed" from "same
  // revision, different image tag" (which can happen if a manifest is
  // mutated in-place without bumping updated_at).
  //
  // deployed_spec captures: image, compute, scale, network, env, secrets,
  // health, target_config. Compared against the current rollout manifest
  // for content drift (Phase 3.5+).
  //
  // Null for pre-3.5 operations and destroy/rollback ops (which do not
  // carry a fresh spec — they reuse the prior deployed_spec).
  //
  // Storage: typically <8KB per row. Retention policy follows the
  // operations table (no special handling).
  operations.schema.addField(new SchemaField({
    "system": false,
    "id": "ops_deployed_spec",
    "name": "deployed_spec",
    "type": "json",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {}
  }))

  // ── Phase 3.8: owned_by_pod ownership stamp ──────────────────────────────
  //
  // The reconciler pod's identity at the moment this operation was
  // claimed. Generated once per process at Reconciler.Start() (UUID).
  //
  // Purpose: foundation for SC-5 multi-pod ownership fencing. The current
  // sweep cannot distinguish a peer pod's live operation from an abandoned
  // one. This column makes it OBSERVABLE that an operation belongs to a
  // specific pod, even before full multi-pod safety is implemented.
  //
  // CRITICAL: writing this column does NOT make multi-pod deployment safe.
  // Full multi-pod safety requires:
  //   - lease-based reconciler election (out of scope; would require Raft)
  //   - sweep that defers to pod identity rather than heartbeat age alone
  //   - claimTarget CAS that excludes operations owned by live peer pods
  //
  // Phase 3.8 deliverable: identity stamp + forensic visibility.
  // Phase 4+ deliverable: full multi-pod ownership fencing.
  //
  // Single-pod behavior (current): every operation carries the stamp;
  // sweep logs the foreign pod_id when reclaiming (forensic clarity).
  operations.schema.addField(new SchemaField({
    "system": false,
    "id": "ops_owned_by_pod",
    "name": "owned_by_pod",
    "type": "text",
    "required": false,
    "presentable": false,
    "unique": false,
    "options": {
      "min": null,
      "max": 64,
      "pattern": ""
    }
  }))

  return dao.saveCollection(operations)
}, (db) => {
  const dao = new Dao(db)
  const operations = dao.findCollectionByNameOrId("operations")
  operations.schema.removeField("ops_deployed_spec")
  operations.schema.removeField("ops_owned_by_pod")
  return dao.saveCollection(operations)
})
