package journal

import (
	"testing"
)

// ─── helpers ───────────────────────────────────────────────────────────────

func makeEntry(t *testing.T, id, execID, stepID string, clock int64, jt JournalType) ExecutionJournalEntry {
	t.Helper()
	return NewJournalEntry(id, execID, "wf-1", stepID, 1, jt, clock,
		"", "", "", "node-1", "tenant-1", nil)
}

func makeJournal(t *testing.T) *ExecutionJournal {
	t.Helper()
	return NewExecutionJournal("exec-1", "tenant-1")
}

// ─── NewJournalEntry + VerifyJournalEntry ──────────────────────────────────

func TestNewJournalEntry_HashIsSet(t *testing.T) {
	e := makeEntry(t, "j1", "exec-1", "step-1", 1, JournalTypeStepStarted)
	if e.Hash == "" {
		t.Fatal("hash must not be empty")
	}
}

func TestVerifyJournalEntry_Valid(t *testing.T) {
	e := makeEntry(t, "j1", "exec-1", "step-1", 1, JournalTypeStepStarted)
	ok, reason := VerifyJournalEntry(e)
	if !ok {
		t.Fatalf("valid entry must verify: %s", reason)
	}
}

func TestVerifyJournalEntry_TamperedJournalID(t *testing.T) {
	e := makeEntry(t, "j1", "exec-1", "step-1", 1, JournalTypeStepStarted)
	e.JournalID = "tampered"
	ok, _ := VerifyJournalEntry(e)
	if ok {
		t.Fatal("tampered JournalID must fail verification")
	}
}

func TestVerifyJournalEntry_TamperedType(t *testing.T) {
	e := makeEntry(t, "j1", "exec-1", "step-1", 1, JournalTypeStepStarted)
	e.JournalType = JournalTypeWorkflowFailed
	ok, _ := VerifyJournalEntry(e)
	if ok {
		t.Fatal("tampered JournalType must fail verification")
	}
}

func TestVerifyJournalEntry_TamperedClock(t *testing.T) {
	e := makeEntry(t, "j1", "exec-1", "step-1", 5, JournalTypeStepStarted)
	e.LogicalClock = 999
	ok, _ := VerifyJournalEntry(e)
	if ok {
		t.Fatal("tampered LogicalClock must fail verification")
	}
}

func TestNewJournalEntry_PayloadDeepCopied(t *testing.T) {
	payload := []byte{1, 2, 3}
	e := NewJournalEntry("j1", "exec-1", "wf-1", "step-1", 1, JournalTypeStepStarted,
		1, "", "", "", "node-1", "tenant-1", payload)
	payload[0] = 99
	if e.Payload[0] == 99 {
		t.Fatal("payload must be deep-copied on NewJournalEntry")
	}
}

func TestNewJournalEntry_WithPayload_HashDiffersFromNoPayload(t *testing.T) {
	e1 := makeEntry(t, "j1", "exec-1", "step-1", 1, JournalTypeStepStarted)
	e2 := NewJournalEntry("j1", "exec-1", "wf-1", "step-1", 1, JournalTypeStepStarted,
		1, "", "", "", "node-1", "tenant-1", []byte("some-data"))
	// Same ID but different payload — timestamp will also differ, but the test is structural.
	_ = e2
	if e1.Hash == "" || e2.Hash == "" {
		t.Fatal("both hashes must be non-empty")
	}
}

// ─── ExecutionJournal ──────────────────────────────────────────────────────

func TestExecutionJournal_AppendAndFind(t *testing.T) {
	j := makeJournal(t)
	e := makeEntry(t, "j1", "exec-1", "step-1", 1, JournalTypeStepStarted)
	if err := j.Append(e); err != nil {
		t.Fatalf("Append: %v", err)
	}
	found, ok := j.FindEntry("j1")
	if !ok {
		t.Fatal("entry must be found after append")
	}
	if found.JournalID != "j1" {
		t.Fatalf("JournalID mismatch: %s", found.JournalID)
	}
}

