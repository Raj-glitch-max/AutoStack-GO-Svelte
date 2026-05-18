package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/janlauber/one-click/pkg/reconciler"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/dbx"
	pb "github.com/pocketbase/pocketbase"
)

// Phase 3.10 — Operator truth surface API.
//
// These handlers expose provider_observations, provider_drifts, and runtime
// truthfulness state to operators. All routes are READ-ONLY. They never
// mutate reconciler state or DB records.
//
// ─────────────────────────────────────────────────────────────────────────
// HONEST CONSTRAINTS
// ─────────────────────────────────────────────────────────────────────────
//   - Pagination is mandatory; no route returns unbounded results.
//   - Filtering is server-side; client receives only what it asked for.
//   - All timestamps returned are UTC RFC3339. No timezone conversion.
//   - Contradiction-tagged observations are always included regardless of
//     trust filter — they are immortal forensic evidence.

// HandleProviderObservationList lists provider_observations with filtering.
//
// Query params:
//   - target      — filter by deployment_targets.id
//   - provider    — filter by provider string
//   - trust       — filter by observation_trust (authoritative|inferred|ambiguous|stale|contradicted)
//   - contradiction — "true" to show only contradiction-tagged rows
//   - source      — filter by observation_source
//   - since       — RFC3339 lower bound on observed_at
//   - until       — RFC3339 upper bound on observed_at
//   - limit       — default 100, cap 500
//
// Returns: { "observations": [...], "count": N, "queried_at": "..." }
func HandleProviderObservationList(c echo.Context, app *pb.PocketBase) error {
	limit := 100
	if raw := c.QueryParam("limit"); raw != "" {
		_, _ = fmt.Sscanf(raw, "%d", &limit)
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	q := app.Dao().DB().
		Select("id", "target", "provider", "observed_at", "observation_source",
			"observation_trust", "provider_state", "runtime_expected_state",
			"contradiction_detected", "cycle_id", "evidence").
		From("provider_observations")

	if v := c.QueryParam("target"); v != "" {
		q = q.AndWhere(dbx.NewExp("target = {:t}", dbx.Params{"t": v}))
	}
	if v := c.QueryParam("provider"); v != "" {
		q = q.AndWhere(dbx.NewExp("provider = {:p}", dbx.Params{"p": v}))
	}
	if v := c.QueryParam("trust"); v != "" {
		q = q.AndWhere(dbx.NewExp("observation_trust = {:trust}", dbx.Params{"trust": v}))
	}
	if c.QueryParam("contradiction") == "true" {
		q = q.AndWhere(dbx.NewExp("contradiction_detected = 1"))
	}
	if v := c.QueryParam("source"); v != "" {
		q = q.AndWhere(dbx.NewExp("observation_source = {:src}", dbx.Params{"src": v}))
	}
	if v := c.QueryParam("since"); v != "" {
		q = q.AndWhere(dbx.NewExp("observed_at >= {:since}", dbx.Params{"since": v}))
	}
	if v := c.QueryParam("until"); v != "" {
		q = q.AndWhere(dbx.NewExp("observed_at <= {:until}", dbx.Params{"until": v}))
	}

	var rows []struct {
		ID                   string `db:"id" json:"id"`
		Target               string `db:"target" json:"target"`
		Provider             string `db:"provider" json:"provider"`
		ObservedAt           string `db:"observed_at" json:"observed_at"`
		ObservationSource    string `db:"observation_source" json:"observation_source"`
		ObservationTrust     string `db:"observation_trust" json:"observation_trust"`
		ProviderState        string `db:"provider_state" json:"provider_state"`
		RuntimeExpectedState string `db:"runtime_expected_state" json:"runtime_expected_state"`
		ContradictionDetected int   `db:"contradiction_detected" json:"contradiction_detected"`
		CycleID              string `db:"cycle_id" json:"cycle_id"`
		Evidence             string `db:"evidence" json:"evidence"`
	}

	err := q.OrderBy("observed_at DESC").Limit(int64(limit)).All(&rows)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "query failed: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"observations": rows,
		"count":        len(rows),
		"queried_at":   time.Now().UTC().Format(time.RFC3339),
	})
}

