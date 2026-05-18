package workflow

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"time"
)

// WorkflowGraph is an immutable directed graph defining the structure of a workflow.
// GraphHash covers all stable fields. Structural validity is enforced by NewWorkflowGraph.
type WorkflowGraph struct {
	GraphID   string         `json:"graph_id"`
	TenantID  string         `json:"tenant_id"`
	Name      string         `json:"name"`
	Nodes     []WorkflowNode `json:"nodes"`
	Edges     []WorkflowEdge `json:"edges"`
	EntryNode string         `json:"entry_node"`
	CreatedAt string         `json:"created_at"`
	GraphHash string         `json:"graph_hash"`
}

// NewWorkflowGraph constructs and validates a WorkflowGraph.
// Errors if: no nodes, no entry node, or entry node not in node list.
func NewWorkflowGraph(
	graphID, tenantID, name, entryNode string,
	nodes []WorkflowNode,
	edges []WorkflowEdge,
) (WorkflowGraph, error) {
	if graphID == "" || tenantID == "" {
		return WorkflowGraph{}, fmt.Errorf("graphID and tenantID are required")
	}
	if len(nodes) == 0 {
		return WorkflowGraph{}, fmt.Errorf("graph %q must have at least one node", graphID)
	}
	if entryNode == "" {
		return WorkflowGraph{}, fmt.Errorf("graph %q must specify an entry node", graphID)
	}
	nodeIDs := make(map[string]struct{}, len(nodes))
	for _, n := range nodes {
		nodeIDs[n.NodeID] = struct{}{}
	}
	if _, ok := nodeIDs[entryNode]; !ok {
		return WorkflowGraph{}, fmt.Errorf("entry node %q not found in graph %q", entryNode, graphID)
	}
	for _, e := range edges {
		if _, ok := nodeIDs[e.FromNode]; !ok {
			return WorkflowGraph{}, fmt.Errorf("edge %q references unknown from-node %q", e.EdgeID, e.FromNode)
		}
		if _, ok := nodeIDs[e.ToNode]; !ok {
			return WorkflowGraph{}, fmt.Errorf("edge %q references unknown to-node %q", e.EdgeID, e.ToNode)
		}
	}

	nodesCopy := make([]WorkflowNode, len(nodes))
	copy(nodesCopy, nodes)
	edgesCopy := make([]WorkflowEdge, len(edges))
	copy(edgesCopy, edges)

	g := WorkflowGraph{
		GraphID:   graphID,
		TenantID:  tenantID,
		Name:      name,
		Nodes:     nodesCopy,
		Edges:     edgesCopy,
		EntryNode: entryNode,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	g.GraphHash = computeGraphHash(g)
	return g, nil
}

// VerifyWorkflowGraph recomputes the hash and returns (true, "") when it matches.
func VerifyWorkflowGraph(g WorkflowGraph) (bool, string) {
	stored := g.GraphHash
	g.GraphHash = ""
	expected := computeGraphHash(g)
	if stored != expected {
		return false, fmt.Sprintf("graph %q hash mismatch: stored=%s computed=%s",
			g.GraphID, stored, expected)
	}
	return true, ""
}

// FindNode returns the node with the given ID, or (zero, false) if absent.
func FindNode(g WorkflowGraph, nodeID string) (WorkflowNode, bool) {
	for _, n := range g.Nodes {
		if n.NodeID == nodeID {
			return n, true
		}
	}
	return WorkflowNode{}, false
}

// EdgesFrom returns all edges originating at fromNodeID.
func EdgesFrom(g WorkflowGraph, fromNodeID string) []WorkflowEdge {
	var out []WorkflowEdge
	for _, e := range g.Edges {
		if e.FromNode == fromNodeID {
			out = append(out, e)
		}
	}
	return out
}

// NextNode returns the destination node for a given (fromNode, edgeType) pair.
// Returns (zero, false) when no matching edge exists.
func NextNode(g WorkflowGraph, fromNodeID string, edgeType EdgeType) (WorkflowNode, bool) {
	for _, e := range g.Edges {
		if e.FromNode == fromNodeID && e.EdgeType == edgeType {
			return FindNode(g, e.ToNode)
		}
	}
	return WorkflowNode{}, false
}

// ReachableNodeIDs returns all node IDs reachable from the entry node via BFS.
func ReachableNodeIDs(g WorkflowGraph) []string {
	visited := make(map[string]struct{})
	queue := []string{g.EntryNode}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if _, seen := visited[cur]; seen {
			continue
		}
		visited[cur] = struct{}{}
		for _, e := range EdgesFrom(g, cur) {
			if _, seen := visited[e.ToNode]; !seen {
				queue = append(queue, e.ToNode)
			}
		}
	}
	ids := make([]string, 0, len(visited))
	for id := range visited {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func computeGraphHash(g WorkflowGraph) string {
	h := sha256.New()
	fmt.Fprintf(h, "graph:%s|tenant:%s|name:%s|entry:%s|nodes:",
		g.GraphID, g.TenantID, g.Name, g.EntryNode)
	for _, n := range g.Nodes {
		fmt.Fprintf(h, "%s/%s,", n.NodeID, string(n.NodeType))
	}
	fmt.Fprintf(h, "|edges:")
	for _, e := range g.Edges {
		fmt.Fprintf(h, "%s/%s/%s/%s,", e.EdgeID, e.FromNode, e.ToNode, string(e.EdgeType))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
