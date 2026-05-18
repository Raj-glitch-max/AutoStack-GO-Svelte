package providers

import (
	"errors"
	"strings"
	"time"
)

// Phase 3.9.3 — Canonical Retry Taxonomy (Retry Semantic Unification).
//
// This file is the SINGLE SOURCE OF TRUTH for retry semantics across the
// AutoStack runtime. Before Phase 3.9.3 the codebase had two competing
// classifications (FailureCategory in pkg/reconciler, RetryClass in
// pkg/providers) plus raw err.Error() string matching scattered through
// dispatch paths. Phase 3.9.3 collapses all of that into the model below.
//
// ─────────────────────────────────────────────────────────────────────────
// DESIGN INVARIANTS
// ─────────────────────────────────────────────────────────────────────────
//
//   1. Every provider error MUST be classified through ClassifyProviderError
//      before any retry / circuit-breaker decision.
//   2. NO code outside this file may match on err.Error() substrings for
//      retry decisions.
//   3. Each RetryClass has exactly one RetryPolicy entry. The policy is
//      authoritative for: retryable, ambiguous, max budget, cooldown,
//      operator-action, circuit-breaker severity.
//   4. RetryClass values are STABLE STRINGS that get persisted to
//      operations.retry_class. Renaming a class is a breaking schema change.
//   5. Provider mutations under RetryClassProviderUncertain MUST NOT be
//      claimed as success. The reconciler routes them through the
//      ambiguity path instead.
//
// ─────────────────────────────────────────────────────────────────────────
// AMBIGUITY VS RETRYABILITY (CRITICAL DISTINCTION)
// ─────────────────────────────────────────────────────────────────────────
//
//   retryable    = "can this same operation safely be tried again?"
//   ambiguous    = "did the previous attempt commit cloud-side state?"
//
// These are independent. Examples:
//
//   503 BEFORE request transmitted   → retryable=true,  ambiguous=false
//   TCP reset DURING request body    → retryable=true,  ambiguous=true
//   401 Unauthorized                  → retryable=false, ambiguous=false
//   ECS UpdateService timeout         → retryable=true,  ambiguous=true
//   GCP quota exceeded                → retryable=true,  ambiguous=false
//   400 validation error              → retryable=false, ambiguous=false
//
// The reconciler reads BOTH dimensions from the RetryDecision struct.

// ─────────────────────────────────────────────────────────────────────────
// CANONICAL RETRY CLASSES
// ─────────────────────────────────────────────────────────────────────────

// RetryClass is the canonical classification of a provider error.
//
// String values are PERSISTED (operations.retry_class). Treat as a stable
// public schema; renames require a migration.
type RetryClass string

