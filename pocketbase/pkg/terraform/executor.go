package terraform

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/aws"
	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/intelligence"
	"github.com/pocketbase/pocketbase/models"
)

type ExecutorService struct {
	workingDir  string
	credManager *aws.CredentialManager
	userLocks   map[string]*sync.Mutex
	mu          sync.Mutex // protects userLocks
}

type ExecutionResult struct {
	ExitCode int
	Output   string
	Error    error
	Duration time.Duration
}

type TerraformConfig struct {
	Template    string
	Variables   map[string]interface{}
	UserID      string
	ProjectID   string
	DeploymentID string
}

type TerraformOutput struct {
	Value     interface{} `json:"value"`
	Type      string      `json:"type"`
	Sensitive bool        `json:"sensitive"`
}

type LogStreamer interface {
	StreamLog(deploymentID, rolloutID, operation, level, message string) error
}

func NewExecutorService(workingDir string, credManager *aws.CredentialManager) *ExecutorService {
	return &ExecutorService{
		workingDir:  workingDir,
		credManager: credManager,
		userLocks:   make(map[string]*sync.Mutex),
	}
}

// LockUser ensures only one deployment runs per user at a time
func (e *ExecutorService) LockUser(userID string) func() {
	e.mu.Lock()
	lock, ok := e.userLocks[userID]
	if !ok {
		lock = &sync.Mutex{}
		e.userLocks[userID] = lock
	}
	e.mu.Unlock()

	lock.Lock()
	return func() {
		lock.Unlock()
	}
}

// Init initializes a Terraform working directory
// SECURITY: Working directory is created with mode 0700 for isolation
func (e *ExecutorService) Init(ctx context.Context, deploymentID, userID string, streamer LogStreamer) (*ExecutionResult, error) {
	workDir := e.getDeploymentWorkingDir(deploymentID)
	
	// SECURITY: Create working directory with restrictive permissions (0700)
	if err := os.MkdirAll(workDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create working directory: %w", err)
	}

	// Get AWS credentials
	creds, err := e.credManager.GetCredentials(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS credentials: %w", err)
	}

	// Configure S3 backend with user-specific path
	if err := e.configureS3Backend(workDir, deploymentID, userID, creds); err != nil {
		return nil, fmt.Errorf("failed to configure S3 backend: %w", err)
	}

	return e.executeTerraformCommand(ctx, workDir, deploymentID, "", "init", streamer, creds, "-no-color", "-input=false", "-reconfigure")
}

// Plan generates a Terraform execution plan.
// Variables are supplied via terraform.tfvars (written by generateTerraformConfig),
// not via -var flags, to avoid shell quoting issues and duplicate-definition errors.
// SECURITY: Working directory is isolated per deployment, no shared state between tenants.
func (e *ExecutorService) Plan(ctx context.Context, deploymentID, userID string, config TerraformConfig, streamer LogStreamer) (*ExecutionResult, error) {
	workDir := e.getDeploymentWorkingDir(deploymentID)

	// Generate Terraform configuration (main.tf + terraform.tfvars).
	if err := e.generateTerraformConfig(workDir, config); err != nil {
		return nil, fmt.Errorf("failed to generate Terraform config: %w", err)
	}

	// Get AWS credentials.
	creds, err := e.credManager.GetCredentials(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS credentials: %w", err)
	}

	// -var-file is not needed here: Terraform automatically loads terraform.tfvars.
	args := []string{"plan", "-no-color", "-input=false", "-out=tfplan"}

	return e.executeTerraformCommand(ctx, workDir, deploymentID, "", "plan", streamer, creds, args...)
}

