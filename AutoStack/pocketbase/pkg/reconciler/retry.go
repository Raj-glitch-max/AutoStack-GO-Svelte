package reconciler

import (
	"fmt"
	"log"
	"time"

	"github.com/janlauber/one-click/pkg/providers"
	"github.com/pocketbase/dbx"
)

// Phase 3.9.3 — Retry Decision Engine.
//
// The classifier (pkg/providers/normalize.go) decides WHAT a failure is.
// This file decides WHAT TO DO about it. The split is intentional:
//
//   - Providers classify (observation producer).
//   - The reconciler decides (policy owner).
//
// No retry decision in the runtime happens outside this engine. The
// previous codebase had retry logic scattered across dispatch.go (via
// FailureCategory) and cloud.go (via ClassifyError); those paths now
// all funnel through Decide().

// ─────────────────────────────────────────────────────────────────────────
// RETRY DECISION (RECONCILER VIEW)
// ─────────────────────────────────────────────────────────────────────────

// RetryAction is the discrete decision the engine returns.
type RetryAction string

const (
	// RetryActionFail — give up; record as failed.
	// The operator must intervene.
	RetryActionFail RetryAction = "fail"

	// RetryActionRetry — schedule a retry after the returned delay.
	// Caller MUST persist retry_count and retry_backoff_until.
	RetryActionRetry RetryAction = "retry"

	// RetryActionEscalateAmbiguity — outcome is unknown; route through
	// the ambiguity escalation path instead of normal retry.
	RetryActionEscalateAmbiguity RetryAction = "escalate_ambiguity"

	// RetryActionOperatorAction — retries cannot resolve; operator must act.
	// Distinguished from Fail because the runtime should NOT burn circuit-breaker
	// budget on operator-only failures (auth, quota).
	RetryActionOperatorAction RetryAction = "operator_action"
)

// RetryEngineDecision is the engine's authoritative output.
type RetryEngineDecision struct {
	Action          RetryAction
	NextAttemptAt   time.Time     // when to retry (zero if not retrying)
	BackoffDelay    time.Duration // delay applied (for forensics)
	BudgetRemaining int           // retries left after this decision (informational)
	Class           providers.RetryClass
	Reason          string // human-readable explanation for the chosen action
}

// ─────────────────────────────────────────────────────────────────────────
// DECISION ENGINE
// ─────────────────────────────────────────────────────────────────────────

