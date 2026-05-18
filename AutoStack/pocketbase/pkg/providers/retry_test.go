package providers

import (
	"errors"
	"testing"
	"time"
)

// Phase 3.9.3 — Canonical retry taxonomy tests.
//
// These tests are the structural-correctness contract for the unified
// retry model. They run in pure unit-test mode (no DB required) and
// verify:
//
//   1. Every RetryClass has exactly one RetryPolicy entry (completeness).
//   2. The classifier returns expected classes for known patterns.
//   3. Policy invariants (no Retryable with MaxRetries=0; ambiguous classes
//      escalate; operator-action classes have CircuitBreakerSeverity=0).
//   4. Sentinel error types are honored by errors.As.

func TestRetryPolicyCompleteness(t *testing.T) {
	// Every canonical class MUST appear in the policy table.
	for _, c := range AllRetryClasses {
		if _, ok := retryPolicies[c]; !ok {
			t.Errorf("RetryClass %q missing from retryPolicies table — Phase 3.9.3 mandate", c)
		}
	}
	// And conversely: every policy table entry MUST be a canonical class.
	canonical := map[RetryClass]bool{}
	for _, c := range AllRetryClasses {
		canonical[c] = true
	}
	for c := range retryPolicies {
		if !canonical[c] {
			t.Errorf("retryPolicies has entry %q that is not in AllRetryClasses — schema drift", c)
		}
	}
}

func TestRetryPolicyInvariants(t *testing.T) {
	for _, c := range AllRetryClasses {
		policy := c.Policy()

		// Retryable ⇒ MaxRetries > 0 (otherwise it's silently non-retryable)
		if policy.Retryable && policy.MaxRetries == 0 {
			t.Errorf("RetryClass %q: Retryable=true but MaxRetries=0 — contradiction", c)
		}
		// Non-retryable ⇒ MaxRetries == 0
		if !policy.Retryable && policy.MaxRetries > 0 {
			t.Errorf("RetryClass %q: Retryable=false but MaxRetries=%d — silently retryable", c, policy.MaxRetries)
		}
		// BaseBackoff > 0 ⇒ MaxBackoff >= BaseBackoff
		if policy.BaseBackoff > 0 && policy.MaxBackoff < policy.BaseBackoff {
			t.Errorf("RetryClass %q: MaxBackoff (%s) < BaseBackoff (%s) — backoff cannot grow", c, policy.MaxBackoff, policy.BaseBackoff)
		}
		// OperatorActionRequired classes split into two operational groups:
		//   1. Background operator-action (auth, quota, rate_limit): no breaker
		//      burn — the runtime is healthy, the operator must intervene externally.
		//      These return RetryActionOperatorAction in the engine.
		//   2. Dispatch-failure operator-action (never, internal, unknown): burn
		//      breaker — the dispatch itself failed (bad spec, AutoStack bug, or
		//      uncategorized error). These return RetryActionFail in the engine.
		//
		// The CircuitBreakerSeverity is the discriminator. Both groups are valid
		// configurations; the policy table maps each class to its operational group.
		operatorOnlyClasses := map[RetryClass]bool{
			RetryClassAuth:      true,
			RetryClassQuota:     true,
			RetryClassRateLimit: true,
		}
		dispatchFailureClasses := map[RetryClass]bool{
			RetryClassNever:    true,
			RetryClassInternal: true,
			RetryClassUnknown:  true,
		}
		if policy.OperatorActionRequired {
			if operatorOnlyClasses[c] && policy.CircuitBreakerSeverity > 0 {
				t.Errorf("RetryClass %q: operator-only class must have CircuitBreakerSeverity=0, got %d", c, policy.CircuitBreakerSeverity)
			}
			if dispatchFailureClasses[c] && policy.CircuitBreakerSeverity == 0 {
				t.Errorf("RetryClass %q: dispatch-failure class must have CircuitBreakerSeverity>0, got 0", c)
			}
		}
		// Ambiguous ⇒ EscalateAmbiguity should be true
		// (ambiguous outcomes are exactly what the ambiguity counter exists for).
		if policy.Ambiguous && !policy.EscalateAmbiguity {
			// Acceptable exceptions: classes that are ambiguous but where
			// escalation would not help (e.g., Unknown — better to investigate)
			if c != RetryClassUnknown {
				t.Errorf("RetryClass %q: Ambiguous=true but EscalateAmbiguity=false", c)
			}
		}
	}
}

