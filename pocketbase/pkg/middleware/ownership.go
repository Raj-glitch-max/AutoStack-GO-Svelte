package middleware

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase/models"
)

var (
	// ErrForbidden is returned when an ownership check fails
	// Maps to HTTP 403 - never return 404 for owned resources (don't leak existence)
	ErrForbidden = errors.New("forbidden: you do not have access to this resource")

	// ErrUnauthorized is returned when authentication is missing or invalid
	ErrUnauthorized = errors.New("unauthorized: authentication required")
)

// AssertOwnership verifies that a record belongs to the specified user.
// This function MUST be called at the top of every handler that touches a user-owned resource.
//
// Parameters:
//   - record: The PocketBase record to check ownership of
//   - userID: The ID of the user making the request
//
// Returns:
//   - nil if the user owns the record
//   - ErrForbidden if the user does not own the record (return 403, never 404)
//
// Security Note: Always return 403 (Forbidden) instead of 404 (Not Found) for ownership
// failures to avoid leaking information about resource existence.
func AssertOwnership(record *models.Record, userID string) error {
	if record == nil {
		return fmt.Errorf("record cannot be nil")
	}

	if userID == "" {
		return ErrUnauthorized
	}

	// Check if the record has a 'user' field
	recordUserID := record.GetString("user")
	if recordUserID == "" {
		// If no user field, this might be a system record or misconfigured collection
		return fmt.Errorf("record does not have a user field")
	}

	if recordUserID != userID {
		return ErrForbidden
	}

	return nil
}

// AssertOwnershipMultiple verifies that all records belong to the specified user.
// Returns error on the first ownership violation.
func AssertOwnershipMultiple(records []*models.Record, userID string) error {
	for i, record := range records {
		if err := AssertOwnership(record, userID); err != nil {
			return fmt.Errorf("ownership check failed for record %d: %w", i, err)
		}
	}
	return nil
}

// OwnershipMiddleware is an Echo middleware that extracts the authenticated user ID
// and makes it available in the context for ownership checks.
//
// This middleware should be applied to routes that require user authentication.
// It expects the user to be authenticated via PocketBase's auth system.
func OwnershipMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get the authenticated user from PocketBase context
			// PocketBase stores the authenticated record in the context
			authRecord := c.Get("authRecord")
			if authRecord == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
			}

			// Type assert to *models.Record
			record, ok := authRecord.(*models.Record)
			if !ok {
				return echo.NewHTTPError(http.StatusInternalServerError, "invalid auth record type")
			}

			// Store user ID in context for easy access
			c.Set("userID", record.Id)

			return next(c)
		}
	}
}

// GetUserIDFromContext extracts the user ID from the Echo context.
// Returns empty string if not found.
func GetUserIDFromContext(c echo.Context) string {
	userID := c.Get("userID")
	if userID == nil {
		return ""
	}

	userIDStr, ok := userID.(string)
	if !ok {
		return ""
	}

	return userIDStr
}

// RequireOwnership is a helper function that combines getting the user ID from context
// and asserting ownership of a record. Use this in handlers for convenience.
//
// Example usage:
//
//	func handleGetDeployment(c echo.Context) error {
//	    deployment := // ... load deployment record
//	    if err := middleware.RequireOwnership(c, deployment); err != nil {
//	        return err // automatically returns appropriate HTTP error
//	    }
//	    // ... continue with handler logic
//	}
func RequireOwnership(c echo.Context, record *models.Record) error {
	userID := GetUserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, ErrUnauthorized.Error())
	}

	if err := AssertOwnership(record, userID); err != nil {
		if errors.Is(err, ErrForbidden) {
			return echo.NewHTTPError(http.StatusForbidden, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "ownership check failed")
	}

	return nil
}

// RequireOwnershipMultiple is a helper for checking ownership of multiple records.
func RequireOwnershipMultiple(c echo.Context, records []*models.Record) error {
	userID := GetUserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, ErrUnauthorized.Error())
	}

	if err := AssertOwnershipMultiple(records, userID); err != nil {
		if errors.Is(err, ErrForbidden) {
			return echo.NewHTTPError(http.StatusForbidden, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "ownership check failed")
	}

	return nil
}

// AssertProjectOwnership is a specialized ownership check for project-related resources.
// It verifies that the user owns the project associated with a resource.
//
// This is useful for resources like deployments that belong to a project,
// where we need to verify the user owns the parent project.
func AssertProjectOwnership(projectRecord *models.Record, userID string) error {
	return AssertOwnership(projectRecord, userID)
}

// AssertDeploymentOwnership verifies ownership of a deployment and its parent project.
// This ensures the user owns both the deployment and the project it belongs to.
func AssertDeploymentOwnership(deploymentRecord *models.Record, projectRecord *models.Record, userID string) error {
	// Check deployment ownership
	if err := AssertOwnership(deploymentRecord, userID); err != nil {
		return fmt.Errorf("deployment ownership check failed: %w", err)
	}

	// Check project ownership
	if err := AssertOwnership(projectRecord, userID); err != nil {
		return fmt.Errorf("project ownership check failed: %w", err)
	}

	// Verify deployment belongs to the project
	deploymentProjectID := deploymentRecord.GetString("project")
	if deploymentProjectID != projectRecord.Id {
		return fmt.Errorf("deployment does not belong to the specified project")
	}

	return nil
}
