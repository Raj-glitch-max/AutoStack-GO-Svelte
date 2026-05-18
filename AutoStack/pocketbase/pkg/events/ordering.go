package events

import (
	"sort"
	"strings"
)

// CompareEvents returns negative, zero, or positive per canonical ordering:
//  1. LogicalClock ascending
//  2. Timestamp lexical ascending (RFC3339Nano is lexically sortable)
//  3. EventID lexical ascending
//
// This ordering is total and deterministic — same events always sort the same way.
func CompareEvents(a, b PlatformEvent) int {
	if a.LogicalClock != b.LogicalClock {
		if a.LogicalClock < b.LogicalClock {
			return -1
		}
		return 1
	}
	if a.Timestamp != b.Timestamp {
		return strings.Compare(a.Timestamp, b.Timestamp)
	}
	return strings.Compare(a.EventID, b.EventID)
}

// SortEvents returns a new slice sorted in canonical order. Original is unchanged.
func SortEvents(evts []PlatformEvent) []PlatformEvent {
	sorted := append([]PlatformEvent(nil), evts...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return CompareEvents(sorted[i], sorted[j]) < 0
	})
	return sorted
}

// IsTotallyOrdered returns true when the slice is already in canonical order.
func IsTotallyOrdered(evts []PlatformEvent) bool {
	for i := 1; i < len(evts); i++ {
		if CompareEvents(evts[i-1], evts[i]) > 0 {
			return false
		}
	}
	return true
}

// MaxLogicalClock returns the highest logical clock in the slice, or 0 if empty.
func MaxLogicalClock(evts []PlatformEvent) int64 {
	var max int64
	for _, e := range evts {
		if e.LogicalClock > max {
			max = e.LogicalClock
		}
	}
	return max
}
