package observability

// TimelineEntry is a single ordered entry in an observability timeline.
type TimelineEntry struct {
	Sequence     int64  `json:"sequence"`
	LogicalClock int64  `json:"logical_clock"`
	Category     string `json:"category"` // task, workflow, governance, contradiction, approval
	EventRef     string `json:"event_ref"`
	Summary      string `json:"summary"`
}

// ObservabilityTimeline is a read-only ordered view of execution events.
// Derived from append-only stores — never authoritative.
type ObservabilityTimeline struct {
	ExecutionID string          `json:"execution_id"`
	TenantID    string          `json:"tenant_id"`
	Entries     []TimelineEntry `json:"entries"`
}

// StreamExecutionTimeline constructs an ordered timeline from raw entries.
// Deterministic: same entries always produce the same ordering (by LogicalClock, then Sequence).
// This is a read-only view — it never modifies the underlying stores.
func StreamExecutionTimeline(executionID, tenantID string, entries []TimelineEntry) ObservabilityTimeline {
	sorted := make([]TimelineEntry, len(entries))
	copy(sorted, entries)
	sortTimelineEntries(sorted)
	return ObservabilityTimeline{
		ExecutionID: executionID,
		TenantID:    tenantID,
		Entries:     sorted,
	}
}

// EntriesAfterClock returns all entries with LogicalClock > afterClock.
func (t ObservabilityTimeline) EntriesAfterClock(afterClock int64) []TimelineEntry {
	var out []TimelineEntry
	for _, e := range t.Entries {
		if e.LogicalClock > afterClock {
			out = append(out, e)
		}
	}
	return out
}

// sortTimelineEntries sorts in place: by LogicalClock ascending, then Sequence ascending.
func sortTimelineEntries(entries []TimelineEntry) {
	n := len(entries)
	for i := 1; i < n; i++ {
		for j := i; j > 0; j-- {
			a, b := entries[j-1], entries[j]
			if a.LogicalClock > b.LogicalClock || (a.LogicalClock == b.LogicalClock && a.Sequence > b.Sequence) {
				entries[j-1], entries[j] = entries[j], entries[j-1]
			} else {
				break
			}
		}
	}
}
