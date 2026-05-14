// Package secrets provides foundational at-rest encryption for credential
// blobs stored in PocketBase.
//
// Phase 2.0 scope: AES-256-GCM with a key sourced from the
// AUTOSTACK_ENCRYPTION_KEY environment variable. This is intentionally NOT
// KMS-grade — see project-context/known-issues/deferred-operational-hardening.md
// for the work that remains (key rotation, HSM, per-tenant keys, at-rest
// SQLite encryption).
//
// Ciphertext format on disk:
//
//	"enc:v1:" + base64(nonce(12) || ciphertext)
//
// The "enc:v1:" prefix is the only durable contract callers should rely on
// for detecting whether a stored value is encrypted. A future v2 format
// will carry the same prefix shape with a different version number.
//
// Plaintext fallback: a value WITHOUT the prefix is treated as legacy
// plaintext and returned as-is from Decrypt. This lets the controller
// lazily re-encrypt on next write without a one-shot migration. Once all
// rows are re-encrypted, the fallback can be removed.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

const (
	// EnvKey is the environment variable that holds the encryption key.
	// The key MUST be 32 raw bytes, encoded as base64 (44 chars).
	EnvKey = "AUTOSTACK_ENCRYPTION_KEY"

	// ciphertextPrefix marks a value as v1 ciphertext on disk.
	ciphertextPrefix = "enc:v1:"

	nonceSize = 12
	keySize   = 32
)

// ErrKeyMissing is returned when AUTOSTACK_ENCRYPTION_KEY is unset or
// malformed. Callers MUST refuse to write or read credential blobs in this
// state — silently falling back to plaintext would re-introduce the very
// problem this package exists to solve.
var ErrKeyMissing = errors.New("secrets: AUTOSTACK_ENCRYPTION_KEY is not set or malformed")

// ErrCiphertextCorrupt is returned when a value carries the v1 prefix but
// fails to decrypt — typically because the encryption key changed between
// process lifetimes. Refuse to use the value; do not silently fall through.
var ErrCiphertextCorrupt = errors.New("secrets: ciphertext failed authenticated decryption")

var (
	cachedAEAD     cipher.AEAD
	cachedKeyPrint string
	cacheMu        sync.RWMutex
)

// Encrypt encrypts plaintext with the configured key and returns the v1
// ciphertext form ("enc:v1:..."). An empty plaintext is returned as the
// empty string, unchanged, since storing encryption of nothing leaks size
// information without protecting any secret.
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	aead, err := getAEAD()
	if err != nil {
		return "", err
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secrets: nonce generation: %w", err)
	}

	ct := aead.Seal(nil, nonce, []byte(plaintext), nil)
	combined := make([]byte, 0, len(nonce)+len(ct))
	combined = append(combined, nonce...)
	combined = append(combined, ct...)
	return ciphertextPrefix + base64.StdEncoding.EncodeToString(combined), nil
}

// Decrypt returns the plaintext for a stored value.
//
//   - If the value carries the "enc:v1:" prefix, it is decrypted; a key
//     mismatch or tampered ciphertext returns ErrCiphertextCorrupt.
//   - If the value does NOT carry the prefix, it is treated as legacy
//     plaintext and returned as-is. This is the only way to read records
//     written before encryption landed.
//   - Empty values are returned unchanged.
//
// Callers wanting to know whether they got back plaintext-fallback data
// should call IsEncrypted on the original stored value.
func Decrypt(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !IsEncrypted(stored) {
		return stored, nil
	}
	aead, err := getAEAD()
	if err != nil {
		return "", err
	}

	body := strings.TrimPrefix(stored, ciphertextPrefix)
	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		return "", ErrCiphertextCorrupt
	}
	if len(raw) < nonceSize {
		return "", ErrCiphertextCorrupt
	}
	nonce, ct := raw[:nonceSize], raw[nonceSize:]
	pt, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", ErrCiphertextCorrupt
	}
	return string(pt), nil
}

// IsEncrypted reports whether a stored value is in v1 ciphertext form.
// Callers can use this to decide whether to lazily re-encrypt on write.
func IsEncrypted(stored string) bool {
	return strings.HasPrefix(stored, ciphertextPrefix)
}

// KeyFingerprint returns a short hex hash of the configured key, useful
// for logging and for detecting key rotation across restarts without
// exposing the key itself. Returns "" if no key is configured.
func KeyFingerprint() string {
	cacheMu.RLock()
	fp := cachedKeyPrint
	cacheMu.RUnlock()
	if fp != "" {
		return fp
	}
	if _, err := getAEAD(); err != nil {
		return ""
	}
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	return cachedKeyPrint
}

// EnsureConfigured returns nil iff the encryption subsystem is usable.
// Call this at process boot to fail loud if AUTOSTACK_ENCRYPTION_KEY is
// missing, rather than letting individual credential reads fail later.
func EnsureConfigured() error {
	_, err := getAEAD()
	return err
}

func getAEAD() (cipher.AEAD, error) {
	cacheMu.RLock()
	if cachedAEAD != nil {
		defer cacheMu.RUnlock()
		return cachedAEAD, nil
	}
	cacheMu.RUnlock()

	cacheMu.Lock()
	defer cacheMu.Unlock()
	if cachedAEAD != nil {
		return cachedAEAD, nil
	}

	rawB64 := strings.TrimSpace(os.Getenv(EnvKey))
	if rawB64 == "" {
		return nil, ErrKeyMissing
	}
	key, err := base64.StdEncoding.DecodeString(rawB64)
	if err != nil || len(key) != keySize {
		return nil, ErrKeyMissing
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secrets: aes.NewCipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secrets: cipher.NewGCM: %w", err)
	}

	sum := sha256.Sum256(key)
	cachedAEAD = aead
	cachedKeyPrint = hex.EncodeToString(sum[:4])
	return cachedAEAD, nil
}
