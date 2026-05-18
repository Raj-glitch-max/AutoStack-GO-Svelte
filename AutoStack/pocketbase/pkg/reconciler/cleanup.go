package reconciler

// Operational hygiene cleanup — Phase 2.5.
//
// Prevents the operations and deployment_history tables from growing
// without bound. Cleanup is best-effort, replay-safe, and disable-able
// via AUTOSTACK_CLEANUP_ENABLED. See:
//   - project-context/phase2.5/operation-cleanup-design.md
//   - project-context/phase2.5/retention-policy.md
//   - project-context/phase2.5/cleanup-safety-analysis.md
//
// Invariants:
//   1. Never touches operations with status = 'in_progress'. The
//      startup sweep is the only writer for those.
//   2. Never deletes an op currently referenced by any target's
//      current_operation (FK-guard subquery).
//   3. Logs every pass with deleted counts so cleanup activity is
//      auditable from logs alone.
//   4. Honors AUTOSTACK_CLEANUP_ENABLED=false (skips entirely).

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	pb "github.com/pocketbase/pocketbase"
)

// CleanupConfig captures retention windows + cadence + master switch.
type CleanupConfig struct {
	Enabled                      bool
	Interval                     time.Duration
	OpsRetentionSuccess          time.Duration
	OpsRetentionFailed           time.Duration
	OpsRetentionCancelled        time.Duration
	HistoryRetentionSuccess      time.Duration
	HistoryRetentionFailed       time.Duration
	HistoryRetentionInProgress   time.Duration
	BatchSize                    int
	// Phase 3.9.8 — provider observation + drift retention.
	// ObsRetentionAuthoritative: age after which non-contradiction authoritative
	// observations are eligible for deletion. Contradiction-tagged rows are immortal.
	ObsRetentionAuthoritative  time.Duration
	// ObsRetentionAmbiguous: age after which stale/ambiguous/inferred observations
	// are eligible for deletion.
	ObsRetentionAmbiguous      time.Duration
	// DriftRetentionResolved: age (from resolved_at) after which resolved drift
	// records are eligible for deletion. Unresolved drifts are never deleted.
	DriftRetentionResolved     time.Duration
}

// DefaultCleanupConfig pulls retention windows from env vars with the
// documented defaults. Disabling via AUTOSTACK_CLEANUP_ENABLED=false
// short-circuits the goroutine in StartCleanupOnBoot.
func DefaultCleanupConfig() CleanupConfig {
	return CleanupConfig{
		Enabled:                    envBool("AUTOSTACK_CLEANUP_ENABLED", true),
		Interval:                   envHours("AUTOSTACK_CLEANUP_INTERVAL_HOURS", 24),
		OpsRetentionSuccess:        envDays("AUTOSTACK_OPS_RETENTION_SUCCESS_DAYS", 30),
		OpsRetentionFailed:         envDays("AUTOSTACK_OPS_RETENTION_FAILED_DAYS", 90),
		OpsRetentionCancelled:      envDays("AUTOSTACK_OPS_RETENTION_CANCELLED_DAYS", 7),
		HistoryRetentionSuccess:    envDays("AUTOSTACK_HISTORY_RETENTION_SUCCESS_DAYS", 90),
		HistoryRetentionFailed:     envDays("AUTOSTACK_HISTORY_RETENTION_FAILED_DAYS", 365),
		HistoryRetentionInProgress: envDays("AUTOSTACK_HISTORY_RETENTION_IN_PROGRESS_DAYS", 365),
		BatchSize:                  1000,
		// Phase 3.9.8 defaults:
		//   Authoritative observations: 90d (same as deployment_history success)
		//   Ambiguous/stale/inferred:   30d (short; low forensic value after resolution)
		//   Contradiction-tagged:       immortal (enforced via WHERE contradiction_detected=0)
		//   Resolved drifts:            90d after resolved_at
		ObsRetentionAuthoritative: envDays("AUTOSTACK_OBS_RETENTION_AUTHORITATIVE_DAYS", 90),
		ObsRetentionAmbiguous:     envDays("AUTOSTACK_OBS_RETENTION_AMBIGUOUS_DAYS", 30),
		DriftRetentionResolved:    envDays("AUTOSTACK_DRIFT_RETENTION_RESOLVED_DAYS", 90),
	}
}

// Cleanup runs the retention sweep on its own goroutine.
type Cleanup struct {
	app    *pb.PocketBase
	config CleanupConfig
	stopCh chan struct{}
	mu     sync.Mutex
	started bool
}

