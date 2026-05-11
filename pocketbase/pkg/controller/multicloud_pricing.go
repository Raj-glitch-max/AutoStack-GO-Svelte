package controller

import (
	"net/http"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/cost"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
)

// RegisterMultiCloudPricingRoutes registers the cross-cloud pricing comparison endpoint
func RegisterMultiCloudPricingRoutes(app *pocketbase.PocketBase, e *echo.Echo) {
	// GET /api/multicloud/pricing?workload=web-app&region=us-east-1
	e.GET("/api/multicloud/pricing", func(c echo.Context) error {
		workload := c.QueryParam("workload")
		region := c.QueryParam("region")

		if workload == "" {
			workload = "web-app"
		}
		if region == "" {
			region = "us-east-1"
		}

		// Map workload type to blueprint IDs
		blueprintMap := map[string]map[string]string{
			"web-app": {
				"aws":   "ecs-web-app",
				"gcp":   "gcp-cloud-run",
				"azure": "azure-container-apps",
			},
			"full-stack": {
				"aws":   "full-stack",
				"gcp":   "gcp-full-stack",
				"azure": "azure-full-stack",
			},
			"static-site": {
				"aws":   "static-site",
				"gcp":   "gcp-static-site",
				"azure": "azure-static-site",
			},
		}

		blueprints, ok := blueprintMap[workload]
		if !ok {
			blueprints = blueprintMap["web-app"]
		}

		// Fetch GCP and Azure pricing (AWS pricing already handled by existing estimator)
		gcpMin, gcpEst, _ := cost.GCPBlueprintEstimate(blueprints["gcp"], region)
		azureMin, azureEst, _ := cost.AzureBlueprintEstimate(blueprints["azure"], region)

		// AWS estimates from existing static data (Infracost handles real AWS pricing)
		awsEstimates := map[string]map[string]float64{
			"web-app":     {"min": 28, "est": 35},
			"full-stack":  {"min": 45, "est": 58},
			"static-site": {"min": 1, "est": 3},
		}
		awsData := awsEstimates[workload]

		return c.JSON(http.StatusOK, map[string]map[string]float64{
			"aws": {
				"min": awsData["min"],
				"est": awsData["est"],
			},
			"gcp": {
				"min": gcpMin,
				"est": gcpEst,
			},
			"azure": {
				"min": azureMin,
				"est": azureEst,
			},
		})
	}, apis.RequireRecordAuth("users"), RateLimitMiddleware(1.0, 5))
}
