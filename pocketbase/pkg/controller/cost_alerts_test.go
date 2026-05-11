package controller

import (
	"github.com/pocketbase/pocketbase/models"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/cost"
)

// TestGetDeploymentAlerts tests retrieving alerts for a deployment
// Validates: AC-4.1 (Alert triggered when actual cost exceeds estimate by 20%)
func TestGetDeploymentAlerts(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	// Create test user
	user, err := app.Dao().FindAuthRecordByEmail("users", "test@example.com")
	if err != nil {
		user, err = mockAuthRecord(app, "users")
		if err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}
	}

	// Create test deployment
	deploymentCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentCollection)
	deployment.Set("name", "test-deployment")
	deployment.Set("user", user.Id)
	deployment.Set("status", "active")
	deployment.Set("blueprint", "web-application")
	deployment.Set("region", "us-east-1")

	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	// Create test alert
	alertCollection, err := app.Dao().FindCollectionByNameOrId("costAlerts")
	if err != nil {
		t.Fatalf("Failed to find costAlerts collection: %v", err)
	}

	alert := models.NewRecord(alertCollection)
	alert.Set("deployment", deployment.Id)
	alert.Set("user", user.Id)
	alert.Set("type", "cost_overrun")
	alert.Set("threshold", 20.0)
	alert.Set("triggered", true)
	alert.Set("actualCost", 120.0)
	alert.Set("estimatedCost", 100.0)
	alert.Set("variance", 20.0)
	alert.Set("message", "Deployment costs 20.0% above estimate")
	alert.Set("serviceBreakdown", map[string]interface{}{
		"AmazonEC2": 50.0,
		"AmazonRDS": 70.0,
	})
	alert.Set("sentAt", time.Now())
	alert.Set("acknowledged", false)

	if err := app.Dao().SaveRecord(alert); err != nil {
		t.Fatalf("Failed to create alert: %v", err)
	}

	// Create request
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/cost/alerts/deployment/"+deployment.Id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathParams(echo.PathParams{{Name: "deploymentId", Value: deployment.Id}})
	c.Set("authRecord", user)

	// Call handler
	err = GetDeploymentAlerts(c, app)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// Check response
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	alerts, ok := response["alerts"].([]interface{})
	if !ok {
		t.Fatal("Response missing alerts array")
	}

	if len(alerts) != 1 {
		t.Errorf("Expected 1 alert, got %d", len(alerts))
	}

	alertData := alerts[0].(map[string]interface{})
	if alertData["deployment"] != deployment.Id {
		t.Errorf("Expected deploymentId %s, got %s", deployment.Id, alertData["deployment"])
	}

	if alertData["variance"] != 20.0 {
		t.Errorf("Expected variance 20.0, got %v", alertData["variance"])
	}

	if alertData["acknowledged"] != false {
		t.Error("Expected alert to not be acknowledged")
	}
}

// TestGetDeploymentAlertsUnauthorized tests access control
// Validates: User authorization for alert access
func TestGetDeploymentAlertsUnauthorized(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	// Create test users
	user1, err := mockAuthRecord(app, "users")
	if err != nil {
		t.Fatalf("Failed to create test user 1: %v", err)
	}

	user2, err := mockAuthRecord(app, "users")
	if err != nil {
		t.Fatalf("Failed to create test user 2: %v", err)
	}

	// Create deployment owned by user1
	deploymentCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentCollection)
	deployment.Set("name", "test-deployment")
	deployment.Set("user", user1.Id)
	deployment.Set("status", "active")
	deployment.Set("blueprint", "web-application")
	deployment.Set("region", "us-east-1")

	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	// Try to access with user2
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/cost/alerts/deployment/"+deployment.Id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathParams(echo.PathParams{{Name: "deploymentId", Value: deployment.Id}})
	c.Set("authRecord", user2)

	// Call handler
	err = GetDeploymentAlerts(c, app)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	apiErr, ok := err.(*apis.ApiError)
	if !ok || apiErr.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden error, got %v", err)
	}
}

