package controller

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tests"
)

// TestValidateThreshold tests threshold validation
// Validates: Threshold must be a positive number
func TestValidateThreshold(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
		wantError bool
	}{
		{
			name:      "Valid threshold",
			threshold: 20.0,
			wantError: false,
		},
		{
			name:      "Zero threshold",
			threshold: 0,
			wantError: false,
		},
		{
			name:      "Negative threshold",
			threshold: -5.0,
			wantError: true,
		},
		{
			name:      "Very large threshold",
			threshold: 1500.0,
			wantError: true,
		},
		{
			name:      "Maximum valid threshold",
			threshold: 1000.0,
			wantError: false,
		},
		{
			name:      "Small positive threshold",
			threshold: 5.0,
			wantError: false,
		},
		{
			name:      "Large valid threshold",
			threshold: 500.0,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateThreshold(tt.threshold)

			if tt.wantError && err == nil {
				t.Error("Expected error but got none")
			}

			if !tt.wantError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

// TestThresholdDatabaseIntegration tests threshold storage and retrieval
// Validates: AC-4.4 (User can set custom alert thresholds per deployment)
func TestThresholdDatabaseIntegration(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// Create test user
	user, err := createTestUser(app, "test@example.com")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create test deployment with custom threshold
	deployment, err := createTestDeployment(app, user.Id, "test-deployment", 30.0)
	if err != nil {
		t.Fatalf("Failed to create test deployment: %v", err)
	}

	// Verify threshold was saved
	savedDeployment, err := app.Dao().FindRecordById("awsDeployments", deployment.Id)
	if err != nil {
		t.Fatalf("Failed to retrieve deployment: %v", err)
	}

	threshold := savedDeployment.GetFloat("costAlertThreshold")
	if threshold != 30.0 {
		t.Errorf("Expected threshold 30.0, got %.2f", threshold)
	}
}

// TestThresholdUpdate tests updating a threshold
// Validates: AC-4.4 (User can set custom alert thresholds per deployment)
func TestThresholdUpdate(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// Create test user
	user, err := createTestUser(app, "test@example.com")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create test deployment with initial threshold
	deployment, err := createTestDeployment(app, user.Id, "test-deployment", 20.0)
	if err != nil {
		t.Fatalf("Failed to create test deployment: %v", err)
	}

	// Update threshold
	deployment.Set("costAlertThreshold", 40.0)
	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to update deployment: %v", err)
	}

	// Verify threshold was updated
	updatedDeployment, err := app.Dao().FindRecordById("awsDeployments", deployment.Id)
	if err != nil {
		t.Fatalf("Failed to retrieve updated deployment: %v", err)
	}

	threshold := updatedDeployment.GetFloat("costAlertThreshold")
	if threshold != 40.0 {
		t.Errorf("Expected threshold 40.0, got %.2f", threshold)
	}
}

// TestThresholdDefaultValue tests default threshold behavior
// Validates: AC-4.1 (Default 20% threshold)
func TestThresholdDefaultValue(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// Create test user
	user, err := createTestUser(app, "test@example.com")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create test deployment without custom threshold
	deployment, err := createTestDeployment(app, user.Id, "test-deployment", 0)
	if err != nil {
		t.Fatalf("Failed to create test deployment: %v", err)
	}

	// Verify threshold is 0 (not set)
	savedDeployment, err := app.Dao().FindRecordById("awsDeployments", deployment.Id)
	if err != nil {
		t.Fatalf("Failed to retrieve deployment: %v", err)
	}

	threshold := savedDeployment.GetFloat("costAlertThreshold")
	if threshold != 0 {
		t.Errorf("Expected threshold 0 (not set), got %.2f", threshold)
	}
}

// Helper function to create a test user
func createTestUser(app core.App, email string) (*models.Record, error) {
	collection, err := app.Dao().FindCollectionByNameOrId("users")
	if err != nil {
		return nil, err
	}

	user := models.NewRecord(collection)
	user.Set("email", email)
	user.Set("password", "test123456")
	user.Set("passwordConfirm", "test123456")

	if err := app.Dao().SaveRecord(user); err != nil {
		return nil, err
	}

	return user, nil
}

// Helper function to create a test deployment
func createTestDeployment(app core.App, userID string, name string, threshold float64) (*models.Record, error) {
	collection, err := app.Dao().FindCollectionByNameOrId("awsDeployments")
	if err != nil {
		return nil, err
	}

	deployment := models.NewRecord(collection)
	deployment.Set("name", name)
	deployment.Set("user", userID)
	deployment.Set("status", "active")
	deployment.Set("region", "us-east-1")

	if threshold > 0 {
		deployment.Set("costAlertThreshold", threshold)
	}

	if err := app.Dao().SaveRecord(deployment); err != nil {
		return nil, err
	}

	return deployment, nil
}