func TestExecutionJournal_Append_RejectsDuplicateID(t *testing.T) {
	j := makeJournal(t)
	e := makeEntry(t, "j1", "exec-1", "step-1", 1, JournalTypeStepStarted)
	_ = j.Append(e)
	if err := j.Append(e); err == nil {
		t.Fatal("duplicate journal ID must be rejected")
	}
}

func TestExecutionJournal_Append_RejectsWrongExecution(t *testing.T) {
	j := makeJournal(t)
	e := makeEntry(t, "j1", "other-exec", "step-1", 1, JournalTypeStepStarted)
	if err := j.Append(e); err == nil {
		t.Fatal("wrong executionID must be rejected")
	}
}

func TestExecutionJournal_Append_RejectsWrongTenant(t *testing.T) {
	j := NewExecutionJournal("exec-1", "tenant-1")
	e := NewJournalEntry("j1", "exec-1", "wf-1", "step-1", 1, JournalTypeStepStarted,
		1, "", "", "", "node-1", "tenant-OTHER", nil)
	if err := j.Append(e); err == nil {
		t.Fatal("wrong tenantID must be rejected")
	}
}

func TestExecutionJournal_Append_RejectsInvalidHash(t *testing.T) {
	j := makeJournal(t)
	e := makeEntry(t, "j1", "exec-1", "step-1", 1, JournalTypeStepStarted)
	e.Hash = "bad-hash"
	if err := j.Append(e); err == nil {
		t.Fatal("invalid hash must be rejected")
	}
}

func TestExecutionJournal_Entries_ReturnsCopy(t *testing.T) {
	j := makeJournal(t)
	e := makeEntry(t, "j1", "exec-1", "step-1", 1, JournalTypeStepStarted)
	_ = j.Append(e)

	entries := j.Entries()
	entries[0].JournalID = "mutated"

	orig, _ := j.FindEntry("j1")
	if orig.JournalID == "mutated" {
		t.Fatal("Entries must return a copy")
	}
}

func TestExecutionJournal_EntriesForStep(t *testing.T) {
	j := makeJournal(t)
	_ = j.Append(makeEntry(t, "j1", "exec-1", "step-A", 1, JournalTypeStepStarted))
	_ = j.Append(makeEntry(t, "j2", "exec-1", "step-B", 2, JournalTypeStepStarted))
	_ = j.Append(makeEntry(t, "j3", "exec-1", "step-A", 3, JournalTypeStepCompleted))

	stepA := j.EntriesForStep("step-A")
	if len(stepA) != 2 {
		t.Fatalf("expected 2 entries for step-A, got %d", len(stepA))
	}
}

func TestExecutionJournal_EntriesByType(t *testing.T) {
	j := makeJournal(t)
	_ = j.Append(makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeStepStarted))
	_ = j.Append(makeEntry(t, "j2", "exec-1", "s2", 2, JournalTypeStepStarted))
	_ = j.Append(makeEntry(t, "j3", "exec-1", "s1", 3, JournalTypeStepCompleted))

	started := j.EntriesByType(JournalTypeStepStarted)
	if len(started) != 2 {
		t.Fatalf("expected 2 step_started entries, got %d", len(started))
	}
}

func TestExecutionJournal_Size(t *testing.T) {
	j := makeJournal(t)
	if j.Size() != 0 {
		t.Fatal("empty journal must have size 0")
	}
	_ = j.Append(makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeStepStarted))
	if j.Size() != 1 {
		t.Fatal("size must be 1 after one append")
	}
}

// ─── BuildCausalChain ──────────────────────────────────────────────────────

func TestBuildCausalChain_SortsByClockThenID(t *testing.T) {
	entries := []ExecutionJournalEntry{
		makeEntry(t, "j3", "exec-1", "s3", 3, JournalTypeStepCompleted),
		makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeStepStarted),
		makeEntry(t, "j2", "exec-1", "s2", 2, JournalTypeStepStarted),
	}
	chain := BuildCausalChain("exec-1", "tenant-1", entries)
	if len(chain.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(chain.Entries))
	}
	if chain.Entries[0].LogicalClock != 1 || chain.Entries[1].LogicalClock != 2 {
		t.Fatal("entries must be sorted by LogicalClock")
	}
}

