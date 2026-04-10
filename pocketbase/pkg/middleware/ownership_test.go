package middleware

import (
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/models/schema"
)

// createTestRecord creates a test record with a user field
func createTestRecord(userID string) *models.Record {
	collection := &models.Collection{
		Name: "test_collection",
		Schema: schema.NewSchema(
			&schema.SchemaField{
				Name: "user",
				Type: schema.FieldTypeRelation,
			},
		),
	}

	record := models.NewRecord(collection)
	record.Set("user", userID)
	return record
}

// TestAssertOwnershipCorrect verifies that ownership check passes when user matches
func TestAssertOwnershipCorrect(t *testing.T) {
	userID := "user123"
	record := createTestRecord(userID)

	err := AssertOwnership(record, userID)
	if err != nil {
		t.Errorf("Expected ownership check to pass, got error: %v", err)
	}
}

// TestAssertOwnershipWrong verifies that ownership check fails when user doesn't match
func TestAssertOwnershipWrong(t *testing.T) {
	recordUserID := "user123"
	requestUserID := "user456"
	record := createTestRecord(recordUserID)

	err := AssertOwnership(record, requestUserID)
	if err == nil {
		t.Error("Expected ownership check to fail, but it passed")
	}

	if !errors.Is(err, ErrForbidden) {
		t.Errorf("Expected ErrForbidden, got: %v", err)
	}
}

// TestAssertOwnershipNilRecord verifies that nil record is rejected
func TestAssertOwnershipNilRecord(t *testing.T) {
	err := AssertOwnership(nil, "user123")
	if err == nil {
		t.Error("Expected error for nil record, got nil")
	}
}

// TestAssertOwnershipEmptyUserID verifies that empty user ID is rejected
func TestAssertOwnershipEmptyUserID(t *testing.T) {
	record := createTestRecord("user123")

	err := AssertOwnership(record, "")
	if err == nil {
		t.Error("Expected error for empty user ID, got nil")
	}

	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("Expected ErrUnauthorized, got: %v", err)
	}
}

// TestAssertOwnershipMultipleAllValid verifies multiple records with same owner
func TestAssertOwnershipMultipleAllValid(t *testing.T) {
	userID := "user123"
	records := []*models.Record{
		createTestRecord(userID),
		createTestRecord(userID),
		createTestRecord(userID),
	}

	err := AssertOwnershipMultiple(records, userID)
	if err != nil {
		t.Errorf("Expected ownership check to pass for all records, got error: %v", err)
	}
}

// TestAssertOwnershipMultipleOneInvalid verifies that check fails if one record has wrong owner
func TestAssertOwnershipMultipleOneInvalid(t *testing.T) {
	userID := "user123"
	records := []*models.Record{
		createTestRecord(userID),
		createTestRecord("user456"), // wrong owner
		createTestRecord(userID),
	}

	err := AssertOwnershipMultiple(records, userID)
	if err == nil {
		t.Error("Expected ownership check to fail, but it passed")
	}
}

// TestAssertOwnershipMultipleEmpty verifies that empty slice passes
func TestAssertOwnershipMultipleEmpty(t *testing.T) {
	err := AssertOwnershipMultiple([]*models.Record{}, "user123")
	if err != nil {
		t.Errorf("Expected empty slice to pass, got error: %v", err)
	}
}

// TestAssertProjectOwnership verifies project ownership check
func TestAssertProjectOwnership(t *testing.T) {
	userID := "user123"
	projectRecord := createTestRecord(userID)

	err := AssertProjectOwnership(projectRecord, userID)
	if err != nil {
		t.Errorf("Expected project ownership check to pass, got error: %v", err)
	}
}

// TestAssertProjectOwnershipWrongUser verifies project ownership fails with wrong user
func TestAssertProjectOwnershipWrongUser(t *testing.T) {
	projectRecord := createTestRecord("user123")

	err := AssertProjectOwnership(projectRecord, "user456")
	if err == nil {
		t.Error("Expected project ownership check to fail, but it passed")
	}

	if !errors.Is(err, ErrForbidden) {
		t.Errorf("Expected ErrForbidden, got: %v", err)
	}
}

