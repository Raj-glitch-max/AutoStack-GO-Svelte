package controller

import (
	"context"
	"net/http"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/health"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/models"
)

type HealthController struct {
	app        *pocketbase.PocketBase
	awsMonitor *aws.HealthMonitor
	k8sMonitor *health.K8sHealthMonitor
}

func NewHealthController(app *pocketbase.PocketBase, awsMonitor *aws.HealthMonitor, k8sMonitor *health.K8sHealthMonitor) *HealthController {
	return &HealthController{
		app:        app,
		awsMonitor: awsMonitor,
		k8sMonitor: k8sMonitor,
	}
}

func (hc *HealthController) RegisterHealthRoutes(router *echo.Group) {
	router.GET("/api/health/summary", hc.HandleHealthSummary, apis.RequireRecordAuth("users"))
	router.GET("/api/health/detail/:deploymentId", hc.HandleHealthDetail, apis.RequireRecordAuth("users"))
}

// HandleHealthSummary returns health status for all active deployments of a user
func (hc *HealthController) HandleHealthSummary(c echo.Context) error {
	user := c.Get("user").(*models.Record)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "Authentication required")
	}

	results := make([]*health.DeploymentHealth, 0)

	// 1. Fetch AWS deployments
	awsDeployments, err := hc.app.Dao().FindRecordsByFilter(
		"awsDeployments",
		"user = {:user} && status = 'active'",
		"-created",
		0, 0,
		map[string]interface{}{"user": user.Id},
	)
	if err == nil {
		for _, dep := range awsDeployments {
			h, err := hc.awsMonitor.GetDeploymentHealth(context.Background(), dep.Id)
			if err != nil {
				hc.app.Logger().Error("Failed to get health for AWS deployment", "deployment", dep.Id, "error", err)
				continue
			}
			results = append(results, h)
		}
	}

	// 2. Fetch K8s deployments
	k8sDeployments, err := hc.app.Dao().FindRecordsByFilter(
		"deployments",
		"user = {:user}",
		"-created",
		0, 0,
		map[string]interface{}{"user": user.Id},
	)
	if err == nil {
		for _, dep := range k8sDeployments {
			h, err := hc.k8sMonitor.GetDeploymentHealth(context.Background(), dep.Id)
			if err != nil {
				hc.app.Logger().Error("Failed to get health for K8s deployment", "deployment", dep.Id, "error", err)
				continue
			}
			results = append(results, h)
		}
	}

	return c.JSON(http.StatusOK, results)
}

// HandleHealthDetail returns detailed health for a specific deployment
func (hc *HealthController) HandleHealthDetail(c echo.Context) error {
	user := c.Get("user").(*models.Record)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "Authentication required")
	}

	deploymentID := c.PathParam("deploymentId")

	// Try AWS first
	deployment, err := hc.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err == nil {
		if deployment.GetString("user") != user.Id {
			return echo.NewHTTPError(http.StatusForbidden, "Access denied")
		}
		h, err := hc.awsMonitor.GetDeploymentHealth(context.Background(), deploymentID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get health data")
		}
		return c.JSON(http.StatusOK, h)
	}

	// Try K8s
	deployment, err = hc.app.Dao().FindRecordById("deployments", deploymentID)
	if err == nil {
		if deployment.GetString("user") != user.Id {
			return echo.NewHTTPError(http.StatusForbidden, "Access denied")
		}
		h, err := hc.k8sMonitor.GetDeploymentHealth(context.Background(), deploymentID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get health data")
		}
		return c.JSON(http.StatusOK, h)
	}

	return echo.NewHTTPError(http.StatusNotFound, "Deployment not found")
}
