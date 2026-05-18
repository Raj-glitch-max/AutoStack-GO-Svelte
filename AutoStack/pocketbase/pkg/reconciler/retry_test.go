package reconciler

import (
	"testing"
	"time"

	"github.com/janlauber/one-click/pkg/providers"
)

// Phase 3.9.3 — DecideRetry engine tests.
//
// Validates the canonical decision logic without DB dependency:
//   - Lease invalidity overrides everything
//   - Non-retryable classes return Fail
//   - Operator-action classes return OperatorAction
//   - Budget exhaustion enforced
//   - Ambiguity escalation routing
//   - Backoff determinism
//   - Lease-aware bound (backoff cannot exceed lease)

func TestDecideRetry_LeaseInvalidOverrides(t *testing.T) {
	decision := providers.RetryDecision{Class: providers.RetryClassTransient}
	result := DecideRetry(decision, 0, time.Hour, false)
	if result.Action != RetryActionFail {
		t.Errorf("lease invalid: action = %s, want %s", result.Action, RetryActionFail)
	}
}

func TestDecideRetry_AuthIsOperatorAction(t *testing.T) {
	decision := providers.RetryDecision{Class: providers.RetryClassAuth}
	result := DecideRetry(decision, 0, time.Hour, true)
	if result.Action != RetryActionOperatorAction {
		t.Errorf("auth: action = %s, want %s", result.Action, RetryActionOperatorAction)
	}
}

func TestDecideRetry_QuotaIsOperatorAction(t *testing.T) {
	decision := providers.RetryDecision{Class: providers.RetryClassQuota}
	result := DecideRetry(decision, 0, time.Hour, true)
	if result.Action != RetryActionOperatorAction {
		t.Errorf("quota: action = %s, want %s", result.Action, RetryActionOperatorAction)
	}
}

func TestDecideRetry_NeverIsFail(t *testing.T) {
	decision := providers.RetryDecision{Class: providers.RetryClassNever}
	result := DecideRetry(decision, 0, time.Hour, true)
	if result.Action != RetryActionFail {
		t.Errorf("never: action = %s, want %s", result.Action, RetryActionFail)
	}
}

func TestDecideRetry_TransientRetries(t *testing.T) {
	decision := providers.RetryDecision{Class: providers.RetryClassTransient}
	result := DecideRetry(decision, 0, time.Hour, true)
	if result.Action != RetryActionRetry {
		t.Errorf("transient attempt 0: action = %s, want %s", result.Action, RetryActionRetry)
	}
	if result.BackoffDelay == 0 {
		t.Errorf("transient retry must have non-zero backoff, got %s", result.BackoffDelay)
	}
}

func TestDecideRetry_BudgetExhaustion(t *testing.T) {
	decision := providers.RetryDecision{Class: providers.RetryClassTransient}
	policy := providers.RetryClassTransient.Policy()
	// At MaxRetries, the next decision should be Fail.
	result := DecideRetry(decision, policy.MaxRetries, time.Hour, true)
	if result.Action != RetryActionFail {
		t.Errorf("budget exhausted: action = %s, want %s", result.Action, RetryActionFail)
	}
	if result.BudgetRemaining != 0 {
		t.Errorf("budget exhausted: BudgetRemaining = %d, want 0", result.BudgetRemaining)
	}
}

func TestDecideRetry_AmbiguityEscalation(t *testing.T) {
	// Provider-uncertain classes escalate on last retry.
	decision := providers.RetryDecision{Class: providers.RetryClassProviderUncertain}
	policy := providers.RetryClassProviderUncertain.Policy()
	// MaxRetries-1 is the trigger for escalation.
	result := DecideRetry(decision, policy.MaxRetries-1, time.Hour, true)
	if result.Action != RetryActionEscalateAmbiguity {
		t.Errorf("provider-uncertain at MaxRetries-1: action = %s, want %s",
			result.Action, RetryActionEscalateAmbiguity)
	}
}

func TestDecideRetry_LeaseTimeBound(t *testing.T) {
	decision := providers.RetryDecision{Class: providers.RetryClassRateLimit}
	// Lease has only 1 second remaining; rate-limit policy starts at 10s backoff.
	result := DecideRetry(decision, 0, 1*time.Second, true)
	if result.Action != RetryActionFail {
		t.Errorf("backoff > lease remaining: action = %s, want %s (fail-safe)", result.Action, RetryActionFail)
	}
}

func TestDecideRetry_BackoffDeterminism(t *testing.T) {
	decision := providers.RetryDecision{Class: providers.RetryClassTransient}
	first := DecideRetry(decision, 0, time.Hour, true).BackoffDelay
	second := DecideRetry(decision, 0, time.Hour, true).BackoffDelay
	if first != second {
		t.Errorf("backoff must be deterministic: first=%s second=%s", first, second)
	}
}

func TestDecideRetry_BackoffMonotonic(t *testing.T) {
	decision := providers.RetryDecision{Class: providers.RetryClassTransient}
	prev := time.Duration(0)
	for attempt := 0; attempt < 5; attempt++ {
		result := DecideRetry(decision, attempt, time.Hour, true)
		if result.BackoffDelay < prev {
			t.Errorf("backoff regressed: attempt=%d delay=%s prev=%s", attempt, result.BackoffDelay, prev)
		}
		prev = result.BackoffDelay
	}
}

func TestDecideRetry_BackoffCap(t *testing.T) {
	policy := providers.RetryClassTransient.Policy()
	delay := computeBackoff(policy, 100, 0) // attempt 100 — way past doubling
	if delay > policy.MaxBackoff {
		t.Errorf("backoff exceeded cap: %s > %s", delay, policy.MaxBackoff)
	}
}

func TestDecideRetry_SuggestedDelayHonored(t *testing.T) {
	policy := providers.RetryClassTransient.Policy()
	// Provider suggested 30s; our base is 2s. Honor the larger.
	delay := computeBackoff(policy, 0, 30*time.Second)
	if delay != 30*time.Second {
		t.Errorf("suggested delay 30s should be honored, got %s", delay)
	}
	// But cap at MaxBackoff.
	delay = computeBackoff(policy, 0, 10*time.Minute)
	if delay != policy.MaxBackoff {
		t.Errorf("suggested delay 10m should clamp to MaxBackoff %s, got %s", policy.MaxBackoff, delay)
	}
}
