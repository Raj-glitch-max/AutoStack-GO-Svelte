package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/terraform"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/util"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/models"
)

type AWSDeploymentController struct {
	app             *pocketbase.PocketBase
	credManager     *aws.CredentialManager
	terraformExec   *terraform.ExecutorService
	logStreamer     terraform.LogStreamer
	// WEEK 2 TASK 2.4: Confirmation timeout tracking
	planConfirmations map[string]*planConfirmation
}

type CreateAWSDeploymentRequest struct {
	Name          string                 `json:"name"`
	ProjectID     string                 `json:"project"`
	BlueprintID   string                 `json:"blueprint"`
	Region        string                 `json:"region"`
	Configuration map[string]interface{} `json:"configuration"`
}

// WEEK 2 TASK 2.4: Plan confirmation tracking
type planConfirmation struct {
	deploymentID string
	userID       string
	createdAt    time.Time
	timer        *time.Timer
}

type AWSDeploymentResponse struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Status        string                 `json:"status"`
	Region        string                 `json:"region"`
	Configuration map[string]interface{} `json:"configuration"`
	Outputs       map[string]interface{} `json:"outputs"`
	Created       time.Time              `json:"created"`
	Updated       time.Time              `json:"updated"`
}

func NewAWSDeploymentController(app *pocketbase.PocketBase, credManager *aws.CredentialManager, terraformExec *terraform.ExecutorService, logStreamer terraform.LogStreamer) *AWSDeploymentController {
	return &AWSDeploymentController{
		app:               app,
		credManager:       credManager,
		terraformExec:     terraformExec,
		logStreamer:       logStreamer,
		planConfirmations: make(map[string]*planConfirmation),
	}
}

// HandleAWSDeploymentCreate handles the creation of AWS deployments
// WEEK 3 TASK 3.1: Prevents concurrent deployments for the same user
func (c *AWSDeploymentController) HandleAWSDeploymentCreate(ctx echo.Context) error {
	var req CreateAWSDeploymentRequest
	if err := ctx.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	// Get authenticated user
	user := ctx.Get("user").(*models.Record)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "Authentication required")
	}

	// WEEK 3 TASK 3.1: Check for concurrent deployments
	// Prevent Terraform state lock conflicts by allowing only one active deployment per user
	hasActiveDeployment, err := c.hasActiveDeployment(user.Id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to check deployment status")
	}
	if hasActiveDeployment {
		return echo.NewHTTPError(http.StatusConflict, map[string]interface{}{
			"error": map[string]string{
				"code":    "DEPLOYMENT_IN_PROGRESS",
				"message": "Wait for current deployment to complete",
			},
		})
	}

	// Validate user has AWS credentials
	hasValidCreds, err := c.credManager.HasValidCredentials(user.Id)
	if err != nil || !hasValidCreds {
		return echo.NewHTTPError(http.StatusBadRequest, "Valid AWS credentials required")
	}

	// Validate project access
	project, err := c.app.Dao().FindRecordById("projects", req.ProjectID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Project not found")
	}
	if project.GetString("user") != user.Id {
		return echo.NewHTTPError(http.StatusForbidden, "Access denied to project")
	}

	// Get blueprint
	blueprint, err := c.app.Dao().FindRecordById("awsBlueprints", req.BlueprintID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Blueprint not found")
	}

	// Validate blueprint access
	if !blueprint.GetBool("public") && blueprint.GetString("owner") != user.Id {
		return echo.NewHTTPError(http.StatusForbidden, "Access denied to blueprint")
	}

	// Create deployment record
	collection, err := c.app.Dao().FindCollectionByNameOrId("awsDeployments")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to find collection")
	}

	deploymentID := util.GenerateId(15)
	deployment := models.NewRecord(collection)
	deployment.Set("id", deploymentID)
	deployment.Set("name", req.Name)
	deployment.Set("project", req.ProjectID)
	deployment.Set("user", user.Id)
	deployment.Set("blueprint", req.BlueprintID)
	deployment.Set("region", req.Region)
	deployment.Set("status", "pending")
	deployment.Set("configuration", req.Configuration)

	if err := c.app.Dao().SaveRecord(deployment); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create deployment")
	}

	// Start initial rollout asynchronously
	go c.createInitialRollout(deploymentID, user.Id, req.ProjectID, blueprint, req.Configuration)

	return ctx.JSON(http.StatusCreated, map[string]interface{}{
		"id":     deploymentID,
		"status": "pending",
	})
}