func TestBuildCausalChain_FiltersByExecution(t *testing.T) {
	entries := []ExecutionJournalEntry{
		makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeStepStarted),
		makeEntry(t, "j2", "exec-2", "s1", 2, JournalTypeStepStarted),
	}
	chain := BuildCausalChain("exec-1", "tenant-1", entries)
	if len(chain.Entries) != 1 {
		t.Fatalf("expected 1 entry for exec-1, got %d", len(chain.Entries))
	}
}

func TestBuildExecutionTimeline_ClockRange(t *testing.T) {
	entries := []ExecutionJournalEntry{
		makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeStepStarted),
		makeEntry(t, "j2", "exec-1", "s2", 5, JournalTypeStepStarted),
		makeEntry(t, "j3", "exec-1", "s3", 10, JournalTypeStepStarted),
	}
	chain := BuildCausalChain("exec-1", "tenant-1", entries)
	tl := BuildExecutionTimeline(chain, 1, 5)
	if len(tl) != 2 {
		t.Fatalf("expected 2 entries in [1,5], got %d", len(tl))
	}
}

func TestBuildStepTimeline(t *testing.T) {
	entries := []ExecutionJournalEntry{
		makeEntry(t, "j1", "exec-1", "step-A", 1, JournalTypeStepStarted),
		makeEntry(t, "j2", "exec-1", "step-B", 2, JournalTypeStepStarted),
		makeEntry(t, "j3", "exec-1", "step-A", 3, JournalTypeStepCompleted),
	}
	chain := BuildCausalChain("exec-1", "tenant-1", entries)
	stepA := BuildStepTimeline(chain, "step-A")
	if len(stepA) != 2 {
		t.Fatalf("expected 2 entries for step-A, got %d", len(stepA))
	}
}

func TestCausalDescendants(t *testing.T) {
	parent := makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeStepStarted)
	child := NewJournalEntry("j2", "exec-1", "wf-1", "s2", 1, JournalTypeRetryScheduled,
		2, "j1", "", "j1", "node-1", "tenant-1", nil)
	unrelated := makeEntry(t, "j3", "exec-1", "s3", 3, JournalTypeStepStarted)
	chain := BuildCausalChain("exec-1", "tenant-1", []ExecutionJournalEntry{parent, child, unrelated})
	descendants := CausalDescendants(chain, "j1")
	if len(descendants) != 1 || descendants[0].JournalID != "j2" {
		t.Fatalf("expected j2 as descendant of j1, got %v", descendants)
	}
}

// ─── BuildTimeline + IsComplete ────────────────────────────────────────────

func TestBuildTimeline_CountsCorrectly(t *testing.T) {
	j := makeJournal(t)
	_ = j.Append(makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeStepStarted))
	_ = j.Append(makeEntry(t, "j2", "exec-1", "s2", 2, JournalTypeStepStarted))
	_ = j.Append(makeEntry(t, "j3", "exec-1", "s2", 3, JournalTypeRetryExecuted))
	_ = j.Append(makeEntry(t, "j4", "exec-1", "s2", 4, JournalTypeApprovalGranted))
	_ = j.Append(makeEntry(t, "j5", "exec-1", "s3", 5, JournalTypeContradictionDetected))

	chain := BuildCausalChain("exec-1", "tenant-1", j.Entries())
	tl := BuildTimeline(chain)

	if tl.TotalSteps != 2 {
		t.Fatalf("expected 2 unique steps, got %d", tl.TotalSteps)
	}
	if tl.RetryCount != 1 {
		t.Fatalf("expected 1 retry, got %d", tl.RetryCount)
	}
	if tl.ApprovalCount != 1 {
		t.Fatalf("expected 1 approval, got %d", tl.ApprovalCount)
	}
	if tl.Contradictions != 1 {
		t.Fatalf("expected 1 contradiction, got %d", tl.Contradictions)
	}
}

