package secrets

import (
	"encoding/base64"
	"os"
	"strings"
	"sync"
	"testing"
)

// resetCache forces getAEAD to re-read the environment. Tests change the
// key per case; without this they would see the first key forever.
func resetCache() {
	cacheMu.Lock()
	cachedAEAD = nil
	cachedKeyPrint = ""
	cacheMu.Unlock()
}

func setKey(t *testing.T) {
	t.Helper()
	key := make([]byte, keySize)
	for i := range key {
		key[i] = byte(i + 1)
	}
	t.Setenv(EnvKey, base64.StdEncoding.EncodeToString(key))
	resetCache()
}

func TestRoundTrip(t *testing.T) {
	setKey(t)
	pt := "service-account-json-blob"
	ct, err := Encrypt(pt)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !strings.HasPrefix(ct, "enc:v1:") {
		t.Fatalf("ciphertext missing v1 prefix: %q", ct)
	}
	if ct == pt {
		t.Fatalf("ciphertext equals plaintext")
	}
	got, err := Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != pt {
		t.Fatalf("roundtrip mismatch: got %q want %q", got, pt)
	}
}

func TestEmptyPassthrough(t *testing.T) {
	setKey(t)
	if got, err := Encrypt(""); err != nil || got != "" {
		t.Fatalf("Encrypt empty: got=%q err=%v", got, err)
	}
	if got, err := Decrypt(""); err != nil || got != "" {
		t.Fatalf("Decrypt empty: got=%q err=%v", got, err)
	}
}

func TestLegacyPlaintextFallback(t *testing.T) {
	setKey(t)
	// A value without the prefix is legacy plaintext and must be returned as-is.
	if got, err := Decrypt("legacy-cred-blob"); err != nil || got != "legacy-cred-blob" {
		t.Fatalf("legacy passthrough: got=%q err=%v", got, err)
	}
	if IsEncrypted("legacy-cred-blob") {
		t.Fatalf("legacy should not be reported as encrypted")
	}
}

func TestMissingKey(t *testing.T) {
	t.Setenv(EnvKey, "")
	resetCache()
	if _, err := Encrypt("anything"); err == nil || err != ErrKeyMissing {
		t.Fatalf("expected ErrKeyMissing, got %v", err)
	}
	if err := EnsureConfigured(); err != ErrKeyMissing {
		t.Fatalf("EnsureConfigured: expected ErrKeyMissing, got %v", err)
	}
}

func TestMalformedKey(t *testing.T) {
	t.Setenv(EnvKey, "not-base64!!!")
	resetCache()
	if _, err := Encrypt("anything"); err != ErrKeyMissing {
		t.Fatalf("expected ErrKeyMissing for bad b64, got %v", err)
	}
}

func TestWrongKeyRejectsCiphertext(t *testing.T) {
	setKey(t)
	ct, err := Encrypt("payload")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Swap the key, then try to decrypt: must fail loud, not return plaintext.
	other := make([]byte, keySize)
	for i := range other {
		other[i] = 0xFF
	}
	t.Setenv(EnvKey, base64.StdEncoding.EncodeToString(other))
	resetCache()

	if _, err := Decrypt(ct); err != ErrCiphertextCorrupt {
		t.Fatalf("expected ErrCiphertextCorrupt under wrong key, got %v", err)
	}
}

func TestConcurrentInit(t *testing.T) {
	setKey(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Encrypt("p"); err != nil {
				t.Errorf("Encrypt under concurrency: %v", err)
			}
		}()
	}
	wg.Wait()
}

// Sanity check on env-var contract: 32 raw bytes b64-encoded is 44 chars.
func TestKeyFingerprintStable(t *testing.T) {
	setKey(t)
	fp1 := KeyFingerprint()
	fp2 := KeyFingerprint()
	if fp1 == "" || fp1 != fp2 {
		t.Fatalf("KeyFingerprint unstable: %q vs %q", fp1, fp2)
	}
	if len(os.Getenv(EnvKey)) != 44 {
		t.Fatalf("test setup wrong: env key should be 44 chars b64")
	}
}
