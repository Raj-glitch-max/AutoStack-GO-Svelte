package executor

import (
	"testing"
)

func TestExportRuntimeSnapshot_NonNil(t *testing.T) {
	snap := newCreateSnapshot(t)
	rs := ExportRuntimeSnapshot("exec-1", snap, StateReady, 0, nil, nil, nil, nil, nil, nil)
	if rs == nil {
		t.Fatal("ExportRuntimeSnapshot must return non-nil")
	}
}

func TestExportRuntimeSnapshot_SchemaVersion(t *testing.T) {
	rs := ExportRuntimeSnapshot("exec-1", nil, StatePending, 0, nil, nil, nil, nil, nil, nil)
	if rs.SchemaVersion != ExecutionRuntimeSnapshotSchemaVersion {
		t.Fatalf("expected schema version %q, got %q", ExecutionRuntimeSnapshotSchemaVersion, rs.SchemaVersion)
	}
}

func TestExportRuntimeSnapshot_ExportHashNonEmpty(t *testing.T) {
	snap := newCreateSnapshot(t)
	rs := ExportRuntimeSnapshot("exec-1", snap, StateReady, 0, nil, nil, nil, nil, nil, nil)
	if rs.ExportHash == "" {
		t.Fatal("ExportHash must be non-empty")
	}
	if len(rs.ExportHash) != 64 {
		t.Fatalf("expected 64-char SHA-256 hex, got %d chars", len(rs.ExportHash))
	}
}

func TestExportRuntimeSnapshot_ExecutionPlanHash_Mirrored(t *testing.T) {
	snap := newCreateSnapshot(t)
	rs := ExportRuntimeSnapshot("exec-1", snap, StateReady, 0, nil, nil, nil, nil, nil, nil)
	if rs.ExecutionPlanHash != snap.ExecutionPlanHash {
		t.Fatalf("ExecutionPlanHash must mirror snapshot: want %q, got %q",
			snap.ExecutionPlanHash, rs.ExecutionPlanHash)
	}
}

func TestExportRuntimeSnapshot_LimitationsNonEmpty(t *testing.T) {
	rs := ExportRuntimeSnapshot("exec-1", nil, StatePending, 0, nil, nil, nil, nil, nil, nil)
	if len(rs.Limitations) == 0 {
		t.Fatal("ExecutionRuntimeSnapshot must document limitations")
	}
}

func TestVerifyRuntimeSnapshot_Valid(t *testing.T) {
	snap := newCreateSnapshot(t)
	rs := ExportRuntimeSnapshot("exec-1", snap, StateReady, 1,
		[]ExecutionTransition{{ExecutionID: "exec-1", FromState: StatePending, ToState: StateReady}},
		nil, nil, nil, nil, nil,
	)
	ok, reason := VerifyExecutionRuntimeSnapshot(rs)
	if !ok {
		t.Fatalf("VerifyExecutionRuntimeSnapshot must return true for valid snapshot: %s", reason)
	}
}

func TestVerifyRuntimeSnapshot_Nil_ReturnsFalse(t *testing.T) {
	ok, reason := VerifyExecutionRuntimeSnapshot(nil)
	if ok {
		t.Fatal("nil snapshot must return false")
	}
	if reason == "" {
		t.Fatal("reason must be non-empty on failure")
	}
}

func TestVerifyRuntimeSnapshot_EmptyHash_ReturnsFalse(t *testing.T) {
	rs := &ExecutionRuntimeSnapshot{ExportHash: ""}
	ok, reason := VerifyExecutionRuntimeSnapshot(rs)
	if ok {
		t.Fatal("empty ExportHash must return false")
	}
	if reason == "" {
		t.Fatal("reason must be non-empty")
	}
}

func TestVerifyRuntimeSnapshot_TamperedState_Detected(t *testing.T) {
	snap := newCreateSnapshot(t)
	rs := ExportRuntimeSnapshot("exec-1", snap, StateReady, 0, nil, nil, nil, nil, nil, nil)
	rs.State = StateFailed // tamper
	ok, _ := VerifyExecutionRuntimeSnapshot(rs)
	if ok {
		t.Fatal("tampered State must cause hash verification to fail")
	}
}

func TestVerifyRuntimeSnapshot_TamperedExecutionID_Detected(t *testing.T) {
	snap := newCreateSnapshot(t)
	rs := ExportRuntimeSnapshot("exec-1", snap, StateReady, 0, nil, nil, nil, nil, nil, nil)
	rs.ExecutionID = "tampered"
	ok, _ := VerifyExecutionRuntimeSnapshot(rs)
	if ok {
		t.Fatal("tampered ExecutionID must cause hash verification to fail")
	}
}

func TestExportRuntimeSnapshot_GeneratedAtExcluded(t *testing.T) {
	snap := newCreateSnapshot(t)
	rs1 := ExportRuntimeSnapshot("exec-1", snap, StateReady, 0, nil, nil, nil, nil, nil, nil)
	rs2 := ExportRuntimeSnapshot("exec-1", snap, StateReady, 0, nil, nil, nil, nil, nil, nil)

	// Override GeneratedAt to simulate different timestamps.
	rs1.GeneratedAt = "2024-01-01T00:00:00Z"
	rs2.GeneratedAt = "2025-06-01T12:00:00Z"

	ok1, _ := VerifyExecutionRuntimeSnapshot(rs1)
	ok2, _ := VerifyExecutionRuntimeSnapshot(rs2)
	if !ok1 || !ok2 {
		t.Fatal("GeneratedAt change must not invalidate ExportHash")
	}
}

func TestExportRuntimeSnapshot_Deterministic(t *testing.T) {
	snap := newCreateSnapshot(t)
	transitions := []ExecutionTransition{
		{ExecutionID: "exec-1", FromState: StatePending, ToState: StateReady},
	}
	rs1 := ExportRuntimeSnapshot("exec-1", snap, StateReady, 1, transitions, nil, nil, nil, nil, nil)
	rs2 := ExportRuntimeSnapshot("exec-1", snap, StateReady, 1, transitions, nil, nil, nil, nil, nil)
	if rs1.ExportHash != rs2.ExportHash {
		t.Fatalf("ExportRuntimeSnapshot must be deterministic: %q vs %q", rs1.ExportHash, rs2.ExportHash)
	}
}

func TestExportRuntimeSnapshot_NilSnap_StillProducesHash(t *testing.T) {
	rs := ExportRuntimeSnapshot("exec-1", nil, StatePending, 0, nil, nil, nil, nil, nil, nil)
	if rs.ExportHash == "" {
		t.Fatal("ExportHash must be set even with nil snapshot")
	}
}

func TestExportRuntimeSnapshot_RollbackRecs_Included(t *testing.T) {
	snap := newCreateSnapshot(t)
	recs := []RollbackRecommendation{
		{RecommendationID: "rec-1", ExecutionID: "exec-1", Reason: "stage failed"},
	}
	rs := ExportRuntimeSnapshot("exec-1", snap, StateRollbackRequired, 0, nil, nil, nil, nil, recs, nil)
	if len(rs.RollbackRecommendations) != 1 {
		t.Fatal("rollback recommendations must be included in runtime snapshot")
	}
}
