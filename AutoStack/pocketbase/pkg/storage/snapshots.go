package storage

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// IndexRebuildStatus captures the result of an index rebuild operation.
type IndexRebuildStatus struct {
	IndexID       string `json:"index_id"`
	RebuildCount  int    `json:"rebuild_count"`
	SourceHash    string `json:"source_hash"`
	IsStale       bool   `json:"is_stale"`
	LastRebuiltAt string `json:"last_rebuilt_at"`
}

// LeaseIntegritySummary summarises the lease state found during durability assessment.
type LeaseIntegritySummary struct {
	TotalLeases    int    `json:"total_leases"`
	ConflictCount  int    `json:"conflict_count"`  // split-brain lease pairs
	ExpiredCount   int    `json:"expired_count"`
	IntegrityOK    bool   `json:"integrity_ok"`
}

// ArchiveIntegritySummary summarises the archive state found during durability assessment.
type ArchiveIntegritySummary struct {
	TotalEntries     int    `json:"total_entries"`
	InvalidHashCount int    `json:"invalid_hash_count"`
	ManifestOK       bool   `json:"manifest_ok"`
}

// DurabilitySnapshot is a tamper-evident, point-in-time report of the platform's
// durable substrate health. Used for operator review and automated recovery gating.
//
// GeneratedAt and SnapshotHash are excluded from the hash input (content-identity).
type DurabilitySnapshot struct {
	SnapshotID         string                  `json:"snapshot_id"`
	TenantID           string                  `json:"tenant_id"`
	WALMetadata        WALMetadata             `json:"wal_metadata"`
	StreamHashes       map[string]string       `json:"stream_hashes"`        // streamID → StreamHash
	CheckpointHashes   map[string]string       `json:"checkpoint_hashes"`    // checkpointID → StateHash
	CorruptionFindings []CorruptionFinding     `json:"corruption_findings"`
	RebuildStatus      []IndexRebuildStatus    `json:"rebuild_status"`
	LeaseIntegrity     LeaseIntegritySummary   `json:"lease_integrity"`
	ArchiveIntegrity   ArchiveIntegritySummary `json:"archive_integrity"`
	GeneratedAt        string                  `json:"generated_at"` // excluded from hash
	SnapshotHash       string                  `json:"snapshot_hash"`
}

// BuildDurabilitySnapshot constructs and hashes a DurabilitySnapshot.
// All slices are deep-copied.
func BuildDurabilitySnapshot(
	snapshotID string,
	tenantID string,
	walMeta WALMetadata,
	streamHashes map[string]string,
	checkpointHashes map[string]string,
	findings []CorruptionFinding,
	rebuildStatus []IndexRebuildStatus,
	leaseIntegrity LeaseIntegritySummary,
	archiveIntegrity ArchiveIntegritySummary,
) DurabilitySnapshot {
	shCopy := make(map[string]string, len(streamHashes))
	for k, v := range streamHashes {
		shCopy[k] = v
	}
	cpCopy := make(map[string]string, len(checkpointHashes))
	for k, v := range checkpointHashes {
		cpCopy[k] = v
	}
	findCopy := make([]CorruptionFinding, len(findings))
	copy(findCopy, findings)
	rebuildCopy := make([]IndexRebuildStatus, len(rebuildStatus))
	copy(rebuildCopy, rebuildStatus)

	snap := DurabilitySnapshot{
		SnapshotID:         snapshotID,
		TenantID:           tenantID,
		WALMetadata:        walMeta,
		StreamHashes:       shCopy,
		CheckpointHashes:   cpCopy,
		CorruptionFindings: findCopy,
		RebuildStatus:      rebuildCopy,
		LeaseIntegrity:     leaseIntegrity,
		ArchiveIntegrity:   archiveIntegrity,
		GeneratedAt:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	snap.SnapshotHash = computeDurabilitySnapshotHash(snap)
	return snap
}

// VerifyDurabilitySnapshot recomputes the hash and compares to the stored value.
func VerifyDurabilitySnapshot(snap DurabilitySnapshot) (bool, string) {
	stored := snap.SnapshotHash
	snap.SnapshotHash = ""
	expected := computeDurabilitySnapshotHash(snap)
	if stored != expected {
		return false, fmt.Sprintf("durability snapshot hash mismatch: stored=%s computed=%s", stored, expected)
	}
	return true, ""
}

// IsSafeToOperate returns true when the snapshot indicates no critical corruption
// and the WAL integrity is not "corrupted".
func IsSafeToOperate(snap DurabilitySnapshot) bool {
	if snap.WALMetadata.IntegrityStatus == "corrupted" {
		return false
	}
	for _, f := range snap.CorruptionFindings {
		switch f.Type {
		case CorruptionTypeInvalidHash, CorruptionTypeClockRegression,
			CorruptionTypeWALEntryHashInvalid, CorruptionTypeReplayDivergence:
			return false
		}
	}
	return true
}

func computeDurabilitySnapshotHash(snap DurabilitySnapshot) string {
	h := sha256.New()
	fmt.Fprintf(h, "snap:%s|tenant:%s|wal_entries:%d|wal_status:%s|",
		snap.SnapshotID, snap.TenantID,
		snap.WALMetadata.EntryCount, snap.WALMetadata.IntegrityStatus)
	// Stream hashes — sort keys for determinism.
	streamKeys := sortedKeys(snap.StreamHashes)
	for _, k := range streamKeys {
		fmt.Fprintf(h, "stream:%s=%s,", k, snap.StreamHashes[k])
	}
	fmt.Fprintf(h, "|checkpoints:")
	cpKeys := sortedKeys(snap.CheckpointHashes)
	for _, k := range cpKeys {
		fmt.Fprintf(h, "cp:%s=%s,", k, snap.CheckpointHashes[k])
	}
	fmt.Fprintf(h, "|findings:%d|", len(snap.CorruptionFindings))
	for _, f := range snap.CorruptionFindings {
		fmt.Fprintf(h, "%s/%s,", f.FindingID, f.Type)
	}
	fmt.Fprintf(h, "|lease_conflicts:%d|archive_invalid:%d",
		snap.LeaseIntegrity.ConflictCount, snap.ArchiveIntegrity.InvalidHashCount)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
