package indexer

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/janlauber/one-click/pkg/events"
)

// BuildIndex builds an index deterministically from the given event slice.
// Only events relevant to the IndexType are processed.
// Same events + same IndexType always produce the same Index.
func BuildIndex(indexID string, indexType IndexType, tenantID string, evts []events.PlatformEvent) Index {
	sorted := events.SortEvents(evts)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	var entries []IndexEntry
	for _, e := range sorted {
		if e.TenantID != tenantID {
			continue
		}
		if entry, ok := extractEntry(indexType, e); ok {
			entries = append(entries, entry)
		}
	}

	return Index{
		IndexID:       indexID,
		IndexType:     indexType,
		TenantID:      tenantID,
		Entries:       entries,
		RebuildCount:  1,
		LastRebuildAt: now,
		SourceHash:    computeSourceHash(sorted),
	}
}

// RebuildIndex rebuilds an existing index from scratch using new events.
// RebuildCount is incremented. Useful for verifying index freshness.
func RebuildIndex(existing Index, evts []events.PlatformEvent) Index {
	rebuilt := BuildIndex(existing.IndexID, existing.IndexType, existing.TenantID, evts)
	rebuilt.RebuildCount = existing.RebuildCount + 1
	return rebuilt
}

// QueryIndex returns entries whose Attributes contain all specified key=value pairs.
// An empty query returns all entries.
func QueryIndex(index Index, attributes map[string]string) []IndexEntry {
	if len(attributes) == 0 {
		return append([]IndexEntry(nil), index.Entries...)
	}
	var result []IndexEntry
	for _, entry := range index.Entries {
		if matchesAttributes(entry.Attributes, attributes) {
			result = append(result, entry)
		}
	}
	return result
}

// IndexSize returns the number of entries in the index.
func IndexSize(index Index) int {
	return len(index.Entries)
}

// IsStale returns true when the index was built from a different event set than
// the provided events (SourceHash mismatch).
func IsStale(index Index, evts []events.PlatformEvent) bool {
	return index.SourceHash != computeSourceHash(events.SortEvents(evts))
}

// ─── internal ──────────────────────────────────────────────────────────────

func extractEntry(indexType IndexType, e events.PlatformEvent) (IndexEntry, bool) {
	attrs := make(map[string]string)

	switch indexType {
	case IndexTypeExecution:
		if e.AggregateType != events.AggregateTypeExecution {
			return IndexEntry{}, false
		}
		attrs["execution_id"] = e.ExecutionID
		attrs["event_type"] = string(e.EventType)

	case IndexTypeContradiction:
		if e.EventType != events.EventTypeContradictionDetected {
			return IndexEntry{}, false
		}
		attrs["execution_id"] = e.ExecutionID
		attrs["aggregate_id"] = e.AggregateID

	case IndexTypeDrift:
		if e.EventType != events.EventTypeDriftDetected {
			return IndexEntry{}, false
		}
		attrs["execution_id"] = e.ExecutionID
		attrs["aggregate_id"] = e.AggregateID

	case IndexTypeGovernanceAction:
		if e.AggregateType != events.AggregateTypeGovernance {
			return IndexEntry{}, false
		}
		attrs["execution_id"] = e.ExecutionID
		attrs["event_type"] = string(e.EventType)
		attrs["actor_id"] = e.Metadata["actor_id"]

	case IndexTypeProviderObservation:
		if e.EventType != events.EventTypeProviderObservation {
			return IndexEntry{}, false
		}
		attrs["execution_id"] = e.ExecutionID
		attrs["aggregate_id"] = e.AggregateID
		attrs["provider"] = e.Metadata["provider"]

	case IndexTypeTopologyRelationship:
		if e.AggregateType != events.AggregateTypeExecution {
			return IndexEntry{}, false
		}
		attrs["execution_id"] = e.ExecutionID
		attrs["provider"] = e.Metadata["provider"]

	default:
		return IndexEntry{}, false
	}

	// Copy metadata into attributes for searchability.
	for k, v := range e.Metadata {
		if _, exists := attrs[k]; !exists {
			attrs[k] = v
		}
	}

	return IndexEntry{
		EntryID:      fmt.Sprintf("%s:%s", string(indexType), e.EventID),
		IndexType:    indexType,
		ExecutionID:  e.ExecutionID,
		TenantID:     e.TenantID,
		AggregateID:  e.AggregateID,
		EventID:      e.EventID,
		EventType:    string(e.EventType),
		LogicalClock: e.LogicalClock,
		Attributes:   attrs,
		IndexedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}, true
}

func matchesAttributes(entryAttrs, query map[string]string) bool {
	for k, v := range query {
		if got, ok := entryAttrs[k]; !ok || !strings.EqualFold(got, v) {
			return false
		}
	}
	return true
}

func computeSourceHash(sortedEvts []events.PlatformEvent) string {
	if len(sortedEvts) == 0 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte("")))
	}
	h := sha256.New()
	for _, e := range sortedEvts {
		fmt.Fprintf(h, "%s|", e.Hash)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// SortedEntries returns entries sorted by LogicalClock ascending then EntryID.
func SortedEntries(entries []IndexEntry) []IndexEntry {
	sorted := append([]IndexEntry(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].LogicalClock != sorted[j].LogicalClock {
			return sorted[i].LogicalClock < sorted[j].LogicalClock
		}
		return sorted[i].EntryID < sorted[j].EntryID
	})
	return sorted
}
