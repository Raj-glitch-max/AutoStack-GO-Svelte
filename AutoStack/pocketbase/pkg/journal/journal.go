package journal

import (
	"fmt"
	"sync"
)

// ExecutionJournal is an append-only, thread-safe store for one execution's journal entries.
// Every entry is hash-verified before insertion. Duplicate JournalIDs are rejected.
type ExecutionJournal struct {
	mu          sync.RWMutex
	executionID string
	tenantID    string
	entries     []ExecutionJournalEntry
	entryIndex  map[string]int // journalID → slice index
}

// NewExecutionJournal creates an empty journal for the given execution.
func NewExecutionJournal(executionID, tenantID string) *ExecutionJournal {
	return &ExecutionJournal{
		executionID: executionID,
		tenantID:    tenantID,
		entryIndex:  make(map[string]int),
	}
}

// Append validates and appends an entry. Errors on: wrong execution/tenant, invalid hash, or duplicate ID.
func (j *ExecutionJournal) Append(entry ExecutionJournalEntry) error {
	if entry.ExecutionID != j.executionID {
		return fmt.Errorf("entry executionID %q does not match journal %q", entry.ExecutionID, j.executionID)
	}
	if entry.TenantID != j.tenantID {
		return fmt.Errorf("entry tenantID %q does not match journal %q", entry.TenantID, j.tenantID)
	}
	if ok, reason := VerifyJournalEntry(entry); !ok {
		return fmt.Errorf("journal entry hash invalid: %s", reason)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, dup := j.entryIndex[entry.JournalID]; dup {
		return fmt.Errorf("duplicate journal entry ID %q", entry.JournalID)
	}
	j.entryIndex[entry.JournalID] = len(j.entries)
	j.entries = append(j.entries, entry)
	return nil
}

// Entries returns a copy of all entries in insertion order.
func (j *ExecutionJournal) Entries() []ExecutionJournalEntry {
	j.mu.RLock()
	defer j.mu.RUnlock()
	out := make([]ExecutionJournalEntry, len(j.entries))
	copy(out, j.entries)
	return out
}

// FindEntry returns the entry with the given JournalID, or (zero, false) if absent.
func (j *ExecutionJournal) FindEntry(journalID string) (ExecutionJournalEntry, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	idx, ok := j.entryIndex[journalID]
	if !ok {
		return ExecutionJournalEntry{}, false
	}
	return j.entries[idx], true
}

// EntriesForStep returns all entries for the given stepID in insertion order.
func (j *ExecutionJournal) EntriesForStep(stepID string) []ExecutionJournalEntry {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var out []ExecutionJournalEntry
	for _, e := range j.entries {
		if e.StepID == stepID {
			out = append(out, e)
		}
	}
	return out
}

// EntriesByType returns all entries of the given JournalType in insertion order.
func (j *ExecutionJournal) EntriesByType(t JournalType) []ExecutionJournalEntry {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var out []ExecutionJournalEntry
	for _, e := range j.entries {
		if e.JournalType == t {
			out = append(out, e)
		}
	}
	return out
}

// Size returns the total number of journal entries.
func (j *ExecutionJournal) Size() int {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return len(j.entries)
}

// GetExecutionID returns the execution this journal belongs to.
func (j *ExecutionJournal) GetExecutionID() string { return j.executionID }

// GetTenantID returns the tenant this journal belongs to.
func (j *ExecutionJournal) GetTenantID() string { return j.tenantID }
