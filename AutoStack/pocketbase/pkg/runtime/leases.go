package runtime

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/janlauber/one-click/pkg/events"
)

// ExecutionLease grants one node exclusive authority over one execution.
// Leases are value types — transfer produces new records, never mutates existing ones.
type ExecutionLease struct {
	LeaseID        string `json:"lease_id"`
	ExecutionID    string `json:"execution_id"`
	OwnerNodeID    string `json:"owner_node_id"`
	TenantID       string `json:"tenant_id"`
	AcquiredAt     string `json:"acquired_at"`
	ExpiresAt      string `json:"expires_at"`
	LeaseEpoch     int64  `json:"lease_epoch"`
	TransferReason string `json:"transfer_reason,omitempty"`
	PreviousOwner  string `json:"previous_owner,omitempty"`
}

// LeaseTransferRecord is an immutable audit entry for one lease handoff.
// Transfer history must be append-only — records are never modified.
type LeaseTransferRecord struct {
	RecordID      string `json:"record_id"`
	LeaseID       string `json:"lease_id"`
	ExecutionID   string `json:"execution_id"`
	FromNodeID    string `json:"from_node_id"`
	ToNodeID      string `json:"to_node_id"`
	TransferEpoch int64  `json:"transfer_epoch"`
	Reason        string `json:"reason"`
	TransferredAt string `json:"transferred_at"`
}

// NewExecutionLease creates a fresh lease. epoch is clamped to ≥ 1.
func NewExecutionLease(leaseID, executionID, ownerNodeID, tenantID string, epoch int64, expiresAt string) (ExecutionLease, error) {
	if leaseID == "" || executionID == "" || ownerNodeID == "" || tenantID == "" {
		return ExecutionLease{}, fmt.Errorf("leaseID, executionID, ownerNodeID, and tenantID are required")
	}
	if expiresAt == "" {
		return ExecutionLease{}, fmt.Errorf("expiresAt is required")
	}
	if epoch < 1 {
		epoch = 1
	}
	return ExecutionLease{
		LeaseID:     leaseID,
		ExecutionID: executionID,
		OwnerNodeID: ownerNodeID,
		TenantID:    tenantID,
		AcquiredAt:  time.Now().UTC().Format(time.RFC3339Nano),
		ExpiresAt:   expiresAt,
		LeaseEpoch:  epoch,
	}, nil
}

// TransferLease hands ownership from the current owner to toNodeID.
// Returns a new ExecutionLease, an immutable LeaseTransferRecord, and a PlatformEvent.
// The original lease is never modified.
func TransferLease(
	lease ExecutionLease,
	toNodeID string,
	reason string,
	recordID string,
	eventID string,
	clock int64,
) (ExecutionLease, LeaseTransferRecord, events.PlatformEvent, error) {
	if toNodeID == "" {
		return lease, LeaseTransferRecord{}, events.PlatformEvent{}, fmt.Errorf("toNodeID is required for transfer")
	}
	if reason == "" {
		return lease, LeaseTransferRecord{}, events.PlatformEvent{}, fmt.Errorf("reason is required for transfer")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	newEpoch := lease.LeaseEpoch + 1
	newLease := ExecutionLease{
		LeaseID:        lease.LeaseID,
		ExecutionID:    lease.ExecutionID,
		OwnerNodeID:    toNodeID,
		TenantID:       lease.TenantID,
		AcquiredAt:     now,
		ExpiresAt:      lease.ExpiresAt,
		LeaseEpoch:     newEpoch,
		TransferReason: reason,
		PreviousOwner:  lease.OwnerNodeID,
	}
	record := LeaseTransferRecord{
		RecordID:      recordID,
		LeaseID:       lease.LeaseID,
		ExecutionID:   lease.ExecutionID,
		FromNodeID:    lease.OwnerNodeID,
		ToNodeID:      toNodeID,
		TransferEpoch: newEpoch,
		Reason:        reason,
		TransferredAt: now,
	}
	payload, _ := json.Marshal(map[string]any{
		"lease_id":     lease.LeaseID,
		"execution_id": lease.ExecutionID,
		"from_node":    lease.OwnerNodeID,
		"to_node":      toNodeID,
		"epoch":        newEpoch,
		"reason":       reason,
	})
	evt := events.NewPlatformEvent(
		eventID, EventTypeLeaseTransferred, AggregateTypeRuntime,
		lease.LeaseID, lease.ExecutionID, lease.TenantID, clock,
		"", "", payload, nil,
	)
	return newLease, record, evt, nil
}

// IsLeaseExpired returns true when the lease ExpiresAt is before or equal to now (RFC3339Nano string).
func IsLeaseExpired(lease ExecutionLease, now string) bool {
	return lease.ExpiresAt <= now
}

// NewLeaseAcquiredEvent emits a PlatformEvent when a lease is first acquired.
func NewLeaseAcquiredEvent(lease ExecutionLease, eventID string, clock int64) events.PlatformEvent {
	payload, _ := json.Marshal(map[string]any{
		"lease_id":     lease.LeaseID,
		"execution_id": lease.ExecutionID,
		"owner_node":   lease.OwnerNodeID,
		"epoch":        lease.LeaseEpoch,
		"expires_at":   lease.ExpiresAt,
	})
	return events.NewPlatformEvent(
		eventID, EventTypeLeaseAcquired, AggregateTypeRuntime,
		lease.LeaseID, lease.ExecutionID, lease.TenantID, clock,
		"", "", payload, nil,
	)
}

// NewLeaseExpiredEvent emits a PlatformEvent when a lease has expired.
func NewLeaseExpiredEvent(lease ExecutionLease, eventID string, clock int64) events.PlatformEvent {
	payload, _ := json.Marshal(map[string]any{
		"lease_id":     lease.LeaseID,
		"execution_id": lease.ExecutionID,
		"owner_node":   lease.OwnerNodeID,
		"epoch":        lease.LeaseEpoch,
		"expired_at":   lease.ExpiresAt,
	})
	return events.NewPlatformEvent(
		eventID, EventTypeLeaseExpired, AggregateTypeRuntime,
		lease.LeaseID, lease.ExecutionID, lease.TenantID, clock,
		"", "", payload, nil,
	)
}
