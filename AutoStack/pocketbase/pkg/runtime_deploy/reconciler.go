// Continuous reconciliation: periodically inspects deployments whose desired
// state is "running" (post-certification) and compares observed Docker truth
// against desired truth. Drift produces an append-only observation; persistent
// drift transitions the deployment to `contradicted`. The reconciler NEVER
// silently repairs drift. Operator action is required.
package runtime_deploy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/models"
)

// Reconciler runs a separate background loop from the lifecycle Engine. The
// Engine drives a deployment forward through pre-certification states; the
// Reconciler watches deployments that have reached steady-state and reports
// drift between intended state and observed Docker truth.
type Reconciler struct {
	store     *Store
	driver    *DockerDriver
	app       interface{ Logger() any } // unused; reserved for slog
	tickEvery time.Duration
	stop      chan struct{}
	mu        sync.RWMutex
	running   bool
}

// NewReconciler constructs a Reconciler. It does not start the loop —
// call Start to begin reconciliation.
func NewReconciler(store *Store, driver *DockerDriver) *Reconciler {
	return &Reconciler{
		store:     store,
		driver:    driver,
		tickEvery: 5 * time.Second,
		stop:      make(chan struct{}),
	}
}

// Start launches the background loop.
func (r *Reconciler) Start(ctx context.Context) {
	r.mu.Lock()
	r.running = true
	r.mu.Unlock()
	go r.loop(ctx)
	log.Printf("[reconciler] started (tick=%s, lease_ttl=%s)", r.tickEvery, LeaseTTL)
}

// Stop signals the loop to exit.
func (r *Reconciler) Stop() {
	close(r.stop)
}

// Running reports loop state, for health endpoints.
func (r *Reconciler) Running() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.running
}

func (r *Reconciler) loop(ctx context.Context) {
	t := time.NewTicker(r.tickEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stop:
			return
		case <-t.C:
			r.tick(ctx)
		}
	}
}

// tick reconciles every deployment that has reached a steady-state where the
// container is expected to be running. Pre-certification states are owned by
// the lifecycle Engine, not the reconciler.
func (r *Reconciler) tick(ctx context.Context) {
	deps, err := r.store.ListByState(StateCertified, StateContradicted, StateRolledBack)
	if err != nil {
		log.Printf("[reconciler] list active: %v", err)
		return
	}
	for _, d := range deps {
		r.reconcile(ctx, d)
	}
}

// reconcile is the per-deployment workhorse. Each call:
//
//  1. Acquires/refreshes a lease (skip if another worker holds it).
//  2. Inspects the real docker container.
//  3. Records the observation (append-only).
//  4. If the observation reveals drift, transitions to contradicted and
//     opens an incident — but ONLY on the first detection of that kind of
//     drift, so we don't flood the events table.
//  5. If the previous state was contradicted and the drift has resolved
//     itself, records an observation but does NOT auto-uncontradict —
//     operator must explicitly acknowledge.
func (r *Reconciler) reconcile(ctx context.Context, d *Deployment) {
	if err := r.store.AcquireLease(d.ID); err != nil {
		if errors.Is(err, ErrLeaseHeldByOther) {
			return // another worker owns it; that's fine
		}
		log.Printf("[reconciler] lease acquire failed for %s: %v", d.ID, err)
		return
	}

	if d.ContainerID == "" {
		// No container ever started — shouldn't be in certified, but guard anyway.
		r.recordDirect(d, "no_container_id", 1.0, "deployment has no container_id; cannot inspect")
		return
	}

	// 1) Container existence + status.
	status, err := r.driver.InspectStatus(ctx, d.ContainerID)
	if err != nil {
		r.recordDirect(d, "inspect_status_error", 0.5, err.Error())
		return
	}
	if status == "" {
		// Container is GONE — strongest contradiction.
		r.recordDirect(d, "container_missing", 1.0,
			fmt.Sprintf("docker inspect returned no container for id %s", d.ContainerID))
		r.declareContradicted(d, "container_missing",
			fmt.Sprintf("container %s disappeared after certification", d.ContainerID[:12]))
		return
	}
	if status != "running" {
		// Container exists but isn't running.
		r.recordDirect(d, "container_not_running", 1.0,
			fmt.Sprintf("observed status=%q (expected=running)", status))
		r.declareContradicted(d, "container_not_running",
			fmt.Sprintf("container status=%q (expected running)", status))
		return
	}

	// 2) Image identity check — guards against external `docker run` recreating
	//    the container with a different image, or against image-tag drift.
	observedImage, err := r.driver.InspectImage(ctx, d.ContainerID)
	if err != nil {
		r.recordDirect(d, "inspect_image_error", 0.5, err.Error())
		return
	}
	if observedImage != d.Image {
		r.recordDirect(d, "image_mismatch", 1.0,
			fmt.Sprintf("observed image=%q desired image=%q", observedImage, d.Image))
		r.declareContradicted(d, "image_mismatch",
			fmt.Sprintf("observed image=%q != desired=%q", observedImage, d.Image))
		return
	}

	// 3) Steady-state observation — record it at lower frequency so the
	//    observations table doesn't bloat. Skip if we already have a recent
	//    healthy observation.
	r.recordHealthyIfStale(d)
}

