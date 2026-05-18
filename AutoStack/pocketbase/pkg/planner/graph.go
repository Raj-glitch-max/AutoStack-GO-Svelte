package planner

import (
	"fmt"
	"sort"
)

// Phase 4.0 — PlanGraph engine.
//
// BuildPlanGraph produces a deterministic directed acyclic graph (DAG) from
// a DesiredState. The graph is the core artifact of the planning engine —
// everything else (ChangeSet, simulation, rollback) derives from it.
//
// ─────────────────────────────────────────────────────────────────────────
// DESIGN INVARIANTS
// ─────────────────────────────────────────────────────────────────────────
//
//   1. DETERMINISTIC: identical DesiredState → identical PlanGraph (same
//      node order, same edge order, same topological sort). No map
//      iteration nondeterminism in output.
//
//   2. CYCLE DETECTION: BuildPlanGraph detects cycles before returning. A
//      graph with cycles is returned with CyclicPaths non-empty and
//      TopologicalOrder empty. Callers must check CyclicPaths before using
//      TopologicalOrder.
//
//   3. ORPHAN DETECTION: nodes with no edges (neither depends on anything
//      nor has anything depending on it) are flagged as orphans but
//      included in the graph. The operator is responsible for deciding
//      whether an orphan is intentional.
//
//   4. PARALLEL STAGE GROUPING: nodes at the same depth in the topological
//      ordering are grouped into parallel stages. All nodes in a stage can
//      be applied simultaneously without violating dependency ordering.
//
//   5. NO PROVIDER CALLS: BuildPlanGraph is a pure function over the
//      DesiredState. It never contacts a provider, reads a DB, or produces
//      side effects.

// PlanEdge represents a directed dependency in the graph.
// From→To means "From depends on To" (To must be applied before From).
type PlanEdge struct {
	From      string `json:"from"`       // node ID that depends
	To        string `json:"to"`         // node ID that is depended on
	EdgeKind  string `json:"edge_kind"`  // "depends-on" | "rollback"
}

// PlanGraph is the fully-constructed DAG with topological ordering.
type PlanGraph struct {
	// Nodes is sorted by node ID for deterministic serialization.
	Nodes []*PlanNode `json:"nodes"`

	// NodeIndex provides O(1) lookup by node ID. Not serialized.
	NodeIndex map[string]*PlanNode `json:"-"`

	// Edges is sorted (From ASC, To ASC) for determinism.
	Edges []PlanEdge `json:"edges"`

	// TopologicalOrder is the stable execution order (dependencies-first).
	// Empty when CyclicPaths is non-empty.
	TopologicalOrder []string `json:"topological_order"`

	// ParallelStages groups nodes that can be applied simultaneously.
	// Stage[0] has no dependencies; Stage[N] depends only on stages < N.
	ParallelStages [][]string `json:"parallel_stages"`

	// OrphanNodes lists node IDs with no in-edges and no out-edges.
	OrphanNodes []string `json:"orphan_nodes"`

	// CyclicPaths lists detected dependency cycles as node-ID paths.
	// Non-empty means the graph is invalid and cannot be planned.
	CyclicPaths [][]string `json:"cyclic_paths,omitempty"`
}

// IsValid reports whether the graph is cycle-free and can be planned.
func (g *PlanGraph) IsValid() bool {
	return len(g.CyclicPaths) == 0
}

