package azure

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
	"net/url"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/models"
)

// AzureCredentials holds Azure Service Principal credentials
type AzureCredentials struct {
	ClientID             string `json:"clientId"`
	ClientSecret         string `json:"clientSecret"`
	TenantID             string `json:"tenantId"`
	SubscriptionID       string `json:"subscriptionId"`
	Region               string `json:"region"`
	StateContainerName   string `json:"stateContainerName"`
	StateStorageAccount  string `json:"stateStorageAccount"`
}

// ValidationResult mirrors the AWS/GCP pattern
type ValidationResult struct {
	Valid          bool
	SubscriptionID string
	TenantID       string
	Error          error
}

// CredentialManager stores/retrieves Azure credentials with AES-256-GCM encryption
type CredentialManager struct {
	encKey []byte
	app    *pocketbase.PocketBase
}

func NewCredentialManager(encKey []byte, app *pocketbase.PocketBase) *CredentialManager {
	return &CredentialManager{encKey: encKey, app: app}
}

// StoreCredentials encrypts sensitive fields and persists them
func (cm *CredentialManager) StoreCredentials(userID string, creds AzureCredentials) error {
	encSecret, err := cm.encrypt(creds.ClientSecret)
	if err != nil {
		return fmt.Errorf("failed to encrypt client secret: %w", err)
	}

	collection, err := cm.app.Dao().FindCollectionByNameOrId("azureCredentials")
	if err != nil {
		return fmt.Errorf("azureCredentials collection not found: %w", err)
	}

	existing, _ := cm.app.Dao().FindFirstRecordByFilter(
		"azureCredentials", "user = {:u}", map[string]any{"u": userID},
	)

	var record *models.Record
	if existing != nil {
		record = existing
	} else {
		record = models.NewRecord(collection)
		record.Set("user", userID)
	}

	record.Set("clientId", creds.ClientID)
	record.Set("clientSecret", encSecret)
	record.Set("tenantId", creds.TenantID)
	record.Set("subscriptionId", creds.SubscriptionID)
	record.Set("region", creds.Region)
	record.Set("stateContainerName", creds.StateContainerName)
	record.Set("stateStorageAccount", creds.StateStorageAccount)
	record.Set("validated", false)

	return cm.app.Dao().SaveRecord(record)
}

// GetCredentials retrieves and decrypts Azure credentials for a user
func (cm *CredentialManager) GetCredentials(userID string) (*AzureCredentials, error) {
	record, err := cm.app.Dao().FindFirstRecordByFilter(
		"azureCredentials", "user = {:u}", map[string]any{"u": userID},
	)
	if err != nil {
		return nil, fmt.Errorf("Azure credentials not found: %w", err)
	}

	secret, err := cm.decrypt(record.GetString("clientSecret"))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt client secret: %w", err)
	}

	return &AzureCredentials{
		ClientID:            record.GetString("clientId"),
		ClientSecret:        secret,
		TenantID:            record.GetString("tenantId"),
		SubscriptionID:      record.GetString("subscriptionId"),
		Region:              record.GetString("region"),
		StateContainerName:  record.GetString("stateContainerName"),
		StateStorageAccount: record.GetString("stateStorageAccount"),
	}, nil
}

// ValidateCredentials calls the Azure Resource Manager API to verify the Service Principal
func (cm *CredentialManager) ValidateCredentials(creds AzureCredentials) (*ValidationResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Get access token via client credentials flow
	token, err := cm.getAccessToken(ctx, creds)
	if err != nil {
		return &ValidationResult{Valid: false, Error: fmt.Errorf("authentication failed: %w", err)}, nil
	}

	// Verify subscription access
	apiURL := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s?api-version=2020-01-01",
		creds.SubscriptionID,
	)
	req, _ := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &ValidationResult{Valid: false, Error: fmt.Errorf("Azure API request failed: %w", err)}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &ValidationResult{Valid: false, Error: fmt.Errorf("Azure subscription access denied (status %d)", resp.StatusCode)}, nil
	}

	return &ValidationResult{
		Valid:          true,
		SubscriptionID: creds.SubscriptionID,
		TenantID:       creds.TenantID,
	}, nil
}

// getAccessToken fetches an Azure AD access token via client credentials flow
func (cm *CredentialManager) getAccessToken(ctx context.Context, creds AzureCredentials) (string, error) {
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", creds.TenantID)

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", creds.ClientID)
	form.Set("client_secret", creds.ClientSecret)
	form.Set("scope", "https://management.azure.com/.default")

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(form.Encode()))
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
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}
	if tokenResp.Error != "" {
		return "", fmt.Errorf("%s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}
	return tokenResp.AccessToken, nil
}

// UpdateValidationStatus marks credentials as validated
func (cm *CredentialManager) UpdateValidationStatus(userID string, valid bool) error {
	record, err := cm.app.Dao().FindFirstRecordByFilter(
		"azureCredentials", "user = {:u}", map[string]any{"u": userID},
	)
	if err != nil {
		return err
	}
	record.Set("validated", valid)
	record.Set("lastValidated", time.Now())
	return cm.app.Dao().SaveRecord(record)
}

// HasValidCredentials checks if a user has validated Azure credentials
func (cm *CredentialManager) HasValidCredentials(userID string) bool {
	record, err := cm.app.Dao().FindFirstRecordByFilter(
		"azureCredentials", "user = {:u}", map[string]any{"u": userID},
	)
	if err != nil {
		return false
	}
	return record.GetBool("validated")
}

// DeleteCredentials removes Azure credentials for a user
func (cm *CredentialManager) DeleteCredentials(userID string) error {
	record, err := cm.app.Dao().FindFirstRecordByFilter(
		"azureCredentials", "user = {:u}", map[string]any{"u": userID},
	)
	if err != nil {
		return err
	}
	return cm.app.Dao().DeleteRecord(record)
}

// encrypt uses AES-256-GCM
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
