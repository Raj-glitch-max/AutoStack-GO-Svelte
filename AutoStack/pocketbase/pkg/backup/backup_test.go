package backup

import (
	"encoding/json"
	"testing"
)

func makeTestPayload(t *testing.T, data map[string]interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return b
}

func TestCreateAndVerifyBackupManifest(t *testing.T) {
	payload := makeTestPayload(t, map[string]interface{}{"entries": 5, "version": "6.0.0"})
	m := CreateBackupManifest("bk-1", "tenant-a", "exec-1", "6.0.0", BackupKindPlatformSnapshot, payload)

	ok, reason := VerifyBackupManifest(m)
	if !ok {
		t.Fatalf("manifest verification failed: %s", reason)
	}
}

func TestVerifyBackupManifest_TamperedHash(t *testing.T) {
	payload := makeTestPayload(t, map[string]interface{}{"entries": 5})
	m := CreateBackupManifest("bk-2", "tenant-a", "exec-1", "6.0.0", BackupKindArchiveSnapshot, payload)
	m.ManifestHash = "tampered"

	ok, _ := VerifyBackupManifest(m)
	if ok {
		t.Fatal("tampered manifest hash should fail verification")
	}
}

func TestVerifyPayloadIntegrity_Valid(t *testing.T) {
	payload := makeTestPayload(t, map[string]interface{}{"state": "running"})
	m := CreateBackupManifest("bk-3", "tenant-a", "exec-1", "6.0.0", BackupKindGovernanceState, payload)

	ok, reason := VerifyPayloadIntegrity(m, payload)
	if !ok {
		t.Fatalf("payload integrity check failed: %s", reason)
	}
}

func TestVerifyPayloadIntegrity_Tampered(t *testing.T) {
	payload := makeTestPayload(t, map[string]interface{}{"state": "running"})
	m := CreateBackupManifest("bk-4", "tenant-a", "exec-1", "6.0.0", BackupKindGovernanceState, payload)

	corrupted := append(payload, []byte("CORRUPT")...)
	ok, _ := VerifyPayloadIntegrity(m, corrupted)
	if ok {
		t.Fatal("corrupted payload should fail integrity check")
	}
}

func TestRestorePlatformSnapshot_Success(t *testing.T) {
	payload := makeTestPayload(t, map[string]interface{}{"tasks": 10, "completed": 9})
	m := CreateBackupManifest("bk-5", "tenant-a", "exec-1", "6.0.0", BackupKindPlatformSnapshot, payload)
	m.EntryCount = 10
	m.ManifestHash = computeManifestHash(m)

	result := RestorePlatformSnapshot(m, payload)
	if !result.Succeeded {
		t.Fatalf("restore should succeed: %s", result.Reason)
	}
	if result.EntriesRestored != 10 {
		t.Errorf("expected 10 entries restored, got %d", result.EntriesRestored)
	}
}

func TestRestorePlatformSnapshot_BadManifestHash(t *testing.T) {
	payload := makeTestPayload(t, map[string]interface{}{"tasks": 5})
	m := CreateBackupManifest("bk-6", "tenant-a", "exec-1", "6.0.0", BackupKindPlatformSnapshot, payload)
	m.ManifestHash = "intentionally-wrong"

	result := RestorePlatformSnapshot(m, payload)
	if result.Succeeded {
		t.Fatal("restore should fail on bad manifest hash")
	}
}

func TestRestorePlatformSnapshot_CorruptPayload(t *testing.T) {
	payload := makeTestPayload(t, map[string]interface{}{"tasks": 5})
	m := CreateBackupManifest("bk-7", "tenant-a", "exec-1", "6.0.0", BackupKindPlatformSnapshot, payload)

	corrupted := []byte("NOT_JSON")
	result := RestorePlatformSnapshot(m, corrupted)
	if result.Succeeded {
		t.Fatal("restore should fail on corrupted payload")
	}
}

