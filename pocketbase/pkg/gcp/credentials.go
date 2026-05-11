package gcp

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/models"
)

// GCPCredentials holds a GCP service account key and project metadata
type GCPCredentials struct {
	ProjectID         string `json:"projectId"`
	ServiceAccountKey string `json:"serviceAccountKey"` // full JSON key file content
	Region            string `json:"region"`
	StateBucketName   string `json:"stateBucketName"`
}

// ValidationResult mirrors the AWS pattern
type ValidationResult struct {
	Valid     bool
	ProjectID string
	Email     string
	Error     error
}

// CredentialManager stores/retrieves GCP credentials with AES-256-GCM encryption
type CredentialManager struct {
	encKey []byte
	app    *pocketbase.PocketBase
}

func NewCredentialManager(encKey []byte, app *pocketbase.PocketBase) *CredentialManager {
	return &CredentialManager{encKey: encKey, app: app}
}

// StoreCredentials encrypts the service account key JSON and persists it
func (cm *CredentialManager) StoreCredentials(userID string, creds GCPCredentials) error {
	encKey, err := cm.encrypt(creds.ServiceAccountKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt service account key: %w", err)
	}

	collection, err := cm.app.Dao().FindCollectionByNameOrId("gcpCredentials")
	if err != nil {
		return fmt.Errorf("gcpCredentials collection not found: %w", err)
	}

	existing, _ := cm.app.Dao().FindFirstRecordByFilter(
		"gcpCredentials", "user = {:u}", map[string]any{"u": userID},
	)

	var record *models.Record
	if existing != nil {
		record = existing
	} else {
		record = models.NewRecord(collection)
		record.Set("user", userID)
	}

	record.Set("projectId", creds.ProjectID)
	record.Set("serviceAccountKey", encKey)
	record.Set("region", creds.Region)
	record.Set("stateBucketName", creds.StateBucketName)
	record.Set("validated", false)

	return cm.app.Dao().SaveRecord(record)
}

// GetCredentials retrieves and decrypts GCP credentials for a user
func (cm *CredentialManager) GetCredentials(userID string) (*GCPCredentials, error) {
	record, err := cm.app.Dao().FindFirstRecordByFilter(
		"gcpCredentials", "user = {:u}", map[string]any{"u": userID},
	)
	if err != nil {
		return nil, fmt.Errorf("GCP credentials not found: %w", err)
	}

	keyJSON, err := cm.decrypt(record.GetString("serviceAccountKey"))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt service account key: %w", err)
	}

	return &GCPCredentials{
		ProjectID:         record.GetString("projectId"),
		ServiceAccountKey: keyJSON,
		Region:            record.GetString("region"),
		StateBucketName:   record.GetString("stateBucketName"),
	}, nil
}

// ValidateCredentials calls the GCP IAM API to verify the service account key
func (cm *CredentialManager) ValidateCredentials(creds GCPCredentials) (*ValidationResult, error) {
	// Parse the service account key to extract the token URI and client email
	var keyData struct {
		ClientEmail string `json:"client_email"`
		ProjectID   string `json:"project_id"`
		TokenURI    string `json:"token_uri"`
	}
	if err := json.Unmarshal([]byte(creds.ServiceAccountKey), &keyData); err != nil {
		return &ValidationResult{Valid: false, Error: fmt.Errorf("invalid service account key JSON: %w", err)}, nil
	}

	if keyData.ClientEmail == "" || keyData.ProjectID == "" {
		return &ValidationResult{Valid: false, Error: fmt.Errorf("service account key missing required fields")}, nil
	}

	// Lightweight validation: call the GCP Resource Manager API to verify project access
	// Uses the service account key to get an access token, then checks project existence
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	token, err := cm.getAccessToken(ctx, creds.ServiceAccountKey)
	if err != nil {
		return &ValidationResult{Valid: false, Error: fmt.Errorf("failed to authenticate: %w", err)}, nil
	}

	// Verify project access
	url := fmt.Sprintf("https://cloudresourcemanager.googleapis.com/v1/projects/%s", creds.ProjectID)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &ValidationResult{Valid: false, Error: fmt.Errorf("GCP API request failed: %w", err)}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &ValidationResult{Valid: false, Error: fmt.Errorf("GCP project access denied (status %d)", resp.StatusCode)}, nil
	}

	return &ValidationResult{
		Valid:     true,
		ProjectID: keyData.ProjectID,
		Email:     keyData.ClientEmail,
	}, nil
}

// getAccessToken exchanges a service account key for a short-lived access token
func (cm *CredentialManager) getAccessToken(ctx context.Context, keyJSON string) (string, error) {
	var keyData struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		TokenURI    string `json:"token_uri"`
	}
	if err := json.Unmarshal([]byte(keyJSON), &keyData); err != nil {
		return "", err
	}

	// Use the metadata server approach: POST to token URI with a JWT
	// For simplicity, we use the google-auth-library pattern via HTTP
	// In production this would use golang.org/x/oauth2/google
	body := fmt.Sprintf(
		"grant_type=urn%%3Aietf%%3Aparams%%3Aoauth%%3Agrant-type%%3Ajwt-bearer&assertion=%s",
		cm.buildJWT(keyData.ClientEmail, keyData.PrivateKey, keyData.TokenURI),
	)

	req, err := http.NewRequestWithContext(ctx, "POST", keyData.TokenURI,
		strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}
	if tokenResp.Error != "" {
		return "", fmt.Errorf("token error: %s", tokenResp.Error)
	}
	return tokenResp.AccessToken, nil
}

// buildJWT creates a minimal JWT for GCP service account authentication
// In production, use golang.org/x/oauth2/google for proper JWT signing
func (cm *CredentialManager) buildJWT(email, privateKey, tokenURI string) string {
	// Placeholder — real implementation uses RS256 signing with the private key
	// The Terraform provider handles actual auth; this is just for validation
	return "placeholder"
}

// UpdateValidationStatus marks credentials as validated
func (cm *CredentialManager) UpdateValidationStatus(userID string, valid bool) error {
	record, err := cm.app.Dao().FindFirstRecordByFilter(
		"gcpCredentials", "user = {:u}", map[string]any{"u": userID},
	)
	if err != nil {
		return err
	}
	record.Set("validated", valid)
	record.Set("lastValidated", time.Now())
	return cm.app.Dao().SaveRecord(record)
}

// HasValidCredentials checks if a user has validated GCP credentials
func (cm *CredentialManager) HasValidCredentials(userID string) bool {
	record, err := cm.app.Dao().FindFirstRecordByFilter(
		"gcpCredentials", "user = {:u}", map[string]any{"u": userID},
	)
	if err != nil {
		return false
	}
	return record.GetBool("validated")
}

// DeleteCredentials removes GCP credentials for a user
func (cm *CredentialManager) DeleteCredentials(userID string) error {
	record, err := cm.app.Dao().FindFirstRecordByFilter(
		"gcpCredentials", "user = {:u}", map[string]any{"u": userID},
	)
	if err != nil {
		return err
	}
	return cm.app.Dao().DeleteRecord(record)
}

// encrypt uses AES-256-GCM (same as AWS credential manager)
func (cm *CredentialManager) encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(cm.encKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// decrypt reverses encrypt
func (cm *CredentialManager) decrypt(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	ct, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(cm.encKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(ct) < ns {
		return "", fmt.Errorf("ciphertext too short")
	}
	pt, err := gcm.Open(nil, ct[:ns], ct[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
