package backends

import (
	"errors"

	"github.com/janlauber/one-click/pkg/archive"
)

// ErrS3NotImplemented is returned by all S3ArchiveBackend methods.
// The S3 backend is an interface-only stub — a real implementation requires
// AWS SDK wiring and is out of scope for Phase 5.2.
var ErrS3NotImplemented = errors.New("S3 archive backend: not implemented in Phase 5.2")

// S3Config holds the configuration for an S3-compatible archive backend.
type S3Config struct {
	Bucket   string `json:"bucket"`
	Region   string `json:"region"`
	Prefix   string `json:"prefix"`
	Endpoint string `json:"endpoint,omitempty"` // optional: for S3-compatible services
}

// S3ArchiveBackend implements ArchiveBackend for S3-compatible object storage.
// All methods return ErrS3NotImplemented — the interface contract is defined here
// so callers can program against it without requiring AWS SDK at build time.
//
// A concrete implementation would use the AWS SDK v2 ECS client, injected via
// the providers interface — NOT by importing the SDK directly in this file.
type S3ArchiveBackend struct {
	config S3Config
}

// NewS3ArchiveBackend creates a stub S3ArchiveBackend with the given config.
func NewS3ArchiveBackend(config S3Config) *S3ArchiveBackend {
	return &S3ArchiveBackend{config: config}
}

func (b *S3ArchiveBackend) Store(_ archive.ArchiveEntry) error {
	return ErrS3NotImplemented
}

func (b *S3ArchiveBackend) Load(_ string) (archive.ArchiveEntry, error) {
	return archive.ArchiveEntry{}, ErrS3NotImplemented
}

func (b *S3ArchiveBackend) StoreManifest(_ archive.ArchiveManifest) error {
	return ErrS3NotImplemented
}

func (b *S3ArchiveBackend) LoadManifest(_ string) (archive.ArchiveManifest, error) {
	return archive.ArchiveManifest{}, ErrS3NotImplemented
}

func (b *S3ArchiveBackend) ListEntries(_ string) ([]string, error) {
	return nil, ErrS3NotImplemented
}
