package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/terraform"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/universal"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/util"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/watcher"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/models"
)

// UniversalDeployController handles the universal deployment pipeline
type UniversalDeployController struct {
	app         *pocketbase.PocketBase
	credManager *aws.CredentialManager
	tfExec      *terraform.ExecutorService
}

// NewUniversalDeployController creates a new universal deploy controller
func NewUniversalDeployController(
	app *pocketbase.PocketBase,
	credManager *aws.CredentialManager,
	tfExec *terraform.ExecutorService,
) *UniversalDeployController {
	return &UniversalDeployController{
		app:         app,
		credManager: credManager,
		tfExec:      tfExec,
	}
}

// RegisterUniversalDeployRoutes registers the universal deployment endpoints
func RegisterUniversalDeployRoutes(app *pocketbase.PocketBase, e *echo.Echo, credManager *aws.CredentialManager, tfExec *terraform.ExecutorService) {
	ctrl := NewUniversalDeployController(app, credManager, tfExec)

	e.POST("/api/deployments/universal", ctrl.HandleUniversalDeploy,
		apis.RequireRecordAuth("users"),
		RateLimitMiddleware(0.2, 2),
	)

	e.POST("/api/deployments/:deploymentId/confirm", ctrl.HandleConfirm,
		apis.RequireRecordAuth("users"),
	)
}

// HandleUniversalDeploy is Stage 1-4: analyze, decide, estimate, return for confirmation
func (c *UniversalDeployController) HandleUniversalDeploy(ctx echo.Context) error {
	authRecord, _ := ctx.Get(apis.ContextAuthRecordKey).(*models.Record)
	if authRecord == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	var req universal.UniversalDeployRequest
	if err := ctx.Bind(&req); err != nil {
		return apis.NewBadRequestError("Invalid request body", err)
	}

	if req.InputType == "" {
		return apis.NewBadRequestError("input_type is required", nil)
	}
	if req.ProjectID == "" {
		return apis.NewBadRequestError("project_id is required", nil)
	}

	// Verify project ownership
	project, err := c.app.Dao().FindRecordById("projects", req.ProjectID)
	if err != nil {
		return apis.NewNotFoundError("Project not found", err)
	}
	if project.GetString("user") != authRecord.Id {
		return apis.NewForbiddenError("Access denied", nil)
	}

	// Create deployment record immediately so we have an ID
	deploymentID := util.GenerateId(15)
	collection, err := c.app.Dao().FindCollectionByNameOrId("awsDeployments")
	if err != nil {
		return apis.NewApiError(http.StatusInternalServerError, "Failed to find collection", err)
	}

	deployment := models.NewRecord(collection)
	deployment.Set("id", deploymentID)
	deployment.Set("name", coalesceStr(req.GitURL, req.DockerImage, req.NaturalLanguage, "universal-deploy"))
	deployment.Set("project", req.ProjectID)
	deployment.Set("user", authRecord.Id)
	deployment.Set("status", "analyzing")
	deployment.Set("region", coalesceStr(req.Region, "us-east-1"))

	if err := c.app.Dao().SaveRecord(deployment); err != nil {
		return apis.NewApiError(http.StatusInternalServerError, "Failed to create deployment", err)
	}

	// Run analysis asynchronously and return immediately with deployment ID
	go c.runAnalysis(deploymentID, authRecord.Id, req)

	return ctx.JSON(http.StatusAccepted, map[string]interface{}{
		"deployment_id": deploymentID,
		"status":        "analyzing",
		"message":       "Analysis started. Connect to WebSocket for progress updates.",
	})
}

// runAnalysis runs Stages 1-4 asynchronously
func (c *UniversalDeployController) runAnalysis(deploymentID, userID string, req universal.UniversalDeployRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	workDir := filepath.Join(os.TempDir(), "autostack", deploymentID)
	os.MkdirAll(workDir, 0755)

	watcher.BroadcastDeploymentStatus(deploymentID, "analyzing", "Analyzing application...")

	// Stage 1: Normalize input
	raw, err := universal.NormalizeInput(req, workDir)
	if err != nil {
		c.failDeployment(deploymentID, fmt.Sprintf("Input normalization failed: %v", err))
		return
	}

	// Stage 2: Analyze app
	profile := universal.AnalyzeApp(*raw)

	// If confidence is low, use AI
	if profile.DetectionConfidence < 0.6 || req.InputType == "natural_language" {
		watcher.BroadcastDeploymentStatus(deploymentID, "analyzing", "Using AI to analyze application...")
		aiProfile, err := universal.AIAnalyzeApp(ctx, *raw, profile)
		if err != nil {
			log.Printf("[UniversalDeploy] AI analysis failed, using partial profile: %v", err)
		} else {
			profile = aiProfile
		}
	}

	// Stage 3: Decide infrastructure
	prefs := universal.UserPreferences{
		Region:           coalesceStr(req.Region, "us-east-1"),
		HighAvailability: req.HighAvailability,
		EnableCDN:        req.EnableCDN,
		CustomDomain:     req.CustomDomain,
		Environment:      coalesceStr(req.Environment, "production"),
	}
	spec := universal.DecideInfrastructure(profile, prefs)

	// Stage 4: Estimate cost
	estimate := universal.EstimateCost(spec)
	spec.EstimatedMonthlyCostUSD = estimate.MonthlyEstimate

	// Store analysis results in deployment record
	deployment, err := c.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		c.failDeployment(deploymentID, "Failed to find deployment record")
		return
	}

	deployment.Set("status", "awaiting_confirmation")
	deployment.Set("app_profile", profile)
	deployment.Set("infra_spec", spec)
	deployment.Set("cost_estimate", estimate)

	if err := c.app.Dao().SaveRecord(deployment); err != nil {
		c.failDeployment(deploymentID, "Failed to save analysis results")
		return
	}

	// Broadcast analysis complete with full results
	watcher.BroadcastDeploymentStatus(deploymentID, "awaiting_confirmation",
		fmt.Sprintf("Analysis complete. Estimated cost: $%.2f/month. Awaiting confirmation.", estimate.MonthlyEstimate))
}

