package runtime_deploy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/models"
)

// Engine walks deployments through the lifecycle state machine using the
// DockerDriver to make real container changes. There is one engine per
// process — no distributed coordination, no lease arbitration. That is a
// deliberate scope choice: this engine is the local-Docker first-class
// runtime, not a cluster scheduler.
type Engine struct {
	store      *Store
	driver     *DockerDriver
	tickEvery  time.Duration
	stabilize  time.Duration
	healthHTTP *http.Client
	dockerOK   bool
	dockerErr  string
	mu         sync.RWMutex
	stop       chan struct{}
}

// NewEngine constructs an engine. Caller must call Start to begin the worker.
func NewEngine(store *Store, driver *DockerDriver) *Engine {
	return &Engine{
		store:      store,
		driver:     driver,
		tickEvery:  3 * time.Second,
		stabilize:  4 * time.Second,
		healthHTTP: &http.Client{Timeout: 3 * time.Second},
		stop:       make(chan struct{}),
	}
}

// Health returns whether the engine can drive real deployments. False
// means docker is unreachable; the engine remains running but every
// deployment will fail at the validating step. The UI surfaces this.
func (e *Engine) Health() (bool, string) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.dockerOK, e.dockerErr
}

// Start probes docker availability and launches the worker loop.
func (e *Engine) Start(ctx context.Context) {
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := e.driver.Available(probeCtx); err != nil {
		e.mu.Lock()
		e.dockerOK = false
		e.dockerErr = err.Error()
		e.mu.Unlock()
		log.Printf("[runtime-engine] docker not available: %v — engine will reject deployments at validate", err)
	} else {
		e.mu.Lock()
		e.dockerOK = true
		e.dockerErr = ""
		e.mu.Unlock()
		log.Printf("[runtime-engine] docker available — engine ready")
	}

	go e.loop(ctx)
}

// Stop signals the loop to exit.
func (e *Engine) Stop() {
	close(e.stop)
}

func (e *Engine) loop(ctx context.Context) {
	t := time.NewTicker(e.tickEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stop:
			return
		case <-t.C:
			e.tick(ctx)
		}
	}
}

// tick advances any deployments in non-terminal forward states.
func (e *Engine) tick(ctx context.Context) {
	// Pull every state the engine still "owns" — terminal states are skipped.
	active, err := e.store.ListByState(
		StatePlanned, StateValidating, StateProvisioning, StateDeploying,
		StateVerifying, StateStabilized, StateRollbackPending, StateRollingBack,
	)
	if err != nil {
		log.Printf("[runtime-engine] list active: %v", err)
		return
	}
	for _, d := range active {
		// Single-writer lease: another engine (or stale dead one) may hold
		// this deployment. Only the holder may advance state. AcquireLease
		// also serves as our heartbeat so the lease doesn't expire while
		// we're working.
		if err := e.store.AcquireLease(d.ID); err != nil {
			// ErrLeaseHeldByOther is expected when running two engines; skip
			// quietly. Any other error is logged but non-fatal.
			continue
		}
		e.advance(ctx, d)
	}
}

// advance runs one transition for a single deployment based on its
// current state. Each step persists before returning.
func (e *Engine) advance(ctx context.Context, d *Deployment) {
	switch d.State {
	case StatePlanned:
		e.toValidating(ctx, d)
	case StateValidating:
		e.toProvisioning(ctx, d)
	case StateProvisioning:
		e.toDeploying(ctx, d)
	case StateDeploying:
		e.toVerifying(ctx, d)
	case StateVerifying:
		e.toStabilized(ctx, d)
	case StateStabilized:
		e.toCertified(ctx, d)
	case StateRollbackPending:
		e.toRollingBack(ctx, d)
	case StateRollingBack:
		e.toRolledBack(ctx, d)
	}
}

func (e *Engine) fail(d *Deployment, from DeploymentState, reason, errMsg string) {
	_ = e.store.Transition(d.ID, from, StateFailed, "engine.fail", reason, errMsg, nil)
}

