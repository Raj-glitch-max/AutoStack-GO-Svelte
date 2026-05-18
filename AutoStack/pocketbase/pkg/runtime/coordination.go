package runtime

import (
	"encoding/json"
	"fmt"

	"github.com/janlauber/one-click/pkg/events"
)

// CoordinationViolation records a rule violation detected during coordination.
// Violations are immutable observation records — never auto-resolved.
type CoordinationViolation struct {
	ViolationType string `json:"violation_type"`
	NodeID        string `json:"node_id,omitempty"`
	ExecutionID   string `json:"execution_id,omitempty"`
	LeaseID       string `json:"lease_id,omitempty"`
	Detail        string `json:"detail"`
}

// ValidateLeaseAcquisition checks all coordination rules before a node may
// acquire a lease for an execution. Returns nil when the acquisition is permitted.
func ValidateLeaseAcquisition(node RuntimeNode, executionID string, existingLeases []ExecutionLease) *CoordinationViolation {
	switch node.State {
	case NodeStateQuarantined:
		return &CoordinationViolation{
			ViolationType: "quarantined_node_cannot_acquire_lease",
			NodeID:        node.NodeID,
			ExecutionID:   executionID,
			Detail:        fmt.Sprintf("node %q is quarantined and cannot acquire leases", node.NodeID),
		}
	case NodeStatePartitioned:
		return &CoordinationViolation{
			ViolationType: "partitioned_node_cannot_acquire_lease",
			NodeID:        node.NodeID,
			ExecutionID:   executionID,
			Detail:        fmt.Sprintf("node %q is partitioned and cannot acquire new leases", node.NodeID),
		}
	case NodeStateStopped:
		return &CoordinationViolation{
			ViolationType: "stopped_node_cannot_acquire_lease",
			NodeID:        node.NodeID,
			ExecutionID:   executionID,
			Detail:        fmt.Sprintf("node %q is stopped and cannot acquire leases", node.NodeID),
		}
	case NodeStateStarting:
		return &CoordinationViolation{
			ViolationType: "starting_node_cannot_acquire_lease",
			NodeID:        node.NodeID,
			ExecutionID:   executionID,
			Detail:        fmt.Sprintf("node %q is still starting and cannot acquire leases", node.NodeID),
		}
	}
	for _, l := range existingLeases {
		if l.ExecutionID == executionID {
			return &CoordinationViolation{
				ViolationType: "execution_already_owned",
				NodeID:        node.NodeID,
				ExecutionID:   executionID,
				LeaseID:       l.LeaseID,
				Detail:        fmt.Sprintf("execution %q already owned by node %q (lease %q)", executionID, l.OwnerNodeID, l.LeaseID),
			}
		}
	}
	return nil
}

// ValidateLeaseTransfer checks coordination rules before transferring a lease.
// Returns nil when the transfer is permitted.
func ValidateLeaseTransfer(lease ExecutionLease, fromNode, toNode RuntimeNode) *CoordinationViolation {
	if fromNode.State == NodeStateQuarantined {
		return &CoordinationViolation{
			ViolationType: "quarantined_node_cannot_transfer_lease",
			NodeID:        fromNode.NodeID,
			LeaseID:       lease.LeaseID,
			Detail:        fmt.Sprintf("node %q is quarantined; transfer requires operator intervention", fromNode.NodeID),
		}
	}
	if lease.OwnerNodeID != fromNode.NodeID {
		return &CoordinationViolation{
			ViolationType: "non_owner_transfer_attempt",
			NodeID:        fromNode.NodeID,
			LeaseID:       lease.LeaseID,
			Detail:        fmt.Sprintf("node %q does not own lease %q (owned by %q)", fromNode.NodeID, lease.LeaseID, lease.OwnerNodeID),
		}
	}
	if !CanAcquireLease(toNode) {
		return &CoordinationViolation{
			ViolationType: "target_node_cannot_accept_lease",
			NodeID:        toNode.NodeID,
			LeaseID:       lease.LeaseID,
			Detail:        fmt.Sprintf("target node %q is in state %q and cannot accept leases", toNode.NodeID, toNode.State),
		}
	}
	return nil
}

// NewPartitionDetectedEvent emits a PlatformEvent when a network partition is detected.
func NewPartitionDetectedEvent(
	nodeID string,
	tenantID string,
	affectedLeaseIDs []string,
	eventID string,
	clock int64,
) events.PlatformEvent {
	ids := make([]string, len(affectedLeaseIDs))
	copy(ids, affectedLeaseIDs)
	payload, _ := json.Marshal(map[string]any{
		"node_id":            nodeID,
		"affected_lease_ids": ids,
	})
	return events.NewPlatformEvent(
		eventID, EventTypePartitionDetected, AggregateTypeRuntime,
		nodeID, "", tenantID, clock,
		"", "", payload, nil,
	)
}
