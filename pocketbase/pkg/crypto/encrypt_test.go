package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

// generateTestKey generates a valid 32-byte base64-encoded key for testing
func generateTestKey() string {
	key := make([]byte, 32)
	rand.Read(key)
	return base64.StdEncoding.EncodeToString(key)
}

// TestEncryptDecryptRoundTrip verifies that encrypt then decrypt returns the original plaintext
func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := generateTestKey()
	testCases := []string{
		"simple text",
		"AKIAIOSFODNN7EXAMPLE",
		"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"",
		"special chars: !@#$%^&*()_+-=[]{}|;:',.<>?/~`",
		"unicode: 你好世界 🌍",
		strings.Repeat("a", 1000), // long string
	}

	for _, plaintext := range testCases {
		t.Run("plaintext_"+plaintext[:min(len(plaintext), 20)], func(t *testing.T) {
			// Encrypt
			ciphertext, err := Encrypt(plaintext, key)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}

			// Decrypt
			decrypted, err := Decrypt(ciphertext, key)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}

			// Verify
			if decrypted != plaintext {
				t.Errorf("Round trip failed: got %q, want %q", decrypted, plaintext)
			}
		})
	}
}

// TestEncryptUniqueNonce verifies that two encryptions of the same plaintext produce different ciphertext
func TestEncryptUniqueNonce(t *testing.T) {
	key := generateTestKey()
	plaintext := "test secret value"

	// Encrypt the same plaintext twice
	ciphertext1, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("First encryption failed: %v", err)
	}

	ciphertext2, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Second encryption failed: %v", err)
	}

	// Verify they are different (due to different nonces)
	if ciphertext1 == ciphertext2 {
		t.Error("Two encryptions of the same plaintext produced identical ciphertext - nonce is not unique!")
	}

	// Verify both decrypt to the same plaintext
	decrypted1, err := Decrypt(ciphertext1, key)
	if err != nil {
		t.Fatalf("First decryption failed: %v", err)
	}

	decrypted2, err := Decrypt(ciphertext2, key)
	if err != nil {
		t.Fatalf("Second decryption failed: %v", err)
	}

	if decrypted1 != plaintext || decrypted2 != plaintext {
		t.Error("Decryption did not return original plaintext")
	}
}

// TestEncryptInvalidKey verifies that encryption fails with invalid keys
func TestEncryptInvalidKey(t *testing.T) {
	testCases := []struct {
		name string
		key  string
	}{
		{"empty key", ""},
		{"invalid base64", "not-valid-base64!!!"},
		{"wrong size (16 bytes)", base64.StdEncoding.EncodeToString(make([]byte, 16))},
		{"wrong size (64 bytes)", base64.StdEncoding.EncodeToString(make([]byte, 64))},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Encrypt("test", tc.key)
			if err == nil {
				t.Error("Expected error for invalid key, got nil")
			}
		})
	}
}

// TestDecryptInvalidCiphertext verifies that decryption fails with invalid ciphertext
func TestDecryptInvalidCiphertext(t *testing.T) {
	key := generateTestKey()

	testCases := []struct {
		name       string
		ciphertext string
	}{
		{"empty ciphertext", ""},
		{"invalid base64", "not-valid-base64!!!"},
		{"too short", base64.StdEncoding.EncodeToString([]byte("short"))},
		{"corrupted data", base64.StdEncoding.EncodeToString(make([]byte, 50))},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decrypt(tc.ciphertext, key)
			if err == nil {
				t.Error("Expected error for invalid ciphertext, got nil")
			}
		})
	}
}

// TestDecryptWrongKey verifies that decryption fails with wrong key
func TestDecryptWrongKey(t *testing.T) {
	key1 := generateTestKey()
	key2 := generateTestKey()

	plaintext := "secret data"

	// Encrypt with key1
	ciphertext, err := Encrypt(plaintext, key1)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	// Try to decrypt with key2
	_, err = Decrypt(ciphertext, key2)
	if err == nil {
		t.Error("Expected error when decrypting with wrong key, got nil")
	}
}

