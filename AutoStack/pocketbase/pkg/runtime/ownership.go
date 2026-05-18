package runtime

import (
	"encoding/json"
	"fmt"

	"github.com/janlauber/one-click/pkg/events"
)

// OwnershipRecord captures the authoritative owner of an execution at a point in time.
// Derived from leases — never mutates existing records.
type OwnershipRecord struct {
	ExecutionID string `json:"execution_id"`
	OwnerNodeID string `json:"owner_node_id"`
	LeaseID     string `json:"lease_id"`
	LeaseEpoch  int64  `json:"lease_epoch"`
	TenantID    string `json:"tenant_id"`
}

// OwnershipConflict captures duplicate ownership — two or more leases for the same
// execution at the same epoch (split-brain). Conflicts are NEVER auto-resolved;
// the operator must resolve them explicitly.
type OwnershipConflict struct {
	ExecutionID       string          `json:"execution_id"`
	TenantID          string          `json:"tenant_id"`
	ConflictingLeases []ExecutionLease `json:"conflicting_leases"`
}

// BuildOwnershipMap returns a map of executionID → the highest-epoch active lease.
// Ties (same epoch, different owners) are conflicts; those executions are omitted
// from the result and must be retrieved via DetectDuplicateOwnership.
func BuildOwnershipMap(leases []ExecutionLease) map[string]OwnershipRecord {
	best := make(map[string]ExecutionLease)
	for _, l := range leases {
		existing, found := best[l.ExecutionID]
		if !found || l.LeaseEpoch > existing.LeaseEpoch {
			best[l.ExecutionID] = l
		}
	}
	// Remove executions that have a same-epoch conflict.
	conflicts := DetectDuplicateOwnership(leases)
	conflicted := make(map[string]struct{}, len(conflicts))
	for _, c := range conflicts {
		conflicted[c.ExecutionID] = struct{}{}
	}
	result := make(map[string]OwnershipRecord, len(best))
	for execID, l := range best {
		if _, ok := conflicted[execID]; ok {
			continue
		}
		result[execID] = OwnershipRecord{
			ExecutionID: execID,
			OwnerNodeID: l.OwnerNodeID,
			LeaseID:     l.LeaseID,
			LeaseEpoch:  l.LeaseEpoch,
			TenantID:    l.TenantID,
		}
	}
	return result
}

// DetectDuplicateOwnership finds all executions that have two or more leases
// sharing the same LeaseEpoch (split-brain condition).
// The result is sorted deterministically by ExecutionID.
func DetectDuplicateOwnership(leases []ExecutionLease) []OwnershipConflict {
	byExec := make(map[string][]ExecutionLease)
	for _, l := range leases {
		byExec[l.ExecutionID] = append(byExec[l.ExecutionID], l)
	}
	var conflicts []OwnershipConflict
	for execID, ls := range byExec {
		if hasDuplicateEpoch(ls) {
			dup := make([]ExecutionLease, len(ls))
			copy(dup, ls)
			tenantID := ""
			if len(ls) > 0 {
				tenantID = ls[0].TenantID
			}
			conflicts = append(conflicts, OwnershipConflict{
				ExecutionID:       execID,
				TenantID:          tenantID,
				ConflictingLeases: dup,
			})
		}
	}
	sortConflicts(conflicts)
	return conflicts
}

// AssertSingleOwner returns an error when the execution has zero or more than one owner.
func AssertSingleOwner(executionID string, leases []ExecutionLease) error {
	var active []ExecutionLease
	for _, l := range leases {
		if l.ExecutionID == executionID {
			active = append(active, l)
		}
	}
	if len(active) == 0 {
		return fmt.Errorf("execution %q has no active owner", executionID)
	}
	if len(active) > 1 {
		return fmt.Errorf("execution %q has %d conflicting owners (split-brain)", executionID, len(active))
	}
	return nil
}

// ReplayOwnership reconstructs the ownership sequence from a sorted PlatformEvent slice.
// Only EventTypeLeaseAcquired and EventTypeLeaseTransferred events are considered.
// The result reflects the highest-epoch ownership per execution seen in the event stream.
func ReplayOwnership(evts []events.PlatformEvent) []OwnershipRecord {
	current := make(map[string]OwnershipRecord)
	for _, e := range evts {
		if e.Payload == nil {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal(e.Payload, &data); err != nil {
			continue
		}
		var execID, ownerNode, leaseID string
		var epoch int64

		switch e.EventType {
		case EventTypeLeaseAcquired:
			execID, _ = data["execution_id"].(string)
			ownerNode, _ = data["owner_node"].(string)
			leaseID, _ = data["lease_id"].(string)
		case EventTypeLeaseTransferred:
			execID, _ = data["execution_id"].(string)
			ownerNode, _ = data["to_node"].(string)
			leaseID, _ = data["lease_id"].(string)
		default:
			continue
		}
		switch v := data["epoch"].(type) {
		case float64:
			epoch = int64(v)
		case int64:
			epoch = v
		case int:
			epoch = int64(v)
		}
		if execID == "" || ownerNode == "" {
			continue
		}
		prev, found := current[execID]
		if !found || epoch > prev.LeaseEpoch {
			current[execID] = OwnershipRecord{
				ExecutionID: execID,
				OwnerNodeID: ownerNode,
				LeaseID:     leaseID,
				LeaseEpoch:  epoch,
				TenantID:    e.TenantID,
			}
		}
	}
	records := make([]OwnershipRecord, 0, len(current))
	for _, r := range current {
		records = append(records, r)
	}
	sortOwnershipRecords(records)
	return records
}

// NewOwnershipConflictEvent emits a PlatformEvent recording a detected split-brain.
// The event is observation-only — it does NOT resolve the conflict.
func NewOwnershipConflictEvent(
	conflict OwnershipConflict,
	eventID string,
	clock int64,
) events.PlatformEvent {
	ids := make([]string, len(conflict.ConflictingLeases))
	for i, l := range conflict.ConflictingLeases {
		ids[i] = l.LeaseID
	}
	payload, _ := json.Marshal(map[string]any{
		"execution_id":       conflict.ExecutionID,
		"conflicting_leases": ids,
	})
	return events.NewPlatformEvent(
		eventID, EventTypeOwnershipConflict, AggregateTypeRuntime,
		conflict.ExecutionID, conflict.ExecutionID, conflict.TenantID, clock,
		"", "", payload, nil,
	)
}

func hasDuplicateEpoch(leases []ExecutionLease) bool {
	seen := make(map[int64]struct{})
	for _, l := range leases {
		if _, ok := seen[l.LeaseEpoch]; ok {
			return true
		}
		seen[l.LeaseEpoch] = struct{}{}
	}
	return false
}

func sortConflicts(cs []OwnershipConflict) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && cs[j].ExecutionID < cs[j-1].ExecutionID; j-- {
			cs[j], cs[j-1] = cs[j-1], cs[j]
		}
	}
}

func sortOwnershipRecords(rs []OwnershipRecord) {
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0 && rs[j].ExecutionID < rs[j-1].ExecutionID; j-- {
			rs[j], rs[j-1] = rs[j-1], rs[j]
		}
	}
}
