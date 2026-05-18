package verifier

import (
	"testing"
)

func makeExportResult() VerificationResult {
	return VerificationResult{
		ExecutionID:      "exec-export",
		StageID:          "stage-0",
		NodeID:           "svc-a",
		State:            VerificationVerified,
		StabilizationMet: true,
		Confidence: ConfidenceScore{
			ExecutionConfidence:    1.0,
			ProviderConfidence:     1.0,
			VerificationConfidence: 1.0,
		},
		Recommendation: "execution verified",
		Limitations:    []string{"test limitation"},
		VerifiedAt:     "2024-01-01T12:00:00Z",
	}
}

func TestExport_SnapshotNonNil(t *testing.T) {
	snap := ExportVerificationSnapshot(
		makeExportResult(),
		NewObservationTimeline("exec-export", "stage-0", "svc-a"),
		DefaultStabilizationWindow,
		DefaultConsistencyPolicy,
	)
	if snap == nil {
		t.Fatal("ExportVerificationSnapshot must return a non-nil snapshot")
	}
}

func TestExport_SchemaVersion(t *testing.T) {
	snap := ExportVerificationSnapshot(
		makeExportResult(),
		NewObservationTimeline("exec-export", "stage-0", "svc-a"),
		DefaultStabilizationWindow,
		DefaultConsistencyPolicy,
	)
	if snap.SchemaVersion != VerificationSnapshotSchemaVersion {
		t.Fatalf("SchemaVersion must be %q, got %q", VerificationSnapshotSchemaVersion, snap.SchemaVersion)
	}
}

func TestExport_HashIs64HexChars(t *testing.T) {
	snap := ExportVerificationSnapshot(
		makeExportResult(),
		NewObservationTimeline("exec-export", "stage-0", "svc-a"),
		DefaultStabilizationWindow,
		DefaultConsistencyPolicy,
	)
	if snap.ExportHash == "" {
		t.Fatal("ExportHash must be non-empty")
	}
	if len(snap.ExportHash) != 64 {
		t.Fatalf("ExportHash must be 64 hex characters (SHA-256), got len=%d: %q", len(snap.ExportHash), snap.ExportHash)
	}
}

func TestExport_GeneratedAtExcludedFromHash(t *testing.T) {
	result := makeExportResult()
	tl := NewObservationTimeline("exec-export", "stage-0", "svc-a")
	snap1 := ExportVerificationSnapshot(result, tl, DefaultStabilizationWindow, DefaultConsistencyPolicy)
	snap2 := ExportVerificationSnapshot(result, tl, DefaultStabilizationWindow, DefaultConsistencyPolicy)

	if snap1.ExportHash != snap2.ExportHash {
		t.Fatalf("ExportHash must be identical across two exports of the same result; got %q vs %q",
			snap1.ExportHash, snap2.ExportHash)
	}
}

func TestExport_GeneratedAtSet(t *testing.T) {
	snap := ExportVerificationSnapshot(
		makeExportResult(),
		NewObservationTimeline("exec-export", "stage-0", "svc-a"),
		DefaultStabilizationWindow,
		DefaultConsistencyPolicy,
	)
	if snap.GeneratedAt == "" {
		t.Fatal("GeneratedAt must be set")
	}
}

func TestExport_FieldsPreserved(t *testing.T) {
	result := makeExportResult()
	tl := NewObservationTimeline("exec-export", "stage-0", "svc-a")
	snap := ExportVerificationSnapshot(result, tl, DefaultStabilizationWindow, DefaultConsistencyPolicy)

	if snap.ExecutionID != result.ExecutionID {
		t.Fatalf("ExecutionID mismatch: %q vs %q", snap.ExecutionID, result.ExecutionID)
	}
	if snap.State != result.State {
		t.Fatalf("State mismatch: %q vs %q", snap.State, result.State)
	}
	if snap.StabilizationMet != result.StabilizationMet {
		t.Fatalf("StabilizationMet mismatch: %v vs %v", snap.StabilizationMet, result.StabilizationMet)
	}
	if snap.ObservationCount != len(tl.Observations) {
		t.Fatalf("ObservationCount must match timeline length: %d vs %d", snap.ObservationCount, len(tl.Observations))
	}
}

func TestExport_VerifySnapshot_Valid(t *testing.T) {
	snap := ExportVerificationSnapshot(
		makeExportResult(),
		NewObservationTimeline("exec-export", "stage-0", "svc-a"),
		DefaultStabilizationWindow,
		DefaultConsistencyPolicy,
	)
	ok, reason := VerifyVerificationSnapshot(snap)
	if !ok {
		t.Fatalf("original snapshot must verify successfully, got reason: %s", reason)
	}
}

func TestExport_TamperDetection_State(t *testing.T) {
	snap := ExportVerificationSnapshot(
		makeExportResult(),
		NewObservationTimeline("exec-export", "stage-0", "svc-a"),
		DefaultStabilizationWindow,
		DefaultConsistencyPolicy,
	)
	snap.State = VerificationUnverifiable
	ok, _ := VerifyVerificationSnapshot(snap)
	if ok {
		t.Fatal("tampered State must cause VerifyVerificationSnapshot to return false")
	}
}

func TestExport_TamperDetection_ExecutionID(t *testing.T) {
	snap := ExportVerificationSnapshot(
		makeExportResult(),
		NewObservationTimeline("exec-export", "stage-0", "svc-a"),
		DefaultStabilizationWindow,
		DefaultConsistencyPolicy,
	)
	snap.ExecutionID = "tampered"
	ok, _ := VerifyVerificationSnapshot(snap)
	if ok {
		t.Fatal("tampered ExecutionID must cause VerifyVerificationSnapshot to return false")
	}
}

func TestExport_TamperDetection_Confidence(t *testing.T) {
	snap := ExportVerificationSnapshot(
		makeExportResult(),
		NewObservationTimeline("exec-export", "stage-0", "svc-a"),
		DefaultStabilizationWindow,
		DefaultConsistencyPolicy,
	)
	snap.Confidence.ExecutionConfidence = 0.0
	ok, _ := VerifyVerificationSnapshot(snap)
	if ok {
		t.Fatal("tampered Confidence must cause VerifyVerificationSnapshot to return false")
	}
}

func TestExport_VerifySnapshot_Nil(t *testing.T) {
	ok, reason := VerifyVerificationSnapshot(nil)
	if ok {
		t.Fatal("nil snapshot must return false")
	}
	if reason == "" {
		t.Fatal("reason must be non-empty for nil snapshot")
	}
}

func TestExport_VerifySnapshot_EmptyHash(t *testing.T) {
	snap := ExportVerificationSnapshot(
		makeExportResult(),
		NewObservationTimeline("exec-export", "stage-0", "svc-a"),
		DefaultStabilizationWindow,
		DefaultConsistencyPolicy,
	)
	snap.ExportHash = ""
	ok, reason := VerifyVerificationSnapshot(snap)
	if ok {
		t.Fatal("empty ExportHash must return false")
	}
	if reason == "" {
		t.Fatal("reason must be non-empty for empty ExportHash")
	}
}