// hasActiveDeployment checks if user has any deployment in progress
// WEEK 3 TASK 3.1: Concurrent deployment prevention
func (c *AWSDeploymentController) hasActiveDeployment(userID string) (bool, error) {
	// Check for deployments in active states
	activeStatuses := []string{"planning", "planned", "applying"}
	
	for _, status := range activeStatuses {
		deployments, err := c.app.Dao().FindRecordsByExpr("awsDeployments",
			dbx.NewExp("user = {:user} AND status = {:status}",
				dbx.Params{"user": userID, "status": status}))
		if err != nil {
			return false, err
		}
		if len(deployments) > 0 {
			return true, nil
		}
	}
	
	return false, nil
}

// HandleAWSDeploymentList lists AWS deployments for a project
func (c *AWSDeploymentController) HandleAWSDeploymentList(ctx echo.Context) error {
	user := ctx.Get("user").(*models.Record)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "Authentication required")
	}

	projectID := ctx.PathParam("projectId")
	if projectID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Project ID required")
	}

	// Validate project access
	project, err := c.app.Dao().FindRecordById("projects", projectID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Project not found")
	}
	if project.GetString("user") != user.Id {
		return echo.NewHTTPError(http.StatusForbidden, "Access denied to project")
	}

	// Get deployments
	deployments, err := c.app.Dao().FindRecordsByExpr("awsDeployments", 
		dbx.NewExp("project = {:project} AND user = {:user}", 
			dbx.Params{"project": projectID, "user": user.Id}))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch deployments")
	}

	var response []AWSDeploymentResponse
	for _, deployment := range deployments {
		var config map[string]interface{}
		var outputs map[string]interface{}
		
		if configStr := deployment.GetString("configuration"); configStr != "" {
			json.Unmarshal([]byte(configStr), &config)
		}
		
		if outputsStr := deployment.GetString("outputs"); outputsStr != "" {
			json.Unmarshal([]byte(outputsStr), &outputs)
		}

		response = append(response, AWSDeploymentResponse{
			ID:            deployment.Id,
			Name:          deployment.GetString("name"),
			Status:        deployment.GetString("status"),
			Region:        deployment.GetString("region"),
			Configuration: config,
			Outputs:       outputs,
			Created:       deployment.Created.Time(),
			Updated:       deployment.Updated.Time(),
		})
	}

	return ctx.JSON(http.StatusOK, response)
}

// HandleAWSDeploymentGet gets a specific AWS deployment
func (c *AWSDeploymentController) HandleAWSDeploymentGet(ctx echo.Context) error {
	user := ctx.Get("user").(*models.Record)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "Authentication required")
	}

	deploymentID := ctx.PathParam("deploymentId")
	deployment, err := c.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Deployment not found")
	}

	if deployment.GetString("user") != user.Id {
		return echo.NewHTTPError(http.StatusForbidden, "Access denied")
	}

	var config map[string]interface{}
	var outputs map[string]interface{}
	
	if configStr := deployment.GetString("configuration"); configStr != "" {
		json.Unmarshal([]byte(configStr), &config)
	}
	
	if outputsStr := deployment.GetString("outputs"); outputsStr != "" {
		json.Unmarshal([]byte(outputsStr), &outputs)
	}

	response := AWSDeploymentResponse{
		ID:            deployment.Id,
		Name:          deployment.GetString("name"),
		Status:        deployment.GetString("status"),
		Region:        deployment.GetString("region"),
		Configuration: config,
		Outputs:       outputs,
		Created:       deployment.Created.Time(),
		Updated:       deployment.Updated.Time(),
	}

	return ctx.JSON(http.StatusOK, response)
}

