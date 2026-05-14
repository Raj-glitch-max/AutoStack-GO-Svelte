package ecs

import (
	"testing"
	"time"

	"github.com/janlauber/one-click/pkg/providers"
)

// TestCapabilitiesCompleteness verifies that every AllCapabilityKeys entry
// is present in the ECS capability profile. Absence would be an honesty
// bug (R-1 mitigation): missing keys must not default to permissive.
func TestCapabilitiesCompleteness(t *testing.T) {
	p := NewProvider()
	caps := p.Capabilities()

	for _, key := range providers.AllCapabilityKeys {
		if _, ok := caps[key]; !ok {
			t.Errorf("ECS capability profile missing key %q — all AllCapabilityKeys must be present (Supported=false with Notes if not implemented)", key)
		}
	}
}

// TestCapabilitiesHonesty verifies that Supported=true implies non-empty
// Semantic and that Supported=false has Semantic="" or "not-implemented".
func TestCapabilitiesHonesty(t *testing.T) {
	p := NewProvider()
	caps := p.Capabilities()

	for key, cap := range caps {
		if cap.Supported && cap.Semantic == "" {
			t.Errorf("ECS capability %q Supported=true but Semantic is empty — an abstraction lie (R-1)", key)
		}
		if !cap.Supported && cap.Semantic != "" && cap.Semantic != "not-implemented" {
			t.Errorf("ECS capability %q Supported=false but Semantic=%q — use \"not-implemented\" or empty for unsupported", key, cap.Semantic)
		}
	}
}

// TestPhase31BaselinePreserved pins the ECS Phase 3.1 baseline so future
// Phase 3 changes cannot silently regress what Phase 3.1 delivers. Lifting
// a capability from Supported=false to Supported=true requires updating
// this test and the design doc in the same PR.
//
// Phase 3.3 update: CapRollback promoted to Supported=true (task-def pinning
// via UpdateService; ARN sourced from operations.checkpoint_data).
func TestPhase31BaselinePreserved(t *testing.T) {
	p := NewProvider()
	caps := p.Capabilities()

	mustBeSupported := []providers.CapabilityKey{
		providers.CapDeploy,
		providers.CapDestroy,
		providers.CapGetStatus,
		providers.CapDestroyConfirmation,
		providers.CapIdempotentDeploy,
		providers.CapEventualConsistency,
		providers.CapRevisionHistory,
		providers.CapHealthReporting,
		providers.CapDeploymentCancel,
		providers.CapRollback, // Phase 3.3: UpdateService to prior task def ARN
	}
	for _, key := range mustBeSupported {
		if !caps[key].Supported {
			t.Errorf("ECS Phase 3.1 baseline regression: %q must remain Supported=true", key)
		}
	}

	mustBeUnsupported := []providers.CapabilityKey{
		providers.CapMetricsVisibility,
		providers.CapLogStreaming,
		providers.CapScalingIntrospection,
		providers.CapDriftVisibility,
		providers.CapOperationTracking,
		providers.CapTrafficSplit,
		providers.CapCanaryRollout,
		providers.CapRevisionCleanup,
	}
	for _, key := range mustBeUnsupported {
		if caps[key].Supported {
			t.Errorf("ECS baseline shift: %q is now Supported=true; update this test + design doc if intentional", key)
		}
	}
}

// TestDestroyConfirmationSemantic pins the ECS destroy-confirm semantic
// so the reconciler's capability-aware dispatch (Phase 3 Change 5) does
// not regress.
func TestDestroyConfirmationSemantic(t *testing.T) {
	p := NewProvider()
	cap := p.Capabilities()[providers.CapDestroyConfirmation]
	if cap.Semantic != "status-inactive-poll" {
		t.Errorf("ECS CapDestroyConfirmation.Semantic = %q, want %q (ecs-fargate-provider-design.md §Destroy)", cap.Semantic, "status-inactive-poll")
	}
	if cap.UncertaintyP99 == 0 {
		t.Errorf("ECS CapDestroyConfirmation.UncertaintyP99 must be non-zero")
	}
	if cap.UncertaintyP99 < 60*time.Second {
		t.Errorf("ECS CapDestroyConfirmation.UncertaintyP99 = %s, must be ≥ 60s (ECS draining is slower than Cloud Run)", cap.UncertaintyP99)
	}
}

