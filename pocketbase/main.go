package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/controller"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/env"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/k8s"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/notifications"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/health"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/intelligence"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/terraform"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/watcher"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/plugins/jsvm"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/pocketbase/pocketbase/tools/cron"
)

func defaultPublicDir() string {
	if strings.HasPrefix(os.Args[0], os.TempDir()) {
		// most likely ran with go run
		return "./pb_public"
	}

	return filepath.Join(os.Args[0], "../pb_public")
}

func init() {
	env.Init()
	k8s.Init()
}

func main() {
	app := pocketbase.New()

	var publicDirFlag string

	// add "--publicDir" option flag
	app.RootCmd.PersistentFlags().StringVar(
		&publicDirFlag,
		"publicDir",
		defaultPublicDir(),
		"the directory to serve static files",
	)
	migrationsDir := "" // default to "pb_migrations" (for js) and "migrations" (for go)

	// Initialize AWS services.
	// The encryption key MUST be loaded from the environment — never generated at
	// runtime. A random key means credentials become unreadable after every restart.
	encryptionKeyHex := env.Config.EncryptionKeyHex
	if encryptionKeyHex == "" {
		log.Fatal("[FATAL] AUTOSTACK_ENCRYPTION_KEY environment variable is not set. " +
			"Generate one with: openssl rand -hex 32")
	}
	encryptionKey, err := hex.DecodeString(encryptionKeyHex)
	if err != nil || len(encryptionKey) != 32 {
		log.Fatal("[FATAL] AUTOSTACK_ENCRYPTION_KEY must be a 64-character hex string (32 bytes). " +
			"Generate one with: openssl rand -hex 32")
	}

	credManager := aws.NewCredentialManager(encryptionKey, app)
	terraformExec := terraform.NewExecutorService(env.Config.TerraformWorkDir, credManager)
	
	// Use WebSocket log streamer for real-time logs
	logStreamer := watcher.GetAWSLogStreamer()
	
	webhookService := notifications.NewWebhookService(app)
	awsController := controller.NewAWSDeploymentController(app, credManager, terraformExec, logStreamer)
	awsLifecycleController := controller.NewAWSLifecycleController(app, terraformExec, credManager, webhookService)
	
	// Initialize Email Service
	emailService := notifications.NewEmailService()
	
	// Initialize Actual Cost Controller
	actualCostController, err := controller.NewActualCostController(app, emailService)
	if err != nil {
		log.Printf("[Error] Failed to initialize ActualCostController: %v", err)
	}
	infracostAPIKey := os.Getenv("INFRACOST_API_KEY")
	if infracostAPIKey == "" {
		log.Println("[WARNING] INFRACOST_API_KEY not set - cost estimation will be unavailable")
	}
	infracostService := aws.NewInfracostService(infracostAPIKey)
	
	// Initialize cost controllers
	// costEstimateController := controller.NewCostEstimateController(app)

	// load js files to allow loading external JavaScript migrations
	jsvm.MustRegister(app, jsvm.Config{
		// Dir: migrationsDir,
		MigrationsDir: migrationsDir,
	})

	// register the `migrate` command
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		TemplateLang: migratecmd.TemplateLangJS, // or migratecmd.TemplateLangGo (default)
		Dir:          migrationsDir,
		Automigrate:  true,
	})

	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		// serves static files from the provided public dir (if exists)
		e.Router.GET("/*", apis.StaticDirectoryHandler(os.DirFS(publicDirFlag), true))

		return nil
	})

	app.OnRecordBeforeCreateRequest().Add(func(e *core.RecordCreateEvent) error {
		switch e.Collection.Name {
		case "rollouts":
			return controller.HandleRolloutCreate(e, app)
		}
		return nil
	})

	app.OnRecordBeforeUpdateRequest().Add(func(e *core.RecordUpdateEvent) error {
		switch e.Collection.Name {
		case "rollouts":
			return controller.HandleRolloutUpdate(e, app)
		}
		return nil
	})

	app.OnRecordBeforeDeleteRequest().Add(func(e *core.RecordDeleteEvent) error {
		switch e.Collection.Name {
		case "rollouts":
			return controller.HandleRolloutDelete(e, app)
		case "deployments":
			return controller.HandleDeploymentDelete(e, app)
		case "projects":
			return controller.HandleProjectDelete(e, app)
		}
		return nil
	})

	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		// get status of a specific deployment
		e.Router.GET("/pb/:projectId/:deploymentId/status", func(c echo.Context) error {
			projectId := c.PathParam("projectId")
			deploymentId := c.PathParam("deploymentId")

			return controller.HandleDeploymentStatus(c, app, projectId, deploymentId)
		}, apis.RequireRecordAuth("users"))

		e.Router.GET("/pb/:projectId/:deploymentId/metrics", func(c echo.Context) error {
			projectId := c.PathParam("projectId")
			deploymentId := c.PathParam("deploymentId")

			return controller.HandleDeploymentMetrics(c, app, projectId, deploymentId)
		}, apis.RequireRecordAuth("users"))

		e.Router.GET("/pb/:projectId/:deploymentId/events", func(c echo.Context) error {
			projectId := c.PathParam("projectId")
			deploymentId := c.PathParam("deploymentId")

			return controller.HandleDeploymentEvents(c, app, projectId, deploymentId)
		}, apis.RequireRecordAuth("users"))

		e.Router.GET("/pb/:projectId/:podName/logs", func(c echo.Context) error {
			projectId := c.PathParam("projectId")
			podName := c.PathParam("podName")

			return k8s.GetRolloutLogs(c.Response().Writer, projectId, podName)
		}, apis.RequireRecordAuth("users"))

		e.Router.GET("/pb/blueprints/:blueprintId", func(c echo.Context) error {
			blueprintId := c.PathParam("blueprintId")

			return controller.HandleBlueprint(c, app, blueprintId)
		}, apis.RequireRecordAuth("users"))

		e.Router.POST("/pb/blueprints/shared/:blueprintId", func(c echo.Context) error {
			blueprintId := c.PathParam("blueprintId")

			return controller.HandleBlueprintAdd(c, app, blueprintId)
		}, apis.RequireRecordAuth("users"))

		e.Router.POST("/pb/auto-update/:autoUpdateId", func(c echo.Context) error {
			// TODO: change the auth to be a token generated by the user
			autoUpdateId := c.PathParam("autoUpdateId")

			return controller.HandleAutoUpdate(c, app, autoUpdateId)
		})

		e.Router.GET("/pb/cluster-info", func(c echo.Context) error {
			return controller.HandleClusterInfo(c, app)
		}, apis.RequireRecordAuth("users"))

		// delete a pod of a rollout by pod name
		e.Router.DELETE("/pb/:projectId/pod/:podName", func(c echo.Context) error {
			projectId := c.PathParam("projectId")
			podName := c.PathParam("podName")

			return controller.HandlePodDelete(c, app, projectId, podName)
		}, apis.RequireRecordAuth("users"))

		// delete a job
		e.Router.DELETE("/pb/:projectId/job/:jobId", func(c echo.Context) error {
			projectId := c.PathParam("projectId")
			jobId := c.PathParam("jobId")

			return controller.HandleJobDelete(c, app, projectId, jobId)
		}, apis.RequireRecordAuth("users"))

		// websocket for deployments status
		e.Router.GET("/ws/k8s/deployments", watcher.WsK8sDeploymentsHandler)

		// websocket for pod logs
		e.Router.GET("/ws/k8s/logs", watcher.WsK8sRolloutLogsHandler)

		// websocket for rollout events
		e.Router.GET("/ws/k8s/events", watcher.WsK8sRolloutEventsHandler)

		// AWS WebSocket endpoints
		e.Router.GET("/ws/aws/logs", watcher.WsAWSTerraformLogsHandler)
		e.Router.GET("/ws/aws/status", watcher.WsAWSDeploymentStatusHandler)

		// AWS Deployment Routes
		e.Router.POST("/api/aws/deployments", awsController.HandleAWSDeploymentCreate, apis.RequireRecordAuth("users"))
		e.Router.GET("/api/aws/deployments/:projectId", awsController.HandleAWSDeploymentList, apis.RequireRecordAuth("users"), controller.RBACMiddleware(app, controller.RoleViewer))
		e.Router.GET("/api/aws/deployments/:deploymentId", awsController.HandleAWSDeploymentGet, apis.RequireRecordAuth("users"), controller.RBACMiddleware(app, controller.RoleViewer))
		e.Router.DELETE("/api/aws/deployments/:deploymentId", awsController.HandleAWSDeploymentDelete, apis.RequireRecordAuth("users"), controller.RBACMiddleware(app, controller.RoleOwner))
		e.Router.POST("/api/aws/deployments/:deploymentId/plan", awsController.HandleAWSDeploymentPlan, apis.RequireRecordAuth("users"), controller.RBACMiddleware(app, controller.RoleDeployer))
		e.Router.POST("/api/aws/deployments/:deploymentId/apply", awsController.HandleAWSDeploymentApply, apis.RequireRecordAuth("users"), controller.RBACMiddleware(app, controller.RoleDeployer))
		e.Router.POST("/api/aws/deployments/:deploymentId/destroy", awsController.HandleAWSDeploymentDestroy, apis.RequireRecordAuth("users"), controller.RBACMiddleware(app, controller.RoleDeployer))

		// New AWS Lifecycle Routes
		awsLifecycleController.RegisterRoutes(e)

		// AWS Cost Estimation
		e.Router.POST("/api/aws/cost-estimate", func(c echo.Context) error {
			return handleCostEstimate(c, infracostService)
		}, apis.RequireRecordAuth("users"), controller.RateLimitMiddleware(1.0, 5))

		// Intelligence Routes
		controller.RegisterIntelligenceRoutes(app, e.Router)

		// Universal Deployment Engine
		controller.RegisterUniversalDeployRoutes(app, e.Router, credManager, terraformExec)
		
		// AWS & Cost Routes
		controller.RegisterAlertRoutes(e, app)
		controller.RegisterAlertPreferencesRoutes(app, e.Router)

		if actualCostController != nil {
			actualCostController.RegisterActualCostRoutes(e.Router)
		}

		// Webhook Routes
		webhookService := notifications.NewWebhookService(app)
		controller.RegisterWebhookRoutes(app, e.Router, webhookService)

		// Health Routes
		healthMonitor := aws.NewHealthMonitor(app, credManager)
		k8sMonitor := health.NewK8sHealthMonitor(app)
		healthController := controller.NewHealthController(app, healthMonitor, k8sMonitor)
		healthController.RegisterHealthRoutes(e.Router.Group("", apis.RequireRecordAuth("users")))
		
		e.Router.GET("/api/aws/deployments/:deploymentId/rollouts", func(c echo.Context) error {
			return awsController.HandleRolloutList(c)
		}, apis.RequireRecordAuth("users"))

		// AWS Credentials Routes
		e.Router.POST("/api/aws/credentials", func(c echo.Context) error {
			return handleAWSCredentialsCreate(c, credManager)
		}, apis.RequireRecordAuth("users"))
		
		e.Router.POST("/api/aws/credentials/validate", func(c echo.Context) error {
			return handleAWSCredentialsValidate(c, credManager)
		}, apis.RequireRecordAuth("users"))
		
		e.Router.GET("/api/aws/credentials/status", func(c echo.Context) error {
			return handleAWSCredentialsStatus(c, credManager)
		}, apis.RequireRecordAuth("users"))
		
		e.Router.DELETE("/api/aws/credentials", func(c echo.Context) error {
			user, _ := c.Get(apis.ContextAuthRecordKey).(*models.Record)
			if user == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "Authentication required")
			}
			
			err := credManager.DeleteCredentials(user.Id)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to delete credentials")
			}
			
			return c.JSON(http.StatusOK, map[string]string{"message": "Credentials deleted successfully"})
		}, apis.RequireRecordAuth("users"))

		// GCP Credential Routes
		controller.RegisterGCPCredentialRoutes(app, e.Router, encryptionKey)

		// Azure Credential Routes
		controller.RegisterAzureCredentialRoutes(app, e.Router, encryptionKey)

		// Multi-Cloud Pricing Comparison
		controller.RegisterMultiCloudPricingRoutes(app, e.Router)

		return nil
	})

	app.OnAfterBootstrap().Add(func(e *core.BootstrapEvent) error {
		scheduler := cron.New()

		// Run image update every minute
		scheduler.MustAdd("autoUpdate", env.Config.CronTick, func() {
			err := controller.AutoUpdateController(app)
			if err != nil {
				log.Printf("Error updating image: %v\n", err)
			}
		})
		// Run AWS pricing refresh daily at midnight
		scheduler.MustAdd("awsPricingRefresh", "0 0 * * *", func() {
			log.Println("Scheduled task: awsPricingRefresh starting...")
			fetcher, err := aws.NewPricingFetcher(app)
			if err != nil {
				log.Printf("Error creating pricing fetcher: %v\n", err)
				return
			}
			if err := fetcher.FetchAllPricing(); err != nil {
				log.Printf("Error refreshing AWS pricing: %v\n", err)
			}
			log.Println("Scheduled task: awsPricingRefresh completed")
		})

		// Run AWS actual cost fetch daily at 1 AM (after pricing refresh)
		scheduler.MustAdd("awsActualCostFetch", "0 1 * * *", func() {
			log.Println("Scheduled task: awsActualCostFetch starting...")
			fetcher, err := aws.NewActualCostFetcher(app, emailService)
			if err != nil {
				log.Printf("Error creating actual cost fetcher: %v\n", err)
				return
			}
			if err := fetcher.FetchActualCostsForActiveDeployments(); err != nil {
				log.Printf("Error fetching actual costs: %v\n", err)
			}
			log.Println("Scheduled task: awsActualCostFetch completed")
		})

		// Run AI Cost Optimizer weekly on Sundays at 2 AM
		scheduler.MustAdd("aiCostOptimizer", "0 2 * * 0", func() {
			log.Println("Scheduled task: aiCostOptimizer starting...")
			optimizer := intelligence.NewCostOptimizer(app)
			if err := optimizer.RunOptimizationJob(); err != nil {
				log.Printf("Error running cost optimizer: %v\n", err)
			}
			log.Println("Scheduled task: aiCostOptimizer completed")
		})

		scheduler.Start()
		return nil
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

// SimpleLogStreamer implements the LogStreamer interface for basic logging
type SimpleLogStreamer struct{}

func (s *SimpleLogStreamer) StreamLog(deploymentID, rolloutID, operation, level, message string) error {
	log.Printf("[%s] %s/%s %s: %s", level, deploymentID, rolloutID, operation, message)
	return nil
}

// AWS Credentials API handlers
func handleAWSCredentialsCreate(c echo.Context, credManager *aws.CredentialManager) error {
	user, _ := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "Authentication required")
	}

	var req aws.AWSCredentials
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	// Validate credentials first
	validation, err := credManager.ValidateCredentials(req)
	if err != nil || !validation.Valid {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid AWS credentials")
	}

	// Store credentials
	if err := credManager.StoreCredentials(user.Id, req); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to store credentials")
	}

	// Update validation status
	credManager.UpdateValidationStatus(user.Id, true)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Credentials stored successfully",
		"valid":   true,
	})
}

