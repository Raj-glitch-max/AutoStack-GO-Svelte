package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/janlauber/one-click/pkg/events"
)

// RecoveryResult describes the outcome of a durable state recovery run.
type RecoveryResult struct {
	RecoveredEventCount     int                 `json:"recovered_event_count"`
	DuplicatesSkipped       int                 `json:"duplicates_skipped"`
	CorruptionFindings      []CorruptionFinding `json:"corruption_findings"`
	IncidentEvents          []events.PlatformEvent `json:"incident_events"`
	SafeToContinue          bool                `json:"safe_to_continue"`
	WALEntriesProcessed     int                 `json:"wal_entries_processed"`
	RecoveredAt             string              `json:"recovered_at"`
}

// RecoverDurableRuntimeState replays all WAL event entries into the EventStore.
//
// Algorithm:
//  1. Verify each WAL entry hash; collect corruption findings for invalid entries.
//  2. For valid event entries, deserialize and append to the EventStore.
//  3. Duplicate events (already in store) are skipped — not an error.
//  4. Corrupt WAL entries and deserialization failures become CorruptionFindings.
//  5. Each CorruptionFinding is converted to a PlatformEvent incident.
//  6. SafeToContinue = true unless critical corruption was found.
//
// The caller is responsible for deciding whether to halt based on SafeToContinue.
func RecoverDurableRuntimeState(
	wal *WAL,
	store events.EventStore,
	tenantID string,
	streamID string,
	startClock int64,
) (RecoveryResult, error) {
	result := RecoveryResult{
		RecoveredAt: time.Now().UTC().Format(time.RFC3339Nano),
		SafeToContinue: true,
	}
	entries := wal.Entries()
	result.WALEntriesProcessed = len(entries)
	clock := startClock

	// Step 1: verify WAL integrity.
	badSeqs := wal.VerifyIntegrity()
	walFindings := CorruptionFindingsFromWAL(badSeqs)
	result.CorruptionFindings = append(result.CorruptionFindings, walFindings...)

	badSet := make(map[int64]struct{}, len(badSeqs))
	for _, seq := range badSeqs {
		badSet[seq] = struct{}{}
	}

	// Step 2: replay event entries.
	for _, entry := range entries {
		if entry.Type != WALEntryTypeEvent {
			continue
		}
		if _, isBad := badSet[entry.Sequence]; isBad {
			continue // already captured as corruption finding
		}
		evt, err := events.DeserializeEvent(entry.Body)
		if err != nil {
			f := NewCorruptionFinding(
				fmt.Sprintf("deser-%s", entry.ID),
				CorruptionTypeTruncatedPayload,
				entry.ID,
				fmt.Sprintf("WAL entry %d: deserialize failed: %v", entry.Sequence, err),
			)
			result.CorruptionFindings = append(result.CorruptionFindings, f)
			continue
		}
		ctx := context.Background()
		if err := store.Append(ctx, streamID, evt); err != nil {
			// Duplicate → skip silently.
			if isDuplicateError(err) {
				result.DuplicatesSkipped++
				continue
			}
			f := NewCorruptionFinding(
				fmt.Sprintf("append-%s", entry.ID),
				CorruptionTypeInvalidHash,
				entry.ID,
				fmt.Sprintf("replay append failed for event %q: %v", entry.ID, err),
			)
			result.CorruptionFindings = append(result.CorruptionFindings, f)
			continue
		}
		result.RecoveredEventCount++
	}

	// Step 3: emit incident events for each corruption finding.
	for i, f := range result.CorruptionFindings {
		clock++
		evt := CorruptionFindingAsEvent(f, tenantID,
			fmt.Sprintf("recovery-incident-%s-%d", f.FindingID, i), clock)
		result.IncidentEvents = append(result.IncidentEvents, evt)
	}

	// Step 4: determine safety.
	for _, f := range result.CorruptionFindings {
		switch f.Type {
		case CorruptionTypeInvalidHash, CorruptionTypeClockRegression,
			CorruptionTypeWALEntryHashInvalid, CorruptionTypeReplayDivergence:
			result.SafeToContinue = false
		}
	}
	return result, nil
}

// isDuplicateError reports whether an EventStore.Append error is a duplicate-ID rejection.
// This is a heuristic — EventStore implementations are expected to produce errors
// containing "duplicate" for duplicate-ID rejections.
func isDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for i := 0; i+8 < len(msg); i++ {
		if msg[i:i+9] == "duplicate" {
			return true
		}
	}
	return false
}
