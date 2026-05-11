package aws

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/models"
)

type CredentialManager struct {
	encryptionKey []byte
	app           *pocketbase.PocketBase
}

type AWSCredentials struct {
	AccessKeyID      string `json:"accessKeyId"`
	SecretAccessKey  string `json:"secretAccessKey"`
	SessionToken     string `json:"sessionToken"`
	Region           string `json:"region"`
	// StateBucketName and StateDynamoTable are the user's own S3 bucket and
	// DynamoDB table for Terraform remote state (BYOC — Bring Your Own Cloud).
	// Required for terraform init to succeed.
	StateBucketName  string `json:"stateBucketName"`
	StateDynamoTable string `json:"stateDynamoTable"`
}


type ValidationResult struct {
	Valid     bool
	AccountID string
	UserArn   string
	Error     error
}

func NewCredentialManager(encryptionKey []byte, app *pocketbase.PocketBase) *CredentialManager {
	return &CredentialManager{
		encryptionKey: encryptionKey,
		app:           app,
	}
}

// EncryptValue encrypts a string value using AES-256-GCM
func (cm *CredentialManager) EncryptValue(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(cm.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptValue decrypts a string value using AES-256-GCM
func (cm *CredentialManager) DecryptValue(encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	block, err := aes.NewCipher(cm.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

// StoreCredentials encrypts and stores AWS credentials for a user.
// StateBucketName and StateDynamoTable are stored unencrypted — they are not
// secrets, they are infrastructure identifiers needed to configure the S3 backend.
func (cm *CredentialManager) StoreCredentials(userID string, creds AWSCredentials) error {
	// Encrypt sensitive values.
	encryptedAccessKey, err := cm.EncryptValue(creds.AccessKeyID)
	if err != nil {
		return fmt.Errorf("failed to encrypt access key: %w", err)
	}

	encryptedSecretKey, err := cm.EncryptValue(creds.SecretAccessKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt secret key: %w", err)
	}

	encryptedSessionToken, err := cm.EncryptValue(creds.SessionToken)
	if err != nil {
		return fmt.Errorf("failed to encrypt session token: %w", err)
	}

	// Look up the collection first — if it doesn't exist, fail fast and clearly.
	collection, err := cm.app.Dao().FindCollectionByNameOrId("awsCredentials")
	if err != nil {
		return fmt.Errorf("failed to find awsCredentials collection: %w", err)
	}

	// Check if credentials already exist for this user.
	// Use a separate variable so the "not found" case (nil record, nil error) is
	// not confused with a real DB error.
	existingRecord, lookupErr := cm.app.Dao().FindFirstRecordByFilter(
		"awsCredentials",
		"user = {:user}",
		map[string]any{"user": userID},
	)

	var record *models.Record
	if lookupErr == nil && existingRecord != nil {
		// Update the existing record in-place.
		record = existingRecord
	} else {
		// Create a new record (either no existing creds or DB returned "not found").
		record = models.NewRecord(collection)
		record.Set("user", userID)
	}

	// Set encrypted credential values.
	record.Set("accessKeyId", encryptedAccessKey)
	record.Set("secretAccessKey", encryptedSecretKey)
	record.Set("sessionToken", encryptedSessionToken)
	record.Set("region", creds.Region)
	record.Set("validated", false)

	// Store Terraform state backend configuration.
	// These are required for terraform init — if missing, every deploy fails.
	// They are infrastructure identifiers, not secrets, so stored in plaintext.
	record.Set("stateBucketName", creds.StateBucketName)
	record.Set("stateDynamoTable", creds.StateDynamoTable)

	return cm.app.Dao().SaveRecord(record)
}


// GetCredentials retrieves and decrypts AWS credentials for a user
func (cm *CredentialManager) GetCredentials(userID string) (*AWSCredentials, error) {
	record, err := cm.app.Dao().FindFirstRecordByFilter(
		"awsCredentials",
		"user = {:user}",
		map[string]any{"user": userID},
	)
	if err != nil {
		return nil, fmt.Errorf("credentials not found for user: %w", err)
	}

	// Decrypt values
	accessKeyID, err := cm.DecryptValue(record.GetString("accessKeyId"))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt access key: %w", err)
	}

	secretAccessKey, err := cm.DecryptValue(record.GetString("secretAccessKey"))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secret key: %w", err)
	}

	sessionToken, err := cm.DecryptValue(record.GetString("sessionToken"))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt session token: %w", err)
	}

	return &AWSCredentials{
		AccessKeyID:      accessKeyID,
		SecretAccessKey:  secretAccessKey,
		SessionToken:     sessionToken,
		Region:           record.GetString("region"),
		StateBucketName:  record.GetString("stateBucketName"),
		StateDynamoTable: record.GetString("stateDynamoTable"),
	}, nil
}

// GetConfigForUser returns an AWS SDK config for a specific user
func (cm *CredentialManager) GetConfigForUser(ctx context.Context, userID string) (aws.Config, error) {
	creds, err := cm.GetCredentials(userID)
	if err != nil {
		return aws.Config{}, err
	}

	return config.LoadDefaultConfig(
		ctx,
		config.WithRegion(creds.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			creds.AccessKeyID,
			creds.SecretAccessKey,
			creds.SessionToken,
		)),
	)
}

