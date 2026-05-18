package reconciler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"sort"
	"time"

	"github.com/pocketbase/dbx"
	pb "github.com/pocketbase/pocketbase"
)

// ForensicSchemaVersion identifies the shape of ForensicSnapshot JSON.
// Increment when fields are added or removed so consumers can detect
// format changes without parsing failures.
const ForensicSchemaVersion = "3.9.10"

// Phase 3.9.5 — Forensic snapshot export + metric rehydration.
//
// This file implements two distinct concerns that share a common theme:
// making the runtime trustworthy AFTER restart.
//
//   1. RehydrateMetrics — read persistent row counts and seed in-memory
//      counters so post-restart metric snapshots do not under-report.
//   2. ForensicSnapshot — deterministic JSON export of all persisted
//      state about a target, suitable for offline operator inspection
//      and incident archives.

// ─────────────────────────────────────────────────────────────────────────
// METRIC REHYDRATION
// ─────────────────────────────────────────────────────────────────────────

// RehydrateMetrics seeds the in-memory atomic counters from the DB so
// post-restart snapshots reflect persistent truth.
//
// Honest scope:
//   - Cumulative counters that map to DB row counts CAN be hydrated:
//     incidents_opened_by_source = COUNT(incidents) GROUP BY source
//     incident_transitions_by_status = best-effort approximation via
//     COUNT(incidents WHERE status=X) — see "approximation" note below.
//   - Counters that are per-process by definition CANNOT be hydrated:
//     reconcile_cycles_total, targets_reconciled, deploys_dispatched,
//     all the *Hold counters. These remain zero post-restart, which is
//     CORRECT — they measure runtime activity since boot.
//
// Approximation note for incident_transitions_by_status: the canonical
// counter measures cumulative transitions per status. Hydrating it from
// the DB underestimates: a single incident that went open → escalating
// → acknowledged → resolved contributes 4 transitions in lifetime but
// only its CURRENT status (resolved) is observable in the row. We choose
// to seed each status counter with COUNT(incidents WHERE status=status)
// — the absolute minimum guaranteed by the DB. The truthfulness state
// records this approximation explicitly.
func (r *Reconciler) RehydrateMetrics() {
	if r == nil || r.metrics == nil {
		return
	}
	app := r.app

	// 1. incidents_opened_by_source — exact COUNT GROUP BY.
	var openedRows []struct {
		Source string `db:"source"`
		Count  int64  `db:"cnt"`
	}
	err := app.Dao().DB().NewQuery(
		`SELECT source AS source, COUNT(*) AS cnt FROM incidents GROUP BY source`,
	).All(&openedRows)
	if err != nil {
		log.Printf("[METRIC_REHYDRATE_ERR] incidents_opened_by_source: %v", err)
		SetMetricTruthfulnessState(MetricTruthfulnessPartial)
		return
	}
	r.metrics.incidentOpenedMu.Lock()
	if r.metrics.incidentOpenedBySource == nil {
		r.metrics.incidentOpenedBySource = make(map[string]int64)
	}
	for _, row := range openedRows {
		r.metrics.incidentOpenedBySource[row.Source] = row.Count
	}
	r.metrics.incidentOpenedMu.Unlock()

	// 2. incident_transitions_by_status — approximation via current-status count.
	// See "Approximation note" above. We seed with floor(current_count).
	var transRows []struct {
		Status string `db:"status"`
		Count  int64  `db:"cnt"`
	}
	err = app.Dao().DB().NewQuery(
		`SELECT status AS status, COUNT(*) AS cnt FROM incidents GROUP BY status`,
	).All(&transRows)
	if err != nil {
		log.Printf("[METRIC_REHYDRATE_ERR] incident_transitions_by_status: %v", err)
		SetMetricTruthfulnessState(MetricTruthfulnessPartial)
		return
	}
	r.metrics.incidentTransitionsMu.Lock()
	if r.metrics.incidentTransitions == nil {
		r.metrics.incidentTransitions = make(map[string]int64)
	}
	for _, row := range transRows {
		r.metrics.incidentTransitions[row.Status] = row.Count
	}
	r.metrics.incidentTransitionsMu.Unlock()

	// 3. incident_escalations_total — approximation via SUM(escalation_count).
	var escTotal struct {
		Total int64 `db:"total"`
	}
	err = app.Dao().DB().NewQuery(
		`SELECT COALESCE(SUM(escalation_count), 0) AS total FROM incidents`,
	).One(&escTotal)
	if err != nil {
		log.Printf("[METRIC_REHYDRATE_ERR] incident_escalations_total: %v", err)
		SetMetricTruthfulnessState(MetricTruthfulnessPartial)
		return
	}
	r.metrics.incidentEscalations.Store(escTotal.Total)

	// 4. provider_observations_by_trust — Phase 3.9.8 rehydration.
	// Exact COUNT GROUP BY observation_trust. Limited to the 5 canonical trust
	// levels; unknown values are skipped (DB should never contain them).
	var obsByTrustRows []struct {
		Trust string `db:"trust"`
		Count int64  `db:"cnt"`
	}
	obsErr := app.Dao().DB().NewQuery(
		`SELECT observation_trust AS trust, COUNT(*) AS cnt FROM provider_observations GROUP BY observation_trust`,
	).All(&obsByTrustRows)
	if obsErr != nil {
		log.Printf("[METRIC_REHYDRATE_ERR] provider_observations_by_trust: %v", obsErr)
		// Non-fatal: continue with partial rehydration.
		SetMetricTruthfulnessState(MetricTruthfulnessPartial)
	} else {
		r.metrics.providerObsMu.Lock()
		if r.metrics.providerObsByTrust == nil {
			r.metrics.providerObsByTrust = make(map[string]int64)
		}
		for _, row := range obsByTrustRows {
			r.metrics.providerObsByTrust[row.Trust] = row.Count
		}
		r.metrics.providerObsMu.Unlock()

		// 5. provider_contradictions_total — exact count.
		var contradictionTotal struct {
			Total int64 `db:"total"`
		}
		contErr := app.Dao().DB().NewQuery(
			`SELECT COUNT(*) AS total FROM provider_observations WHERE contradiction_detected = 1`,
		).One(&contradictionTotal)
		if contErr != nil {
			log.Printf("[METRIC_REHYDRATE_ERR] provider_contradictions_total: %v", contErr)
			SetMetricTruthfulnessState(MetricTruthfulnessPartial)
		} else {
			r.metrics.providerContradictions.Store(contradictionTotal.Total)

			// 6. provider_open_drifts_count — Phase 3.9.9 point-in-time count.
			// Not a monotonic counter; reflects DB truth at startup only.
			var openDriftTotal struct {
				Total int64 `db:"total"`
			}
			driftErr := app.Dao().DB().NewQuery(
				`SELECT COUNT(*) AS total FROM provider_drifts
				 WHERE (resolved_at IS NULL OR resolved_at = '')`,
			).One(&openDriftTotal)
			if driftErr != nil {
				log.Printf("[METRIC_REHYDRATE_ERR] provider_open_drifts_count: %v", driftErr)
				// Non-fatal; continue.
			} else {
				r.metrics.providerOpenDrifts.Store(openDriftTotal.Total)
			}

			log.Printf("[METRIC_REHYDRATE_OK] incidents_by_source=%d states incidents_by_status=%d states escalations_sum=%d obs_by_trust=%d contradiction_total=%d open_drifts=%d",
				len(openedRows), len(transRows), escTotal.Total, len(obsByTrustRows), contradictionTotal.Total, openDriftTotal.Total)
			SetMetricTruthfulnessState(MetricTruthfulnessHydrated)
			return
		}
	}

	SetMetricTruthfulnessState(MetricTruthfulnessHydrated)
	log.Printf("[METRIC_REHYDRATE_OK] incidents_by_source=%d states incidents_by_status=%d states escalations_sum=%d",
		len(openedRows), len(transRows), escTotal.Total)
}

