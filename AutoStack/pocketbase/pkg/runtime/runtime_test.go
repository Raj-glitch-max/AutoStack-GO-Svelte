package runtime

import (
	"strings"
	"testing"
	"time"

	"github.com/janlauber/one-click/pkg/events"
)

// ─── helpers ───────────────────────────────────────────────────────────────

func futureExpiry() string {
	return time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
}

func pastExpiry() string {
	return time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
}

func makeNode(t *testing.T, id string, state NodeState) RuntimeNode {
	t.Helper()
	n := NewRuntimeNode(id, []NodeCapability{CapabilityReplay})
	n.State = state
	return n
}

func mustLease(t *testing.T, id, execID, nodeID, tenantID string, epoch int64) ExecutionLease {
	t.Helper()
	l, err := NewExecutionLease(id, execID, nodeID, tenantID, epoch, futureExpiry())
	if err != nil {
		t.Fatalf("NewExecutionLease(%s): %v", id, err)
	}
	return l
}

// ─── RuntimeNode ───────────────────────────────────────────────────────────

func TestNewRuntimeNode_DefaultState(t *testing.T) {
	n := NewRuntimeNode("node-1", []NodeCapability{CapabilityReplay, CapabilityScheduling})
	if n.State != NodeStateStarting {
		t.Fatalf("new node must be in starting state, got %s", n.State)
	}
	if n.NodeID != "node-1" {
		t.Fatal("NodeID mismatch")
	}
	if n.JoinedAt == "" || n.StateChangedAt == "" {
		t.Fatal("timestamps must be set")
	}
}

func TestNewRuntimeNode_CapabilitiesDeepCopied(t *testing.T) {
	caps := []NodeCapability{CapabilityReplay}
	n := NewRuntimeNode("node-1", caps)
	caps[0] = CapabilityArchive
	if n.Capabilities[0] != CapabilityReplay {
		t.Fatal("capabilities must be deep-copied; mutation of original must not affect node")
	}
}

func TestHasCapability(t *testing.T) {
	n := NewRuntimeNode("n", []NodeCapability{CapabilityReplay, CapabilityGovernance})
	if !HasCapability(n, CapabilityReplay) {
		t.Fatal("HasCapability must return true for CapabilityReplay")
	}
	if HasCapability(n, CapabilityArchive) {
		t.Fatal("HasCapability must return false for CapabilityArchive")
	}
}

func TestIsActive(t *testing.T) {
	n := NewRuntimeNode("n", nil)
	if IsActive(n) {
		t.Fatal("starting node must not be active")
	}
	n.State = NodeStateActive
	if !IsActive(n) {
		t.Fatal("node in active state must be IsActive")
	}
}

func TestTransitionNodeState_ValidTransition(t *testing.T) {
	n := NewRuntimeNode("n", nil)
	updated, evt, err := TransitionNodeState(n, NodeStateActive, "health check passed", "evt-1", "tenant-1", 1)
	if err != nil {
		t.Fatalf("valid transition must not error: %v", err)
	}
	if updated.State != NodeStateActive {
		t.Fatal("state must be updated in returned node")
	}
	if n.State != NodeStateStarting {
		t.Fatal("original node must not be mutated")
	}
	if evt.EventType != EventTypeNodeStateChanged {
		t.Fatalf("event type mismatch: %s", evt.EventType)
	}
}

func TestTransitionNodeState_InvalidTransition(t *testing.T) {
	n := NewRuntimeNode("n", nil)
	n.State = NodeStateStopped
	_, _, err := TransitionNodeState(n, NodeStateActive, "attempt", "evt-2", "tenant-1", 2)
	if err == nil {
		t.Fatal("stopped → active must be rejected")
	}
}

func TestTransitionNodeState_QuarantineEmitsCorrectEvent(t *testing.T) {
	n := makeNode(t, "n", NodeStateActive)
	_, evt, err := TransitionNodeState(n, NodeStateQuarantined, "misbehaving", "evt-3", "t1", 3)
	if err != nil {
		t.Fatalf("active → quarantined must be valid: %v", err)
	}
	if evt.EventType != EventTypeNodeQuarantined {
		t.Fatalf("quarantine transition must emit %s, got %s", EventTypeNodeQuarantined, evt.EventType)
	}
}

func TestCanAcquireLease_States(t *testing.T) {
	cases := []struct {
		state    NodeState
		expected bool
	}{
		{NodeStateActive, true},
		{NodeStateDraining, true},
		{NodeStateStarting, false},
		{NodeStatePartitioned, false},
		{NodeStateQuarantined, false},
		{NodeStateStopped, false},
	}
	for _, tc := range cases {
		n := RuntimeNode{State: tc.state}
		if CanAcquireLease(n) != tc.expected {
			t.Errorf("CanAcquireLease(%s) = %v, want %v", tc.state, !tc.expected, tc.expected)
		}
	}
}

