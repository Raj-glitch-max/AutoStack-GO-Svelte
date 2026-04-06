package terraform

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/PranavMagar/autostack/pkg/aws"
)

type ExecutorService struct {
	workingDir  string
	credManager *aws.CredentialManager
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
	}
}

// Init initializes a Terraform working directory
func (e *ExecutorService) Init(ctx context.Context, deploymentID string, streamer LogStreamer) (*ExecutionResult, error) {
	workDir := e.getDeploymentWorkingDir(deploymentID)
	
	// Ensure working directory exists
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create working directory: %w", err)
	}

	// Configure S3 backend
	if err := e.configureS3Backend(workDir, deploymentID); err != nil {
		return nil, fmt.Errorf("failed to configure S3 backend: %w", err)
	}

	return e.executeTerraformCommand(ctx, workDir, deploymentID, "", "init", streamer, "-no-color")
}

// Plan generates a Terraform execution plan
func (e *ExecutorService) Plan(ctx context.Context, deploymentID string, config TerraformConfig, streamer LogStreamer) (*ExecutionResult, error) {
	workDir := e.getDeploymentWorkingDir(deploymentID)

	// Generate Terraform configuration
	if err := e.generateTerraformConfig(workDir, config); err != nil {
		return nil, fmt.Errorf("failed to generate Terraform config: %w", err)
	}

	// Set AWS credentials
	if err := e.setAWSCredentials(config.UserID); err != nil {
		return nil, fmt.Errorf("failed to set AWS credentials: %w", err)
	}

	return e.executeTerraformCommand(ctx, workDir, deploymentID, "", "plan", streamer, "-no-color", "-detailed-exitcode")
}

// Apply applies the Terraform configuration
func (e *ExecutorService) Apply(ctx context.Context, deploymentID, rolloutID string, autoApprove bool, streamer LogStreamer) (*ExecutionResult, error) {
	workDir := e.getDeploymentWorkingDir(deploymentID)

	args := []string{"apply", "-no-color"}
	if autoApprove {
		args = append(args, "-auto-approve")
	}

	return e.executeTerraformCommand(ctx, workDir, deploymentID, rolloutID, "apply", streamer, args...)
}

// Destroy destroys the Terraform-managed infrastructure
func (e *ExecutorService) Destroy(ctx context.Context, deploymentID, rolloutID string, autoApprove bool, streamer LogStreamer) (*ExecutionResult, error) {
	workDir := e.getDeploymentWorkingDir(deploymentID)

	args := []string{"destroy", "-no-color"}
	if autoApprove {
		args = append(args, "-auto-approve")
	}

	return e.executeTerraformCommand(ctx, workDir, deploymentID, rolloutID, "destroy", streamer, args...)
}

