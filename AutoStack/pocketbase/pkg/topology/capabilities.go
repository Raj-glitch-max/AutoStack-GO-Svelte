package topology

import "sort"

// knownCapabilities is the canonical, deterministic capability registry.
// Adding a new provider requires an entry here only — no changes to other files.
var knownCapabilities = map[string]ProviderCapabilities{
	"gcp-cloudrun": {
		Provider:             "gcp-cloudrun",
		SupportsCancellation: true,
		SupportsRollback:     true,
		SupportsHealthProbe:  true,
		SupportsAsyncRollout: true,
		SupportsTrafficShift: true,
		SupportsStagedVerify: true,
		Limitations: []string{
			"Traffic split operations may lag 30s–2min before taking effect.",
			"Rollback restores the previous revision but does not guarantee traffic consistency.",
			"Multi-region deployments require separate verification per region.",
		},
	},
	"aws-ecs": {
		Provider:             "aws-ecs",
		SupportsCancellation: true,
		SupportsRollback:     true,
		SupportsHealthProbe:  true,
		SupportsAsyncRollout: true,
		SupportsTrafficShift: false,
		SupportsStagedVerify: true,
		Limitations: []string{
			"ECS does not natively support traffic shifting without an ALB listener rule.",
			"Task replacement during rolling update extends the effective blast radius.",
			"VPC networking propagation can add 60–90s to stabilization.",
		},
	},
	"azure-aca": {
		Provider:             "azure-aca",
		SupportsCancellation: true,
		SupportsRollback:     false,
		SupportsHealthProbe:  true,
		SupportsAsyncRollout: true,
		SupportsTrafficShift: true,
		SupportsStagedVerify: false,
		Limitations: []string{
			"Azure Container Apps does not support first-class rollback — re-deploy previous image.",
			"ARM eventual consistency may delay status visibility by 45–90s.",
			"Staged verification requires manual polling — no push-based readiness signal.",
		},
	},
}

// defaultCapabilities is returned for providers not in the registry.
var defaultCapabilities = ProviderCapabilities{
	SupportsCancellation: false,
	SupportsRollback:     false,
	SupportsHealthProbe:  false,
	SupportsAsyncRollout: false,
	SupportsTrafficShift: false,
	SupportsStagedVerify: false,
	Limitations: []string{
		"Provider is not in the capability registry — assume no advanced operations are supported.",
		"Operator must manually verify state after every action.",
	},
}

// CapabilitiesForProvider returns the ProviderCapabilities for a named provider.
// Returns a default (all-false) entry for unknown providers.
func CapabilitiesForProvider(provider string) ProviderCapabilities {
	if caps, ok := knownCapabilities[provider]; ok {
		return caps
	}
	cap := defaultCapabilities
	cap.Provider = provider
	return cap
}

// BuildCapabilityMatrix returns the ProviderCapabilities for each provider in the list.
// The result is sorted by Provider for determinism.
func BuildCapabilityMatrix(providers []string) []ProviderCapabilities {
	// Deduplicate first.
	seen := make(map[string]bool, len(providers))
	unique := make([]string, 0, len(providers))
	for _, p := range providers {
		if !seen[p] {
			seen[p] = true
			unique = append(unique, p)
		}
	}
	sort.Strings(unique)

	result := make([]ProviderCapabilities, 0, len(unique))
	for _, p := range unique {
		result = append(result, CapabilitiesForProvider(p))
	}
	return result
}

// ProvidersFromGraph extracts the unique set of providers from a TopologyGraph.
func ProvidersFromGraph(graph TopologyGraph) []string {
	seen := make(map[string]bool)
	for _, n := range graph.Nodes {
		seen[n.Provider] = true
	}
	result := make([]string, 0, len(seen))
	for p := range seen {
		result = append(result, p)
	}
	sort.Strings(result)
	return result
}