// HandleAWSDeploymentDelete deletes an AWS deployment
func (c *AWSDeploymentController) HandleAWSDeploymentDelete(ctx echo.Context) error {
	user := ctx.Get("user").(*models.Record)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "Authentication required")
	}

	deploymentID := ctx.PathParam("deploymentId")
	deployment, err := c.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Deployment not found")
	}

	if deployment.GetString("user") != user.Id {
		return echo.NewHTTPError(http.StatusForbidden, "Access denied")
	}

	// Check if deployment is active - if so, require destroy first
	if deployment.GetString("status") == "active" {
		return echo.NewHTTPError(http.StatusBadRequest, "Active deployment must be destroyed before deletion")
	}

	// Delete all related rollouts
	rollouts, err := c.app.Dao().FindRecordsByExpr("awsRollouts",
		dbx.NewExp("deployment = {:deployment}", dbx.Params{"deployment": deploymentID}))
	if err == nil {
		for _, rollout := range rollouts {
			c.app.Dao().DeleteRecord(rollout)
		}
	}

	// Delete deployment
	if err := c.app.Dao().DeleteRecord(deployment); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to delete deployment")
	}

	// Cleanup working directory
	go c.terraformExec.CleanupWorkingDir(deploymentID)

	return ctx.NoContent(http.StatusNoContent)
}

// HandleAWSDeploymentPlan generates a Terraform plan
// WEEK 2 TASK 2.1: Implements confirmation gate (planning → planned → waiting for user confirmation)
func (c *AWSDeploymentController) HandleAWSDeploymentPlan(ctx echo.Context) error {
	user := ctx.Get("user").(*models.Record)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "Authentication required")
	}

	deploymentID := ctx.PathParam("deploymentId")
	deployment, err := c.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Deployment not found")
	}

	if deployment.GetString("user") != user.Id {
		return echo.NewHTTPError(http.StatusForbidden, "Access denied")
	}

	// Get blueprint
	blueprint, err := c.app.Dao().FindRecordById("awsBlueprints", deployment.GetString("blueprint"))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Blueprint not found")
	}

	// Parse configuration
	var config map[string]interface{}
	if configStr := deployment.GetString("configuration"); configStr != "" {
		json.Unmarshal([]byte(configStr), &config)
	}

	// Create Terraform config
	tfConfig := terraform.TerraformConfig{
		Template:     blueprint.GetString("terraformTemplate"),
		Variables:    config,
		UserID:       user.Id,
		ProjectID:    deployment.GetString("project"),
		DeploymentID: deploymentID,
	}

	// WEEK 2 TASK 2.1: Update deployment status to "planning"
	deployment.Set("status", "planning")
	c.app.Dao().SaveRecord(deployment)

	// Execute plan
	result, err := c.terraformExec.Plan(context.Background(), deploymentID, user.Id, tfConfig, c.logStreamer)
	if err != nil {
		deployment.Set("status", "failed")
		deployment.Set("errorMessage", fmt.Sprintf("Plan failed: %v", err))
		c.app.Dao().SaveRecord(deployment)
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Plan failed: %v", err))
	}

	if result.ExitCode != 0 {
		deployment.Set("status", "failed")
		deployment.Set("errorMessage", "Terraform plan failed")
		c.app.Dao().SaveRecord(deployment)
		return echo.NewHTTPError(http.StatusInternalServerError, "Terraform plan failed")
	}

	// WEEK 2 TASK 2.1: Update status to "planned" - waiting for user confirmation
	deployment.Set("status", "planned")
	c.app.Dao().SaveRecord(deployment)

	// WEEK 2 TASK 2.4: Start confirmation timeout (10 minutes)
	c.startConfirmationTimeout(deploymentID, user.Id)

	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"exitCode": result.ExitCode,
		"output":   result.Output,
		"duration": result.Duration.String(),
		"status":   "planned",
		"message":  "Plan complete - waiting for user confirmation",
	})
}

// startConfirmationTimeout starts a 10-minute timeout for plan confirmation
// WEEK 2 TASK 2.4: Add confirmation timeout
func (c *AWSDeploymentController) startConfirmationTimeout(deploymentID, userID string) {
	// Cancel any existing timeout for this deployment
	if existing, ok := c.planConfirmations[deploymentID]; ok {
		if existing.timer != nil {
			existing.timer.Stop()
		}
	}

	// Create new confirmation tracking
	confirmation := &planConfirmation{
		deploymentID: deploymentID,
		userID:       userID,
		createdAt:    time.Now(),
	}

	// Set 10-minute timeout
	confirmation.timer = time.AfterFunc(10*time.Minute, func() {
		c.handleConfirmationTimeout(deploymentID)
	})

	c.planConfirmations[deploymentID] = confirmation
}