// Apply applies the Terraform configuration
// SECURITY: Uses tfplan file from Plan step, credentials passed via environment only
// WEEK 2 TASK 2.2: Auto-destroy on apply failure
func (e *ExecutorService) Apply(ctx context.Context, deploymentID, rolloutID, userID string, autoApprove bool, streamer LogStreamer) (*ExecutionResult, error) {
	workDir := e.getDeploymentWorkingDir(deploymentID)

	// Get AWS credentials
	creds, err := e.credManager.GetCredentials(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS credentials: %w", err)
	}

	args := []string{"apply", "-no-color", "-input=false"}
	if autoApprove {
		args = append(args, "-auto-approve")
	}
	args = append(args, "tfplan")

	result, err := e.executeTerraformCommand(ctx, workDir, deploymentID, rolloutID, "apply", streamer, creds, args...)
	
	// WEEK 2 TASK 2.2: Auto-destroy on apply failure
	// If apply fails partway through, run terraform destroy to clean up orphaned resources
	if err != nil || result.ExitCode != 0 {
		if streamer != nil {
			streamer.StreamLog(deploymentID, rolloutID, "apply", "error", "Apply failed - attempting automatic cleanup")
		}
		
		// Attempt auto-destroy to clean up partial resources
		destroyResult := e.attemptAutoDestroy(ctx, workDir, deploymentID, rolloutID, streamer, creds)
		
		if destroyResult.ExitCode != 0 {
			// Destroy also failed - log critical error
			if streamer != nil {
				streamer.StreamLog(deploymentID, rolloutID, "apply", "error", 
					"[CRITICAL] Auto-destroy failed - orphaned resources may exist. Manual cleanup required.")
			}
			return result, fmt.Errorf("apply failed and auto-destroy also failed: %w", err)
		}
		
		if streamer != nil {
			streamer.StreamLog(deploymentID, rolloutID, "apply", "info", "[CLEANUP] Auto-destroy completed successfully")
		}
	}
	
	return result, err
}

// attemptAutoDestroy attempts to destroy infrastructure after a failed apply
// WEEK 2 TASK 2.2: Auto-destroy on apply failure
func (e *ExecutorService) attemptAutoDestroy(ctx context.Context, workDir, deploymentID, rolloutID string, streamer LogStreamer, creds *aws.AWSCredentials) *ExecutionResult {
	if streamer != nil {
		streamer.StreamLog(deploymentID, rolloutID, "cleanup", "info", "[CLEANUP] Starting auto-destroy...")
	}
	
	args := []string{"destroy", "-no-color", "-input=false", "-auto-approve"}
	
	result, err := e.executeTerraformCommand(ctx, workDir, deploymentID, rolloutID, "cleanup", streamer, creds, args...)
	if err != nil {
		if streamer != nil {
			streamer.StreamLog(deploymentID, rolloutID, "cleanup", "error", fmt.Sprintf("[CLEANUP] Destroy error: %v", err))
		}
	}
	
	return result
}

// Destroy destroys the Terraform-managed infrastructure
// SECURITY: Credentials passed via environment only
func (e *ExecutorService) Destroy(ctx context.Context, deploymentID, rolloutID, userID string, autoApprove bool, streamer LogStreamer) (*ExecutionResult, error) {
	workDir := e.getDeploymentWorkingDir(deploymentID)

	// Get AWS credentials
	creds, err := e.credManager.GetCredentials(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS credentials: %w", err)
	}

	args := []string{"destroy", "-no-color", "-input=false"}
	if autoApprove {
		args = append(args, "-auto-approve")
	}

	return e.executeTerraformCommand(ctx, workDir, deploymentID, rolloutID, "destroy", streamer, creds, args...)
}

// GetOutputs retrieves Terraform outputs
// SECURITY: Credentials passed via environment, not stored in process environment
func (e *ExecutorService) GetOutputs(ctx context.Context, deploymentID string, userID string) (map[string]TerraformOutput, error) {
	workDir := e.getDeploymentWorkingDir(deploymentID)

	// Get AWS credentials
	creds, err := e.credManager.GetCredentials(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS credentials: %w", err)
	}

	cmd := exec.CommandContext(ctx, "terraform", "output", "-json")
	cmd.Dir = workDir
	cmd.Env = buildSafeEnv(creds)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get Terraform outputs: %w", err)
	}

	var outputs map[string]TerraformOutput
	if err := json.Unmarshal(output, &outputs); err != nil {
		return nil, fmt.Errorf("failed to parse Terraform outputs: %w", err)
	}

	return outputs, nil
}

