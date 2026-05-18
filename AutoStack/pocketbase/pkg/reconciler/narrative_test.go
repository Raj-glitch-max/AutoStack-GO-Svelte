package reconciler

import (
	"testing"
)

// Phase 3.9.6 — Narrative merge pure-logic tests.
//
// Verifies the deterministic merge+sort algorithm without a DB.

func TestMergeNarrativeEmpty(t *testing.T) {
	out := mergeNarrativeEntries(nil, nil, nil, nil)
	if len(out) != 0 {
		t.Errorf("merge of empties = %d entries, want 0", len(out))
	}
}

func TestMergeNarrativeSortOrder(t *testing.T) {
	// Build out-of-order entries; merge must sort by timestamp asc.
	decisions := []NarrativeEntry{
		{Timestamp: "2026-05-15T12:00:30Z", Source: "decision", SourceID: "d1", EventType: "retry_scheduled"},
		{Timestamp: "2026-05-15T12:00:10Z", Source: "decision", SourceID: "d2", EventType: "cooldown_active"},
	}
	ops := []NarrativeEntry{
		{Timestamp: "2026-05-15T12:00:20Z", Source: "operation", SourceID: "o1", EventType: "deploy"},
	}
	history := []NarrativeEntry{
		{Timestamp: "2026-05-15T12:00:00Z", Source: "history", SourceID: "h1", EventType: "created"},
	}
	incidents := []NarrativeEntry{
		{Timestamp: "2026-05-15T12:00:40Z", Source: "incident", SourceID: "i1", EventType: "incident_opened"},
	}

	merged := mergeNarrativeEntries(decisions, ops, history, incidents)
	if len(merged) != 5 {
		t.Fatalf("expected 5 entries; got %d", len(merged))
	}
	// Verify ascending timestamp order.
	for i := 1; i < len(merged); i++ {
		if merged[i].Timestamp < merged[i-1].Timestamp {
			t.Errorf("non-monotonic at idx=%d: prev=%s curr=%s",
				i, merged[i-1].Timestamp, merged[i].Timestamp)
		}
	}
	// Verify the earliest entry is "created" history.
	if merged[0].SourceID != "h1" {
		t.Errorf("first entry source_id = %s, want h1", merged[0].SourceID)
	}
}

func TestMergeNarrativeTieBreaker(t *testing.T) {
	// All entries share the same timestamp — source-priority dictates order.
	// Priority order: decision (0) < history (1) < operation (2) < incident (3).
	ts := "2026-05-15T12:00:00Z"
	merged := mergeNarrativeEntries(
		[]NarrativeEntry{{Timestamp: ts, Source: "decision", SourceID: "d", EventType: "retry_scheduled"}},
		[]NarrativeEntry{{Timestamp: ts, Source: "operation", SourceID: "o", EventType: "deploy"}},
		[]NarrativeEntry{{Timestamp: ts, Source: "history", SourceID: "h", EventType: "created"}},
		[]NarrativeEntry{{Timestamp: ts, Source: "incident", SourceID: "i", EventType: "incident_opened"}},
	)
	wantOrder := []string{"decision", "history", "operation", "incident"}
	for idx, want := range wantOrder {
		if merged[idx].Source != want {
			t.Errorf("tie-break idx=%d: got source=%s, want %s", idx, merged[idx].Source, want)
		}
	}
}

func TestMergeNarrativeDeterminismOnSourceID(t *testing.T) {
	// Same timestamp + same source — must break ties by SourceID lexicographic.
	ts := "2026-05-15T12:00:00Z"
	merged := mergeNarrativeEntries(
		[]NarrativeEntry{
			{Timestamp: ts, Source: "decision", SourceID: "z"},
			{Timestamp: ts, Source: "decision", SourceID: "a"},
			{Timestamp: ts, Source: "decision", SourceID: "m"},
		},
		nil, nil, nil,
	)
	wantOrder := []string{"a", "m", "z"}
	for idx, want := range wantOrder {
		if merged[idx].SourceID != want {
			t.Errorf("source-id tie-break idx=%d: got %s, want %s", idx, merged[idx].SourceID, want)
		}
	}
}

func TestSourcePriorityCompleteness(t *testing.T) {
	// Every source kind we emit must have a priority entry.
	requiredSources := []string{"decision", "history", "operation", "incident"}
	for _, s := range requiredSources {
		if _, ok := sourcePriority[s]; !ok {
			t.Errorf("sourcePriority missing entry for %q", s)
		}
	}
	// Decision MUST come first in ties (it records reasoning before action).
	if sourcePriority["decision"] >= sourcePriority["history"] {
		t.Error("decision priority must be lower than history priority (decisions ordered first in ties)")
	}
}

func TestNarrativeQueryLimitReasonable(t *testing.T) {
	// Sanity floor + ceiling. Too small caps narrative excessively; too large risks DB pressure.
	if narrativeQueryLimit < 100 {
		t.Errorf("narrativeQueryLimit=%d; too low for useful narratives", narrativeQueryLimit)
	}
	if narrativeQueryLimit > 10000 {
		t.Errorf("narrativeQueryLimit=%d; too high — DB pressure risk", narrativeQueryLimit)
	}
}
