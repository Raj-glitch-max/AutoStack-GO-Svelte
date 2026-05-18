package runtime

import (
	"testing"
	"time"
)

// ─── helpers local to durable_store_test ──────────────────────────────────

func makeLeaseDS(t *testing.T, leaseID, executionID, ownerNodeID, tenantID string, epoch int64) ExecutionLease {
	t.Helper()
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	lease, err := NewExecutionLease(leaseID, executionID, ownerNodeID, tenantID, epoch, expires)
	if err != nil {
		t.Fatalf("NewExecutionLease: %v", err)
	}
	return lease
}

func makeHeartbeatDS(nodeID, tenantID string, clock int64) RuntimeHeartbeat {
	return NewHeartbeat(nodeID, tenantID, clock, 0.1, 0, 0, 0, 1)
}

// ─── nodes ─────────────────────────────────────────────────────────────────

func TestFederationDurableStore_SaveAndLoadNode(t *testing.T) {
	s := NewFederationDurableStore()
	node := makeNode(t, "node-1", NodeStateActive)
	s.SaveNode(node)

	loaded, ok := s.LoadNode("node-1")
	if !ok {
		t.Fatal("node must be found after save")
	}
	if loaded.NodeID != "node-1" {
		t.Fatalf("NodeID mismatch: %s", loaded.NodeID)
	}
}

func TestFederationDurableStore_LoadNode_Missing(t *testing.T) {
	s := NewFederationDurableStore()
	_, ok := s.LoadNode("no-such-node")
	if ok {
		t.Fatal("missing node must return false")
	}
}

func TestFederationDurableStore_AllNodes_Sorted(t *testing.T) {
	s := NewFederationDurableStore()
	s.SaveNode(makeNode(t, "node-c", NodeStateActive))
	s.SaveNode(makeNode(t, "node-a", NodeStateActive))
	s.SaveNode(makeNode(t, "node-b", NodeStateActive))

	nodes := s.AllNodes()
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}
	if nodes[0].NodeID != "node-a" || nodes[1].NodeID != "node-b" || nodes[2].NodeID != "node-c" {
		t.Fatalf("nodes must be sorted by NodeID: got %v", nodeIDsDS(nodes))
	}
}

func TestFederationDurableStore_SaveNode_Replaces(t *testing.T) {
	s := NewFederationDurableStore()
	node := makeNode(t, "node-1", NodeStateActive)
	s.SaveNode(node)

	node.State = NodeStateDraining
	s.SaveNode(node)

	loaded, _ := s.LoadNode("node-1")
	if loaded.State != NodeStateDraining {
		t.Fatalf("expected draining state, got %s", loaded.State)
	}
}

// ─── leases ────────────────────────────────────────────────────────────────

func TestFederationDurableStore_SaveAndLoadLease(t *testing.T) {
	s := NewFederationDurableStore()
	lease := makeLeaseDS(t, "lease-1", "exec-1", "node-1", "t1", 1)
	s.SaveLease(lease)

	loaded, ok := s.LoadLease("lease-1")
	if !ok {
		t.Fatal("lease must be found after save")
	}
	if loaded.LeaseEpoch != 1 {
		t.Fatalf("LeaseEpoch mismatch: %d", loaded.LeaseEpoch)
	}
}

func TestFederationDurableStore_LoadLease_Missing(t *testing.T) {
	s := NewFederationDurableStore()
	_, ok := s.LoadLease("no-such-lease")
	if ok {
		t.Fatal("missing lease must return false")
	}
}

func TestFederationDurableStore_SaveLease_HigherEpochWins(t *testing.T) {
	s := NewFederationDurableStore()
	low := makeLeaseDS(t, "lease-1", "exec-1", "node-1", "t1", 1)
	high := makeLeaseDS(t, "lease-1", "exec-1", "node-2", "t1", 5)

	s.SaveLease(low)
	s.SaveLease(high)

	loaded, _ := s.LoadLease("lease-1")
	if loaded.LeaseEpoch != 5 {
		t.Fatalf("higher epoch must win: got epoch %d", loaded.LeaseEpoch)
	}
	if loaded.OwnerNodeID != "node-2" {
		t.Fatalf("owner must be node-2: got %s", loaded.OwnerNodeID)
	}
}

