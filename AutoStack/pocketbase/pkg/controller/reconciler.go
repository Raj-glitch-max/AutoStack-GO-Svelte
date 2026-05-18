package controller

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/janlauber/one-click/pkg/reconciler"
	"github.com/labstack/echo/v5"
)

// Phase 3.9.1 + 3.9.6 — Reconciler observability + incident API.
//
// These routes expose the reconciler's in-memory metrics and the
// forensic incident reconstruction. They are READ-ONLY: handlers
// never mutate orchestration state.
//
// Honest scope:
//   - /metrics returns a JSON snapshot of the in-process counters.
//   - /metrics/prom returns Prometheus text exposition format for
//     scraping by a single prometheus instance. Cardinality is bounded
//     (no per-target metric labels) to prevent series explosion.
//   - /incident/:targetID returns a structured incident over a window.
//
// What these routes do NOT do:
//   - Durable metric storage (in-memory only; restart resets)
//   - Distributed tracing
//   - Histograms (counters only)
//   - Authorization beyond PocketBase auth gate at the route level

// HandleReconcilerMetrics returns the current metrics snapshot as JSON.
//
// Operational notes:
//   - Snapshot is atomic across all counters (atomic.Load each)
//   - Restart resets the snapshot to zero — operators computing rates
//     over a restart window will see counter resets
func HandleReconcilerMetrics(c echo.Context) error {
	r := reconciler.Active()
	if r == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "reconciler not started",
		})
	}
	return c.JSON(http.StatusOK, r.Metrics())
}