func (e *Engine) toValidating(ctx context.Context, d *Deployment) {
	if d.Image == "" {
		e.fail(d, StatePlanned, "validation refused: image is required", "missing image")
		return
	}
	if ok, derr := e.Health(); !ok {
		e.fail(d, StatePlanned, "docker unavailable; engine refused to proceed", derr)
		return
	}
	_ = e.store.Transition(d.ID, StatePlanned, StateValidating, "engine.validate", fmt.Sprintf("validating image=%s port=%d:%d", d.Image, d.HostPort, d.ContainerPort), "", nil)
}

func (e *Engine) toProvisioning(ctx context.Context, d *Deployment) {
	_ = e.store.Transition(d.ID, StateValidating, StateProvisioning, "engine.pull", "pulling image "+d.Image, "", nil)
	out, err := e.driver.Pull(ctx, d.Image)
	if err != nil {
		e.fail(d, StateProvisioning, "image pull failed", truncate(out, 500))
		return
	}
	_ = e.store.AppendEvent(d.ID, "docker.pull", truncate(out, 500))
}

func (e *Engine) toDeploying(ctx context.Context, d *Deployment) {
	// Belt + braces cleanup: if a previous container with this name exists, remove it.
	_ = e.driver.Remove(ctx, d.ContainerName)

	cid, err := e.driver.Run(ctx, d.ContainerName, d.Image, d.HostPort, d.ContainerPort)
	if err != nil {
		e.fail(d, StateProvisioning, "container run failed", err.Error())
		return
	}
	healthURL := ""
	if d.HostPort > 0 {
		healthURL = fmt.Sprintf("http://127.0.0.1:%d/", d.HostPort)
	}
	_ = e.store.Transition(d.ID, StateProvisioning, StateDeploying, "engine.run", "container started "+cid[:12], "",
		func(rec *models.Record) {
			rec.Set("container_id", cid)
			rec.Set("health_url", healthURL)
		})
}

func (e *Engine) toVerifying(ctx context.Context, d *Deployment) {
	st, err := e.driver.InspectStatus(ctx, d.ContainerID)
	if err != nil {
		e.fail(d, StateDeploying, "inspect failed", err.Error())
		return
	}
	if st != "running" {
		// Container died — fail loud.
		logs, _ := e.driver.Logs(ctx, d.ContainerID, 50)
		e.fail(d, StateDeploying, fmt.Sprintf("container is not running (status=%q)", st), truncate(logs, 500))
		return
	}
	_ = e.store.Transition(d.ID, StateDeploying, StateVerifying, "engine.verify", "container running; probing health", "", nil)
}