func TestFederationDurableStore_SaveLease_LowerEpochIgnored(t *testing.T) {
	s := NewFederationDurableStore()
	high := makeLeaseDS(t, "lease-1", "exec-1", "node-2", "t1", 5)
	low := makeLeaseDS(t, "lease-1", "exec-1", "node-1", "t1", 1)

	s.SaveLease(high)
	s.SaveLease(low)

	loaded, _ := s.LoadLease("lease-1")
	if loaded.LeaseEpoch != 5 {
		t.Fatalf("lower epoch must be ignored: got epoch %d", loaded.LeaseEpoch)
	}
}

func TestFederationDurableStore_ActiveLeases(t *testing.T) {
	s := NewFederationDurableStore()
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)

	expiredLease, _ := NewExecutionLease("lease-expired", "exec-x", "node-1", "t1", 1, past)
	activeLease, _ := NewExecutionLease("lease-active", "exec-y", "node-1", "t1", 1, future)
	s.SaveLease(expiredLease)
	s.SaveLease(activeLease)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	active := s.ActiveLeases(now)
	if len(active) != 1 {
		t.Fatalf("expected 1 active lease, got %d", len(active))
	}
	if active[0].LeaseID != "lease-active" {
		t.Fatalf("wrong active lease: %s", active[0].LeaseID)
	}
}

// ─── heartbeats ────────────────────────────────────────────────────────────

func TestFederationDurableStore_SaveHeartbeat_AppendOnly(t *testing.T) {
	s := NewFederationDurableStore()
	s.SaveHeartbeat(makeHeartbeatDS("node-1", "t1", 1))
	s.SaveHeartbeat(makeHeartbeatDS("node-1", "t1", 2))
	s.SaveHeartbeat(makeHeartbeatDS("node-1", "t1", 3))

	all := s.AllHeartbeats()
	if len(all) != 3 {
		t.Fatalf("expected 3 heartbeats, got %d", len(all))
	}
}

func TestFederationDurableStore_LatestHeartbeatForNode(t *testing.T) {
	s := NewFederationDurableStore()
	s.SaveHeartbeat(makeHeartbeatDS("node-1", "t1", 1))
	s.SaveHeartbeat(makeHeartbeatDS("node-1", "t1", 5))
	s.SaveHeartbeat(makeHeartbeatDS("node-1", "t1", 3))

	latest, ok := s.LatestHeartbeatForNode("node-1")
	if !ok {
		t.Fatal("heartbeat must be found")
	}
	if latest.LogicalClock != 5 {
		t.Fatalf("expected clock 5, got %d", latest.LogicalClock)
	}
}

func TestFederationDurableStore_LatestHeartbeat_Missing(t *testing.T) {
	s := NewFederationDurableStore()
	_, ok := s.LatestHeartbeatForNode("ghost-node")
	if ok {
		t.Fatal("missing node must return false")
	}
}

func TestFederationDurableStore_AllHeartbeats_Sorted(t *testing.T) {
	s := NewFederationDurableStore()
	s.SaveHeartbeat(makeHeartbeatDS("node-a", "t1", 10))
	s.SaveHeartbeat(makeHeartbeatDS("node-b", "t1", 2))
	s.SaveHeartbeat(makeHeartbeatDS("node-a", "t1", 5))

	all := s.AllHeartbeats()
	if len(all) != 3 {
		t.Fatalf("expected 3 heartbeats, got %d", len(all))
	}
	if all[0].LogicalClock > all[1].LogicalClock || all[1].LogicalClock > all[2].LogicalClock {
		t.Fatalf("heartbeats must be sorted ascending by clock: got %v", clocksDS(all))
	}
}

// ─── partition maps ────────────────────────────────────────────────────────