// handleConfirmationTimeout handles the case when user doesn't confirm within 10 minutes
// WEEK 2 TASK 2.4: Add confirmation timeout
func (c *AWSDeploymentController) handleConfirmationTimeout(deploymentID string) {
	deployment, err := c.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		log.Printf("Failed to find deployment %s for timeout: %v", deploymentID, err)
		return
	}

	// Only timeout if still in "planned" status
	if deployment.GetString("status") == "planned" {
		deployment.Set("status", "failed")
		deployment.Set("errorMessage", "Plan expired - user did not confirm within 10 minutes")
		c.app.Dao().SaveRecord(deployment)

		// Cleanup working directory
		go c.terraformExec.CleanupWorkingDir(deploymentID)

		// Send WebSocket notification if streamer available
		if c.logStreamer != nil {
			c.logStreamer.StreamLog(deploymentID, "", "timeout", "error", 
				"Plan expired - user did not confirm within 10 minutes")
		}

		log.Printf("Deployment %s timed out - plan not confirmed within 10 minutes", deploymentID)
	}

	// Remove from tracking
	delete(c.planConfirmations, deploymentID)
}

// HandleAWSDeploymentApply applies the Terraform configuration
// WEEK 2 TASK 2.1: Implements confirmation gate - user must explicitly confirm plan
func (c *AWSDeploymentController) HandleAWSDeploymentApply(ctx echo.Context) error {
	user := ctx.Get("user").(*models.Record)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "Authentication required")
	}

	deploymentID := ctx.PathParam("deploymentId")
	deployment, err := c.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Deployment not found")
	}

	if deployment.GetString("user") != user.Id {
		return echo.NewHTTPError(http.StatusForbidden, "Access denied")
	}

	// WEEK 2 TASK 2.1: Verify deployment is in "planned" status
	// This ensures user has seen the plan before applying
	if deployment.GetString("status") != "planned" {
		return echo.NewHTTPError(http.StatusBadRequest, 
			fmt.Sprintf("Cannot apply deployment in status '%s'. Must be in 'planned' status.", 
				deployment.GetString("status")))
	}

	// WEEK 2 TASK 2.4: Cancel confirmation timeout since user confirmed
	if confirmation, ok := c.planConfirmations[deploymentID]; ok {
		if confirmation.timer != nil {
			confirmation.timer.Stop()
		}
		delete(c.planConfirmations, deploymentID)
	}

	// WEEK 2 TASK 2.1: Update status to "applying"
	deployment.Set("status", "applying")
	c.app.Dao().SaveRecord(deployment)

	// Create new rollout for apply operation
	rolloutID := util.GenerateId(15)
	go c.executeApply(deploymentID, rolloutID, user.Id)

	return ctx.JSON(http.StatusAccepted, map[string]interface{}{
		"rolloutId": rolloutID,
		"status":    "applying",
		"message":   "Apply started - infrastructure is being created",
	})
}

// HandleAWSDeploymentDestroy destroys the AWS infrastructure
func (c *AWSDeploymentController) HandleAWSDeploymentDestroy(ctx echo.Context) error {
	user := ctx.Get("user").(*models.Record)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "Authentication required")
	}

	deploymentID := ctx.PathParam("deploymentId")
	deployment, err := c.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Deployment not found")
	}

	if deployment.GetString("user") != user.Id {
		return echo.NewHTTPError(http.StatusForbidden, "Access denied")
	}

	// Update deployment status
	deployment.Set("status", "destroying")
	c.app.Dao().SaveRecord(deployment)

	// Execute destroy asynchronously
	rolloutID := util.GenerateId(15)
	go c.executeDestroy(deploymentID, rolloutID, user.Id)

	return ctx.JSON(http.StatusAccepted, map[string]interface{}{
		"status": "destroying",
	})
}

// createInitialRollout creates the initial rollout for a deployment
func (c *AWSDeploymentController) createInitialRollout(deploymentID, userID, projectID string, blueprint *models.Record, config map[string]interface{}) {
	// Create rollout record
	collection, err := c.app.Dao().FindCollectionByNameOrId("awsRollouts")
	if err != nil {
		log.Printf("Failed to find awsRollouts collection: %v", err)
		return
	}

	rolloutID := util.GenerateId(15)
	rollout := models.NewRecord(collection)
	rollout.Set("id", rolloutID)
	rollout.Set("deployment", deploymentID)
	rollout.Set("project", projectID)
	rollout.Set("user", userID)
	rollout.Set("terraformConfig", blueprint.GetString("terraformTemplate"))
	rollout.Set("status", "planning")
	rollout.Set("startDate", time.Now())

	if err := c.app.Dao().SaveRecord(rollout); err != nil {
		log.Printf("Failed to create rollout: %v", err)
		return
	}

	// Initialize Terraform
	if _, err := c.terraformExec.Init(context.Background(), deploymentID, userID, c.logStreamer); err != nil {
		log.Printf("Terraform init failed: %v", err)
		c.updateDeploymentStatus(deploymentID, "failed", fmt.Sprintf("Init failed: %v", err))
		return
	}

	c.updateDeploymentStatus(deploymentID, "planned", "")
}

