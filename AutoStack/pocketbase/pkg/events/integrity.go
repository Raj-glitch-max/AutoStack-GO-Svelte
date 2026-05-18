package events

import "fmt"

// VerifyEventHash recomputes the event hash and checks it matches the stored Hash.
func VerifyEventHash(event PlatformEvent) (bool, string) {
	if event.Hash == "" {
		return false, fmt.Sprintf("event %q has empty hash", event.EventID)
	}
	expected := ComputeEventHash(event)
	if event.Hash != expected {
		return false, fmt.Sprintf("event %q hash mismatch: stored %q expected %q",
			event.EventID, event.Hash, expected)
	}
	return true, ""
}

// VerifyStreamIntegrity checks every event's hash and the overall StreamHash.
// Returns (true, "") if intact, (false, reason) if tampered.
func VerifyStreamIntegrity(stream EventStream) (bool, string) {
	for _, e := range stream.Events {
		ok, reason := VerifyEventHash(e)
		if !ok {
			return false, reason
		}
	}
	expected := computeStreamHash(stream.Events)
	if stream.StreamHash != expected {
		return false, fmt.Sprintf("stream %q StreamHash mismatch: expected %q got %q",
			stream.StreamID, expected, stream.StreamHash)
	}
	return true, ""
}

// VerifyEventSequence checks that a slice of events is totally ordered and
// each event's hash is valid. Returns (true, "") or (false, reason).
func VerifyEventSequence(evts []PlatformEvent) (bool, string) {
	for _, e := range evts {
		ok, reason := VerifyEventHash(e)
		if !ok {
			return false, reason
		}
	}
	if !IsTotallyOrdered(evts) {
		return false, "event sequence is not in canonical order"
	}
	return true, ""
}
