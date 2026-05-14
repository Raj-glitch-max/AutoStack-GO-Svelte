package reconciler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/janlauber/one-click/pkg/providers"
	"github.com/janlauber/one-click/pkg/providers/cloudrun"
	"github.com/janlauber/one-click/pkg/providers/ecs"
	"github.com/janlauber/one-click/pkg/secrets"
	pb "github.com/pocketbase/pocketbase"
	"github.com/pocketbase/dbx"
)

// newCycleID returns a short random hex string used to correlate all log
// emissions from a single reconciliation cycle. Eight hex characters is
// enough entropy to avoid collisions within a single process lifetime
// at the current tick rate, and short enough to read at a glance in logs.
func newCycleID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0"
	}
	return hex.EncodeToString(b[:])
}

// Config holds configuration for the reconciler.
//
// The reconciler loop is intentionally single-threaded today. A worker pool
// will be added when (a) operation persistence exists and (b) the Cloud Run
// provider is verified safe under concurrent calls. Until then, do not add
// a MaxConcurrency field: that promise would be unfulfilled and misleading
// in logs and metrics.
type Config struct {
	Interval         time.Duration // Default 30 seconds
	BackoffBase      time.Duration // Base backoff duration
	BackoffMax       time.Duration // Maximum backoff duration
	FailureThreshold int           // Failures before circuit opens
}

// DefaultConfig returns the default reconciler configuration
func DefaultConfig() Config {
	return Config{
		Interval:         30 * time.Second,
		BackoffBase:      30 * time.Second,
		BackoffMax:       5 * time.Minute,
		FailureThreshold: 5,
	}
}

// Reconciler reconciles cloud deployments with desired state
type Reconciler struct {
	app      *pb.PocketBase
	config   Config
	stopCh   chan struct{}
	started  bool
	startMu  sync.Mutex

	// Failure tracking for circuit breaker
	failures    map[string]int // targetID -> failure count
	failureMu   sync.RWMutex

	// Backoff state
	lastErrorTime time.Time
	lastErrorMu  sync.RWMutex

	// Suspicion counter for updating→error tolerance.
	// Cloud Run can flap Ready=FAILED transiently during revision swap;
	// persisting `error` on a single observation would lie about state.
	// We require two consecutive `error` observations from `updating`
	// before flipping. The counter is per-target, in-memory only — a
	// process restart resets it, which is safe (it just delays the
	// detection of a real error by one tick after restart).
	suspicions    map[string]int // targetID -> consecutive error observations from updating
	suspicionMu   sync.RWMutex

	// Phase 2.6: consecutive succeeded_stale outcomes per target. After
	// staleThreshold (3) the dispatcher releases the target to `error`
	// instead of `pending` to prevent a respec-flapping rollout from
	// burning provider quota indefinitely. In-memory; restart resets to 0.
	staleCount    map[string]int // targetID -> consecutive succeeded_stale outcomes
	staleCountMu  sync.Mutex

	// Phase 2.7: track whether the most recent releaseTargetWithExternal
	// for a given opID actually wrote rows. true → release succeeded;
	// false → 0 rows affected (sweep had reclaimed ownership). Read
	// once and deleted by the caller via releaseStillOwner.
	releaseOutcome   map[string]bool
	releaseOutcomeMu sync.Mutex

	// Phase 2.7: track heartbeat-fail counts per op for the
	// `[HEARTBEAT_FAIL_PERSISTENT]` escalation.
	heartbeatFails  map[string]int
	heartbeatFailMu sync.Mutex

	// Phase 3.6: in-memory operational metrics. Atomic counters; safe for
	// concurrent reconciler ticks. Read via /api/v1/reconciler/metrics (if
	// wired) or via Snapshot() in tests.
	metrics *Metrics

	// Phase 3.6: per-target cooldown timestamps. When a target hits the
	// suspicion threshold repeatedly, the reconciler enforces a cooldown
	// before re-evaluating it — prevents retry-storm pressure on providers.
	cooldown   map[string]time.Time
	cooldownMu sync.RWMutex
}

// staleThreshold is the number of consecutive succeeded_stale outcomes
// before the dispatcher refuses to keep re-dispatching. See
// [[phase2.6/succeeded-stale-guard.md]].
const staleThreshold = 3

// New creates a new cloud reconciler
func New(app *pb.PocketBase, config Config) *Reconciler {
	if config.Interval == 0 {
		config = DefaultConfig()
	}
	return &Reconciler{
		app:            app,
		config:         config,
		stopCh:         make(chan struct{}),
		failures:       make(map[string]int),
		suspicions:     make(map[string]int),
		staleCount:     make(map[string]int),
		releaseOutcome: make(map[string]bool),
		heartbeatFails: make(map[string]int),
		metrics:        &Metrics{},
		cooldown:       make(map[string]time.Time),
	}
}

// Metrics returns the in-memory metrics snapshot for the reconciler.
// Phase 3.6: lightweight, atomic counters. Safe for concurrent reads.
func (r *Reconciler) Metrics() MetricsSnapshot {
	if r.metrics == nil {
		return MetricsSnapshot{}
	}
	return r.metrics.Snapshot()
}

// cooldownActive reports whether a target is currently in a Phase 3.8
// reconciliation cooldown window. Used to enforce bounded reconciliation
// economics: a target that repeatedly fails the suspicion guard is
// rate-limited so we don't burn provider quota in a tight loop.
//
// Cooldown duration: 30 seconds, applied when a suspicion guard fires
// AT threshold (the guard would have promoted; we slow it down instead).
const reconciliationCooldown = 30 * time.Second

func (r *Reconciler) cooldownActive(targetID string) bool {
	r.cooldownMu.RLock()
	until, ok := r.cooldown[targetID]
	r.cooldownMu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(until) {
		// Expired; clear it lazily.
		r.cooldownMu.Lock()
		delete(r.cooldown, targetID)
		r.cooldownMu.Unlock()
		return false
	}
	return true
}

func (r *Reconciler) setCooldown(targetID string) {
	r.cooldownMu.Lock()
	r.cooldown[targetID] = time.Now().Add(reconciliationCooldown)
	r.cooldownMu.Unlock()
}