// Validate validates the Terraform configuration
func (e *ExecutorService) Validate(ctx context.Context, deploymentID, userID string, config TerraformConfig, streamer LogStreamer) (*ExecutionResult, error) {
	workDir := e.getDeploymentWorkingDir(deploymentID)

	// Generate Terraform configuration
	if err := e.generateTerraformConfig(workDir, config); err != nil {
		return nil, fmt.Errorf("failed to generate Terraform config: %w", err)
	}

	// Get AWS credentials
	creds, err := e.credManager.GetCredentials(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS credentials: %w", err)
	}

	return e.executeTerraformCommand(ctx, workDir, deploymentID, "", "validate", streamer, creds, "-no-color")
}

// buildSafeEnv returns only the environment variables terraform needs.
// It inherits the host PATH (so terraform can be found at its installed location)
// but NEVER includes any other host environment variables.
//
// SECURITY CONTRACT:
//   - Never pass os.Environ() — would leak host secrets into terraform subprocess
//   - Never set TF_LOG — would cause terraform to print credentials to logs
//   - Credentials are passed only as AWS_* variables, never written to disk
func buildSafeEnv(creds *aws.AWSCredentials) []string {
	// Inherit only PATH from the host environment so terraform can be found
	// at non-standard install locations (e.g. /usr/local/bin, /opt/homebrew/bin).
	hostPath := os.Getenv("PATH")
	if hostPath == "" {
		hostPath = "/usr/local/bin:/usr/bin:/bin"
	}

	env := []string{
		"AWS_ACCESS_KEY_ID=" + creds.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY=" + creds.SecretAccessKey,
		"AWS_REGION=" + creds.Region,
		"AWS_DEFAULT_REGION=" + creds.Region,
		"PATH=" + hostPath,
		// Suppress terraform upgrade checks and telemetry in subprocess.
		"CHECKPOINT_DISABLE=1",
	}

	if creds.SessionToken != "" {
		env = append(env, "AWS_SESSION_TOKEN="+creds.SessionToken)
	}

	// CRITICAL: TF_LOG must NOT be set — would print credentials to logs.
	// CRITICAL: Do NOT include os.Environ() — would leak host environment.

	return env
}


// filterSensitiveLine filters out lines that may contain AWS credentials
// Returns [REDACTED] if the line contains sensitive patterns
func filterSensitiveLine(line string) string {
	lowerLine := strings.ToLower(line)
	
	// Check for sensitive patterns
	sensitivePatterns := []string{
		"aws_access_key_id",
		"aws_secret_access_key",
		"aws_session_token",
		"secret",
		"password",
		"token",
		"credential",
	}
	
	for _, pattern := range sensitivePatterns {
		if strings.Contains(lowerLine, pattern) {
			// Check if it's just a variable declaration (safe) or actual value (sensitive)
			if strings.Contains(line, "=") && !strings.Contains(line, "variable") {
				return "[REDACTED - sensitive data filtered]"
			}
		}
	}
	
	return line
}

