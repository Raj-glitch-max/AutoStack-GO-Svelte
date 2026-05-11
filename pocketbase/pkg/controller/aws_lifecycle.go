package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/terraform"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/util"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/notifications"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/watcher"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
)

type AWSLifecycleController struct {
	app           *pocketbase.PocketBase
	terraformExec *terraform.ExecutorService
	credManager   *aws.CredentialManager
	webhookService *notifications.WebhookService
}

func NewAWSLifecycleController(app *pocketbase.PocketBase, terraformExec *terraform.ExecutorService, credManager *aws.CredentialManager, webhookService *notifications.WebhookService) *AWSLifecycleController {
	return &AWSLifecycleController{
		app:           app,
		terraformExec: terraformExec,
		credManager:   credManager,
		webhookService: webhookService,
	}
}

func (c *AWSLifecycleController) RegisterRoutes(e *core.ServeEvent) {
	subGroup := e.Router.Group("/api/aws/deployments", apis.RequireRecordAuth())

	subGroup.POST("/:id/deploy", c.HandleDeploy, RBACMiddleware(c.app, RoleDeployer))
	subGroup.POST("/:id/destroy", c.HandleDestroy, RBACMiddleware(c.app, RoleDeployer))
	subGroup.GET("/:id/status", c.HandleGetStatus, RBACMiddleware(c.app, RoleViewer))
}

// HandleDeploy triggers the Plan -> Apply workflow
func (c *AWSLifecycleController) HandleDeploy(ctx echo.Context) error {
	deploymentID := ctx.PathParam("id")
	user, _ := ctx.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	// Fetch deployment
	deployment, err := c.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		return apis.NewNotFoundError("Deployment not found", err)
	}

	if deployment.GetString("user") != user.Id {
		return apis.NewForbiddenError("Access denied", nil)
	}

	// Get blueprint
	blueprint, err := c.app.Dao().FindRecordById("awsBlueprints", deployment.GetString("blueprint"))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Blueprint not found", err)
	}

	// Parse configuration
	var config map[string]interface{}
	if configStr := deployment.GetString("configuration"); configStr != "" {
		json.Unmarshal([]byte(configStr), &config)
	}

	tfConfig := terraform.TerraformConfig{
		Template:     blueprint.GetString("terraformTemplate"),
		Variables:    config,
		UserID:       user.Id,
		ProjectID:    deployment.GetString("project"),
		DeploymentID: deploymentID,
	}

	// Create rollout
	rolloutID := util.GenerateId(15)
	
	// Start deployment in background with queuing
	go func() {
		// Serialize deploys for this user
		unlock := c.terraformExec.LockUser(user.Id)
		defer unlock()

		// Stream initial status
		c.updateStatus(deploymentID, "deploying", "Starting deployment sequence...")

		// Use intelligent recovery engine for the execution
		// Note: The LogStreamer should be passed here. For now using the app's default streamer if available.
		// In a real scenario, we'd inject the CompositeLogStreamer.
		result, err := c.terraformExec.ExecuteWithIntelligentRecovery(
			context.Background(),
			deploymentID,
			rolloutID,
			user.Id,
			tfConfig,
			nil, // LogStreamer will be handled by watcher/websocket in production
		)

		if err != nil {
			c.updateStatus(deploymentID, "failed", err.Error())
			return
		}

		if result.ExitCode == 0 {
			// Fetch outputs to update record
			outputs, _ := c.terraformExec.GetOutputs(context.Background(), deploymentID, user.Id)
			outputsJSON, _ := json.Marshal(outputs)
			deployment.Set("outputs", string(outputsJSON))
			deployment.Set("status", "active")
			c.app.Dao().SaveRecord(deployment)
			
			c.updateStatus(deploymentID, "active", "Deployment completed successfully")
		} else {
			c.updateStatus(deploymentID, "failed", "Terraform execution failed")
		}
	}()

	return ctx.JSON(http.StatusAccepted, map[string]interface{}{
		"id":        deploymentID,
		"rolloutId": rolloutID,
		"status":    "queued",
	})
}

// HandleDestroy triggers the infrastructure teardown
func (c *AWSLifecycleController) HandleDestroy(ctx echo.Context) error {
	deploymentID := ctx.PathParam("id")
	user, _ := ctx.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return apis.NewUnauthorizedError("Authentication required", nil)
	}

	deployment, err := c.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		return apis.NewNotFoundError("Deployment not found", err)
	}

	if deployment.GetString("user") != user.Id {
		return apis.NewForbiddenError("Access denied", nil)
	}

	rolloutID := util.GenerateId(15)

	go func() {
		unlock := c.terraformExec.LockUser(user.Id)
		defer unlock()

		c.updateStatus(deploymentID, "destroying", "Starting infrastructure teardown...")

		result, err := c.terraformExec.Destroy(context.Background(), deploymentID, rolloutID, user.Id, true, nil)
		if err != nil {
			c.updateStatus(deploymentID, "failed", fmt.Sprintf("Teardown failed: %v", err))
			return
		}

		if result.ExitCode == 0 {
			c.updateStatus(deploymentID, "destroyed", "Infrastructure destroyed successfully")
		} else {
			c.updateStatus(deploymentID, "failed", "Teardown failed")
		}
	}()

	return ctx.JSON(http.StatusAccepted, map[string]interface{}{
		"id":     deploymentID,
		"status": "queued_destroy",
	})
}

func (c *AWSLifecycleController) HandleGetStatus(ctx echo.Context) error {
	deploymentID := ctx.PathParam("id")
	deployment, err := c.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		return apis.NewNotFoundError("Deployment not found", err)
	}

	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"status":       deployment.GetString("status"),
		"errorMessage": deployment.GetString("errorMessage"),
		"updated":      deployment.Updated.Time(),
	})
}

func (c *AWSLifecycleController) updateStatus(deploymentID, status, message string) {
	deployment, err := c.app.Dao().FindRecordById("awsDeployments", deploymentID)
	if err != nil {
		return
	}

	deployment.Set("status", status)
	deployment.Set("errorMessage", message)
	c.app.Dao().SaveRecord(deployment)

	// Broadcast via WebSocket
	watcher.BroadcastDeploymentStatus(deploymentID, status, message)

	// Dispatch webhook event
	if c.webhookService != nil {
		userID := deployment.GetString("user")
		eventType := notifications.WebhookEventType("deploy." + status)
		c.webhookService.TriggerEvent(eventType, userID, map[string]interface{}{
			"deployment_id":   deploymentID,
			"deployment_name": deployment.GetString("name"),
			"status":          status,
			"message":         message,
		})
	}
}
