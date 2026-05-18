package events

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// EventStream is an ordered, append-only sequence of PlatformEvents for one
// execution. The StreamHash is SHA-256 over all event hashes in order —
// appending any event changes the hash.
type EventStream struct {
	StreamID    string          `json:"stream_id"`
	TenantID    string          `json:"tenant_id"`
	ExecutionID string          `json:"execution_id"`
	Events      []PlatformEvent `json:"events"`
	StreamHash  string          `json:"stream_hash"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

// NewEventStream creates an empty stream with its hash pre-computed.
func NewEventStream(streamID, tenantID, executionID string) EventStream {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return EventStream{
		StreamID:    streamID,
		TenantID:    tenantID,
		ExecutionID: executionID,
		Events:      nil,
		StreamHash:  computeStreamHash(nil),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// AppendEvent appends a validated event to the stream.
// Returns (updated stream, error) — the original stream is never mutated.
// Error if: event hash invalid, or logical clock regresses below the last event.
func AppendEvent(stream EventStream, event PlatformEvent) (EventStream, error) {
	expected := ComputeEventHash(event)
	if event.Hash != expected {
		return stream, fmt.Errorf("event %q hash mismatch: stored %q expected %q",
			event.EventID, event.Hash, expected)
	}
	if len(stream.Events) > 0 {
		last := stream.Events[len(stream.Events)-1]
		if event.LogicalClock < last.LogicalClock {
			return stream, fmt.Errorf("event %q logical clock %d regresses below last %d",
				event.EventID, event.LogicalClock, last.LogicalClock)
		}
	}
	newEvents := append([]PlatformEvent(nil), stream.Events...)
	newEvents = append(newEvents, event)
	return EventStream{
		StreamID:    stream.StreamID,
		TenantID:    stream.TenantID,
		ExecutionID: stream.ExecutionID,
		Events:      newEvents,
		StreamHash:  computeStreamHash(newEvents),
		CreatedAt:   stream.CreatedAt,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

// EventsInRange returns events with LogicalClock in [fromClock, toClock] inclusive.
func EventsInRange(stream EventStream, fromClock, toClock int64) []PlatformEvent {
	var result []PlatformEvent
	for _, e := range stream.Events {
		if e.LogicalClock >= fromClock && e.LogicalClock <= toClock {
			result = append(result, e)
		}
	}
	return result
}

// StreamLength returns the number of events in the stream.
func StreamLength(stream EventStream) int {
	return len(stream.Events)
}

// computeStreamHash derives SHA-256 over all event hashes concatenated in order.
func computeStreamHash(evts []PlatformEvent) string {
	if len(evts) == 0 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte("")))
	}
	h := sha256.New()
	for _, e := range evts {
		fmt.Fprintf(h, "%s|", e.Hash)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