// ValidateCredentials tests AWS credentials by making an STS GetCallerIdentity call
func (cm *CredentialManager) ValidateCredentials(creds AWSCredentials) (*ValidationResult, error) {
	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion(creds.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			creds.AccessKeyID,
			creds.SecretAccessKey,
			creds.SessionToken,
		)),
	)
	if err != nil {
		return &ValidationResult{
			Valid: false,
			Error: fmt.Errorf("failed to load AWS config: %w", err),
		}, nil
	}

	stsClient := sts.NewFromConfig(cfg)
	
	result, err := stsClient.GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{})
	if err != nil {
		return &ValidationResult{
			Valid: false,
			Error: fmt.Errorf("AWS credentials validation failed: %w", err),
		}, nil
	}

	return &ValidationResult{
		Valid:     true,
		AccountID: aws.ToString(result.Account),
		UserArn:   aws.ToString(result.Arn),
		Error:     nil,
	}, nil
}

// UpdateValidationStatus updates the validation status of stored credentials
func (cm *CredentialManager) UpdateValidationStatus(userID string, valid bool) error {
	record, err := cm.app.Dao().FindFirstRecordByFilter(
		"awsCredentials",
		"user = {:user}",
		map[string]any{"user": userID},
	)
	if err != nil {
		return fmt.Errorf("credentials not found for user: %w", err)
	}

	record.Set("validated", valid)
	record.Set("lastValidated", time.Now())

	return cm.app.Dao().SaveRecord(record)
}

// ParseCredentialsFromCSV parses AWS credentials from a CSV file (rootkey.csv format)
func (cm *CredentialManager) ParseCredentialsFromCSV(csvContent io.Reader, defaultRegion string) (*AWSCredentials, error) {
	reader := csv.NewReader(csvContent)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV: %w", err)
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("CSV file must contain at least a header and one data row")
	}

	// Find column indices
	header := records[0]
	accessKeyIndex := -1
	secretKeyIndex := -1
	sessionTokenIndex := -1

	for i, col := range header {
		col = strings.TrimSpace(strings.ToLower(col))
		switch col {
		case "access key id", "accesskeyid", "access_key_id":
			accessKeyIndex = i
		case "secret access key", "secretaccesskey", "secret_access_key":
			secretKeyIndex = i
		case "session token", "sessiontoken", "session_token":
			sessionTokenIndex = i
		}
	}

	if accessKeyIndex == -1 || secretKeyIndex == -1 {
		return nil, fmt.Errorf("CSV must contain 'Access Key ID' and 'Secret Access Key' columns")
	}

	// Parse first data row
	if len(records[1]) <= accessKeyIndex || len(records[1]) <= secretKeyIndex {
		return nil, fmt.Errorf("invalid CSV data row")
	}

	creds := &AWSCredentials{
		AccessKeyID:     strings.TrimSpace(records[1][accessKeyIndex]),
		SecretAccessKey: strings.TrimSpace(records[1][secretKeyIndex]),
		Region:          defaultRegion,
	}

	// Add session token if present
	if sessionTokenIndex != -1 && len(records[1]) > sessionTokenIndex {
		creds.SessionToken = strings.TrimSpace(records[1][sessionTokenIndex])
	}

	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return nil, fmt.Errorf("access key ID and secret access key cannot be empty")
	}

	return creds, nil
}

// DeleteCredentials removes stored AWS credentials for a user
func (cm *CredentialManager) DeleteCredentials(userID string) error {
	record, err := cm.app.Dao().FindFirstRecordByFilter(
		"awsCredentials",
		"user = {:user}",
		map[string]any{"user": userID},
	)
	if err != nil {
		return fmt.Errorf("credentials not found for user: %w", err)
	}

	return cm.app.Dao().DeleteRecord(record)
}

// HasValidCredentials checks if a user has valid AWS credentials
func (cm *CredentialManager) HasValidCredentials(userID string) (bool, error) {
	record, err := cm.app.Dao().FindFirstRecordByFilter(
		"awsCredentials",
		"user = {:user}",
		map[string]any{"user": userID},
	)
	if err != nil {
		return false, nil // No credentials found
	}

	return record.GetBool("validated"), nil
}