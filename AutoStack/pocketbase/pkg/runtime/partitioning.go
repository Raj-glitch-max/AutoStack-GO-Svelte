package runtime

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// PartitionEntry maps one execution to one node within a partition map.
type PartitionEntry struct {
	ExecutionID    string `json:"execution_id"`
	TenantID       string `json:"tenant_id"`
	AssignedNodeID string `json:"assigned_node_id"`
	TopologyRegion string `json:"topology_region,omitempty"`
	Reason         string `json:"reason"`
}

// ExecutionPartitionMap is the deterministic assignment of executions to nodes.
// Given identical inputs (same executionIDs, same nodeIDs, same tenantID, same mapID),
// the resulting MapHash is always identical.
type ExecutionPartitionMap struct {
	MapID       string           `json:"map_id"`
	TenantID    string           `json:"tenant_id"`
	Entries     []PartitionEntry `json:"entries"`
	BuiltAt     string           `json:"built_at"`
	ActiveNodes []string         `json:"active_nodes"` // sorted snapshot of nodes used to build
	MapHash     string           `json:"map_hash"`
}

// BuildPartitionMap assigns each executionID to a node deterministically.
// Assignment: SHA-256(executionID + ":" + tenantID) mod len(activeNodes).
// Both executionIDs and activeNodes are sorted internally before use.
func BuildPartitionMap(mapID, tenantID string, executionIDs, activeNodes []string) (ExecutionPartitionMap, error) {
	if len(activeNodes) == 0 {
		return ExecutionPartitionMap{}, fmt.Errorf("activeNodes must not be empty")
	}
	if mapID == "" || tenantID == "" {
		return ExecutionPartitionMap{}, fmt.Errorf("mapID and tenantID are required")
	}
	sortedExecs := sortedStringCopy(executionIDs)
	sortedNodes := sortedStringCopy(activeNodes)
	entries := make([]PartitionEntry, 0, len(sortedExecs))
	for _, execID := range sortedExecs {
		node := assignNode(execID, tenantID, sortedNodes)
		entries = append(entries, PartitionEntry{
			ExecutionID:    execID,
			TenantID:       tenantID,
			AssignedNodeID: node,
			Reason:         "hash_assignment",
		})
	}
	nodesCopy := make([]string, len(sortedNodes))
	copy(nodesCopy, sortedNodes)
	pm := ExecutionPartitionMap{
		MapID:       mapID,
		TenantID:    tenantID,
		Entries:     entries,
		BuiltAt:     time.Now().UTC().Format(time.RFC3339Nano),
		ActiveNodes: nodesCopy,
	}
	pm.MapHash = computePartitionMapHash(pm)
	return pm, nil
}

// LookupPartition returns the assigned node for an execution, or ("", false) if not found.
func LookupPartition(pm ExecutionPartitionMap, executionID string) (string, bool) {
	for _, e := range pm.Entries {
		if e.ExecutionID == executionID {
			return e.AssignedNodeID, true
		}
	}
	return "", false
}

// IsStalePartitionMap returns true when any assigned node is absent from currentActiveNodes.
func IsStalePartitionMap(pm ExecutionPartitionMap, currentActiveNodes []string) bool {
	active := make(map[string]struct{}, len(currentActiveNodes))
	for _, n := range currentActiveNodes {
		active[n] = struct{}{}
	}
	for _, e := range pm.Entries {
		if _, ok := active[e.AssignedNodeID]; !ok {
			return true
		}
	}
	return false
}

// VerifyPartitionMapHash checks whether the stored hash matches the recomputed hash.
func VerifyPartitionMapHash(pm ExecutionPartitionMap) (bool, string) {
	stored := pm.MapHash
	pm.MapHash = ""
	expected := computePartitionMapHash(pm)
	if stored != expected {
		return false, fmt.Sprintf("partition map hash mismatch: stored=%s computed=%s", stored, expected)
	}
	return true, ""
}

func assignNode(executionID, tenantID string, sortedNodes []string) string {
	h := sha256.Sum256([]byte(executionID + ":" + tenantID))
	var n uint64
	for i := 0; i < 8; i++ {
		n = n<<8 | uint64(h[i])
	}
	return sortedNodes[n%uint64(len(sortedNodes))]
}

func computePartitionMapHash(pm ExecutionPartitionMap) string {
	h := sha256.New()
	// BuiltAt is excluded so two builds of the same partition produce the same hash (content-identity).
	fmt.Fprintf(h, "map:%s|tenant:%s|nodes:", pm.MapID, pm.TenantID)
	for _, n := range pm.ActiveNodes {
		fmt.Fprintf(h, "%s,", n)
	}
	fmt.Fprintf(h, "|entries:")
	for _, e := range pm.Entries {
		fmt.Fprintf(h, "%s->%s,", e.ExecutionID, e.AssignedNodeID)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// sortedStringCopy returns a sorted copy of ss without modifying the original.
func sortedStringCopy(ss []string) []string {
	out := make([]string, len(ss))
	copy(out, ss)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