func (e *Engine) toStabilized(ctx context.Context, d *Deployment) {
	// If a host port was provided, attempt a real HTTP probe to the
	// container. Treat any 2xx/3xx/4xx as "the container is responding."
	// 5xx and connection errors fail verification.
	if d.HealthURL != "" {
		resp, err := e.healthHTTP.Get(d.HealthURL)
		if err != nil {
			_ = e.store.AppendEvent(d.ID, "engine.probe", "probe error: "+err.Error())
			// retry next tick rather than failing immediately
			return
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 500 {
			_ = e.store.AppendEvent(d.ID, "engine.probe", fmt.Sprintf("probe returned %d, retrying", resp.StatusCode))
			return
		}
		_ = e.store.Transition(d.ID, StateVerifying, StateStabilized, "engine.probe", fmt.Sprintf("health probe %s returned %d", d.HealthURL, resp.StatusCode), "", nil)
		return
	}
	// No probe URL — fall back to confirming container is still up.
	st, err := e.driver.InspectStatus(ctx, d.ContainerID)
	if err != nil || st != "running" {
		e.fail(d, StateVerifying, "container left running state during verification", st)
		return
	}
	_ = e.store.Transition(d.ID, StateVerifying, StateStabilized, "engine.stabilize", "no health URL — container still running", "", nil)
}

func (e *Engine) toCertified(ctx context.Context, d *Deployment) {
	// Wait for the stabilization window before promoting. This delay is
	// observable in the timeline as the time the engine spent in StateStabilized.
	if time.Since(d.UpdatedAt) < e.stabilize {
		return
	}
	_ = e.store.Transition(d.ID, StateStabilized, StateCertified, "engine.certify",
		"container stable; deployment certified", "",
		func(rec *models.Record) {
			// Set started_at on first certification; preserve it on later transitions.
			if rec.GetString("started_at") == "" {
				rec.Set("started_at", pbDate(time.Now().UTC()))
			}
		})
}

// RequestRollback transitions the deployment into the rollback queue. The
// engine picks it up on its next tick. Caller-provided reason is recorded.
func (e *Engine) RequestRollback(d *Deployment, req RollbackRequest) error {
	target := req.TargetImage
	if target == "" {
		target = d.PreviousImage
	}
	if target == "" {
		return fmt.Errorf("rollback refused: no previous image recorded for deployment %s", d.ID)
	}
	if !canRollback(d.State) {
		return fmt.Errorf("rollback refused: deployment in state %q is not rollback-eligible", d.State)
	}
	return e.store.Transition(d.ID, d.State, StateRollbackPending, "operator.rollback_requested",
		"rollback requested: target="+target+" reason="+req.Reason, "",
		func(rec *models.Record) {
			// Repurpose the desired image as the rollback target; the engine
			// will swap containers using this new value.
			rec.Set("image", target)
		})
}

func canRollback(s DeploymentState) bool {
	switch s {
	case StateCertified, StateContradicted, StateStabilized, StateVerifying, StateDeploying:
		return true
	}
	return false
}

func (e *Engine) toRollingBack(ctx context.Context, d *Deployment) {
	if d.ContainerID != "" {
		if err := e.driver.Stop(ctx, d.ContainerID); err != nil {
			_ = e.store.AppendEvent(d.ID, "engine.rollback.stop_err", err.Error())
		}
		_ = e.driver.Remove(ctx, d.ContainerID)
	}
	out, err := e.driver.Pull(ctx, d.Image)
	if err != nil {
		e.fail(d, StateRollbackPending, "rollback image pull failed", truncate(out, 500))
		return
	}
	cid, err := e.driver.Run(ctx, d.ContainerName, d.Image, d.HostPort, d.ContainerPort)
	if err != nil {
		e.fail(d, StateRollbackPending, "rollback run failed", err.Error())
		return
	}
	_ = e.store.Transition(d.ID, StateRollbackPending, StateRollingBack, "engine.rollback.run", "rolled back container started "+cid[:12], "",
		func(rec *models.Record) { rec.Set("container_id", cid) })
}

func (e *Engine) toRolledBack(ctx context.Context, d *Deployment) {
	st, err := e.driver.InspectStatus(ctx, d.ContainerID)
	if err != nil || st != "running" {
		e.fail(d, StateRollingBack, "rolled-back container did not stabilize", st)
		return
	}
	_ = e.store.Transition(d.ID, StateRollingBack, StateRolledBack, "engine.rollback.complete",
		"rollback complete; container running", "",
		func(rec *models.Record) {
			// Rollback is a restart event — increment restart_count.
			rec.Set("restart_count", rec.GetInt("restart_count")+1)
			// Reset started_at to now (the rolled-back container is a fresh process).
			rec.Set("started_at", pbDate(time.Now().UTC()))
		})
}

// Destroy removes the container and marks the deployment as destroyed.
// This is an operator-initiated terminal action — not part of the auto loop.
func (e *Engine) Destroy(ctx context.Context, d *Deployment, reason string) error {
	if d.ContainerID != "" {
		_ = e.driver.Stop(ctx, d.ContainerID)
		_ = e.driver.Remove(ctx, d.ContainerID)
	}
	return e.store.Transition(d.ID, d.State, StateDestroyed, "operator.destroy", "destroy: "+reason, "", nil)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
