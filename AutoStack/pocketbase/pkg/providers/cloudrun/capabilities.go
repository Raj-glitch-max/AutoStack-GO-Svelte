package cloudrun

import (
	"time"

	"github.com/janlauber/one-click/pkg/providers"
)

// Capabilities returns the Cloud Run provider's capability profile.
//
// This is the Phase 2 baseline, honestly reported. Where Phase 2
// declines to implement a capability (rollback, metrics, etc.) the
// capability is Supported: false with explanatory Notes. Where Phase 2
// implements a capability with provider-native semantics (destroy
// confirmation via NOT_FOUND poll, eventual-consistency lag), the
// semantic profile reflects what the code actually does.
//
// Reviewers: this file is the authority on what Cloud Run claims.
// It MUST stay in sync with phase3/provider-capability-matrix.md and
// with the actual method implementations in provider.go. Failure to
// keep them aligned is an R-1 (abstraction lie) violation.
func (p *Provider) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{
		providers.CapDeploy: {
			Supported: true,
			Semantic:  "create-or-update-via-services-api",
			Constraints: []string{
				"idempotent-on-existing-service",
				"updateService-is-PATCH-not-PUT",
			},
			UncertaintyP99: 0,
			Notes:          "P-1 compliant. GetService precheck then CreateService or UpdateService.",
		},
		providers.CapDestroy: {
			Supported: true,
			Semantic:  "delete-then-not-found-poll",
			Constraints: []string{
				"idempotent-on-NOT_FOUND",
				"confirm-window-60s",
			},
			UncertaintyP99: 60 * time.Second,
			Notes:          "P-2 + P-15 compliant. Phase 2.8 confirmDeleted enforces NOT_FOUND confirmation.",
		},
		providers.CapGetStatus: {
			Supported: true,
			Semantic:  "ready-condition-based",
			Constraints: []string{
				"non-nil-status-or-non-nil-error-required-by-P-3",
				"NOT_FOUND-classified-as-permanent-failure",
			},
			UncertaintyP99: 5 * time.Second,
			Notes:          "Maps Cloud Run Ready conditions onto canonical states per lifecycle-normalization-model.md.",
		},
		providers.CapRollback: {
			Supported: false,
			Semantic:  "not-implemented",
			Constraints: []string{
				"requires-revision-history-not-yet-persisted",
				"requires-traffic-targeting-impl-Phase-3.3",
			},
			Notes: "Phase 2 declined per P-11. Phase 3.3 (PR-4) makes this Supported: true with semantic 'gradual-traffic-shift'.",
		},
		providers.CapTrafficSplit: {
			Supported: true,
			Semantic:  "gradual-percentage-weighted-native",
			Constraints: []string{
				"via-Service.Traffic-array",
				"up-to-5-traffic-targets",
				"percent-must-sum-to-100",
			},
			Notes: "Provider-native capability. AutoStack does NOT yet wire this through the workflow layer (Phase 3.3).",
		},
		providers.CapRevisionHistory: {
			Supported: true,
			Semantic:  "provider-retained-revisions",
			Constraints: []string{
				"provider-managed-revision-lifecycle",
				"AutoStack-does-not-yet-persist-revision-lineage",
			},
			Notes: "Cloud Run retains revisions natively. AutoStack lineage of revisions is Phase 3 SC-1 (deployed_spec) + Phase 3.3 work.",
		},
		providers.CapCanaryRollout: {
			Supported: false,
			Semantic:  "not-implemented",
			Notes:     "Provider supports it via Traffic. AutoStack workflow layer (Phase 3.3) will expose.",
		},
		providers.CapHealthReporting: {
			Supported: false,
			Semantic:  "not-implemented",
			Constraints: []string{
				"Ready-condition-known",
				"ServingRevision-not-yet-extracted-from-Service.Traffic",
			},
			Notes: "Phase 3 Change 3: implement reading Service.Traffic into TargetStatus.ServingRevision. Phase 3.0 ships the field; Phase 3.1+ populates it.",
		},
		providers.CapDeploymentCancel: {
			Supported: false,
			Semantic:  "not-implemented",
			Notes:     "Cloud Run does not expose deploy cancellation; new revisions are atomic.",
		},
		providers.CapRevisionCleanup: {
			Supported: false,
			Semantic:  "not-implemented",
			Notes:     "Provider auto-garbage-collects old revisions; AutoStack does not orchestrate cleanup.",
		},
		providers.CapDriftVisibility: {
			Supported: false,
			Semantic:  "not-implemented",
			Constraints: []string{
				"requires-deployed_spec-snapshot-Phase-3-SC-1",
			},
			Notes: "Phase 3.2 deferred. Today GetStatus does not diff spec vs actual (P-14).",
		},
		providers.CapScalingIntrospection: {
			Supported: false,
			Semantic:  "not-implemented",
			Notes:     "GetMetrics returns ErrNotImplemented (P-10). Cloud Monitoring API integration is Phase 3.",
		},
		providers.CapLogStreaming: {
			Supported: false,
			Semantic:  "not-implemented",
			Notes:     "StreamLogs returns ErrNotImplemented (P-10).",
		},
		providers.CapMetricsVisibility: {
			Supported: false,
			Semantic:  "not-implemented",
			Notes:     "UI MUST surface 'metrics unavailable' rather than rendering 0.",
		},
		providers.CapDestroyConfirmation: {
			Supported: true,
			Semantic:  "not-found-poll",
			Constraints: []string{
				"post-DeleteService-GetService-returns-NOT_FOUND-eventually",
				"poll-every-5s",
				"max-window-60s",
			},
			UncertaintyP99: 60 * time.Second,
			Notes:          "Phase 2.8 confirmDeleted. On timeout, target is marked deleted with [DESTROY_CONFIRM_TIMEOUT] surfaced.",
		},
		providers.CapIdempotentDeploy: {
			Supported: true,
			Semantic:  "get-then-create-or-update",
			Notes:     "P-1 compliant. Reconciler may re-dispatch pending targets safely.",
		},
		providers.CapOperationTracking: {
			Supported: false,
			Semantic:  "not-implemented",
			Constraints: []string{
				"requires-operation-name-from-CreateService-Phase-3",
			},
			Notes: "Phase 2 declined per P-12. GetOperation returns ErrNotImplemented.",
		},
		providers.CapEventualConsistency: {
			Supported: true,
			Semantic:  "read-after-write-lag-typical",
			UncertaintyP99: 5 * time.Second,
			Notes: "G-6 suspicion counter tolerates first error observation from updating. Per-provider threshold derived from this value (Phase 3 Change 6).",
		},
	}
}