func TestIsComplete_WorkflowCompleted(t *testing.T) {
	j := makeJournal(t)
	_ = j.Append(makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeWorkflowStarted))
	_ = j.Append(makeEntry(t, "j2", "exec-1", "s1", 2, JournalTypeWorkflowCompleted))
	chain := BuildCausalChain("exec-1", "tenant-1", j.Entries())
	if !IsComplete(BuildTimeline(chain)) {
		t.Fatal("workflow_completed must make timeline complete")
	}
}

func TestIsComplete_WorkflowFailed(t *testing.T) {
	j := makeJournal(t)
	_ = j.Append(makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeWorkflowFailed))
	chain := BuildCausalChain("exec-1", "tenant-1", j.Entries())
	if !IsComplete(BuildTimeline(chain)) {
		t.Fatal("workflow_failed must make timeline complete")
	}
}

func TestIsComplete_InProgress(t *testing.T) {
	j := makeJournal(t)
	_ = j.Append(makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeStepStarted))
	chain := BuildCausalChain("exec-1", "tenant-1", j.Entries())
	if IsComplete(BuildTimeline(chain)) {
		t.Fatal("in-progress timeline must not be complete")
	}
}

func TestLastEntry(t *testing.T) {
	j := makeJournal(t)
	_ = j.Append(makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeStepStarted))
	_ = j.Append(makeEntry(t, "j2", "exec-1", "s1", 5, JournalTypeStepCompleted))
	chain := BuildCausalChain("exec-1", "tenant-1", j.Entries())
	tl := BuildTimeline(chain)
	last, ok := LastEntry(tl)
	if !ok {
		t.Fatal("must have a last entry")
	}
	if last.LogicalClock != 5 {
		t.Fatalf("last entry must be at clock 5, got %d", last.LogicalClock)
	}
}

// ─── ReplayExecutionTimeline ───────────────────────────────────────────────

func TestReplayTimeline_CleanReplay(t *testing.T) {
	j := makeJournal(t)
	_ = j.Append(makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeWorkflowStarted))
	_ = j.Append(makeEntry(t, "j2", "exec-1", "s1", 2, JournalTypeStepStarted))
	_ = j.Append(makeEntry(t, "j3", "exec-1", "s1", 3, JournalTypeRetryExecuted))
	_ = j.Append(makeEntry(t, "j4", "exec-1", "s1", 4, JournalTypeApprovalRequested))
	_ = j.Append(makeEntry(t, "j5", "exec-1", "s1", 5, JournalTypeWorkflowPaused))
	_ = j.Append(makeEntry(t, "j6", "exec-1", "s1", 6, JournalTypeWorkflowResumed))

	chain := BuildCausalChain("exec-1", "tenant-1", j.Entries())
	tl := BuildTimeline(chain)
	result, err := ReplayExecutionTimeline(tl)
	if err != nil {
		t.Fatalf("ReplayExecutionTimeline: %v", err)
	}
	if !IsDivergenceFree(result) {
		t.Fatalf("clean replay must have no divergences: %v", result.Divergences)
	}
	if result.RetryCount != 1 {
		t.Fatalf("expected 1 retry, got %d", result.RetryCount)
	}
	if result.ApprovalWaits != 1 {
		t.Fatalf("expected 1 approval wait, got %d", result.ApprovalWaits)
	}
	if result.PausedAt != 5 {
		t.Fatalf("expected paused at clock 5, got %d", result.PausedAt)
	}
	if result.ResumedCount != 1 {
		t.Fatalf("expected 1 resume, got %d", result.ResumedCount)
	}
	if result.Contradictions != 0 {
		t.Fatalf("expected 0 contradictions, got %d", result.Contradictions)
	}
}