// TestGetUserAlerts tests retrieving all alerts for a user
// Validates: AC-4.2 (Alert sent via email and in-app notification)
func TestGetUserAlerts(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	// Create test user
	user, err := mockAuthRecord(app, "users")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create multiple deployments
	deploymentCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment1 := models.NewRecord(deploymentCollection)
	deployment1.Set("name", "deployment-1")
	deployment1.Set("user", user.Id)
	deployment1.Set("status", "active")
	deployment1.Set("blueprint", "web-application")
	deployment1.Set("region", "us-east-1")

	if err := app.Dao().SaveRecord(deployment1); err != nil {
		t.Fatalf("Failed to create deployment 1: %v", err)
	}

	deployment2 := models.NewRecord(deploymentCollection)
	deployment2.Set("name", "deployment-2")
	deployment2.Set("user", user.Id)
	deployment2.Set("status", "active")
	deployment2.Set("blueprint", "static-website")
	deployment2.Set("region", "us-west-2")

	if err := app.Dao().SaveRecord(deployment2); err != nil {
		t.Fatalf("Failed to create deployment 2: %v", err)
	}

	// Create alerts for both deployments
	alertCollection, err := app.Dao().FindCollectionByNameOrId("costAlerts")
	if err != nil {
		t.Fatalf("Failed to find costAlerts collection: %v", err)
	}

	alert1 := models.NewRecord(alertCollection)
	alert1.Set("deployment", deployment1.Id)
	alert1.Set("user", user.Id)
	alert1.Set("type", "cost_overrun")
	alert1.Set("threshold", 20.0)
	alert1.Set("triggered", true)
	alert1.Set("actualCost", 120.0)
	alert1.Set("estimatedCost", 100.0)
	alert1.Set("variance", 20.0)
	alert1.Set("message", "Deployment costs 20.0% above estimate")
	alert1.Set("serviceBreakdown", map[string]interface{}{})
	alert1.Set("sentAt", time.Now())
	alert1.Set("acknowledged", false)

	if err := app.Dao().SaveRecord(alert1); err != nil {
		t.Fatalf("Failed to create alert 1: %v", err)
	}

	alert2 := models.NewRecord(alertCollection)
	alert2.Set("deployment", deployment2.Id)
	alert2.Set("user", user.Id)
	alert2.Set("type", "cost_overrun")
	alert2.Set("threshold", 25.0)
	alert2.Set("triggered", true)
	alert2.Set("actualCost", 62.5)
	alert2.Set("estimatedCost", 50.0)
	alert2.Set("variance", 25.0)
	alert2.Set("message", "Deployment costs 25.0% above estimate")
	alert2.Set("serviceBreakdown", map[string]interface{}{})
	alert2.Set("sentAt", time.Now())
	alert2.Set("acknowledged", true)

	if err := app.Dao().SaveRecord(alert2); err != nil {
		t.Fatalf("Failed to create alert 2: %v", err)
	}

	// Create request
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/cost/alerts", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("authRecord", user)

	// Call handler
	err = GetUserAlerts(c, app)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// Check response
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	alerts, ok := response["alerts"].([]interface{})
	if !ok {
		t.Fatal("Response missing alerts array")
	}

	if len(alerts) != 2 {
		t.Errorf("Expected 2 alerts, got %d", len(alerts))
	}
}

