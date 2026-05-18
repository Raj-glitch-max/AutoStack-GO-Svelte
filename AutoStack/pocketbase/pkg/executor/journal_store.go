package executor

import (
	"errors"
	"sort"
	"sync"

	"github.com/janlauber/one-click/pkg/orchestrator"
)

// JournalStore persists execution journal entries and state transitions.
// All writes must be durable before the caller proceeds — journal entries
// are the source of truth for replay after restart.
// Implementations must be safe for concurrent use.
type JournalStore interface {
	// Append adds a journal entry. Returns error if ExecutionID is empty.
	Append(entry orchestrator.ExecutionJournalEntry) error
	// List returns all entries for an execution, ordered by Timestamp ascending.
	List(executionID string) ([]orchestrator.ExecutionJournalEntry, error)
	// AppendTransition persists a state transition before it is applied.
	AppendTransition(t ExecutionTransition) error
	// ListTransitions returns all transitions for an execution ordered by Timestamp ascending.
	ListTransitions(executionID string) ([]ExecutionTransition, error)
}

// InMemoryJournalStore is a thread-safe, in-memory JournalStore for testing.
// PocketBase-backed production implementations follow the same interface.
type InMemoryJournalStore struct {
	mu          sync.RWMutex
	entries     map[string][]orchestrator.ExecutionJournalEntry
	transitions map[string][]ExecutionTransition
}

func NewInMemoryJournalStore() *InMemoryJournalStore {
	return &InMemoryJournalStore{
		entries:     make(map[string][]orchestrator.ExecutionJournalEntry),
		transitions: make(map[string][]ExecutionTransition),
	}
}

func (s *InMemoryJournalStore) Append(entry orchestrator.ExecutionJournalEntry) error {
	if entry.ExecutionID == "" {
		return errors.New("journal: entry must have non-empty ExecutionID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[entry.ExecutionID] = append(s.entries[entry.ExecutionID], entry)
	return nil
}

func (s *InMemoryJournalStore) List(executionID string) ([]orchestrator.ExecutionJournalEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.entries[executionID]
	result := make([]orchestrator.ExecutionJournalEntry, len(src))
	copy(result, src)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp < result[j].Timestamp
	})
	return result, nil
}

func (s *InMemoryJournalStore) AppendTransition(t ExecutionTransition) error {
	if t.ExecutionID == "" {
		return errors.New("journal: transition must have non-empty ExecutionID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transitions[t.ExecutionID] = append(s.transitions[t.ExecutionID], t)
	return nil
}

func (s *InMemoryJournalStore) ListTransitions(executionID string) ([]ExecutionTransition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.transitions[executionID]
	result := make([]ExecutionTransition, len(src))
	copy(result, src)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp < result[j].Timestamp
	})
	return result, nil
}
