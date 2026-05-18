// Package runtime implements Phase 5.1 Distributed Runtime Coordination.
//
// Scope:
//   - RuntimeNode identity, state transitions, capability declaration.
//   - ExecutionLease acquisition, transfer, expiry tracking.
//   - Heartbeat ordering and staleness detection.
//   - Deterministic execution partitioning.
//   - Federation snapshot integrity.
//   - Ownership conflict detection (explicit ambiguity — never auto-resolved).
//
// Invariants:
//   - Single active owner per execution at any logical clock position.
//   - All coordination state changes emit PlatformEvents.
//   - Ownership history is append-only; conflicts are recorded, not resolved.
//   - Partition maps are deterministic for the same inputs.
//   - Quarantined nodes cannot acquire new leases.
//   - Partitioned nodes cannot acquire new leases.
package runtime

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/janlauber/one-click/pkg/events"
)

// NodeState enumerates the allowed states of a RuntimeNode.
type NodeState string

const (
	NodeStateStarting    NodeState = "starting"
	NodeStateActive      NodeState = "active"
	NodeStateDraining    NodeState = "draining"
	NodeStateStopped     NodeState = "stopped"
	NodeStatePartitioned NodeState = "partitioned"
	NodeStateQuarantined NodeState = "quarantined"
)

// NodeCapability declares what workloads a node can handle.
type NodeCapability string

const (
	CapabilityReplay       NodeCapability = "replay"
	CapabilityScheduling   NodeCapability = "scheduling"
	CapabilityVerification NodeCapability = "verification"
	CapabilityGovernance   NodeCapability = "governance"
	CapabilityExport       NodeCapability = "export"
	CapabilityTopology     NodeCapability = "topology"
	CapabilityArchive      NodeCapability = "archive"
)

// Runtime-domain event type constants.
const (
	EventTypeNodeJoined        events.EventType = "runtime.node.joined"
	EventTypeNodeStateChanged  events.EventType = "runtime.node.state_changed"
	EventTypeLeaseAcquired     events.EventType = "runtime.lease.acquired"
	EventTypeLeaseTransferred  events.EventType = "runtime.lease.transferred"
	EventTypeLeaseExpired      events.EventType = "runtime.lease.expired"
	EventTypePartitionDetected events.EventType = "runtime.partition.detected"
	EventTypeNodeQuarantined   events.EventType = "runtime.node.quarantined"
	EventTypeOwnershipConflict events.EventType = "runtime.ownership.conflict"
)

// AggregateTypeRuntime is the aggregate type for all runtime-domain events.
const AggregateTypeRuntime events.AggregateType = "runtime"

// RuntimeNode represents one process-level participant in the coordination fabric.
// State transitions are explicit and always produce a PlatformEvent.
type RuntimeNode struct {
	NodeID         string            `json:"node_id"`
	State          NodeState         `json:"state"`
	Capabilities   []NodeCapability  `json:"capabilities"`
	JoinedAt       string            `json:"joined_at"`
	StateChangedAt string            `json:"state_changed_at"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// NewRuntimeNode creates a node in NodeStateStarting with the given capabilities.
// Capabilities are deep-copied.
func NewRuntimeNode(nodeID string, capabilities []NodeCapability) RuntimeNode {
	caps := make([]NodeCapability, len(capabilities))
	copy(caps, capabilities)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return RuntimeNode{
		NodeID:         nodeID,
		State:          NodeStateStarting,
		Capabilities:   caps,
		JoinedAt:       now,
		StateChangedAt: now,
	}
}

// TransitionNodeState returns a new node with the updated state plus the platform event
// that records the transition. Does NOT modify the original node.
// Invalid transitions return an error without emitting an event.
func TransitionNodeState(
	node RuntimeNode,
	newState NodeState,
	reason string,
	eventID string,
	tenantID string,
	clock int64,
) (RuntimeNode, events.PlatformEvent, error) {
	if err := validateStateTransition(node.State, newState); err != nil {
		return node, events.PlatformEvent{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	updated := node
	updated.State = newState
	updated.StateChangedAt = now

	evtType := EventTypeNodeStateChanged
	if newState == NodeStateQuarantined {
		evtType = EventTypeNodeQuarantined
	}
	payload, _ := json.Marshal(map[string]any{
		"node_id":    node.NodeID,
		"from_state": string(node.State),
		"to_state":   string(newState),
		"reason":     reason,
	})
	evt := events.NewPlatformEvent(
		eventID, evtType, AggregateTypeRuntime,
		node.NodeID, "", tenantID, clock,
		"", "", payload, nil,
	)
	return updated, evt, nil
}

// HasCapability reports whether the node advertises the given capability.
func HasCapability(node RuntimeNode, cap NodeCapability) bool {
	for _, c := range node.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// IsActive returns true only when the node is in NodeStateActive.
func IsActive(node RuntimeNode) bool {
	return node.State == NodeStateActive
}

// CanAcquireLease returns true when the node may take an execution lease.
// Quarantined, partitioned, starting, and stopped nodes cannot acquire leases.
func CanAcquireLease(node RuntimeNode) bool {
	return node.State == NodeStateActive || node.State == NodeStateDraining
}

// NodeHash returns a deterministic SHA-256 hash over stable node fields.
func NodeHash(node RuntimeNode) string {
	h := sha256.New()
	fmt.Fprintf(h, "node:%s|state:%s|joined:%s|caps:", node.NodeID, node.State, node.JoinedAt)
	for _, c := range node.Capabilities {
		fmt.Fprintf(h, "%s,", c)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// validateStateTransition enforces the allowed node state machine.
func validateStateTransition(from, to NodeState) error {
	allowed := map[NodeState][]NodeState{
		NodeStateStarting:    {NodeStateActive, NodeStateStopped, NodeStateQuarantined},
		NodeStateActive:      {NodeStateDraining, NodeStatePartitioned, NodeStateQuarantined, NodeStateStopped},
		NodeStateDraining:    {NodeStateStopped, NodeStateActive, NodeStateQuarantined},
		NodeStatePartitioned: {NodeStateActive, NodeStateStopped, NodeStateQuarantined},
		NodeStateQuarantined: {NodeStateStopped},
		NodeStateStopped:     {},
	}
	for _, t := range allowed[from] {
		if t == to {
			return nil
		}
	}
	return fmt.Errorf("invalid state transition: %s → %s", from, to)
}
