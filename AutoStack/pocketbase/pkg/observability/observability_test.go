package observability

import (
	"testing"
)

// ─── ComputeMetrics ───────────────────────────────────────────────────────────

func TestComputeMetrics_BasicCounts(t *testing.T) {
	m := ComputeMetrics("tenant-1", "exec-1",
		10, 8, 1, 1, 0,
		2, 1, 1, 0,
		0.75,
		50, 3, 2,
	)
	if m.TotalTasks != 10 {
		t.Fatalf("expected total 10, got %d", m.TotalTasks)
	}
	if m.CompletedTasks != 8 {
		t.Fatalf("expected completed 8, got %d", m.CompletedTasks)
	}
	if m.ConfidenceScore != 0.75 {
		t.Fatalf("expected confidence 0.75, got %f", m.ConfidenceScore)
	}
}

// ─── AssessHealth ─────────────────────────────────────────────────────────────

func TestAssessHealth_Healthy(t *testing.T) {
	m := ComputeMetrics("t1", "e1", 5, 5, 0, 0, 0, 0, 0, 0, 0, 0.90, 10, 2, 2)
	a := AssessHealth(m)
	if a.Status != HealthStatusHealthy {
		t.Fatalf("expected healthy, got %s", a.Status)
	}
}

func TestAssessHealth_DegradedByConfidence(t *testing.T) {
	m := ComputeMetrics("t1", "e1", 5, 3, 1, 0, 0, 0, 0, 0, 0, 0.40, 10, 2, 2)
	a := AssessHealth(m)
	if a.Status != HealthStatusDegraded {
		t.Fatalf("expected degraded, got %s", a.Status)
	}
}

func TestAssessHealth_CriticalByContradictions(t *testing.T) {
	m := ComputeMetrics("t1", "e1", 5, 3, 1, 1, 0, 0, 0, 1, 0, 0.85, 10, 2, 2)
	a := AssessHealth(m)
	if a.Status != HealthStatusCritical {
		t.Fatalf("expected critical, got %s", a.Status)
	}
	if !a.HasContradictions {
		t.Fatal("HasContradictions must be true")
	}
}

func TestAssessHealth_CriticalByLowConfidence(t *testing.T) {
	m := ComputeMetrics("t1", "e1", 5, 1, 4, 0, 0, 0, 0, 0, 0, 0.10, 0, 0, 0)
	a := AssessHealth(m)
	if a.Status != HealthStatusCritical {
		t.Fatalf("expected critical, got %s", a.Status)
	}
}

func TestAssessHealth_DegradedByDivergences(t *testing.T) {
	m := ComputeMetrics("t1", "e1", 5, 5, 0, 0, 0, 0, 0, 0, 3, 0.80, 0, 0, 0)
	a := AssessHealth(m)
	if a.Status != HealthStatusDegraded {
		t.Fatalf("expected degraded for divergences, got %s", a.Status)
	}
	if !a.HasDivergences {
		t.Fatal("HasDivergences must be true")
	}
}

// ─── ObservabilityTimeline ────────────────────────────────────────────────────

func TestStreamExecutionTimeline_SortsByLogicalClock(t *testing.T) {
	entries := []TimelineEntry{
		{Sequence: 3, LogicalClock: 30, Category: "task", Summary: "third"},
		{Sequence: 1, LogicalClock: 10, Category: "task", Summary: "first"},
		{Sequence: 2, LogicalClock: 20, Category: "task", Summary: "second"},
	}
	tl := StreamExecutionTimeline("exec-1", "tenant-1", entries)
	if tl.Entries[0].Summary != "first" {
		t.Fatalf("first entry must be earliest, got %s", tl.Entries[0].Summary)
	}
	if tl.Entries[2].Summary != "third" {
		t.Fatalf("last entry must be latest, got %s", tl.Entries[2].Summary)
	}
}

func TestStreamExecutionTimeline_TiebreakBySequence(t *testing.T) {
	entries := []TimelineEntry{
		{Sequence: 2, LogicalClock: 10, Summary: "second"},
		{Sequence: 1, LogicalClock: 10, Summary: "first"},
	}
	tl := StreamExecutionTimeline("exec-1", "t1", entries)
	if tl.Entries[0].Summary != "first" {
		t.Fatalf("sequence tiebreak failed: got %s", tl.Entries[0].Summary)
	}
}

func TestObservabilityTimeline_EntriesAfterClock(t *testing.T) {
	entries := []TimelineEntry{
		{Sequence: 1, LogicalClock: 5, Summary: "a"},
		{Sequence: 2, LogicalClock: 10, Summary: "b"},
		{Sequence: 3, LogicalClock: 15, Summary: "c"},
	}
	tl := StreamExecutionTimeline("exec-1", "t1", entries)
	after := tl.EntriesAfterClock(7)
	if len(after) != 2 {
		t.Fatalf("expected 2 entries after clock 7, got %d", len(after))
	}
}

// ─── ContradictionSummary ─────────────────────────────────────────────────────

func TestSummarizeContradictions_CriticalImpact(t *testing.T) {
	inputs := []ContradictionInput{
		{Type: "deploy_success_resource_missing", Resolved: false, ConfidenceImpact: 0.40},
	}
	summary := SummarizeContradictions("exec-1", "t1", inputs)
	if summary.TotalCount != 1 {
		t.Fatalf("expected total 1, got %d", summary.TotalCount)
	}
	if summary.HighestSeverity != ContradictionSeverityCritical {
		t.Fatalf("expected critical, got %s", summary.HighestSeverity)
	}
	if summary.UnresolvedCount != 1 {
		t.Fatalf("expected 1 unresolved, got %d", summary.UnresolvedCount)
	}
}

func TestSummarizeContradictions_Resolved(t *testing.T) {
	inputs := []ContradictionInput{
		{Type: "rollback_success_unhealthy", Resolved: true, ConfidenceImpact: 0.30},
	}
	summary := SummarizeContradictions("exec-1", "t1", inputs)
	if summary.ResolvedCount != 1 {
		t.Fatalf("expected resolved 1, got %d", summary.ResolvedCount)
	}
}

// ─── ObservabilityExport ──────────────────────────────────────────────────────

func TestBuildObservabilityExport_HashValid(t *testing.T) {
	m := ComputeMetrics("t1", "e1", 5, 5, 0, 0, 0, 0, 0, 0, 0, 0.90, 10, 2, 2)
	h := AssessHealth(m)
	tl := StreamExecutionTimeline("e1", "t1", nil)
	cs := SummarizeContradictions("e1", "t1", nil)
	exp := BuildObservabilityExport("export-1", "e1", "t1", m, h, tl, cs)
	ok, reason := VerifyObservabilityExport(exp)
	if !ok {
		t.Fatalf("export hash invalid: %s", reason)
	}
}

func TestBuildObservabilityExport_ExportedAtExcluded(t *testing.T) {
	m := ComputeMetrics("t1", "e1", 0, 0, 0, 0, 0, 0, 0, 0, 0, 1.0, 0, 0, 0)
	h := AssessHealth(m)
	tl := StreamExecutionTimeline("e1", "t1", nil)
	cs := SummarizeContradictions("e1", "t1", nil)
	exp1 := BuildObservabilityExport("export-1", "e1", "t1", m, h, tl, cs)
	exp2 := exp1
	exp2.ExportedAt = "2099-01-01T00:00:00Z"
	if exp1.ExportHash != exp2.ExportHash {
		t.Fatal("ExportedAt must not affect export hash")
	}
}
