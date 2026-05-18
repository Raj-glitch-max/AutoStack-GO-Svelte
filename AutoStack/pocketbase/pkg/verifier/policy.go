package verifier

import "time"

// Provider-specific consistency policies.
// Every policy documents the honest operational characteristics of that provider.
var (
	// CloudRunConsistencyPolicy: Cloud Run has strong eventual consistency.
	// Revision propagation can lag 30s–2min after creation.
	CloudRunConsistencyPolicy = ConsistencyPolicy{
		Provider:            "gcp-cloudrun",
		EventualConsistency: true,
		MaxStaleness:        int64(5 * time.Minute),
		PropagationDelay:    int64(30 * time.Second),
		RequiresPolling:     true,
		Description: "Cloud Run uses eventual consistency. A revision in SERVING state may not " +
			"be receiving traffic immediately. Traffic propagation can lag 30s–2min. " +
			"GetStatus may return UPDATING while the revision is being promoted.",
	}

	// ECSConsistencyPolicy: ECS Fargate has longer stabilization windows.
	// Tasks show PENDING before RUNNING; capacity provisioning adds latency.
	ECSConsistencyPolicy = ConsistencyPolicy{
		Provider:            "aws-ecs",
		EventualConsistency: true,
		MaxStaleness:        int64(10 * time.Minute),
		PropagationDelay:    int64(60 * time.Second),
		RequiresPolling:     true,
		Description: "ECS Fargate task lifecycle goes PENDING → RUNNING with no guaranteed timing. " +
			"Capacity provisioning, image pulls, and VPC attachment each add latency. " +
			"A service showing ACTIVE does not guarantee all tasks are RUNNING.",
	}

	// AzureACAConsistencyPolicy: Azure Container Apps uses ARM-backed eventual consistency.
	AzureACAConsistencyPolicy = ConsistencyPolicy{
		Provider:            "azure-aca",
		EventualConsistency: true,
		MaxStaleness:        int64(7 * time.Minute),
		PropagationDelay:    int64(45 * time.Second),
		RequiresPolling:     true,
		Description: "Azure Container Apps deployments are ARM-backed with eventual consistency. " +
			"Replica provisioning can lag 45s–2min after the ARM operation completes. " +
			"ARM may return Succeeded before container readiness is confirmed.",
	}

	// DefaultConsistencyPolicy: conservative fallback for unknown providers.
	DefaultConsistencyPolicy = ConsistencyPolicy{
		Provider:            "unknown",
		EventualConsistency: true,
		MaxStaleness:        int64(5 * time.Minute),
		PropagationDelay:    int64(30 * time.Second),
		RequiresPolling:     true,
		Description: "Default policy for unknown providers. Assumes eventual consistency with " +
			"a 30-second propagation delay and 5-minute maximum staleness. " +
			"Use a provider-specific policy where available.",
	}
)

// ConsistencyPolicyForProvider returns the canonical consistency policy for a provider.
// Returns DefaultConsistencyPolicy for unrecognized providers.
func ConsistencyPolicyForProvider(provider string) ConsistencyPolicy {
	switch provider {
	case "gcp-cloudrun", "cloudrun":
		return CloudRunConsistencyPolicy
	case "aws-ecs", "ecs":
		return ECSConsistencyPolicy
	case "azure-aca", "aca":
		return AzureACAConsistencyPolicy
	default:
		return DefaultConsistencyPolicy
	}
}

// MaxStalenessFor returns the MaxStaleness duration for a named provider.
func MaxStalenessFor(provider string) time.Duration {
	return time.Duration(ConsistencyPolicyForProvider(provider).MaxStaleness)
}

// PropagationDelayFor returns the PropagationDelay for a named provider.
func PropagationDelayFor(provider string) time.Duration {
	return time.Duration(ConsistencyPolicyForProvider(provider).PropagationDelay)
}