// HandleReconcilerMetricsProm returns metrics in Prometheus text exposition format.
//
// Cardinality contract: NO per-target labels in this exposition. All series
// are process-scoped totals. This is intentional — adding target-id labels
// would create an unbounded label cardinality vulnerable to the Prometheus
// cardinality explosion failure mode.
//
// Phase 3.9.1 honest scope: counters only (no histograms). Histograms
// require careful bucket selection per metric; deferred.
func HandleReconcilerMetricsProm(c echo.Context) error {
	r := reconciler.Active()
	if r == nil {
		return c.String(http.StatusServiceUnavailable, "# reconciler not started\n")
	}
	snap := r.Metrics()

	// Build text exposition. Stable ordering for diffable scrapes.
	type metric struct {
		Name string
		Help string
		Val  int64
	}
	metrics := []metric{
		{"autostack_reconcile_cycles_total", "Total reconcile cycles executed.", snap.CyclesTotal},
		{"autostack_reconcile_cycles_failed_total", "Total reconcile cycles that panicked at the cycle level.", snap.CyclesFailed},
		{"autostack_targets_reconciled_total", "Total target reconcile outcomes classified as success.", snap.TargetsReconciled},
		{"autostack_targets_skipped_total", "Total target reconcile outcomes classified as skipped.", snap.TargetsSkipped},

		{"autostack_deploys_dispatched_total", "Total deploy operations dispatched (post-CAS).", snap.DeploysDispatched},
		{"autostack_deploys_succeeded_total", "Total deploy operations that completed successfully.", snap.DeploysSucceeded},
		{"autostack_deploys_failed_total", "Total deploy operations that ended in failure.", snap.DeploysFailed},
		{"autostack_destroys_dispatched_total", "Total destroy operations dispatched.", snap.DestroysDispatched},
		{"autostack_destroys_succeeded_total", "Total destroy operations confirmed.", snap.DestroysSucceeded},
		{"autostack_destroys_failed_total", "Total destroy operations that ended in failure.", snap.DestroysFailed},
		{"autostack_rollbacks_dispatched_total", "Total rollback operations dispatched.", snap.RollbacksDispatched},
		{"autostack_rollbacks_succeeded_total", "Total rollback operations confirmed.", snap.RollbacksSucceeded},
		{"autostack_rollbacks_failed_total", "Total rollback operations that ended in failure.", snap.RollbacksFailed},

		{"autostack_suspicion_holds_total", "Total updating->error suspicion holds (Phase 3 Change 6 guard).", snap.SuspicionHolds},
		{"autostack_forward_progress_holds_total", "Total forward-progress regression holds (Phase 3.4 guard).", snap.ForwardProgressHolds},
		{"autostack_stale_read_holds_total", "Total stale-read holds (Phase 3.4 guard).", snap.StaleReadHolds},
		{"autostack_low_confidence_holds_total", "Total low-confidence observation holds (Phase 3.4 guard).", snap.LowConfidenceHolds},

		{"autostack_drift_detected_total", "Total transitions into a non-no_drift state.", snap.DriftDetected},
		{"autostack_drift_resolved_total", "Total transitions from drift back to no_drift.", snap.DriftResolved},

		{"autostack_ambiguity_set_total", "Total transitions into ambiguous state.", snap.AmbiguitySet},
		{"autostack_ambiguity_resolved_total", "Total ambiguity resolutions.", snap.AmbiguityResolved},
		{"autostack_ambiguity_timeouts_total", "Total [AMBIGUITY_TIMEOUT] observations.", snap.AmbiguityTimeouts},
		{"autostack_ambiguity_escalations_total", "Total escalations to error after consecutive timeouts.", snap.AmbiguityEscalations},

		{"autostack_sweep_reclaimed_total", "Total operations reclaimed by startup or runtime sweep.", snap.SweepReclaimed},
		{"autostack_sweep_foreign_pod_total", "Total reclamations of operations stamped with a foreign pod identity.", snap.SweepForeignPod},

		{"autostack_provider_observations_high_total", "Total provider GetStatus observations classified as high confidence.", snap.HighConfidenceObs},
		{"autostack_provider_observations_medium_total", "Total provider GetStatus observations classified as medium confidence.", snap.MediumConfidenceObs},
		{"autostack_provider_observations_low_total", "Total provider GetStatus observations classified as low confidence.", snap.LowConfidenceObs},

		// Phase 3.9.3 — retry runtime counters.
		{"autostack_retries_suppressed_by_lease_loss_total", "Retries suppressed because the lease was invalid at decision time.", snap.RetrySuppressedLease},
		{"autostack_retries_suppressed_by_cooldown_total", "Reconcile ticks skipped because next_retry_at is in the future.", snap.RetrySuppressedCooldown},
		{"autostack_retries_activated_total", "retry_pending → pending transitions after backoff elapsed.", snap.RetryActivated},
		{"autostack_retry_operator_action_required_total", "Failures that required operator action (auth/quota/rate_limit).", snap.RetryOperatorActions},
		{"autostack_ambiguity_escalations_from_retry_total", "Ambiguity escalations triggered by retry exhaustion on provider-uncertain class.", snap.AmbiguityFromRetry},

		// Phase 3.9.4 — incident lifecycle counters.
		{"autostack_incident_escalations_total", "Incidents that escalated (severity bumped or status moved to escalating).", snap.IncidentEscalations},
	}
	sort.SliceStable(metrics, func(i, j int) bool { return metrics[i].Name < metrics[j].Name })

	var b strings.Builder
	for _, m := range metrics {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s counter\n%s %d\n",
			m.Name, m.Help, m.Name, m.Name, m.Val)
	}

	// Phase 3.9.3 per-class counters with bounded cardinality (10 RetryClass values).
	// Format: autostack_retries_scheduled_total{class="retry_transient"} N
	b.WriteString("# HELP autostack_retries_scheduled_total Retries scheduled by RetryClass.\n")
	b.WriteString("# TYPE autostack_retries_scheduled_total counter\n")
	for _, cls := range sortedKeys(snap.RetryScheduledByClass) {
		fmt.Fprintf(&b, "autostack_retries_scheduled_total{class=%q} %d\n", cls, snap.RetryScheduledByClass[cls])
	}
	b.WriteString("# HELP autostack_retries_exhausted_total Retries that exhausted their budget by RetryClass.\n")
	b.WriteString("# TYPE autostack_retries_exhausted_total counter\n")
	for _, cls := range sortedKeys(snap.RetryExhaustedByClass) {
		fmt.Fprintf(&b, "autostack_retries_exhausted_total{class=%q} %d\n", cls, snap.RetryExhaustedByClass[cls])
	}

	// Phase 3.9.4 incident counters (bounded cardinality: max 7 sources, 6 statuses).
	b.WriteString("# HELP autostack_incidents_opened_total Incidents opened by source.\n")
	b.WriteString("# TYPE autostack_incidents_opened_total counter\n")
	for _, src := range sortedKeys(snap.IncidentsOpenedBySource) {
		fmt.Fprintf(&b, "autostack_incidents_opened_total{source=%q} %d\n", src, snap.IncidentsOpenedBySource[src])
	}
	b.WriteString("# HELP autostack_incident_transitions_total Incident state transitions by new status.\n")
	b.WriteString("# TYPE autostack_incident_transitions_total counter\n")
	for _, status := range sortedKeys(snap.IncidentTransitionsByStatus) {
		fmt.Fprintf(&b, "autostack_incident_transitions_total{status=%q} %d\n", status, snap.IncidentTransitionsByStatus[status])
	}

	// Phase 3.9.6 decision counter (bounded: 16 decision types).
	b.WriteString("# HELP autostack_decisions_recorded_total Runtime decisions persisted by decision_type.\n")
	b.WriteString("# TYPE autostack_decisions_recorded_total counter\n")
	for _, dt := range sortedKeys(snap.DecisionsRecordedByType) {
		fmt.Fprintf(&b, "autostack_decisions_recorded_total{decision_type=%q} %d\n", dt, snap.DecisionsRecordedByType[dt])
	}

	return c.Blob(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(b.String()))
}