// DecideRetry is the canonical retry decision entrypoint.
//
// Inputs:
//   - decision: the classifier's output (class + native code + ambiguity)
//   - attemptCount: how many times THIS operation has already been retried
//   - leaseRemaining: how much lease time is left for the current pod
//   - leaseValid: whether the current pod still holds the lease at all
//
// Outputs:
//   - RetryEngineDecision with Action + scheduling info
//
// Decision rules (in priority order):
//
//   1. If lease is invalid → RetryActionFail (no authority to retry)
//   2. If class.OperatorActionRequired and not retryable → RetryActionOperatorAction
//   3. If class.Retryable is false → RetryActionFail
//   4. If attemptCount >= policy.MaxRetries → RetryActionFail (budget exhausted)
//   5. If class.Ambiguous AND escalate-eligible → RetryActionEscalateAmbiguity
//   6. Otherwise: RetryActionRetry with computed backoff
//
// The engine NEVER decides "retry forever"; every retry path has a budget.
func DecideRetry(decision providers.RetryDecision, attemptCount int, leaseRemaining time.Duration, leaseValid bool) RetryEngineDecision {
	policy := decision.Class.Policy()

	// Rule 1: lease loss takes precedence. No authority → no retry.
	if !leaseValid {
		return RetryEngineDecision{
			Action: RetryActionFail,
			Class:  decision.Class,
			Reason: "lease invalid; retry authority absent",
		}
	}

	// Rule 2: operator-action classes that do NOT burn breaker budget
	// (auth, quota, rate_limit) → RetryActionOperatorAction. These are
	// truly operator-only failures: no AutoStack action helps until the
	// operator rotates creds, raises quota, or waits out the rate limit.
	//
	// Classes with OperatorActionRequired=true AND CircuitBreakerSeverity>0
	// (never, internal, unknown) → fall through to Rule 3 (Fail) because
	// they SHOULD burn budget — they represent dispatch failures the operator
	// must surface, not background operator-action waits.
	if policy.OperatorActionRequired && !policy.Retryable && policy.CircuitBreakerSeverity == 0 {
		return RetryEngineDecision{
			Action: RetryActionOperatorAction,
			Class:  decision.Class,
			Reason: fmt.Sprintf("%s requires operator action; not retryable", decision.Class),
		}
	}

	// Rule 3: terminal non-retryable. Burns circuit-breaker budget per policy.
	if !policy.Retryable {
		return RetryEngineDecision{
			Action: RetryActionFail,
			Class:  decision.Class,
			Reason: fmt.Sprintf("%s is non-retryable; %s", decision.Class, decision.ForensicReason),
		}
	}

	// Rule 4: budget exhaustion.
	if attemptCount >= policy.MaxRetries {
		return RetryEngineDecision{
			Action:          RetryActionFail,
			Class:           decision.Class,
			BudgetRemaining: 0,
			Reason:          fmt.Sprintf("retry budget exhausted (attempt %d >= max %d)", attemptCount, policy.MaxRetries),
		}
	}

	// Rule 5: ambiguity escalation routing.
	// Provider-uncertain classes contribute to the ambiguity escalation
	// counter. The reconciler may still retry, but the escalation path
	// runs in parallel (set by the caller via the ambiguity columns).
	// We signal the caller via the dedicated action so they know to
	// increment the ambiguity counter alongside scheduling the retry.
	if policy.EscalateAmbiguity && policy.Retryable && attemptCount >= policy.MaxRetries-1 {
		// Last retry attempt for an ambiguous class — route to escalation.
		return RetryEngineDecision{
			Action: RetryActionEscalateAmbiguity,
			Class:  decision.Class,
			Reason: fmt.Sprintf("ambiguous outcome after %d attempts; escalating", attemptCount),
		}
	}

	// Rule 6: normal retry with exponential backoff.
	delay := computeBackoff(policy, attemptCount, decision.SuggestedDelay)
	// Lease-aware bound: if the lease will expire before the backoff
	// completes, we cannot guarantee retry authority. Fail safe.
	if leaseRemaining > 0 && delay > leaseRemaining {
		return RetryEngineDecision{
			Action: RetryActionFail,
			Class:  decision.Class,
			Reason: fmt.Sprintf("backoff %s exceeds lease remaining %s; fail to release lease cleanly", delay, leaseRemaining),
		}
	}
	return RetryEngineDecision{
		Action:          RetryActionRetry,
		Class:           decision.Class,
		NextAttemptAt:   time.Now().Add(delay),
		BackoffDelay:    delay,
		BudgetRemaining: policy.MaxRetries - attemptCount - 1,
		Reason:          fmt.Sprintf("%s; attempt %d of %d", decision.ForensicReason, attemptCount+1, policy.MaxRetries),
	}
}

// computeBackoff is the deterministic backoff function.
// No jitter — Phase 3.9.3 prefers determinism (reproducible incident timelines)
// over thundering-herd mitigation. The retry budget caps the worst-case load.
//
// Formula: delay = min(MaxBackoff, BaseBackoff * 2^attempt).
// If the provider suggested a delay (SuggestedDelay) and it's larger than
// our computed delay, honor the larger one (provider knows its rate limit).
func computeBackoff(policy providers.RetryPolicy, attempt int, suggestedDelay time.Duration) time.Duration {
	if policy.BaseBackoff <= 0 {
		return 0
	}
	delay := policy.BaseBackoff
	for i := 0; i < attempt && delay < policy.MaxBackoff; i++ {
		delay *= 2
	}
	if delay > policy.MaxBackoff {
		delay = policy.MaxBackoff
	}
	if suggestedDelay > delay {
		// Provider's hint exceeds our policy; honor it (clamp to MaxBackoff).
		if suggestedDelay > policy.MaxBackoff {
			return policy.MaxBackoff
		}
		return suggestedDelay
	}
	return delay
}

