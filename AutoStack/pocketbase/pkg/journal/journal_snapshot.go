package journal

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// JournalCheckpoint marks a stable recovery point within a journal.
type JournalCheckpoint struct {
	CheckpointID string `json:"checkpoint_id"`
	ExecutionID  string `json:"execution_id"`
	AtClock      int64  `json:"at_clock"`
	AtJournalID  string `json:"at_journal_id"`
	StateHash    string `json:"state_hash"` // SHA-256 over entry hashes up to this point
}

// ExecutionJournalSnapshot is a tamper-evident, point-in-time export of an ExecutionJournal.
// SnapshotHash covers all stable fields; GeneratedAt is excluded (content-identity).
type ExecutionJournalSnapshot struct {
	SnapshotID     string                  `json:"snapshot_id"`
	ExecutionID    string                  `json:"execution_id"`
	TenantID       string                  `json:"tenant_id"`
	Entries        []ExecutionJournalEntry `json:"entries"`
	Checkpoints    []JournalCheckpoint     `json:"checkpoints"`
	EntryCount     int                     `json:"entry_count"`
	RetryCount     int                     `json:"retry_count"`
	ApprovalCount  int                     `json:"approval_count"`
	Contradictions int                     `json:"contradictions"`
	GeneratedAt    string                  `json:"generated_at"` // excluded from hash
	SnapshotHash   string                  `json:"snapshot_hash"`
}

// BuildJournalSnapshot creates a tamper-evident snapshot of a journal's current state.
// GeneratedAt is recorded but excluded from the hash.
func BuildJournalSnapshot(snapshotID string, j *ExecutionJournal) ExecutionJournalSnapshot {
	entries := j.Entries()
	chain := BuildCausalChain(j.executionID, j.tenantID, entries)
	tl := BuildTimeline(chain)

	snap := ExecutionJournalSnapshot{
		SnapshotID:     snapshotID,
		ExecutionID:    j.executionID,
		TenantID:       j.tenantID,
		Entries:        entries,
		EntryCount:     len(entries),
		RetryCount:     tl.RetryCount,
		ApprovalCount:  tl.ApprovalCount,
		Contradictions: tl.Contradictions,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	snap.SnapshotHash = computeSnapshotHash(snap)
	return snap
}

// VerifyJournalSnapshot recomputes the hash and returns (true, "") when it matches.
func VerifyJournalSnapshot(snap ExecutionJournalSnapshot) (bool, string) {
	stored := snap.SnapshotHash
	snap.SnapshotHash = ""
	expected := computeSnapshotHash(snap)
	if stored != expected {
		return false, fmt.Sprintf("journal snapshot %q hash mismatch: stored=%s computed=%s",
			snap.SnapshotID, stored, expected)
	}
	return true, ""
}

// BuildJournalCheckpoint creates a checkpoint at the given journal entry.
// StateHash covers all entry hashes up to and including the checkpoint entry.
func BuildJournalCheckpoint(checkpointID string, entries []ExecutionJournalEntry, atJournalID string, atClock int64) JournalCheckpoint {
	h := sha256.New()
	for _, e := range entries {
		if e.LogicalClock > atClock {
			break
		}
		fmt.Fprintf(h, "%s|", e.Hash)
	}
	return JournalCheckpoint{
		CheckpointID: checkpointID,
		ExecutionID:  executionIDFromEntries(entries),
		AtClock:      atClock,
		AtJournalID:  atJournalID,
		StateHash:    fmt.Sprintf("%x", h.Sum(nil)),
	}
}

func executionIDFromEntries(entries []ExecutionJournalEntry) string {
	if len(entries) == 0 {
		return ""
	}
	return entries[0].ExecutionID
}

func computeSnapshotHash(snap ExecutionJournalSnapshot) string {
	h := sha256.New()
	fmt.Fprintf(h, "snap:%s|exec:%s|tenant:%s|count:%d|retries:%d|approvals:%d|contradictions:%d|entries:",
		snap.SnapshotID, snap.ExecutionID, snap.TenantID,
		snap.EntryCount, snap.RetryCount, snap.ApprovalCount, snap.Contradictions)
	for _, e := range snap.Entries {
		fmt.Fprintf(h, "%s/%s,", e.JournalID, e.Hash)
	}
	fmt.Fprintf(h, "|checkpoints:")
	for _, cp := range snap.Checkpoints {
		fmt.Fprintf(h, "%s/%s/%d,", cp.CheckpointID, cp.StateHash, cp.AtClock)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