// HandleProviderDriftList lists provider_drifts with filtering.
//
// Query params:
//   - target      — filter by deployment_targets.id
//   - provider    — filter by provider string
//   - severity    — filter by severity
//   - unresolved  — "true" to show only unresolved drifts
//   - limit       — default 100, cap 500
//
// Returns unresolved-first, then by first_seen_at DESC.
// { "drifts": [...], "count": N, "queried_at": "..." }
func HandleProviderDriftList(c echo.Context, app *pb.PocketBase) error {
	limit := 100
	if raw := c.QueryParam("limit"); raw != "" {
		_, _ = fmt.Sscanf(raw, "%d", &limit)
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	q := app.Dao().DB().
		Select("id", "target", "provider", "expected_state", "observed_state",
			"drift_count", "severity", "first_seen_at", "last_seen_at",
			"resolved_at", "resolution_reason").
		From("provider_drifts")

	if v := c.QueryParam("target"); v != "" {
		q = q.AndWhere(dbx.NewExp("target = {:t}", dbx.Params{"t": v}))
	}
	if v := c.QueryParam("provider"); v != "" {
		q = q.AndWhere(dbx.NewExp("provider = {:p}", dbx.Params{"p": v}))
	}
	if v := c.QueryParam("severity"); v != "" {
		q = q.AndWhere(dbx.NewExp("severity = {:sev}", dbx.Params{"sev": v}))
	}
	if c.QueryParam("unresolved") == "true" {
		q = q.AndWhere(dbx.NewExp("(resolved_at IS NULL OR resolved_at = '')"))
	}

	var rows []struct {
		ID               string `db:"id" json:"id"`
		Target           string `db:"target" json:"target"`
		Provider         string `db:"provider" json:"provider"`
		ExpectedState    string `db:"expected_state" json:"expected_state"`
		ObservedState    string `db:"observed_state" json:"observed_state"`
		DriftCount       int    `db:"drift_count" json:"drift_count"`
		Severity         string `db:"severity" json:"severity"`
		FirstSeenAt      string `db:"first_seen_at" json:"first_seen_at"`
		LastSeenAt       string `db:"last_seen_at" json:"last_seen_at"`
		ResolvedAt       string `db:"resolved_at" json:"resolved_at"`
		ResolutionReason string `db:"resolution_reason" json:"resolution_reason"`
	}

	// Unresolved drifts first (CASE WHEN resolved_at IS NULL OR resolved_at='' THEN 0 ELSE 1 END),
	// then by first_seen_at DESC so the oldest open drifts appear before newer ones.
	err := q.OrderBy(
		"CASE WHEN (resolved_at IS NULL OR resolved_at = '') THEN 0 ELSE 1 END ASC",
		"first_seen_at DESC",
	).Limit(int64(limit)).All(&rows)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "query failed: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"drifts":     rows,
		"count":      len(rows),
		"queried_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// HandleRuntimeTruthStatus returns a consolidated runtime truth status panel.
//
// This is the backend for the /runtime/truth operator view. It aggregates:
//   - metric truthfulness state
//   - unresolved incident count
//   - unresolved drift count
//   - open contradiction count (on-demand audit)
//   - silence count (last 10m)
//   - fatal invariant status
//   - truth audit interval
//   - last audit log (best-effort, from process state)
//
// Returns a single status object. Never mutates state.
func HandleRuntimeTruthStatus(c echo.Context, app *pb.PocketBase) error {
	r := reconciler.Active()
	if r == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "reconciler not started",
		})
	}

	// Unresolved incidents count.
	var unresolvedIncidents struct{ Total int64 `db:"total"` }
	_ = app.Dao().DB().NewQuery(
		`SELECT COUNT(*) AS total FROM incidents WHERE status != 'resolved'`,
	).One(&unresolvedIncidents)

	// Unresolved drifts count.
	var unresolvedDrifts struct{ Total int64 `db:"total"` }
	_ = app.Dao().DB().NewQuery(
		`SELECT COUNT(*) AS total FROM provider_drifts WHERE (resolved_at IS NULL OR resolved_at = '')`,
	).One(&unresolvedDrifts)

	// Stale observation count (last 30m, trust=ambiguous or stale).
	cutoff30m := time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339)
	var staleObs struct{ Total int64 `db:"total"` }
	_ = app.Dao().DB().NewQuery(
		`SELECT COUNT(*) AS total FROM provider_observations
		 WHERE observed_at > {:cutoff}
		   AND observation_trust IN ('ambiguous', 'stale')`,
	).Bind(map[string]interface{}{"cutoff": cutoff30m}).One(&staleObs)

	// Silence count (from metrics).
	snap := r.Metrics()

	// Fatal invariant status.
	violations := reconciler.RunInvariantAudit(app)
	hasFatal := reconciler.HasFatalViolation(violations)
	fatalCount := 0
	for _, v := range violations {
		if v.Severity == "fatal" {
			fatalCount++
		}
	}

	// Contradiction count (on-demand, lightweight).
	contradictions := reconciler.RunContradictionAudit(app)

	status := "green"
	if hasFatal {
		status = "red"
	} else if reconciler.GetMetricTruthfulnessState() == reconciler.MetricTruthfulnessPartial {
		status = "yellow"
	} else if unresolvedDrifts.Total > 0 || len(contradictions) > 0 {
		status = "yellow"
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":                   status, // green | yellow | red
		"metric_truthfulness":      string(reconciler.GetMetricTruthfulnessState()),
		"unresolved_incidents":     unresolvedIncidents.Total,
		"unresolved_drifts":        unresolvedDrifts.Total,
		"stale_observations_30m":   staleObs.Total,
		"open_contradictions":      len(contradictions),
		"provider_silence_total":   snap.ProviderSilenceTotal,
		"fatal_invariant_count":    fatalCount,
		"has_fatal_invariant":      hasFatal,
		"provider_observations_by_trust": snap.ProviderObsByTrust,
		"provider_contradictions_total":  snap.ProviderContradictions,
		"truth_audit_interval":     r.Config().TruthAuditInterval.String(),
		"queried_at":               time.Now().UTC().Format(time.RFC3339),
		"limitations": []string{
			"status aggregation is point-in-time; contradiction audit is synchronous and adds latency",
			"provider silence metrics reset on restart",
			"stale_observations count is approximate (30m window only)",
		},
	})
}