// BuildPlanGraph constructs a PlanGraph from a DesiredState.
// The planner caller (simulation, changeset) must check g.IsValid() before use.
func BuildPlanGraph(s *DesiredState, actions map[string]ActionType) *PlanGraph {
	g := &PlanGraph{
		NodeIndex: make(map[string]*PlanNode),
	}

	// 1. Create nodes for all services.
	for _, svc := range s.Services {
		action := ActionNoOp
		if a, ok := actions[svc.ID]; ok {
			action = a
		}
		node := &PlanNode{
			ID:            svc.ID,
			Type:          NodeTypeService,
			Provider:      svc.Provider,
			Dependencies:  sortedCopy(svc.DependsOn),
			DesiredAction: action,
			RiskLevel:     classifyNodeRisk(action, svc),
			CanRollback:   action != ActionDelete,
			WhyNeeded:     whyNeeded(action, svc.ID),
			WhyRisk:       whyNodeRisk(action, svc),
		}
		node.RollbackRefs = node.Dependencies // initial: rollback deps before self
		g.NodeIndex[svc.ID] = node
	}

	// 2. Create nodes for all dependencies (databases, queues, etc.)
	for _, dep := range s.Dependencies {
		action := ActionNoOp
		if a, ok := actions[dep.ID]; ok {
			action = a
		}
		node := &PlanNode{
			ID:            dep.ID,
			Type:          nodeTypeForDep(dep.Kind),
			Provider:      dep.Provider,
			Dependencies:  []string{}, // dep nodes don't depend on services
			DesiredAction: action,
			RiskLevel:     RiskLevelMedium,
			CanRollback:   action != ActionDelete,
			WhyNeeded:     whyNeeded(action, dep.ID),
			WhyRisk:       fmt.Sprintf("dependency node kind=%s action=%s", dep.Kind, action),
		}
		g.NodeIndex[dep.ID] = node
	}

	// 3. Build sorted node slice.
	nodeIDs := make([]string, 0, len(g.NodeIndex))
	for id := range g.NodeIndex {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)
	for _, id := range nodeIDs {
		g.Nodes = append(g.Nodes, g.NodeIndex[id])
	}

	// 4. Build edges and reverse-dependency index.
	for _, node := range g.Nodes {
		for _, dep := range node.Dependencies {
			if _, exists := g.NodeIndex[dep]; !exists {
				// Dependency references a node not in the graph — skip.
				// This is a planning warning, not a fatal error.
				continue
			}
			g.Edges = append(g.Edges, PlanEdge{
				From:     node.ID,
				To:       dep,
				EdgeKind: "depends-on",
			})
			// Populate reverse dependency on the target.
			target := g.NodeIndex[dep]
			target.ReverseDependencies = append(target.ReverseDependencies, node.ID)
		}
	}

	// Sort edges for determinism.
	sort.Slice(g.Edges, func(i, j int) bool {
		if g.Edges[i].From != g.Edges[j].From {
			return g.Edges[i].From < g.Edges[j].From
		}
		return g.Edges[i].To < g.Edges[j].To
	})

	// Sort reverse dependencies for determinism.
	for _, node := range g.Nodes {
		sort.Strings(node.ReverseDependencies)
	}

	// 5. Detect orphans (no in-edges AND no out-edges from the perspective of
	// this node: no deps and nothing depends on it).
	for _, node := range g.Nodes {
		if len(node.Dependencies) == 0 && len(node.ReverseDependencies) == 0 {
			node.IsOrphan = true
			g.OrphanNodes = append(g.OrphanNodes, node.ID)
		}
	}
	sort.Strings(g.OrphanNodes)

	// 6. Detect cycles using DFS.
	g.CyclicPaths = detectCycles(g)

	// 7. Compute topological order (only when cycle-free).
	if len(g.CyclicPaths) == 0 {
		g.TopologicalOrder = topologicalSort(g)
		g.ParallelStages = computeParallelStages(g)
	}

	return g
}

// ─────────────────────────────────────────────────────────────────────────
// TOPOLOGICAL SORT
// ─────────────────────────────────────────────────────────────────────────

// topologicalSort returns a stable execution order for the graph.
// Dependencies are placed before the nodes that depend on them.
// Uses Kahn's algorithm with a sorted queue for determinism.
func topologicalSort(g *PlanGraph) []string {
	// Build in-degree map.
	inDegree := make(map[string]int, len(g.Nodes))
	for _, n := range g.Nodes {
		inDegree[n.ID] = 0
	}
	for _, e := range g.Edges {
		if e.EdgeKind == "depends-on" {
			inDegree[e.From]++
		}
	}

	// Initialize queue with nodes that have no dependencies (in-degree 0).
	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue) // stable starting point

	var order []string
	for len(queue) > 0 {
		// Always pick the lexicographically smallest available node.
		sort.Strings(queue)
		n := queue[0]
		queue = queue[1:]
		order = append(order, n)

		// Reduce in-degree for nodes that depend on n.
		node := g.NodeIndex[n]
		for _, rev := range node.ReverseDependencies {
			inDegree[rev]--
			if inDegree[rev] == 0 {
				queue = append(queue, rev)
			}
		}
	}
	return order
}

