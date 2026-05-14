package ecs

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// confirmInterval is the poll interval inside the destroy-confirm loop.
const confirmInterval = 5 * time.Second

// confirmTimeout is the maximum time the confirm loop waits for INACTIVE.
// Longer than Cloud Run's 60s because ECS task draining is slower.
// Overridable in tests via the Provider's confirmTimeoutOverride field.
const confirmTimeout = 120 * time.Second

// confirmDeleted polls DescribeServices until the service reaches INACTIVE
// or the confirm window expires. It implements the `status-inactive-poll`
// destroy-confirm semantic (CapDestroyConfirmation.Semantic).
//
// Contract (per ecs-fargate-provider-design.md §Destroy Confirmation):
//   - Success: service.Status == "INACTIVE" observed within confirmTimeout.
//   - Timeout: returns a non-nil error with [DESTROY_CONFIRM_TIMEOUT] prefix.
//     The caller MUST mark the target deleted with lifecycle_ambiguous=true,
//     lifecycle_ambiguity_source="S-4" (bounded confirmation ambiguity).
//   - ServiceNotFoundException during the poll means the service was deleted
//     successfully; return nil (idempotent).
//
// The outer ctx is the reconciler's operation context. If the ctx is
// cancelled (operator abort, process shutdown) the poll exits without
// marking timeout — the caller handles ctx.Err() separately.
func confirmDeleted(ctx context.Context, client *ecs.Client, cluster, serviceName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("[DESTROY_CONFIRM_TIMEOUT] ECS service %q in cluster %q did not reach INACTIVE within %s; marking deleted with ambiguity",
				serviceName, cluster, timeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		out, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{
			Cluster:  &cluster,
			Services: []string{serviceName},
		})
		if err != nil {
			// ServiceNotFoundException = already gone. Success.
			if isServiceNotFound(err) {
				return nil
			}
			// Other transient errors: log and continue polling.
			log.Printf("[ECS_CONFIRM_POLL_ERR] service=%q cluster=%q err=%v; retrying", serviceName, cluster, err)
		} else if len(out.Services) == 0 {
			// Empty response without error = provider-side lag; treat as not-found.
			return nil
		} else {
			switch out.Services[0].Status {
			case nil:
				// nil status in describe = already cleaned up
				return nil
			default:
				status := *out.Services[0].Status
				switch status {
				case "INACTIVE":
					return nil
				case "ACTIVE", "DRAINING":
					// Still transitioning; keep polling.
				default:
					// Unknown status — log and keep polling to avoid premature exit.
					log.Printf("[ECS_CONFIRM_UNKNOWN_STATUS] service=%q status=%q; continuing poll", serviceName, status)
				}
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(confirmInterval):
		}
	}
}

// isServiceNotFound returns true if the AWS error indicates the ECS service
// was not found. Provider-specific error classification (HC-6: kept inside
// pkg/providers/ecs/).
func isServiceNotFound(err error) bool {
	if err == nil {
		return false
	}
	var nf *types.ServiceNotFoundException
	return errors.As(err, &nf)
}