// executeApply executes the Terraform apply operation
// WEEK 2 TASK 2.2: Auto-destroy on apply failure is handled in executor.Apply()
func (c *AWSDeploymentController) executeApply(deploymentID, rolloutID, userID string) {
	// Add panic recovery as per master plan error handling rules
	defer func() {
		if r := recover(); r != nil {
			log.Printf("deployment goroutine panicked: deploymentID=%s panic=%v", deploymentID, r)
			c.updateDeploymentStatus(deploymentID, "failed", "internal error")
			if c.logStreamer != nil {
				c.logStreamer.StreamLog(deploymentID, rolloutID, "apply", "error", "internal error")
			}
		}
	}()

	c.updateDeploymentStatus(deploymentID, "applying", "")

	// WEEK 2 TASK 2.2: Apply now includes auto-destroy on failure
	result, err := c.terraformExec.Apply(context.Background(), deploymentID, rolloutID, userID, true, c.logStreamer)
	if err != nil || result.ExitCode != 0 {
		log.Printf("Terraform apply failed: %v", err)
		errorMsg := "Terraform apply failed"
		if err != nil {
			errorMsg = fmt.Sprintf("Terraform apply failed: %v", err)
		}
		c.updateDeploymentStatus(deploymentID, "failed", errorMsg)
		return
	}

	// Get outputs
	outputs, err := c.terraformExec.GetOutputs(context.Background(), deploymentID, userID)
	if err != nil {
		log.Printf("Failed to get Terraform outputs: %v", err)
	}

	// Update deployment with outputs
	deployment, err := c.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err == nil {
		outputsJSON, _ := json.Marshal(outputs)
		deployment.Set("outputs", string(outputsJSON))
		deployment.Set("status", "active")
		deployment.Set("errorMessage", "")
		c.app.Dao().SaveRecord(deployment)
	}
}

// executeDestroy executes the Terraform destroy operation
func (c *AWSDeploymentController) executeDestroy(deploymentID, rolloutID, userID string) {
	// Add panic recovery as per master plan error handling rules
	defer func() {
		if r := recover(); r != nil {
			log.Printf("destroy goroutine panicked: deploymentID=%s panic=%v", deploymentID, r)
			c.updateDeploymentStatus(deploymentID, "failed", "internal error during destroy")
			if c.logStreamer != nil {
				c.logStreamer.StreamLog(deploymentID, rolloutID, "destroy", "error", "internal error")
			}
		}
	}()

	result, err := c.terraformExec.Destroy(context.Background(), deploymentID, rolloutID, userID, true, c.logStreamer)
	if err != nil || result.ExitCode != 0 {
		log.Printf("Terraform destroy failed: %v", err)
		errorMsg := "Terraform destroy failed"
		if err != nil {
			errorMsg = fmt.Sprintf("Terraform destroy failed: %v", err)
		}
		c.updateDeploymentStatus(deploymentID, "failed", errorMsg)
		return
	}

	c.updateDeploymentStatus(deploymentID, "destroyed", "")
}

// updateDeploymentStatus updates the deployment status
func (c *AWSDeploymentController) updateDeploymentStatus(deploymentID, status string, errorMessage string) {
	deployment, err := c.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		log.Printf("Failed to find deployment %s: %v", deploymentID, err)
		return
	}

	deployment.Set("status", status)
	if errorMessage != "" {
		deployment.Set("errorMessage", errorMessage)
	}
	if err := c.app.Dao().SaveRecord(deployment); err != nil {
		log.Printf("Failed to update deployment status: %v", err)
	}
}

// WEEK 3 TASK 3.1: Test helper for concurrent deployment prevention
// This would be used in integration tests to verify the 409 response