const (
	// RetryClassNever — request is invalid as-issued; no retry will ever succeed.
	// Examples: 400 Bad Request, validation failure, malformed spec.
	// Action: surface error to operator; do not retry.
	RetryClassNever RetryClass = "retry_never"

	// RetryClassTransient — temporary failure unrelated to request content.
	// Examples: 503 Service Unavailable BEFORE request transmitted, transient
	// gRPC UNAVAILABLE. Retryable; outcome is NOT ambiguous because the
	// mutation never started cloud-side.
	RetryClassTransient RetryClass = "retry_transient"

	// RetryClassRateLimit — provider explicitly throttled the request.
	// Examples: 429 Too Many Requests, AWS Throttling, GCP rate_limit.
	// Retryable with longer backoff. Operator action only if persistent.
	RetryClassRateLimit RetryClass = "retry_rate_limit"

	// RetryClassProviderUncertain — request may have committed cloud-side
	// but the reconciler cannot tell from the response.
	// Examples: TCP reset after request body sent, polling timeout on
	// long-running operation, ECS UpdateService timeout after acceptance.
	// AMBIGUOUS. Retry safe ONLY if provider mutation is idempotent.
	RetryClassProviderUncertain RetryClass = "retry_provider_uncertain"

	// RetryClassConflict — request lost a state race; re-read + retry may succeed.
	// Examples: 409 Conflict, "already_exists" on create, stale revision.
	// Retryable after a state-refresh.
	RetryClassConflict RetryClass = "retry_conflict"

	// RetryClassAuth — credentials invalid, expired, or insufficient.
	// Examples: 401, 403, "invalid credentials", "permission denied".
	// NEVER retried automatically; operator must rotate/repair credentials.
	RetryClassAuth RetryClass = "retry_auth"

	// RetryClassQuota — provider quota exhausted; operator must request more.
	// Examples: "quota exceeded", "resource_exhausted" (per-account quota,
	// NOT per-request rate limit), billing block.
	// Retryable AFTER long cooldown but operator action is the real remedy.
	RetryClassQuota RetryClass = "retry_quota"

	// RetryClassInternal — internal logic error in AutoStack itself.
	// Examples: spec marshal failure, missing required config, programmer error.
	// NEVER retried; surfaced as a bug.
	RetryClassInternal RetryClass = "retry_internal"

	// RetryClassTransport — low-level network/TLS failure.
	// Examples: connection refused, DNS resolution failure, TLS handshake
	// timeout. Retryable; ambiguous IFF the request body was sent.
	// Most transport failures are pre-transmission, so default is non-ambiguous;
	// providers with finer-grained signals should override via classifyByContext.
	RetryClassTransport RetryClass = "retry_transport"

	// RetryClassUnknown — classification failed. SAFE DEFAULT for unrecognised errors.
	// Treated as ambiguous + non-retryable to err on the safe side.
	// Operator action: investigate the provider native error code.
	RetryClassUnknown RetryClass = "retry_unknown"
)

// AllRetryClasses lists the canonical class set in stable order.
// Used by tests and verification tooling to assert policy completeness.
var AllRetryClasses = []RetryClass{
	RetryClassNever,
	RetryClassTransient,
	RetryClassRateLimit,
	RetryClassProviderUncertain,
	RetryClassConflict,
	RetryClassAuth,
	RetryClassQuota,
	RetryClassInternal,
	RetryClassTransport,
	RetryClassUnknown,
}

// ─────────────────────────────────────────────────────────────────────────
// RETRY POLICY TABLE
// ─────────────────────────────────────────────────────────────────────────

// RetryPolicy is the per-class authoritative decision policy.
//
// Read-mostly: the policy table is initialised once and never mutated.
// To change retry behavior for a class, edit the table here — NO scattered
// per-call-site overrides allowed.
type RetryPolicy struct {
	// Retryable: may the reconciler retry this same operation?
	// Combined with retry-budget exhaustion AND lease validity to make
	// the final decision.
	Retryable bool

	// Ambiguous: may the cloud-side mutation have already committed?
	// If true, retrying is safe ONLY when the provider mutation is idempotent
	// AND the reconciler is willing to write a forensic "interrupted, retried"
	// history row before the next attempt.
	Ambiguous bool

	// CircuitBreakerSeverity: how heavily this class trips the per-target
	// circuit breaker. 0 = does not increment (auth/quota wait for operator);
	// 1 = normal failure increment; 2 = double-weight (provider-uncertain
	// trips faster because outcome is unknown).
	CircuitBreakerSeverity int

	// MaxRetries: hard ceiling on retry attempts per operation.
	// 0 means "never retry" (must align with Retryable=false).
	MaxRetries int

	// BaseBackoff: starting delay for exponential backoff.
	// Final delay = BaseBackoff * 2^(attempt-1), capped at MaxBackoff.
	BaseBackoff time.Duration

	// MaxBackoff: ceiling on per-retry delay. Prevents arbitrarily long waits.
	MaxBackoff time.Duration

	// OperatorActionRequired: true means retries alone will not resolve;
	// the operator must take an external action (rotate creds, raise quota,
	// fix spec). Even if Retryable=true, surface this in logs/incidents.
	OperatorActionRequired bool

	// EscalateAmbiguity: should this class contribute to the ambiguity
	// escalation counter? Only true for classes whose outcome is unknown
	// and where repeated occurrences suggest a stuck deployment state.
	EscalateAmbiguity bool
}