// Start begins the reconciliation loop.
// Safe to call multiple times — only starts once.
//
// Ordering invariant (Phase 2.1): the crash-recovery sweep runs
// SYNCHRONOUSLY under the same startMu lock that gates the ticker.
// This makes the "sweep-before-ticker" guarantee local to the
// reconciler rather than dependent on main.go calling them in the
// right order. A future contributor adding another OnAfterBootstrap
// handler can't accidentally break sequencing.
func (r *Reconciler) Start() {
	r.startMu.Lock()
	defer r.startMu.Unlock()

	if r.started {
		log.Println("Reconciler already started, skipping")
		return
	}
	r.started = true

	log.Println("Starting cloud reconciler")

	// Phase 3.8: initialize pod identity BEFORE any operation can be claimed.
	// This identity stamps every operation we own and provides forensic
	// visibility for the sweep when it reclaims foreign-pod operations.
	initPodIdentity()

	// Register providers and cache their capability profiles.
	// Capability caching must happen BEFORE any reconciler tick fires.
	crProvider := cloudrun.NewProvider()
	providers.RegisterProvider(providers.ProviderGCPCloudRun, crProvider)
	cacheCapabilities(providers.ProviderGCPCloudRun, crProvider)

	ecsProvider := ecs.NewProvider()
	providers.RegisterProvider(providers.ProviderAWSECS, ecsProvider)
	cacheCapabilities(providers.ProviderAWSECS, ecsProvider)

	// Crash-recovery sweep runs BEFORE we accept any tick. Any operation
	// left in_progress at this moment belongs to the previous process
	// (single-pod) and must be marked failed.
	if err := SweepAbandonedOperations(r.app); err != nil {
		log.Printf("[STARTUP_SWEEP_ERR] %v", err)
	}

	// Start the reconciliation ticker
	go r.run()

	// Phase 2.6: runtime sweep goroutine. Periodically reclaims ops
	// whose heartbeat went stale (post-first-heartbeat-death stuck-state).
	// Single-pod safe; multi-pod work blocked until Phase 2.7
	// pod-identity stamping.
	go RunRuntimeSweepLoop(r.app, r.stopCh)
}

// Stop stops the reconciler
func (r *Reconciler) Stop() {
	log.Println("Stopping cloud reconciler")
	close(r.stopCh)
}

func (r *Reconciler) run() {
	ticker := time.NewTicker(r.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.reconcileWithBackoff()
		}
	}
}

// reconcileWithBackoff applies exponential backoff on cycle-level errors.
//
// "Cycle-level error" means anything that fires recordError() — typically a
// failure to query deployment_targets at all. Per-target failures are
// independently tracked by the circuit breaker.
//
// The backoff multiplier is derived from the current maximum per-target
// failure count under the failureMu mutex. Previously this read r.failures
// while holding lastErrorMu, which is the wrong lock for that field — a
// data race per the Go memory model and a source of inconsistent backoff
// decisions across cycles.
func (r *Reconciler) reconcileWithBackoff() {
	r.lastErrorMu.RLock()
	lastErr := r.lastErrorTime
	r.lastErrorMu.RUnlock()

	if !lastErr.IsZero() {
		sinceLastError := time.Since(lastErr)
		backoffDuration := r.backoffDuration()
		if sinceLastError < backoffDuration {
			// Phase 2.7: surface backoff skips so operators see why
			// no cycle ran. Was previously silent.
			log.Printf("[CYCLE_BACKED_OFF] since_last_error=%s backoff_duration=%s", sinceLastError, backoffDuration)
			return // Skip this cycle, still in backoff
		}
	}

	r.reconcileAll()
}

// backoffDuration computes the current backoff window based on the maximum
// per-target failure count, capped at BackoffMax. Reads r.failures under the
// correct mutex.
func (r *Reconciler) backoffDuration() time.Duration {
	r.failureMu.RLock()
	maxFailures := 0
	for _, count := range r.failures {
		if count > maxFailures {
			maxFailures = count
		}
	}
	r.failureMu.RUnlock()

	d := r.config.BackoffBase
	for i := 0; i < maxFailures; i++ {
		d *= 2
		if d >= r.config.BackoffMax {
			return r.config.BackoffMax
		}
	}
	return d
}

// reconcileAll is wrapped with panic recovery.
//
// IMPORTANT: A panic at the cycle level must NOT clear the per-target
// failures map. Doing so would silently re-close circuit breakers across
// unrelated targets and let known-broken deployments retry indefinitely.
// Per-target panics are handled and scoped inside reconcileOne; a panic
// reaching this level is a bug we want to leave visible, not to swallow
// alongside other state.
func (r *Reconciler) reconcileAll() {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[PANIC] reconcileAll: %v", rec)
		}
	}()

	ctx := context.Background()

	cycleID := newCycleID()
	cycleStart := time.Now()

	// Find all deployment_targets with cloud deployments (not kubernetes)
	// and their associated rollouts. We also pull rollouts.manifest (the
	// YAML spec — used to build a DeploySpec for dispatch) and
	// rollouts.updated (used as a revision token for stale-spec detection).
	//
	// Phase 2.3 additions in the projection:
	//   - deployment_targets.provider AS target_provider — canonical provider
	//     name (gcp-cloudrun) used in deployment_history rows, distinct from
	//     cloud_accounts.provider (gcp).
	//   - deployment_targets.pending_destroy — read so the dispatcher can
	//     route post-deploy success to `deleting` when an endDate-set
	//     arrived mid-flight.
	var rows []map[string]interface{}
	err := r.app.Dao().DB().
		Select("deployment_targets.*, deployment_targets.provider as target_provider, deployment_targets.pending_destroy as target_pending_destroy, rollouts.manifest as rollout_manifest, rollouts.target_type, rollouts.target_config, rollouts.id as rollout_id, rollouts.updated as rollout_updated, rollouts.endDate as rollout_end_date, cloud_accounts.credentials_encrypted, cloud_accounts.provider, cloud_accounts.region").
		From("deployment_targets").
		InnerJoin("rollouts", dbx.NewExp("rollouts.id = deployment_targets.rollout")).
		InnerJoin("cloud_accounts", dbx.NewExp("cloud_accounts.id = deployment_targets.cloud_account")).
		Where(dbx.NewExp("rollouts.target_type != 'kubernetes'")).
		All(&rows)
	if err != nil {
		log.Printf("Failed to query deployment targets: %v", err)
		r.recordError()
		return
	}

	// Log target count for cycle visibility
	log.Printf("[RECONCILE] cycle=%s cycle_start target_count=%d", cycleID, len(rows))

	// Clear last error on successful query
	r.clearError()

	targetsProcessed := 0
	targetsSucceeded := 0
	targetsFailed := 0

	for _, row := range rows {
		select {
		case <-r.stopCh:
			log.Printf("[RECONCILE] cycle=%s cycle_cancelled targets_processed=%d", cycleID, targetsProcessed)
			return
		default:
			row["__cycle_id"] = cycleID
			result := r.reconcileOne(ctx, row)
			targetsProcessed++
			if result == reconcileSuccess {
				targetsSucceeded++
			} else if result == reconcileFailed {
				targetsFailed++
			}
		}
	}

	// Log reconciliation cycle completion with stats
	cycleDuration := time.Since(cycleStart)
	log.Printf("[RECONCILE] cycle=%s cycle_complete target_count=%d succeeded=%d failed=%d duration_ms=%d",
		cycleID, targetsProcessed, targetsSucceeded, targetsFailed, cycleDuration.Milliseconds())
}