// executeTerraformCommand executes a Terraform command with real-time log streaming
// SECURITY: Uses buildSafeEnv to prevent credential leakage
// SECURITY: Filters sensitive output before streaming
func (e *ExecutorService) executeTerraformCommand(ctx context.Context, workDir, deploymentID, rolloutID, operation string, streamer LogStreamer, creds *aws.AWSCredentials, args ...string) (*ExecutionResult, error) {
	startTime := time.Now()
	
	// SECURITY: Use exec.Command with individual args, NEVER exec.Command("sh", "-c", ...)
	cmd := exec.CommandContext(ctx, "terraform", args...)
	cmd.Dir = workDir
	
	// SECURITY: Use buildSafeEnv instead of inheriting full environment
	cmd.Env = buildSafeEnv(creds)

	// Create pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start terraform command: %w", err)
	}

	// Stream logs in real-time
	var outputBuffer strings.Builder
	done := make(chan error, 2)

	// Stream stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			
			// SECURITY: Filter sensitive data before logging
			filteredLine := filterSensitiveLine(line)
			outputBuffer.WriteString(filteredLine + "\n")
			
			if streamer != nil {
				streamer.StreamLog(deploymentID, rolloutID, operation, "info", filteredLine)
			}
		}
		done <- scanner.Err()
	}()

	// Stream stderr
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			
			// SECURITY: Filter sensitive data before logging
			filteredLine := filterSensitiveLine(line)
			outputBuffer.WriteString(filteredLine + "\n")
			
			if streamer != nil {
				level := "error"
				if strings.Contains(strings.ToLower(line), "warning") {
					level = "warn"
				}
				streamer.StreamLog(deploymentID, rolloutID, operation, level, filteredLine)
			}
		}
		done <- scanner.Err()
	}()

	// Wait for streaming to complete
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			return nil, fmt.Errorf("error streaming logs: %w", err)
		}
	}

	// Wait for command to complete
	err = cmd.Wait()
	duration := time.Since(startTime)

	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		}
	}

	return &ExecutionResult{
		ExitCode: exitCode,
		Output:   outputBuffer.String(),
		Error:    err,
		Duration: duration,
	}, nil
}

// generateTerraformConfig writes the Terraform configuration to the working directory.
//
// The template is written verbatim as main.tf — NO string substitution is performed.
// String substitution would corrupt HCL interpolation syntax (e.g. ${var.app_name})
// and is the wrong approach for Terraform. Variable values are written to a separate
// terraform.tfvars file which Terraform reads automatically at plan/apply time.
//
// This ensures the template is valid HCL that terraform validate can check, and that
// variable values are passed safely without risk of HCL injection.
func (e *ExecutorService) generateTerraformConfig(workDir string, config TerraformConfig) error {
	if config.Template == "" {
		return fmt.Errorf("terraform template is empty")
	}

	// Add required AutoStack tagging variables.
	// These are injected by AutoStack, not provided by the user.
	if config.Variables == nil {
		config.Variables = make(map[string]interface{})
	}
	config.Variables["user_id"] = config.UserID
	config.Variables["project_id"] = config.ProjectID
	config.Variables["deployment_id"] = config.DeploymentID

	// Write the blueprint template verbatim as main.tf.
	// CRITICAL: Do NOT do string replacement here — the template contains HCL
	// interpolation syntax like ${var.app_name} which must be preserved as-is.
	mainTfPath := filepath.Join(workDir, "main.tf")
	if err := os.WriteFile(mainTfPath, []byte(config.Template), 0644); err != nil {
		return fmt.Errorf("failed to write main.tf: %w", err)
	}

	// Write variable values to terraform.tfvars.
	// Terraform reads this file automatically at plan/apply time.
	// Using tfvars (not -var flags) avoids shell quoting issues with complex values.
	tfvars, err := e.generateTfvarsFile(config.Variables)
	if err != nil {
		return fmt.Errorf("failed to generate terraform.tfvars: %w", err)
	}
	tfvarsPath := filepath.Join(workDir, "terraform.tfvars")
	if err := os.WriteFile(tfvarsPath, []byte(tfvars), 0644); err != nil {
		return fmt.Errorf("failed to write terraform.tfvars: %w", err)
	}

	return nil
}

// generateTfvarsFile produces a terraform.tfvars file from the given variable map.
// String values are quoted; numeric and boolean values are written as literals.
// Map and list values are serialised to JSON strings (Terraform accepts JSON in tfvars).
func (e *ExecutorService) generateTfvarsFile(variables map[string]interface{}) (string, error) {
	var builder strings.Builder

	for key, value := range variables {
		switch v := value.(type) {
		case string:
			// Escape any double-quotes inside the string value.
			escaped := strings.ReplaceAll(v, `"`, `\"`)
			builder.WriteString(fmt.Sprintf("%s = \"%s\"\n", key, escaped))
		case bool:
			builder.WriteString(fmt.Sprintf("%s = %t\n", key, v))
		case int, int32, int64, float32, float64:
			builder.WriteString(fmt.Sprintf("%s = %v\n", key, v))
		default:
			// For maps, lists and other complex types, use a JSON-encoded string.
			// Terraform can decode these in the template with jsondecode().
			encoded, err := json.Marshal(v)
			if err != nil {
				return "", fmt.Errorf("failed to encode variable %q: %w", key, err)
			}
			escaped := strings.ReplaceAll(string(encoded), `"`, `\"`)
			builder.WriteString(fmt.Sprintf("%s = \"%s\"\n", key, escaped))
		}
	}

	return builder.String(), nil
}