// TestEventualConsistencyLongerThanCloudRun documents the R-8 requirement:
// ECS UncertaintyP99 must be >= Cloud Run's 5s AND the reconciler suspicion
// threshold must be tuned accordingly.
func TestEventualConsistencyLongerThanCloudRun(t *testing.T) {
	p := NewProvider()
	cap := p.Capabilities()[providers.CapEventualConsistency]
	if !cap.Supported {
		t.Fatal("ECS must declare CapEventualConsistency Supported=true")
	}
	cloudRunUncertainty := 5 * time.Second
	if cap.UncertaintyP99 <= cloudRunUncertainty {
		t.Errorf("ECS EventualConsistency.UncertaintyP99 = %s, must be > %s (Cloud Run baseline); ECS has longer R-8 lag", cap.UncertaintyP99, cloudRunUncertainty)
	}
}

// TestLifecycleMapping_running verifies the happy-path canonical state.
func TestLifecycleMapping_running(t *testing.T) {
	ts := mapLifecycle(lifecycleInput{
		serviceStatus:    "ACTIVE",
		rolloutState:     "COMPLETED",
		runningCount:     2,
		desiredCount:     2,
		latestTaskDefARN: "arn:aws:ecs:us-east-1:123:task-definition/myapp-fargate:5",
	})
	if ts.Status != "running" {
		t.Errorf("expected status=running, got %q (msg: %s)", ts.Status, ts.Message)
	}
	if ts.ServingRevision == "" {
		t.Errorf("expected ServingRevision to be populated when COMPLETED + healthy")
	}
}

// TestLifecycleMapping_creating verifies initial deploy state.
func TestLifecycleMapping_creating(t *testing.T) {
	ts := mapLifecycle(lifecycleInput{
		serviceStatus: "ACTIVE",
		rolloutState:  "IN_PROGRESS",
		runningCount:  0,
		desiredCount:  2,
	})
	if ts.Status != "creating" {
		t.Errorf("expected status=creating, got %q", ts.Status)
	}
}

// TestLifecycleMapping_updating verifies update-in-progress state.
func TestLifecycleMapping_updating(t *testing.T) {
	ts := mapLifecycle(lifecycleInput{
		serviceStatus:   "ACTIVE",
		rolloutState:    "IN_PROGRESS",
		runningCount:    1,
		desiredCount:    2,
		hadPriorRunning: true,
	})
	if ts.Status != "updating" {
		t.Errorf("expected status=updating, got %q", ts.Status)
	}
}

// TestLifecycleMapping_deleting verifies AutoStack-initiated DRAINING.
func TestLifecycleMapping_deleting(t *testing.T) {
	ts := mapLifecycle(lifecycleInput{
		serviceStatus: "DRAINING",
		destroyIntent: true,
	})
	if ts.Status != "deleting" {
		t.Errorf("expected status=deleting, got %q", ts.Status)
	}
}

// TestLifecycleMapping_drainWithoutIntent verifies external drift detection.
func TestLifecycleMapping_drainWithoutIntent(t *testing.T) {
	ts := mapLifecycle(lifecycleInput{
		serviceStatus: "DRAINING",
		destroyIntent: false,
	})
	if ts.Status != "error" {
		t.Errorf("expected status=error (drift), got %q", ts.Status)
	}
}

// TestLifecycleMapping_deleted verifies INACTIVE → deleted.
func TestLifecycleMapping_deleted(t *testing.T) {
	ts := mapLifecycle(lifecycleInput{serviceStatus: "INACTIVE"})
	if ts.Status != "deleted" {
		t.Errorf("expected status=deleted, got %q", ts.Status)
	}
}

// TestLifecycleMapping_notFound verifies NOT_FOUND → error.
func TestLifecycleMapping_notFound(t *testing.T) {
	ts := mapLifecycle(lifecycleInput{notFound: true})
	if ts.Status != "error" {
		t.Errorf("expected status=error (not found), got %q", ts.Status)
	}
}

// TestLifecycleMapping_scaleLag verifies COMPLETED but running < desired.
func TestLifecycleMapping_scaleLag(t *testing.T) {
	ts := mapLifecycle(lifecycleInput{
		serviceStatus: "ACTIVE",
		rolloutState:  "COMPLETED",
		runningCount:  1,
		desiredCount:  3,
	})
	if ts.Status != "creating" {
		t.Errorf("expected status=creating (scale lag), got %q", ts.Status)
	}
}