// TestAssertDeploymentOwnershipValid verifies deployment and project ownership
func TestAssertDeploymentOwnershipValid(t *testing.T) {
	userID := "user123"
	projectID := "project456"

	// Create project record
	projectCollection := &models.Collection{
		Name: "projects",
		Schema: schema.NewSchema(
			&schema.SchemaField{
				Name: "user",
				Type: schema.FieldTypeRelation,
			},
		),
	}
	projectRecord := models.NewRecord(projectCollection)
	projectRecord.Id = projectID
	projectRecord.Set("user", userID)

	// Create deployment record
	deploymentCollection := &models.Collection{
		Name: "deployments",
		Schema: schema.NewSchema(
			&schema.SchemaField{
				Name: "user",
				Type: schema.FieldTypeRelation,
			},
			&schema.SchemaField{
				Name: "project",
				Type: schema.FieldTypeRelation,
			},
		),
	}
	deploymentRecord := models.NewRecord(deploymentCollection)
	deploymentRecord.Set("user", userID)
	deploymentRecord.Set("project", projectID)

	err := AssertDeploymentOwnership(deploymentRecord, projectRecord, userID)
	if err != nil {
		t.Errorf("Expected deployment ownership check to pass, got error: %v", err)
	}
}

// TestAssertDeploymentOwnershipWrongUser verifies check fails with wrong user
func TestAssertDeploymentOwnershipWrongUser(t *testing.T) {
	ownerID := "user123"
	requestUserID := "user456"
	projectID := "project789"

	// Create project record owned by ownerID
	projectCollection := &models.Collection{
		Name: "projects",
		Schema: schema.NewSchema(
			&schema.SchemaField{
				Name: "user",
				Type: schema.FieldTypeRelation,
			},
		),
	}
	projectRecord := models.NewRecord(projectCollection)
	projectRecord.Id = projectID
	projectRecord.Set("user", ownerID)

	// Create deployment record owned by ownerID
	deploymentCollection := &models.Collection{
		Name: "deployments",
		Schema: schema.NewSchema(
			&schema.SchemaField{
				Name: "user",
				Type: schema.FieldTypeRelation,
			},
			&schema.SchemaField{
				Name: "project",
				Type: schema.FieldTypeRelation,
			},
		),
	}
	deploymentRecord := models.NewRecord(deploymentCollection)
	deploymentRecord.Set("user", ownerID)
	deploymentRecord.Set("project", projectID)

	// Try to access with wrong user
	err := AssertDeploymentOwnership(deploymentRecord, projectRecord, requestUserID)
	if err == nil {
		t.Error("Expected deployment ownership check to fail, but it passed")
	}
}

// TestAssertDeploymentOwnershipWrongProject verifies check fails when deployment doesn't belong to project
func TestAssertDeploymentOwnershipWrongProject(t *testing.T) {
	userID := "user123"
	projectID := "project456"
	wrongProjectID := "project789"

	// Create project record
	projectCollection := &models.Collection{
		Name: "projects",
		Schema: schema.NewSchema(
			&schema.SchemaField{
				Name: "user",
				Type: schema.FieldTypeRelation,
			},
		),
	}
	projectRecord := models.NewRecord(projectCollection)
	projectRecord.Id = projectID
	projectRecord.Set("user", userID)

	// Create deployment record belonging to different project
	deploymentCollection := &models.Collection{
		Name: "deployments",
		Schema: schema.NewSchema(
			&schema.SchemaField{
				Name: "user",
				Type: schema.FieldTypeRelation,
			},
			&schema.SchemaField{
				Name: "project",
				Type: schema.FieldTypeRelation,
			},
		),
	}
	deploymentRecord := models.NewRecord(deploymentCollection)
	deploymentRecord.Set("user", userID)
	deploymentRecord.Set("project", wrongProjectID) // Different project!

	err := AssertDeploymentOwnership(deploymentRecord, projectRecord, userID)
	if err == nil {
		t.Error("Expected deployment ownership check to fail when project doesn't match, but it passed")
	}
}

// TestErrForbiddenIsError verifies that ErrForbidden implements error interface
func TestErrForbiddenIsError(t *testing.T) {
	var err error = ErrForbidden
	if err == nil {
		t.Error("ErrForbidden should implement error interface")
	}
}

// TestErrUnauthorizedIsError verifies that ErrUnauthorized implements error interface
func TestErrUnauthorizedIsError(t *testing.T) {
	var err error = ErrUnauthorized
	if err == nil {
		t.Error("ErrUnauthorized should implement error interface")
	}
}
