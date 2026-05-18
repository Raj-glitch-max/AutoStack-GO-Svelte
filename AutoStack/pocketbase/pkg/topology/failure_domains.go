package topology

import (
	"fmt"
	"sort"
)

// BuildFailureDomains derives FailureDomains from the topology graph.
//
// A failure domain groups all nodes on the same provider/region — they are
// assumed to fail together during a provider or regional outage.
//
// Pure function: same graph → same domains.
func BuildFailureDomains(graph TopologyGraph) []FailureDomain {
	// Group nodes by provider/region.
	type domainKey struct{ provider, region string }
	domainMap := make(map[domainKey]*FailureDomain)

	for _, node := range graph.Nodes {
		key := domainKey{node.Provider, node.Region}
		if _, exists := domainMap[key]; !exists {
			domainID := fmt.Sprintf("fd:%s", node.ProviderID)
			risk := outageRiskForProvider(node.Provider)
			domainMap[key] = &FailureDomain{
				DomainID:    domainID,
				Provider:    node.Provider,
				Region:      node.Region,
				OutageRisk:  risk,
				Explanation: fmt.Sprintf("All nodes on %q share the same provider failure domain; a %q outage affects all %d node(s).", node.Provider, node.Provider, len(node.NodeIDs)),
			}
		}
		domainMap[key].NodeIDs = append(domainMap[key].NodeIDs, node.NodeIDs...)
	}

	// Build stable result.
	domains := make([]FailureDomain, 0, len(domainMap))
	for _, d := range domainMap {
		sort.Strings(d.NodeIDs)
		domains = append(domains, *d)
	}
	sort.Slice(domains, func(i, j int) bool { return domains[i].DomainID < domains[j].DomainID })
	return domains
}

// CrossDomainEdges returns the subset of topology edges that cross failure-domain
// boundaries (i.e., from one provider to another).
func CrossDomainEdges(graph TopologyGraph) []TopologyEdge {
	var result []TopologyEdge
	for _, e := range graph.Edges {
		if e.CrossProvider {
			result = append(result, e)
		}
	}
	return result
}

// outageRiskForProvider returns a qualitative outage risk string.
// This is a conservative static classification — not a live SLA assessment.
func outageRiskForProvider(provider string) string {
	switch provider {
	case "gcp-cloudrun", "aws-ecs":
		return "low" // highly available managed services
	case "azure-aca":
		return "low"
	default:
		return "medium" // unknown provider — assume elevated risk
	}
}