// retryPolicies is the canonical policy table. Read-mostly after init.
// Phase 3.9.3 mandate: every RetryClass MUST have an entry. Missing entries
// are caught by TestRetryPolicyCompleteness.
var retryPolicies = map[RetryClass]RetryPolicy{
	RetryClassNever: {
		Retryable:              false,
		Ambiguous:              false,
		CircuitBreakerSeverity: 1,
		MaxRetries:             0,
		BaseBackoff:            0,
		MaxBackoff:             0,
		OperatorActionRequired: true,
		EscalateAmbiguity:      false,
	},
	RetryClassTransient: {
		Retryable:              true,
		Ambiguous:              false,
		CircuitBreakerSeverity: 1,
		MaxRetries:             5,
		BaseBackoff:            2 * time.Second,
		MaxBackoff:             60 * time.Second,
		OperatorActionRequired: false,
		EscalateAmbiguity:      false,
	},
	RetryClassRateLimit: {
		Retryable:              true,
		Ambiguous:              false,
		CircuitBreakerSeverity: 0, // rate limits aren't deployment failures
		MaxRetries:             10,
		BaseBackoff:            10 * time.Second,
		MaxBackoff:             5 * time.Minute,
		OperatorActionRequired: false,
		EscalateAmbiguity:      false,
	},
	RetryClassProviderUncertain: {
		Retryable:              true, // but bounded; provider must be idempotent
		Ambiguous:              true,
		CircuitBreakerSeverity: 2, // outcome unknown — trip breaker faster
		MaxRetries:             2, // strict cap; ambiguous retries can amplify damage
		BaseBackoff:            5 * time.Second,
		MaxBackoff:             30 * time.Second,
		OperatorActionRequired: false,
		EscalateAmbiguity:      true,
	},
	RetryClassConflict: {
		Retryable:              true,
		Ambiguous:              false,
		CircuitBreakerSeverity: 1,
		MaxRetries:             3,
		BaseBackoff:            1 * time.Second,
		MaxBackoff:             10 * time.Second,
		OperatorActionRequired: false,
		EscalateAmbiguity:      false,
	},
	RetryClassAuth: {
		Retryable:              false,
		Ambiguous:              false,
		CircuitBreakerSeverity: 0, // operator-only; do not burn circuit budget
		MaxRetries:             0,
		BaseBackoff:            0,
		MaxBackoff:             0,
		OperatorActionRequired: true,
		EscalateAmbiguity:      false,
	},
	RetryClassQuota: {
		Retryable:              false, // operator must raise quota
		Ambiguous:              false,
		CircuitBreakerSeverity: 0,
		MaxRetries:             0,
		BaseBackoff:            0,
		MaxBackoff:             0,
		OperatorActionRequired: true,
		EscalateAmbiguity:      false,
	},
	RetryClassInternal: {
		Retryable:              false,
		Ambiguous:              false,
		CircuitBreakerSeverity: 1, // AutoStack bug; trip breaker so it surfaces
		MaxRetries:             0,
		BaseBackoff:            0,
		MaxBackoff:             0,
		OperatorActionRequired: true,
		EscalateAmbiguity:      false,
	},
	RetryClassTransport: {
		Retryable:              true,
		Ambiguous:              false, // default: pre-transmission; provider may override
		CircuitBreakerSeverity: 1,
		MaxRetries:             5,
		BaseBackoff:            2 * time.Second,
		MaxBackoff:             60 * time.Second,
		OperatorActionRequired: false,
		EscalateAmbiguity:      false,
	},
	RetryClassUnknown: {
		Retryable:              false, // safe default — investigate
		Ambiguous:              true,  // assume worst case
		CircuitBreakerSeverity: 2,
		MaxRetries:             0,
		BaseBackoff:            0,
		MaxBackoff:             0,
		OperatorActionRequired: true,
		EscalateAmbiguity:      true,
	},
}

// Policy returns the RetryPolicy for a class. Falls back to RetryClassUnknown's
// policy if an unknown class string is passed (defensive against persisted
// stale data from a future renamed class).
func (c RetryClass) Policy() RetryPolicy {
	if p, ok := retryPolicies[c]; ok {
		return p
	}
	return retryPolicies[RetryClassUnknown]
}

