package journal

import "sort"

// CausalChain is a deterministically ordered slice of journal entries
// for one execution, linked by CausationID / ParentJournalID relationships.
type CausalChain struct {
	ExecutionID string
	TenantID    string
	Entries     []ExecutionJournalEntry
}

// BuildCausalChain reconstructs the causal chain from a flat entry slice.
// Entries are filtered to the given execution+tenant then sorted canonically:
// LogicalClock → Timestamp lexical → JournalID lexical.
// The input slice is never mutated.
func BuildCausalChain(executionID, tenantID string, entries []ExecutionJournalEntry) CausalChain {
	var filtered []ExecutionJournalEntry
	for _, e := range entries {
		if e.ExecutionID == executionID && e.TenantID == tenantID {
			filtered = append(filtered, e)
		}
	}
	return CausalChain{
		ExecutionID: executionID,
		TenantID:    tenantID,
		Entries:     sortEntriesCanonical(filtered),
	}
}

// BuildExecutionTimeline returns entries from the chain with LogicalClock in [fromClock, toClock].
func BuildExecutionTimeline(chain CausalChain, fromClock, toClock int64) []ExecutionJournalEntry {
	var out []ExecutionJournalEntry
	for _, e := range chain.Entries {
		if e.LogicalClock >= fromClock && e.LogicalClock <= toClock {
			out = append(out, e)
		}
	}
	return out
}

// BuildStepTimeline returns all entries for the given stepID from the chain in causal order.
func BuildStepTimeline(chain CausalChain, stepID string) []ExecutionJournalEntry {
	var out []ExecutionJournalEntry
	for _, e := range chain.Entries {
		if e.StepID == stepID {
			out = append(out, e)
		}
	}
	return out
}

// CausalDescendants returns all entries whose ParentJournalID or CausationID
// matches the given journalID, in canonical order.
func CausalDescendants(chain CausalChain, journalID string) []ExecutionJournalEntry {
	var out []ExecutionJournalEntry
	for _, e := range chain.Entries {
		if e.ParentJournalID == journalID || e.CausationID == journalID {
			out = append(out, e)
		}
	}
	return out
}

// EntriesByCorrelation returns all entries sharing the given correlationID.
func EntriesByCorrelation(chain CausalChain, correlationID string) []ExecutionJournalEntry {
	var out []ExecutionJournalEntry
	for _, e := range chain.Entries {
		if e.CorrelationID == correlationID {
			out = append(out, e)
		}
	}
	return out
}

func sortEntriesCanonical(entries []ExecutionJournalEntry) []ExecutionJournalEntry {
	out := make([]ExecutionJournalEntry, len(entries))
	copy(out, entries)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].LogicalClock != out[j].LogicalClock {
			return out[i].LogicalClock < out[j].LogicalClock
		}
		if out[i].Timestamp != out[j].Timestamp {
			return out[i].Timestamp < out[j].Timestamp
		}
		return out[i].JournalID < out[j].JournalID
	})
	return out
}
