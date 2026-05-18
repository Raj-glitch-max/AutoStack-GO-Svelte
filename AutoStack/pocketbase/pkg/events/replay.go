package events

// FilterByExecution returns events for a specific executionID in canonical order.
func FilterByExecution(evts []PlatformEvent, executionID string) []PlatformEvent {
	var result []PlatformEvent
	for _, e := range evts {
		if e.ExecutionID == executionID {
			result = append(result, e)
		}
	}
	return SortEvents(result)
}

// FilterByTenant returns events for a specific tenantID in canonical order.
func FilterByTenant(evts []PlatformEvent, tenantID string) []PlatformEvent {
	var result []PlatformEvent
	for _, e := range evts {
		if e.TenantID == tenantID {
			result = append(result, e)
		}
	}
	return SortEvents(result)
}

// FilterByEventTypes returns events matching any of the given event types, in canonical order.
func FilterByEventTypes(evts []PlatformEvent, types ...EventType) []PlatformEvent {
	set := make(map[EventType]struct{}, len(types))
	for _, t := range types {
		set[t] = struct{}{}
	}
	var result []PlatformEvent
	for _, e := range evts {
		if _, ok := set[e.EventType]; ok {
			result = append(result, e)
		}
	}
	return SortEvents(result)
}

// FilterByTimeRange returns events with Timestamp in [from, to] (lexical RFC3339Nano comparison).
func FilterByTimeRange(evts []PlatformEvent, from, to string) []PlatformEvent {
	var result []PlatformEvent
	for _, e := range evts {
		if e.Timestamp >= from && e.Timestamp <= to {
			result = append(result, e)
		}
	}
	return SortEvents(result)
}

// FilterByAggregateType returns events for a specific aggregate type.
func FilterByAggregateType(evts []PlatformEvent, aggType AggregateType) []PlatformEvent {
	var result []PlatformEvent
	for _, e := range evts {
		if e.AggregateType == aggType {
			result = append(result, e)
		}
	}
	return SortEvents(result)
}