// sortedKeys returns the map keys in lexicographic order — needed for
// diffable scrape output. Without sorting, Prometheus would still parse
// the response correctly, but scrape-to-scrape diffs would be noisy.
func sortedKeys(m map[string]int64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// HandleReconcilerDecisions — Phase 3.9.6: list runtime decisions.
//
// Query params:
//   - target — filter by deployment_target id
//   - cycle_id — filter by reconcile cycle
//   - decision_type — filter by canonical DecisionType
//   - since — RFC3339 lower bound
//   - until — RFC3339 upper bound
//   - limit — default 100, cap 500
//
// Returns: { "decisions": [...], "count": N }
func HandleReconcilerDecisions(c echo.Context) error {
	r := reconciler.Active()
	if r == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "reconciler not started",
		})
	}
	target := c.QueryParam("target")
	cycleID := c.QueryParam("cycle_id")
	decisionType := c.QueryParam("decision_type")
	since := c.QueryParam("since")
	until := c.QueryParam("until")
	limit := 100
	if raw := c.QueryParam("limit"); raw != "" {
		_, _ = fmt.Sscanf(raw, "%d", &limit)
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	decisions, err := reconciler.ListDecisions(r.App(), target, cycleID, decisionType, since, until, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "list decisions failed: " + err.Error(),
		})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"decisions": decisions,
		"count":     len(decisions),
	})
}

// HandleReconcilerNarrative — Phase 3.9.6: deterministic merged timeline
// for a target.
//
// Path:   /api/v1/reconciler/narrative/:targetID
// Query:  window_hours (default 24, cap 168)
//
// Returns: a Narrative with sorted entries from decisions+operations+
// history+incidents. Two calls against unchanged DB produce byte-identical
// output modulo timestamps.
func HandleReconcilerNarrative(c echo.Context) error {
	r := reconciler.Active()
	if r == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "reconciler not started",
		})
	}
	targetID := c.PathParam("targetID")
	if targetID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "targetID required"})
	}
	windowHours := 24
	if raw := c.QueryParam("window_hours"); raw != "" {
		var parsed int
		_, _ = fmt.Sscanf(raw, "%d", &parsed)
		if parsed > 0 && parsed <= 168 {
			windowHours = parsed
		}
	}
	end := time.Now()
	start := end.Add(-time.Duration(windowHours) * time.Hour)
	narrative := reconciler.ReconstructNarrative(r.App(), targetID, start, end)
	if narrative == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "narrative unavailable"})
	}
	return c.JSON(http.StatusOK, narrative)
}

// HandleReconcilerContradictions — Phase 3.9.6: on-demand contradiction audit.
//
// Query params:
//   - target — optional filter
//   - severity — optional (high|warning|fatal)
//
// Returns: { "contradictions": [...], "count": N, "audited_at": "..." }
func HandleReconcilerContradictions(c echo.Context) error {
	r := reconciler.Active()
	if r == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "reconciler not started",
		})
	}
	findings := reconciler.RunContradictionAudit(r.App())
	targetFilter := c.QueryParam("target")
	severityFilter := c.QueryParam("severity")
	if targetFilter != "" || severityFilter != "" {
		filtered := make([]reconciler.Contradiction, 0, len(findings))
		for _, f := range findings {
			if targetFilter != "" && f.TargetID != targetFilter {
				continue
			}
			if severityFilter != "" && f.Severity != severityFilter {
				continue
			}
			filtered = append(filtered, f)
		}
		findings = filtered
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"contradictions": findings,
		"count":          len(findings),
		"audited_at":     time.Now().UTC().Format(time.RFC3339),
	})
}

// HandleReconcilerForensicSnapshot — Phase 3.9.5: deterministic export
// of all persisted state about a target (operations, history, incidents,
// lease, scoped invariant audit).
//
// Path param:
//   targetID — the deployment_targets.id
//
// Returns a JSON ForensicSnapshot. Two calls against unchanged DB state
// produce byte-identical output modulo the snapshot_at timestamp.
//
// Auth-gated.
func HandleReconcilerForensicSnapshot(c echo.Context) error {
	r := reconciler.Active()
	if r == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "reconciler not started",
		})
	}
	targetID := c.PathParam("targetID")
	if targetID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "targetID required",
		})
	}
	snap := reconciler.ExportForensicSnapshot(r.App(), targetID)
	if snap == nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "snapshot unavailable",
		})
	}
	return c.JSON(http.StatusOK, snap)
}