// HandleConfirm triggers Stages 5-8 after user confirms
func (c *UniversalDeployController) HandleConfirm(ctx echo.Context) error {
	authRecord, _ := ctx.Get(apis.ContextAuthRecordKey).(*models.Record)
	if authRecord == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	deploymentID := ctx.PathParam("deploymentId")
	deployment, err := c.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		return apis.NewNotFoundError("Deployment not found", err)
	}

	if deployment.GetString("user") != authRecord.Id {
		return apis.NewForbiddenError("Access denied", nil)
	}

	if deployment.GetString("status") != "awaiting_confirmation" {
		return apis.NewBadRequestError(
			fmt.Sprintf("Deployment is in status '%s', expected 'awaiting_confirmation'", deployment.GetString("status")),
			nil,
		)
	}

	// Retrieve stored spec
	var spec universal.InfrastructureSpec
	deployment.UnmarshalJSONField("infra_spec", &spec)

	// Update status
	deployment.Set("status", "generating_terraform")
	c.app.Dao().SaveRecord(deployment)

	// Run Stages 5-8 asynchronously
	go c.runDeployment(deploymentID, authRecord.Id, spec)

	return ctx.JSON(http.StatusAccepted, map[string]interface{}{
		"deployment_id": deploymentID,
		"status":        "generating_terraform",
		"message":       "Deployment started. Connect to WebSocket for real-time progress.",
	})
}

// runDeployment runs Stages 5-8 asynchronously
func (c *UniversalDeployController) runDeployment(deploymentID, userID string, spec universal.InfrastructureSpec) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	workDir := filepath.Join(os.TempDir(), "autostack", deploymentID)
	os.MkdirAll(workDir, 0755)

	// Stage 5: Generate Terraform
	watcher.BroadcastDeploymentStatus(deploymentID, "generating_terraform", "Generating infrastructure code...")

	hcl, err := universal.GenerateTerraform(ctx, spec)
	if err != nil {
		c.failDeployment(deploymentID, fmt.Sprintf("Terraform generation failed: %v", err))
		return
	}

	// Stage 6: Validate + auto-fix
	watcher.BroadcastDeploymentStatus(deploymentID, "validating", "Validating Terraform (attempt 1/3)...")

	validHCL, err := universal.ValidateAndFix(ctx, hcl, workDir, spec)
	if err != nil {
		c.failDeployment(deploymentID, fmt.Sprintf("Terraform validation failed: %v", err))
		return
	}

	// Write final validated HCL
	if err := os.WriteFile(filepath.Join(workDir, "main.tf"), []byte(validHCL), 0644); err != nil {
		c.failDeployment(deploymentID, "Failed to write Terraform file")
		return
	}

	// Stage 7: Execute via existing terraform executor
	watcher.BroadcastDeploymentStatus(deploymentID, "applying", "Applying infrastructure...")

	// Get AWS credentials for this user (validates they exist before starting)
	_, err = c.credManager.GetCredentials(userID)
	if err != nil {
		c.failDeployment(deploymentID, "AWS credentials not found. Please configure credentials first.")
		return
	}

	// Use existing executor
	logStreamer := terraform.NewWebSocketLogStreamer()
	result, err := c.tfExec.Apply(ctx, deploymentID, deploymentID, userID, true, logStreamer)
	if err != nil || result.ExitCode != 0 {
		errMsg := "Terraform apply failed"
		if result != nil {
			errMsg = result.Output
		}
		c.failDeployment(deploymentID, errMsg)
		return
	}

	// Stage 8: Collect outputs
	rawOutputs, err := c.tfExec.GetOutputs(ctx, deploymentID, userID)
	outputJSON := "{}"
	if err == nil && rawOutputs != nil {
		if b, jsonErr := json.Marshal(rawOutputs); jsonErr == nil {
			outputJSON = string(b)
		}
	}
	outputs, err := universal.CollectOutputs(outputJSON, deploymentID, c.app)
	if err != nil {
		log.Printf("[UniversalDeploy] Failed to collect outputs: %v", err)
	}

	endpoint := ""
	if outputs != nil {
		endpoint = outputs.AppEndpoint
	}

	watcher.BroadcastDeploymentStatus(deploymentID, "active",
		fmt.Sprintf("Deployment complete! Endpoint: %s", endpoint))
}

// failDeployment marks a deployment as failed and broadcasts the error
func (c *UniversalDeployController) failDeployment(deploymentID, errMsg string) {
	log.Printf("[UniversalDeploy] Deployment %s failed: %s", deploymentID, errMsg)

	deployment, err := c.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		return
	}
	deployment.Set("status", "failed")
	deployment.Set("errorMessage", errMsg)
	c.app.Dao().SaveRecord(deployment)

	watcher.BroadcastDeploymentStatus(deploymentID, "failed", errMsg)
}

// coalesceStr returns the first non-empty string
func coalesceStr(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