// reconcileResult indicates the result of reconciling one target
type reconcileResult int

const (
	reconcileSuccess reconcileResult = iota
	reconcileFailed
	reconcileSkipped
)

// reconcileOne handles a single target reconciliation with panic recovery.
//
// Panic handling is intentionally scoped to ONE target:
//   - log the panicking target,
//   - increment that target's circuit-breaker count so it stops being retried,
//   - record cycle-level error state so backoff applies on the next cycle.
//
// We do NOT clear the failures map (see reconcileAll). We also do not write
// a status here: writing "error" without a previous-status would bypass the
// transition guard and could overwrite a healthy state with an artifact of
// our own crash.
func (r *Reconciler) reconcileOne(ctx context.Context, row map[string]interface{}) reconcileResult {
	defer func() {
		if rec := recover(); rec != nil {
			targetID, _ := row["id"].(string)
			log.Printf("[PANIC] reconcileOne target=%s: %v", targetID, rec)
			if targetID != "" {
				r.failureMu.Lock()
				r.failures[targetID]++
				r.failureMu.Unlock()
			}
			r.recordError()
		}
	}()

	// Extract fields from the row with nil checks
	targetID, ok := row["id"].(string)
	if !ok || targetID == "" {
		log.Printf("Skipping target: missing or null id")
		return reconcileSkipped
	}

	// Phase 2.4 M-2 + M-3 (pre-circuit-check pass): clear in-memory failure
	// and suspicion state when a target is in the fresh dispatch-eligible
	// state (pending + no in-flight op). MUST run BEFORE the circuit-breaker
	// check below, otherwise an operator-recovered target stays circuit-open
	// until process restart.
	//
	// The on-disk target row's status=pending is the source of truth that
	// "this target is ready to deploy again" — a respec via
	// flipCloudTargetsToPendingOnRespec, an operator manually unblocking it,
	// or a Phase 2.3 stale-spec release all converge through this state.
	prevStatusEarly, _ := row["status"].(string)
	currentOpEarly, _ := row["current_operation"].(string)
	if prevStatusEarly == "pending" && currentOpEarly == "" {
		r.failureMu.Lock()
		if r.failures[targetID] > 0 {
			cycleIDEarly, _ := row["__cycle_id"].(string)
			log.Printf("[CIRCUIT_RESET] cycle=%s target=%s previous_failures=%d reason=pending_dispatch_eligible", cycleIDEarly, targetID, r.failures[targetID])
			delete(r.failures, targetID)
		}
		r.failureMu.Unlock()
		r.clearSuspect(targetID)
		// Phase 2.6: operator respec to recover from a stale-loop hold
		// also clears the in-memory stale count. The threshold restarts.
		r.clearStaleCount(targetID)
	}

	// Check circuit breaker for this target
	r.failureMu.RLock()
	failureCount := r.failures[targetID]
	r.failureMu.RUnlock()

	if failureCount >= r.config.FailureThreshold {
		log.Printf("Target %s: circuit open (failure count %d >= threshold %d), skipping",
			targetID, failureCount, r.config.FailureThreshold)
		return reconcileSkipped
	}

	rolloutID, ok := row["rollout_id"].(string)
	if !ok || rolloutID == "" {
		log.Printf("Skipping target %s: missing rollout_id", targetID)
		return reconcileSkipped
	}

	externalID, _ := row["external_id"].(string)
	provider, ok := row["provider"].(string)
	if !ok || provider == "" {
		log.Printf("Skipping target %s: missing provider", targetID)
		return reconcileSkipped
	}

	region, _ := row["region"].(string)
	previousStatus, _ := row["status"].(string)
	storedCreds, _ := row["credentials_encrypted"].(string)
	credentialsJSON, err := secrets.Decrypt(storedCreds)
	if err != nil {
		log.Printf("[CRED_DECRYPT_FAIL] target=%s error=%v", targetID, err)
		// Refuse to operate. Treat as auth-class failure (no retry storm),
		// surface a clear marker in the target status.
		r.updateTargetStatus(targetID, previousStatus, "error", "credential decryption failed")
		r.recordTargetFailureWithCategory(targetID, FailureAuth)
		return reconcileFailed
	}

	// Parse target_config from rollout.
	// PocketBase JSON columns are returned as map[string]interface{} (not
	// string) when queried via dbx.Select. Support both so we don't silently
	// drop the field in one path or the other.
	var targetConfig map[string]interface{}
	switch tc := row["target_config"].(type) {
	case map[string]interface{}:
		targetConfig = tc
	case string:
		if tc != "" {
			if err := json.Unmarshal([]byte(tc), &targetConfig); err != nil {
				log.Printf("[TARGET_CONFIG_PARSE_ERR] target=%s raw=%q: %v", targetID, tc, err)
			}
		}
	case []byte:
		if len(tc) > 0 {
			if err := json.Unmarshal(tc, &targetConfig); err != nil {
				log.Printf("[TARGET_CONFIG_PARSE_ERR] target=%s raw=%q: %v", targetID, string(tc), err)
			}
		}
	default:
		// Fallback: try string anyway (some PocketBase versions return JSON as string)
		if spec, ok := row["target_config"].(string); ok && spec != "" {
			json.Unmarshal([]byte(spec), &targetConfig)
		}
	}

	// Get the provider
	providerName := providerToProviderName(provider)
	p, err := providers.GetProvider(providerName)
	if err != nil {
		log.Printf("Failed to get provider for %s: %v", providerName, err)
		return reconcileSkipped
	}

	// Build cloud account
	cloudAccountID, _ := row["cloud_account"].(string)
	account := &providers.CloudAccount{
		ID:                   cloudAccountID,
		Provider:             provider,
		Region:               region,
		CredentialsEncrypted: credentialsJSON,
		Status:               "active",
	}

	// Build deployment target
	target := &providers.DeploymentTarget{
		ID:         targetID,
		ExternalID: externalID,
		Provider:   provider,
		Region:     region,
	}

	cycleID, _ := row["__cycle_id"].(string)

	// Log start of target reconciliation
	log.Printf("[RECONCILE_TARGET] cycle=%s target=%s provider=%s external_id=%s status=%s", cycleID, targetID, provider, externalID, previousStatus)

	// Honesty guard #1: terminal `deleted` targets are skipped entirely.
	// Calling GetStatus on a service whose target row is marked deleted
	// produces NOT_FOUND in a loop, burning circuit-breaker budget and
	// flooding logs. The transition guard already refuses to overwrite a
	// `deleted` status, so the poll is also useless.
	if previousStatus == "deleted" {
		log.Printf("[RECONCILE_SKIP] cycle=%s target=%s reason=terminal_deleted", cycleID, targetID)
		return reconcileSkipped
	}

	// Honesty guard #2: if a dispatcher owns this target (current_operation
	// non-empty), the status-poll path MUST defer. Polling mid-deploy
	// against a not-yet-created service produces NOT_FOUND errors that
	// would (a) flap the persisted status to `error`, (b) burn circuit-
	// breaker budget, (c) be silently overwritten by the dispatcher's
	// release. All three are visible to operators and erode trust in the
	// status surface. The dispatcher is the authority during its own
	// in-flight window.
	currentOp, _ := row["current_operation"].(string)
	if currentOp != "" {
		log.Printf("[DISPATCH_IN_FLIGHT] cycle=%s target=%s operation=%s", cycleID, targetID, currentOp)
		return reconcileSkipped
	}

	// Read the canonical target-side provider value. Phase 2.3 uses this
	// (not cloud_accounts.provider) for deployment_history writes so the
	// `provider` field is consistent across collections. Falls back to the
	// `provider` column when `target_provider` (from the explicit AS in the
	// SELECT) is unavailable.
	targetProvider, _ := row["target_provider"].(string)
	if targetProvider == "" {
		targetProvider, _ = row["provider"].(string)
	}

	// pending_destroy is the Phase 2.3 re-arm flag. When set, the dispatcher's
	// post-deploy success path flips the target to `deleting` instead of
	// `updating`. PocketBase bool columns can deserialize as bool, int (0/1),
	// or string ("true"/"false") depending on driver/version, so accept all.
	pendingDestroyRaw := readBoolColumn(row["target_pending_destroy"])
	rolloutEndDate, _ := row["rollout_end_date"].(string)

	// Phase 2.4 M-4: only honor pending_destroy when the rollout still has
	// endDate set. An operator who set endDate, then cleared it (e.g. manual
	// recovery), should not have their target auto-destroyed on the next
	// deploy success. The flag's "ground truth" is the rollout's endDate.
	pendingDestroy := pendingDestroyRaw && rolloutEndDate != ""

	// Phase 2.4 H-1: auto-promote targets with pending_destroy intent
	// to `deleting` so the reconciler dispatches the destroy the operator
	// intended. Without this, the destroy intent preserved on the flag is
	// silently ignored when the target is in `running` or `error` state —
	// the two states from which operators most commonly set endDate.
	//
	// Phase 2.9 AW-C1 fix: now handles `running` (previously only `error`).
	// The `updating` state is intentionally excluded: a target mid-deploy
	// already routes to `deleting` on success via `pendingDestroy` in
	// dispatchDeploy's postSuccessStatus.
	//
	// Guards:
	//   - status must be `error` or `running`
	//   - pending_destroy must be true (endDate is set on the rollout)
	//   - rollout.endDate must still be set (respect operator manual recovery)
	//   - current_operation must be empty (true here — Honesty guard #2)
	if (previousStatus == "error" || previousStatus == "running") && pendingDestroyRaw && rolloutEndDate != "" {
		log.Printf("[CLOUD_DESTROY_AUTO_PROMOTE] cycle=%s target=%s reason=%s_with_pending_destroy_intent", cycleID, targetID, previousStatus)
		r.promoteToDeleting(targetID)
		return reconcileSkipped
	}

	// Dispatch branching:
	//   - pending with no in-flight op  → call Provider.Deploy (claim via CAS)
	//   - deleting with no in-flight op → call Provider.Destroy (claim via CAS)
	//   - anything else                 → status poll via Provider.GetStatus
	//
	// The CAS inside dispatchDeploy/dispatchDestroy is the load-bearing
	// safety check: even if two reconciler ticks reach this branch
	// simultaneously, only one will own the operation.
	if shouldDispatchDeploy(row) {
		manifest, _ := row["rollout_manifest"].(string)
		rolloutRevision, _ := row["rollout_updated"].(string)
		ctx, cancel := context.WithTimeout(ctx, DeployTimeout)
		defer cancel()
		return r.dispatchDeploy(ctx, p, account, targetID, rolloutID, externalID, manifest, rolloutRevision, targetConfig, cycleID, targetProvider, pendingDestroy)
	}
	if shouldDispatchDestroy(row) {
		ctx, cancel := context.WithTimeout(ctx, DeployTimeout)
		defer cancel()
		return r.dispatchDestroy(ctx, p, account, targetID, rolloutID, externalID, cycleID, targetProvider)
	}
	if shouldDispatchRollback(row) {
		// Phase 3.3: look up the prior task def ARN from the most recent
		// succeeded deploy operation's checkpoint_data. If the lookup fails
		// (no prior deploy, no checkpoint), refuse rollback with a clear error
		// rather than dispatching with an empty targetRevision.
		priorTaskDefARN, lookupErr := r.lookupPriorTaskDefARN(targetID)
		if lookupErr != nil {
			log.Printf("[ROLLBACK_NO_PRIOR_CHECKPOINT] cycle=%s target=%s err=%v — refusing rollback", cycleID, targetID, lookupErr)
			r.markTargetError(targetID, "rollback refused: "+lookupErr.Error())
			return reconcileFailed
		}
		ctx, cancel := context.WithTimeout(ctx, DeployTimeout)
		defer cancel()
		return r.dispatchRollback(ctx, p, account, targetID, rolloutID, externalID, priorTaskDefARN, cycleID, targetProvider)
	}

	// Status-poll path.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	status, err := p.GetStatus(ctx, account, target)
	if err != nil {
		sanitizedMsg := sanitizeError(err.Error())
		category := ClassifyError(err.Error())
		log.Printf("[FAILURE] cycle=%s target=%s category=%s message=%v", cycleID, targetID, category, sanitizedMsg)
		r.recordTargetFailureWithCategory(targetID, category)
		// Suspicion tolerance: an `updating` target reporting an error
		// observation may be a provider convergence flap (common in Cloud Run,
		// possible in ECS). Refuse until the per-provider threshold is reached.
		// Phase 3 Change 6: threshold derived from CapEventualConsistency.
		if previousStatus == "updating" && !r.noteSuspectError(targetID, providerName) {
			log.Printf("[SUSPICION_HOLD] cycle=%s target=%s previous=%s reason=updating_error_first_observation", cycleID, targetID, previousStatus)
			return reconcileFailed
		}
		r.updateTargetStatus(targetID, previousStatus, "error", sanitizedMsg)
		return reconcileFailed
	}

	r.clearTargetFailure(targetID)
	r.clearSuspect(targetID)

	if previousStatus != status.Status {
		log.Printf("[STATE_TRANSITION] cycle=%s target=%s from=%s to=%s", cycleID, targetID, previousStatus, status.Status)
	}

	// Phase 3.4: observation guards — mutually exclusive, evaluated in priority order.
	// Each guard calls noteSuspectError at most once per tick per target.
	// Guards use early return; only the first matching guard fires.
	// Phase 3.6: each hold increments a counter for operational visibility.
	// Phase 3.8: at-threshold promotions trigger a cooldown to bound retry economics.
	//
	// Priority:
	//   1. Forward-progress: hard regression (running→creating) — strongest signal
	//   2. Stale-read: known eventual-consistency pattern (running→updating)
	//   3. Low-confidence: provider explicit low-confidence signal on regressions
	//   4. Existing: updating→error suspicion (Phase 3 Change 6)
	switch {
	case !isForwardProgressTransition(previousStatus, status.Status):
		if !r.noteSuspectError(targetID, providerName) {
			log.Printf("[FORWARD_PROGRESS_HOLD] cycle=%s target=%s previous=%s provider_reported=%s — regression held pending threshold",
				cycleID, targetID, previousStatus, status.Status)
			if r.metrics != nil {
				r.metrics.IncForwardProgressHold()
			}
			return reconcileFailed
		}
		log.Printf("[FORWARD_PROGRESS_PROMOTED] cycle=%s target=%s previous=%s provider_reported=%s — threshold reached",
			cycleID, targetID, previousStatus, status.Status)
		r.setCooldown(targetID)

	case isLikelyStaleRead(previousStatus, status.Status):
		if !r.noteSuspectError(targetID, providerName) {
			log.Printf("[STALE_READ_HOLD] cycle=%s target=%s previous=%s provider_reported=%s confidence=%s",
				cycleID, targetID, previousStatus, status.Status, status.ConfidenceLevel)
			if r.metrics != nil {
				r.metrics.IncStaleReadHold()
			}
			return reconcileFailed
		}
		log.Printf("[STALE_READ_PROMOTED] cycle=%s target=%s previous=%s provider_reported=%s — threshold reached",
			cycleID, targetID, previousStatus, status.Status)
		r.setCooldown(targetID)

	case status.ConfidenceLevel == "low" && previousStatus == "running" && status.Status != "running":
		if !r.noteSuspectError(targetID, providerName) {
			log.Printf("[LOW_CONFIDENCE_HOLD] cycle=%s target=%s previous=%s provider_reported=%s — low confidence held",
				cycleID, targetID, previousStatus, status.Status)
			if r.metrics != nil {
				r.metrics.IncLowConfidenceHold()
			}
			return reconcileFailed
		}

	case previousStatus == "updating" && status.Status == "error":
		if !r.noteSuspectError(targetID, providerName) {
			log.Printf("[SUSPICION_HOLD] cycle=%s target=%s previous=%s reason=updating_error_first_observation", cycleID, targetID, previousStatus)
			if r.metrics != nil {
				r.metrics.IncSuspicionHold()
			}
			return reconcileFailed
		}
	}

	// Phase 3.6: track provider observation confidence.
	if r.metrics != nil {
		r.metrics.IncConfidenceObs(status.ConfidenceLevel)
	}

	r.updateTargetStatus(targetID, previousStatus, status.Status, status.Message)
	// Phase 3.2: propagate ambiguity columns from the provider observation.
	r.applyAmbiguityToTarget(targetID, rolloutID, providerName, account.Provider, status)

	// Phase 3.4: drift detection — compare desired spec revision (rollout.updated_at)
	// vs last_deployed_revision (baseline set on deploy success).
	desiredRevision, _ := row["rollout_updated"].(string)
	lastDeployedRevision, _ := row["last_deployed_revision"].(string)
	currentDriftState, _ := row["drift_state"].(string)
	driftState := detectRevisionDrift(desiredRevision, lastDeployedRevision)
	// Promote suspected_drift to confirmed_drift when the target is in a stable
	// state (running or error) — meaning the spec changed but convergence has
	// not been dispatched yet.
	if driftState == DriftStateSuspected && (status.Status == "running" || status.Status == "error") {
		driftState = DriftStateConfirmed
	}
	if driftState != DriftStateNoDrift && driftState != DriftStateUnknown {
		log.Printf("[DRIFT_DETECTED] cycle=%s target=%s drift=%s desired=%s deployed=%s",
			cycleID, targetID, driftState, desiredRevision, lastDeployedRevision)
	}
	driftDetail := ""
	if driftState == DriftStateConfirmed || driftState == DriftStateSuspected {
		driftDetail = "desired revision " + desiredRevision + " differs from deployed revision " + lastDeployedRevision
	}
	r.updateDriftState(targetID, currentDriftState, driftState, driftDetail)

	// Intentionally do NOT propagate status onto the rollout record. See
	// project-context/reconciler/lifecycle-assumptions.md — `rollouts` has
	// no `status` column; writing it would be a phantom-success signal.
	_ = rolloutID

	log.Printf("[RECONCILE_TARGET_COMPLETE] target=%s status=%s", targetID, status.Status)
	return reconcileSuccess
}

