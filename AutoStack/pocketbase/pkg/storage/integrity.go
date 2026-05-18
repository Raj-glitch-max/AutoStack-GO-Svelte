package storage

import (
	"github.com/janlauber/one-click/pkg/events"
)

// ClockRegression records a position where the logical clock went backwards.
type ClockRegression struct {
	EventID      string `json:"event_id"`
	Clock        int64  `json:"clock"`
	PreviousClock int64 `json:"previous_clock"`
}

// StorageIntegrityReport summarises the integrity of one event stream in storage.
type StorageIntegrityReport struct {
	StreamID         string            `json:"stream_id"`
	EventCount       int               `json:"event_count"`
	InvalidHashes    []string          `json:"invalid_hashes,omitempty"`
	ClockRegressions []ClockRegression `json:"clock_regressions,omitempty"`
	Status           string            `json:"status"` // "ok" | "corrupted"
}

// VerifyEventStorageIntegrity checks hash validity and clock monotonicity for
// a list of records from one stream. The records must be pre-sorted by LogicalClock.
func VerifyEventStorageIntegrity(streamID string, records []events.EventStorageRecord) StorageIntegrityReport {
	report := StorageIntegrityReport{
		StreamID:   streamID,
		EventCount: len(records),
		Status:     "ok",
	}
	var lastClock int64 = -1
	for _, r := range records {
		// Deserialize to verify the hash.
		evt, err := events.DeserializeEvent(r.Body)
		if err != nil {
			report.InvalidHashes = append(report.InvalidHashes, r.EventID)
			report.Status = "corrupted"
			lastClock = r.LogicalClock
			continue
		}
		expected := events.ComputeEventHash(evt)
		if evt.Hash != expected {
			report.InvalidHashes = append(report.InvalidHashes, r.EventID)
			report.Status = "corrupted"
		}
		if lastClock >= 0 && r.LogicalClock < lastClock {
			report.ClockRegressions = append(report.ClockRegressions, ClockRegression{
				EventID:       r.EventID,
				Clock:         r.LogicalClock,
				PreviousClock: lastClock,
			})
			report.Status = "corrupted"
		}
		lastClock = r.LogicalClock
	}
	return report
}

// VerifyWALCheckpoint checks whether a WAL checkpoint's StateHash matches the
// entry hashes for all WAL entries up to (and including) cp.LastSequence.
func VerifyWALCheckpoint(cp WALCheckpoint, entries []WALEntry) (bool, string) {
	var hashes []string
	for _, e := range entries {
		if e.Sequence > cp.LastSequence {
			break
		}
		hashes = append(hashes, e.EntryHash)
	}
	expected := computeStateHash(hashes)
	if cp.StateHash != expected {
		return false, "checkpoint state hash mismatch"
	}
	return true, ""
}
