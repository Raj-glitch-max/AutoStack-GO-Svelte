package controller

import (
	"context"
	"log"
	"time"

	"github.com/janlauber/one-click/pkg/providers"
	"github.com/janlauber/one-click/pkg/secrets"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/models"
)

// authUserFromContext returns the authenticated user record from the echo
// context. apis.RequireRecordAuth("users") stores it under
// apis.ContextAuthRecordKey ("authRecord"). Older code in this file used
// c.Get("user"), which is the wrong key and made every cloud-account
// endpoint reject all requests with HTTP 401.
func authUserFromContext(c echo.Context) *models.Record {
	if r, ok := c.Get(apis.ContextAuthRecordKey).(*models.Record); ok {
		return r
	}
	return nil
}

// decryptCredentials reads the credentials_encrypted field from a
// cloud_accounts record and returns the plaintext blob suitable for
// passing to a Provider. Legacy plaintext rows (written before Phase 2.0
// encryption landed) are returned as-is — see pkg/secrets.Decrypt. A
// failure to decrypt a v1 ciphertext (key mismatch, tamper) returns an
// error; callers MUST refuse the operation rather than guess.
func decryptCredentials(record *models.Record) (string, error) {
	return secrets.Decrypt(record.GetString("credentials_encrypted"))
}