// FailureCategory classifies errors for retry decisions
type FailureCategory string

const (
	FailureTransient  FailureCategory = "transient"
	FailurePermanent FailureCategory = "permanent"
	FailureQuota     FailureCategory = "quota"
	FailureAuth      FailureCategory = "auth"
	FailureTimeout   FailureCategory = "timeout"
)

// ClassifyError determines the failure category for retry decisions
func ClassifyError(errMsg string) FailureCategory {
	errLower := strings.ToLower(errMsg)

	// Auth errors - never retry
	authPatterns := []string{"unauthorized", "401", "403", "forbidden", "permission denied",
		"credentials", "authentication", "invalid token", "expired", "access denied"}
	for _, pattern := range authPatterns {
		if strings.Contains(errLower, pattern) {
			return FailureAuth
		}
	}

	// Quota errors - never retry
	quotaPatterns := []string{"quota", "limit exceeded", "rate limit", "429", "billing",
		"resource exhausted", "rate_limit"}
	for _, pattern := range quotaPatterns {
		if strings.Contains(errLower, pattern) {
			return FailureQuota
		}
	}

	// Permanent errors - never retry
	permanentPatterns := []string{"not found", "not_found", "invalid", "malformed",
		"bad request", "400", "422", "409", "conflict"}
	for _, pattern := range permanentPatterns {
		if strings.Contains(errLower, pattern) {
			return FailurePermanent
		}
	}

	// Timeout errors - maybe retry
	timeoutPatterns := []string{"timeout", "timed out", "deadline"}
	for _, pattern := range timeoutPatterns {
		if strings.Contains(errLower, pattern) {
			return FailureTimeout
		}
	}

	// Default to transient (retry)
	return FailureTransient
}