// GetOutputs retrieves Terraform outputs
func (e *ExecutorService) GetOutputs(ctx context.Context, deploymentID string, userID string) (map[string]TerraformOutput, error) {
	workDir := e.getDeploymentWorkingDir(deploymentID)

	// Set AWS credentials
	if err := e.setAWSCredentials(userID); err != nil {
		return nil, fmt.Errorf("failed to set AWS credentials: %w", err)
	}

	cmd := exec.CommandContext(ctx, "terraform", "output", "-json")
	cmd.Dir = workDir

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
func (e *ExecutorService) Validate(ctx context.Context, deploymentID string, config TerraformConfig, streamer LogStreamer) (*ExecutionResult, error) {
	workDir := e.getDeploymentWorkingDir(deploymentID)

	// Generate Terraform configuration
	if err := e.generateTerraformConfig(workDir, config); err != nil {
		return nil, fmt.Errorf("failed to generate Terraform config: %w", err)
	}

	return e.executeTerraformCommand(ctx, workDir, deploymentID, "", "validate", streamer, "-no-color")
}

// executeTerraformCommand executes a Terraform command with real-time log streaming
func (e *ExecutorService) executeTerraformCommand(ctx context.Context, workDir, deploymentID, rolloutID, operation string, streamer LogStreamer, args ...string) (*ExecutionResult, error) {
	startTime := time.Now()
	
	cmd := exec.CommandContext(ctx, "terraform", args...)
	cmd.Dir = workDir

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
			outputBuffer.WriteString(line + "\n")
			
			if streamer != nil {
				streamer.StreamLog(deploymentID, rolloutID, operation, "info", line)
			}
		}
		done <- scanner.Err()
	}()

	// Stream stderr
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			outputBuffer.WriteString(line + "\n")
			
			if streamer != nil {
				level := "error"
				if strings.Contains(strings.ToLower(line), "warning") {
					level = "warn"
				}
				streamer.StreamLog(deploymentID, rolloutID, operation, level, line)
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

// generateTerraformConfig generates the Terraform configuration file
func (e *ExecutorService) generateTerraformConfig(workDir string, config TerraformConfig) error {
	// Replace template variables
	terraformConfig := config.Template
	
	// Add required variables for resource tagging
	config.Variables["user_id"] = config.UserID
	config.Variables["project_id"] = config.ProjectID
	config.Variables["deployment_id"] = config.DeploymentID

	// Replace variables in template
	for key, value := range config.Variables {
		placeholder := fmt.Sprintf("${%s}", key)
		terraformConfig = strings.ReplaceAll(terraformConfig, placeholder, fmt.Sprintf("%v", value))
	}

	// Write main.tf file
	mainTfPath := filepath.Join(workDir, "main.tf")
	if err := os.WriteFile(mainTfPath, []byte(terraformConfig), 0644); err != nil {
		return fmt.Errorf("failed to write main.tf: %w", err)
	}

	// Generate variables.tf file
	variablesTf := e.generateVariablesFile(config.Variables)
	variablesTfPath := filepath.Join(workDir, "variables.tf")
	if err := os.WriteFile(variablesTfPath, []byte(variablesTf), 0644); err != nil {
		return fmt.Errorf("failed to write variables.tf: %w", err)
	}

	return nil
}

// generateVariablesFile generates the variables.tf file
func (e *ExecutorService) generateVariablesFile(variables map[string]interface{}) string {
	var builder strings.Builder
	
	for key, value := range variables {
		builder.WriteString(fmt.Sprintf(`variable "%s" {
  description = "Variable for %s"
  type        = string
  default     = "%v"
}

`, key, key, value))
	}

	return builder.String()
}

// configureS3Backend configures the S3 backend for Terraform state
func (e *ExecutorService) configureS3Backend(workDir, deploymentID string) error {
	backendConfig := fmt.Sprintf(`terraform {
  backend "s3" {
    bucket         = "autostack-terraform-state"
    key            = "deployments/%s/terraform.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "autostack-terraform-locks"
  }
}
`, deploymentID)

	backendPath := filepath.Join(workDir, "backend.tf")
	return os.WriteFile(backendPath, []byte(backendConfig), 0644)
}

// setAWSCredentials sets AWS credentials as environment variables
func (e *ExecutorService) setAWSCredentials(userID string) error {
	creds, err := e.credManager.GetCredentials(userID)
	if err != nil {
		return fmt.Errorf("failed to get AWS credentials: %w", err)
	}

	os.Setenv("AWS_ACCESS_KEY_ID", creds.AccessKeyID)
	os.Setenv("AWS_SECRET_ACCESS_KEY", creds.SecretAccessKey)
	os.Setenv("AWS_DEFAULT_REGION", creds.Region)
	
	if creds.SessionToken != "" {
		os.Setenv("AWS_SESSION_TOKEN", creds.SessionToken)
	}

	return nil
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

// GetStateVersion retrieves the current Terraform state version
func (e *ExecutorService) GetStateVersion(ctx context.Context, deploymentID string, userID string) (string, error) {
	workDir := e.getDeploymentWorkingDir(deploymentID)

	// Set AWS credentials
	if err := e.setAWSCredentials(userID); err != nil {
		return "", fmt.Errorf("failed to set AWS credentials: %w", err)
	}

	cmd := exec.CommandContext(ctx, "terraform", "state", "list")
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get state version: %w", err)
	}

	// For now, return a simple hash of the output
	// In production, you might want to use the actual state file version from S3
	return fmt.Sprintf("%x", len(output)), nil
}