// configureS3Backend configures the S3 backend for Terraform state
// SECURITY: Each deployment gets its own S3 state path: tfstate/{userID}/{deploymentID}/terraform.tfstate
// SECURITY: DynamoDB lock key is also unique: {userID}/{deploymentID}
func (e *ExecutorService) configureS3Backend(workDir, deploymentID, userID string, creds *aws.AWSCredentials) error {
	// Use user's own state bucket (BYOC - Bring Your Own Cloud)
	// The user creates the S3 bucket and DynamoDB table once (guided setup)
	stateBucket := creds.StateBucketName
	if stateBucket == "" {
		return fmt.Errorf("state bucket name not configured for user")
	}

	stateDynamoTable := creds.StateDynamoTable
	if stateDynamoTable == "" {
		return fmt.Errorf("state DynamoDB table not configured for user")
	}

	// CRITICAL: State path MUST be unique per user and deployment
	// Format: tfstate/{userID}/{deploymentID}/terraform.tfstate
	stateKey := fmt.Sprintf("tfstate/%s/%s/terraform.tfstate", userID, deploymentID)

	// CRITICAL: Lock key MUST be unique per user and deployment
	// This prevents lock conflicts between users
	// DynamoDB will use this as the primary key
	lockKey := fmt.Sprintf("%s/%s", userID, deploymentID)

	backendConfig := fmt.Sprintf(`terraform {
  backend "s3" {
    bucket         = "%s"
    key            = "%s"
    region         = "%s"
    encrypt        = true
    dynamodb_table = "%s"
    
    # DynamoDB lock configuration
    # The lock ID will be: %s
  }
}
`, stateBucket, stateKey, creds.Region, stateDynamoTable, lockKey)

	backendPath := filepath.Join(workDir, "backend.tf")
	return os.WriteFile(backendPath, []byte(backendConfig), 0600) // Restrictive permissions
}

// GetStateVersion retrieves the current Terraform state version
func (e *ExecutorService) GetStateVersion(ctx context.Context, deploymentID string, userID string) (string, error) {
	workDir := e.getDeploymentWorkingDir(deploymentID)

	// Get AWS credentials
	creds, err := e.credManager.GetCredentials(userID)
	if err != nil {
		return "", fmt.Errorf("failed to get AWS credentials: %w", err)
	}

	cmd := exec.CommandContext(ctx, "terraform", "state", "list")
	cmd.Dir = workDir
	cmd.Env = buildSafeEnv(creds)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get state version: %w", err)
	}

	// For now, return a simple hash of the output
	// In production, you might want to use the actual state file version from S3
	return fmt.Sprintf("%x", len(output)), nil
}


// getDeploymentWorkingDir returns the working directory for a deployment
func (e *ExecutorService) getDeploymentWorkingDir(deploymentID string) string {
	return filepath.Join(e.workingDir, "deployments", deploymentID)
}

// CleanupWorkingDir removes the working directory for a deployment
func (e *ExecutorService) CleanupWorkingDir(deploymentID string) error {
	workDir := e.getDeploymentWorkingDir(deploymentID)
	return os.RemoveAll(workDir)
}

// ValidateTerraformBinary validates that the terraform binary is available and functional
// This should be called during application startup
func ValidateTerraformBinary(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "terraform", "version")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("terraform binary not found or not functional: %w", err)
	}
	
	// Basic validation that output contains "Terraform"
	if !strings.Contains(string(output), "Terraform") {
		return fmt.Errorf("terraform binary produced unexpected output")
	}
	
	return nil
}