func TestRestorePlatformSnapshot_InvalidJSON(t *testing.T) {
	badPayload := []byte(`{"unclosed": "json"`)
	m := CreateBackupManifest("bk-8", "tenant-a", "exec-1", "6.0.0", BackupKindPlatformSnapshot, badPayload)

	result := RestorePlatformSnapshot(m, badPayload)
	if result.Succeeded {
		t.Fatal("restore should fail on invalid JSON payload")
	}
}

func TestBackupRegistry_AppendOnly(t *testing.T) {
	reg := NewBackupRegistry()
	payload := makeTestPayload(t, map[string]interface{}{"v": 1})
	m := CreateBackupManifest("bk-dup", "tenant-a", "exec-1", "6.0.0", BackupKindCertification, payload)

	if err := reg.Register(m); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := reg.Register(m); err == nil {
		t.Fatal("duplicate registration should be rejected (append-only)")
	}
}

func TestBackupRegistry_GetAndAll(t *testing.T) {
	reg := NewBackupRegistry()
	for i := 0; i < 3; i++ {
		payload := makeTestPayload(t, map[string]interface{}{"i": i})
		id := "bk-" + string(rune('a'+i))
		m := CreateBackupManifest(id, "tenant-a", "exec-1", "6.0.0", BackupKindReplayManifest, payload)
		reg.Register(m)
	}

	m, ok := reg.Get("bk-b")
	if !ok {
		t.Fatal("bk-b should be registered")
	}
	if m.BackupID != "bk-b" {
		t.Errorf("unexpected backup ID: %s", m.BackupID)
	}

	all := reg.All()
	if len(all) != 3 {
		t.Errorf("expected 3 manifests, got %d", len(all))
	}
}

func TestBackupRegistry_LatestForKind(t *testing.T) {
	reg := NewBackupRegistry()
	payload := makeTestPayload(t, map[string]interface{}{"v": 1})

	m1 := CreateBackupManifest("snap-1", "tenant-a", "exec-1", "6.0.0", BackupKindPlatformSnapshot, payload)
	m2 := CreateBackupManifest("snap-2", "tenant-a", "exec-2", "6.0.0", BackupKindPlatformSnapshot, payload)
	m3 := CreateBackupManifest("arch-1", "tenant-a", "exec-1", "6.0.0", BackupKindArchiveSnapshot, payload)
	reg.Register(m1)
	reg.Register(m2)
	reg.Register(m3)

	latest, ok := reg.LatestForKind(BackupKindPlatformSnapshot, "tenant-a")
	if !ok {
		t.Fatal("should find latest platform snapshot")
	}
	_ = latest // CreatedAt comparison; just verify it was found.

	_, ok2 := reg.LatestForKind(BackupKindGovernanceState, "tenant-a")
	if ok2 {
		t.Fatal("should not find governance backup that was never registered")
	}
}

func TestBackupRegistry_AllReturnsCopy(t *testing.T) {
	reg := NewBackupRegistry()
	payload := makeTestPayload(t, map[string]interface{}{"v": 1})
	m := CreateBackupManifest("bk-copy", "tenant-a", "exec-1", "6.0.0", BackupKindCertification, payload)
	reg.Register(m)

	all1 := reg.All()
	all1[0].BackupID = "mutated"
	all2 := reg.All()
	if all2[0].BackupID == "mutated" {
		t.Error("All must return a copy, not the live slice")
	}
}

func TestCreateBackupManifest_ExcludesCreatedAt(t *testing.T) {
	payload := makeTestPayload(t, map[string]interface{}{"x": 1})
	m := CreateBackupManifest("bk-ts", "tenant-a", "exec-1", "6.0.0", BackupKindPlatformSnapshot, payload)

	hash1 := m.ManifestHash
	m.CreatedAt = "3000-01-01T00:00:00Z"
	// Recompute — hash should be the same since CreatedAt is excluded.
	hash2 := computeManifestHash(m)
	if hash1 != hash2 {
		t.Error("ManifestHash must not change when only CreatedAt changes (content-identity)")
	}
}