func TestNodeHash_Deterministic(t *testing.T) {
	n := NewRuntimeNode("n1", []NodeCapability{CapabilityReplay, CapabilityArchive})
	if NodeHash(n) != NodeHash(n) {
		t.Fatal("NodeHash must be deterministic")
	}
}

func TestNodeHash_DifferentNodes(t *testing.T) {
	n1 := NewRuntimeNode("node-a", []NodeCapability{CapabilityReplay})
	n2 := NewRuntimeNode("node-b", []NodeCapability{CapabilityReplay})
	if NodeHash(n1) == NodeHash(n2) {
		t.Fatal("different nodes must produce different hashes")
	}
}

// ─── ExecutionLease ────────────────────────────────────────────────────────

func TestNewExecutionLease_Fields(t *testing.T) {
	l, err := NewExecutionLease("l1", "exec-1", "node-1", "tenant-1", 1, futureExpiry())
	if err != nil {
		t.Fatalf("NewExecutionLease must not error: %v", err)
	}
	if l.LeaseID != "l1" || l.ExecutionID != "exec-1" || l.OwnerNodeID != "node-1" {
		t.Fatal("fields must match inputs")
	}
	if l.LeaseEpoch != 1 {
		t.Fatalf("epoch must be 1, got %d", l.LeaseEpoch)
	}
	if l.AcquiredAt == "" {
		t.Fatal("AcquiredAt must be set")
	}
}

func TestNewExecutionLease_RequiredFields(t *testing.T) {
	if _, err := NewExecutionLease("", "exec-1", "node-1", "tenant-1", 1, futureExpiry()); err == nil {
		t.Fatal("empty leaseID must error")
	}
	if _, err := NewExecutionLease("l1", "exec-1", "node-1", "tenant-1", 1, ""); err == nil {
		t.Fatal("empty expiresAt must error")
	}
}

func TestIsLeaseExpired(t *testing.T) {
	l, _ := NewExecutionLease("l1", "exec-1", "node-1", "tenant-1", 1, pastExpiry())
	if !IsLeaseExpired(l, time.Now().UTC().Format(time.RFC3339Nano)) {
		t.Fatal("past expiry must be expired")
	}
	l2, _ := NewExecutionLease("l2", "exec-2", "node-1", "tenant-1", 1, futureExpiry())
	if IsLeaseExpired(l2, time.Now().UTC().Format(time.RFC3339Nano)) {
		t.Fatal("future expiry must not be expired")
	}
}

func TestTransferLease_ValueSemantics(t *testing.T) {
	orig := mustLease(t, "l1", "exec-1", "node-1", "tenant-1", 1)
	newLease, record, evt, err := TransferLease(orig, "node-2", "draining", "rec-1", "evt-10", 10)
	if err != nil {
		t.Fatalf("TransferLease must not error: %v", err)
	}
	if orig.OwnerNodeID != "node-1" {
		t.Fatal("original lease must not be mutated")
	}
	if newLease.OwnerNodeID != "node-2" {
		t.Fatal("new lease must have new owner")
	}
	if newLease.LeaseEpoch != orig.LeaseEpoch+1 {
		t.Fatalf("epoch must increment, got %d", newLease.LeaseEpoch)
	}
	if newLease.PreviousOwner != "node-1" {
		t.Fatal("PreviousOwner must be set")
	}
	if record.FromNodeID != "node-1" || record.ToNodeID != "node-2" {
		t.Fatal("transfer record must capture from/to correctly")
	}
	if evt.EventType != EventTypeLeaseTransferred {
		t.Fatalf("event type must be lease.transferred, got %s", evt.EventType)
	}
}

func TestTransferLease_RequiresReason(t *testing.T) {
	l := mustLease(t, "l1", "exec-1", "node-1", "tenant-1", 1)
	if _, _, _, err := TransferLease(l, "node-2", "", "r", "e", 1); err == nil {
		t.Fatal("TransferLease without reason must error")
	}
}

func TestTransferLease_RequiresToNodeID(t *testing.T) {
	l := mustLease(t, "l1", "exec-1", "node-1", "tenant-1", 1)
	if _, _, _, err := TransferLease(l, "", "reason", "r", "e", 1); err == nil {
		t.Fatal("TransferLease without toNodeID must error")
	}
}

func TestNewLeaseAcquiredEvent(t *testing.T) {
	l := mustLease(t, "l1", "exec-1", "node-1", "t1", 1)
	evt := NewLeaseAcquiredEvent(l, "evt-a", 5)
	if evt.EventType != EventTypeLeaseAcquired {
		t.Fatalf("event type must be lease.acquired, got %s", evt.EventType)
	}
	if evt.TenantID != "t1" {
		t.Fatal("event must carry tenantID")
	}
}