// ShouldRetry returns true if the error category should be retried
func ShouldRetry(category FailureCategory) bool {
	switch category {
	case FailureTransient, FailureTimeout:
		return true
	case FailurePermanent, FailureQuota, FailureAuth:
		return false
	default:
		return false
	}
}

// recordTargetFailureWithCategory records failure with category-aware logic.
//
// Auth and quota errors do not increment the circuit breaker because retrying
// them is wasted API quota — they require external intervention. Transient,
// timeout, and permanent errors all increment the per-target failure count
// AND mark the global lastErrorTime so reconcileWithBackoff actually applies
// backoff at the cycle level. Without that mark, backoff was unreachable for
// any failure that wasn't a top-level DB query failure.
func (r *Reconciler) recordTargetFailureWithCategory(targetID string, category FailureCategory) {
	if category == FailureAuth || category == FailureQuota {
		log.Printf("Target %s: %s failure - not retrying, requires intervention", targetID, category)
		return
	}

	r.failureMu.Lock()
	r.failures[targetID]++
	r.failureMu.Unlock()

	r.recordError()
}

// clearTargetFailure resets failure count for a target
func (r *Reconciler) clearTargetFailure(targetID string) {
	r.failureMu.Lock()
	defer r.failureMu.Unlock()
	delete(r.failures, targetID)
}