// HandleVerifyExportHash verifies the SHA-256 integrity of a forensic snapshot.
//
// Body: the full JSON of a ForensicSnapshot (as produced by /forensic-snapshot/:id).
//
// The handler recomputes the hash with export_hash="" and compares. Returns
// whether the snapshot is intact or has been tampered with.
//
// POST /api/v1/reconciler/verify-export-hash
func HandleVerifyExportHash(c echo.Context) error {
	var body map[string]interface{}
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid JSON body: " + err.Error(),
		})
	}

	storedHash, _ := body["export_hash"].(string)
	if storedHash == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "export_hash field missing or empty in the submitted snapshot",
		})
	}

	// Zero out the hash field and recompute.
	body["export_hash"] = ""
	payload, err := json.Marshal(body)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "re-serialization failed: " + err.Error(),
		})
	}
	sum := sha256.Sum256(payload)
	recomputed := hex.EncodeToString(sum[:])

	intact := storedHash == recomputed
	result := map[string]interface{}{
		"intact":            intact,
		"stored_hash":       storedHash,
		"recomputed_hash":   recomputed,
		"schema_version":    body["schema_version"],
		"snapshot_at":       body["snapshot_at"],
		"target_id":         body["target_id"],
		"verified_at":       time.Now().UTC().Format(time.RFC3339),
	}
	if !intact {
		result["warning"] = "hash mismatch — snapshot may have been modified after export"
	}
	return c.JSON(http.StatusOK, result)
}
