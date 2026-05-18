package runtime

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// RuntimeSnapshot is a tamper-evident, point-in-time export of all runtime coordination
// state for one tenant. Used for operator review, audit, and forensic replay.
//
// GeneratedAt and SnapshotHash are excluded from the hash input (content-identity).
type RuntimeSnapshot struct {
	SnapshotID      string                 `json:"snapshot_id"`
	TenantID        string                 `json:"tenant_id"`
	Nodes           []RuntimeNode          `json:"nodes"`
	ActiveLeases    []ExecutionLease       `json:"active_leases"`
	OwnershipMap    []OwnershipRecord      `json:"ownership_map"`
	Conflicts       []OwnershipConflict    `json:"ownership_conflicts"`
	Heartbeats      []RuntimeHeartbeat     `json:"heartbeats"`
	PartitionMap    *ExecutionPartitionMap `json:"partition_map,omitempty"`
	TransferHistory []LeaseTransferRecord  `json:"transfer_history"`
	GeneratedAt     string                 `json:"generated_at"`
	SnapshotHash    string                 `json:"snapshot_hash"`
}

// ExportRuntimeSnapshot builds and hashes a RuntimeSnapshot from current coordination state.
// Ownership map and conflict list are derived from the provided leases.
// All slices are deep-copied.
func ExportRuntimeSnapshot(
	snapshotID string,
	tenantID string,
	nodes []RuntimeNode,
	leases []ExecutionLease,
	heartbeats []RuntimeHeartbeat,
	partitionMap *ExecutionPartitionMap,
	transferHistory []LeaseTransferRecord,
) RuntimeSnapshot {
	ownerMap := BuildOwnershipMap(leases)
	ownerSlice := make([]OwnershipRecord, 0, len(ownerMap))
	for _, r := range ownerMap {
		ownerSlice = append(ownerSlice, r)
	}
	sortOwnershipRecords(ownerSlice)

	conflicts := DetectDuplicateOwnership(leases)

	var pm *ExecutionPartitionMap
	if partitionMap != nil {
		entries := make([]PartitionEntry, len(partitionMap.Entries))
		copy(entries, partitionMap.Entries)
		activeNodes := make([]string, len(partitionMap.ActiveNodes))
		copy(activeNodes, partitionMap.ActiveNodes)
		cp := *partitionMap
		cp.Entries = entries
		cp.ActiveNodes = activeNodes
		pm = &cp
	}

	snap := RuntimeSnapshot{
		SnapshotID:      snapshotID,
		TenantID:        tenantID,
		Nodes:           deepCopyNodes(nodes),
		ActiveLeases:    deepCopyLeases(leases),
		OwnershipMap:    ownerSlice,
		Conflicts:       conflicts,
		Heartbeats:      deepCopyHeartbeats(heartbeats),
		PartitionMap:    pm,
		TransferHistory: deepCopyTransfers(transferHistory),
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	snap.SnapshotHash = computeRuntimeSnapshotHash(snap)
	return snap
}

// VerifyRuntimeSnapshot recomputes the hash and compares it to the stored value.
func VerifyRuntimeSnapshot(snap RuntimeSnapshot) (bool, string) {
	stored := snap.SnapshotHash
	snap.SnapshotHash = ""
	expected := computeRuntimeSnapshotHash(snap)
	if stored != expected {
		return false, fmt.Sprintf("runtime snapshot hash mismatch: stored=%s computed=%s", stored, expected)
	}
	return true, ""
}

func computeRuntimeSnapshotHash(snap RuntimeSnapshot) string {
	h := sha256.New()
	fmt.Fprintf(h, "snap:%s|tenant:%s|nodes:", snap.SnapshotID, snap.TenantID)
	for _, n := range snap.Nodes {
		fmt.Fprintf(h, "%s/%s,", n.NodeID, n.State)
	}
	fmt.Fprintf(h, "|leases:")
	for _, l := range snap.ActiveLeases {
		fmt.Fprintf(h, "%s/%s/%d,", l.LeaseID, l.OwnerNodeID, l.LeaseEpoch)
	}
	fmt.Fprintf(h, "|ownership:")
	for _, r := range snap.OwnershipMap {
		fmt.Fprintf(h, "%s->%s/%d,", r.ExecutionID, r.OwnerNodeID, r.LeaseEpoch)
	}
	fmt.Fprintf(h, "|conflicts:%d|heartbeats:", len(snap.Conflicts))
	for _, hb := range snap.Heartbeats {
		fmt.Fprintf(h, "%s/%d,", hb.NodeID, hb.LogicalClock)
	}
	fmt.Fprintf(h, "|transfers:")
	for _, t := range snap.TransferHistory {
		fmt.Fprintf(h, "%s/%s->%s/%d,", t.RecordID, t.FromNodeID, t.ToNodeID, t.TransferEpoch)
	}
	if snap.PartitionMap != nil {
		fmt.Fprintf(h, "|pmap:%s/%s", snap.PartitionMap.MapID, snap.PartitionMap.MapHash)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
