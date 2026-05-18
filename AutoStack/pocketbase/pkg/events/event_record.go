package events

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// EventStorageRecord is the serialized form of a PlatformEvent for durable storage.
// It carries the full JSON body plus indexed metadata for efficient queries.
type EventStorageRecord struct {
	StreamID      string `json:"stream_id"`
	EventID       string `json:"event_id"`
	EventType     string `json:"event_type"`
	AggregateType string `json:"aggregate_type"`
	AggregateID   string `json:"aggregate_id"`
	ExecutionID   string `json:"execution_id"`
	TenantID      string `json:"tenant_id"`
	LogicalClock  int64  `json:"logical_clock"`
	Timestamp     string `json:"timestamp"`
	Hash          string `json:"hash"`
	Body          []byte `json:"body"` // JSON-encoded PlatformEvent
}

// RecordPersistence is the storage interface used by PocketBaseEventStore.
// Tests inject InMemoryRecordPersistence; production uses a database-backed implementation.
type RecordPersistence interface {
	AppendRecord(r EventStorageRecord) error
	FindRecord(eventID string) (EventStorageRecord, bool, error)
	FindRecordsByStream(streamID string) ([]EventStorageRecord, error)
	FindRecordsInRange(streamID string, fromClock, toClock int64) ([]EventStorageRecord, error)
	FindRecordsByAggregate(tenantID, aggregateType, aggregateID string) ([]EventStorageRecord, error)
	FindRecordsByTenant(tenantID string) ([]EventStorageRecord, error)
	CurrentClock() int64
	NextClock() (int64, error)
}

// InMemoryRecordPersistence is a thread-safe in-memory implementation of RecordPersistence.
// It provides no durability guarantees — use it in tests.
type InMemoryRecordPersistence struct {
	mu       sync.RWMutex
	records  []EventStorageRecord
	byID     map[string]EventStorageRecord
	byStream map[string][]EventStorageRecord
	clock    int64
}

// NewInMemoryRecordPersistence returns an empty InMemoryRecordPersistence.
func NewInMemoryRecordPersistence() *InMemoryRecordPersistence {
	return &InMemoryRecordPersistence{
		byID:     make(map[string]EventStorageRecord),
		byStream: make(map[string][]EventStorageRecord),
	}
}

func (p *InMemoryRecordPersistence) AppendRecord(r EventStorageRecord) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.byID[r.EventID]; exists {
		return fmt.Errorf("duplicate event ID %q", r.EventID)
	}
	p.records = append(p.records, r)
	p.byID[r.EventID] = r
	p.byStream[r.StreamID] = append(p.byStream[r.StreamID], r)
	return nil
}

func (p *InMemoryRecordPersistence) FindRecord(eventID string) (EventStorageRecord, bool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	r, ok := p.byID[eventID]
	return r, ok, nil
}

func (p *InMemoryRecordPersistence) FindRecordsByStream(streamID string) ([]EventStorageRecord, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	rs := p.byStream[streamID]
	out := make([]EventStorageRecord, len(rs))
	copy(out, rs)
	sortRecordsByClock(out)
	return out, nil
}

func (p *InMemoryRecordPersistence) FindRecordsInRange(streamID string, fromClock, toClock int64) ([]EventStorageRecord, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var out []EventStorageRecord
	for _, r := range p.byStream[streamID] {
		if r.LogicalClock >= fromClock && r.LogicalClock <= toClock {
			out = append(out, r)
		}
	}
	sortRecordsByClock(out)
	return out, nil
}

func (p *InMemoryRecordPersistence) FindRecordsByAggregate(tenantID, aggregateType, aggregateID string) ([]EventStorageRecord, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var out []EventStorageRecord
	for _, r := range p.records {
		if r.TenantID == tenantID && r.AggregateType == aggregateType && r.AggregateID == aggregateID {
			out = append(out, r)
		}
	}
	sortRecordsByClock(out)
	return out, nil
}

func (p *InMemoryRecordPersistence) FindRecordsByTenant(tenantID string) ([]EventStorageRecord, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var out []EventStorageRecord
	for _, r := range p.records {
		if r.TenantID == tenantID {
			out = append(out, r)
		}
	}
	sortRecordsByClock(out)
	return out, nil
}

func (p *InMemoryRecordPersistence) CurrentClock() int64 {
	return atomic.LoadInt64(&p.clock)
}

func (p *InMemoryRecordPersistence) NextClock() (int64, error) {
	return atomic.AddInt64(&p.clock, 1), nil
}

func sortRecordsByClock(rs []EventStorageRecord) {
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0; j-- {
			a, b := rs[j-1], rs[j]
			if a.LogicalClock > b.LogicalClock ||
				(a.LogicalClock == b.LogicalClock && a.Timestamp > b.Timestamp) ||
				(a.LogicalClock == b.LogicalClock && a.Timestamp == b.Timestamp && a.EventID > b.EventID) {
				rs[j-1], rs[j] = rs[j], rs[j-1]
			} else {
				break
			}
		}
	}
}
