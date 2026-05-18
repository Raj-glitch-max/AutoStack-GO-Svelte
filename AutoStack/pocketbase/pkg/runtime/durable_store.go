package runtime

import (
	"sync"
	"time"
)

// FederationDurableStore is a thread-safe, in-memory store for all runtime
// coordination state. It is the authoritative in-process substrate for:
//   - RuntimeNodes and their state transitions
//   - ExecutionLeases and ownership mapping
//   - RuntimeHeartbeats per node
//   - ExecutionPartitionMaps
//   - Append-only LeaseTransferHistory
//   - Quarantine set
//
// Durability across restarts requires serializing the store to a DurabilitySnapshot
// and persisting it to a database backend. The serialization/deserialization is
// handled by ExportFederationSnapshot() and RestoreFromFederationSnapshot().
type FederationDurableStore struct {
	mu              sync.RWMutex
	nodes           map[string]RuntimeNode
	leases          map[string]ExecutionLease      // leaseID → lease
	heartbeats      map[string][]RuntimeHeartbeat  // nodeID → heartbeats (append-only)
	partitionMaps   map[string]ExecutionPartitionMap
	transferHistory []LeaseTransferRecord
	quarantined     map[string]struct{}
}

// NewFederationDurableStore returns an empty FederationDurableStore.
func NewFederationDurableStore() *FederationDurableStore {
	return &FederationDurableStore{
		nodes:         make(map[string]RuntimeNode),
		leases:        make(map[string]ExecutionLease),
		heartbeats:    make(map[string][]RuntimeHeartbeat),
		partitionMaps: make(map[string]ExecutionPartitionMap),
		quarantined:   make(map[string]struct{}),
	}
}

// ─── nodes ─────────────────────────────────────────────────────────────────

// SaveNode stores or replaces a RuntimeNode.
func (s *FederationDurableStore) SaveNode(node RuntimeNode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[node.NodeID] = node
}

// LoadNode returns the RuntimeNode for nodeID, or (zero, false) if absent.
func (s *FederationDurableStore) LoadNode(nodeID string) (RuntimeNode, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.nodes[nodeID]
	return n, ok
}

// AllNodes returns all stored nodes in sorted order.
func (s *FederationDurableStore) AllNodes() []RuntimeNode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RuntimeNode, 0, len(s.nodes))
	for _, n := range s.nodes {
		out = append(out, n)
	}
	sortNodesByID(out)
	return out
}

// ─── leases ────────────────────────────────────────────────────────────────

// SaveLease stores or replaces an ExecutionLease.
// Higher-epoch leases always replace lower-epoch leases for the same leaseID.
func (s *FederationDurableStore) SaveLease(lease ExecutionLease) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.leases[lease.LeaseID]
	if !ok || lease.LeaseEpoch >= existing.LeaseEpoch {
		s.leases[lease.LeaseID] = lease
	}
}

// LoadLease returns the lease for leaseID, or (zero, false) if absent.
func (s *FederationDurableStore) LoadLease(leaseID string) (ExecutionLease, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	l, ok := s.leases[leaseID]
	return l, ok
}

// AllLeases returns all stored leases.
func (s *FederationDurableStore) AllLeases() []ExecutionLease {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ExecutionLease, 0, len(s.leases))
	for _, l := range s.leases {
		out = append(out, l)
	}
	sortLeasesByEpoch(out)
	return out
}

// ActiveLeases returns leases whose ExpiresAt is after now.
func (s *FederationDurableStore) ActiveLeases(now string) []ExecutionLease {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ExecutionLease
	for _, l := range s.leases {
		if !IsLeaseExpired(l, now) {
			out = append(out, l)
		}
	}
	sortLeasesByEpoch(out)
	return out
}

// ─── heartbeats ────────────────────────────────────────────────────────────

// SaveHeartbeat appends a heartbeat to the node's history (append-only).
func (s *FederationDurableStore) SaveHeartbeat(hb RuntimeHeartbeat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.heartbeats[hb.NodeID] = append(s.heartbeats[hb.NodeID], hb)
}

// LatestHeartbeatForNode returns the heartbeat with the highest LogicalClock for nodeID.
func (s *FederationDurableStore) LatestHeartbeatForNode(nodeID string) (RuntimeHeartbeat, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	hbs := s.heartbeats[nodeID]
	if len(hbs) == 0 {
		return RuntimeHeartbeat{}, false
	}
	return LatestHeartbeat(hbs), true
}

// AllHeartbeats returns all heartbeats across all nodes, sorted by LogicalClock.
func (s *FederationDurableStore) AllHeartbeats() []RuntimeHeartbeat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var all []RuntimeHeartbeat
	for _, hbs := range s.heartbeats {
		all = append(all, hbs...)
	}
	return SortHeartbeats(all)
}

