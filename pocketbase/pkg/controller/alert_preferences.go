package controller

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/models"
)

// AlertPreferences represents user preferences for cost alerts
type AlertPreferences struct {
	Threshold      float64 `json:"threshold"`      // Alert threshold percentage (default 20%)
	EmailEnabled   bool    `json:"emailEnabled"`   // Whether to send email notifications
	Frequency      string  `json:"frequency"`      // "immediate", "daily", "weekly"
	SlackEnabled   bool    `json:"slackEnabled"`   // Future: Slack notifications
	SlackWebhook   string  `json:"slackWebhook"`   // Future: Slack webhook URL
}

// DefaultAlertPreferences returns the default alert preferences
func DefaultAlertPreferences() AlertPreferences {
	return AlertPreferences{
		Threshold:    20.0,
		EmailEnabled: true,
		Frequency:    "immediate",
		SlackEnabled: false,
		SlackWebhook: "",
	}
}

// RegisterAlertPreferencesRoutes registers alert preference endpoints
func RegisterAlertPreferencesRoutes(app *pocketbase.PocketBase, e *echo.Echo) {
	e.GET("/api/alerts/preferences", getAlertPreferences(app), apis.RequireRecordAuth("users"))
	e.PUT("/api/alerts/preferences", updateAlertPreferences(app), apis.RequireRecordAuth("users"))
}

// getAlertPreferences returns the current user's alert preferences
func getAlertPreferences(app *pocketbase.PocketBase) echo.HandlerFunc {
	return func(c echo.Context) error {
		authRecord, _ := c.Get(apis.ContextAuthRecordKey).(*models.Record)
		if authRecord == nil {
			return apis.NewUnauthorizedError("Unauthorized", nil)
		}

		// Get preferences from user record
		prefsData := authRecord.Get("alertPreferences")
		
		var prefs AlertPreferences
		if prefsData == nil {
			// Return defaults if not set
			prefs = DefaultAlertPreferences()
		} else {
			// Parse existing preferences
			prefsJSON, err := json.Marshal(prefsData)
			if err != nil {
				prefs = DefaultAlertPreferences()
			} else {
				if err := json.Unmarshal(prefsJSON, &prefs); err != nil {
					prefs = DefaultAlertPreferences()
				}
			}
		}

		return c.JSON(http.StatusOK, prefs)
	}
}

// updateAlertPreferences updates the current user's alert preferences
func updateAlertPreferences(app *pocketbase.PocketBase) echo.HandlerFunc {
	return func(c echo.Context) error {
		authRecord, _ := c.Get(apis.ContextAuthRecordKey).(*models.Record)
		if authRecord == nil {
			return apis.NewUnauthorizedError("Unauthorized", nil)
		}

		var prefs AlertPreferences
		if err := c.Bind(&prefs); err != nil {
			return apis.NewBadRequestError("Invalid request body", err)
		}

		// Validate preferences
		if prefs.Threshold < 0 || prefs.Threshold > 100 {
			return apis.NewBadRequestError("Threshold must be between 0 and 100", nil)
		}

		if prefs.Frequency != "immediate" && prefs.Frequency != "daily" && prefs.Frequency != "weekly" {
			return apis.NewBadRequestError("Frequency must be 'immediate', 'daily', or 'weekly'", nil)
		}

		// Update user record
		authRecord.Set("alertPreferences", prefs)
		
		if err := app.Dao().SaveRecord(authRecord); err != nil {
			return apis.NewApiError(http.StatusInternalServerError, "Failed to update preferences", err)
		}

		return c.JSON(http.StatusOK, prefs)
	}
}
