package journal

// ExecutionTimeline is an immutable, ordered view of all journal entries for one execution.
// Summary counters are derived deterministically from entry types.
type ExecutionTimeline struct {
	ExecutionID    string
	TenantID       string
	Entries        []ExecutionJournalEntry
	TotalSteps     int // unique step IDs that have a step_started entry
	RetryCount     int // number of retry_executed entries
	ApprovalCount  int // number of approval_granted + approval_denied entries
	Contradictions int // number of contradiction_detected entries
}

// BuildTimeline constructs an ExecutionTimeline from a CausalChain.
// All counters are derived from entry types.
func BuildTimeline(chain CausalChain) ExecutionTimeline {
	t := ExecutionTimeline{
		ExecutionID: chain.ExecutionID,
		TenantID:    chain.TenantID,
		Entries:     append([]ExecutionJournalEntry(nil), chain.Entries...),
	}
	stepsSeen := make(map[string]struct{})
	for _, e := range chain.Entries {
		switch e.JournalType {
		case JournalTypeStepStarted:
			stepsSeen[e.StepID] = struct{}{}
		case JournalTypeRetryExecuted:
			t.RetryCount++
		case JournalTypeApprovalGranted, JournalTypeApprovalDenied:
			t.ApprovalCount++
		case JournalTypeContradictionDetected:
			t.Contradictions++
		}
	}
	t.TotalSteps = len(stepsSeen)
	return t
}

// IsComplete returns true when the timeline contains a workflow_completed or workflow_failed entry.
func IsComplete(t ExecutionTimeline) bool {
	for _, e := range t.Entries {
		if e.JournalType == JournalTypeWorkflowCompleted || e.JournalType == JournalTypeWorkflowFailed {
			return true
		}
	}
	return false
}

// LastEntry returns the last entry in the timeline, or (zero, false) if empty.
func LastEntry(t ExecutionTimeline) (ExecutionJournalEntry, bool) {
	if len(t.Entries) == 0 {
		return ExecutionJournalEntry{}, false
	}
	return t.Entries[len(t.Entries)-1], true
}

// EntriesAfterClock returns all timeline entries with LogicalClock > afterClock.
func EntriesAfterClock(t ExecutionTimeline, afterClock int64) []ExecutionJournalEntry {
	var out []ExecutionJournalEntry
	for _, e := range t.Entries {
		if e.LogicalClock > afterClock {
			out = append(out, e)
		}
	}
	return out
}
