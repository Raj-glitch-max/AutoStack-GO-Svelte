package runtime

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/janlauber/one-click/pkg/events"
)

// RuntimeFederationSnapshot is a point-in-time tamper-evident export of the full
// coordination state: nodes, leases, partition maps, transfer history, heartbeats,
// and quarantined nodes.
//
// GeneratedAt and FederationHash are excluded from the hash input so that two
// exports of identical state produce the same hash (content-identity).
type RuntimeFederationSnapshot struct {
	SnapshotID       string                  `json:"snapshot_id"`
	TenantID         string                  `json:"tenant_id"`
	Nodes            []RuntimeNode           `json:"nodes"`
	ActiveLeases     []ExecutionLease        `json:"active_leases"`
	PartitionMaps    []ExecutionPartitionMap  `json:"partition_maps"`
	TransferHistory  []LeaseTransferRecord   `json:"transfer_history"`
	Heartbeats       []RuntimeHeartbeat      `json:"heartbeats"`
	QuarantinedNodes []string                `json:"quarantined_nodes"`
	GeneratedAt      string                  `json:"generated_at"`
	FederationHash   string                  `json:"federation_hash"`
}

// BuildFederationSnapshot constructs and hashes a federation snapshot.
// All slices are deep-copied.
func BuildFederationSnapshot(
	snapshotID string,
	tenantID string,
	nodes []RuntimeNode,
	leases []ExecutionLease,
	partitionMaps []ExecutionPartitionMap,
	transferHistory []LeaseTransferRecord,
	heartbeats []RuntimeHeartbeat,
	quarantinedNodeIDs []string,
) RuntimeFederationSnapshot {
	snap := RuntimeFederationSnapshot{
		SnapshotID:       snapshotID,
		TenantID:         tenantID,
		Nodes:            deepCopyNodes(nodes),
		ActiveLeases:     deepCopyLeases(leases),
		PartitionMaps:    deepCopyPartitionMaps(partitionMaps),
		TransferHistory:  deepCopyTransfers(transferHistory),
		Heartbeats:       deepCopyHeartbeats(heartbeats),
		QuarantinedNodes: sortedStringCopy(quarantinedNodeIDs),
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	snap.FederationHash = computeFederationHash(snap)
	return snap
}

// VerifyFederationSnapshot recomputes the hash and compares it to the stored value.
func VerifyFederationSnapshot(snap RuntimeFederationSnapshot) (bool, string) {
	stored := snap.FederationHash
	snap.FederationHash = ""
	expected := computeFederationHash(snap)
	if stored != expected {
		return false, fmt.Sprintf("federation hash mismatch: stored=%s computed=%s", stored, expected)
	}
	return true, ""
}

// QuarantineNode transitions a node to NodeStateQuarantined within a snapshot and
// emits a PlatformEvent. Returns a new snapshot; the original is not modified.
func QuarantineNode(
	snap RuntimeFederationSnapshot,
	nodeID string,
	reason string,
	eventID string,
	clock int64,
) (RuntimeFederationSnapshot, events.PlatformEvent, error) {
	updatedNodes := deepCopyNodes(snap.Nodes)
	found := false
	for i, n := range updatedNodes {
		if n.NodeID == nodeID {
			if n.State == NodeStateQuarantined {
				return snap, events.PlatformEvent{}, fmt.Errorf("node %q is already quarantined", nodeID)
			}
			updatedNodes[i].State = NodeStateQuarantined
			updatedNodes[i].StateChangedAt = time.Now().UTC().Format(time.RFC3339Nano)
			found = true
			break
		}
	}
	if !found {
		return snap, events.PlatformEvent{}, fmt.Errorf("node %q not found in federation snapshot", nodeID)
	}
	newSnap := snap
	newSnap.Nodes = updatedNodes
	newSnap.QuarantinedNodes = sortedStringCopy(append(snap.QuarantinedNodes, nodeID))
	newSnap.GeneratedAt = time.Now().UTC().Format(time.RFC3339Nano)
	newSnap.FederationHash = computeFederationHash(newSnap)

	payload, _ := json.Marshal(map[string]any{
		"node_id": nodeID,
		"reason":  reason,
	})
	evt := events.NewPlatformEvent(
		eventID, EventTypeNodeQuarantined, AggregateTypeRuntime,
		nodeID, "", snap.TenantID, clock,
		"", "", payload, nil,
	)
	return newSnap, evt, nil
}

func computeFederationHash(snap RuntimeFederationSnapshot) string {
	h := sha256.New()
	fmt.Fprintf(h, "snap:%s|tenant:%s|nodes:", snap.SnapshotID, snap.TenantID)
	for _, n := range snap.Nodes {
		fmt.Fprintf(h, "%s/%s,", n.NodeID, n.State)
	}
	fmt.Fprintf(h, "|leases:")
	for _, l := range snap.ActiveLeases {
		fmt.Fprintf(h, "%s/%s/%d,", l.LeaseID, l.OwnerNodeID, l.LeaseEpoch)
	}
	fmt.Fprintf(h, "|transfers:")
	for _, t := range snap.TransferHistory {
		fmt.Fprintf(h, "%s/%s->%s/%d,", t.RecordID, t.FromNodeID, t.ToNodeID, t.TransferEpoch)
	}
	fmt.Fprintf(h, "|heartbeats:")
	for _, hb := range snap.Heartbeats {
		fmt.Fprintf(h, "%s/%d,", hb.NodeID, hb.LogicalClock)
	}
	fmt.Fprintf(h, "|quarantined:")
	for _, qn := range snap.QuarantinedNodes {
		fmt.Fprintf(h, "%s,", qn)
	}
	fmt.Fprintf(h, "|pmaps:")
	for _, pm := range snap.PartitionMaps {
		fmt.Fprintf(h, "%s/%s,", pm.MapID, pm.MapHash)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func deepCopyNodes(ns []RuntimeNode) []RuntimeNode {
	out := make([]RuntimeNode, len(ns))
	for i, n := range ns {
		caps := make([]NodeCapability, len(n.Capabilities))
		copy(caps, n.Capabilities)
		meta := make(map[string]string, len(n.Metadata))
		for k, v := range n.Metadata {
			meta[k] = v
		}
		out[i] = n
		out[i].Capabilities = caps
		out[i].Metadata = meta
	}
	return out
}

func deepCopyLeases(ls []ExecutionLease) []ExecutionLease {
	out := make([]ExecutionLease, len(ls))
	copy(out, ls)
	return out
}

func deepCopyPartitionMaps(pms []ExecutionPartitionMap) []ExecutionPartitionMap {
	out := make([]ExecutionPartitionMap, len(pms))
	for i, pm := range pms {
		entries := make([]PartitionEntry, len(pm.Entries))
		copy(entries, pm.Entries)
		nodes := make([]string, len(pm.ActiveNodes))
		copy(nodes, pm.ActiveNodes)
		out[i] = pm
		out[i].Entries = entries
		out[i].ActiveNodes = nodes
	}
	return out
}

func deepCopyTransfers(ts []LeaseTransferRecord) []LeaseTransferRecord {
	out := make([]LeaseTransferRecord, len(ts))
	copy(out, ts)
	return out
}

func deepCopyHeartbeats(hbs []RuntimeHeartbeat) []RuntimeHeartbeat {
	out := make([]RuntimeHeartbeat, len(hbs))
	copy(out, hbs)
	return out
}
