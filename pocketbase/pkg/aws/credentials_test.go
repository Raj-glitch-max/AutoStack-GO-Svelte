package aws

import (
	"crypto/rand"
	"strings"
	"testing"
)

func TestCredentialEncryptionDecryption(t *testing.T) {
	// Generate a test encryption key
	encryptionKey := make([]byte, 32)
	if _, err := rand.Read(encryptionKey); err != nil {
		t.Fatal("Failed to generate encryption key:", err)
	}

	credManager := &CredentialManager{
		encryptionKey: encryptionKey,
	}

	testCases := []struct {
		name      string
		plaintext string
	}{
		{"Empty string", ""},
		{"Simple text", "hello world"},
		{"AWS Access Key", "AKIAIOSFODNN7EXAMPLE"},
		{"AWS Secret Key", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"},
		{"Special characters", "!@#$%^&*()_+-=[]{}|;':\",./<>?"},
		{"Unicode", "Hello 世界 🌍"},
		{"Long text", "This is a very long string that contains multiple words and should test the encryption and decryption of longer content to ensure it works properly with various lengths of input data."},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test encryption
			encrypted, err := credManager.EncryptValue(tc.plaintext)
			if err != nil {
				t.Fatalf("Encryption failed: %v", err)
			}

			// Empty strings should return empty
			if tc.plaintext == "" {
				if encrypted != "" {
					t.Errorf("Expected empty encrypted string for empty input, got: %s", encrypted)
				}
				return
			}

			// Encrypted value should be different from plaintext
			if encrypted == tc.plaintext {
				t.Error("Encrypted value should be different from plaintext")
			}

			// Test decryption
			decrypted, err := credManager.DecryptValue(encrypted)
			if err != nil {
				t.Fatalf("Decryption failed: %v", err)
			}

			// Decrypted value should match original
			if decrypted != tc.plaintext {
				t.Errorf("Decrypted value doesn't match original. Expected: %s, Got: %s", tc.plaintext, decrypted)
			}
		})
	}
}

func TestCredentialEncryptionConsistency(t *testing.T) {
	// Generate a test encryption key
	encryptionKey := make([]byte, 32)
	if _, err := rand.Read(encryptionKey); err != nil {
		t.Fatal("Failed to generate encryption key:", err)
	}

	credManager := &CredentialManager{
		encryptionKey: encryptionKey,
	}

	plaintext := "AKIAIOSFODNN7EXAMPLE"

	// Encrypt the same value multiple times
	encrypted1, err := credManager.EncryptValue(plaintext)
	if err != nil {
		t.Fatal("First encryption failed:", err)
	}

	encrypted2, err := credManager.EncryptValue(plaintext)
	if err != nil {
		t.Fatal("Second encryption failed:", err)
	}

	// Encrypted values should be different (due to random nonce)
	if encrypted1 == encrypted2 {
		t.Error("Multiple encryptions of the same value should produce different results")
	}

	// But both should decrypt to the same original value
	decrypted1, err := credManager.DecryptValue(encrypted1)
	if err != nil {
		t.Fatal("First decryption failed:", err)
	}

	decrypted2, err := credManager.DecryptValue(encrypted2)
	if err != nil {
		t.Fatal("Second decryption failed:", err)
	}

	if decrypted1 != plaintext || decrypted2 != plaintext {
		t.Error("Both decrypted values should match the original plaintext")
	}
}

func TestCredentialEncryptionWithDifferentKeys(t *testing.T) {
	// Generate two different encryption keys
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	
	if _, err := rand.Read(key1); err != nil {
		t.Fatal("Failed to generate first key:", err)
	}
	if _, err := rand.Read(key2); err != nil {
		t.Fatal("Failed to generate second key:", err)
	}

	credManager1 := &CredentialManager{encryptionKey: key1}
	credManager2 := &CredentialManager{encryptionKey: key2}

	plaintext := "AKIAIOSFODNN7EXAMPLE"

	// Encrypt with first key
	encrypted, err := credManager1.EncryptValue(plaintext)
	if err != nil {
		t.Fatal("Encryption failed:", err)
	}

	// Try to decrypt with second key (should fail)
	_, err = credManager2.DecryptValue(encrypted)
	if err == nil {
		t.Error("Decryption with wrong key should fail")
	}

	// Decrypt with correct key (should succeed)
	decrypted, err := credManager1.DecryptValue(encrypted)
	if err != nil {
		t.Fatal("Decryption with correct key failed:", err)
	}

	if decrypted != plaintext {
		t.Error("Decrypted value doesn't match original")
	}
}

func TestParseCredentialsFromCSV(t *testing.T) {
	credManager := &CredentialManager{}

	testCases := []struct {
		name        string
		csvContent  string
		expectError bool
		expectedKey string
		expectedSecret string
	}{
		{
			name: "Standard AWS CSV format",
			csvContent: `Access key ID,Secret access key
AKIAIOSFODNN7EXAMPLE,wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`,
			expectError: false,
			expectedKey: "AKIAIOSFODNN7EXAMPLE",
			expectedSecret: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		},
		{
			name: "Alternative column names",
			csvContent: `AccessKeyId,SecretAccessKey
AKIAIOSFODNN7EXAMPLE,wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`,
			expectError: false,
			expectedKey: "AKIAIOSFODNN7EXAMPLE",
			expectedSecret: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		},
		{
			name: "With session token",
			csvContent: `Access key ID,Secret access key,Session token
AKIAIOSFODNN7EXAMPLE,wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY,FwoGZXIvYXdzEBQaDH`,
			expectError: false,
			expectedKey: "AKIAIOSFODNN7EXAMPLE",
			expectedSecret: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		},
		{
			name: "Missing secret key column",
			csvContent: `Access key ID,Other column
AKIAIOSFODNN7EXAMPLE,some value`,
			expectError: true,
		},
		{
			name: "Empty CSV",
			csvContent: ``,
			expectError: true,
		},
		{
			name: "Only header",
			csvContent: `Access key ID,Secret access key`,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader := strings.NewReader(tc.csvContent)
			creds, err := credManager.ParseCredentialsFromCSV(reader, "us-east-1")

			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if creds.AccessKeyID != tc.expectedKey {
				t.Errorf("Expected access key %s, got %s", tc.expectedKey, creds.AccessKeyID)
			}

			if creds.SecretAccessKey != tc.expectedSecret {
				t.Errorf("Expected secret key %s, got %s", tc.expectedSecret, creds.SecretAccessKey)
			}

			if creds.Region != "us-east-1" {
				t.Errorf("Expected region us-east-1, got %s", creds.Region)
			}
		})
	}
}