// ─────────────────────────────────────────────────────────────────────────
// PERSISTENT RETRY STATE
// ─────────────────────────────────────────────────────────────────────────

// recordRetryState writes the canonical retry metadata to operations.
// Called from dispatch paths after every failure classification.
//
// Persistence is intentional: a process crash must not "forget" how many
// times an operation has been retried. The retry budget survives restart.
//
// Best-effort: errors are logged but do not propagate. The operation row
// remains writeable by other code paths.
func (r *Reconciler) recordRetryState(opID string, class providers.RetryClass, attempt int, backoffUntil time.Time, nativeCode, forensicReason string) {
	if opID == "" {
		return
	}
	backoffStr := ""
	if !backoffUntil.IsZero() {
		backoffStr = backoffUntil.UTC().Format(time.RFC3339)
	}
	_, err := r.app.Dao().DB().NewQuery(
		`UPDATE operations
		 SET retry_count = {:count},
		     retry_class = {:class},
		     retry_backoff_until = {:until},
		     retry_native_code = {:native},
		     retry_forensic_reason = {:reason},
		     last_retry_at = {:now},
		     updated_at = {:now}
		 WHERE id = {:id}`,
	).Bind(dbx.Params{
		"count":  attempt,
		"class":  string(class),
		"until":  backoffStr,
		"native": nativeCode,
		"reason": forensicReason,
		"now":    time.Now().UTC().Format(time.RFC3339),
		"id":     opID,
	}).Execute()
	if err != nil {
		log.Printf("[RETRY_STATE_WRITE_ERR] op=%s err=%v", opID, err)
	}
}

// loadRetryCount reads operations.retry_count for replay-safe budget
// enforcement. Returns 0 for a fresh operation or on DB error (fail-open
// reading; the next retry decision will still be bounded by MaxRetries).
func (r *Reconciler) loadRetryCount(opID string) int {
	var rows []struct {
		RetryCount int `db:"retry_count"`
	}
	err := r.app.Dao().DB().
		Select("retry_count").
		From("operations").
		Where(dbx.NewExp("id = {:id}", dbx.Params{"id": opID})).
		All(&rows)
	if err != nil || len(rows) == 0 {
		return 0
	}
	return rows[0].RetryCount
}

// ─────────────────────────────────────────────────────────────────────────
// CIRCUIT BREAKER (UNIFIED ON RETRYCLASS)
// ─────────────────────────────────────────────────────────────────────────

// recordTargetFailureWithClass is the Phase 3.9.3 replacement for
// recordTargetFailureWithCategory. The decision to increment the circuit
// breaker is driven by the class's CircuitBreakerSeverity policy field:
//
//   severity 0 → do not increment (auth/quota/rate-limit — operator concerns)
//   severity 1 → increment by 1 (normal failure)
//   severity 2 → increment by 2 (provider-uncertain — trip faster)
func (r *Reconciler) recordTargetFailureWithClass(targetID string, class providers.RetryClass) {
	policy := class.Policy()
	if policy.CircuitBreakerSeverity == 0 {
		log.Printf("Target %s: %s failure - not incrementing circuit (operator action required: %v)",
			targetID, class, policy.OperatorActionRequired)
		return
	}

	r.failureMu.Lock()
	r.failures[targetID] += policy.CircuitBreakerSeverity
	r.failureMu.Unlock()

	r.recordError()
}