// ExecuteWithIntelligentRecovery executes Terraform with automatic error recovery
func (e *ExecutorService) ExecuteWithIntelligentRecovery(
	ctx context.Context,
	deploymentID, rolloutID, userID string,
	config TerraformConfig,
	streamer LogStreamer,
) (*ExecutionResult, error) {
	recoveryEngine := intelligence.NewRecoveryEngine()
	maxAttempts := 3
	
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		log.Printf("[Terraform] Execution attempt %d/%d for deployment %s", attempt, maxAttempts, deploymentID)
		
		// Stream attempt info
		if streamer != nil {
			streamer.StreamLog(deploymentID, rolloutID, "apply", "info",
				fmt.Sprintf("🚀 Deployment attempt %d/%d starting...", attempt, maxAttempts))
		}
		
		// Execute the full Terraform workflow
		result, err := e.executeFullWorkflow(ctx, deploymentID, rolloutID, userID, config, streamer)
		
		// If successful, return immediately
		if err == nil && result.ExitCode == 0 {
			if streamer != nil {
				streamer.StreamLog(deploymentID, rolloutID, "apply", "success",
					"✅ Deployment completed successfully!")
			}
			return result, nil
		}
		
		// Deployment failed - analyze the error
		log.Printf("[Terraform] Deployment failed on attempt %d: %v", attempt, err)
		
		if streamer != nil {
			streamer.StreamLog(deploymentID, rolloutID, "apply", "warning",
				fmt.Sprintf("⚠️  Deployment failed on attempt %d. Analyzing error...", attempt))
		}
		
		// Analyze the error
		errorLogs := result.Output
		if err != nil {
			errorLogs += "\n" + err.Error()
		}
		
		analysis := recoveryEngine.GetAnalyzer().AnalyzeTerraformError(ctx, errorLogs)
		
		// Stream analysis results
		if streamer != nil {
			streamer.StreamLog(deploymentID, rolloutID, "apply", "info",
				fmt.Sprintf("🔍 Error Analysis:\n  Type: %s\n  Severity: %s\n  Description: %s",
					analysis.ErrorType, analysis.Severity, analysis.Description))
			
			streamer.StreamLog(deploymentID, rolloutID, "apply", "info",
				fmt.Sprintf("💡 Suggested Fix: %s", analysis.SuggestedFix))
			
			if len(analysis.FixSteps) > 0 {
				streamer.StreamLog(deploymentID, rolloutID, "apply", "info",
					"📋 Fix Steps:")
				for i, step := range analysis.FixSteps {
					streamer.StreamLog(deploymentID, rolloutID, "apply", "info",
						fmt.Sprintf("  %d. %s", i+1, step))
				}
			}
		}
		
		// Check if we should attempt recovery
		if !analysis.AutoFixable {
			if streamer != nil {
				streamer.StreamLog(deploymentID, rolloutID, "apply", "error",
					"❌ Error is not auto-fixable. Manual intervention required.")
			}
			return result, fmt.Errorf("deployment failed: %s (not auto-fixable)", analysis.Description)
		}
		
		// Don't retry on last attempt
		if attempt >= maxAttempts {
			if streamer != nil {
				streamer.StreamLog(deploymentID, rolloutID, "apply", "error",
					"❌ Max retry attempts reached. Deployment failed.")
			}
			return result, fmt.Errorf("deployment failed after %d attempts: %s", maxAttempts, analysis.Description)
		}
		
		// Attempt recovery
		if streamer != nil {
			streamer.StreamLog(deploymentID, rolloutID, "apply", "info",
				fmt.Sprintf("🔧 Attempting automatic recovery (confidence: %.0f%%)...", analysis.Confidence*100))
		}
		
		// Create a mock deployment record for recovery engine
		// In production, this would be fetched from the database
		deployment := &models.Record{}
		deployment.Set("id", deploymentID)
		deployment.Set("status", "failed")
		
		recoveryAttempt, recoveryErr := recoveryEngine.AttemptRecovery(ctx, deployment, errorLogs, attempt)
		
		if recoveryErr != nil {
			log.Printf("[Terraform] Recovery failed: %v", recoveryErr)
			if streamer != nil {
				streamer.StreamLog(deploymentID, rolloutID, "apply", "warning",
					fmt.Sprintf("⚠️  Automatic recovery failed: %s", recoveryErr.Error()))
			}
			// Continue to next attempt anyway
			continue
		}
		
		// Apply the fix to the configuration
		if recoveryAttempt.Fix != nil {
			for _, action := range recoveryAttempt.Fix.Actions {
				switch action.Type {
				case "update_variable":
					// Update the variable in config
					if action.Target != "" && action.NewValue != nil {
						config.Variables[action.Target] = action.NewValue
						if streamer != nil {
							streamer.StreamLog(deploymentID, rolloutID, "apply", "info",
								fmt.Sprintf("🔧 Updated %s to %v", action.Target, action.NewValue))
						}
					}
				case "retry":
					// Just wait and retry
					if action.Delay > 0 {
						if streamer != nil {
							streamer.StreamLog(deploymentID, rolloutID, "apply", "info",
								fmt.Sprintf("⏳ Waiting %d seconds before retry...", action.Delay))
						}
						time.Sleep(time.Duration(action.Delay) * time.Second)
					}
				}
			}
		}
		
		if streamer != nil {
			streamer.StreamLog(deploymentID, rolloutID, "apply", "info",
				"✅ Recovery actions applied. Retrying deployment...")
		}
		
		// Wait a bit before retrying
		time.Sleep(5 * time.Second)
	}
	
	return nil, fmt.Errorf("deployment failed after %d attempts with intelligent recovery", maxAttempts)
}

