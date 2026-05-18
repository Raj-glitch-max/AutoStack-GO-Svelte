package persistence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/janlauber/one-click/pkg/audit"
)

// SnapshotPersistenceStore persists platform-level snapshots and certifications.
type SnapshotPersistenceStore struct {
	kv DurableKVStore
}

// NewSnapshotPersistenceStore wraps a KV store for snapshot persistence.
func NewSnapshotPersistenceStore(kv DurableKVStore) *SnapshotPersistenceStore {
	return &SnapshotPersistenceStore{kv: kv}
}

// SavePlatformSnapshot persists a PlatformRuntimeSnapshot.
func (s *SnapshotPersistenceStore) SavePlatformSnapshot(ctx context.Context, snap audit.PlatformRuntimeSnapshot) error {
	if ok, reason := audit.VerifyPlatformSnapshot(snap); !ok {
		return fmt.Errorf("snapshot store: invalid platform snapshot hash: %s", reason)
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("snapshot store: marshal: %w", err)
	}
	return s.kv.Put(ctx, "platformsnap:"+snap.SnapshotID, b)
}

// LoadPlatformSnapshot retrieves a platform snapshot by ID.
func (s *SnapshotPersistenceStore) LoadPlatformSnapshot(ctx context.Context, snapshotID string) (audit.PlatformRuntimeSnapshot, bool, error) {
	b, ok, err := s.kv.Get(ctx, "platformsnap:"+snapshotID)
	if err != nil || !ok {
		return audit.PlatformRuntimeSnapshot{}, ok, err
	}
	var snap audit.PlatformRuntimeSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return audit.PlatformRuntimeSnapshot{}, false, err
	}
	return snap, true, nil
}

// SaveReplayCertification persists a ReplayCertificationReport.
func (s *SnapshotPersistenceStore) SaveReplayCertification(ctx context.Context, cert audit.ReplayCertificationReport) error {
	if ok, reason := audit.VerifyCertificationHash(cert); !ok {
		return fmt.Errorf("snapshot store: invalid certification hash: %s", reason)
	}
	b, err := json.Marshal(cert)
	if err != nil {
		return fmt.Errorf("snapshot store: certification marshal: %w", err)
	}
	return s.kv.Put(ctx, "cert:"+cert.CertificationID, b)
}

// LoadReplayCertification retrieves a certification by ID.
func (s *SnapshotPersistenceStore) LoadReplayCertification(ctx context.Context, certID string) (audit.ReplayCertificationReport, bool, error) {
	b, ok, err := s.kv.Get(ctx, "cert:"+certID)
	if err != nil || !ok {
		return audit.ReplayCertificationReport{}, ok, err
	}
	var cert audit.ReplayCertificationReport
	if err := json.Unmarshal(b, &cert); err != nil {
		return audit.ReplayCertificationReport{}, false, err
	}
	return cert, true, nil
}

// AllCertificationsForExecution returns all certifications for an executionID.
func (s *SnapshotPersistenceStore) AllCertificationsForExecution(ctx context.Context, executionID string) ([]audit.ReplayCertificationReport, error) {
	all, err := s.kv.List(ctx, "cert:")
	if err != nil {
		return nil, err
	}
	var out []audit.ReplayCertificationReport
	for _, b := range all {
		var cert audit.ReplayCertificationReport
		if err := json.Unmarshal(b, &cert); err != nil {
			continue
		}
		if cert.ExecutionID == executionID {
			out = append(out, cert)
		}
	}
	return out, nil
}