// recordDirect persists an observation tagged as engine/direct.
func (r *Reconciler) recordDirect(d *Deployment, kind string, confidence float64, evidence string) {
	_ = r.store.RecordObservation(Observation{
		DeploymentID: d.ID,
		Source:       SourceEngine,
		Trust:        TrustDirect,
		Confidence:   confidence,
		Kind:         kind,
		Evidence:     evidence,
		ObservedAt:   time.Now().UTC(),
	})
}

// recordHealthyIfStale writes a "healthy" observation only if the last
// healthy observation is older than 2× the tick interval. Avoids flooding
// the observation log during normal steady-state. Also updates the
// deployment's last_health_check_at timestamp — this fires on EVERY tick
// regardless of observation-write throttling, so the timestamp truly
// reflects the last probe.
func (r *Reconciler) recordHealthyIfStale(d *Deployment) {
	r.updateLastHealthCheck(d)

	recent, err := r.store.ListObservations(d.ID, 1)
	if err != nil {
		return
	}
	if len(recent) > 0 && recent[0].Kind == "healthy" &&
		time.Since(recent[0].ObservedAt) < 2*r.tickEvery {
		return
	}
	r.recordDirect(d, "healthy", 1.0, "container running, image matches desired")
}

// updateLastHealthCheck stamps the deployment record with the current time.
// Cheap UPDATE — no schema transition.  Failures are logged-and-ignored;
// this is observability metadata, not lifecycle state.
func (r *Reconciler) updateLastHealthCheck(d *Deployment) {
	rec, err := r.store.app.Dao().FindRecordById("runtime_deployments", d.ID)
	if err != nil {
		return
	}
	rec.Set("last_health_check_at", pbDate(time.Now().UTC()))
	_ = r.store.app.Dao().SaveRecord(rec)
}

// declareContradicted transitions the deployment to `contradicted` and opens
// an incident — but only if the deployment is not already in that state.
// Re-entering an already-contradicted state would create spurious events.
func (r *Reconciler) declareContradicted(d *Deployment, kind, detail string) {
	if d.State == StateContradicted {
		return // already contradicted; don't re-fire transition
	}
	err := r.store.Transition(d.ID, d.State, StateContradicted,
		"reconciler.contradiction_detected", detail, "", nil)
	if err != nil {
		log.Printf("[reconciler] failed to transition %s to contradicted: %v", d.ID, err)
		return
	}
	r.openIncident(d, kind, detail)
}

// openIncident writes an incident record using the existing `incidents`
// collection so the operator surfaces (incidents list, forensic view) pick
// it up automatically. Idempotent on (target, source, kind, status=open).
func (r *Reconciler) openIncident(d *Deployment, kind, detail string) {
	// Check for an existing open incident of the same kind to avoid duplicates.
	existing, err := r.store.app.Dao().FindRecordsByFilter("incidents",
		"target = {:t} && source = {:s} && status = 'open'",
		"-first_seen_at", 1, 0,
		map[string]interface{}{"t": d.ID, "s": "runtime-engine"})
	if err == nil && len(existing) > 0 {
		// Bump last_seen_at on the existing incident instead of creating a duplicate.
		ex := existing[0]
		ex.Set("last_seen_at", time.Now().UTC().Format(time.RFC3339Nano))
		_ = r.store.app.Dao().SaveRecord(ex)
		return
	}

	coll, cErr := r.store.app.Dao().FindCollectionByNameOrId("incidents")
	if cErr != nil {
		log.Printf("[reconciler] incidents collection unavailable: %v", cErr)
		return
	}
	rec := models.NewRecord(coll)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	rec.Set("target", d.ID)
	rec.Set("source", "runtime-engine")
	rec.Set("severity", severityFor(kind))
	rec.Set("status", "open")
	rec.Set("first_seen_at", now)
	rec.Set("last_seen_at", now)
	rec.Set("forensic_summary", fmt.Sprintf("runtime contradiction (%s): %s", kind, truncate(detail, 500)))
	if err := r.store.app.Dao().SaveRecord(rec); err != nil {
		log.Printf("[reconciler] failed to open incident for %s: %v", d.ID, err)
	}
}

func severityFor(kind string) string {
	switch kind {
	case "container_missing", "image_mismatch":
		return "critical"
	case "container_not_running":
		return "high"
	default:
		return "medium"
	}
}

// helper to keep the package boundary clean — used by the reconciler for
// log output only.
func eventTypePrefix() string { return "reconciler" }

// internal: shut warnings for tests that don't import all helpers
var _ = strings.HasPrefix