func handleAWSCredentialsValidate(c echo.Context, credManager *aws.CredentialManager) error {
	var req aws.AWSCredentials
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	validation, err := credManager.ValidateCredentials(req)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"valid": false,
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"valid":     validation.Valid,
		"accountId": validation.AccountID,
		"userArn":   validation.UserArn,
		"error":     validation.Error,
	})
}

func handleAWSCredentialsStatus(c echo.Context, credManager *aws.CredentialManager) error {
	user, _ := c.Get(apis.ContextAuthRecordKey).(*models.Record)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "Authentication required")
	}

	hasValid, err := credManager.HasValidCredentials(user.Id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to check credentials")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"hasValidCredentials": hasValid,
	})
}

// handleCostEstimate handles cost estimation requests using Infracost
func handleCostEstimate(c echo.Context, infracostService *aws.InfracostService) error {
	var req struct {
		TerraformCode string `json:"terraform_code"`
		Blueprint     string `json:"blueprint"`
		Region        string `json:"region"`
	}
	
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}
	
	// Validate region
	if req.Region == "" {
		req.Region = "us-east-1" // Default region
	}
	
	// If terraform_code not provided, try to load from blueprint
	if req.TerraformCode == "" && req.Blueprint != "" {
		templatePath := fmt.Sprintf("./pocketbase/templates/%s.tf", req.Blueprint)
		templateBytes, err := os.ReadFile(templatePath)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Blueprint '%s' not found", req.Blueprint))
		}
		req.TerraformCode = string(templateBytes)
	}
	
	if req.TerraformCode == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Either terraform_code or blueprint must be provided")
	}
	
	// Check if Infracost service is available
	if infracostService == nil || os.Getenv("INFRACOST_API_KEY") == "" {
		// Return fallback estimate
		return c.JSON(http.StatusOK, map[string]interface{}{
			"totalMonthlyCost": getFallbackCost(req.Blueprint),
			"breakdown": map[string]float64{
				"Estimated Base Cost": getFallbackCost(req.Blueprint),
			},
			"currency":   "USD",
			"region":     req.Region,
			"disclaimer": "Infracost API key not configured. Showing approximate estimates. Configure INFRACOST_API_KEY for accurate pricing.",
		})
	}
	
	// Call Infracost API
	estimate, err := infracostService.EstimateCost(c.Request().Context(), req.TerraformCode, req.Region)
	if err != nil {
		log.Printf("[Cost Estimate] Infracost API error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to estimate cost: %v", err))
	}
	
	return c.JSON(http.StatusOK, estimate)
}

// getFallbackCost provides fallback cost estimates when Infracost is unavailable
func getFallbackCost(blueprint string) float64 {
	switch blueprint {
	case "ecs-web-app":
		return 32.0
	case "full-stack":
		return 53.0
	case "static-site":
		return 2.0
	case "serverless":
		return 10.0
	default:
		return 25.0
	}
}
