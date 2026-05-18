package events

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// EventStore is the append-only, immutable interface for PlatformEvent persistence.
//
// Invariants:
//   - Append MUST NOT modify any previously stored event.
//   - Load MUST return events in canonical order (LogicalClock → Timestamp → EventID).
//   - NextLogicalClock MUST be strictly monotonic across concurrent callers.
type EventStore interface {
	Append(ctx context.Context, streamID string, event PlatformEvent) error
	Load(ctx context.Context, streamID string) (EventStream, error)
	LoadRange(ctx context.Context, streamID string, fromClock, toClock int64) ([]PlatformEvent, error)
	LoadByAggregate(ctx context.Context, tenantID, aggregateType, aggregateID string) ([]PlatformEvent, error)
	NextLogicalClock(ctx context.Context) (int64, error)
}

// InMemoryEventStore is a thread-safe, test-friendly EventStore.
// It provides no durability guarantees — use it in tests and as a wiring stub.
type InMemoryEventStore struct {
	mu      sync.RWMutex
	streams map[string]EventStream
	seen    map[string]struct{} // duplicate EventID guard
	clock   int64
}

// NewInMemoryEventStore returns an empty InMemoryEventStore.
func NewInMemoryEventStore() *InMemoryEventStore {
	return &InMemoryEventStore{
		streams: make(map[string]EventStream),
		seen:    make(map[string]struct{}),
	}
}

// NextLogicalClock returns a strictly monotonic clock value (1-based).
func (s *InMemoryEventStore) NextLogicalClock(_ context.Context) (int64, error) {
	return atomic.AddInt64(&s.clock, 1), nil
}

// Append validates and appends an event to the named stream.
func (s *InMemoryEventStore) Append(_ context.Context, streamID string, event PlatformEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.seen[event.EventID]; dup {
		return fmt.Errorf("duplicate event ID %q", event.EventID)
	}
	stream, ok := s.streams[streamID]
	if !ok {
		stream = NewEventStream(streamID, event.TenantID, event.ExecutionID)
	}
	updated, err := AppendEvent(stream, event)
	if err != nil {
		return err
	}
	s.streams[streamID] = updated
	s.seen[event.EventID] = struct{}{}
	return nil
}

// Load returns a copy of the stream for the given streamID.
func (s *InMemoryEventStore) Load(_ context.Context, streamID string) (EventStream, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stream, ok := s.streams[streamID]
	if !ok {
		return EventStream{}, fmt.Errorf("stream %q not found", streamID)
	}
	return EventStream{
		StreamID:    stream.StreamID,
		TenantID:    stream.TenantID,
		ExecutionID: stream.ExecutionID,
		Events:      append([]PlatformEvent(nil), stream.Events...),
		StreamHash:  stream.StreamHash,
		CreatedAt:   stream.CreatedAt,
		UpdatedAt:   stream.UpdatedAt,
	}, nil
}

// LoadRange returns events with LogicalClock in [fromClock, toClock].
func (s *InMemoryEventStore) LoadRange(_ context.Context, streamID string, fromClock, toClock int64) ([]PlatformEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stream, ok := s.streams[streamID]
	if !ok {
		return nil, fmt.Errorf("stream %q not found", streamID)
	}
	return EventsInRange(stream, fromClock, toClock), nil
}

// LoadByAggregate returns all events for a given tenant + aggregateType + aggregateID,
// sorted in canonical order, across all streams.
func (s *InMemoryEventStore) LoadByAggregate(_ context.Context, tenantID, aggregateType, aggregateID string) ([]PlatformEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var results []PlatformEvent
	for _, stream := range s.streams {
		if stream.TenantID != tenantID {
			continue
		}
		for _, e := range stream.Events {
			if string(e.AggregateType) == aggregateType && e.AggregateID == aggregateID {
				results = append(results, e)
			}
		}
	}
	return SortEvents(results), nil
}