// IsRetriable is shorthand for c.Policy().Retryable.
func (c RetryClass) IsRetriable() bool { return c.Policy().Retryable }

// IsAmbiguous is shorthand for c.Policy().Ambiguous.
func (c RetryClass) IsAmbiguous() bool { return c.Policy().Ambiguous }

// ─────────────────────────────────────────────────────────────────────────
// RETRY DECISION
// ─────────────────────────────────────────────────────────────────────────

// RetryDecision is the rich classification output. Providers MUST return
// this from ClassifyProviderError; the reconciler consumes it to make the
// canonical retry decision.
//
// Persisted fields (Class, NativeCode, ForensicReason) are recorded on
// operations.retry_class and operations.retry_history for forensic
// reconstruction.
type RetryDecision struct {
	Class           RetryClass    // canonical class
	NativeCode      string        // provider's native error code (e.g., "ThrottlingException")
	ForensicReason  string        // operator-readable explanation
	SuggestedDelay  time.Duration // provider hint; reconciler may override via policy
	ProviderUncertain bool        // explicit ambiguity signal from provider
}

// ─────────────────────────────────────────────────────────────────────────
// CLASSIFIER
// ─────────────────────────────────────────────────────────────────────────

// ClassifyProviderError is the SINGLE entry point for converting a raw
// error into a RetryDecision. Every provider error path in the AutoStack
// runtime MUST flow through this function (or its per-provider wrapper).
//
// Order of matching matters: more specific patterns are tested first so
// the most informative class wins.
//
// Honest limitation: string matching is fundamentally fragile against
// provider SDK changes. Long-term, providers should expose typed errors
// (errors.As) — that mechanism is checked first when available. The
// string heuristics are a fallback for SDK-error opaqueness.
func ClassifyProviderError(err error) RetryDecision {
	if err == nil {
		return RetryDecision{Class: RetryClassUnknown, ForensicReason: "nil error"}
	}

	// Typed-error short-circuit: structured errors carry their own meaning.
	var capErr ErrCapabilityUnavailable
	if errors.As(err, &capErr) {
		return RetryDecision{
			Class:          RetryClassNever,
			ForensicReason: "capability unavailable: " + string(capErr.Cap),
			NativeCode:     "CAPABILITY_UNAVAILABLE",
		}
	}

	msg := strings.ToLower(err.Error())

	// Order matters — most specific patterns first.

	// Auth — highest priority; never retried.
	for _, p := range []string{"unauthorized", " 401", "401 ", "403", "forbidden",
		"permission denied", "invalid credentials", "credential",
		"access denied", "accessdenied", "expired token", "invalid token", "authentication",
		"not authorized", "notauthorized"} {
		if strings.Contains(msg, p) {
			return RetryDecision{
				Class:          RetryClassAuth,
				ForensicReason: "auth/permission failure",
				NativeCode:     extractNativeCode(msg, p),
			}
		}
	}

	// Quota — operator action required.
	for _, p := range []string{"quota", "billing", "resource_exhausted",
		"resource exhausted"} {
		if strings.Contains(msg, p) {
			return RetryDecision{
				Class:          RetryClassQuota,
				ForensicReason: "provider quota exhausted",
				NativeCode:     extractNativeCode(msg, p),
			}
		}
	}

	// Rate limit — retry with longer backoff.
	for _, p := range []string{"429", "too many requests", "rate limit",
		"rate_limit", "throttle", "throttled", "throttling"} {
		if strings.Contains(msg, p) {
			return RetryDecision{
				Class:          RetryClassRateLimit,
				ForensicReason: "provider rate limit",
				NativeCode:     extractNativeCode(msg, p),
			}
		}
	}

	// Provider-uncertain — explicit ambiguity from polling timeouts
	// AND from request-body-sent then connection-lost patterns.
	for _, p := range []string{"deadline exceeded after",
		"operation timed out", "request canceled while waiting",
		"response body", "connection reset by peer"} {
		if strings.Contains(msg, p) {
			return RetryDecision{
				Class:             RetryClassProviderUncertain,
				ForensicReason:    "request transmitted but outcome unknown",
				NativeCode:        extractNativeCode(msg, p),
				ProviderUncertain: true,
			}
		}
	}

	// Conflict — re-read and retry.
	for _, p := range []string{"already exists", "already_exists",
		"409", "conflict", "stale revision", "stale_revision",
		"version_conflict", "precondition failed"} {
		if strings.Contains(msg, p) {
			return RetryDecision{
				Class:          RetryClassConflict,
				ForensicReason: "state conflict; safe to re-read and retry",
				NativeCode:     extractNativeCode(msg, p),
			}
		}
	}

	// Never — request is invalid; no retry can succeed.
	for _, p := range []string{"not found", "not_found", "404",
		"malformed", "invalid argument", "invalid_argument",
		"validation", "bad request", "400", "422"} {
		if strings.Contains(msg, p) {
			return RetryDecision{
				Class:          RetryClassNever,
				ForensicReason: "request invalid; retry will not succeed",
				NativeCode:     extractNativeCode(msg, p),
			}
		}
	}

	// Transport — low-level network failures.
	for _, p := range []string{"connection refused", "no such host",
		"i/o timeout", "tls handshake", "dial tcp", "network is unreachable"} {
		if strings.Contains(msg, p) {
			return RetryDecision{
				Class:          RetryClassTransport,
				ForensicReason: "transport failure pre-transmission",
				NativeCode:     extractNativeCode(msg, p),
			}
		}
	}

	// Transient — generic 5xx-class signals.
	for _, p := range []string{"503", "502", "504",
		"service unavailable", "bad gateway", "gateway timeout",
		"timeout", "deadline"} {
		if strings.Contains(msg, p) {
			return RetryDecision{
				Class:          RetryClassTransient,
				ForensicReason: "transient provider failure",
				NativeCode:     extractNativeCode(msg, p),
			}
		}
	}

	// Default: unknown. Conservative — assume ambiguous + non-retryable.
	return RetryDecision{
		Class:          RetryClassUnknown,
		ForensicReason: "unclassified provider error; investigate native code",
	}
}