// NewCleanup constructs a Cleanup with the provided config.
func NewCleanup(app *pb.PocketBase, config CleanupConfig) *Cleanup {
	return &Cleanup{
		app:    app,
		config: config,
		stopCh: make(chan struct{}),
	}
}

// Start launches the cleanup goroutine. Idempotent. No-op when disabled.
func (c *Cleanup) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return
	}
	c.started = true

	if !c.config.Enabled {
		log.Printf("[CLEANUP_DISABLED] retention skipped — set AUTOSTACK_CLEANUP_ENABLED=true to enable")
		return
	}
	log.Printf("[CLEANUP_READY] interval=%s ops_success=%s ops_failed=%s ops_cancelled=%s history_success=%s history_failed=%s",
		c.config.Interval, c.config.OpsRetentionSuccess, c.config.OpsRetentionFailed,
		c.config.OpsRetentionCancelled, c.config.HistoryRetentionSuccess, c.config.HistoryRetentionFailed)

	go c.run()
}

// Stop signals the goroutine to exit at its next iteration boundary.
func (c *Cleanup) Stop() {
	close(c.stopCh)
}

func (c *Cleanup) run() {
	// Run an initial pass shortly after start so retention takes effect
	// without a 24h wait on first boot.
	initialDelay := time.NewTimer(5 * time.Minute)
	defer initialDelay.Stop()

	select {
	case <-c.stopCh:
		return
	case <-initialDelay.C:
		c.runOnce()
	}

	ticker := time.NewTicker(c.config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.runOnce()
		}
	}
}

// runOnce executes one cleanup pass against operations and history.
// Errors are logged but never propagated — cleanup is best-effort.
func (c *Cleanup) runOnce() {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[CLEANUP_PANIC] %v", rec)
		}
	}()

	now := time.Now().UTC()

	opsDeleted := 0
	opsDeleted += c.deleteOps(now.Add(-c.config.OpsRetentionSuccess), "succeeded", "succeeded_stale")
	opsDeleted += c.deleteOps(now.Add(-c.config.OpsRetentionFailed), "failed")
	opsDeleted += c.deleteOps(now.Add(-c.config.OpsRetentionCancelled), "cancelled")
	log.Printf("[CLEANUP_OPS] deleted=%d", opsDeleted)

	historyDeleted := 0
	historyDeleted += c.deleteHistory(now.Add(-c.config.HistoryRetentionSuccess), "success")
	historyDeleted += c.deleteHistory(now.Add(-c.config.HistoryRetentionFailed), "failed")
	historyDeleted += c.deleteHistory(now.Add(-c.config.HistoryRetentionInProgress), "in_progress")
	log.Printf("[CLEANUP_HISTORY] deleted=%d", historyDeleted)

	// Phase 3.9.4: purge RESOLVED incidents whose resolved_at is older
	// than the incident retention TTL. Unresolved incidents are NEVER
	// touched (PurgeResolvedIncidents enforces this via WHERE status='resolved').
	incidentsDeleted, err := PurgeResolvedIncidents(c.app, incidentRetentionTTL)
	if err != nil {
		log.Printf("[CLEANUP_INCIDENTS_ERR] %v", err)
	} else if incidentsDeleted > 0 {
		log.Printf("[CLEANUP_INCIDENTS] deleted=%d (resolved older than %s)", incidentsDeleted, incidentRetentionTTL)
	}

	// Phase 3.9.2: also purge orphan expired leases (not referenced by
	// any in-progress operation). Best-effort.
	r := &Reconciler{app: c.app}
	if leasesPurged, err := r.PurgeExpiredLeases(c.app); err != nil {
		log.Printf("[CLEANUP_LEASES_ERR] %v", err)
	} else if leasesPurged > 0 {
		log.Printf("[CLEANUP_LEASES] expired_orphans_deleted=%d", leasesPurged)
	}

	// Phase 3.9.8: purge eligible provider observations and resolved drifts.
	// Contradiction-tagged observations are NEVER deleted (immortal forensic evidence).
	// Unresolved drifts are NEVER deleted.
	obsDeleted := c.deleteProviderObservations(now)
	if obsDeleted > 0 {
		log.Printf("[CLEANUP_OBS] deleted=%d", obsDeleted)
	}
	driftsDeleted := c.deleteResolvedProviderDrifts(now.Add(-c.config.DriftRetentionResolved))
	if driftsDeleted > 0 {
		log.Printf("[CLEANUP_DRIFTS] deleted=%d resolved older than %s", driftsDeleted, c.config.DriftRetentionResolved)
	}
}