// recordError marks that an error occurred
func (r *Reconciler) recordError() {
	r.lastErrorMu.Lock()
	defer r.lastErrorMu.Unlock()
	r.lastErrorTime = time.Now()
}

// clearError clears error state (call on success)
func (r *Reconciler) clearError() {
	r.lastErrorMu.Lock()
	defer r.lastErrorMu.Unlock()
	r.lastErrorTime = time.Time{}
}

// updateTargetStatus writes the new status to the deployment_targets record
// after validating that the transition is permitted by the documented state
// model (see project-context/reconciler/state-model.md and lifecycle-assumptions.md).
//
// A transition is REFUSED (logged, but not persisted) when:
//   - the current status is a terminal "deleted" (no transition is valid out)
//   - the provider reports "pending" while the target is already "running"
//     or "updating" (a single observation cannot regress a healthy state)
//
// last_synced is always updated to reflect that we reached the provider,
// even when we refuse to persist the status field. That lets operators see
// whether the reconciler ran at all, independent of whether it accepted the
// observation.
func (r *Reconciler) updateTargetStatus(targetID, previousStatus, status, message string) {
	target, err := r.app.Dao().FindRecordById("deployment_targets", targetID)
	if err != nil {
		log.Printf("Failed to find deployment target %s: %v", targetID, err)
		return
	}

	// "unknown" is an honest provider-side signal that the response did not
	// contain a condition we can interpret. It is NOT a value in the
	// deployment_targets.status select enum; writing it would either fail
	// the save or silently coerce. Touch last_synced so operators know the
	// poll happened, but leave the persisted status alone.
	if status == "unknown" {
		log.Printf("[STATUS_UNKNOWN] target=%s previous=%s message=%s", targetID, previousStatus, message)
		target.Set("last_synced", time.Now().UTC().Format(time.RFC3339))
		if err := r.app.Dao().SaveRecord(target); err != nil {
			log.Printf("Failed to update last_synced on target %s: %v", targetID, err)
		}
		return
	}

	if !isAllowedTransition(previousStatus, status) {
		reason := ForbiddenTargetTransitionReason(previousStatus, status)
		log.Printf("[TRANSITION_REFUSED] target=%s from=%s to=%s reason=%s", targetID, previousStatus, status, reason)
		target.Set("last_synced", time.Now().UTC().Format(time.RFC3339))
		if err := r.app.Dao().SaveRecord(target); err != nil {
			log.Printf("Failed to update last_synced on target %s: %v", targetID, err)
		}
		return
	}

	target.Set("status", status)
	target.Set("last_synced", time.Now().UTC().Format(time.RFC3339))
	if message != "" {
		target.Set("drift_summary", message)
	}

	if err := r.app.Dao().SaveRecord(target); err != nil {
		log.Printf("Failed to update deployment target %s: %v", targetID, err)
	}
}

// isAllowedTransition delegates to IsAllowedTargetTransition in
// lifecycle_guards.go. Phase 3.2 replaced the stub implementation with
// a proper transition table; this wrapper maintains call-site compatibility.
func isAllowedTransition(previous, next string) bool {
	return IsAllowedTargetTransition(previous, next)
}

// lookupPriorTaskDefARN finds the task_def_arn from the most recent succeeded
// deploy operation for targetID that has a non-null checkpoint_data.
//
// This is the Phase 3.3 rollback source: when the operator triggers a rollback
// (status→"rolling_back"), we need the prior task definition ARN to pass to
// Provider.Rollback(). The ARN is captured in operations.checkpoint_data by
// the ECS provider during Deploy() (Phase 3.2 checkpoint wiring).
//
// Returns the task_def_arn string or a descriptive error if no usable
// checkpoint exists. The caller must refuse rollback on error.
func (r *Reconciler) lookupPriorTaskDefARN(targetID string) (string, error) {
	var rows []map[string]interface{}
	err := r.app.Dao().DB().
		Select("checkpoint_data").
		From("operations").
		Where(dbx.NewExp(
			"target = {:tid} AND kind = 'deploy' AND status = 'succeeded' AND checkpoint_data IS NOT NULL AND checkpoint_data != '' AND checkpoint_data != 'null'",
			dbx.Params{"tid": targetID},
		)).
		OrderBy("started_at DESC").
		Limit(1).
		All(&rows)
	if err != nil {
		return "", fmt.Errorf("checkpoint query failed: %w", err)
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("no prior succeeded deploy with checkpoint_data found for target %s; rollback requires at least one prior deploy", targetID)
	}

	// checkpoint_data is a JSON column — could be returned as string or []byte.
	var checkpointJSON string
	switch v := rows[0]["checkpoint_data"].(type) {
	case string:
		checkpointJSON = v
	case []byte:
		checkpointJSON = string(v)
	default:
		return "", fmt.Errorf("checkpoint_data has unexpected type %T for target %s", rows[0]["checkpoint_data"], targetID)
	}

	var checkpoint map[string]interface{}
	if err := json.Unmarshal([]byte(checkpointJSON), &checkpoint); err != nil {
		return "", fmt.Errorf("checkpoint_data parse failed: %w", err)
	}

	taskDefARN, ok := checkpoint["task_def_arn"].(string)
	if !ok || taskDefARN == "" {
		return "", fmt.Errorf("checkpoint_data for target %s has no task_def_arn (provider may not support checkpoint rollback)", targetID)
	}
	return taskDefARN, nil
}

