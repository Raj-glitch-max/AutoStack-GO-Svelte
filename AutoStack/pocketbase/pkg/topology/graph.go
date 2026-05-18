package topology

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/janlauber/one-click/pkg/orchestrator"
)

// BuildTopologyGraph constructs a deterministic TopologyGraph from an ExecutionPlan.
//
// Provider nodes are grouped by "<provider>/<region>" identity.
// Edges are derived from the stage dependency relationships.
// Cycles are detected using DFS.
//
// Pure function: same ExecutionPlan → same TopologyGraph.
func BuildTopologyGraph(plan *orchestrator.ExecutionPlan) TopologyGraph {
	if plan == nil || len(plan.Stages) == 0 {
		return TopologyGraph{
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
	}

	// Build provider node map: providerID → ProviderNode.
	providerMap := make(map[string]*ProviderNode)
	for _, stage := range plan.Stages {
		for _, node := range stage.Nodes {
			region := regionFromProvider(node.Provider)
			pid := providerID(node.Provider, region)
			if _, exists := providerMap[pid]; !exists {
				providerMap[pid] = &ProviderNode{
					ProviderID: pid,
					Provider:   node.Provider,
					Region:     region,
				}
			}
			providerMap[pid].NodeIDs = append(providerMap[pid].NodeIDs, node.NodeID)
		}
	}

	// Stable provider node list.
	nodes := make([]ProviderNode, 0, len(providerMap))
	for _, n := range providerMap {
		sort.Strings(n.NodeIDs)
		nodes = append(nodes, *n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ProviderID < nodes[j].ProviderID })

	// Build edges from stage dependencies.
	// An edge (stage-A → stage-B dependency) means nodes in A depend on nodes in B.
	nodeProviderIndex := buildNodeProviderIndex(plan.Stages)
	var edges []TopologyEdge
	for _, stage := range plan.Stages {
		for _, dep := range stage.Dependencies {
			// Find dep stage.
			depStage := findStage(plan.Stages, dep)
			if depStage == nil {
				continue
			}
			for _, fromNode := range stage.Nodes {
				for _, toNode := range depStage.Nodes {
					crossProvider := nodeProviderIndex[fromNode.NodeID] != nodeProviderIndex[toNode.NodeID]
					edges = append(edges, TopologyEdge{
						FromNodeID:               fromNode.NodeID,
						ToNodeID:                 toNode.NodeID,
						EdgeType:                 inferEdgeType(fromNode.NodeID, toNode.NodeID),
						CrossProvider:            crossProvider,
						Explanation:              fmt.Sprintf("node %q depends on node %q via stage dependency %q→%q", fromNode.NodeID, toNode.NodeID, stage.StageID, dep),
						BlastRadiusImplication:   fmt.Sprintf("failure of %q may impact %q", toNode.NodeID, fromNode.NodeID),
						RollbackImplication:      fmt.Sprintf("roll back %q before %q", fromNode.NodeID, toNode.NodeID),
						FailureDomainImplication: failureDomainImplication(fromNode.Provider, toNode.Provider, crossProvider),
					})
				}
			}
		}
	}

	// Stable edge ordering.
	sort.Slice(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if a.FromNodeID != b.FromNodeID {
			return a.FromNodeID < b.FromNodeID
		}
		return a.ToNodeID < b.ToNodeID
	})

	// Cycle detection on the node-level dependency graph.
	cyclePaths := detectCycles(edges)

	return TopologyGraph{
		ExecutionID:   executionIDFromPlan(plan),
		Nodes:         nodes,
		Edges:         edges,
		CycleDetected: len(cyclePaths) > 0,
		CyclePaths:    cyclePaths,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
}

// DetectTopologyCycles returns all cycle paths in the topology graph.
// An empty result means no cycles.
func DetectTopologyCycles(graph TopologyGraph) [][]string {
	return detectCycles(graph.Edges)
}

// ─── internal helpers ──────────────────────────────────────────────────────

func buildNodeProviderIndex(stages []orchestrator.ExecutionStage) map[string]string {
	index := make(map[string]string)
	for _, s := range stages {
		for _, n := range s.Nodes {
			index[n.NodeID] = n.Provider
		}
	}
	return index
}

func findStage(stages []orchestrator.ExecutionStage, stageID string) *orchestrator.ExecutionStage {
	for i := range stages {
		if stages[i].StageID == stageID {
			return &stages[i]
		}
	}
	return nil
}

func detectCycles(edges []TopologyEdge) [][]string {
	// Build adjacency list.
	adj := make(map[string][]string)
	for _, e := range edges {
		adj[e.FromNodeID] = append(adj[e.FromNodeID], e.ToNodeID)
	}

	visited := make(map[string]bool)
	inStack := make(map[string]bool)
	var cycles [][]string

	var dfs func(node string, path []string)
	dfs = func(node string, path []string) {
		visited[node] = true
		inStack[node] = true
		path = append(path, node)
		for _, neighbor := range adj[node] {
			if !visited[neighbor] {
				dfs(neighbor, path)
			} else if inStack[neighbor] {
				// Found a cycle — record it.
				cycle := append([]string(nil), path...)
				cycle = append(cycle, neighbor)
				cycles = append(cycles, cycle)
			}
		}
		inStack[node] = false
	}

	// Sort nodes for deterministic traversal.
	nodes := make([]string, 0, len(adj))
	for n := range adj {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)
	for _, n := range nodes {
		if !visited[n] {
			dfs(n, nil)
		}
	}
	return cycles
}

func providerID(provider, region string) string {
	if region == "" {
		return provider
	}
	return provider + "/" + region
}

func regionFromProvider(provider string) string {
	// Providers don't embed region in the string — return empty for now.
	// A real implementation would resolve this from the deployment target.
	return ""
}

func inferEdgeType(fromNodeID, toNodeID string) string {
	// Heuristic inference from node ID naming conventions.
	from := strings.ToLower(fromNodeID)
	to := strings.ToLower(toNodeID)
	switch {
	case strings.Contains(from, "ingress") || strings.Contains(from, "lb"):
		return "ingress-to-backend"
	case strings.Contains(to, "db") || strings.Contains(to, "database"):
		return "service-to-db"
	case strings.Contains(to, "cache") || strings.Contains(to, "redis"):
		return "cache-to-api"
	case strings.Contains(to, "queue") || strings.Contains(to, "sqs") || strings.Contains(to, "pubsub"):
		return "queue-to-service"
	default:
		return "service-dependency"
	}
}

func failureDomainImplication(fromProvider, toProvider string, crossProvider bool) string {
	if crossProvider {
		return fmt.Sprintf("cross-provider dependency: %q failure does not automatically bring down %q provider domain", toProvider, fromProvider)
	}
	return fmt.Sprintf("same-provider dependency: both nodes share the %q failure domain", fromProvider)
}

func executionIDFromPlan(plan *orchestrator.ExecutionPlan) string {
	if plan == nil {
		return ""
	}
	return fmt.Sprintf("plan:%s", plan.ExecutionPlanHash)
}