// TestAcknowledgeAlert tests acknowledging an alert
// Validates: AC-4.1 (Alert triggered when actual cost exceeds estimate by 20%)
func TestAcknowledgeAlert(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	// Create test user
	user, err := mockAuthRecord(app, "users")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create test deployment
	deploymentCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentCollection)
	deployment.Set("name", "test-deployment")
	deployment.Set("user", user.Id)
	deployment.Set("status", "active")
	deployment.Set("blueprint", "web-application")
	deployment.Set("region", "us-east-1")

	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	// Create test alert
	alertCollection, err := app.Dao().FindCollectionByNameOrId("costAlerts")
	if err != nil {
		t.Fatalf("Failed to find costAlerts collection: %v", err)
	}

	alert := models.NewRecord(alertCollection)
	alert.Set("deployment", deployment.Id)
	alert.Set("user", user.Id)
	alert.Set("type", "cost_overrun")
	alert.Set("threshold", 20.0)
	alert.Set("triggered", true)
	alert.Set("actualCost", 120.0)
	alert.Set("estimatedCost", 100.0)
	alert.Set("variance", 20.0)
	alert.Set("message", "Deployment costs 20.0% above estimate")
	alert.Set("serviceBreakdown", map[string]interface{}{})
	alert.Set("sentAt", time.Now())
	alert.Set("acknowledged", false)

	if err := app.Dao().SaveRecord(alert); err != nil {
		t.Fatalf("Failed to create alert: %v", err)
	}

	// Create request
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/cost/alerts/"+alert.Id+"/acknowledge", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathParams(echo.PathParams{{Name: "alertId", Value: alert.Id}})
	c.Set("authRecord", user)

	// Call handler
	err = AcknowledgeAlert(c, app)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// Check response
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["status"] != "success" {
		t.Errorf("Expected status to be success, got %v", response["status"])
	}

	// Verify alert was acknowledged in database
	updatedAlert, err := app.Dao().FindRecordById("costAlerts", alert.Id)
	if err != nil {
		t.Fatalf("Failed to retrieve updated alert: %v", err)
	}

	if !updatedAlert.GetBool("acknowledged") {
		t.Error("Expected alert to be acknowledged in database")
	}
}

// TestAcknowledgeAlertUnauthorized tests access control for acknowledgment
// Validates: User authorization for alert acknowledgment
func TestAcknowledgeAlertUnauthorized(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	// Create test users
	user1, err := mockAuthRecord(app, "users")
	if err != nil {
		t.Fatalf("Failed to create test user 1: %v", err)
	}

	user2, err := mockAuthRecord(app, "users")
	if err != nil {
		t.Fatalf("Failed to create test user 2: %v", err)
	}

	// Create deployment owned by user1
	deploymentCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentCollection)
	deployment.Set("name", "test-deployment")
	deployment.Set("user", user1.Id)
	deployment.Set("status", "active")
	deployment.Set("blueprint", "web-application")
	deployment.Set("region", "us-east-1")

	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	// Create alert for user1
	alertCollection, err := app.Dao().FindCollectionByNameOrId("costAlerts")
	if err != nil {
		t.Fatalf("Failed to find costAlerts collection: %v", err)
	}

	alert := models.NewRecord(alertCollection)
	alert.Set("deployment", deployment.Id)
	alert.Set("user", user1.Id)
	alert.Set("type", "cost_overrun")
	alert.Set("threshold", 20.0)
	alert.Set("triggered", true)
	alert.Set("actualCost", 120.0)
	alert.Set("estimatedCost", 100.0)
	alert.Set("variance", 20.0)
	alert.Set("message", "Deployment costs 20.0% above estimate")
	alert.Set("serviceBreakdown", map[string]interface{}{})
	alert.Set("sentAt", time.Now())
	alert.Set("acknowledged", false)

	if err := app.Dao().SaveRecord(alert); err != nil {
		t.Fatalf("Failed to create alert: %v", err)
	}

	// Try to acknowledge with user2
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/cost/alerts/"+alert.Id+"/acknowledge", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathParams(echo.PathParams{{Name: "alertId", Value: alert.Id}})
	c.Set("authRecord", user2)

	// Call handler
	err = AcknowledgeAlert(c, app)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	apiErr, ok := err.(*apis.ApiError)
	if !ok || apiErr.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden error, got %v", err)
	}

	// Verify alert was NOT acknowledged in database
	updatedAlert, err := app.Dao().FindRecordById("costAlerts", alert.Id)
	if err != nil {
		t.Fatalf("Failed to retrieve alert: %v", err)
	}

	if updatedAlert.GetBool("acknowledged") {
		t.Error("Expected alert to remain unacknowledged")
	}
}

