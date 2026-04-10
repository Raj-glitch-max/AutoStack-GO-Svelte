package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
)

// Encrypt encrypts plaintext using AES-256-GCM.
// Returns base64(nonce + ciphertext). Never logs key or plaintext.
// Algorithm: AES-256-GCM
// Key source: AUTOSTACK_ENCRYPTION_KEY env var (32 bytes, base64-encoded)
// Nonce: Random 12 bytes, stored prepended to ciphertext
// Storage: Base64(nonce + ciphertext) stored in DB field
func Encrypt(plaintext string, keyBase64 string) (string, error) {
	// Decode the base64 key
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return "", fmt.Errorf("failed to decode encryption key: %w", err)
	}

	// Verify key is 32 bytes (AES-256)
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes for AES-256, got %d bytes", len(key))
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce (12 bytes for GCM)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and seal
	plaintextBytes := []byte(plaintext)
	ciphertext := gcm.Seal(nonce, nonce, plaintextBytes, nil)

	// Zero out plaintext from memory
	for i := range plaintextBytes {
		plaintextBytes[i] = 0
	}

	// Return base64 encoded (nonce + ciphertext)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64(nonce+ciphertext) string.
// Returns plaintext. Never logs key or plaintext.
func Decrypt(ciphertext string, keyBase64 string) (string, error) {
	// Decode the base64 key
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return "", fmt.Errorf("failed to decode encryption key: %w", err)
	}

	// Verify key is 32 bytes (AES-256)
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes for AES-256, got %d bytes", len(key))
	}

	// Decode the base64 ciphertext
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Verify data is long enough to contain nonce
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	// Extract nonce and ciphertext
	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

// GetEncryptionKey retrieves the encryption key from environment variable.
// Returns error if AUTOSTACK_ENCRYPTION_KEY is not set.
// This function should be called at startup to validate the key exists.
func GetEncryptionKey() (string, error) {
	key := os.Getenv("AUTOSTACK_ENCRYPTION_KEY")
	if key == "" {
		return "", fmt.Errorf("AUTOSTACK_ENCRYPTION_KEY environment variable is not set")
	}

	// Validate it's valid base64 and 32 bytes when decoded
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return "", fmt.Errorf("AUTOSTACK_ENCRYPTION_KEY must be valid base64: %w", err)
	}

	if len(decoded) != 32 {
		return "", fmt.Errorf("AUTOSTACK_ENCRYPTION_KEY must be 32 bytes when decoded, got %d bytes", len(decoded))
	}

	return key, nil
}

// ValidateEncryptionKeyAtStartup validates that the encryption key is properly configured.
// This should be called during application startup. If it returns an error, the application
// should exit immediately with a clear error message.
func ValidateEncryptionKeyAtStartup() error {
	_, err := GetEncryptionKey()
	if err != nil {
		return fmt.Errorf("CRITICAL: %w - Application cannot start without valid encryption key", err)
	}
	return nil
}
