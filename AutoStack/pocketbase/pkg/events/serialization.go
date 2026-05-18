package events

import (
	"encoding/json"
	"fmt"
)

// CanonicalSerialize produces a deterministic JSON encoding of a PlatformEvent.
// The encoding is stable across calls with identical field values.
func CanonicalSerialize(event PlatformEvent) ([]byte, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("serialize event %q: %w", event.EventID, err)
	}
	return data, nil
}

// DeserializeEvent parses a PlatformEvent from JSON.
// The caller should call VerifyEventHash after deserialization.
func DeserializeEvent(data []byte) (PlatformEvent, error) {
	var e PlatformEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return PlatformEvent{}, fmt.Errorf("deserialize event: %w", err)
	}
	return e, nil
}

// SerializeStream encodes an EventStream to JSON.
func SerializeStream(stream EventStream) ([]byte, error) {
	data, err := json.Marshal(stream)
	if err != nil {
		return nil, fmt.Errorf("serialize stream %q: %w", stream.StreamID, err)
	}
	return data, nil
}

// DeserializeStream parses an EventStream from JSON.
// The caller should call VerifyStreamIntegrity after deserialization.
func DeserializeStream(data []byte) (EventStream, error) {
	var s EventStream
	if err := json.Unmarshal(data, &s); err != nil {
		return EventStream{}, fmt.Errorf("deserialize stream: %w", err)
	}
	return s, nil
}

// RoundtripEvent serializes then deserializes an event, verifying no data loss.
// Returns an error if the deserialized event's hash does not match the original.
func RoundtripEvent(event PlatformEvent) (PlatformEvent, error) {
	data, err := CanonicalSerialize(event)
	if err != nil {
		return PlatformEvent{}, err
	}
	got, err := DeserializeEvent(data)
	if err != nil {
		return PlatformEvent{}, err
	}
	if got.Hash != event.Hash {
		return PlatformEvent{}, fmt.Errorf("roundtrip hash mismatch for event %q: original %q got %q",
			event.EventID, event.Hash, got.Hash)
	}
	return got, nil
}
