/// <reference path="../pb_data/types.d.ts" />
migrate((db) => {
  const dao = new Dao(db)

  // Phase 3.9.2 — Real Multi-Pod Lease Coordination (SC-5).
  //
  // Creates the authoritative lease table that converts owned_by_pod from
  // a FORENSIC WITNESS (Phase 3.8) into a real DISTRIBUTED COORDINATION
  // FENCE. Before any provider mutation, the reconciler MUST hold a
  // valid lease for the target. Lease ownership is enforced via
  // monotonic-epoch CAS at the DB layer.
  //
  // Operational scope: this table protects against TWO reconciler pods
  // accidentally running simultaneously. It does NOT make AutoStack a
  // distributed orchestrator. The SQLite WAL is the underlying
  // serialization mechanism; multi-region / multi-node DB clustering is
  // NOT in scope.
  //
  // Honest limitations:
  //   - SQLite is the serialization point. A network partition between
  //     two pods sharing the same DB file is impossible by construction
  //     (file is local). When the DB is remote (NFS, EFS), file-locking
  //     semantics dictate correctness; this is the operator's deployment
  //     concern, NOT this lease layer.
  //   - Clock skew between pods is mitigated by DB-side expiration
  //     comparisons (the DB clock is the authority). Pod-local clocks
  //     are only used to compute lease_expires_at when acquiring.
  //   - A pod whose lease expired due to a heartbeat lag MUST detect the
  //     loss BEFORE committing any further provider mutation; this is
  //     the responsibility of the lease verification call before each
  //     provider boundary, not of this schema.

  const collection = new Collection({
    "id": "reconciler_leases",
    "name": "reconciler_leases",
    "type": "base",
    "system": false,
    "schema": [
      {
        // lease_key — the resource that this lease coordinates. For
        // per-target execution leases, this is the deployment_targets.id.
        // For future global reconcile leases, this would be "__global__".
        //
        // Unique constraint enforces "at most one active lease per key".
        // CAS acquisition relies on this uniqueness.
        "system": false,
        "id": "rl_lease_key",
        "name": "lease_key",
        "type": "text",
        "required": true,
        "presentable": true,
        "unique": false,
        "options": {
          "min": 1,
          "max": 64,
          "pattern": ""
        }
      },
      {
        // holder_pod_id — the PodIdentity string of the reconciler pod
        // currently holding the lease. Format: pod-<8hex>-<unix-seconds>
        // (see pkg/reconciler/ownership.go).
        "system": false,
        "id": "rl_holder_pod_id",
        "name": "holder_pod_id",
        "type": "text",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "min": 1,
          "max": 64,
          "pattern": ""
        }
      },
      {
        // lease_epoch — monotonically increasing on every acquisition.
        // Refresh and Release require the caller to pass the epoch they
        // believe they hold. If the DB row's epoch differs, the CAS fails
        // — this is how stale (post-eviction) pods are made harmless.
        //
        // Monotonicity is enforced by the AcquireLease SQL: each new
        // acquisition increments the existing row's epoch by 1 (or
        // inserts with epoch=1 for first-ever acquisition).
        "system": false,
        "id": "rl_lease_epoch",
        "name": "lease_epoch",
        "type": "number",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "min": 1,
          "max": null,
          "noDecimal": true
        }
      },
      {
        // lease_expires_at — absolute UTC time at which the lease becomes
        // eligible for stealing. Refreshed on every heartbeat.
        //
        // CAS comparison: AcquireLease only succeeds if the existing row
        // shows expires_at <= NOW, OR the caller already holds the lease.
        // This is the core multi-pod safety check.
        "system": false,
        "id": "rl_lease_expires_at",
        "name": "lease_expires_at",
        "type": "date",
        "required": true,
        "presentable": false,
        "unique": false,
        "options": {
          "min": "",
          "max": ""
        }
      },
      {
        // heartbeat_at — last refresh time. Distinct from expires_at so
        // operators can detect "lease still valid but heartbeat stale"
        // (indicates the holder pod is sluggish but not yet evicted).
        "system": false,
        "id": "rl_heartbeat_at",
        "name": "heartbeat_at",
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
      // Unique per-key constraint — enforces at-most-one active lease.
      "CREATE UNIQUE INDEX idx_reconciler_leases_key ON reconciler_leases (lease_key)",
      // Expiration sweep index.
      "CREATE INDEX idx_reconciler_leases_expires ON reconciler_leases (lease_expires_at)"
    ],
    "listRule": null,
    "viewRule": null,
    "createRule": null,
    "updateRule": null,
    "deleteRule": null,
    "options": {}
  })

  return Dao(db).saveCollection(collection)
}, (db) => {
  const dao = new Dao(db)
  const collection = dao.findCollectionByNameOrId("reconciler_leases")
  return dao.deleteCollection(collection)
})