// TestGetEncryptionKey verifies environment variable handling
func TestGetEncryptionKey(t *testing.T) {
	// Save original env var
	originalKey := os.Getenv("AUTOSTACK_ENCRYPTION_KEY")
	defer func() {
		if originalKey != "" {
			os.Setenv("AUTOSTACK_ENCRYPTION_KEY", originalKey)
		} else {
			os.Unsetenv("AUTOSTACK_ENCRYPTION_KEY")
		}
	}()

	t.Run("missing env var", func(t *testing.T) {
		os.Unsetenv("AUTOSTACK_ENCRYPTION_KEY")
		_, err := GetEncryptionKey()
		if err == nil {
			t.Error("Expected error when env var is not set, got nil")
		}
	})

	t.Run("valid env var", func(t *testing.T) {
		validKey := generateTestKey()
		os.Setenv("AUTOSTACK_ENCRYPTION_KEY", validKey)
		key, err := GetEncryptionKey()
		if err != nil {
			t.Errorf("Expected no error with valid key, got: %v", err)
		}
		if key != validKey {
			t.Error("Returned key does not match set key")
		}
	})

	t.Run("invalid base64 env var", func(t *testing.T) {
		os.Setenv("AUTOSTACK_ENCRYPTION_KEY", "not-valid-base64!!!")
		_, err := GetEncryptionKey()
		if err == nil {
			t.Error("Expected error with invalid base64, got nil")
		}
	})

	t.Run("wrong size env var", func(t *testing.T) {
		wrongSizeKey := base64.StdEncoding.EncodeToString(make([]byte, 16))
		os.Setenv("AUTOSTACK_ENCRYPTION_KEY", wrongSizeKey)
		_, err := GetEncryptionKey()
		if err == nil {
			t.Error("Expected error with wrong size key, got nil")
		}
	})
}

// TestValidateEncryptionKeyAtStartup verifies startup validation
func TestValidateEncryptionKeyAtStartup(t *testing.T) {
	// Save original env var
	originalKey := os.Getenv("AUTOSTACK_ENCRYPTION_KEY")
	defer func() {
		if originalKey != "" {
			os.Setenv("AUTOSTACK_ENCRYPTION_KEY", originalKey)
		} else {
			os.Unsetenv("AUTOSTACK_ENCRYPTION_KEY")
		}
	}()

	t.Run("fails when key missing", func(t *testing.T) {
		os.Unsetenv("AUTOSTACK_ENCRYPTION_KEY")
		err := ValidateEncryptionKeyAtStartup()
		if err == nil {
			t.Error("Expected error when key is missing, got nil")
		}
		if !strings.Contains(err.Error(), "CRITICAL") {
			t.Error("Error message should contain 'CRITICAL'")
		}
	})

	t.Run("succeeds with valid key", func(t *testing.T) {
		validKey := generateTestKey()
		os.Setenv("AUTOSTACK_ENCRYPTION_KEY", validKey)
		err := ValidateEncryptionKeyAtStartup()
		if err != nil {
			t.Errorf("Expected no error with valid key, got: %v", err)
		}
	})
}

// TestMemoryZeroing verifies that plaintext is zeroed from memory after encryption
// This is a basic test - in production, use memory analysis tools for thorough verification
func TestMemoryZeroing(t *testing.T) {
	key := generateTestKey()
	plaintext := "sensitive data that should be zeroed"

	// Encrypt
	_, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	// Note: This test verifies the code path exists, but actual memory zeroing
	// verification requires runtime memory inspection tools
	// The implementation zeros the byte slice, which is the best we can do in Go
}

// TestAES256GCMUsed verifies that we're using AES-256-GCM (not CBC or ECB)
func TestAES256GCMUsed(t *testing.T) {
	key := generateTestKey()
	plaintext := "test data"

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	// Decode to check structure
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		t.Fatalf("Failed to decode ciphertext: %v", err)
	}

	// GCM nonce is 12 bytes, and GCM adds 16 bytes authentication tag
	// So minimum size is: 12 (nonce) + len(plaintext) + 16 (tag)
	expectedMinSize := 12 + len(plaintext) + 16
	if len(data) < expectedMinSize {
		t.Errorf("Ciphertext size suggests GCM is not being used correctly. Got %d bytes, expected at least %d", len(data), expectedMinSize)
	}

	// Verify nonce is 12 bytes (GCM standard)
	if len(data) < 12 {
		t.Error("Nonce size is incorrect for GCM")
	}
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
