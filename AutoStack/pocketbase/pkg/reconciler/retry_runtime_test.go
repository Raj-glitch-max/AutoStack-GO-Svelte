package reconciler

import (
	"errors"
	"testing"
	"time"

	"github.com/janlauber/one-click/pkg/providers"
)

// Phase 3.9.3 Runtime Integration tests.
//
// SCOPE STATEMENT (honest):
//
// These tests verify the PURE-LOGIC layer of the runtime integration:
//
//   - Decision engine routing produces correct RetryAction per class
//   - Backoff scheduling is bounded and deterministic
//   - Provider-override hook semantics
//
// They do NOT verify the DB-side effects of scheduleRetry /
// activateRetryEligibility / clearRetrySchedule. Those require a
// PocketBase test fixture which this codebase does not yet have for
// the reconciler package.
//
// Production validation depends on observing the new logs:
//   [RETRY_SCHEDULED]  — scheduleRetry path
//   [RETRY_ACTIVATED]  — backoff elapsed
//   [RETRY_EXHAUSTED]  — engine returned Fail
//   [RETRY_OPERATOR_ACTION] — engine returned OperatorAction
//   [RETRY_ESCALATED_AMBIGUITY] — last retry on ambiguous class

func TestRuntimeIntegration_TransientClassRoutesToRetry(t *testing.T) {
	decision := providers.RetryDecision{Class: providers.RetryClassTransient}
	engine := DecideRetry(decision, 0, time.Hour, true)
	if engine.Action != RetryActionRetry {
		t.Errorf("transient at attempt 0: action=%s, want %s", engine.Action, RetryActionRetry)
	}
	if engine.NextAttemptAt.IsZero() {
		t.Error("RetryActionRetry must set NextAttemptAt")
	}
	if engine.BackoffDelay <= 0 {
		t.Errorf("RetryActionRetry must set BackoffDelay > 0, got %s", engine.BackoffDelay)
	}
}

func TestRuntimeIntegration_AuthRoutesToOperatorAction(t *testing.T) {
	decision := providers.RetryDecision{Class: providers.RetryClassAuth}
	engine := DecideRetry(decision, 0, time.Hour, true)
	if engine.Action != RetryActionOperatorAction {
		t.Errorf("auth: action=%s, want %s", engine.Action, RetryActionOperatorAction)
	}
}

func TestRuntimeIntegration_NeverRoutesToFail(t *testing.T) {
	decision := providers.RetryDecision{Class: providers.RetryClassNever}
	engine := DecideRetry(decision, 0, time.Hour, true)
	if engine.Action != RetryActionFail {
		t.Errorf("never: action=%s, want %s", engine.Action, RetryActionFail)
	}
}

func TestRuntimeIntegration_ProviderUncertainEscalatesOnLastAttempt(t *testing.T) {
	decision := providers.RetryDecision{Class: providers.RetryClassProviderUncertain}
	policy := providers.RetryClassProviderUncertain.Policy()
	// First attempt: should retry.
	first := DecideRetry(decision, 0, time.Hour, true)
	if first.Action != RetryActionRetry {
		t.Errorf("provider_uncertain attempt 0: action=%s, want retry", first.Action)
	}
	// Last allowed attempt: should escalate.
	last := DecideRetry(decision, policy.MaxRetries-1, time.Hour, true)
	if last.Action != RetryActionEscalateAmbiguity {
		t.Errorf("provider_uncertain at MaxRetries-1: action=%s, want escalate_ambiguity", last.Action)
	}
}

func TestRuntimeIntegration_LeaseInvalidFailsBeforeAnythingElse(t *testing.T) {
	// Even transient + budget available should fail when lease is lost.
	decision := providers.RetryDecision{Class: providers.RetryClassTransient}
	engine := DecideRetry(decision, 0, time.Hour, false)
	if engine.Action != RetryActionFail {
		t.Errorf("lease invalid: action=%s, want fail (lease invalid overrides)", engine.Action)
	}
}

func TestRuntimeIntegration_BackoffExceedingLeaseFailsSafely(t *testing.T) {
	// Lease has only 1s; rate_limit base backoff is 10s.
	decision := providers.RetryDecision{Class: providers.RetryClassRateLimit}
	engine := DecideRetry(decision, 0, 1*time.Second, true)
	if engine.Action != RetryActionFail {
		t.Errorf("backoff > lease remaining: action=%s, want fail (fail-safe)", engine.Action)
	}
}

func TestRuntimeIntegration_BudgetExhaustionTerminates(t *testing.T) {
	decision := providers.RetryDecision{Class: providers.RetryClassTransient}
	policy := providers.RetryClassTransient.Policy()
	engine := DecideRetry(decision, policy.MaxRetries, time.Hour, true)
	if engine.Action != RetryActionFail {
		t.Errorf("budget exhausted: action=%s, want fail", engine.Action)
	}
	if engine.BudgetRemaining != 0 {
		t.Errorf("budget exhausted: BudgetRemaining=%d, want 0", engine.BudgetRemaining)
	}
}

func TestRuntimeIntegration_ProviderOverrideUsedWhenAvailable(t *testing.T) {
	// Build a mock provider that overrides classification.
	override := &providers.RetryDecision{
		Class:          providers.RetryClassConflict,
		ForensicReason: "provider-side typed-error override",
	}
	p := &mockProvider{classifyOverride: override}

	got := classifyWithProviderOverride(p, errors.New("403 forbidden"))
	if got.Class != providers.RetryClassConflict {
		t.Errorf("override should win: got class=%s, want %s", got.Class, providers.RetryClassConflict)
	}
	if got.ForensicReason != "provider-side typed-error override" {
		t.Errorf("override forensic reason lost: got %q", got.ForensicReason)
	}
}

func TestRuntimeIntegration_ProviderOverrideFallsThroughOnNil(t *testing.T) {
	// Provider returns nil — generic classifier should run and classify as Auth.
	p := &mockProvider{classifyOverride: nil}
	got := classifyWithProviderOverride(p, errors.New("403 forbidden"))
	if got.Class != providers.RetryClassAuth {
		t.Errorf("nil override should fall through: got %s, want %s", got.Class, providers.RetryClassAuth)
	}
}

func TestRuntimeIntegration_NonClassifierProviderFallsThrough(t *testing.T) {
	// Provider does not implement ErrorClassifier; generic classifier runs.
	p := &plainProvider{}
	got := classifyWithProviderOverride(p, errors.New("503 Service Unavailable"))
	if got.Class != providers.RetryClassTransient {
		t.Errorf("non-classifier provider: got %s, want %s", got.Class, providers.RetryClassTransient)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Mock providers for override test plumbing.
//
// classifyWithProviderOverride takes `any`, so we don't need a full
// providers.Provider mock — just types that the type-assertion can
// inspect for ErrorClassifier implementation.
// ─────────────────────────────────────────────────────────────────────────

type mockProvider struct {
	classifyOverride *providers.RetryDecision
}

func (m *mockProvider) ClassifyError(_ error) *providers.RetryDecision {
	return m.classifyOverride
}

// plainProvider intentionally does NOT implement ErrorClassifier.
type plainProvider struct{}
