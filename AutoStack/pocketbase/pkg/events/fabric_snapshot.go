package events

import (
	"crypto/sha256"
	"fmt"
	"time"
)

const FabricSnapshotSchemaVersion = "5.0.0"

// EventCheckpoint marks a stable position in an event stream for replay optimization.
// A checkpoint's StateHash is SHA-256 over all event hashes up to that point.
type EventCheckpoint struct {
	CheckpointID string `json:"checkpoint_id"`
	StreamID     string `json:"stream_id"`
	ExecutionID  string `json:"execution_id"`
	TenantID     string `json:"tenant_id"`
	AtClock      int64  `json:"at_clock"`
	AtTimestamp  string `json:"at_timestamp"`
	StateHash    string `json:"state_hash"`
	CreatedAt    string `json:"created_at"`
}

// ArchiveRef is a lightweight tamper-evident reference to an archived payload.
type ArchiveRef struct {
	RefID       string `json:"ref_id"`
	ArchiveType string `json:"archive_type"`
	ExecutionID string `json:"execution_id"`
	TenantID    string `json:"tenant_id"`
	PayloadHash string `json:"payload_hash"`
	ArchivedAt  string `json:"archived_at"`
}

// EventFabricSnapshot is a tamper-evident export of the event fabric state for
// an execution. It bundles the ordered event log, replay checkpoints, and
// archive references into a single integrity-verified payload.
//
// GeneratedAt and IntegrityHash are excluded from the hash input for
// content-identity semantics: two exports of the same state produce
// the same IntegrityHash regardless of when they were generated.
type EventFabricSnapshot struct {
	SchemaVersion     string            `json:"schema_version"`
	TenantID          string            `json:"tenant_id"`
	ExecutionID       string            `json:"execution_id"`
	Events            []PlatformEvent   `json:"events"`
	ReplayCheckpoints []EventCheckpoint `json:"replay_checkpoints"`
	ArchiveRefs       []ArchiveRef      `json:"archive_refs"`
	IntegrityHash     string            `json:"integrity_hash"`
	GeneratedAt       string            `json:"generated_at"`
}

// ExportFabricSnapshot builds and hashes an EventFabricSnapshot.
// Events are sorted in canonical order before hashing.
func ExportFabricSnapshot(
	tenantID string,
	executionID string,
	evts []PlatformEvent,
	checkpoints []EventCheckpoint,
	archiveRefs []ArchiveRef,
) *EventFabricSnapshot {
	snap := &EventFabricSnapshot{
		SchemaVersion:     FabricSnapshotSchemaVersion,
		TenantID:          tenantID,
		ExecutionID:       executionID,
		Events:            SortEvents(evts),
		ReplayCheckpoints: append([]EventCheckpoint(nil), checkpoints...),
		ArchiveRefs:       append([]ArchiveRef(nil), archiveRefs...),
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	snap.IntegrityHash = computeFabricHash(snap)
	return snap
}

// VerifyFabricSnapshot recomputes IntegrityHash and checks it matches.
func VerifyFabricSnapshot(snap *EventFabricSnapshot) (bool, string) {
	if snap == nil {
		return false, "snapshot is nil"
	}
	if snap.IntegrityHash == "" {
		return false, "IntegrityHash is empty"
	}
	expected := computeFabricHash(snap)
	if snap.IntegrityHash != expected {
		return false, fmt.Sprintf("IntegrityHash mismatch: expected %q got %q", expected, snap.IntegrityHash)
	}
	return true, ""
}

// NewEventCheckpoint builds a checkpoint at the tail of a sorted event slice.
func NewEventCheckpoint(checkpointID, streamID, executionID, tenantID string, evts []PlatformEvent) EventCheckpoint {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var atClock int64
	var atTimestamp string
	if len(evts) > 0 {
		last := evts[len(evts)-1]
		atClock = last.LogicalClock
		atTimestamp = last.Timestamp
	}
	return EventCheckpoint{
		CheckpointID: checkpointID,
		StreamID:     streamID,
		ExecutionID:  executionID,
		TenantID:     tenantID,
		AtClock:      atClock,
		AtTimestamp:  atTimestamp,
		StateHash:    computeCheckpointStateHash(evts),
		CreatedAt:    now,
	}
}

// VerifyCheckpointStateHash recomputes the checkpoint's StateHash from the
// provided event slice and checks it matches.
func VerifyCheckpointStateHash(checkpoint EventCheckpoint, evts []PlatformEvent) (bool, string) {
	expected := computeCheckpointStateHash(evts)
	if checkpoint.StateHash != expected {
		return false, fmt.Sprintf("checkpoint %q StateHash mismatch: expected %q got %q",
			checkpoint.CheckpointID, expected, checkpoint.StateHash)
	}
	return true, ""
}

func computeFabricHash(snap *EventFabricSnapshot) string {
	h := sha256.New()
	fmt.Fprintf(h, "schema:%s|tenant:%s|exec:%s|", snap.SchemaVersion, snap.TenantID, snap.ExecutionID)
	for _, e := range snap.Events {
		fmt.Fprintf(h, "event:%s|", e.Hash)
	}
	for _, cp := range snap.ReplayCheckpoints {
		fmt.Fprintf(h, "checkpoint:%s:%d:%s|", cp.CheckpointID, cp.AtClock, cp.StateHash)
	}
	for _, ar := range snap.ArchiveRefs {
		fmt.Fprintf(h, "archive:%s:%s|", ar.RefID, ar.PayloadHash)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func computeCheckpointStateHash(evts []PlatformEvent) string {
	if len(evts) == 0 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte("")))
	}
	h := sha256.New()
	for _, e := range evts {
		fmt.Fprintf(h, "%s|", e.Hash)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