// HandleReconcilerInvariantAudit — Phase 3.9.5: on-demand invariant audit.
// Returns the full list of violations across all registered invariants.
//
// Auth-gated. Read-only.
func HandleReconcilerInvariantAudit(c echo.Context) error {
	r := reconciler.Active()
	if r == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "reconciler not started",
		})
	}
	violations := reconciler.RunInvariantAudit(r.App())
	return c.JSON(http.StatusOK, map[string]interface{}{
		"violations": violations,
		"count":      len(violations),
		"has_fatal":  reconciler.HasFatalViolation(violations),
	})
}

// HandleReconcilerMetricTruthfulness — Phase 3.9.5: report whether the
// in-memory metrics have been rehydrated from DB truth post-restart.
//
// States:
//   "untrusted_pre_rehydrate" — counters are zero or partial; reconciler
//                               has not yet completed startup
//   "hydrated_from_db"        — counters reflect persistent truth + activity since
//   "partial_db_read_error"   — rehydration partially failed; metrics may underreport
//
// Auth-gated.
func HandleReconcilerMetricTruthfulness(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"metric_truthfulness_state": string(reconciler.GetMetricTruthfulnessState()),
	})
}

// HandleReconcilerIncidentList — Phase 3.9.4: list persisted incidents.
//
// Query params:
//   - status: filter by IncidentStatus (open, escalating, acknowledged,
//     resolved, reopened, suppressed). Omit or "all" for unfiltered.
//   - limit: max rows (1–500, default 100).
//
// Returns JSON array of IncidentRecord, newest-first by last_seen_at.
// Auth-gated.
func HandleReconcilerIncidentList(c echo.Context) error {
	r := reconciler.Active()
	if r == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "reconciler not started",
		})
	}
	status := c.QueryParam("status")
	limit := 100
	if raw := c.QueryParam("limit"); raw != "" {
		_, _ = fmt.Sscanf(raw, "%d", &limit)
	}
	incidents, err := reconciler.ListIncidents(r.App(), status, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "list incidents failed: " + err.Error(),
		})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"incidents": incidents,
		"count":     len(incidents),
	})
}

// HandleReconcilerIncidentGet — Phase 3.9.4: fetch a single incident.
func HandleReconcilerIncidentGet(c echo.Context) error {
	r := reconciler.Active()
	if r == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "reconciler not started",
		})
	}
	id := c.PathParam("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "incident id required",
		})
	}
	inc, err := reconciler.GetIncident(r.App(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "incident not found",
		})
	}
	return c.JSON(http.StatusOK, inc)
}

// HandleReconcilerIncidentTransition — Phase 3.9.4: operator-driven state
// transition (acknowledge, resolve, reopen, suppress).
//
// Body JSON:
//   {
//     "to": "acknowledged" | "resolved" | "reopened" | "suppressed",
//     "reason": "...",        // REQUIRED for resolved
//     "actor": "user-id"      // REQUIRED for acknowledged
//   }
//
// Returns 400 on forbidden transitions (state machine refused).
// Auth-gated.
func HandleReconcilerIncidentTransition(c echo.Context) error {
	r := reconciler.Active()
	if r == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "reconciler not started",
		})
	}
	id := c.PathParam("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "incident id required",
		})
	}
	var body struct {
		To     string `json:"to"`
		Reason string `json:"reason"`
		Actor  string `json:"actor"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid body: " + err.Error(),
		})
	}
	if body.To == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "to field required",
		})
	}
	if err := r.UpdateIncidentStatus(id, reconciler.IncidentStatus(body.To), body.Reason, body.Actor); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
	}
	// Return the updated record.
	inc, _ := reconciler.GetIncident(r.App(), id)
	return c.JSON(http.StatusOK, inc)
}

// HandleReconcilerIncident reconstructs an IncidentTimeline for a target over
// a time window (defaults: last 24h).
//
// Query params:
//   - window_hours: integer, default 24, capped at 168 (7 days). The
//     cap exists to bound DB query cost — incident reconstruction reads
//     all operations + history within the window.
//
// Returns: JSON Incident struct from pkg/reconciler/incident.go.
func HandleReconcilerIncident(c echo.Context) error {
	r := reconciler.Active()
	if r == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "reconciler not started",
		})
	}
	targetID := c.PathParam("targetID")
	if targetID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "targetID path parameter required",
		})
	}

	windowHours := 24
	if raw := c.QueryParam("window_hours"); raw != "" {
		var parsed int
		_, _ = fmt.Sscanf(raw, "%d", &parsed)
		if parsed > 0 && parsed <= 168 {
			windowHours = parsed
		}
	}
	end := time.Now()
	start := end.Add(-time.Duration(windowHours) * time.Hour)

	inc := reconciler.ReconstructIncidentTimeline(r.App(), targetID, start, end)
	if inc == nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error":     "target not found or no reconstructable history",
			"target_id": targetID,
		})
	}
	return c.JSON(http.StatusOK, inc)
}