func TestFederationDurableStore_SaveAndLoadPartitionMap(t *testing.T) {
	s := NewFederationDurableStore()
	pm, err := BuildPartitionMap("map-1", "t1", []string{"exec-1"}, []string{"node-1"})
	if err != nil {
		t.Fatalf("BuildPartitionMap: %v", err)
	}
	s.SavePartitionMap(pm)

	loaded, ok := s.LoadPartitionMap("map-1")
	if !ok {
		t.Fatal("partition map must be found after save")
	}
	if loaded.MapID != "map-1" {
		t.Fatalf("MapID mismatch: %s", loaded.MapID)
	}
}

func TestFederationDurableStore_LoadPartitionMap_Missing(t *testing.T) {
	s := NewFederationDurableStore()
	_, ok := s.LoadPartitionMap("no-such-map")
	if ok {
		t.Fatal("missing map must return false")
	}
}

// ─── transfer history ──────────────────────────────────────────────────────

func TestFederationDurableStore_TransferHistory_AppendOnly(t *testing.T) {
	s := NewFederationDurableStore()
	r1 := LeaseTransferRecord{RecordID: "r1", LeaseID: "lease-1", FromNodeID: "n1", ToNodeID: "n2", TransferEpoch: 2, Reason: "rebalance"}
	r2 := LeaseTransferRecord{RecordID: "r2", LeaseID: "lease-1", FromNodeID: "n2", ToNodeID: "n3", TransferEpoch: 3, Reason: "failover"}
	s.AppendTransfer(r1)
	s.AppendTransfer(r2)

	history := s.TransferHistory()
	if len(history) != 2 {
		t.Fatalf("expected 2 transfer records, got %d", len(history))
	}
	if history[0].RecordID != "r1" || history[1].RecordID != "r2" {
		t.Fatal("transfer history must preserve insertion order")
	}
}

func TestFederationDurableStore_TransferHistory_ReturnsCopy(t *testing.T) {
	s := NewFederationDurableStore()
	s.AppendTransfer(LeaseTransferRecord{RecordID: "r1"})

	h1 := s.TransferHistory()
	h1[0].RecordID = "mutated"

	h2 := s.TransferHistory()
	if h2[0].RecordID == "mutated" {
		t.Fatal("TransferHistory must return a copy, not the internal slice")
	}
}

// ─── quarantine ────────────────────────────────────────────────────────────

func TestFederationDurableStore_Quarantine_SetsFlag(t *testing.T) {
	s := NewFederationDurableStore()
	if s.IsQuarantined("node-x") {
		t.Fatal("node must not be quarantined initially")
	}
	s.Quarantine("node-x")
	if !s.IsQuarantined("node-x") {
		t.Fatal("node must be quarantined after Quarantine call")
	}
}

func TestFederationDurableStore_Quarantine_UpdatesNodeState(t *testing.T) {
	s := NewFederationDurableStore()
	node := makeNode(t, "node-1", NodeStateActive)
	s.SaveNode(node)
	s.Quarantine("node-1")

	loaded, _ := s.LoadNode("node-1")
	if loaded.State != NodeStateQuarantined {
		t.Fatalf("quarantined node must have quarantined state, got %s", loaded.State)
	}
}

func TestFederationDurableStore_Quarantine_WorksWithoutSavedNode(t *testing.T) {
	s := NewFederationDurableStore()
	s.Quarantine("ghost-node")
	if !s.IsQuarantined("ghost-node") {
		t.Fatal("ghost node must be quarantined even without a saved node record")
	}
}

func TestFederationDurableStore_QuarantinedNodeIDs_Sorted(t *testing.T) {
	s := NewFederationDurableStore()
	s.Quarantine("node-c")
	s.Quarantine("node-a")
	s.Quarantine("node-b")

	ids := s.QuarantinedNodeIDs()
	if len(ids) != 3 {
		t.Fatalf("expected 3 quarantined nodes, got %d", len(ids))
	}
	if ids[0] != "node-a" || ids[1] != "node-b" || ids[2] != "node-c" {
		t.Fatalf("quarantined IDs must be sorted: got %v", ids)
	}
}

