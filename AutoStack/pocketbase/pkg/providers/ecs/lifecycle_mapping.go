package ecs

import (
	"github.com/janlauber/one-click/pkg/providers"
)

// mapLifecycle translates ECS-native service state into a canonical
// TargetStatus. It implements the N-rule discipline from
// project-context/phase3/lifecycle-normalization-model.md and the specific
// ECS lifecycle table in project-context/phase3/ecs-fargate-provider-design.md.
//
// The caller supplies the current ECS service state fields. The returned
// TargetStatus carries status, message, and ambiguity information — but
// NOT Replicas counts (those are set by the caller from the task data).
//
// Rules applied (see ecs-fargate-provider-design.md for the full table):
//
//	ACTIVE + rolloutState=COMPLETED + runningCount==desiredCount → running
//	ACTIVE + rolloutState=COMPLETED + runningCount<desiredCount  → creating (S-2 ambiguous)
//	ACTIVE + rolloutState=IN_PROGRESS + no prior running         → creating
//	ACTIVE + rolloutState=IN_PROGRESS + prior running            → updating
//	ACTIVE + rolloutState=FAILED                                 → error (after suspicion)
//	DRAINING + destroy-intent                                    → deleting (N-5)
//	DRAINING + no destroy-intent                                 → error (S-2 drift)
//	INACTIVE                                                     → deleted (N-6)
//	NOT_FOUND                                                    → error (P-4)
func mapLifecycle(input lifecycleInput) providers.TargetStatus {
	switch {
	case input.notFound:
		return providers.TargetStatus{
			Status:  "error",
			Message: "[NOT_FOUND] ECS service does not exist",
		}

	case input.serviceStatus == "INACTIVE":
		return providers.TargetStatus{
			Status:  "deleted",
			Message: "Service confirmed deleted (INACTIVE)",
		}

	case input.serviceStatus == "DRAINING":
		if input.destroyIntent {
			return providers.TargetStatus{
				Status:  "deleting",
				Message: "Service draining (DRAINING); deletion in progress",
			}
		}
		// DRAINING without destroy-intent = external mutation / drift
		return providers.TargetStatus{
			Status:  "error",
			Message: "[DRIFT] Service entered DRAINING without AutoStack destroy-intent; manual external action suspected",
		}

	case input.serviceStatus == "ACTIVE":
		return mapActiveLifecycle(input)
	}

	// Defensive: unrecognised status — surface verbatim as ambiguous.
	return providers.TargetStatus{
		Status:  "error",
		Message: "[UNKNOWN_STATE] Unrecognised ECS service status: " + input.serviceStatus,
	}
}

func mapActiveLifecycle(input lifecycleInput) providers.TargetStatus {
	switch input.rolloutState {
	case "COMPLETED":
		if input.runningCount >= input.desiredCount && input.desiredCount > 0 {
			// N-3 affirmative readiness: stable, all tasks up.
			ts := providers.TargetStatus{
				Status:  "running",
				Message: "Service healthy; all tasks running",
			}
			if input.latestTaskDefARN != "" {
				ts.ServingRevision = input.latestTaskDefARN
			}
			return ts
		}
		// runningCount < desiredCount with COMPLETED rollout = scale-up in progress.
		// This is S-2 (provider-side lag), not a failure.
		return providers.TargetStatus{
			Status:  "creating",
			Message: "[SCALE_LAG] Rollout COMPLETED but runningCount < desiredCount; tasks still launching",
		}

	case "IN_PROGRESS":
		if input.hadPriorRunning {
			return providers.TargetStatus{
				Status:  "updating",
				Message: "Deployment rollout IN_PROGRESS (update)",
			}
		}
		return providers.TargetStatus{
			Status:  "creating",
			Message: "Deployment rollout IN_PROGRESS (initial deploy)",
		}

	case "FAILED":
		// G-6 suspicion logic lives in the reconciler; the provider surfaces
		// the raw state. The reconciler increments the suspicion counter and
		// only writes `error` after threshold is exceeded.
		return providers.TargetStatus{
			Status:  "error",
			Message: "[ROLLOUT_FAILED] ECS deployment circuit breaker triggered; service rollout FAILED",
		}
	}

	// Unrecognised rolloutState — surface with ambiguity.
	return providers.TargetStatus{
		Status:  "error",
		Message: "[UNKNOWN_ROLLOUT_STATE] Unrecognised ECS rolloutState: " + input.rolloutState,
	}
}

// lifecycleInput is the canonical set of ECS state signals that
// mapLifecycle and mapActiveLifecycle need. Isolating them here means
// the mapping logic is testable without a real AWS call.
type lifecycleInput struct {
	// notFound is true when DescribeServices returned a ServiceNotFoundException.
	notFound bool

	// serviceStatus is the ECS Service.Status field ("ACTIVE", "DRAINING", "INACTIVE").
	serviceStatus string

	// rolloutState is services[0].deployments[0].rolloutState
	// ("IN_PROGRESS", "COMPLETED", "FAILED"). Empty when DRAINING/INACTIVE.
	rolloutState string

	// runningCount is the number of running tasks.
	runningCount int32

	// desiredCount is the number of desired tasks.
	desiredCount int32

	// hadPriorRunning indicates the target was previously in `running` state
	// (read from the PocketBase row before this call). Used to distinguish
	// initial deploy from update in the IN_PROGRESS branch.
	hadPriorRunning bool

	// destroyIntent is true when the target has pending_destroy or deleting
	// status set by AutoStack (distinguishes AutoStack-initiated DRAINING from
	// external drift).
	destroyIntent bool

	// latestTaskDefARN is the ARN of the task definition revision currently
	// in the COMPLETED deployment. Carried into ServingRevision (PR-1).
	latestTaskDefARN string
}
