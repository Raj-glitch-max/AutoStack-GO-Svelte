package events

import "time"

// Subscription is a read-only filter specification for an event stream.
// Subscriptions NEVER mutate state — they describe what to observe.
type Subscription struct {
	SubscriptionID string          `json:"subscription_id"`
	TenantID       string          `json:"tenant_id"`
	AggregateTypes []AggregateType `json:"aggregate_types,omitempty"`
	EventTypes     []EventType     `json:"event_types,omitempty"`
	ExecutionIDs   []string        `json:"execution_ids,omitempty"`
	CreatedAt      string          `json:"created_at"`
}

// NewSubscription creates a subscription with deep-copied filter slices.
func NewSubscription(
	id, tenantID string,
	aggTypes []AggregateType,
	evtTypes []EventType,
) Subscription {
	return Subscription{
		SubscriptionID: id,
		TenantID:       tenantID,
		AggregateTypes: append([]AggregateType(nil), aggTypes...),
		EventTypes:     append([]EventType(nil), evtTypes...),
		CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
}

// MatchesSubscription returns true when the event satisfies all subscription criteria.
// An empty criteria list (e.g., no EventTypes) is treated as "match all" for that criterion.
func MatchesSubscription(event PlatformEvent, sub Subscription) bool {
	if event.TenantID != sub.TenantID {
		return false
	}
	if len(sub.AggregateTypes) > 0 {
		found := false
		for _, at := range sub.AggregateTypes {
			if event.AggregateType == at {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(sub.EventTypes) > 0 {
		found := false
		for _, et := range sub.EventTypes {
			if event.EventType == et {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(sub.ExecutionIDs) > 0 {
		found := false
		for _, id := range sub.ExecutionIDs {
			if event.ExecutionID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// FilterBySubscription returns events matching the subscription in canonical order.
func FilterBySubscription(evts []PlatformEvent, sub Subscription) []PlatformEvent {
	var result []PlatformEvent
	for _, e := range evts {
		if MatchesSubscription(e, sub) {
			result = append(result, e)
		}
	}
	return SortEvents(result)
}