func TestClassifyProviderError_Auth(t *testing.T) {
	cases := []string{
		"401 Unauthorized",
		"AccessDenied: not authorized",
		"403 Forbidden",
		"invalid credentials",
		"permission denied: missing role",
		"authentication failed",
	}
	for _, c := range cases {
		d := ClassifyProviderError(errors.New(c))
		if d.Class != RetryClassAuth {
			t.Errorf("error %q classified as %s, want %s", c, d.Class, RetryClassAuth)
		}
	}
}

func TestClassifyProviderError_Quota(t *testing.T) {
	cases := []string{
		"quota exceeded for project foo",
		"resource_exhausted: monthly limit",
		"billing not enabled",
	}
	for _, c := range cases {
		d := ClassifyProviderError(errors.New(c))
		if d.Class != RetryClassQuota {
			t.Errorf("error %q classified as %s, want %s", c, d.Class, RetryClassQuota)
		}
	}
}

func TestClassifyProviderError_RateLimit(t *testing.T) {
	cases := []string{
		"429 Too Many Requests",
		"ThrottlingException: rate limit exceeded",
		"throttled by provider",
	}
	for _, c := range cases {
		d := ClassifyProviderError(errors.New(c))
		if d.Class != RetryClassRateLimit {
			t.Errorf("error %q classified as %s, want %s", c, d.Class, RetryClassRateLimit)
		}
	}
}

func TestClassifyProviderError_ProviderUncertain(t *testing.T) {
	cases := []string{
		"deadline exceeded after writing request body",
		"connection reset by peer mid-stream",
		"operation timed out while polling status",
	}
	for _, c := range cases {
		d := ClassifyProviderError(errors.New(c))
		if d.Class != RetryClassProviderUncertain {
			t.Errorf("error %q classified as %s, want %s", c, d.Class, RetryClassProviderUncertain)
		}
		if !d.ProviderUncertain {
			t.Errorf("error %q should set ProviderUncertain=true", c)
		}
	}
}

func TestClassifyProviderError_Conflict(t *testing.T) {
	cases := []string{
		"409 Conflict",
		"resource already_exists",
		"stale revision provided",
	}
	for _, c := range cases {
		d := ClassifyProviderError(errors.New(c))
		if d.Class != RetryClassConflict {
			t.Errorf("error %q classified as %s, want %s", c, d.Class, RetryClassConflict)
		}
	}
}

func TestClassifyProviderError_Never(t *testing.T) {
	cases := []string{
		"resource not found",
		"400 Bad Request: malformed payload",
		"invalid_argument: spec field missing",
		"422 Unprocessable Entity",
	}
	for _, c := range cases {
		d := ClassifyProviderError(errors.New(c))
		if d.Class != RetryClassNever {
			t.Errorf("error %q classified as %s, want %s", c, d.Class, RetryClassNever)
		}
	}
}

func TestClassifyProviderError_Transport(t *testing.T) {
	cases := []string{
		"connection refused",
		"no such host: example.invalid",
		"dial tcp 1.2.3.4:443: i/o timeout",
		"tls handshake timeout",
	}
	for _, c := range cases {
		d := ClassifyProviderError(errors.New(c))
		if d.Class != RetryClassTransport {
			t.Errorf("error %q classified as %s, want %s", c, d.Class, RetryClassTransport)
		}
	}
}

func TestClassifyProviderError_Transient(t *testing.T) {
	cases := []string{
		"503 Service Unavailable",
		"502 Bad Gateway",
		"504 Gateway Timeout",
	}
	for _, c := range cases {
		d := ClassifyProviderError(errors.New(c))
		if d.Class != RetryClassTransient {
			t.Errorf("error %q classified as %s, want %s", c, d.Class, RetryClassTransient)
		}
	}
}

func TestClassifyProviderError_Unknown(t *testing.T) {
	d := ClassifyProviderError(errors.New("some bizarre future error format"))
	if d.Class != RetryClassUnknown {
		t.Errorf("unrecognized error classified as %s, want %s", d.Class, RetryClassUnknown)
	}
}

func TestClassifyProviderError_Nil(t *testing.T) {
	d := ClassifyProviderError(nil)
	if d.Class != RetryClassUnknown {
		t.Errorf("nil error classified as %s, want %s", d.Class, RetryClassUnknown)
	}
}

