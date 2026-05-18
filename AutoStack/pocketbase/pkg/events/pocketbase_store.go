package events

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PocketBaseEventStore implements EventStore using a RecordPersistence backend.
// In production, wire RecordPersistence to a PocketBase-backed implementation.
// In tests, use InMemoryRecordPersistence (no database required).
//
// Durability guarantees depend entirely on the RecordPersistence implementation.
// The store itself enforces:
//   - append-only semantics (no mutation of existing events)
//   - duplicate EventID rejection
//   - hash validation before persistence
//   - clock monotonicity per stream
type PocketBaseEventStore struct {
	mu         sync.RWMutex
	records    RecordPersistence
	seen       map[string]struct{}  // dedup set: EventID → {}
	lastClock  map[string]int64     // streamID → last LogicalClock written
	streamMeta map[string]smeta     // streamID → creation metadata
}

type smeta struct {
	tenantID    string
	executionID string
	createdAt   string
}

// NewPocketBaseEventStore creates a PocketBaseEventStore backed by the given RecordPersistence.
func NewPocketBaseEventStore(rp RecordPersistence) *PocketBaseEventStore {
	return &PocketBaseEventStore{
		records:    rp,
		seen:       make(map[string]struct{}),
		lastClock:  make(map[string]int64),
		streamMeta: make(map[string]smeta),
	}
}

// NextLogicalClock returns a strictly monotonic clock from the underlying persistence layer.
func (s *PocketBaseEventStore) NextLogicalClock(_ context.Context) (int64, error) {
	return s.records.NextClock()
}

// Append validates and durably appends an event to the named stream.
//
// Rejected when:
//   - event hash is invalid
//   - EventID is a duplicate
//   - LogicalClock regresses below the last event in the stream
func (s *PocketBaseEventStore) Append(_ context.Context, streamID string, event PlatformEvent) error {
	if streamID == "" {
		return fmt.Errorf("streamID must not be empty")
	}
	// Hash validation.
	if expected := ComputeEventHash(event); event.Hash != expected {
		return fmt.Errorf("event %q hash invalid: stored %q expected %q",
			event.EventID, event.Hash, expected)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Duplicate EventID check.
	if _, dup := s.seen[event.EventID]; dup {
		return fmt.Errorf("duplicate event ID %q", event.EventID)
	}
	// Clock monotonicity check.
	if last, ok := s.lastClock[streamID]; ok {
		if event.LogicalClock < last {
			return fmt.Errorf("event %q clock %d regresses below stream last %d",
				event.EventID, event.LogicalClock, last)
		}
	}

	body, err := CanonicalSerialize(event)
	if err != nil {
		return fmt.Errorf("serialize event %q: %w", event.EventID, err)
	}
	rec := EventStorageRecord{
		StreamID:      streamID,
		EventID:       event.EventID,
		EventType:     string(event.EventType),
		AggregateType: string(event.AggregateType),
		AggregateID:   event.AggregateID,
		ExecutionID:   event.ExecutionID,
		TenantID:      event.TenantID,
		LogicalClock:  event.LogicalClock,
		Timestamp:     event.Timestamp,
		Hash:          event.Hash,
		Body:          body,
	}
	if err := s.records.AppendRecord(rec); err != nil {
		return fmt.Errorf("persist event %q: %w", event.EventID, err)
	}
	s.seen[event.EventID] = struct{}{}
	s.lastClock[streamID] = event.LogicalClock
	if _, ok := s.streamMeta[streamID]; !ok {
		s.streamMeta[streamID] = smeta{
			tenantID:    event.TenantID,
			executionID: event.ExecutionID,
			createdAt:   time.Now().UTC().Format(time.RFC3339Nano),
		}
	}
	return nil
}

// Load rebuilds and returns the EventStream for streamID from persisted records.
// Returns an error when the stream has no events.
func (s *PocketBaseEventStore) Load(_ context.Context, streamID string) (EventStream, error) {
	s.mu.RLock()
	meta, hasMeta := s.streamMeta[streamID]
	s.mu.RUnlock()

	recs, err := s.records.FindRecordsByStream(streamID)
	if err != nil {
		return EventStream{}, fmt.Errorf("load stream %q: %w", streamID, err)
	}
	if len(recs) == 0 {
		return EventStream{}, fmt.Errorf("stream %q not found", streamID)
	}

	tenantID := recs[0].TenantID
	executionID := recs[0].ExecutionID
	if hasMeta {
		tenantID = meta.tenantID
		executionID = meta.executionID
	}
	stream := NewEventStream(streamID, tenantID, executionID)
	for _, r := range recs {
		evt, err := DeserializeEvent(r.Body)
		if err != nil {
			return EventStream{}, fmt.Errorf("deserialize event %q in stream %q: %w", r.EventID, streamID, err)
		}
		stream, err = AppendEvent(stream, evt)
		if err != nil {
			return EventStream{}, fmt.Errorf("rebuild stream %q at event %q: %w", streamID, r.EventID, err)
		}
	}
	return stream, nil
}

// LoadRange returns events in [fromClock, toClock] for the given stream.
func (s *PocketBaseEventStore) LoadRange(_ context.Context, streamID string, fromClock, toClock int64) ([]PlatformEvent, error) {
	recs, err := s.records.FindRecordsInRange(streamID, fromClock, toClock)
	if err != nil {
		return nil, fmt.Errorf("load range for stream %q: %w", streamID, err)
	}
	return recordsToEvents(recs)
}

// LoadByAggregate returns all events for a tenant + aggregateType + aggregateID, sorted canonically.
func (s *PocketBaseEventStore) LoadByAggregate(_ context.Context, tenantID, aggregateType, aggregateID string) ([]PlatformEvent, error) {
	recs, err := s.records.FindRecordsByAggregate(tenantID, aggregateType, aggregateID)
	if err != nil {
		return nil, fmt.Errorf("load by aggregate (%s/%s/%s): %w", tenantID, aggregateType, aggregateID, err)
	}
	return recordsToEvents(recs)
}

// Records returns the underlying RecordPersistence for direct inspection.
// Use this only in tests and diagnostics — production code must go through EventStore methods.
func (s *PocketBaseEventStore) Records() RecordPersistence {
	return s.records
}

func recordsToEvents(recs []EventStorageRecord) ([]PlatformEvent, error) {
	evts := make([]PlatformEvent, 0, len(recs))
	for _, r := range recs {
		evt, err := DeserializeEvent(r.Body)
		if err != nil {
			return nil, fmt.Errorf("deserialize event %q: %w", r.EventID, err)
		}
		evts = append(evts, evt)
	}
	return SortEvents(evts), nil
}