// ─────────────────────────────────────────────────────────────────────────
// FORENSIC SNAPSHOT EXPORT
// ─────────────────────────────────────────────────────────────────────────

// ForensicSnapshot is the deterministic, replay-safe export shape.
// All slices are sorted by their natural keys (created/started_at ASC)
// so two snapshots taken against identical DB state produce byte-identical
// JSON (modulo the snapshot_at timestamp).
//
// Phase 3.9.7 additions: ProviderObservations + ProviderDrifts include
// all provider truth evidence for this target. Operators can reconstruct
// what the provider claimed, when it diverged from runtime expectation,
// and when (if) it converged.
type ForensicSnapshot struct {
	// Phase 3.9.10 — integrity block. Always first two fields so readers can
	// validate schema compatibility before parsing the payload.
	SchemaVersion string `json:"schema_version"`
	// ExportHash is the hex-encoded SHA-256 of the canonical JSON of this
	// snapshot with ExportHash set to the empty string. Allows tamper
	// detection after export. Two exports of the same snapshot file with the
	// same data produce the same hash; two exports taken at different times
	// (different SnapshotAt) produce different hashes by design.
	ExportHash string `json:"export_hash"`

	TargetID             string                   `json:"target_id"`
	SnapshotAt           string                   `json:"snapshot_at"`
	TruthfulnessNote     string                   `json:"truthfulness_note"`
	Target               map[string]interface{}   `json:"target,omitempty"`
	Operations           []map[string]interface{} `json:"operations"`
	History              []map[string]interface{} `json:"history"`
	Incidents            []map[string]interface{} `json:"incidents"`
	Lease                map[string]interface{}   `json:"lease,omitempty"`
	InvariantAudit       []InvariantViolation     `json:"invariant_audit"`
	MetricTruthfulness   string                   `json:"metric_truthfulness"`
	// Phase 3.9.7 — provider truth timeline.
	ProviderObservations []map[string]interface{} `json:"provider_observations"`
	ProviderDrifts       []map[string]interface{} `json:"provider_drifts"`
}