// incidentRetentionTTL is the window after which a RESOLVED incident is
// eligible for deletion. Unresolved incidents are immortal regardless.
// Default 90 days — same as deployment_history success retention.
//
// Override via env: AUTOSTACK_INCIDENT_RETENTION_DAYS.
var incidentRetentionTTL = envDays("AUTOSTACK_INCIDENT_RETENTION_DAYS", 90)

// deleteProviderObservations purges eligible provider_observations rows:
//   - Non-contradiction authoritative observations older than ObsRetentionAuthoritative
//   - Non-contradiction ambiguous/stale/inferred observations older than ObsRetentionAmbiguous
//
// Contradiction-tagged rows (contradiction_detected = 1) are NEVER deleted — they
// are immortal forensic evidence of provider divergence. Returns total deleted.
func (c *Cleanup) deleteProviderObservations(now time.Time) int {
	authCutoff := now.Add(-c.config.ObsRetentionAuthoritative).Format(time.RFC3339)
	ambCutoff := now.Add(-c.config.ObsRetentionAmbiguous).Format(time.RFC3339)

	// Delete old authoritative/inferred non-contradiction observations.
	res1, err := c.app.Dao().DB().NewQuery(
		"DELETE FROM provider_observations " +
			"WHERE contradiction_detected = 0 " +
			"  AND observation_trust IN ('authoritative', 'inferred') " +
			"  AND observed_at < {:cutoff} " +
			"LIMIT {:limit}",
	).Bind(dbx.Params{"cutoff": authCutoff, "limit": c.config.BatchSize}).Execute()
	n1 := int64(0)
	if err != nil {
		log.Printf("[CLEANUP_OBS_ERR] authoritative: %v", err)
	} else {
		n1, _ = res1.RowsAffected()
	}

	// Delete old ambiguous/stale non-contradiction observations.
	res2, err := c.app.Dao().DB().NewQuery(
		"DELETE FROM provider_observations " +
			"WHERE contradiction_detected = 0 " +
			"  AND observation_trust IN ('ambiguous', 'stale') " +
			"  AND observed_at < {:cutoff} " +
			"LIMIT {:limit}",
	).Bind(dbx.Params{"cutoff": ambCutoff, "limit": c.config.BatchSize}).Execute()
	n2 := int64(0)
	if err != nil {
		log.Printf("[CLEANUP_OBS_ERR] ambiguous/stale: %v", err)
	} else {
		n2, _ = res2.RowsAffected()
	}

	return int(n1 + n2)
}

// deleteResolvedProviderDrifts purges provider_drifts rows that are resolved
// and whose resolved_at is older than cutoff. Unresolved drifts are never touched.
// Returns count deleted.
func (c *Cleanup) deleteResolvedProviderDrifts(cutoff time.Time) int {
	cutoffStr := cutoff.Format(time.RFC3339)
	res, err := c.app.Dao().DB().NewQuery(
		"DELETE FROM provider_drifts " +
			"WHERE resolved_at IS NOT NULL AND resolved_at != '' " +
			"  AND resolved_at < {:cutoff} " +
			"LIMIT {:limit}",
	).Bind(dbx.Params{"cutoff": cutoffStr, "limit": c.config.BatchSize}).Execute()
	if err != nil {
		log.Printf("[CLEANUP_DRIFTS_ERR] %v", err)
		return 0
	}
	n, _ := res.RowsAffected()
	return int(n)
}