// ─── partition maps ────────────────────────────────────────────────────────

// SavePartitionMap stores or replaces a partition map.
func (s *FederationDurableStore) SavePartitionMap(pm ExecutionPartitionMap) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.partitionMaps[pm.MapID] = pm
}

// LoadPartitionMap returns the partition map for mapID, or (zero, false) if absent.
func (s *FederationDurableStore) LoadPartitionMap(mapID string) (ExecutionPartitionMap, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pm, ok := s.partitionMaps[mapID]
	return pm, ok
}

// AllPartitionMaps returns all stored partition maps.
func (s *FederationDurableStore) AllPartitionMaps() []ExecutionPartitionMap {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ExecutionPartitionMap, 0, len(s.partitionMaps))
	for _, pm := range s.partitionMaps {
		out = append(out, pm)
	}
	return out
}

// ─── transfer history ──────────────────────────────────────────────────────

// AppendTransfer adds a transfer record to the append-only history.
func (s *FederationDurableStore) AppendTransfer(record LeaseTransferRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transferHistory = append(s.transferHistory, record)
}

// TransferHistory returns a copy of all transfer records in order.
func (s *FederationDurableStore) TransferHistory() []LeaseTransferRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LeaseTransferRecord, len(s.transferHistory))
	copy(out, s.transferHistory)
	return out
}

// ─── quarantine ────────────────────────────────────────────────────────────

// Quarantine marks nodeID as quarantined in the store and updates its state.
func (s *FederationDurableStore) Quarantine(nodeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quarantined[nodeID] = struct{}{}
	if n, ok := s.nodes[nodeID]; ok {
		n.State = NodeStateQuarantined
		n.StateChangedAt = time.Now().UTC().Format(time.RFC3339Nano)
		s.nodes[nodeID] = n
	}
}

// IsQuarantined returns true when nodeID is in the quarantine set.
func (s *FederationDurableStore) IsQuarantined(nodeID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.quarantined[nodeID]
	return ok
}

// QuarantinedNodeIDs returns a sorted list of quarantined node IDs.
func (s *FederationDurableStore) QuarantinedNodeIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.quarantined))
	for id := range s.quarantined {
		ids = append(ids, id)
	}
	return sortedStringCopy(ids)
}

// ─── snapshot integration ──────────────────────────────────────────────────

// ExportFederationSnapshot builds a RuntimeFederationSnapshot from current store state.
func (s *FederationDurableStore) ExportFederationSnapshot(snapshotID string, tenantID string) RuntimeFederationSnapshot {
	return BuildFederationSnapshot(
		snapshotID,
		tenantID,
		s.AllNodes(),
		s.AllLeases(),
		s.AllPartitionMaps(),
		s.TransferHistory(),
		s.AllHeartbeats(),
		s.QuarantinedNodeIDs(),
	)
}

// RestoreFromFederationSnapshot populates the store from a previously exported snapshot.
// Existing state is replaced entirely.
func (s *FederationDurableStore) RestoreFromFederationSnapshot(snap RuntimeFederationSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes = make(map[string]RuntimeNode, len(snap.Nodes))
	for _, n := range snap.Nodes {
		s.nodes[n.NodeID] = n
	}
	s.leases = make(map[string]ExecutionLease, len(snap.ActiveLeases))
	for _, l := range snap.ActiveLeases {
		s.leases[l.LeaseID] = l
	}
	s.partitionMaps = make(map[string]ExecutionPartitionMap, len(snap.PartitionMaps))
	for _, pm := range snap.PartitionMaps {
		s.partitionMaps[pm.MapID] = pm
	}
	s.transferHistory = make([]LeaseTransferRecord, len(snap.TransferHistory))
	copy(s.transferHistory, snap.TransferHistory)
	s.heartbeats = make(map[string][]RuntimeHeartbeat)
	for _, hb := range snap.Heartbeats {
		s.heartbeats[hb.NodeID] = append(s.heartbeats[hb.NodeID], hb)
	}
	s.quarantined = make(map[string]struct{}, len(snap.QuarantinedNodes))
	for _, id := range snap.QuarantinedNodes {
		s.quarantined[id] = struct{}{}
	}
}

func sortNodesByID(ns []RuntimeNode) {
	for i := 1; i < len(ns); i++ {
		for j := i; j > 0 && ns[j].NodeID < ns[j-1].NodeID; j-- {
			ns[j], ns[j-1] = ns[j-1], ns[j]
		}
	}
}

func sortLeasesByEpoch(ls []ExecutionLease) {
	for i := 1; i < len(ls); i++ {
		for j := i; j > 0 && ls[j].LeaseEpoch < ls[j-1].LeaseEpoch; j-- {
			ls[j], ls[j-1] = ls[j-1], ls[j]
		}
	}
}