func TestNewLeaseExpiredEvent(t *testing.T) {
	l := mustLease(t, "l1", "exec-1", "node-1", "t1", 1)
	evt := NewLeaseExpiredEvent(l, "evt-e", 6)
	if evt.EventType != EventTypeLeaseExpired {
		t.Fatalf("event type must be lease.expired, got %s", evt.EventType)
	}
}

// ─── Coordination ──────────────────────────────────────────────────────────

func TestValidateLeaseAcquisition_QuarantinedNode(t *testing.T) {
	n := RuntimeNode{NodeID: "n1", State: NodeStateQuarantined}
	v := ValidateLeaseAcquisition(n, "exec-1", nil)
	if v == nil {
		t.Fatal("quarantined node must be rejected")
	}
	if !strings.Contains(v.ViolationType, "quarantined") {
		t.Fatalf("violation type must mention quarantine, got %s", v.ViolationType)
	}
}

func TestValidateLeaseAcquisition_PartitionedNode(t *testing.T) {
	n := RuntimeNode{NodeID: "n1", State: NodeStatePartitioned}
	v := ValidateLeaseAcquisition(n, "exec-1", nil)
	if v == nil {
		t.Fatal("partitioned node must be rejected")
	}
}

func TestValidateLeaseAcquisition_StoppedNode(t *testing.T) {
	n := RuntimeNode{NodeID: "n1", State: NodeStateStopped}
	v := ValidateLeaseAcquisition(n, "exec-1", nil)
	if v == nil {
		t.Fatal("stopped node must be rejected")
	}
}

func TestValidateLeaseAcquisition_DuplicateOwnership(t *testing.T) {
	n := RuntimeNode{NodeID: "n2", State: NodeStateActive}
	existing := mustLease(t, "l1", "exec-1", "n1", "tenant-1", 1)
	v := ValidateLeaseAcquisition(n, "exec-1", []ExecutionLease{existing})
	if v == nil {
		t.Fatal("execution already owned must produce violation")
	}
	if v.ViolationType != "execution_already_owned" {
		t.Fatalf("wrong violation type: %s", v.ViolationType)
	}
}

func TestValidateLeaseAcquisition_ActiveNode_NoExisting(t *testing.T) {
	n := RuntimeNode{NodeID: "n1", State: NodeStateActive}
	if v := ValidateLeaseAcquisition(n, "exec-1", nil); v != nil {
		t.Fatalf("active node with no existing lease must be allowed, got: %v", v)
	}
}

func TestValidateLeaseTransfer_NonOwner(t *testing.T) {
	lease := mustLease(t, "l1", "exec-1", "node-1", "tenant-1", 1)
	from := RuntimeNode{NodeID: "node-2", State: NodeStateActive}
	to := RuntimeNode{NodeID: "node-3", State: NodeStateActive}
	if v := ValidateLeaseTransfer(lease, from, to); v == nil {
		t.Fatal("non-owner transfer must be rejected")
	}
}

func TestValidateLeaseTransfer_QuarantinedFrom(t *testing.T) {
	lease := mustLease(t, "l1", "exec-1", "node-1", "tenant-1", 1)
	from := RuntimeNode{NodeID: "node-1", State: NodeStateQuarantined}
	to := RuntimeNode{NodeID: "node-2", State: NodeStateActive}
	if v := ValidateLeaseTransfer(lease, from, to); v == nil {
		t.Fatal("quarantined source node must be rejected")
	}
}

func TestValidateLeaseTransfer_IneligibleTarget(t *testing.T) {
	lease := mustLease(t, "l1", "exec-1", "node-1", "tenant-1", 1)
	from := RuntimeNode{NodeID: "node-1", State: NodeStateActive}
	to := RuntimeNode{NodeID: "node-2", State: NodeStateQuarantined}
	if v := ValidateLeaseTransfer(lease, from, to); v == nil {
		t.Fatal("quarantined target node must be rejected")
	}
}

func TestNewPartitionDetectedEvent(t *testing.T) {
	evt := NewPartitionDetectedEvent("n1", "t1", []string{"l1", "l2"}, "evt-p", 42)
	if evt.EventType != EventTypePartitionDetected {
		t.Fatalf("event type must be partition.detected, got %s", evt.EventType)
	}
	if evt.AggregateID != "n1" {
		t.Fatalf("aggregate ID must be nodeID, got %s", evt.AggregateID)
	}
}

// ─── Ownership ─────────────────────────────────────────────────────────────

func TestBuildOwnershipMap_HighestEpochWins(t *testing.T) {
	l1 := mustLease(t, "l1", "exec-1", "node-1", "tenant-1", 1)
	l2 := mustLease(t, "l2", "exec-1", "node-2", "tenant-1", 2)
	om := BuildOwnershipMap([]ExecutionLease{l1, l2})
	rec, ok := om["exec-1"]
	if !ok {
		t.Fatal("exec-1 must appear in ownership map")
	}
	if rec.OwnerNodeID != "node-2" {
		t.Fatalf("higher epoch must win, got %s", rec.OwnerNodeID)
	}
}