// deleteOps removes terminal-status operations older than cutoff while
// excluding any op still referenced by a target's current_operation.
// Returns the number of rows deleted.
//
// Statuses passed in must be a subset of the terminal statuses; this
// function refuses to operate on 'in_progress'.
func (c *Cleanup) deleteOps(cutoff time.Time, statuses ...string) int {
	if len(statuses) == 0 {
		return 0
	}
	// Defensive: ensure 'in_progress' never sneaks in.
	for _, s := range statuses {
		if s == "in_progress" {
			log.Printf("[CLEANUP_REFUSED] deleteOps called with status=in_progress — refusing")
			return 0
		}
	}

	cutoffStr := cutoff.Format(time.RFC3339)

	// Build the IN clause inline; statuses come from constants in this
	// package, so manual interpolation is safe (no user input).
	params := dbx.Params{
		"cutoff": cutoffStr,
		"limit":  c.config.BatchSize,
	}
	placeholders := make([]string, len(statuses))
	for i, s := range statuses {
		key := fmt.Sprintf("st%d", i)
		placeholders[i] = "{:" + key + "}"
		params[key] = s
	}
	inClause := "status IN (" + strings.Join(placeholders, ", ") + ")"

	// Phase 3.9.4 retention safety: DO NOT delete operations referenced
	// by unresolved incidents.
	// Phase 3.9.6 retention safety: DO NOT delete operations referenced
	// by runtime_decisions newer than the FK-guard window (90d). This
	// preserves the decision → operation link for narrative reconstruction.
	//
	// Older decisions may legitimately reference deleted operations; the
	// decision.decision_inputs JSON preserves the relevant fields inline.
	fkCutoff := time.Now().UTC().AddDate(0, 0, -90).Format(time.RFC3339)
	params["fk_cutoff"] = fkCutoff
	res, err := c.app.Dao().DB().NewQuery(
		"DELETE FROM operations " +
			"WHERE " + inClause +
			"  AND updated_at < {:cutoff} " +
			"  AND (id NOT IN (" +
			"     SELECT current_operation FROM deployment_targets " +
			"     WHERE current_operation IS NOT NULL AND current_operation != ''" +
			"  )) " +
			"  AND (id NOT IN (" +
			"     SELECT first_operation_id FROM incidents " +
			"     WHERE status != 'resolved' AND first_operation_id IS NOT NULL AND first_operation_id != ''" +
			"  )) " +
			"  AND (id NOT IN (" +
			"     SELECT last_operation_id FROM incidents " +
			"     WHERE status != 'resolved' AND last_operation_id IS NOT NULL AND last_operation_id != ''" +
			"  )) " +
			"  AND (id NOT IN (" +
			"     SELECT operation_id FROM runtime_decisions " +
			"     WHERE operation_id IS NOT NULL AND operation_id != '' " +
			"       AND decision_timestamp > {:fk_cutoff}" +
			"  )) " +
			"  LIMIT {:limit}",
	).Bind(params).Execute()

	if err != nil {
		log.Printf("[CLEANUP_OPS_ERR] statuses=%v cutoff=%s err=%v", statuses, cutoffStr, err)
		return 0
	}
	n, _ := res.RowsAffected()
	return int(n)
}

// deleteHistory removes deployment_history rows older than cutoff for
// the given status. Returns count.
func (c *Cleanup) deleteHistory(cutoff time.Time, status string) int {
	cutoffStr := cutoff.Format(time.RFC3339)
	res, err := c.app.Dao().DB().NewQuery(
		"DELETE FROM deployment_history " +
			"WHERE status = {:status} AND created < {:cutoff} " +
			"LIMIT {:limit}",
	).Bind(dbx.Params{
		"status": status,
		"cutoff": cutoffStr,
		"limit":  c.config.BatchSize,
	}).Execute()
	if err != nil {
		log.Printf("[CLEANUP_HISTORY_ERR] status=%s cutoff=%s err=%v", status, cutoffStr, err)
		return 0
	}
	n, _ := res.RowsAffected()
	return int(n)
}

// envBool returns the env var parsed as bool, defaulting to def.
func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// envHours returns the env var parsed as an integer number of hours.
func envHours(key string, defHours int) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return time.Duration(defHours) * time.Hour
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return time.Duration(defHours) * time.Hour
	}
	return time.Duration(n) * time.Hour
}

// envMinutes returns the env var parsed as an integer number of minutes.
func envMinutes(key string, defMinutes int) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return time.Duration(defMinutes) * time.Minute
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return time.Duration(defMinutes) * time.Minute
	}
	if n == 0 {
		return 0 // explicit zero disables the feature
	}
	return time.Duration(n) * time.Minute
}

// envDays returns the env var parsed as an integer number of days.
func envDays(key string, defDays int) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return time.Duration(defDays) * 24 * time.Hour
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return time.Duration(defDays) * 24 * time.Hour
	}
	return time.Duration(n) * 24 * time.Hour
}

// StartCleanupOnBoot constructs and starts the cleanup goroutine. Call
// from main.go alongside the reconciler.
func StartCleanupOnBoot(app *pb.PocketBase) {
	c := NewCleanup(app, DefaultCleanupConfig())
	c.Start()
}
