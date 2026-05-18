package providers

import "strings"

// Phase 3.9.8 — Canonical Provider State Vocabulary.
//
// NormalizeProviderState converts a raw provider state string into the
// canonical observation vocabulary before any contradiction or drift logic
// runs. The reconciler's TargetStatus.Status already uses this vocabulary
// (providers normalize their native states inside GetStatus). This function
// is the safety net for:
//   - Case variants ("RUNNING" → "running")
//   - Direct raw-API strings that bypass GetStatus (destroy-confirm loop,
//     dispatch post-result paths)
//   - Unknown/unrecognised states that must be preserved verbatim as forensic
//     evidence (unknown states must NOT be silently coerced)
//
// ─────────────────────────────────────────────────────────────────────────
// CANONICAL STATE SET
// ─────────────────────────────────────────────────────────────────────────
//
// These are the observation-layer state labels used in
// provider_observations.provider_state. They are a superset of the
// deployment_targets.status select enum (which the reconciler writes to DB)
// and exist so forensic records carry consistent vocabulary regardless of
// which provider generated the state.

const (
	CanonicalStateRunning     = "running"
	CanonicalStateCreating    = "creating"
	CanonicalStateUpdating    = "updating"
	CanonicalStateError       = "error"
	CanonicalStateDeleting    = "deleting"
	CanonicalStateDeleted     = "deleted"
	CanonicalStateMissing     = "missing"
	CanonicalStateUnknown     = "unknown"
	CanonicalStateAmbiguous   = "ambiguous"
	CanonicalStatePending     = "pending"
	CanonicalStateRollingBack = "rolling_back"
	CanonicalStateRolledBack  = "rolled_back"
)

// providerStateAliases maps case variants and provider-native state strings
// to canonical observation vocabulary. The alias table is deliberately
// conservative: only well-understood equivalences are listed. Novel states
// from new providers should NOT be silently added here — they should flow
// through as unknown until a human confirms the equivalence.
var providerStateAliases = map[string]string{
	// Case variants of canonical states (most common case)
	"running":        CanonicalStateRunning,
	"creating":       CanonicalStateCreating,
	"updating":       CanonicalStateUpdating,
	"error":          CanonicalStateError,
	"deleting":       CanonicalStateDeleting,
	"deleted":        CanonicalStateDeleted,
	"missing":        CanonicalStateMissing,
	"unknown":        CanonicalStateUnknown,
	"ambiguous":      CanonicalStateAmbiguous,
	"pending":        CanonicalStatePending,
	"rolling_back":   CanonicalStateRollingBack,
	"rolled_back":    CanonicalStateRolledBack,

	// Common provider-native aliases (ECS, Cloud Run, Azure ACA)
	"not_found":        CanonicalStateMissing,
	"notfound":         CanonicalStateMissing,
	"not found":        CanonicalStateMissing,
	"inactive":         CanonicalStateDeleted,   // ECS INACTIVE = confirmed deleted
	"draining":         CanonicalStateDeleting,  // ECS DRAINING with destroy-intent
	"provisioning":     CanonicalStateCreating,  // GCP / Azure provisioning
	"deprovisioning":   CanonicalStateDeleting,
	"ready":            CanonicalStateRunning,   // GCP Cloud Run READY = serving
	"failed":           CanonicalStateError,
	"rollback_complete": CanonicalStateRolledBack,
}

// NormalizeProviderState converts rawState to canonical observation vocabulary.
// Returns the canonical form when a known alias exists. Returns the lowercased
// raw value verbatim when unknown — unknown states are forensic evidence and
// MUST NOT be silently erased.
//
// Callers that detect a non-canonical result (IsCanonicalProviderState returns
// false) should downgrade ObservationTrust to ObservationTrustAmbiguous so the
// record accurately reflects reduced confidence.
//
// Pure function — no side effects, no DB access.
func NormalizeProviderState(rawState string) string {
	if rawState == "" {
		return CanonicalStateUnknown
	}
	lower := strings.ToLower(strings.TrimSpace(rawState))
	if canonical, ok := providerStateAliases[lower]; ok {
		return canonical
	}
	return lower
}

// IsCanonicalProviderState reports whether s is one of the canonical state
// constants. Returns false for unknown or non-canonical states. Use this to
// decide whether to downgrade observation trust.
func IsCanonicalProviderState(s string) bool {
	switch s {
	case CanonicalStateRunning, CanonicalStateCreating, CanonicalStateUpdating,
		CanonicalStateError, CanonicalStateDeleting, CanonicalStateDeleted,
		CanonicalStateMissing, CanonicalStateUnknown, CanonicalStateAmbiguous,
		CanonicalStatePending, CanonicalStateRollingBack, CanonicalStateRolledBack:
		return true
	}
	return false
}