// HandleCloudAccountCreate creates a new cloud account
func HandleCloudAccountCreate(c echo.Context, app *pocketbase.PocketBase) error {
	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}

	var req struct {
		Name            string `json:"name"`
		Provider        string `json:"provider"`
		Region          string `json:"region"`
		CredentialsJSON string `json:"credentials_json"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request"})
	}

	if req.Name == "" || req.Provider == "" {
		return c.JSON(400, map[string]string{"error": "name and provider are required"})
	}

	// Validate provider
	if req.Provider != "aws" && req.Provider != "gcp" && req.Provider != "azure" {
		return c.JSON(400, map[string]string{"error": "invalid provider"})
	}

	// Create the cloud account record
	collection, err := app.Dao().FindCollectionByNameOrId("cloud_accounts")
	if err != nil {
		return c.JSON(500, map[string]string{"error": "collection not found"})
	}

	// Encrypt credentials at the write boundary. If the encryption key is
	// missing or malformed, refuse to persist the account — silently storing
	// plaintext would re-introduce the very gap pkg/secrets exists to close.
	encrypted, err := secrets.Encrypt(req.CredentialsJSON)
	if err != nil {
		log.Printf("Failed to encrypt credentials: %v", err)
		return c.JSON(500, map[string]string{"error": "encryption not configured"})
	}

	record := models.NewRecord(collection)
	record.Set("name", req.Name)
	record.Set("user", user.Id)
	record.Set("provider", req.Provider)
	record.Set("region", req.Region)
	record.Set("credentials_encrypted", encrypted)
	record.Set("status", "validating")

	// Validate credentials immediately
	ctx := context.Background()
	var providerName string
	switch req.Provider {
	case "gcp":
		providerName = providers.ProviderGCPCloudRun
	case "aws":
		providerName = providers.ProviderAWSECS
	case "azure":
		providerName = providers.ProviderAzureACA
	}

	p, err := providers.GetProvider(providerName)
	if err != nil {
		record.Set("status", "error")
		record.Set("validation_error", "provider not available")
	} else {
		account := &providers.CloudAccount{
			ID:                   "",
			Name:                 req.Name,
			Provider:             req.Provider,
			Region:               req.Region,
			CredentialsEncrypted: req.CredentialsJSON,
		}

		if err := p.ValidateCredentials(ctx, account); err != nil {
			record.Set("status", "error")
			record.Set("validation_error", err.Error())
		} else {
			record.Set("status", "active")
			record.Set("validated_at", time.Now().UTC().Format(time.RFC3339))
		}
	}

	record.Set("last_validated", time.Now().UTC().Format(time.RFC3339))

	if err := app.Dao().SaveRecord(record); err != nil {
		log.Printf("Failed to save cloud account: %v", err)
		return c.JSON(500, map[string]string{"error": "failed to save cloud account"})
	}

	return c.JSON(201, sanitizeCloudAccountResponse(record))
}

// sanitizeCloudAccountResponse returns the cloud_accounts record fields safe
// to send back to a client. Never include credentials_encrypted in any HTTP
// response: that field stores the raw credential blob and exposing it would
// negate the entire reason cloud accounts are a separate collection.
func sanitizeCloudAccountResponse(record *models.Record) map[string]interface{} {
	return map[string]interface{}{
		"id":               record.Id,
		"name":             record.GetString("name"),
		"user":             record.GetString("user"),
		"provider":         record.GetString("provider"),
		"region":           record.GetString("region"),
		"status":           record.GetString("status"),
		"validation_error": record.GetString("validation_error"),
		"validated_at":     record.GetString("validated_at"),
		"last_validated":   record.GetString("last_validated"),
		"created":          record.GetString("created"),
		"updated":          record.GetString("updated"),
	}
}

// HandleCloudAccountValidate validates cloud account credentials
func HandleCloudAccountValidate(c echo.Context, app *pocketbase.PocketBase, accountID string) error {
	record, err := app.Dao().FindRecordById("cloud_accounts", accountID)
	if err != nil {
		return c.JSON(404, map[string]string{"error": "cloud account not found"})
	}

	// Get the user from context
	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}

	// Check ownership
	if record.GetString("user") != user.Id {
		return c.JSON(403, map[string]string{"error": "forbidden"})
	}

	provider := record.GetString("provider")
	region := record.GetString("region")
	credentials, err := decryptCredentials(record)
	if err != nil {
		log.Printf("Failed to decrypt credentials for account %s: %v", record.Id, err)
		return c.JSON(500, map[string]string{"error": "credential decryption failed"})
	}

	var providerName string
	switch provider {
	case "gcp":
		providerName = providers.ProviderGCPCloudRun
	case "aws":
		providerName = providers.ProviderAWSECS
	case "azure":
		providerName = providers.ProviderAzureACA
	default:
		return c.JSON(400, map[string]string{"error": "invalid provider"})
	}

	p, err := providers.GetProvider(providerName)
	if err != nil {
		return c.JSON(500, map[string]string{"error": "provider not available"})
	}

	account := &providers.CloudAccount{
		ID:                   record.Id,
		Name:                 record.GetString("name"),
		Provider:             provider,
		Region:               region,
		CredentialsEncrypted: credentials,
	}

	ctx := context.Background()
	if err := p.ValidateCredentials(ctx, account); err != nil {
		record.Set("status", "error")
		record.Set("validation_error", err.Error())
	} else {
		record.Set("status", "active")
		record.Set("validation_error", "")
		record.Set("validated_at", time.Now().UTC().Format(time.RFC3339))
	}

	// Lazy migration: if the stored value was legacy plaintext, re-encrypt
	// it on this validation pass. This lets pre-Phase-2.0 records reach an
	// encrypted-at-rest state without a one-shot migration script.
	if !secrets.IsEncrypted(record.GetString("credentials_encrypted")) && credentials != "" {
		if encrypted, encErr := secrets.Encrypt(credentials); encErr == nil {
			record.Set("credentials_encrypted", encrypted)
			log.Printf("[CRED_REENCRYPT] account=%s key_fp=%s", record.Id, secrets.KeyFingerprint())
		}
	}

	record.Set("last_validated", time.Now().UTC().Format(time.RFC3339))

	if err := app.Dao().SaveRecord(record); err != nil {
		return c.JSON(500, map[string]string{"error": "failed to update"})
	}

	return c.JSON(200, sanitizeCloudAccountResponse(record))
}

// HandleCloudAccountList lists cloud accounts for the current user
func HandleCloudAccountList(c echo.Context, app *pocketbase.PocketBase) error {
	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}

	records, err := app.Dao().FindRecordsByFilter("cloud_accounts", "user = {:user}", "-created", 100, 0, map[string]interface{}{
		"user": user.Id,
	})
	if err != nil {
		return c.JSON(500, map[string]string{"error": "failed to list accounts"})
	}

	sanitized := make([]map[string]interface{}, 0, len(records))
	for _, r := range records {
		sanitized = append(sanitized, sanitizeCloudAccountResponse(r))
	}
	return c.JSON(200, sanitized)
}

// HandleCloudAccountRegions lists available regions for a cloud account
func HandleCloudAccountRegions(c echo.Context, app *pocketbase.PocketBase, accountID string) error {
	record, err := app.Dao().FindRecordById("cloud_accounts", accountID)
	if err != nil {
		return c.JSON(404, map[string]string{"error": "cloud account not found"})
	}

	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}

	if record.GetString("user") != user.Id {
		return c.JSON(403, map[string]string{"error": "forbidden"})
	}

	provider := record.GetString("provider")
	var providerName string
	switch provider {
	case "gcp":
		providerName = providers.ProviderGCPCloudRun
	case "aws":
		providerName = providers.ProviderAWSECS
	case "azure":
		providerName = providers.ProviderAzureACA
	default:
		return c.JSON(400, map[string]string{"error": "invalid provider"})
	}

	p, err := providers.GetProvider(providerName)
	if err != nil {
		return c.JSON(500, map[string]string{"error": "provider not available"})
	}

	credentials, err := decryptCredentials(record)
	if err != nil {
		log.Printf("Failed to decrypt credentials for account %s: %v", record.Id, err)
		return c.JSON(500, map[string]string{"error": "credential decryption failed"})
	}
	account := &providers.CloudAccount{
		ID:                   record.Id,
		Provider:             provider,
		Region:               record.GetString("region"),
		CredentialsEncrypted: credentials,
	}

	ctx := context.Background()
	regions, err := p.ListRegions(ctx, account)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}

	return c.JSON(200, map[string][]string{"regions": regions})
}

// HandleCostEstimate estimates cost for a deployment
func HandleCostEstimate(c echo.Context, app *pocketbase.PocketBase) error {
	user := authUserFromContext(c)
	if user == nil {
		return c.JSON(401, map[string]string{"error": "unauthorized"})
	}

	var req struct {
		CloudAccountID string              `json:"cloud_account_id"`
		Spec           providers.DeploySpec `json:"spec"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request"})
	}

	record, err := app.Dao().FindRecordById("cloud_accounts", req.CloudAccountID)
	if err != nil {
		return c.JSON(404, map[string]string{"error": "cloud account not found"})
	}

	if record.GetString("user") != user.Id {
		return c.JSON(403, map[string]string{"error": "forbidden"})
	}

	provider := record.GetString("provider")
	var providerName string
	switch provider {
	case "gcp":
		providerName = providers.ProviderGCPCloudRun
	case "aws":
		providerName = providers.ProviderAWSECS
	case "azure":
		providerName = providers.ProviderAzureACA
	default:
		return c.JSON(400, map[string]string{"error": "invalid provider"})
	}

	p, err := providers.GetProvider(providerName)
	if err != nil {
		return c.JSON(500, map[string]string{"error": "provider not available"})
	}

	credentials, err := decryptCredentials(record)
	if err != nil {
		log.Printf("Failed to decrypt credentials for account %s: %v", record.Id, err)
		return c.JSON(500, map[string]string{"error": "credential decryption failed"})
	}
	account := &providers.CloudAccount{
		ID:                   record.Id,
		Provider:             provider,
		Region:               record.GetString("region"),
		CredentialsEncrypted: credentials,
	}

	ctx := context.Background()
	estimate, err := p.EstimateCost(ctx, account, &req.Spec)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}

	return c.JSON(200, estimate)
}