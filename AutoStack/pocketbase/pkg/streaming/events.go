package streaming

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// StreamType categorizes what a stream carries.
type StreamType string

const (
	StreamTypeExecution    StreamType = "execution"
	StreamTypeWorkflow     StreamType = "workflow"
	StreamTypeGovernance   StreamType = "governance"
	StreamTypeFederation   StreamType = "federation"
	StreamTypeContradiction StreamType = "contradiction"
	StreamTypeApproval     StreamType = "approval"
)

// StreamEvent is an immutable, hashed event emitted on a stream.
// EmittedAt is excluded from the hash (content-identity).
type StreamEvent struct {
	EventID      string     `json:"event_id"`
	StreamID     string     `json:"stream_id"`
	StreamType   StreamType `json:"stream_type"`
	ExecutionID  string     `json:"execution_id"`
	TenantID     string     `json:"tenant_id"`
	Sequence     int64      `json:"sequence"`
	LogicalClock int64      `json:"logical_clock"`
	Payload      string     `json:"payload"` // JSON-encoded, caller-supplied
	EmittedAt    string     `json:"emitted_at"` // excluded from hash
	EventHash    string     `json:"event_hash"`
}

// NewStreamEvent creates a hashed stream event.
func NewStreamEvent(eventID, streamID string, streamType StreamType, executionID, tenantID string, sequence, logicalClock int64, payload string) StreamEvent {
	e := StreamEvent{
		EventID:      eventID,
		StreamID:     streamID,
		StreamType:   streamType,
		ExecutionID:  executionID,
		TenantID:     tenantID,
		Sequence:     sequence,
		LogicalClock: logicalClock,
		Payload:      payload,
		EmittedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	e.EventHash = computeStreamEventHash(e)
	return e
}

// VerifyStreamEventHash recomputes the hash and returns (true, "") when it matches.
func VerifyStreamEventHash(e StreamEvent) (bool, string) {
	stored := e.EventHash
	e.EventHash = ""
	expected := computeStreamEventHash(e)
	if stored != expected {
		return false, fmt.Sprintf("stream event %q hash mismatch: stored=%s computed=%s",
			e.EventID, stored, expected)
	}
	return true, ""
}

func computeStreamEventHash(e StreamEvent) string {
	h := sha256.New()
	fmt.Fprintf(h, "streamevent:%s|stream:%s|type:%s|exec:%s|tenant:%s|seq:%d|clock:%d|payload:%s",
		e.EventID, e.StreamID, e.StreamType, e.ExecutionID, e.TenantID,
		e.Sequence, e.LogicalClock, e.Payload)
	return fmt.Sprintf("%x", h.Sum(nil))
}
