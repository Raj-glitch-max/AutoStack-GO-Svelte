package ecs

import (
	"time"

	"github.com/janlauber/one-click/pkg/providers"
)

// Capabilities returns the ECS/Fargate provider's Phase 3.1 baseline
// capability profile.
//
// The profile is the Phase 3.1 baseline: Deploy, Destroy, GetStatus,
// DestroyConfirmation, IdempotentDeploy, EventualConsistency,
// RevisionHistory, and HealthReporting are implemented. All others are
// Supported=false with Notes pointing to the Phase that will implement them.
//
// This file is the authority for what ECS claims. It MUST stay in sync
// with project-context/phase3/ecs-fargate-provider-design.md (capability
// table) and project-context/phase3/provider-capability-matrix.md.
// Divergence is an R-1 (abstraction lie) violation.
func (p *Provider) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{
		providers.CapDeploy: {
			Supported: true,
			Semantic:  "create-or-update-service",
			Constraints: []string{
				"RegisterTaskDefinition-then-CreateService-or-UpdateService",
				"cluster-must-exist-as-autostack-<region>",
				"idempotent-on-existing-service",
			},
			Notes: "DescribeServices precheck; creates or updates ECS service. TaskDefinition family: <rolloutID>-fargate.",
		},
		providers.CapDestroy: {
			Supported: true,
			Semantic:  "scale-to-zero-then-delete-then-inactive-poll",
			Constraints: []string{
				"UpdateService(desiredCount=0)-then-DeleteService",
				"confirm-window-120s",
				"idempotent-on-ServiceNotFoundException",
			},
			UncertaintyP99: 120 * time.Second,
			Notes:          "Two-step destroy: drain tasks then delete. confirmDeleted polls INACTIVE status.",
		},
		providers.CapGetStatus: {
			Supported: true,
			Semantic:  "deployment-rollout-state-based",
			Constraints: []string{
				"reads-services[0].deployments[0].rolloutState",
				"non-nil-status-or-non-nil-error-required",
				"ServiceNotFoundException-classified-as-permanent-failure",
			},
			UncertaintyP99: 30 * time.Second,
			Notes:          "Maps ECS rolloutState+serviceStatus per lifecycle-normalization-model.md N-rules.",
		},
		providers.CapRollback: {
			Supported: true,
			Semantic:  "update-service-to-prior-task-def",
			Constraints: []string{
				"targetRevision-must-be-task-definition-ARN-or-family:revision",
				"prior-task-def-ARN-sourced-from-operations.checkpoint_data",
				"idempotent-on-already-at-target-revision",
			},
			UncertaintyP99: 30 * time.Second,
			Notes: "Phase 3.3: rolls back by UpdateService to a prior task definition ARN. Requires checkpoint_data from a prior deploy operation.",
		},
		providers.CapTrafficSplit: {
			Supported: false,
			Semantic:  "not-implemented",
			Notes:     "ECS CodeDeploy blue/green is provider-native but AutoStack does not orchestrate it in Phase 3.1.",
		},
		providers.CapRevisionHistory: {
			Supported: true,
			Semantic:  "task-definition-revisions-retained",
			Constraints: []string{
				"AWS-retains-task-definition-revisions-per-family",
				"AutoStack-does-not-yet-persist-revision-lineage",
			},
			Notes: "Task definition revisions persist on AWS side. AutoStack lineage tracking is Phase 3 SC-1 work.",
		},
		providers.CapCanaryRollout: {
			Supported: false,
			Semantic:  "not-implemented",
			Notes:     "Phase 3.3 may unlock via CodeDeploy blue/green integration.",
		},
		providers.CapHealthReporting: {
			Supported: true,
			Semantic:  "deployment-circuit-breaker-state",
			Constraints: []string{
				"reads-deployments[0].rolloutState=FAILED-as-unhealthy",
				"ServingRevision-set-to-latestTaskDefinitionArn-when-COMPLETED",
			},
			Notes: "ECS deployment circuit breaker provides health signal via rolloutState=FAILED.",
		},
		providers.CapDeploymentCancel: {
			Supported: true,
			Semantic:  "update-service-force-new-deployment",
			Constraints: []string{
				"cancellation-is-roll-forward-to-prior-task-def",
				"not-a-true-cancel-provder-does-not-expose-cancel-api",
			},
			Notes: "UpdateService with forceNewDeployment=true stops current rollout by launching prior task def.",
		},
		providers.CapRevisionCleanup: {
			Supported: false,
			Semantic:  "not-implemented",
			Notes:     "Task definition revisions are auto-retained by AWS; AutoStack does not orchestrate cleanup.",
		},
		providers.CapDriftVisibility: {
			Supported: false,
			Semantic:  "not-implemented",
			Constraints: []string{
				"requires-deployed_spec-snapshot-Phase-3-SC-1",
			},
			Notes: "Phase 3.2 deferred. GetStatus does not diff spec vs actual.",
		},
		providers.CapScalingIntrospection: {
			Supported: false,
			Semantic:  "not-implemented",
			Notes:     "Phase 3.5+ via CloudWatch metrics.",
		},
		providers.CapLogStreaming: {
			Supported: false,
			Semantic:  "not-implemented",
			Notes:     "Phase 3+ via CloudWatch Logs integration.",
		},
		providers.CapMetricsVisibility: {
			Supported: false,
			Semantic:  "not-implemented",
			Notes:     "Phase 3+ via CloudWatch metrics. UI MUST surface 'metrics unavailable' rather than rendering 0.",
		},
		providers.CapDestroyConfirmation: {
			Supported: true,
			Semantic:  "status-inactive-poll",
			Constraints: []string{
				"post-DeleteService-DescribeServices-returns-INACTIVE-eventually",
				"poll-every-5s",
				"max-window-120s",
			},
			UncertaintyP99: 120 * time.Second,
			Notes:          "Longer window than Cloud Run (60s) because ECS task draining is slower. On timeout, target marked deleted with [DESTROY_CONFIRM_TIMEOUT] + ambiguity bit.",
		},
		providers.CapIdempotentDeploy: {
			Supported: true,
			Semantic:  "describe-then-create-or-update",
			Notes:     "DescribeServices precheck; safe to re-dispatch pending targets.",
		},
		providers.CapOperationTracking: {
			Supported: false,
			Semantic:  "not-implemented",
			Notes:     "ECS does not expose true LRO operation names. Phase 3 deferred.",
		},
		providers.CapEventualConsistency: {
			Supported: true,
			Semantic:  "read-after-write-lag-typical",
			UncertaintyP99: 30 * time.Second,
			Notes: "ECS R-8: longer eventual-consistency window than Cloud Run (30s vs 5s). Suspicion threshold tuned accordingly.",
		},
	}
}