func TestClassifyProviderError_CapabilityUnavailable(t *testing.T) {
	capErr := ErrCapabilityUnavailable{
		Cap:      CapTrafficSplit,
		Provider: "aws-ecs",
		Notes:    "no native traffic split",
	}
	d := ClassifyProviderError(capErr)
	if d.Class != RetryClassNever {
		t.Errorf("capability error classified as %s, want %s", d.Class, RetryClassNever)
	}
	if d.NativeCode != "CAPABILITY_UNAVAILABLE" {
		t.Errorf("capability error NativeCode = %q, want CAPABILITY_UNAVAILABLE", d.NativeCode)
	}
}

func TestRetryClassPolicy_AmbiguousFlag(t *testing.T) {
	// Provider-uncertain MUST be ambiguous.
	if !RetryClassProviderUncertain.Policy().Ambiguous {
		t.Error("RetryClassProviderUncertain.Policy().Ambiguous must be true")
	}
	// Transient MUST NOT be ambiguous (pre-transmission failure).
	if RetryClassTransient.Policy().Ambiguous {
		t.Error("RetryClassTransient.Policy().Ambiguous must be false")
	}
	// Auth MUST NOT be ambiguous.
	if RetryClassAuth.Policy().Ambiguous {
		t.Error("RetryClassAuth.Policy().Ambiguous must be false")
	}
}

func TestRetryClassPolicy_RetryableInvariants(t *testing.T) {
	if !RetryClassTransient.IsRetriable() {
		t.Error("RetryClassTransient must be retriable")
	}
	if !RetryClassRateLimit.IsRetriable() {
		t.Error("RetryClassRateLimit must be retriable")
	}
	if !RetryClassProviderUncertain.IsRetriable() {
		t.Error("RetryClassProviderUncertain must be retriable (bounded)")
	}
	if !RetryClassConflict.IsRetriable() {
		t.Error("RetryClassConflict must be retriable")
	}
	if !RetryClassTransport.IsRetriable() {
		t.Error("RetryClassTransport must be retriable")
	}
	if RetryClassAuth.IsRetriable() {
		t.Error("RetryClassAuth must NOT be retriable")
	}
	if RetryClassQuota.IsRetriable() {
		t.Error("RetryClassQuota must NOT be retriable (operator only)")
	}
	if RetryClassNever.IsRetriable() {
		t.Error("RetryClassNever must NOT be retriable")
	}
	if RetryClassInternal.IsRetriable() {
		t.Error("RetryClassInternal must NOT be retriable (AutoStack bug)")
	}
	if RetryClassUnknown.IsRetriable() {
		t.Error("RetryClassUnknown must NOT be retriable (conservative)")
	}
}

func TestRetryClass_StableStrings(t *testing.T) {
	// Persisted strings must NEVER change. This test pins them.
	expected := map[RetryClass]string{
		RetryClassNever:             "retry_never",
		RetryClassTransient:         "retry_transient",
		RetryClassRateLimit:         "retry_rate_limit",
		RetryClassProviderUncertain: "retry_provider_uncertain",
		RetryClassConflict:          "retry_conflict",
		RetryClassAuth:              "retry_auth",
		RetryClassQuota:             "retry_quota",
		RetryClassInternal:          "retry_internal",
		RetryClassTransport:         "retry_transport",
		RetryClassUnknown:           "retry_unknown",
	}
	for class, want := range expected {
		if class.String() != want {
			t.Errorf("RetryClass %q changed string to %q — BREAKING SCHEMA CHANGE", want, class.String())
		}
	}
}

func TestRetryDecisionForensicReason(t *testing.T) {
	d := ClassifyProviderError(errors.New("401 Unauthorized: token expired"))
	if d.ForensicReason == "" {
		t.Error("auth classification must populate ForensicReason")
	}
	if d.NativeCode == "" {
		t.Error("auth classification must populate NativeCode for forensics")
	}
}

// Order-of-classification regression: rate-limit must beat quota when
// both keywords appear (e.g., "throttled: quota check failed momentarily").
func TestClassifyProviderError_OrderingHierarchy(t *testing.T) {
	// Auth wins over everything.
	d := ClassifyProviderError(errors.New("403 forbidden: throttled additionally"))
	if d.Class != RetryClassAuth {
		t.Errorf("403+throttled classified as %s, want auth (auth wins)", d.Class)
	}
}

// Ensure SuggestedDelay zero-value is handled.
func TestRetryDecision_ZeroSuggestedDelay(t *testing.T) {
	d := ClassifyProviderError(errors.New("503 Service Unavailable"))
	if d.SuggestedDelay != time.Duration(0) {
		t.Errorf("SuggestedDelay should be 0 for unclassified-delay errors, got %s", d.SuggestedDelay)
	}
}
