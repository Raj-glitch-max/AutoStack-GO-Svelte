package indexer

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// IndexSnapshot is a tamper-evident, point-in-time export of an Index for durable storage.
// SnapshotHash covers all stable fields — SnapshotAt excluded (content-identity).
type IndexSnapshot struct {
	SnapshotID   string       `json:"snapshot_id"`
	IndexID      string       `json:"index_id"`
	TenantID     string       `json:"tenant_id"`
	IndexType    IndexType    `json:"index_type"`
	SourceHash   string       `json:"source_hash"`
	EntryCount   int          `json:"entry_count"`
	RebuildCount int          `json:"rebuild_count"`
	Entries      []IndexEntry `json:"entries"`
	SnapshotAt   string       `json:"snapshot_at"` // excluded from hash
	SnapshotHash string       `json:"snapshot_hash"`
}

// BuildIndexSnapshot creates a tamper-evident snapshot of an Index.
func BuildIndexSnapshot(snapshotID string, idx Index) IndexSnapshot {
	entries := SortedEntries(idx.Entries)
	snap := IndexSnapshot{
		SnapshotID:   snapshotID,
		IndexID:      idx.IndexID,
		TenantID:     idx.TenantID,
		IndexType:    idx.IndexType,
		SourceHash:   idx.SourceHash,
		EntryCount:   len(entries),
		RebuildCount: idx.RebuildCount,
		Entries:      entries,
		SnapshotAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	snap.SnapshotHash = computeIndexSnapshotHash(snap)
	return snap
}

// VerifyIndexSnapshot recomputes the hash and compares it to the stored value.
func VerifyIndexSnapshot(snap IndexSnapshot) (bool, string) {
	stored := snap.SnapshotHash
	snap.SnapshotHash = ""
	expected := computeIndexSnapshotHash(snap)
	if stored != expected {
		return false, fmt.Sprintf("index snapshot hash mismatch: stored=%s computed=%s", stored, expected)
	}
	return true, ""
}

// PersistentIndexStore is a thread-safe, in-memory store of IndexSnapshots.
// It tracks the latest snapshot per index and counts rebuild operations.
//
// Durability note: this implementation is in-memory. For true durability,
// snapshots should be serialized (via BuildIndexSnapshot) and persisted to
// PocketBase or a filesystem backend. That wiring is left for the controller layer.
type PersistentIndexStore struct {
	mu       sync.RWMutex
	latest   map[string]IndexSnapshot // indexID → latest snapshot
	rebuildN map[string]int           // indexID → total rebuild count
}

// NewPersistentIndexStore returns an empty PersistentIndexStore.
func NewPersistentIndexStore() *PersistentIndexStore {
	return &PersistentIndexStore{
		latest:   make(map[string]IndexSnapshot),
		rebuildN: make(map[string]int),
	}
}

// SaveSnapshot stores the latest snapshot for an index, replacing any prior snapshot.
func (s *PersistentIndexStore) SaveSnapshot(snap IndexSnapshot) error {
	ok, reason := VerifyIndexSnapshot(snap)
	if !ok {
		return fmt.Errorf("invalid index snapshot: %s", reason)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latest[snap.IndexID] = snap
	return nil
}

// LoadSnapshot returns the latest snapshot for an index, or (zero, false) if none exists.
func (s *PersistentIndexStore) LoadSnapshot(indexID string) (IndexSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap, ok := s.latest[indexID]
	return snap, ok
}

// IsStale returns true when the stored snapshot's SourceHash differs from currentSourceHash,
// or when no snapshot exists for the index.
func (s *PersistentIndexStore) IsStale(indexID string, currentSourceHash string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap, ok := s.latest[indexID]
	if !ok {
		return true
	}
	return snap.SourceHash != currentSourceHash
}

// RecordRebuild increments the rebuild counter for an index and returns the new count.
func (s *PersistentIndexStore) RecordRebuild(indexID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rebuildN[indexID]++
	return s.rebuildN[indexID]
}

// TotalRebuildCount returns the total number of rebuild operations for an index.
func (s *PersistentIndexStore) TotalRebuildCount(indexID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rebuildN[indexID]
}

// AllIndexIDs returns the list of index IDs with stored snapshots.
func (s *PersistentIndexStore) AllIndexIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.latest))
	for id := range s.latest {
		ids = append(ids, id)
	}
	sortStrings(ids)
	return ids
}

func computeIndexSnapshotHash(snap IndexSnapshot) string {
	h := sha256.New()
	fmt.Fprintf(h, "snap:%s|index:%s|tenant:%s|type:%s|source:%s|count:%d|rebuilds:%d|entries:",
		snap.SnapshotID, snap.IndexID, snap.TenantID, snap.IndexType,
		snap.SourceHash, snap.EntryCount, snap.RebuildCount)
	for _, e := range snap.Entries {
		fmt.Fprintf(h, "%s/%s,", e.EntryID, e.AggregateID)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}