// extractNativeCode pulls a short token near the matched pattern for
// inclusion in forensic records. Best-effort: returns the matched
// pattern itself if no better token can be extracted.
func extractNativeCode(msg, matchedPattern string) string {
	idx := strings.Index(msg, matchedPattern)
	if idx < 0 {
		return matchedPattern
	}
	// Grab up to 40 chars around the match for context.
	start := idx
	end := idx + len(matchedPattern) + 20
	if end > len(msg) {
		end = len(msg)
	}
	snippet := msg[start:end]
	// Trim to a reasonable single-token boundary.
	if cut := strings.IndexAny(snippet, " ,;:\n\t"); cut > 0 {
		snippet = snippet[:cut]
	}
	return snippet
}

// String returns the canonical class string. Stable; persisted.
func (c RetryClass) String() string { return string(c) }

// ─────────────────────────────────────────────────────────────────────────
// PROVIDER OVERRIDE HOOK
// ─────────────────────────────────────────────────────────────────────────

// ErrorClassifier is an OPTIONAL interface providers may implement to
// override the generic string-matching classifier for typed errors they
// can recognize natively. Returning nil falls through to ClassifyProviderError.
//
// Phase 3.9.3 Runtime Integration honest scope:
//   - Adding this interface does NOT change any existing provider.
//   - It is a SEAM for future per-provider typed-error refinement.
//   - The generic classifier remains the safe default.
//   - Implementations MUST NOT introduce hidden retry loops; they classify
//     observations only. The reconciler decides retry policy.
//
// Example future use: ECS could implement this and use errors.As to detect
// types like *types.ThrottlingException or *types.ResourceNotFoundException
// directly, yielding more accurate classification than string matching.
type ErrorClassifier interface {
	// ClassifyError returns a refined RetryDecision for the given error,
	// or nil to let the generic classifier run. The returned class MUST
	// be one of the canonical RetryClass values — providers cannot invent
	// new classes.
	ClassifyError(err error) *RetryDecision
}