// ExportForensicSnapshot reads the full persisted state for a target and
// returns a deterministic JSON-friendly snapshot. Includes operations,
// history, incidents, and lease state — plus an inline invariant audit
// scoped to this target.
//
// Best-effort: rows that fail to load are omitted with a log; the snapshot
// includes a truthfulness note when this happens.
func ExportForensicSnapshot(app *pb.PocketBase, targetID string) *ForensicSnapshot {
	if targetID == "" {
		return nil
	}
	snap := &ForensicSnapshot{
		SchemaVersion:      ForensicSchemaVersion,
		ExportHash:         "", // filled in after all data is collected
		TargetID:           targetID,
		SnapshotAt:         time.Now().UTC().Format(time.RFC3339),
		MetricTruthfulness: string(GetMetricTruthfulnessState()),
	}

	// 1. Target row.
	if rec, err := app.Dao().FindRecordById("deployment_targets", targetID); err == nil {
		snap.Target = recordToMap(rec.SchemaData())
		snap.Target["id"] = rec.Id
	} else {
		snap.TruthfulnessNote = "target row not found"
		log.Printf("[FORENSIC_SNAPSHOT_TARGET_MISSING] target=%s", targetID)
	}

	// 2. Operations — sorted by started_at ASC.
	var opRows []dbx.NullStringMap
	err := app.Dao().DB().
		Select("*").
		From("operations").
		Where(dbx.NewExp("target = {:t}", dbx.Params{"t": targetID})).
		OrderBy("started_at ASC").
		All(&opRows)
	if err != nil {
		log.Printf("[FORENSIC_SNAPSHOT_OPS_ERR] target=%s err=%v", targetID, err)
		snap.TruthfulnessNote = appendNote(snap.TruthfulnessNote, "operations read failed: "+err.Error())
	} else {
		snap.Operations = nullStringMapsToMaps(opRows)
	}

	// 3. History — sorted by created ASC.
	var histRows []dbx.NullStringMap
	err = app.Dao().DB().
		Select("*").
		From("deployment_history").
		Where(dbx.NewExp("target = {:t}", dbx.Params{"t": targetID})).
		OrderBy("created ASC").
		All(&histRows)
	if err != nil {
		log.Printf("[FORENSIC_SNAPSHOT_HISTORY_ERR] target=%s err=%v", targetID, err)
		snap.TruthfulnessNote = appendNote(snap.TruthfulnessNote, "history read failed: "+err.Error())
	} else {
		snap.History = nullStringMapsToMaps(histRows)
	}

	// 4. Incidents — sorted by first_seen_at ASC.
	var incRows []dbx.NullStringMap
	err = app.Dao().DB().
		Select("*").
		From("incidents").
		Where(dbx.NewExp("target = {:t}", dbx.Params{"t": targetID})).
		OrderBy("first_seen_at ASC").
		All(&incRows)
	if err != nil {
		log.Printf("[FORENSIC_SNAPSHOT_INCIDENTS_ERR] target=%s err=%v", targetID, err)
		snap.TruthfulnessNote = appendNote(snap.TruthfulnessNote, "incidents read failed: "+err.Error())
	} else {
		snap.Incidents = nullStringMapsToMaps(incRows)
	}

	// 5. Lease (current, if any).
	var leaseRows []dbx.NullStringMap
	err = app.Dao().DB().
		Select("*").
		From("reconciler_leases").
		Where(dbx.NewExp("lease_key = {:t}", dbx.Params{"t": targetID})).
		All(&leaseRows)
	if err == nil && len(leaseRows) > 0 {
		maps := nullStringMapsToMaps(leaseRows)
		if len(maps) > 0 {
			snap.Lease = maps[0]
		}
	}

	// 6. Provider observations — sorted by observed_at ASC for determinism.
	var obsRows []dbx.NullStringMap
	err = app.Dao().DB().
		Select("*").
		From("provider_observations").
		Where(dbx.NewExp("target = {:t}", dbx.Params{"t": targetID})).
		OrderBy("observed_at ASC").
		All(&obsRows)
	if err != nil {
		log.Printf("[FORENSIC_SNAPSHOT_OBS_ERR] target=%s err=%v", targetID, err)
		snap.TruthfulnessNote = appendNote(snap.TruthfulnessNote, "provider_observations read failed: "+err.Error())
	} else {
		snap.ProviderObservations = nullStringMapsToMaps(obsRows)
	}

	// 7. Provider drifts — sorted by first_seen_at ASC for determinism.
	var driftRows []dbx.NullStringMap
	err = app.Dao().DB().
		Select("*").
		From("provider_drifts").
		Where(dbx.NewExp("target = {:t}", dbx.Params{"t": targetID})).
		OrderBy("first_seen_at ASC").
		All(&driftRows)
	if err != nil {
		log.Printf("[FORENSIC_SNAPSHOT_DRIFTS_ERR] target=%s err=%v", targetID, err)
		snap.TruthfulnessNote = appendNote(snap.TruthfulnessNote, "provider_drifts read failed: "+err.Error())
	} else {
		snap.ProviderDrifts = nullStringMapsToMaps(driftRows)
	}

	// 8. Inline invariant audit scoped to this target.
	allViolations := RunInvariantAudit(app)
	for _, v := range allViolations {
		if v.TargetID == targetID {
			snap.InvariantAudit = append(snap.InvariantAudit, v)
		}
	}
	// Deterministic ordering on output.
	sort.Slice(snap.InvariantAudit, func(i, j int) bool {
		return snap.InvariantAudit[i].InvariantName < snap.InvariantAudit[j].InvariantName
	})

	// 9. Phase 3.9.10 — compute export integrity checksum.
	// Hash is computed with ExportHash="" so the hash covers all actual data.
	// encoding/json serializes struct fields in declaration order; all slice
	// elements are already sorted above so the JSON is deterministic for
	// identical DB content and SnapshotAt.
	snap.ExportHash = "" // ensure field is empty before hashing
	hashPayload, err := json.Marshal(snap)
	if err != nil {
		log.Printf("[FORENSIC_HASH_ERR] target=%s err=%v — export_hash will be empty", targetID, err)
	} else {
		sum := sha256.Sum256(hashPayload)
		snap.ExportHash = hex.EncodeToString(sum[:])
	}

	return snap
}

// ─────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────

func nullStringMapsToMaps(rows []dbx.NullStringMap) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		m := make(map[string]interface{}, len(row))
		// Sort keys for determinism — though Go map iteration order varies,
		// JSON encoding uses key order which is alphabetical for map keys
		// in the standard library.
		keys := make([]string, 0, len(row))
		for k := range row {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			ns := row[k]
			if ns.Valid {
				m[k] = ns.String
			} else {
				m[k] = nil
			}
		}
		out = append(out, m)
	}
	return out
}

func recordToMap(data map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(data))
	for k, v := range data {
		out[k] = v
	}
	return out
}

func appendNote(existing, addition string) string {
	if existing == "" {
		return addition
	}
	return existing + "; " + addition
}
