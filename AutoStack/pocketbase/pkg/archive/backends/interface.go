// Package backends provides durable storage backends for the archive subsystem.
//
// Invariants inherited from pkg/archive:
//   - Payloads are never mutated after archival.
//   - Restoration always verifies the PayloadHash before returning data.
//   - Manifests are tamper-evident.
package backends

import (
	"errors"

	"github.com/janlauber/one-click/pkg/archive"
)

// ErrEntryNotFound is returned when an entry ID is not found in the backend.
var ErrEntryNotFound = errors.New("archive entry not found")

// ErrManifestNotFound is returned when a manifest is not found.
var ErrManifestNotFound = errors.New("archive manifest not found")

// ErrPayloadCorrupt is returned when the stored payload's hash does not match
// the expected PayloadHash. Restoration is aborted — corrupt data is NEVER returned.
var ErrPayloadCorrupt = errors.New("archive payload hash mismatch: entry is corrupt")

// ArchiveBackend is the storage interface for durable archive persistence.
// All implementations MUST reject corrupt payloads on Load — returning corrupt
// data is a stronger failure mode than returning an error.
type ArchiveBackend interface {
	// Store persists an ArchiveEntry. Implementations must be idempotent for the
	// same EntryID (re-storing the same entry must not corrupt state).
	Store(entry archive.ArchiveEntry) error

	// Load retrieves an ArchiveEntry by ID and verifies its PayloadHash.
	// Returns ErrEntryNotFound if the ID is absent.
	// Returns ErrPayloadCorrupt if the payload hash does not match.
	Load(entryID string) (archive.ArchiveEntry, error)

	// StoreManifest persists an ArchiveManifest.
	StoreManifest(manifest archive.ArchiveManifest) error

	// LoadManifest retrieves an ArchiveManifest by ID.
	// Returns ErrManifestNotFound if absent.
	LoadManifest(manifestID string) (archive.ArchiveManifest, error)

	// ListEntries returns all entryIDs for the given tenant.
	ListEntries(tenantID string) ([]string, error)
}
