package security

import (
	"testing"
	"time"
)

func TestInProcessRevocationList_RevokeAndCheck(t *testing.T) {
	rl := NewInProcessRevocationList()
	if err := rl.Revoke("tok-1", "tenant-a", "manual"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	revoked, err := rl.IsRevoked("tok-1")
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if !revoked {
		t.Fatal("tok-1 should be revoked")
	}
}

func TestInProcessRevocationList_NotRevoked(t *testing.T) {
	rl := NewInProcessRevocationList()
	revoked, _ := rl.IsRevoked("unknown-token")
	if revoked {
		t.Fatal("unknown token should not be revoked")
	}
}

func TestInProcessRevocationList_Idempotent(t *testing.T) {
	rl := NewInProcessRevocationList()
	rl.Revoke("tok-2", "tenant-a", "first")
	if err := rl.Revoke("tok-2", "tenant-a", "second"); err != nil {
		t.Errorf("re-revoking same token should be idempotent: %v", err)
	}
	n, _ := rl.Count()
	if n != 1 {
		t.Errorf("expected 1 revoked token, got %d", n)
	}
}

func TestInProcessRevocationList_EmptyTokenRejected(t *testing.T) {
	rl := NewInProcessRevocationList()
	if err := rl.Revoke("", "tenant-a", "reason"); err == nil {
		t.Fatal("empty token ID should be rejected")
	}
}

func TestPersistentRevocationStore_RevokeAndCheck(t *testing.T) {
	store, err := NewPersistentRevocationStore(":memory:")
	if err != nil {
		t.Fatalf("NewPersistentRevocationStore: %v", err)
	}
	defer store.Close()

	if err := store.Revoke("tok-p1", "tenant-a", "key-rotation"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	revoked, err := store.IsRevoked("tok-p1")
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if !revoked {
		t.Fatal("tok-p1 should be revoked")
	}
}

func TestPersistentRevocationStore_NotRevoked(t *testing.T) {
	store, err := NewPersistentRevocationStore(":memory:")
	if err != nil {
		t.Fatalf("NewPersistentRevocationStore: %v", err)
	}
	defer store.Close()

	revoked, _ := store.IsRevoked("not-revoked")
	if revoked {
		t.Fatal("not-revoked should not be in the revocation store")
	}
}

func TestPersistentRevocationStore_Idempotent(t *testing.T) {
	store, err := NewPersistentRevocationStore(":memory:")
	if err != nil {
		t.Fatalf("NewPersistentRevocationStore: %v", err)
	}
	defer store.Close()

	store.Revoke("tok-idem", "tenant-a", "first")
	if err := store.Revoke("tok-idem", "tenant-a", "second"); err != nil {
		t.Errorf("re-revocation should be idempotent: %v", err)
	}
	n, _ := store.Count()
	if n != 1 {
		t.Errorf("expected 1 revoked token, got %d", n)
	}
}

func TestVerifyJWTWithRevocation_ValidNotRevoked(t *testing.T) {
	secret := "rotation-test-secret"
	tok, err := IssueJWT("tok-vr-1", "tenant-a", RoleOperator, time.Hour, secret)
	if err != nil {
		t.Fatalf("IssueJWT: %v", err)
	}
	rl := NewInProcessRevocationList()
	result := VerifyJWTWithRevocation(tok, secret, "tenant-a", rl)
	if !result.Valid {
		t.Fatalf("valid non-revoked token should pass: %s", result.Reason)
	}
}

func TestVerifyJWTWithRevocation_RevokedToken(t *testing.T) {
	secret := "rotation-test-secret"
	tok, _ := IssueJWT("tok-vr-2", "tenant-a", RoleViewer, time.Hour, secret)
	rl := NewInProcessRevocationList()
	rl.Revoke("tok-vr-2", "tenant-a", "manual revocation")

	result := VerifyJWTWithRevocation(tok, secret, "tenant-a", rl)
	if result.Valid {
		t.Fatal("revoked token should be rejected")
	}
	if result.Reason != "token revoked" {
		t.Errorf("expected reason 'token revoked', got %q", result.Reason)
	}
}

func TestVerifyJWTWithRevocation_InvalidTokenUnchanged(t *testing.T) {
	rl := NewInProcessRevocationList()
	result := VerifyJWTWithRevocation("bad.token.here", "secret", "tenant-a", rl)
	if result.Valid {
		t.Fatal("invalid token should fail before revocation check")
	}
}

func TestPerformKeyRotation_RevokesAllTokens(t *testing.T) {
	rl := NewInProcessRevocationList()
	oldTokens := []string{"tok-old-1", "tok-old-2", "tok-old-3"}

	record, err := PerformKeyRotation("rot-1", "tenant-a", oldTokens, rl)
	if err != nil {
		t.Fatalf("PerformKeyRotation: %v", err)
	}
	if record.RevokedCount != 3 {
		t.Errorf("expected 3 revoked, got %d", record.RevokedCount)
	}
	if record.RotationID != "rot-1" {
		t.Errorf("unexpected rotation ID: %s", record.RotationID)
	}

	// All old tokens must be revoked.
	for _, id := range oldTokens {
		revoked, _ := rl.IsRevoked(id)
		if !revoked {
			t.Errorf("token %s should be revoked after key rotation", id)
		}
	}
}

func TestPerformKeyRotation_PersistentRevocationSurvivesRestart(t *testing.T) {
	store, err := NewPersistentRevocationStore(":memory:")
	if err != nil {
		t.Fatalf("NewPersistentRevocationStore: %v", err)
	}
	defer store.Close()

	_, err = PerformKeyRotation("rot-persist", "tenant-a", []string{"persist-tok-1", "persist-tok-2"}, store)
	if err != nil {
		t.Fatalf("PerformKeyRotation: %v", err)
	}

	// Simulate restart: same store, new check.
	revoked, _ := store.IsRevoked("persist-tok-1")
	if !revoked {
		t.Error("persist-tok-1 should be revoked after restart simulation")
	}

	n, _ := store.Count()
	if n != 2 {
		t.Errorf("expected 2 revoked tokens, got %d", n)
	}
}

func TestPerformKeyRotation_EmptyTokenList(t *testing.T) {
	rl := NewInProcessRevocationList()
	record, err := PerformKeyRotation("rot-empty", "tenant-a", nil, rl)
	if err != nil {
		t.Fatalf("rotation with empty token list: %v", err)
	}
	if record.RevokedCount != 0 {
		t.Errorf("expected 0 revoked, got %d", record.RevokedCount)
	}
}
