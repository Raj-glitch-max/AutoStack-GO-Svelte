package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase/tests"
)

func TestGetAlertPreferences(t *testing.T) {
	setupTestApp := func() (*tests.TestApp, *echo.Echo) {
		testApp, err := tests.NewTestApp()
		if err != nil {
			t.Fatal(err)
		}
		e := echo.New()
		return testApp, e
	}

	t.Run("Returns default preferences for new user", func(t *testing.T) {
		testApp, e := setupTestApp()
		defer testApp.Cleanup()

		// Create test user
		user, err := testApp.Dao().FindAuthRecordByEmail("users", "test@example.com")
		if err != nil {
			t.Skip("Test user not found")
		}

		req := httptest.NewRequest(http.MethodGet, "/api/alerts/preferences", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("authRecord", user)

		handler := getAlertPreferences(testApp.PocketBase)
		if err := handler(c); err != nil {
			t.Fatalf("Handler returned error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rec.Code)
		}

		var prefs AlertPreferences
		if err := json.Unmarshal(rec.Body.Bytes(), &prefs); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		// Check defaults
		if prefs.Threshold != 20.0 {
			t.Errorf("Expected threshold 20.0, got %f", prefs.Threshold)
		}
		if !prefs.EmailEnabled {
			t.Error("Expected email enabled by default")
		}
		if prefs.Frequency != "immediate" {
			t.Errorf("Expected frequency 'immediate', got %s", prefs.Frequency)
		}
	})

	t.Run("Returns 401 for unauthenticated request", func(t *testing.T) {
		testApp, e := setupTestApp()
		defer testApp.Cleanup()

		req := httptest.NewRequest(http.MethodGet, "/api/alerts/preferences", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := getAlertPreferences(testApp.PocketBase)
		err := handler(c)
		
		if err == nil {
			t.Error("Expected error for unauthenticated request")
		}
	})
}

func TestUpdateAlertPreferences(t *testing.T) {
	setupTestApp := func() (*tests.TestApp, *echo.Echo) {
		testApp, err := tests.NewTestApp()
		if err != nil {
			t.Fatal(err)
		}
		e := echo.New()
		return testApp, e
	}

	t.Run("Updates preferences successfully", func(t *testing.T) {
		testApp, e := setupTestApp()
		defer testApp.Cleanup()

		user, err := testApp.Dao().FindAuthRecordByEmail("users", "test@example.com")
		if err != nil {
			t.Skip("Test user not found")
		}

		prefs := AlertPreferences{
			Threshold:    30.0,
			EmailEnabled: false,
			Frequency:    "daily",
		}
		body, _ := json.Marshal(prefs)

		req := httptest.NewRequest(http.MethodPut, "/api/alerts/preferences", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("authRecord", user)

		handler := updateAlertPreferences(testApp.PocketBase)
		if err := handler(c); err != nil {
			t.Fatalf("Handler returned error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rec.Code)
		}

		var updatedPrefs AlertPreferences
		if err := json.Unmarshal(rec.Body.Bytes(), &updatedPrefs); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if updatedPrefs.Threshold != 30.0 {
			t.Errorf("Expected threshold 30.0, got %f", updatedPrefs.Threshold)
		}
		if updatedPrefs.EmailEnabled {
			t.Error("Expected email disabled")
		}
		if updatedPrefs.Frequency != "daily" {
			t.Errorf("Expected frequency 'daily', got %s", updatedPrefs.Frequency)
		}
	})

	t.Run("Rejects invalid threshold", func(t *testing.T) {
		testApp, e := setupTestApp()
		defer testApp.Cleanup()

		user, err := testApp.Dao().FindAuthRecordByEmail("users", "test@example.com")
		if err != nil {
			t.Skip("Test user not found")
		}

		prefs := AlertPreferences{
			Threshold:    150.0, // Invalid
			EmailEnabled: true,
			Frequency:    "immediate",
		}
		body, _ := json.Marshal(prefs)

		req := httptest.NewRequest(http.MethodPut, "/api/alerts/preferences", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("authRecord", user)

		handler := updateAlertPreferences(testApp.PocketBase)
		err = handler(c)
		
		if err == nil {
			t.Error("Expected error for invalid threshold")
		}
	})

	t.Run("Rejects invalid frequency", func(t *testing.T) {
		testApp, e := setupTestApp()
		defer testApp.Cleanup()

		user, err := testApp.Dao().FindAuthRecordByEmail("users", "test@example.com")
		if err != nil {
			t.Skip("Test user not found")
		}

		prefs := AlertPreferences{
			Threshold:    20.0,
			EmailEnabled: true,
			Frequency:    "invalid", // Invalid
		}
		body, _ := json.Marshal(prefs)

		req := httptest.NewRequest(http.MethodPut, "/api/alerts/preferences", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("authRecord", user)

		handler := updateAlertPreferences(testApp.PocketBase)
		err = handler(c)
		
		if err == nil {
			t.Error("Expected error for invalid frequency")
		}
	})
}

func TestDefaultAlertPreferences(t *testing.T) {
	prefs := DefaultAlertPreferences()

	if prefs.Threshold != 20.0 {
		t.Errorf("Expected default threshold 20.0, got %f", prefs.Threshold)
	}
	if !prefs.EmailEnabled {
		t.Error("Expected email enabled by default")
	}
	if prefs.Frequency != "immediate" {
		t.Errorf("Expected default frequency 'immediate', got %s", prefs.Frequency)
	}
	if prefs.SlackEnabled {
		t.Error("Expected Slack disabled by default")
	}
}