// ─────────────────────────────────────────────────────────────────────────
// RUNTIME RETRY SCHEDULING
// ─────────────────────────────────────────────────────────────────────────

// scheduleRetry persists a retry schedule on the target. After this call:
//   - target.status = "retry_pending"
//   - target.next_retry_at = engine.NextAttemptAt
//   - target.consecutive_failures incremented
//
// The reconcile loop reads next_retry_at on every tick. While it is in the
// future, the target is skipped (returns reconcileSkipped). When the time
// elapses, the reconciler flips retry_pending → pending so dispatch
// eligibility activates.
//
// CRITICAL: scheduling a retry is a forensic action. We write a history
// row so the timeline reflects "retry scheduled at T, due at T+backoff".
//
// Best-effort persistence: errors are logged but do not propagate. A
// failure here means the target stays in its current status; the next
// reconcile tick may re-evaluate and reach a terminal outcome instead.
func (r *Reconciler) scheduleRetry(targetID, rolloutID, opID, fromStatus, action, targetProvider string, engineDecision RetryEngineDecision) {
	if targetID == "" || engineDecision.NextAttemptAt.IsZero() {
		log.Printf("[RETRY_SCHEDULE_SKIP] target=%s reason=invalid_decision", targetID)
		return
	}
	nextRetryStr := engineDecision.NextAttemptAt.UTC().Format(time.RFC3339)
	now := time.Now().UTC().Format(time.RFC3339)

	// Update deployment_targets row. We avoid a FindRecordById + SaveRecord
	// roundtrip in favor of a single UPDATE that keeps the transition
	// atomic relative to other writers.
	_, err := r.app.Dao().DB().NewQuery(
		`UPDATE deployment_targets
		 SET status = 'retry_pending',
		     next_retry_at = {:next},
		     consecutive_failures = COALESCE(consecutive_failures, 0) + 1,
		     current_operation = '',
		     last_state_change_at = {:now},
		     last_synced = {:now}
		 WHERE id = {:id}`,
	).Bind(dbx.Params{
		"next": nextRetryStr,
		"now":  now,
		"id":   targetID,
	}).Execute()
	if err != nil {
		log.Printf("[RETRY_SCHEDULE_ERR] target=%s op=%s err=%v", targetID, opID, err)
		return
	}

	// Forensic history row: "retry scheduled".
	msg := fmt.Sprintf("retry scheduled in %s (class=%s budget_remaining=%d)",
		engineDecision.BackoffDelay, engineDecision.Class, engineDecision.BudgetRemaining)
	r.writeHistory(rolloutID, targetID, action, "failed", "", "", msg, targetProvider, opID, "retry-scheduled-"+string(engineDecision.Class))

	if r.metrics != nil {
		r.metrics.IncRetryScheduled(engineDecision.Class)
	}
	log.Printf("[RETRY_SCHEDULED] target=%s op=%s class=%s due_at=%s delay=%s budget_remaining=%d",
		targetID, opID, engineDecision.Class, nextRetryStr, engineDecision.BackoffDelay, engineDecision.BudgetRemaining)
}

// clearRetrySchedule resets the retry state on success. Called from the
// dispatch success paths after a successful deploy/destroy/rollback.
//
// Resets consecutive_failures to 0 and clears next_retry_at. Subsequent
// failures start fresh from the bottom of the backoff curve.
func (r *Reconciler) clearRetrySchedule(targetID string) {
	if targetID == "" {
		return
	}
	_, err := r.app.Dao().DB().NewQuery(
		`UPDATE deployment_targets
		 SET next_retry_at = NULL,
		     consecutive_failures = 0
		 WHERE id = {:id}`,
	).Bind(dbx.Params{"id": targetID}).Execute()
	if err != nil {
		log.Printf("[RETRY_CLEAR_ERR] target=%s err=%v", targetID, err)
	}
}