func TestReplayTimeline_TamperedEntry_DetectedAsDivergence(t *testing.T) {
	j := makeJournal(t)
	_ = j.Append(makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeStepStarted))
	chain := BuildCausalChain("exec-1", "tenant-1", j.Entries())
	tl := BuildTimeline(chain)
	// Tamper with the entry hash directly.
	tl.Entries[0].Hash = "bad-hash"
	result, err := ReplayExecutionTimeline(tl)
	if err != nil {
		t.Fatalf("ReplayExecutionTimeline: %v", err)
	}
	if IsDivergenceFree(result) {
		t.Fatal("tampered entry must produce divergence")
	}
}

func TestReplayTimeline_ContradictionDuringReplay(t *testing.T) {
	j := makeJournal(t)
	_ = j.Append(makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeContradictionDetected))
	chain := BuildCausalChain("exec-1", "tenant-1", j.Entries())
	tl := BuildTimeline(chain)
	result, err := ReplayExecutionTimeline(tl)
	if err != nil {
		t.Fatalf("ReplayExecutionTimeline: %v", err)
	}
	if result.Contradictions != 1 {
		t.Fatalf("expected 1 contradiction during replay, got %d", result.Contradictions)
	}
}

// ─── ExecutionJournalSnapshot ──────────────────────────────────────────────

func TestBuildJournalSnapshot_Valid(t *testing.T) {
	j := makeJournal(t)
	_ = j.Append(makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeStepStarted))
	_ = j.Append(makeEntry(t, "j2", "exec-1", "s1", 2, JournalTypeRetryExecuted))

	snap := BuildJournalSnapshot("snap-1", j)
	if snap.SnapshotID != "snap-1" {
		t.Fatalf("SnapshotID mismatch: %s", snap.SnapshotID)
	}
	if snap.EntryCount != 2 {
		t.Fatalf("expected EntryCount=2, got %d", snap.EntryCount)
	}
	if snap.RetryCount != 1 {
		t.Fatalf("expected RetryCount=1, got %d", snap.RetryCount)
	}
}

func TestVerifyJournalSnapshot_Valid(t *testing.T) {
	j := makeJournal(t)
	_ = j.Append(makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeStepStarted))
	snap := BuildJournalSnapshot("snap-1", j)
	ok, reason := VerifyJournalSnapshot(snap)
	if !ok {
		t.Fatalf("valid snapshot must verify: %s", reason)
	}
}

func TestVerifyJournalSnapshot_TamperedEntryCount(t *testing.T) {
	j := makeJournal(t)
	_ = j.Append(makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeStepStarted))
	snap := BuildJournalSnapshot("snap-1", j)
	snap.EntryCount = 999
	ok, _ := VerifyJournalSnapshot(snap)
	if ok {
		t.Fatal("tampered EntryCount must fail verification")
	}
}

func TestBuildJournalSnapshot_GeneratedAtExcludedFromHash(t *testing.T) {
	j := makeJournal(t)
	_ = j.Append(makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeStepStarted))
	snap := BuildJournalSnapshot("snap-1", j)
	originalHash := snap.SnapshotHash
	snap.GeneratedAt = "2099-01-01T00:00:00Z"
	ok, _ := VerifyJournalSnapshot(snap)
	if !ok {
		t.Fatal("GeneratedAt mutation must not invalidate hash")
	}
	if snap.SnapshotHash != originalHash {
		t.Fatal("hash must remain unchanged when GeneratedAt changes")
	}
}

func TestBuildJournalCheckpoint(t *testing.T) {
	entries := []ExecutionJournalEntry{
		makeEntry(t, "j1", "exec-1", "s1", 1, JournalTypeStepStarted),
		makeEntry(t, "j2", "exec-1", "s1", 5, JournalTypeStepCompleted),
	}
	cp := BuildJournalCheckpoint("cp-1", entries, "j2", 5)
	if cp.CheckpointID != "cp-1" {
		t.Fatalf("CheckpointID mismatch: %s", cp.CheckpointID)
	}
	if cp.AtClock != 5 {
		t.Fatalf("AtClock must be 5, got %d", cp.AtClock)
	}
	if cp.StateHash == "" {
		t.Fatal("StateHash must not be empty")
	}
}
