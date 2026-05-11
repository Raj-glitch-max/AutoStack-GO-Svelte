package controller

import (
	"github.com/pocketbase/pocketbase/tools/security"
	"github.com/pocketbase/pocketbase/models"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/models/schema"
	"github.com/pocketbase/pocketbase/tests"
)

func TestDataIsolationRBAC(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	ptrInt := func(i int) *int { return &i }

	// 1. Create collections manually for testing
	ensureCollection := func(name string, fields []*schema.SchemaField) *models.Collection {
		collection, err := app.Dao().FindCollectionByNameOrId(name)
		if err == nil {
			return collection
		}

		collection = &models.Collection{
			Name:   name,
			Type:   models.CollectionTypeBase,
			Schema: schema.NewSchema(fields...),
		}
		collection.ListRule = nil
		collection.ViewRule = nil
		collection.CreateRule = nil
		collection.UpdateRule = nil
		collection.DeleteRule = nil

		if err := app.Dao().SaveCollection(collection); err != nil {
			t.Fatalf("Failed to create collection %s: %v", name, err)
		}
		return collection
	}

	usersCollection, _ := app.Dao().FindCollectionByNameOrId("users")
	projectsCollection := ensureCollection("projects", []*schema.SchemaField{
		{Name: "name", Type: schema.FieldTypeText},
		{Name: "user", Type: schema.FieldTypeRelation, Options: &schema.RelationOptions{CollectionId: usersCollection.Id, MaxSelect: ptrInt(1)}},
	})
	membersCollection := ensureCollection("project_members", []*schema.SchemaField{
		{Name: "user", Type: schema.FieldTypeRelation, Options: &schema.RelationOptions{CollectionId: usersCollection.Id, MaxSelect: ptrInt(1)}},
		{Name: "project", Type: schema.FieldTypeRelation, Options: &schema.RelationOptions{CollectionId: projectsCollection.Id, MaxSelect: ptrInt(1)}},
		{Name: "role", Type: schema.FieldTypeText},
	})
	deploymentsCollection := ensureCollection("awsDeployments", []*schema.SchemaField{
		{Name: "name", Type: schema.FieldTypeText},
		{Name: "project", Type: schema.FieldTypeRelation, Options: &schema.RelationOptions{CollectionId: projectsCollection.Id, MaxSelect: ptrInt(1)}},
		{Name: "user", Type: schema.FieldTypeRelation, Options: &schema.RelationOptions{CollectionId: usersCollection.Id, MaxSelect: ptrInt(1)}},
	})

	// 2. Create User A
	userA := models.NewRecord(usersCollection)
	userA.Set("username", "user_a_test")
	userA.Set("email", "user_a@example.com")
	userA.Set("password", "1234567890")
	userA.Set("tokenKey", security.RandomString(50))
	if err := app.Dao().SaveRecord(userA); err != nil { t.Fatalf("Failed to save User A: %v", err) }

	// 3. Create User B
	userB := models.NewRecord(usersCollection)
	userB.Set("username", "user_b_test")
	userB.Set("email", "user_b@example.com")
	userB.Set("password", "1234567890")
	userB.Set("tokenKey", security.RandomString(50))
	if err := app.Dao().SaveRecord(userB); err != nil { t.Fatalf("Failed to save User B: %v", err) }

	// 4. Create Project A owned by User A
	projectA := models.NewRecord(projectsCollection)
	projectA.Set("name", "Project A")
	projectA.Set("user", userA.Id)
	if err := app.Dao().SaveRecord(projectA); err != nil { t.Fatalf("Failed to save Project A: %v", err) }

	// 5. Create Owner membership for User A
	memberA := models.NewRecord(membersCollection)
	memberA.Set("user", userA.Id)
	memberA.Set("project", projectA.Id)
	memberA.Set("role", RoleOwner)
	if err := app.Dao().SaveRecord(memberA); err != nil { t.Fatalf("Failed to save Member A: %v", err) }

	// 6. Create Deployment in Project A
	deploymentA := models.NewRecord(deploymentsCollection)
	deploymentA.Set("name", "Deployment A")
	deploymentA.Set("project", projectA.Id)
	deploymentA.Set("user", userA.Id)
	if err := app.Dao().SaveRecord(deploymentA); err != nil { t.Fatalf("Failed to save Deployment A: %v", err) }

	// Helper to create echo context
	createContext := func(method, path string, user *models.Record, idParam string) (echo.Context, *httptest.ResponseRecorder) {
		e := echo.New()
		req := httptest.NewRequest(method, path, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		
		if idParam != "" {
			c.SetPathParams(echo.PathParams{{Name: "id", Value: idParam}})
		}
		
		if user != nil {
			c.Set(apis.ContextAuthRecordKey, user)
		}
		
		c.Request().URL.Path = path
		
		return c, rec
	}

	// Test Case 1: User B (no membership) accessing Deployment A -> Forbidden
	t.Run("Unauthorized user access", func(t *testing.T) {
		c, _ := createContext(http.MethodGet, "/api/aws/deployments/"+deploymentA.Id, userB, deploymentA.Id)
		
		rbac := RBACMiddleware(app, RoleViewer)
		handler := func(c echo.Context) error { return c.String(http.StatusOK, "OK") }
		
		err := rbac(handler)(c)
		if err == nil {
			t.Fatal("Expected forbidden error, got nil")
		}
		
		apiErr, ok := err.(*apis.ApiError)
		if !ok || apiErr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %v", err)
		}
	})

	// Test Case 2: User A (owner) accessing Deployment A -> OK
	t.Run("Owner access", func(t *testing.T) {
		c, _ := createContext(http.MethodGet, "/api/aws/deployments/"+deploymentA.Id, userA, deploymentA.Id)
		
		rbac := RBACMiddleware(app, RoleOwner)
		handler := func(c echo.Context) error { return c.String(http.StatusOK, "OK") }
		
		err := rbac(handler)(c)
		if err != nil {
			t.Fatalf("Unexpected error for owner: %v", err)
		}
	})

	// Test Case 3: User B added as Viewer -> OK for Read
	memberB := models.NewRecord(membersCollection)
	memberB.Set("user", userB.Id)
	memberB.Set("project", projectA.Id)
	memberB.Set("role", RoleViewer)
	if err := app.Dao().SaveRecord(memberB); err != nil { t.Fatalf("Failed to save Member B: %v", err) }

	t.Run("Viewer read access", func(t *testing.T) {
		c, _ := createContext(http.MethodGet, "/api/aws/deployments/"+deploymentA.Id, userB, deploymentA.Id)
		
		rbac := RBACMiddleware(app, RoleViewer)
		handler := func(c echo.Context) error { return c.String(http.StatusOK, "OK") }
		
		err := rbac(handler)(c)
		if err != nil {
			t.Fatalf("Unexpected error for viewer: %v", err)
		}
	})

	// Test Case 4: User B (viewer) attempting to Deploy -> Forbidden
	t.Run("Viewer write access (Forbidden)", func(t *testing.T) {
		c, _ := createContext(http.MethodPost, "/api/aws/deployments/"+deploymentA.Id+"/deploy", userB, deploymentA.Id)
		
		rbac := RBACMiddleware(app, RoleDeployer)
		handler := func(c echo.Context) error { return c.String(http.StatusOK, "OK") }
		
		err := rbac(handler)(c)
		if err == nil {
			t.Fatal("Expected forbidden error for viewer trying to deploy")
		}
		
		apiErr, ok := err.(*apis.ApiError)
		if !ok || apiErr.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %v", err)
		}
	})
}