// ─── snapshot round-trip ───────────────────────────────────────────────────

func TestFederationDurableStore_ExportAndRestore(t *testing.T) {
	s := NewFederationDurableStore()
	s.SaveNode(makeNode(t, "node-1", NodeStateActive))
	s.SaveLease(makeLeaseDS(t, "lease-1", "exec-1", "node-1", "t1", 1))
	s.SaveHeartbeat(makeHeartbeatDS("node-1", "t1", 10))
	s.Quarantine("node-99")

	snap := s.ExportFederationSnapshot("snap-1", "t1")
	if snap.SnapshotID != "snap-1" {
		t.Fatalf("SnapshotID mismatch: %s", snap.SnapshotID)
	}

	s2 := NewFederationDurableStore()
	s2.RestoreFromFederationSnapshot(snap)

	loadedNode, ok := s2.LoadNode("node-1")
	if !ok {
		t.Fatal("node must be restored")
	}
	if loadedNode.NodeID != "node-1" {
		t.Fatalf("restored NodeID mismatch: %s", loadedNode.NodeID)
	}

	loadedLease, ok := s2.LoadLease("lease-1")
	if !ok {
		t.Fatal("lease must be restored")
	}
	if loadedLease.LeaseEpoch != 1 {
		t.Fatalf("restored LeaseEpoch mismatch: %d", loadedLease.LeaseEpoch)
	}

	if !s2.IsQuarantined("node-99") {
		t.Fatal("quarantine set must be restored")
	}

	_, ok = s2.LatestHeartbeatForNode("node-1")
	if !ok {
		t.Fatal("heartbeat must be restored")
	}
}

func TestFederationDurableStore_RestoreReplacesExistingState(t *testing.T) {
	s := NewFederationDurableStore()
	s.SaveNode(makeNode(t, "old-node", NodeStateActive))

	s2 := NewFederationDurableStore()
	s2.SaveNode(makeNode(t, "new-node", NodeStateActive))
	snap := s2.ExportFederationSnapshot("snap-fresh", "t1")

	s.RestoreFromFederationSnapshot(snap)

	_, hasOld := s.LoadNode("old-node")
	_, hasNew := s.LoadNode("new-node")
	if hasOld {
		t.Fatal("restore must clear old nodes")
	}
	if !hasNew {
		t.Fatal("restore must load new nodes")
	}
}

func TestFederationDurableStore_ExportSnapshot_HashIntegrity(t *testing.T) {
	s := NewFederationDurableStore()
	s.SaveNode(makeNode(t, "node-1", NodeStateActive))
	snap := s.ExportFederationSnapshot("snap-1", "t1")

	ok, reason := VerifyFederationSnapshot(snap)
	if !ok {
		t.Fatalf("exported snapshot must have valid hash: %s", reason)
	}
}

// ─── concurrency sanity ────────────────────────────────────────────────────

func TestFederationDurableStore_ConcurrentSave(t *testing.T) {
	s := NewFederationDurableStore()
	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func(i int) {
			nodeID := string(rune('a' + i%10))
			n := NewRuntimeNode("node-"+nodeID, []NodeCapability{CapabilityScheduling})
			s.SaveNode(n)
			s.SaveHeartbeat(makeHeartbeatDS("node-"+nodeID, "t1", int64(i)))
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 20; i++ {
		<-done
	}
	nodes := s.AllNodes()
	if len(nodes) == 0 {
		t.Fatal("at least some nodes must be present after concurrent saves")
	}
}

// ─── local helpers ─────────────────────────────────────────────────────────

func nodeIDsDS(nodes []RuntimeNode) []string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.NodeID
	}
	return ids
}

func clocksDS(hbs []RuntimeHeartbeat) []int64 {
	cs := make([]int64, len(hbs))
	for i, hb := range hbs {
		cs[i] = hb.LogicalClock
	}
	return cs
}