func TestBuildOwnershipMap_ConflictedExecOmitted(t *testing.T) {
	l1 := mustLease(t, "l1", "exec-1", "node-1", "tenant-1", 3)
	l2 := mustLease(t, "l2", "exec-1", "node-2", "tenant-1", 3)
	om := BuildOwnershipMap([]ExecutionLease{l1, l2})
	if _, ok := om["exec-1"]; ok {
		t.Fatal("conflicted execution (same epoch) must be omitted from ownership map")
	}
}

func TestDetectDuplicateOwnership_SameEpoch(t *testing.T) {
	l1 := mustLease(t, "l1", "exec-1", "node-1", "tenant-1", 3)
	l2 := mustLease(t, "l2", "exec-1", "node-2", "tenant-1", 3)
	conflicts := DetectDuplicateOwnership([]ExecutionLease{l1, l2})
	if len(conflicts) != 1 {
		t.Fatalf("same-epoch leases must produce 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].ExecutionID != "exec-1" {
		t.Fatalf("conflict must reference exec-1, got %s", conflicts[0].ExecutionID)
	}
}

func TestDetectDuplicateOwnership_DifferentEpochs_NoConflict(t *testing.T) {
	l1 := mustLease(t, "l1", "exec-1", "node-1", "tenant-1", 1)
	l2 := mustLease(t, "l2", "exec-1", "node-2", "tenant-1", 2)
	if cs := DetectDuplicateOwnership([]ExecutionLease{l1, l2}); len(cs) != 0 {
		t.Fatalf("different-epoch leases must not conflict, got %d conflicts", len(cs))
	}
}

func TestDetectDuplicateOwnership_SortedByExecutionID(t *testing.T) {
	la1 := mustLease(t, "la1", "exec-b", "n1", "t1", 1)
	la2 := mustLease(t, "la2", "exec-b", "n2", "t1", 1)
	lb1 := mustLease(t, "lb1", "exec-a", "n1", "t1", 5)
	lb2 := mustLease(t, "lb2", "exec-a", "n2", "t1", 5)
	conflicts := DetectDuplicateOwnership([]ExecutionLease{la1, la2, lb1, lb2})
	if len(conflicts) != 2 {
		t.Fatalf("expected 2 conflicts, got %d", len(conflicts))
	}
	if conflicts[0].ExecutionID != "exec-a" {
		t.Fatal("conflicts must be sorted by executionID")
	}
}

func TestAssertSingleOwner_NoOwner(t *testing.T) {
	if err := AssertSingleOwner("exec-1", nil); err == nil {
		t.Fatal("no owner must error")
	}
}

func TestAssertSingleOwner_MultipleOwners(t *testing.T) {
	l1 := mustLease(t, "l1", "exec-1", "n1", "t1", 1)
	l2 := mustLease(t, "l2", "exec-1", "n2", "t1", 1)
	if err := AssertSingleOwner("exec-1", []ExecutionLease{l1, l2}); err == nil {
		t.Fatal("two same-epoch leases must produce error")
	}
}

func TestAssertSingleOwner_OK(t *testing.T) {
	l := mustLease(t, "l1", "exec-1", "n1", "t1", 1)
	if err := AssertSingleOwner("exec-1", []ExecutionLease{l}); err != nil {
		t.Fatalf("single owner must not error: %v", err)
	}
}

func TestReplayOwnership_LatestEpochWins(t *testing.T) {
	l1 := mustLease(t, "l1", "exec-1", "node-1", "tenant-1", 1)
	acqEvt := NewLeaseAcquiredEvent(l1, "evt-acquire", 1)

	l2 := l1
	l2.OwnerNodeID = "node-2"
	l2.LeaseEpoch = 2
	transferEvt := NewLeaseAcquiredEvent(l2, "evt-transfer", 2)

	records := ReplayOwnership([]events.PlatformEvent{acqEvt, transferEvt})
	if len(records) != 1 {
		t.Fatalf("must produce 1 ownership record, got %d", len(records))
	}
	if records[0].OwnerNodeID != "node-2" {
		t.Fatalf("highest epoch must win in replay, got %s", records[0].OwnerNodeID)
	}
}

func TestReplayOwnership_MultipleTransfers(t *testing.T) {
	l, _ := NewExecutionLease("l1", "exec-1", "node-1", "t1", 1, futureExpiry())
	evt1 := NewLeaseAcquiredEvent(l, "e1", 1)

	l2 := l
	l2.OwnerNodeID = "node-2"
	l2.LeaseEpoch = 2
	evt2 := NewLeaseAcquiredEvent(l2, "e2", 2)

	l3 := l2
	l3.OwnerNodeID = "node-3"
	l3.LeaseEpoch = 3
	evt3 := NewLeaseAcquiredEvent(l3, "e3", 3)

	records := ReplayOwnership([]events.PlatformEvent{evt1, evt2, evt3})
	if len(records) != 1 {
		t.Fatalf("must produce 1 ownership record, got %d", len(records))
	}
	if records[0].OwnerNodeID != "node-3" {
		t.Fatalf("final owner must be node-3, got %s", records[0].OwnerNodeID)
	}
	if records[0].LeaseEpoch != 3 {
		t.Fatalf("final epoch must be 3, got %d", records[0].LeaseEpoch)
	}
}

func TestReplayOwnership_TransferEvent(t *testing.T) {
	l := mustLease(t, "l1", "exec-1", "node-1", "t1", 1)
	acqEvt := NewLeaseAcquiredEvent(l, "e1", 1)
	_, _, transferEvt, _ := TransferLease(l, "node-2", "draining", "rec-1", "e2", 2)

	records := ReplayOwnership([]events.PlatformEvent{acqEvt, transferEvt})
	if len(records) != 1 {
		t.Fatalf("must produce 1 ownership record, got %d", len(records))
	}
	if records[0].OwnerNodeID != "node-2" {
		t.Fatalf("transfer event must update ownership to node-2, got %s", records[0].OwnerNodeID)
	}
}

func TestReplayOwnership_EmptyEvents(t *testing.T) {
	records := ReplayOwnership(nil)
	if len(records) != 0 {
		t.Fatal("empty events must produce empty ownership")
	}
}

func TestNewOwnershipConflictEvent(t *testing.T) {
	l1 := mustLease(t, "l1", "exec-1", "n1", "t1", 1)
	l2 := mustLease(t, "l2", "exec-1", "n2", "t1", 1)
	conflict := OwnershipConflict{
		ExecutionID:       "exec-1",
		TenantID:          "t1",
		ConflictingLeases: []ExecutionLease{l1, l2},
	}
	evt := NewOwnershipConflictEvent(conflict, "evt-c", 99)
	if evt.EventType != EventTypeOwnershipConflict {
		t.Fatalf("event type must be ownership.conflict, got %s", evt.EventType)
	}
	if evt.TenantID != "t1" {
		t.Fatal("event must carry correct tenantID")
	}
}

// ─── RuntimeHeartbeat ─────────────────────────────────────────────────────

func TestNewHeartbeat_HashSet(t *testing.T) {
	hb := NewHeartbeat("n1", "t1", 5, 0.3, 2, 1, 0, 4)
	if hb.HeartbeatHash == "" {
		t.Fatal("HeartbeatHash must be set")
	}
}

func TestVerifyHeartbeatHash_Fresh(t *testing.T) {
	hb := NewHeartbeat("n1", "t1", 5, 0.3, 2, 1, 0, 4)
	ok, reason := VerifyHeartbeatHash(hb)
	if !ok {
		t.Fatalf("fresh heartbeat hash must verify: %s", reason)
	}
}

func TestVerifyHeartbeatHash_Tampered(t *testing.T) {
	hb := NewHeartbeat("n1", "t1", 5, 0.3, 2, 1, 0, 4)
	hb.ReplayBacklog = 999
	ok, _ := VerifyHeartbeatHash(hb)
	if ok {
		t.Fatal("tampered heartbeat must fail verification")
	}
}

func TestCompareHeartbeats_LogicalClock(t *testing.T) {
	a := NewHeartbeat("n1", "t1", 1, 0, 0, 0, 0, 0)
	b := NewHeartbeat("n2", "t1", 2, 0, 0, 0, 0, 0)
	if CompareHeartbeats(a, b) >= 0 {
		t.Fatal("lower clock must sort before higher clock")
	}
}

func TestLatestHeartbeat(t *testing.T) {
	h1 := NewHeartbeat("n1", "t1", 1, 0, 0, 0, 0, 0)
	h2 := NewHeartbeat("n2", "t1", 5, 0, 0, 0, 0, 0)
	h3 := NewHeartbeat("n3", "t1", 3, 0, 0, 0, 0, 0)
	latest := LatestHeartbeat([]RuntimeHeartbeat{h1, h2, h3})
	if latest.NodeID != "n2" {
		t.Fatalf("latest must be clock=5 (n2), got %s", latest.NodeID)
	}
}

func TestLatestHeartbeat_Empty(t *testing.T) {
	hb := LatestHeartbeat(nil)
	if hb.NodeID != "" {
		t.Fatal("empty slice must return zero value")
	}
}

func TestIsHeartbeatStale(t *testing.T) {
	hb := NewHeartbeat("n1", "t1", 10, 0, 0, 0, 0, 0)
	if IsHeartbeatStale(hb, 15, 10) {
		t.Fatal("15-10=5 ≤ 10: must not be stale")
	}
	if !IsHeartbeatStale(hb, 25, 10) {
		t.Fatal("25-10=15 > 10: must be stale")
	}
}

func TestSortHeartbeats_AscendingClock(t *testing.T) {
	h1 := NewHeartbeat("n1", "t1", 3, 0, 0, 0, 0, 0)
	h2 := NewHeartbeat("n2", "t1", 1, 0, 0, 0, 0, 0)
	h3 := NewHeartbeat("n3", "t1", 2, 0, 0, 0, 0, 0)
	sorted := SortHeartbeats([]RuntimeHeartbeat{h1, h2, h3})
	if sorted[0].LogicalClock != 1 || sorted[1].LogicalClock != 2 || sorted[2].LogicalClock != 3 {
		t.Fatal("heartbeats must be sorted by ascending LogicalClock")
	}
}

func TestSortHeartbeats_OriginalUnchanged(t *testing.T) {
	h1 := NewHeartbeat("n1", "t1", 3, 0, 0, 0, 0, 0)
	h2 := NewHeartbeat("n2", "t1", 1, 0, 0, 0, 0, 0)
	orig := []RuntimeHeartbeat{h1, h2}
	SortHeartbeats(orig)
	if orig[0].LogicalClock != 3 {
		t.Fatal("original slice must not be mutated by SortHeartbeats")
	}
}

func TestHeartbeatsForNode(t *testing.T) {
	h1 := NewHeartbeat("n1", "t1", 1, 0, 0, 0, 0, 0)
	h2 := NewHeartbeat("n2", "t1", 2, 0, 0, 0, 0, 0)
	h3 := NewHeartbeat("n1", "t1", 3, 0, 0, 0, 0, 0)
	result := HeartbeatsForNode([]RuntimeHeartbeat{h1, h2, h3}, "n1")
	if len(result) != 2 {
		t.Fatalf("must return 2 heartbeats for n1, got %d", len(result))
	}
	for _, hb := range result {
		if hb.NodeID != "n1" {
			t.Fatal("all returned heartbeats must belong to n1")
		}
	}
}

// ─── ExecutionPartitionMap ─────────────────────────────────────────────────

func TestBuildPartitionMap_Deterministic(t *testing.T) {
	execs := []string{"exec-c", "exec-a", "exec-b"}
	nodes := []string{"node-2", "node-1"}
	pm1, err := BuildPartitionMap("pm-1", "tenant-1", execs, nodes)
	if err != nil {
		t.Fatalf("BuildPartitionMap must not error: %v", err)
	}
	pm2, _ := BuildPartitionMap("pm-1", "tenant-1", execs, nodes)
	if pm1.MapHash != pm2.MapHash {
		t.Fatal("BuildPartitionMap must be deterministic for same inputs")
	}
}

func TestBuildPartitionMap_InputOrderIndependent(t *testing.T) {
	pm1, _ := BuildPartitionMap("pm-1", "t1", []string{"e2", "e1"}, []string{"n2", "n1"})
	pm2, _ := BuildPartitionMap("pm-1", "t1", []string{"e1", "e2"}, []string{"n1", "n2"})
	if pm1.MapHash != pm2.MapHash {
		t.Fatal("partition map must be identical regardless of input order")
	}
}

func TestBuildPartitionMap_AllExecutionsAssigned(t *testing.T) {
	execs := []string{"e1", "e2", "e3", "e4", "e5"}
	nodes := []string{"n1", "n2", "n3"}
	pm, _ := BuildPartitionMap("pm-1", "t1", execs, nodes)
	if len(pm.Entries) != len(execs) {
		t.Fatalf("all executions must be assigned, got %d entries", len(pm.Entries))
	}
	for _, e := range pm.Entries {
		if e.AssignedNodeID == "" {
			t.Fatal("every entry must have an assigned node")
		}
	}
}

func TestBuildPartitionMap_EmptyNodes(t *testing.T) {
	if _, err := BuildPartitionMap("pm-1", "t1", []string{"e1"}, nil); err == nil {
		t.Fatal("empty nodes must error")
	}
}

func TestLookupPartition_Found(t *testing.T) {
	pm, _ := BuildPartitionMap("pm-1", "t1", []string{"exec-1", "exec-2"}, []string{"n1", "n2"})
	node, ok := LookupPartition(pm, "exec-1")
	if !ok || node == "" {
		t.Fatal("exec-1 must have a partition assignment")
	}
}

func TestLookupPartition_NotFound(t *testing.T) {
	pm, _ := BuildPartitionMap("pm-1", "t1", []string{"exec-1"}, []string{"n1"})
	_, ok := LookupPartition(pm, "exec-999")
	if ok {
		t.Fatal("missing execution must return not-found")
	}
}

func TestIsStalePartitionMap_NodeGone(t *testing.T) {
	pm, _ := BuildPartitionMap("pm-1", "t1", []string{"e1", "e2"}, []string{"n1", "n2"})
	// Replace with entirely different nodes — guaranteed stale regardless of assignment.
	if !IsStalePartitionMap(pm, []string{"n3", "n4"}) {
		t.Fatal("partition map must be stale when none of the assigned nodes are active")
	}
}

func TestIsStalePartitionMap_AllNodesPresent(t *testing.T) {
	pm, _ := BuildPartitionMap("pm-1", "t1", []string{"e1"}, []string{"n1", "n2"})
	if IsStalePartitionMap(pm, []string{"n1", "n2"}) {
		t.Fatal("partition map must not be stale when all nodes are active")
	}
}

func TestVerifyPartitionMapHash_Fresh(t *testing.T) {
	pm, _ := BuildPartitionMap("pm-1", "t1", []string{"e1", "e2"}, []string{"n1", "n2"})
	ok, reason := VerifyPartitionMapHash(pm)
	if !ok {
		t.Fatalf("fresh partition map hash must verify: %s", reason)
	}
}

func TestVerifyPartitionMapHash_Tampered(t *testing.T) {
	pm, _ := BuildPartitionMap("pm-1", "t1", []string{"e1"}, []string{"n1"})
	pm.Entries[0].AssignedNodeID = "n-evil"
	ok, _ := VerifyPartitionMapHash(pm)
	if ok {
		t.Fatal("tampered partition map must fail verification")
	}
}

// ─── RuntimeFederationSnapshot ─────────────────────────────────────────────

func TestBuildFederationSnapshot_HashNonEmpty(t *testing.T) {
	snap := BuildFederationSnapshot("fs-1", "t1", nil, nil, nil, nil, nil, nil)
	if snap.FederationHash == "" {
		t.Fatal("FederationHash must be set")
	}
}

func TestVerifyFederationSnapshot_Fresh(t *testing.T) {
	n := makeNode(t, "n1", NodeStateActive)
	l := mustLease(t, "l1", "exec-1", "n1", "t1", 1)
	snap := BuildFederationSnapshot("fs-1", "t1", []RuntimeNode{n}, []ExecutionLease{l}, nil, nil, nil, nil)
	ok, reason := VerifyFederationSnapshot(snap)
	if !ok {
		t.Fatalf("fresh federation snapshot must verify: %s", reason)
	}
}

func TestVerifyFederationSnapshot_Tampered(t *testing.T) {
	snap := BuildFederationSnapshot("fs-1", "t1", nil, nil, nil, nil, nil, nil)
	snap.TenantID = "t-evil"
	ok, _ := VerifyFederationSnapshot(snap)
	if ok {
		t.Fatal("tampered federation snapshot must fail verification")
	}
}

func TestVerifyFederationSnapshot_ContentIdentity(t *testing.T) {
	n := makeNode(t, "n1", NodeStateActive)
	snap1 := BuildFederationSnapshot("fs-1", "t1", []RuntimeNode{n}, nil, nil, nil, nil, nil)
	snap2 := BuildFederationSnapshot("fs-1", "t1", []RuntimeNode{n}, nil, nil, nil, nil, nil)
	if snap1.FederationHash != snap2.FederationHash {
		t.Fatal("federation snapshot hash must be content-identity (GeneratedAt excluded from hash input)")
	}
}

func TestQuarantineNode_UpdatesSnapshot(t *testing.T) {
	n1 := makeNode(t, "n1", NodeStateActive)
	n2 := makeNode(t, "n2", NodeStateActive)
	snap := BuildFederationSnapshot("fs-1", "t1", []RuntimeNode{n1, n2}, nil, nil, nil, nil, nil)

	newSnap, evt, err := QuarantineNode(snap, "n1", "misbehaving", "evt-q", 10)
	if err != nil {
		t.Fatalf("QuarantineNode must not error: %v", err)
	}
	// Original must not be mutated.
	for _, n := range snap.Nodes {
		if n.NodeID == "n1" && n.State == NodeStateQuarantined {
			t.Fatal("original snapshot must not be mutated")
		}
	}
	// New snapshot must have n1 quarantined.
	found := false
	for _, n := range newSnap.Nodes {
		if n.NodeID == "n1" {
			if n.State != NodeStateQuarantined {
				t.Fatalf("n1 must be quarantined in new snapshot, got %s", n.State)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("n1 must still appear in new snapshot")
	}
	// n1 must appear in quarantined list.
	inList := false
	for _, qn := range newSnap.QuarantinedNodes {
		if qn == "n1" {
			inList = true
			break
		}
	}
	if !inList {
		t.Fatal("quarantined list must include n1")
	}
	if evt.EventType != EventTypeNodeQuarantined {
		t.Fatalf("event type must be node.quarantined, got %s", evt.EventType)
	}
	ok, reason := VerifyFederationSnapshot(newSnap)
	if !ok {
		t.Fatalf("new snapshot hash must verify: %s", reason)
	}
}

func TestQuarantineNode_AlreadyQuarantined(t *testing.T) {
	n := makeNode(t, "n1", NodeStateQuarantined)
	snap := BuildFederationSnapshot("fs-1", "t1", []RuntimeNode{n}, nil, nil, nil, nil, nil)
	if _, _, err := QuarantineNode(snap, "n1", "double", "evt-q2", 20); err == nil {
		t.Fatal("quarantining already-quarantined node must error")
	}
}

func TestQuarantineNode_NotFound(t *testing.T) {
	snap := BuildFederationSnapshot("fs-1", "t1", nil, nil, nil, nil, nil, nil)
	if _, _, err := QuarantineNode(snap, "n-missing", "reason", "e", 1); err == nil {
		t.Fatal("quarantining non-existent node must error")
	}
}

// ─── RuntimeSnapshot ───────────────────────────────────────────────────────

func TestExportRuntimeSnapshot_HashNonEmpty(t *testing.T) {
	snap := ExportRuntimeSnapshot("rs-1", "t1", nil, nil, nil, nil, nil)
	if snap.SnapshotHash == "" {
		t.Fatal("SnapshotHash must be set")
	}
}

func TestVerifyRuntimeSnapshot_Fresh(t *testing.T) {
	n := makeNode(t, "n1", NodeStateActive)
	l := mustLease(t, "l1", "exec-1", "n1", "t1", 1)
	snap := ExportRuntimeSnapshot("rs-1", "t1", []RuntimeNode{n}, []ExecutionLease{l}, nil, nil, nil)
	ok, reason := VerifyRuntimeSnapshot(snap)
	if !ok {
		t.Fatalf("fresh runtime snapshot must verify: %s", reason)
	}
}

func TestVerifyRuntimeSnapshot_Tampered(t *testing.T) {
	snap := ExportRuntimeSnapshot("rs-1", "t1", nil, nil, nil, nil, nil)
	snap.TenantID = "evil"
	ok, _ := VerifyRuntimeSnapshot(snap)
	if ok {
		t.Fatal("tampered runtime snapshot must fail verification")
	}
}

func TestExportRuntimeSnapshot_ContentIdentity(t *testing.T) {
	l := mustLease(t, "l1", "exec-1", "n1", "t1", 1)
	snap1 := ExportRuntimeSnapshot("rs-1", "t1", nil, []ExecutionLease{l}, nil, nil, nil)
	snap2 := ExportRuntimeSnapshot("rs-1", "t1", nil, []ExecutionLease{l}, nil, nil, nil)
	if snap1.SnapshotHash != snap2.SnapshotHash {
		t.Fatal("runtime snapshot hash must be content-identity (GeneratedAt excluded)")
	}
}

func TestExportRuntimeSnapshot_OwnershipMapDerived(t *testing.T) {
	l1 := mustLease(t, "l1", "exec-1", "n1", "t1", 1)
	l2 := mustLease(t, "l2", "exec-2", "n2", "t1", 1)
	snap := ExportRuntimeSnapshot("rs-1", "t1", nil, []ExecutionLease{l1, l2}, nil, nil, nil)
	if len(snap.OwnershipMap) != 2 {
		t.Fatalf("ownership map must have 2 entries, got %d", len(snap.OwnershipMap))
	}
}

func TestExportRuntimeSnapshot_ConflictsDetected(t *testing.T) {
	l1 := mustLease(t, "l1", "exec-1", "n1", "t1", 1)
	l2 := mustLease(t, "l2", "exec-1", "n2", "t1", 1)
	snap := ExportRuntimeSnapshot("rs-1", "t1", nil, []ExecutionLease{l1, l2}, nil, nil, nil)
	if len(snap.Conflicts) != 1 {
		t.Fatalf("must detect 1 conflict, got %d", len(snap.Conflicts))
	}
}

func TestExportRuntimeSnapshot_WithPartitionMap(t *testing.T) {
	pm, _ := BuildPartitionMap("pm-1", "t1", []string{"exec-1"}, []string{"n1"})
	snap := ExportRuntimeSnapshot("rs-1", "t1", nil, nil, nil, &pm, nil)
	if snap.PartitionMap == nil {
		t.Fatal("snapshot must include partition map when provided")
	}
	if snap.PartitionMap.MapID != "pm-1" {
		t.Fatal("partition map must be deep-copied correctly")
	}
	ok, reason := VerifyRuntimeSnapshot(snap)
	if !ok {
		t.Fatalf("snapshot with partition map must verify: %s", reason)
	}
}

func TestExportRuntimeSnapshot_LeaseInputNotMutated(t *testing.T) {
	l := mustLease(t, "l1", "exec-1", "n1", "t1", 1)
	origOwner := l.OwnerNodeID
	ExportRuntimeSnapshot("rs-1", "t1", nil, []ExecutionLease{l}, nil, nil, nil)
	if l.OwnerNodeID != origOwner {
		t.Fatal("ExportRuntimeSnapshot must not mutate input leases")
	}
}
