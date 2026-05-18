// Package events implements the Phase 5.0 event fabric:
// append-only, monotonically ordered, tamper-evident PlatformEvents.
//
// Core invariants:
//   - Events are never mutated after creation.
//   - Every event carries a SHA-256 hash over all stable fields.
//   - Logical clock + timestamp + event ID define a total, deterministic order.
//   - Replay over the same event sequence always produces the same result.
package events

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"time"
)

const EventSchemaVersion = "5.0.0"

// EventType classifies what happened.
type EventType string

// AggregateType identifies the domain object the event belongs to.
type AggregateType string

const (
	EventTypeExecutionStarted   EventType = "execution.started"
	EventTypeExecutionAdvanced  EventType = "execution.advanced"
	EventTypeExecutionPaused    EventType = "execution.paused"
	EventTypeExecutionResumed   EventType = "execution.resumed"
	EventTypeExecutionCompleted EventType = "execution.completed"
	EventTypeExecutionFailed    EventType = "execution.failed"
	EventTypeExecutionCancelled EventType = "execution.cancelled"

	EventTypeStageEnqueued  EventType = "stage.enqueued"
	EventTypeStageDequeued  EventType = "stage.dequeued"
	EventTypeStageBlocked   EventType = "stage.blocked"
	EventTypeStageCompleted EventType = "stage.completed"
	EventTypeStageFailed    EventType = "stage.failed"

	EventTypeVerificationRecorded  EventType = "verification.recorded"
	EventTypeContradictionDetected EventType = "contradiction.detected"
	EventTypeDriftDetected         EventType = "drift.detected"

	EventTypeGovernanceDecision EventType = "governance.decision"
	EventTypeGovernanceApproval EventType = "governance.approval"
	EventTypeGovernanceOverride EventType = "governance.override"

	EventTypeProviderObservation EventType = "provider.observation"

	AggregateTypeExecution    AggregateType = "execution"
	AggregateTypeStage        AggregateType = "stage"
	AggregateTypeVerification AggregateType = "verification"
	AggregateTypeGovernance   AggregateType = "governance"
	AggregateTypeProvider     AggregateType = "provider"
)

// PlatformEvent is the canonical, immutable event record.
// Once written to an EventStore, no field may be modified.
type PlatformEvent struct {
	EventID       string            `json:"event_id"`
	EventType     EventType         `json:"event_type"`
	AggregateType AggregateType     `json:"aggregate_type"`
	AggregateID   string            `json:"aggregate_id"`
	ExecutionID   string            `json:"execution_id"`
	TenantID      string            `json:"tenant_id"`
	Timestamp     string            `json:"timestamp"`
	LogicalClock  int64             `json:"logical_clock"`
	CausationID   string            `json:"causation_id,omitempty"`
	CorrelationID string            `json:"correlation_id,omitempty"`
	Payload       []byte            `json:"payload,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	SchemaVersion string            `json:"schema_version"`
	// Hash is SHA-256 over all fields except Hash itself.
	Hash string `json:"hash"`
}

// NewPlatformEvent creates a PlatformEvent with its hash pre-computed.
// LogicalClock must be provided by the caller (monotonic counter from EventStore).
// Payload and Metadata are deep-copied so the caller cannot mutate the event.
func NewPlatformEvent(
	eventID string,
	eventType EventType,
	aggregateType AggregateType,
	aggregateID string,
	executionID string,
	tenantID string,
	logicalClock int64,
	causationID string,
	correlationID string,
	payload []byte,
	metadata map[string]string,
) PlatformEvent {
	e := PlatformEvent{
		EventID:       eventID,
		EventType:     eventType,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		ExecutionID:   executionID,
		TenantID:      tenantID,
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		LogicalClock:  logicalClock,
		CausationID:   causationID,
		CorrelationID: correlationID,
		Payload:       append([]byte(nil), payload...),
		Metadata:      copyMetadata(metadata),
		SchemaVersion: EventSchemaVersion,
	}
	e.Hash = ComputeEventHash(e)
	return e
}

// ComputeEventHash derives SHA-256 over all stable fields.
// Hash itself is excluded (self-referential). Metadata keys are sorted for determinism.
func ComputeEventHash(e PlatformEvent) string {
	h := sha256.New()
	fmt.Fprintf(h, "event_id:%s|event_type:%s|aggregate_type:%s|aggregate_id:%s|",
		e.EventID, string(e.EventType), string(e.AggregateType), e.AggregateID)
	fmt.Fprintf(h, "execution_id:%s|tenant_id:%s|timestamp:%s|logical_clock:%d|",
		e.ExecutionID, e.TenantID, e.Timestamp, e.LogicalClock)
	fmt.Fprintf(h, "causation_id:%s|correlation_id:%s|schema_version:%s|",
		e.CausationID, e.CorrelationID, e.SchemaVersion)
	fmt.Fprintf(h, "payload:%x|", e.Payload)
	// Sort metadata keys for determinism.
	keys := make([]string, 0, len(e.Metadata))
	for k := range e.Metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(h, "meta:%s=%s|", k, e.Metadata[k])
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func copyMetadata(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
