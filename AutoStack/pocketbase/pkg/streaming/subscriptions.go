package streaming

import (
	"fmt"
	"sync"
)

// Subscription tracks a subscriber's position in a stream.
type Subscription struct {
	SubscriptionID string
	StreamID       string
	TenantID       string
	LastSeenSeq    int64
}

// SubscriptionManager manages subscriptions to execution streams.
// It does not push events — callers poll via Poll().
// Streams are tenant-scoped; a subscriber from one tenant cannot read another tenant's stream.
type SubscriptionManager struct {
	mu            sync.RWMutex
	subscriptions map[string]*Subscription
	streams       map[string]*ExecutionStream
}

// NewSubscriptionManager returns an empty manager.
func NewSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{
		subscriptions: make(map[string]*Subscription),
		streams:       make(map[string]*ExecutionStream),
	}
}

// RegisterStream adds a stream to the manager.
func (m *SubscriptionManager) RegisterStream(s *ExecutionStream) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.streams[s.StreamID()] = s
}

// Subscribe creates a new subscription to a stream. Returns an error when
// the stream does not exist or the tenant does not match.
func (m *SubscriptionManager) Subscribe(subscriptionID, streamID, tenantID string) (*Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.streams[streamID]
	if !ok {
		return nil, fmt.Errorf("stream %q not found", streamID)
	}
	if s.TenantID() != tenantID {
		return nil, fmt.Errorf("tenant %q cannot subscribe to stream %q", tenantID, streamID)
	}
	if _, exists := m.subscriptions[subscriptionID]; exists {
		return nil, fmt.Errorf("subscription %q already exists", subscriptionID)
	}
	sub := &Subscription{
		SubscriptionID: subscriptionID,
		StreamID:       streamID,
		TenantID:       tenantID,
		LastSeenSeq:    0,
	}
	m.subscriptions[subscriptionID] = sub
	return sub, nil
}

// Unsubscribe removes a subscription.
func (m *SubscriptionManager) Unsubscribe(subscriptionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.subscriptions, subscriptionID)
}

// Poll returns new events for a subscription since its last seen sequence,
// and advances LastSeenSeq. Returns an error when the subscription is not found.
func (m *SubscriptionManager) Poll(subscriptionID string) ([]StreamEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub, ok := m.subscriptions[subscriptionID]
	if !ok {
		return nil, fmt.Errorf("subscription %q not found", subscriptionID)
	}
	stream, ok := m.streams[sub.StreamID]
	if !ok {
		return nil, fmt.Errorf("stream %q not found", sub.StreamID)
	}

	events := stream.EventsAfter(sub.LastSeenSeq)
	if len(events) > 0 {
		sub.LastSeenSeq = events[len(events)-1].Sequence
	}
	return events, nil
}