// updateDriftState writes the drift_state, drift_detected_at, and drift_detail
// columns on deployment_targets when the drift state changes from the current
// persisted value. No-ops when the state is unchanged.
//
// Phase 3.4: revision-only drift detection. called from reconcileOne GetStatus path.
// Best-effort: errors are logged but do not propagate.
func (r *Reconciler) updateDriftState(targetID string, currentPersistedDrift string, state DriftState, detail string) {
	if currentPersistedDrift == string(state) {
		return
	}
	rec, err := r.app.Dao().FindRecordById("deployment_targets", targetID)
	if err != nil {
		log.Printf("[DRIFT_STATE_ERR] target=%s find: %v", targetID, err)
		return
	}
	rec.Set("drift_state", string(state))
	now := time.Now().UTC().Format(time.RFC3339)
	switch state {
	case DriftStateNoDrift, DriftStateUnknown:
		rec.Set("drift_detected_at", nil)
		rec.Set("drift_detail", "")
	default:
		if rec.GetString("drift_detected_at") == "" {
			rec.Set("drift_detected_at", now)
		}
		if detail != "" {
			rec.Set("drift_detail", detail)
		}
	}
	if err := r.app.Dao().SaveRecord(rec); err != nil {
		log.Printf("[DRIFT_STATE_ERR] target=%s save: %v", targetID, err)
		return
	}
	log.Printf("[DRIFT_STATE_CHANGE] target=%s from=%s to=%s", targetID, currentPersistedDrift, state)
}

// applyAmbiguityToTarget writes or clears the five lifecycle ambiguity columns
// on deployment_targets based on ts.Ambiguous.
//
// When ts.Ambiguous == true:
//   - lifecycle_ambiguous         = true
//   - lifecycle_native_state      = ts.NativeState   (verbatim provider state)
//   - lifecycle_ambiguity_source  = ts.AmbiguitySource ("S-1","S-2","S-4","S-5")
//   - lifecycle_ambiguity_detail  = ts.AmbiguityDetail (operator-readable)
//   - lifecycle_ambiguity_deadline = ambiguityDeadline(providerName) [first observation only]
//
// When ts.Ambiguous == false:
//   - All columns including the escalation counter are cleared.
//
// Phase 3.3: forensic history row on deadline expiry (triggeredBy="ambiguity-expiry").
// Phase 3.4: escalation counter (lifecycle_ambiguity_consecutive_timeouts) is
// incremented on each timeout. After ambiguityEscalationThreshold consecutive
// timeouts, the target is escalated to status=error and all ambiguity state
// is cleared. A forensic history row with triggeredBy="ambiguity-escalation"
// is written so operators can reconstruct the escalation decision.
//
// Best-effort: DB errors are logged but do not propagate.
func (r *Reconciler) applyAmbiguityToTarget(targetID, rolloutID, providerName, rawProvider string, ts *providers.TargetStatus) {
	if ts == nil {
		return
	}
	rec, err := r.app.Dao().FindRecordById("deployment_targets", targetID)
	if err != nil {
		log.Printf("[AMBIGUITY_WRITE_ERR] target=%s find: %v", targetID, err)
		return
	}
	if ts.Ambiguous {
		deadline := ambiguityDeadline(providerName)
		existingDeadline := rec.GetString("lifecycle_ambiguity_deadline")
		if existingDeadline != "" {
			parsed, parseErr := time.Parse(time.RFC3339, existingDeadline)
			if parseErr == nil {
				if time.Now().After(parsed) {
					// Phase 3.4: increment escalation counter.
					consecutive := rec.GetInt("lifecycle_ambiguity_consecutive_timeouts") + 1
					rec.Set("lifecycle_ambiguity_consecutive_timeouts", consecutive)

					log.Printf("[AMBIGUITY_TIMEOUT] target=%s source=%s native_state=%s deadline=%s consecutive=%d",
						targetID, ts.AmbiguitySource, ts.NativeState, existingDeadline, consecutive)
					msg := "ambiguity unresolved past deadline: source=" + ts.AmbiguitySource + " native=" + ts.NativeState
					r.writeHistory(rolloutID, targetID, "ambiguous", "failed", "", "", msg, rawProvider, "", "ambiguity-expiry")

					// Phase 3.4: escalate to error after threshold consecutive timeouts.
					if consecutive >= ambiguityEscalationThreshold {
						log.Printf("[AMBIGUITY_ESCALATION] target=%s source=%s consecutive=%d — escalating to error",
							targetID, ts.AmbiguitySource, consecutive)
						escalateMsg := fmt.Sprintf("ambiguity escalated after %d consecutive timeout observations; source=%s native=%s",
							consecutive, ts.AmbiguitySource, ts.NativeState)
						r.writeHistory(rolloutID, targetID, "ambiguous", "failed", "", "", escalateMsg, rawProvider, "", "ambiguity-escalation")
						// Force target to error; clear all ambiguity and counter.
						now := time.Now().UTC().Format(time.RFC3339)
						rec.Set("lifecycle_ambiguous", false)
						rec.Set("lifecycle_native_state", "")
						rec.Set("lifecycle_ambiguity_source", "")
						rec.Set("lifecycle_ambiguity_detail", "")
						rec.Set("lifecycle_ambiguity_deadline", nil)
						rec.Set("lifecycle_ambiguity_consecutive_timeouts", 0)
						rec.Set("status", "error")
						rec.Set("last_state_change_at", now)
						rec.Set("last_synced", now)
						if err := r.app.Dao().SaveRecord(rec); err != nil {
							log.Printf("[AMBIGUITY_ESCALATION_ERR] target=%s save: %v", targetID, err)
						}
						return
					}
				}
				// Keep the original deadline; don't extend it.
				deadline = parsed
			}
		}
		rec.Set("lifecycle_ambiguous", true)
		rec.Set("lifecycle_native_state", ts.NativeState)
		rec.Set("lifecycle_ambiguity_source", ts.AmbiguitySource)
		rec.Set("lifecycle_ambiguity_detail", ts.AmbiguityDetail)
		rec.Set("lifecycle_ambiguity_deadline", deadline.UTC().Format(time.RFC3339))
		log.Printf("[AMBIGUITY_SET] target=%s source=%s native_state=%s deadline=%s",
			targetID, ts.AmbiguitySource, ts.NativeState, deadline.UTC().Format(time.RFC3339))
	} else {
		// Ambiguity resolved: clear all state including escalation counter.
		rec.Set("lifecycle_ambiguous", false)
		rec.Set("lifecycle_native_state", "")
		rec.Set("lifecycle_ambiguity_source", "")
		rec.Set("lifecycle_ambiguity_detail", "")
		rec.Set("lifecycle_ambiguity_deadline", nil)
		rec.Set("lifecycle_ambiguity_consecutive_timeouts", 0)
	}
	if err := r.app.Dao().SaveRecord(rec); err != nil {
		log.Printf("[AMBIGUITY_WRITE_ERR] target=%s save: %v", targetID, err)
	}
}