// computeParallelStages groups topologically-ordered nodes into stages
// where all nodes in a stage can be applied simultaneously.
func computeParallelStages(g *PlanGraph) [][]string {
	if len(g.TopologicalOrder) == 0 {
		return nil
	}
	// Compute depth of each node (max depth of its dependencies + 1).
	depth := make(map[string]int, len(g.Nodes))
	for _, id := range g.TopologicalOrder {
		d := 0
		for _, dep := range g.NodeIndex[id].Dependencies {
			if depDepth, ok := depth[dep]; ok && depDepth+1 > d {
				d = depDepth + 1
			}
		}
		depth[id] = d
	}

	// Group nodes by depth.
	maxDepth := 0
	for _, d := range depth {
		if d > maxDepth {
			maxDepth = d
		}
	}
	stages := make([][]string, maxDepth+1)
	for _, id := range g.TopologicalOrder {
		d := depth[id]
		stages[d] = append(stages[d], id)
	}
	// Sort within each stage for determinism.
	for i := range stages {
		sort.Strings(stages[i])
	}
	return stages
}

// ─────────────────────────────────────────────────────────────────────────
// CYCLE DETECTION
// ─────────────────────────────────────────────────────────────────────────

func detectCycles(g *PlanGraph) [][]string {
	type state int
	const (
		unvisited state = iota
		inStack
		done
	)
	visited := make(map[string]state, len(g.Nodes))
	var cycles [][]string

	var dfs func(id string, path []string)
	dfs = func(id string, path []string) {
		visited[id] = inStack
		path = append(path, id)

		node := g.NodeIndex[id]
		deps := sortedCopy(node.Dependencies)
		for _, dep := range deps {
			if _, exists := g.NodeIndex[dep]; !exists {
				continue
			}
			switch visited[dep] {
			case inStack:
				// Found a cycle. Record the cycle path from dep to current.
				cycle := []string{}
				for i, p := range path {
					if p == dep {
						cycle = append(cycle, path[i:]...)
						break
					}
				}
				cycle = append(cycle, dep) // close the loop
				cycles = append(cycles, cycle)
			case unvisited:
				dfs(dep, path)
			}
		}
		visited[id] = done
	}

	// Iterate in sorted order for deterministic cycle reporting.
	ids := make([]string, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		ids = append(ids, n.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if visited[id] == unvisited {
			dfs(id, nil)
		}
	}
	return cycles
}

// ─────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────

func nodeTypeForDep(kind string) NodeType {
	switch kind {
	case "database":
		return NodeTypeDatabase
	case "queue":
		return NodeTypeQueue
	case "cache":
		return NodeTypeCache
	case "storage":
		return NodeTypeStorage
	default:
		return NodeTypeDeployment
	}
}

func whyNeeded(action ActionType, id string) string {
	switch action {
	case ActionCreate:
		return fmt.Sprintf("node %s is declared in DesiredState but does not exist in current provider state", id)
	case ActionUpdate:
		return fmt.Sprintf("node %s exists but spec has diverged from current provider state", id)
	case ActionDelete:
		return fmt.Sprintf("node %s exists in provider state but is absent from DesiredState", id)
	case ActionRollback:
		return fmt.Sprintf("node %s failed during a previous apply; rollback to last known good state", id)
	default:
		return fmt.Sprintf("node %s is already in the desired state; no action required", id)
	}
}

func sortedCopy(ss []string) []string {
	out := make([]string, len(ss))
	copy(out, ss)
	sort.Strings(out)
	return out
}
