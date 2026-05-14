package reconciler

import (
	"fmt"
	"time"

	"github.com/janlauber/one-click/pkg/providers"
)

// capabilityCache caches the Capabilities() result per provider name for the
// process lifetime. Capabilities() is deterministic and stable for the
// lifetime of a registered provider instance.
//
// The cache is intentionally a package-level map (not a field of Reconciler)
// because the Provider interface's capability contract is tied to the
// registered provider, not to any particular reconciler instance.
//
// Access pattern: read-mostly after process startup. Write-once per provider
// at registration time. No lock needed given the single-threaded reconciler
// loop and the fact that providers are registered before any tick fires.
var capabilityCache = make(map[string]providers.CapabilitySet)

// cacheCapabilities stores the capability profile for a provider.
// Called from the reconciler Start() path, after RegisterProvider.
func cacheCapabilities(providerName string, p providers.Provider) {
	capabilityCache[providerName] = p.Capabilities()
}

// getCapabilities returns the cached capability set for a provider.
// Returns an empty set and false if the provider is not in the cache
// (treated as "all capabilities unknown" — the reconciler must handle
// this as a configuration error, not silently permit operations).
func getCapabilities(providerName string) (providers.CapabilitySet, bool) {
	caps, ok := capabilityCache[providerName]
	return caps, ok
}

// destroyConfirmDispatch selects the correct destroy-confirmation strategy
// for a provider based on its CapDestroyConfirmation semantic.
//
// This is the Phase 3 Change 5 capability-aware dispatch table. The reconciler
// calls this INSTEAD of hard-coding provider-specific confirmation logic.
//
// Supported semantics:
//   - "not-found-poll"        — Cloud Run: poll until GetService returns NOT_FOUND
//   - "status-inactive-poll"  — ECS: poll until service.Status == INACTIVE
//
// If the provider does not support CapDestroyConfirmation (Supported=false),
// the reconciler skips confirmation and marks the target deleted immediately
// (this matches Phase 2 behaviour for providers without confirmation support).
//
// Returns the UncertaintyP99 window that the caller should use as the poll
// timeout. Returns (0, nil) to signal "skip confirmation".
func destroyConfirmDispatch(providerName string) (semantic string, timeout time.Duration, err error) {
	caps, ok := getCapabilities(providerName)
	if !ok {
		return "", 0, fmt.Errorf("[DISPATCH_TABLE] no capability profile cached for provider %q; cannot select destroy-confirm strategy", providerName)
	}

	cap, present := caps[providers.CapDestroyConfirmation]
	if !present || !cap.Supported {
		// Provider has no destroy confirmation; reconciler marks deleted immediately.
		return "none", 0, nil
	}

	switch cap.Semantic {
	case "not-found-poll", "status-inactive-poll":
		return cap.Semantic, cap.UncertaintyP99, nil
	default:
		// Unknown semantic — fail safe: do not mark deleted until we understand
		// what confirmation looks like for this provider.
		return "", 0, fmt.Errorf("[DISPATCH_TABLE] unrecognised CapDestroyConfirmation.Semantic %q for provider %q; cannot dispatch; reject until semantic is registered", cap.Semantic, providerName)
	}
}

// suspicionThreshold returns the number of consecutive error observations
// (from updating state) before the reconciler promotes to `error` status
// for a given provider. This is the Phase 3 Change 6 per-provider tuning
// of the G-6 suspicion counter.
//
// The threshold is derived from UncertaintyP99 / reconciler tick interval.
// A provider with a 30s eventual-consistency window needs fewer ticks
// before we trust an error observation (because the window is already 1 tick)
// vs. a provider with a 5s window that might flap within a single tick.
//
// Minimum: 2 (never accept single-tick errors as definitive).
// Maximum: 5 (cap to avoid too-long delay in surfacing real failures).
func suspicionThreshold(providerName string, tickInterval time.Duration) int {
	caps, ok := getCapabilities(providerName)
	if !ok || tickInterval == 0 {
		return 2 // safe default
	}

	cap, present := caps[providers.CapEventualConsistency]
	if !present || !cap.Supported || cap.UncertaintyP99 == 0 {
		return 2
	}

	// ticks needed to cover the uncertainty window
	ticks := int(cap.UncertaintyP99 / tickInterval)
	if ticks < 2 {
		return 2
	}
	if ticks > 5 {
		return 5
	}
	return ticks
}

// supportsCapability is a concise helper for "can this provider do X?"
// Returns false if the capability profile is missing (safe: treat as
// unsupported rather than permissive).
func supportsCapability(providerName string, key providers.CapabilityKey) bool {
	caps, ok := getCapabilities(providerName)
	if !ok {
		return false
	}
	cap, present := caps[key]
	return present && cap.Supported
}

// capabilityNotes returns the Notes field for a capability, or empty string
// if the profile is missing. Used to populate ErrCapabilityUnavailable.
func capabilityNotes(providerName string, key providers.CapabilityKey) string {
	caps, ok := getCapabilities(providerName)
	if !ok {
		return ""
	}
	return caps[key].Notes
}

// ambiguityDeadline returns the absolute time at which an ambiguous observation
// should escalate. Derived from CapDestroyConfirmation.UncertaintyP99 × 3
// because destroy is the longest provider operation; capped at 10 minutes.
//
// Used to populate deployment_targets.lifecycle_ambiguity_deadline when the
// status-poll path returns ts.Ambiguous=true for the first time on a target.
func ambiguityDeadline(providerName string) time.Time {
	const defaultDelay = 5 * time.Minute
	const maxDelay = 10 * time.Minute
	caps, ok := getCapabilities(providerName)
	if !ok {
		return time.Now().Add(defaultDelay)
	}
	cap, present := caps[providers.CapDestroyConfirmation]
	if !present || !cap.Supported || cap.UncertaintyP99 == 0 {
		return time.Now().Add(defaultDelay)
	}
	d := cap.UncertaintyP99 * 3
	if d > maxDelay {
		d = maxDelay
	}
	return time.Now().Add(d)
}
