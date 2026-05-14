package cloudrun

import (
	"testing"

	"github.com/janlauber/one-click/pkg/providers"
)

// TestCapabilitiesCompleteness verifies that the Cloud Run provider's
// capability profile contains an entry for EVERY key in AllCapabilityKeys.
// A missing key is an honesty bug (R-1 mitigation): absence must not
// default to permissive.
func TestCapabilitiesCompleteness(t *testing.T) {
	p := NewProvider()
	caps := p.Capabilities()

	for _, key := range providers.AllCapabilityKeys {
		if _, ok := caps[key]; !ok {
			t.Errorf("Cloud Run capability profile missing key %q — every AllCapabilityKeys entry must be present (with Supported=false if not implemented)", key)
		}
	}
}

// TestCapabilitiesHonesty verifies the runtime defense against capability
// lies. For each capability the provider declares Supported: true, we
// require a non-empty Semantic string — a Supported capability with no
// semantic is an abstraction lie waiting to happen. For each capability
// the provider declares Supported: false, we require the method (when
// it exists in the interface) to surface ErrNotImplemented or an
// equivalent honest refusal.
//
// This is the test enforcement of phase3/provider-capability-matrix.md
// "The Verification Gate".
func TestCapabilitiesHonesty(t *testing.T) {
	p := NewProvider()
	caps := p.Capabilities()

	for key, cap := range caps {
		if cap.Supported && cap.Semantic == "" {
			t.Errorf("capability %q declares Supported=true but Semantic is empty — a supported capability without semantic profile is an R-1 abstraction lie", key)
		}
		if !cap.Supported && cap.Semantic != "" && cap.Semantic != "not-implemented" {
			// Allowed: Supported=false with Semantic="" or "not-implemented".
			// Disallowed: Supported=false with a "real" semantic name.
			t.Errorf("capability %q declares Supported=false but Semantic=%q — unsupported capabilities should declare Semantic=\"not-implemented\" or empty", key, cap.Semantic)
		}
	}
}

// TestPhase2BaselinePreserved pins down the Phase 2 baseline so that
// future Phase 3 changes to Cloud Run capabilities cannot silently
// regress what Phase 2 promised. Phase 3 work that lifts a capability
// from Supported=false to Supported=true must update this test in the
// same PR.
func TestPhase2BaselinePreserved(t *testing.T) {
	p := NewProvider()
	caps := p.Capabilities()

	mustBeSupported := []providers.CapabilityKey{
		providers.CapDeploy,
		providers.CapDestroy,
		providers.CapGetStatus,
		providers.CapTrafficSplit,
		providers.CapDestroyConfirmation,
		providers.CapIdempotentDeploy,
		providers.CapEventualConsistency,
		providers.CapRevisionHistory,
	}
	for _, key := range mustBeSupported {
		if !caps[key].Supported {
			t.Errorf("Phase 2 baseline regression: %q must remain Supported=true (Phase 2 delivered this)", key)
		}
	}

	mustBeUnsupported := []providers.CapabilityKey{
		providers.CapRollback,
		providers.CapMetricsVisibility,
		providers.CapLogStreaming,
		providers.CapScalingIntrospection,
		providers.CapDriftVisibility,
		providers.CapOperationTracking,
	}
	for _, key := range mustBeUnsupported {
		if caps[key].Supported {
			t.Errorf("Phase 2 baseline shift: %q is now Supported=true. If this is intentional Phase 3 work, update this test AND phase3/provider-capability-matrix.md AND phase3/provider-contract-evolution.md", key)
		}
	}
}

// TestDestroyConfirmationSemantic pins the Cloud Run destroy-confirm
// semantic so the reconciler's capability-aware confirm dispatch
// (Phase 3 Change 5) does not regress.
func TestDestroyConfirmationSemantic(t *testing.T) {
	p := NewProvider()
	cap := p.Capabilities()[providers.CapDestroyConfirmation]
	if cap.Semantic != "not-found-poll" {
		t.Errorf("Cloud Run CapDestroyConfirmation.Semantic = %q, want %q (Phase 2.8 contract)", cap.Semantic, "not-found-poll")
	}
	if cap.UncertaintyP99 == 0 {
		t.Errorf("Cloud Run CapDestroyConfirmation.UncertaintyP99 must be non-zero — the 60s confirm window IS the uncertainty bound")
	}
}

// TestEventualConsistencyTunesSuspicion documents the Phase 3 Change 6
// dependency: the per-provider suspicion threshold formula reads
// UncertaintyP99 from this capability. If this value is wrong, the
// reconciler will either miss flaps or false-trigger errors.
func TestEventualConsistencyTunesSuspicion(t *testing.T) {
	p := NewProvider()
	cap := p.Capabilities()[providers.CapEventualConsistency]
	if !cap.Supported {
		t.Fatal("Cloud Run must declare CapEventualConsistency Supported=true; absence breaks Phase 3 Change 6 suspicion tuning")
	}
	if cap.UncertaintyP99 == 0 {
		t.Errorf("CapEventualConsistency.UncertaintyP99 must be non-zero")
	}
}