// executeFullWorkflow executes the complete Terraform workflow (init, plan, apply)
func (e *ExecutorService) executeFullWorkflow(
	ctx context.Context,
	deploymentID, rolloutID, userID string,
	config TerraformConfig,
	streamer LogStreamer,
) (*ExecutionResult, error) {
	// Step 1: Initialize
	if streamer != nil {
		streamer.StreamLog(deploymentID, rolloutID, "init", "info", "Initializing Terraform...")
	}
	
	initResult, err := e.Init(ctx, deploymentID, userID, streamer)
	if err != nil {
		return initResult, fmt.Errorf("terraform init failed: %w", err)
	}
	
	// Step 2: Plan
	if streamer != nil {
		streamer.StreamLog(deploymentID, rolloutID, "plan", "info", "Planning infrastructure changes...")
	}
	
	planResult, err := e.Plan(ctx, deploymentID, userID, config, streamer)
	if err != nil {
		return planResult, fmt.Errorf("terraform plan failed: %w", err)
	}
	
	// Step 3: Apply
	if streamer != nil {
		streamer.StreamLog(deploymentID, rolloutID, "apply", "info", "Applying infrastructure changes...")
	}
	
	applyResult, err := e.Apply(ctx, deploymentID, rolloutID, userID, false, streamer)
	if err != nil {
		return applyResult, fmt.Errorf("terraform apply failed: %w", err)
	}
	
	return applyResult, nil
}

// GetAnalyzer returns a new error analyzer instance
func (e *ExecutorService) GetAnalyzer() *intelligence.ErrorAnalyzer {
	return intelligence.NewErrorAnalyzer()
}

// AnalyzeLastError analyzes the last error from a deployment
func (e *ExecutorService) AnalyzeLastError(ctx context.Context, deploymentID string) (*intelligence.ErrorAnalysis, error) {
	workDir := e.getDeploymentWorkingDir(deploymentID)
	
	// Read the last error log
	logFile := filepath.Join(workDir, "terraform.log")
	logContent, err := os.ReadFile(logFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}
	
	analyzer := intelligence.NewErrorAnalyzer()
	analysis := analyzer.AnalyzeTerraformError(ctx, string(logContent))
	
	return analysis, nil
}