// activateRetryEligibility flips a retry_pending target to pending when
// its backoff has elapsed. Called from reconcileOne on the eligibility
// check.
//
// Idempotent: a 0-row update means another tick already activated it.
func (r *Reconciler) activateRetryEligibility(targetID string) {
	if targetID == "" {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.app.Dao().DB().NewQuery(
		`UPDATE deployment_targets
		 SET status = 'pending',
		     last_state_change_at = {:now}
		 WHERE id = {:id}
		   AND status = 'retry_pending'
		   AND (next_retry_at IS NULL OR next_retry_at <= {:now})`,
	).Bind(dbx.Params{
		"now": now,
		"id":  targetID,
	}).Execute()
	if err != nil {
		log.Printf("[RETRY_ACTIVATE_ERR] target=%s err=%v", targetID, err)
		return
	}
	log.Printf("[RETRY_ACTIVATED] target=%s — backoff elapsed; eligible for dispatch", targetID)
}

// ─────────────────────────────────────────────────────────────────────────
// LEASE-AWARE DECISION HELPER
// ─────────────────────────────────────────────────────────────────────────

// resolveRetryDecision is the single canonical entrypoint used by all
// dispatch failure paths. It:
//   1. Classifies the error via the provider (with optional override)
//   2. Loads the persisted retry budget from the operation row
//   3. Computes lease remaining time (if leaseEpoch > 0)
//   4. Calls DecideRetry for the canonical action
//
// Returns the engine decision + the classifier decision (callers need
// both for history/metrics).
func (r *Reconciler) resolveRetryDecision(opID string, p providers.Provider, providerErr error, leaseRemaining time.Duration, leaseValid bool) (providers.RetryDecision, RetryEngineDecision) {
	classDecision := classifyWithProviderOverride(p, providerErr)
	attemptCount := r.loadRetryCount(opID)
	engineDecision := DecideRetry(classDecision, attemptCount, leaseRemaining, leaseValid)
	return classDecision, engineDecision
}

// applyRetryAction is the single dispatch routing function. Given the
// engine's RetryEngineDecision, it performs ONE of:
//
//   - schedule a retry (status → retry_pending; next_retry_at set)
//   - escalate ambiguity (status → ambiguous; lifecycle_ambiguity_* set;
//     escalation counter incremented)
//   - terminal fail (status → error; operator must act)
//   - operator action (status → error; no breaker burn — already handled
//     by recordTargetFailureWithClass)
//
// The caller has already:
//   - completed the operation row
//   - persisted retry state via recordRetryState
//   - incremented the circuit breaker via recordTargetFailureWithClass
//
// applyRetryAction is the LAST step: it transitions the target's status
// to its canonical post-failure state and writes the history row that
// reflects the engine's decision.
//
// fromStatus is the in-flight status the dispatch path was in (creating,
// deleting, rolling_back). releaseTarget uses this as the CAS guard.
//
// Phase 3.9.3 Runtime Integration: this is the function that closes the
// "release-and-wait-for-tick" gap. Every dispatch failure now has an
// EXPLICIT post-failure status based on engine decision, not an implicit
// "go to error and hope".
func (r *Reconciler) applyRetryAction(engineDecision RetryEngineDecision, classDecision providers.RetryDecision, opID, rolloutID, targetID, historyAction, targetProvider, fromStatus, msg, cycleID string) {
	switch engineDecision.Action {
	case RetryActionRetry:
		// Schedule a retry. The target moves through retry_pending and the
		// reconcile loop will re-dispatch when next_retry_at elapses.
		// The releaseTarget call here transitions creating→retry_pending
		// via the lifecycle_guards table (Phase 3.9.3 transition added).
		r.releaseTarget(opID, targetID, fromStatus, "retry_pending", msg)
		r.scheduleRetry(targetID, rolloutID, opID, fromStatus, historyAction, targetProvider, engineDecision)
		// Phase 3.9.6: persist the decision for narrative reconstruction.
		r.RecordDecision(DecisionRecord{
			TargetID:       targetID,
			RolloutID:      rolloutID,
			OperationID:    opID,
			CycleID:        cycleID,
			DecisionType:   DecisionRetryScheduled,
			DecisionReason: "transient_or_recoverable_class_within_budget",
			HumanSummary:   "retry scheduled in " + engineDecision.BackoffDelay.String() + " (class=" + string(classDecision.Class) + ", budget_remaining=" + intToStr(engineDecision.BudgetRemaining) + ")",
			DecisionInputs: map[string]interface{}{
				"class":            string(classDecision.Class),
				"attempt_count":    r.loadRetryCount(opID),
				"budget_remaining": engineDecision.BudgetRemaining,
				"backoff_delay_ms": engineDecision.BackoffDelay.Milliseconds(),
				"native_code":      classDecision.NativeCode,
				"forensic_reason":  classDecision.ForensicReason,
			},
			EvidenceRefs: []EvidenceRef{{Collection: "operations", ID: opID}},
			ClassDecision: &classDecision,
			LeaseValid:    true,
		})

	case RetryActionEscalateAmbiguity:
		// Provider-uncertain class on last attempt — escalate to ambiguity.
		// The target moves to ambiguous; the ambiguity columns are written
		// so the next GetStatus tick observes the existing escalation.
		now := time.Now().UTC().Format(time.RFC3339)
		// Set ambiguity columns directly via UPDATE.
		_, err := r.app.Dao().DB().NewQuery(
			`UPDATE deployment_targets
			 SET lifecycle_ambiguous = 1,
			     lifecycle_ambiguity_source = 'S-4',
			     lifecycle_ambiguity_detail = {:detail},
			     lifecycle_ambiguity_consecutive_timeouts = COALESCE(lifecycle_ambiguity_consecutive_timeouts, 0) + 1,
			     last_state_change_at = {:now}
			 WHERE id = {:id}`,
		).Bind(dbx.Params{
			"detail": "retry exhausted on ambiguous outcome: " + classDecision.ForensicReason,
			"now":    now,
			"id":     targetID,
		}).Execute()
		if err != nil {
			log.Printf("[RETRY_ESCALATE_AMBIGUITY_ERR] target=%s err=%v", targetID, err)
		}
		r.releaseTarget(opID, targetID, fromStatus, "ambiguous", "retry exhausted; outcome uncertain")
		r.writeHistory(rolloutID, targetID, historyAction, "failed", "", "", "retry exhausted on ambiguous class", targetProvider, opID, "retry-exhausted-ambiguity")
		if r.metrics != nil {
			r.metrics.IncAmbiguityFromRetry()
			r.metrics.IncRetryExhausted(classDecision.Class)
		}
		// Phase 3.9.4: open or escalate an incident — outcome unknown,
		// operator must investigate.
		runtimeState := IncidentRuntimeState{
			RetryClass: string(classDecision.Class),
			RetryCount: r.loadRetryCount(opID),
			NativeProviderCode: classDecision.NativeCode,
			AmbiguitySource: "S-4",
		}
		incID, _ := r.CreateOrEscalateIncident(targetID, rolloutID, IncidentSourceAmbiguityEscalation,
			IncidentSeverityHigh, opID,
			"retry exhausted on ambiguous class: "+classDecision.ForensicReason,
			runtimeState)
		// Phase 3.9.6: persist the escalation decision.
		evRefs := []EvidenceRef{{Collection: "operations", ID: opID}}
		if incID != "" {
			evRefs = append(evRefs, EvidenceRef{Collection: "incidents", ID: incID})
		}
		r.RecordDecision(DecisionRecord{
			TargetID:       targetID,
			RolloutID:      rolloutID,
			OperationID:    opID,
			CycleID:        cycleID,
			DecisionType:   DecisionRetryEscalatedAmbiguity,
			DecisionReason: "ambiguous_outcome_retry_budget_reached",
			HumanSummary:   "retry exhausted on ambiguous class " + string(classDecision.Class) + "; outcome uncertain — escalated to ambiguity",
			DecisionInputs: map[string]interface{}{
				"class":           string(classDecision.Class),
				"attempt_count":   r.loadRetryCount(opID),
				"native_code":     classDecision.NativeCode,
				"forensic_reason": classDecision.ForensicReason,
			},
			EvidenceRefs:  evRefs,
			ClassDecision: &classDecision,
			LeaseValid:    true,
		})
		log.Printf("[RETRY_ESCALATED_AMBIGUITY] target=%s op=%s class=%s", targetID, opID, classDecision.Class)

	case RetryActionOperatorAction:
		// Auth / quota / rate-limit. No breaker burn (handled above).
		// Status → error with explicit "operator action required" marker.
		r.releaseTarget(opID, targetID, fromStatus, "error", msg+" [operator action required: "+string(classDecision.Class)+"]")
		r.writeHistory(rolloutID, targetID, historyAction, "failed", "", "", msg, targetProvider, opID, "operator-action-"+string(classDecision.Class))
		if r.metrics != nil {
			r.metrics.IncRetryOperatorAction()
		}
		// Phase 3.9.4: open incident for operator-action-required failures.
		// Severity is High for auth/quota (operator must intervene); Medium
		// for rate_limit (often self-resolving but still operationally meaningful).
		sev := severityForRetryClass(classDecision.Class)
		if classDecision.Class == providers.RetryClassRateLimit {
			sev = IncidentSeverityMedium
		}
		incID2, _ := r.CreateOrEscalateIncident(targetID, rolloutID, IncidentSourceRetryExhausted,
			sev, opID,
			"operator action required: "+string(classDecision.Class)+" — "+classDecision.ForensicReason,
			IncidentRuntimeState{
				RetryClass: string(classDecision.Class),
				NativeProviderCode: classDecision.NativeCode,
			})
		// Phase 3.9.6: persist the operator-action decision.
		evRefs2 := []EvidenceRef{{Collection: "operations", ID: opID}}
		if incID2 != "" {
			evRefs2 = append(evRefs2, EvidenceRef{Collection: "incidents", ID: incID2})
		}
		r.RecordDecision(DecisionRecord{
			TargetID:       targetID,
			RolloutID:      rolloutID,
			OperationID:    opID,
			CycleID:        cycleID,
			DecisionType:   DecisionRetryOperatorAction,
			DecisionReason: "operator_intervention_required_" + string(classDecision.Class),
			HumanSummary:   "retries cannot resolve; operator action required (class=" + string(classDecision.Class) + ")",
			DecisionInputs: map[string]interface{}{
				"class":       string(classDecision.Class),
				"native_code": classDecision.NativeCode,
				"reason":      engineDecision.Reason,
			},
			EvidenceRefs:  evRefs2,
			ClassDecision: &classDecision,
			LeaseValid:    true,
		})
		log.Printf("[RETRY_OPERATOR_ACTION] target=%s op=%s class=%s reason=%s", targetID, opID, classDecision.Class, engineDecision.Reason)

	case RetryActionFail:
		// Terminal failure — either budget exhausted, non-retryable class,
		// or lease invalid. Status → error; operator must investigate.
		r.releaseTarget(opID, targetID, fromStatus, "error", msg+" [retry exhausted: "+engineDecision.Reason+"]")
		r.writeHistory(rolloutID, targetID, historyAction, "failed", "", "", msg, targetProvider, opID, "retry-exhausted-"+string(classDecision.Class))
		if r.metrics != nil {
			r.metrics.IncRetryExhausted(classDecision.Class)
		}
		// Phase 3.9.4: open incident for terminal failure.
		// Source distinguishes lease loss from generic exhaustion so
		// operators see the cause class at a glance.
		incidentSource := IncidentSourceRetryExhausted
		incidentSummary := "retry exhausted: " + engineDecision.Reason
		if engineDecision.Reason == "lease invalid; retry authority absent" {
			if r.metrics != nil {
				r.metrics.IncRetrySuppressedLease()
			}
			incidentSource = IncidentSourceLeaseLossMidCall
			incidentSummary = "lease lost during dispatch; provider outcome unknown"
		}
		incID3, _ := r.CreateOrEscalateIncident(targetID, rolloutID, incidentSource,
			severityForRetryClass(classDecision.Class), opID,
			incidentSummary,
			IncidentRuntimeState{
				RetryClass: string(classDecision.Class),
				RetryCount: r.loadRetryCount(opID),
				NativeProviderCode: classDecision.NativeCode,
			})
		// Phase 3.9.6: distinguish lease-invalid failure from budget exhaustion
		// in the persisted decision so operators can filter by cause.
		failDecisionType := DecisionRetryExhausted
		failDecisionReason := "retry_budget_exhausted"
		if engineDecision.Reason == "lease invalid; retry authority absent" {
			failDecisionType = DecisionRetryRefusedLeaseInvalid
			failDecisionReason = "lease_invalid_at_decision_time"
		} else if !classDecision.Class.IsRetriable() {
			failDecisionType = DecisionRetryRefusedNonRetryable
			failDecisionReason = "non_retryable_class_" + string(classDecision.Class)
		}
		evRefs3 := []EvidenceRef{{Collection: "operations", ID: opID}}
		if incID3 != "" {
			evRefs3 = append(evRefs3, EvidenceRef{Collection: "incidents", ID: incID3})
		}
		r.RecordDecision(DecisionRecord{
			TargetID:       targetID,
			RolloutID:      rolloutID,
			OperationID:    opID,
			CycleID:        cycleID,
			DecisionType:   failDecisionType,
			DecisionReason: failDecisionReason,
			HumanSummary:   "retry stopped: " + engineDecision.Reason,
			DecisionInputs: map[string]interface{}{
				"class":         string(classDecision.Class),
				"attempt_count": r.loadRetryCount(opID),
				"native_code":   classDecision.NativeCode,
				"reason":        engineDecision.Reason,
			},
			EvidenceRefs:  evRefs3,
			ClassDecision: &classDecision,
			LeaseValid:    failDecisionType != DecisionRetryRefusedLeaseInvalid,
		})
		log.Printf("[RETRY_EXHAUSTED] target=%s op=%s class=%s reason=%s", targetID, opID, classDecision.Class, engineDecision.Reason)

	default:
		// Unknown action — fail-safe to error.
		log.Printf("[RETRY_UNKNOWN_ACTION] target=%s op=%s action=%s — failing to error",
			targetID, opID, engineDecision.Action)
		r.releaseTarget(opID, targetID, fromStatus, "error", msg+" [unknown retry action]")
	}
}

// classifyWithProviderOverride lets a provider refine the generic
// classification before it reaches the engine. Any object implementing
// providers.ErrorClassifier may return a non-nil RetryDecision to
// override; returning nil falls through to the generic classifier.
//
// Parameter is `any` (not providers.Provider) so the function can be
// unit-tested without building a full Provider mock — the only behavior
// under test is the type-assertion-and-fallback. Callers in dispatch.go
// pass a providers.Provider; Go's structural typing makes this work.
//
// This is the seam for provider-specific override hooks. Currently no
// Phase 3.1 provider implements the interface; the hook exists so ECS
// or Cloud Run can add typed-error handling without breaking the
// canonical pipeline.
func classifyWithProviderOverride(p any, err error) providers.RetryDecision {
	if classifier, ok := p.(providers.ErrorClassifier); ok {
		if override := classifier.ClassifyError(err); override != nil {
			return *override
		}
	}
	return providers.ClassifyProviderError(err)
}