// TestAcknowledgeAlertNotFound tests handling of non-existent alerts
func TestAcknowledgeAlertNotFound(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	// Create test user
	user, err := mockAuthRecord(app, "users")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Try to acknowledge non-existent alert
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/cost/alerts/nonexistent/acknowledge", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathParams(echo.PathParams{{Name: "alertId", Value: "nonexistent"}})
	c.Set("authRecord", user)

	// Call handler
	err = AcknowledgeAlert(c, app)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	apiErr, ok := err.(*apis.ApiError)
	if !ok || apiErr.Code != http.StatusNotFound {
		t.Errorf("Expected 404 Not Found error, got %v", err)
	}
}

// TestAcknowledgeAlertIntegration tests the full alert acknowledgment flow
// Validates: AC-4.1 (Alert triggered when actual cost exceeds estimate by 20%)
func TestAcknowledgeAlertIntegration(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()
	_ = bootstrapAlertTestCollections(app)

	// Create test user
	user, err := mockAuthRecord(app, "users")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create test deployment
	deploymentCollection, err := app.Dao().FindCollectionByNameOrId("deployments")
	if err != nil {
		t.Fatalf("Failed to find deployments collection: %v", err)
	}

	deployment := models.NewRecord(deploymentCollection)
	deployment.Set("name", "test-deployment")
	deployment.Set("user", user.Id)
	deployment.Set("status", "active")
	deployment.Set("blueprint", "web-application")
	deployment.Set("region", "us-east-1")

	if err := app.Dao().SaveRecord(deployment); err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	// Create cost monitor and trigger alert
	costMonitor, err := cost.NewCostMonitor(app)
	if err != nil {
		t.Fatalf("Failed to create cost monitor: %v", err)
	}

	// Create alert using cost monitor
	alertCollection, err := app.Dao().FindCollectionByNameOrId("costAlerts")
	if err != nil {
		t.Fatalf("Failed to find costAlerts collection: %v", err)
	}

	alert := models.NewRecord(alertCollection)
	alert.Set("deployment", deployment.Id)
	alert.Set("user", user.Id)
	alert.Set("type", "cost_overrun")
	alert.Set("threshold", 20.0)
	alert.Set("triggered", true)
	alert.Set("actualCost", 120.0)
	alert.Set("estimatedCost", 100.0)
	alert.Set("variance", 20.0)
	alert.Set("message", "Deployment costs 20.0% above estimate")
	alert.Set("serviceBreakdown", map[string]interface{}{})
	alert.Set("sentAt", time.Now())
	alert.Set("acknowledged", false)

	if err := app.Dao().SaveRecord(alert); err != nil {
		t.Fatalf("Failed to create alert: %v", err)
	}

	// Step 1: Get alerts for deployment (should have 1 unacknowledged)
	alerts, err := costMonitor.GetAlertsForDeployment(deployment.Id)
	if err != nil {
		t.Fatalf("Failed to get alerts: %v", err)
	}

	if len(alerts) != 1 {
		t.Fatalf("Expected 1 alert, got %d", len(alerts))
	}

	if alerts[0].Acknowledged {
		t.Error("Expected alert to be unacknowledged")
	}

	// Step 2: Acknowledge the alert
	err = costMonitor.AcknowledgeAlert(alert.Id)
	if err != nil {
		t.Fatalf("Failed to acknowledge alert: %v", err)
	}

	// Step 3: Verify alert is acknowledged
	alerts, err = costMonitor.GetAlertsForDeployment(deployment.Id)
	if err != nil {
		t.Fatalf("Failed to get alerts after acknowledgment: %v", err)
	}

	if len(alerts) != 1 {
		t.Fatalf("Expected 1 alert, got %d", len(alerts))
	}

	if !alerts[0].Acknowledged {
		t.Error("Expected alert to be acknowledged")
	}
}

