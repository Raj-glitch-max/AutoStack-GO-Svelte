package backends

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/janlauber/one-click/pkg/archive"
)

// FilesystemArchiveBackend stores archive entries as JSON files under rootDir.
//
// Layout:
//
//	<rootDir>/entries/<entryID>.json
//	<rootDir>/manifests/<manifestID>.json
//
// Restore always verifies PayloadHash before returning data.
// Corrupt files are reported as ErrPayloadCorrupt — never silently returned.
type FilesystemArchiveBackend struct {
	rootDir string
}

// NewFilesystemArchiveBackend creates a backend rooted at rootDir.
// The required subdirectories are created if they do not exist.
func NewFilesystemArchiveBackend(rootDir string) (*FilesystemArchiveBackend, error) {
	if err := os.MkdirAll(filepath.Join(rootDir, "entries"), 0700); err != nil {
		return nil, fmt.Errorf("create entries dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(rootDir, "manifests"), 0700); err != nil {
		return nil, fmt.Errorf("create manifests dir: %w", err)
	}
	return &FilesystemArchiveBackend{rootDir: rootDir}, nil
}

// Store persists an ArchiveEntry as JSON to disk.
// Writing is atomic: we write to a temp file and rename.
func (b *FilesystemArchiveBackend) Store(entry archive.ArchiveEntry) error {
	if entry.EntryID == "" {
		return fmt.Errorf("entry ID must not be empty")
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal entry %q: %w", entry.EntryID, err)
	}
	path := b.entryPath(entry.EntryID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write entry %q: %w", entry.EntryID, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename entry %q: %w", entry.EntryID, err)
	}
	return nil
}

// Load retrieves an ArchiveEntry and verifies its PayloadHash.
// Returns ErrEntryNotFound when the file is absent.
// Returns ErrPayloadCorrupt when the hash does not match.
func (b *FilesystemArchiveBackend) Load(entryID string) (archive.ArchiveEntry, error) {
	data, err := os.ReadFile(b.entryPath(entryID))
	if os.IsNotExist(err) {
		return archive.ArchiveEntry{}, ErrEntryNotFound
	}
	if err != nil {
		return archive.ArchiveEntry{}, fmt.Errorf("read entry %q: %w", entryID, err)
	}
	var entry archive.ArchiveEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return archive.ArchiveEntry{}, fmt.Errorf("unmarshal entry %q: %w", entryID, err)
	}
	// Always verify before returning.
	ok, reason := archive.VerifyArchiveEntry(entry)
	if !ok {
		return archive.ArchiveEntry{}, fmt.Errorf("%w: %s", ErrPayloadCorrupt, reason)
	}
	return entry, nil
}

// StoreManifest persists an ArchiveManifest atomically.
func (b *FilesystemArchiveBackend) StoreManifest(manifest archive.ArchiveManifest) error {
	if manifest.ManifestID == "" {
		return fmt.Errorf("manifest ID must not be empty")
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal manifest %q: %w", manifest.ManifestID, err)
	}
	path := b.manifestPath(manifest.ManifestID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write manifest %q: %w", manifest.ManifestID, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename manifest %q: %w", manifest.ManifestID, err)
	}
	return nil
}

// LoadManifest retrieves an ArchiveManifest and verifies its ManifestHash.
func (b *FilesystemArchiveBackend) LoadManifest(manifestID string) (archive.ArchiveManifest, error) {
	data, err := os.ReadFile(b.manifestPath(manifestID))
	if os.IsNotExist(err) {
		return archive.ArchiveManifest{}, ErrManifestNotFound
	}
	if err != nil {
		return archive.ArchiveManifest{}, fmt.Errorf("read manifest %q: %w", manifestID, err)
	}
	var manifest archive.ArchiveManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return archive.ArchiveManifest{}, fmt.Errorf("unmarshal manifest %q: %w", manifestID, err)
	}
	ok, reason := archive.VerifyArchiveManifest(manifest)
	if !ok {
		return archive.ArchiveManifest{}, fmt.Errorf("%w: manifest %s", ErrPayloadCorrupt, reason)
	}
	return manifest, nil
}

// ListEntries returns all entryIDs stored for a given tenant.
func (b *FilesystemArchiveBackend) ListEntries(tenantID string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(b.rootDir, "entries"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read entries dir: %w", err)
	}
	var ids []string
	for _, de := range entries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".json") {
			continue
		}
		entryID := strings.TrimSuffix(de.Name(), ".json")
		// Load to check tenant ownership — only include matching entries.
		entry, err := b.Load(entryID)
		if err != nil {
			continue // corrupt or missing — skip
		}
		if entry.TenantID == tenantID {
			ids = append(ids, entryID)
		}
	}
	return ids, nil
}

func (b *FilesystemArchiveBackend) entryPath(entryID string) string {
	return filepath.Join(b.rootDir, "entries", entryID+".json")
}

func (b *FilesystemArchiveBackend) manifestPath(manifestID string) string {
	return filepath.Join(b.rootDir, "manifests", manifestID+".json")
}
