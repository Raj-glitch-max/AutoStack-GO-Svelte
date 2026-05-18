package streaming

import (
	"fmt"
	"sync"
)

// ExecutionStream is an append-only, tenant-scoped sequence of stream events.
// Events are never mutated. The stream never loses events.
type ExecutionStream struct {
	mu          sync.RWMutex
	streamID    string
	tenantID    string
	executionID string
	streamType  StreamType
	events      []StreamEvent
	seqCounter  int64
}

// NewExecutionStream creates an empty stream.
func NewExecutionStream(streamID, tenantID, executionID string, streamType StreamType) *ExecutionStream {
	return &ExecutionStream{
		streamID:    streamID,
		tenantID:    tenantID,
		executionID: executionID,
		streamType:  streamType,
	}
}

// Emit appends a stream event. Validates tenant, execution, stream type, and hash.
// Sequence is automatically assigned; the caller's Sequence field is ignored.
func (s *ExecutionStream) Emit(e StreamEvent) error {
	if e.TenantID != s.tenantID {
		return fmt.Errorf("stream: event tenant %q does not match stream tenant %q", e.TenantID, s.tenantID)
	}
	if e.ExecutionID != s.executionID {
		return fmt.Errorf("stream: event execution %q does not match stream execution %q", e.ExecutionID, s.executionID)
	}
	if e.StreamType != s.streamType {
		return fmt.Errorf("stream: event type %q does not match stream type %q", e.StreamType, s.streamType)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.seqCounter++
	sealed := e
	sealed.StreamID = s.streamID
	sealed.Sequence = s.seqCounter
	sealed.EventHash = ""
	sealed.EventHash = computeStreamEventHash(sealed)

	s.events = append(s.events, sealed)
	return nil
}

// Events returns a copy of all events in emission order.
func (s *ExecutionStream) Events() []StreamEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]StreamEvent, len(s.events))
	copy(out, s.events)
	return out
}

// EventsAfter returns all events with Sequence > afterSeq.
func (s *ExecutionStream) EventsAfter(afterSeq int64) []StreamEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []StreamEvent
	for _, e := range s.events {
		if e.Sequence > afterSeq {
			out = append(out, e)
		}
	}
	return out
}

// Size returns the number of events in the stream.
func (s *ExecutionStream) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.events)
}

// StreamID returns the stream's identifier.
func (s *ExecutionStream) StreamID() string { return s.streamID }

// TenantID returns the stream's tenant scope.
func (s *ExecutionStream) TenantID() string { return s.tenantID }
