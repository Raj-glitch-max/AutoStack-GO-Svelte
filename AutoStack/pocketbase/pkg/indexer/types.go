// Package indexer implements deterministic, event-derived indexes.
//
// Core invariants:
//   - Indexes are NEVER the source of truth — they are derived from events only.
//   - Same event set always produces the same index (deterministic rebuild).
//   - Indexes can be rebuilt from scratch at any time without data loss.
//   - Querying an index never mutates it.
package indexer

// IndexType identifies which kind of operational data is indexed.
type IndexType string

const (
	IndexTypeExecution             IndexType = "execution"
	IndexTypeContradiction         IndexType = "contradiction"
	IndexTypeDrift                 IndexType = "drift"
	IndexTypeGovernanceAction      IndexType = "governance_action"
	IndexTypeProviderObservation   IndexType = "provider_observation"
	IndexTypeTopologyRelationship  IndexType = "topology_relationship"
)

// IndexEntry is one record in an index, derived from a single source event.
type IndexEntry struct {
	EntryID     string            `json:"entry_id"`
	IndexType   IndexType         `json:"index_type"`
	ExecutionID string            `json:"execution_id"`
	TenantID    string            `json:"tenant_id"`
	AggregateID string            `json:"aggregate_id"`
	EventID     string            `json:"event_id"` // source event
	EventType   string            `json:"event_type"`
	LogicalClock int64            `json:"logical_clock"`
	// Attributes holds searchable key=value pairs extracted from the event.
	Attributes map[string]string `json:"attributes"`
	IndexedAt  string            `json:"indexed_at"`
}

// Index is a collection of IndexEntries derived deterministically from events.
// It carries a SourceHash — SHA-256 over the event hashes used to build it —
// so the caller can verify whether the index is stale.
type Index struct {
	IndexID       string       `json:"index_id"`
	IndexType     IndexType    `json:"index_type"`
	TenantID      string       `json:"tenant_id"`
	Entries       []IndexEntry `json:"entries"`
	RebuildCount  int          `json:"rebuild_count"`
	LastRebuildAt string       `json:"last_rebuild_at"`
	// SourceHash is SHA-256 over the hashes of events used to build this index.
	SourceHash string `json:"source_hash"`
}
