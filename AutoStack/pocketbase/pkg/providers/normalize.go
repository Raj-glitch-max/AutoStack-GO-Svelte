package providers

import (
	"errors"
	"strings"
)

// Phase 3.7 — Provider Reliability Hardening.
//
// This file establishes a small, shared vocabulary for classifying
// provider responses. Each provider (ECS, Cloud Run, future ACA) maps
// its native errors into these categories so the reconciler can apply
// uniform retry / abort / hold semantics without provider-specific
// branching in core paths.
//
// Design constraints:
//   - NO external retry libraries
//   - NO hidden retry loops at the provider boundary
//   - The reconciler is the ONLY retry authority; providers only classify

// RetryClass is the structured retry recommendation for a provider error.
//
// Providers DO NOT retry themselves. They classify, the reconciler decides.
// This keeps retry economics under reconciler control (budget, backoff,
// circuit breaker) — provider-internal retry would invisibly multiply load.
type RetryClass string

const (
	// RetryClassTransient — temporary failure, retry with backoff.
	// Examples: network timeout, 503 Service Unavailable, connection reset.
	RetryClassTransient RetryClass = "transient"

	// RetryClassRateLimited — provider says slow down; retry with longer backoff.
	// Examples: 429 Too Many Requests, "rate limit exceeded".
	RetryClassRateLimited RetryClass = "rate_limited"

	// RetryClassPermanent — request cannot succeed as-is; do not retry.
	// Examples: 400 Bad Request, "resource not found", spec validation failure.
	RetryClassPermanent RetryClass = "permanent"

	// RetryClassAuth — credentials are invalid or expired; operator action required.
	// Examples: 401 Unauthorized, 403 Forbidden, "invalid credentials".
	RetryClassAuth RetryClass = "auth"

	// RetryClassQuota — provider quota exhausted; operator action required.
	// Examples: "quota exceeded", billing block.
	RetryClassQuota RetryClass = "quota"

	// RetryClassUncertain — provider gave an ambiguous response; treat as transient
	// but mark with provider-uncertainty flag. The reconciler may choose to hold
	// rather than retry.
	RetryClassUncertain RetryClass = "uncertain"
)

// ClassifyProviderError maps a provider error to a RetryClass.
// Conservative: when in doubt, returns RetryClassUncertain (safest).
//
// This is a SHARED utility — providers may also classify natively using
// their own error types where available. This function handles the
// common-case string-pattern fallback.
func ClassifyProviderError(err error) RetryClass {
	if err == nil {
		return ""
	}
	// Honor structured provider errors before falling through to string matching.
	var capErr ErrCapabilityUnavailable
	if errors.As(err, &capErr) {
		return RetryClassPermanent
	}

	msg := strings.ToLower(err.Error())

	// Auth patterns — highest priority; never retry without operator action.
	for _, p := range []string{"unauthorized", "401", "403", "forbidden",
		"permission denied", "invalid credentials", "credential",
		"access denied", "expired token", "invalid token"} {
		if strings.Contains(msg, p) {
			return RetryClassAuth
		}
	}

	// Quota patterns — never retry; operator must intervene.
	for _, p := range []string{"quota", "billing", "resource_exhausted",
		"resource exhausted"} {
		if strings.Contains(msg, p) {
			return RetryClassQuota
		}
	}

	// Rate limit — retry with longer backoff.
	for _, p := range []string{"429", "too many requests", "rate limit",
		"rate_limit", "throttle", "throttled"} {
		if strings.Contains(msg, p) {
			return RetryClassRateLimited
		}
	}

	// Permanent failures — request cannot be made valid by retrying.
	for _, p := range []string{"not found", "not_found", "404",
		"malformed", "invalid argument", "invalid_argument",
		"validation", "bad request", "400", "422", "409", "conflict",
		"already exists", "already_exists"} {
		if strings.Contains(msg, p) {
			return RetryClassPermanent
		}
	}

	// Transient network/IO patterns.
	for _, p := range []string{"timeout", "deadline", "connection reset",
		"connection refused", "no such host", "i/o timeout",
		"503", "502", "504", "service unavailable", "bad gateway",
		"gateway timeout"} {
		if strings.Contains(msg, p) {
			return RetryClassTransient
		}
	}

	// Default: uncertain. Reconciler may choose to retry once or hold.
	return RetryClassUncertain
}

// IsRetriable reports whether the reconciler should retry an operation
// that failed with this class.
//
// Note: this returns the retry recommendation. The reconciler ALSO checks
// circuit breaker state, per-target cooldown, and global backoff before
// actually scheduling a retry.
func (c RetryClass) IsRetriable() bool {
	switch c {
	case RetryClassTransient, RetryClassRateLimited, RetryClassUncertain:
		return true
	case RetryClassPermanent, RetryClassAuth, RetryClassQuota:
		return false
	default:
		return false
	}
}