// noteSuspectError records that the provider returned an error observation
// while the target was `updating`. Returns true when the observation count
// reaches the per-provider suspicion threshold, derived from the provider's
// CapEventualConsistency.UncertaintyP99 via suspicionThreshold() (Phase 3
// Change 6). ECS (30s uncertainty) requires more observations than Cloud Run
// (5s uncertainty) before the reconciler commits to `error`.
//
// Called only from the status-poll path. Cleared by clearSuspect on any
// observation that contradicts the suspicion (e.g., `updating → running`,
// `updating → updating`).
func (r *Reconciler) noteSuspectError(targetID, providerName string) bool {
	r.suspicionMu.Lock()
	defer r.suspicionMu.Unlock()
	r.suspicions[targetID]++
	threshold := suspicionThreshold(providerName, r.config.Interval)
	return r.suspicions[targetID] >= threshold
}

// clearSuspect resets the suspicion counter for a target. Called whenever
// a non-error observation occurs so a transient flap does not accumulate
// across unrelated cycles.
func (r *Reconciler) clearSuspect(targetID string) {
	r.suspicionMu.Lock()
	defer r.suspicionMu.Unlock()
	delete(r.suspicions, targetID)
}

// noteStaleSucceeded records a `succeeded_stale` outcome for a target
// and returns the new count. Phase 2.6: when count reaches
// staleThreshold the dispatcher releases to `error` instead of
// `pending`. See [[phase2.6/succeeded-stale-guard.md]].
func (r *Reconciler) noteStaleSucceeded(targetID string) int {
	r.staleCountMu.Lock()
	defer r.staleCountMu.Unlock()
	r.staleCount[targetID]++
	return r.staleCount[targetID]
}

// clearStaleCount resets the consecutive succeeded_stale count for a
// target. Called on any non-stale outcome and on operator-recovery
// entry to dispatch (Phase 2.4 in-memory state clearing).
func (r *Reconciler) clearStaleCount(targetID string) {
	r.staleCountMu.Lock()
	defer r.staleCountMu.Unlock()
	delete(r.staleCount, targetID)
}

// providerToProviderName maps PocketBase provider to Provider interface name
func providerToProviderName(provider string) string {
	switch provider {
	case "gcp":
		return providers.ProviderGCPCloudRun
	case "aws":
		return providers.ProviderAWSECS
	case "azure":
		return providers.ProviderAzureACA
	default:
		return ""
	}
}

// sanitizeError removes potentially sensitive information from error messages
func sanitizeError(errMsg string) string {
	// List of sensitive patterns to redact
	sensitivePatterns := []string{
		"credential",
		"secret",
		"token",
		"key",
		"password",
		"private_key",
		"service_account",
		"access_key",
		"api_key",
	}

	result := errMsg
	for _, pattern := range sensitivePatterns {
		// Replace anything that looks like a credential value
		// This is a simple approach - in production, use structured logging
		if strings.Contains(strings.ToLower(result), pattern) {
			// Truncate to prevent leaking context
			if len(result) > 200 {
				result = result[:200] + "... [REDACTED]"
			}
			break
		}
	}

	// Limit total length
	if len(result) > 500 {
		result = result[:500] + "..."
	}

	return result
}

// StartReconcilerOnBoot registers the reconciler to start when PocketBase boots
// Call this from main.go after app.Bootstrap() completes
func StartReconcilerOnBoot(app *pb.PocketBase) {
	reconciler := New(app, DefaultConfig())
	reconciler.Start()
}

// promoteToDeleting flips a target from `error` to `deleting` so the
// dispatcher's next cycle picks it up via shouldDispatchDestroy. Phase 2.4
// H-1: closes the "error + pending_destroy + endDate-still-set" stuck state
// where an operator's in-flight destroy intent was preserved on the flag
// but no path consumed it.
//
// Best-effort; failures logged. Status transition itself bypasses the
// transition guard (which would refuse error → deleting); this is
// intentional, since the operator's intent (endDate set) authorizes the
// destroy regardless of the intermediate error state.
func (r *Reconciler) promoteToDeleting(targetID string) {
	rec, err := r.app.Dao().FindRecordById("deployment_targets", targetID)
	if err != nil {
		log.Printf("[CLOUD_DESTROY_AUTO_PROMOTE_ERR] target=%s find: %v", targetID, err)
		return
	}
	rec.Set("status", "deleting")
	rec.Set("last_state_change_at", time.Now().UTC().Format(time.RFC3339))
	rec.Set("drift_summary", "auto-promoted from error to honor pending destroy intent")
	if err := r.app.Dao().SaveRecord(rec); err != nil {
		log.Printf("[CLOUD_DESTROY_AUTO_PROMOTE_ERR] target=%s save: %v", targetID, err)
	}
}

// readBoolColumn coerces a PocketBase-returned column value to a bool. dbx's
// underlying driver may return bool, integer (0/1), or string ("true"/"false")
// depending on column type and adapter; rather than guess, we accept all.
func readBoolColumn(v interface{}) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case string:
		return x == "true" || x == "1"
	case []byte:
		s := string(x)
		return s == "true" || s == "1"
	default:
		return false
	}
}