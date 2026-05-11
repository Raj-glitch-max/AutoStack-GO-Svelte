package controller

import (
	"fmt"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
)

// Role constants
const (
	RoleOwner    = "owner"
	RoleDeployer = "deployer"
	RoleViewer   = "viewer"
)

// roleHierarchy defines the weight of each role
var roleHierarchy = map[string]int{
	RoleOwner:    3,
	RoleDeployer: 2,
	RoleViewer:   1,
}

// CheckRole checks if a user has at least the required role for a project
func CheckRole(app core.App, userID string, projectID string, requiredRole string) (bool, error) {
	if userID == "" || projectID == "" {
		return false, nil
	}

	// Fetch membership record
	expr := dbx.NewExp("user = {:user} AND project = {:project}", dbx.Params{
		"user":    userID,
		"project": projectID,
	})

	records, err := app.Dao().FindRecordsByExpr("project_members", expr)
	if err != nil || len(records) == 0 {
		// No membership found
		return false, nil
	}

	userRole := records[0].GetString("role")
	
	// Check hierarchy
	if roleHierarchy[userRole] >= roleHierarchy[requiredRole] {
		return true, nil
	}

	return false, nil
}

// RBACMiddleware returns an echo middleware that enforces project-level roles.
// It assumes the project ID is available as a path parameter "projectId" or "id" (fallback).
func RBACMiddleware(app core.App, requiredRole string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user, _ := c.Get(apis.ContextAuthRecordKey).(*models.Record)
			if user == nil {
				return apis.NewUnauthorizedError("Authentication required", nil)
			}

			// Try to get project ID from path
			projectID := c.PathParam("projectId")
			if projectID == "" {
				projectID = c.PathParam("id")
				// If the path contains /deployments/, this ID might be a deployment ID
				if strings.Contains(c.Request().URL.Path, "/deployments/") {
					deploymentID := projectID
					deployment, err := app.Dao().FindRecordById("awsDeployments", deploymentID)
					if err == nil {
						projectID = deployment.GetString("project")
					}
				}
			}

			// Direct deploymentId parameter
			deploymentID := c.PathParam("deploymentId")
			if deploymentID != "" {
				deployment, err := app.Dao().FindRecordById("awsDeployments", deploymentID)
				if err == nil {
					projectID = deployment.GetString("project")
				}
			}

			if projectID == "" {
				return next(c)
			}

			hasAccess, err := CheckRole(app, user.Id, projectID, requiredRole)
			if err != nil {
				return apis.NewBadRequestError("Error checking permissions", err)
			}

			if !hasAccess {
				return apis.NewForbiddenError(fmt.Sprintf("Insufficient permissions. Required: %s", requiredRole), nil)
			}

			return next(c)
		}
	}
}
