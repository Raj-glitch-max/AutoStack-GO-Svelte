package intelligence

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/models"
)

// ErrorPattern represents a known error pattern with its solution
type ErrorPattern struct {
	Pattern     *regexp.Regexp
	Category    string
	Severity    string
	Description string
	Solution    string
	AutoFixable bool
}

// ErrorAnalysis contains the analysis result of a deployment failure
type ErrorAnalysis struct {
	ErrorType        string                 `json:"error_type"`
	Category         string                 `json:"category"`
	Severity         string                 `json:"severity"`
	Description      string                 `json:"description"`
	RootCause        string                 `json:"root_cause"`
	SuggestedFix     string                 `json:"suggested_fix"`
	AutoFixable      bool                   `json:"auto_fixable"`
	FixSteps         []string               `json:"fix_steps"`
	RelatedDocs      []string               `json:"related_docs"`
	EstimatedFixTime string                 `json:"estimated_fix_time"`
	Confidence       float64                `json:"confidence"`
	RawError         string                 `json:"raw_error"`
	Context          map[string]interface{} `json:"context"`
}

// ErrorAnalyzer analyzes deployment failures and suggests fixes
type ErrorAnalyzer struct {
	patterns []ErrorPattern
}

// NewErrorAnalyzer creates a new error analyzer with predefined patterns
func NewErrorAnalyzer() *ErrorAnalyzer {
	analyzer := &ErrorAnalyzer{
		patterns: make([]ErrorPattern, 0),
	}
	analyzer.initializePatterns()
	return analyzer
}

// initializePatterns loads all known error patterns
func (ea *ErrorAnalyzer) initializePatterns() {
	// Terraform AWS Errors
	ea.patterns = append(ea.patterns, []ErrorPattern{
		{
			Pattern:     regexp.MustCompile(`(?i)InvalidParameterException.*parameter.*invalid`),
			Category:    "terraform_aws",
			Severity:    "high",
			Description: "Invalid AWS parameter configuration",
			Solution:    "Review and correct the parameter values in your blueprint configuration",
			AutoFixable: false,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)AccessDenied.*not authorized`),
			Category:    "terraform_aws",
			Severity:    "critical",
			Description: "AWS credentials lack required permissions",
			Solution:    "Update IAM policy to include required permissions or use credentials with appropriate access",
			AutoFixable: false,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)ResourceAlreadyExists|AlreadyExists`),
			Category:    "terraform_aws",
			Severity:    "medium",
			Description: "Resource with this name already exists",
			Solution:    "Choose a different name or import the existing resource into Terraform state",
			AutoFixable: true,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)LimitExceeded|QuotaExceeded`),
			Category:    "terraform_aws",
			Severity:    "high",
			Description: "AWS service limit or quota exceeded",
			Solution:    "Request a service limit increase or clean up unused resources",
			AutoFixable: false,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)InvalidSubnet|subnet.*not found`),
			Category:    "terraform_aws",
			Severity:    "high",
			Description: "Specified subnet does not exist or is invalid",
			Solution:    "Verify subnet ID exists in the target region and is accessible",
			AutoFixable: false,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)InvalidSecurityGroup|security group.*not found`),
			Category:    "terraform_aws",
			Severity:    "high",
			Description: "Security group not found or invalid",
			Solution:    "Verify security group ID exists in the target VPC",
			AutoFixable: false,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)timeout while waiting for state`),
			Category:    "terraform_aws",
			Severity:    "medium",
			Description: "Resource creation timed out",
			Solution:    "Resource may still be creating. Wait and check AWS console, or increase timeout value",
			AutoFixable: true,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)Error: error creating.*S3.*BucketAlreadyExists`),
			Category:    "terraform_aws",
			Severity:    "medium",
			Description: "S3 bucket name is already taken globally",
			Solution:    "S3 bucket names must be globally unique. Choose a different bucket name",
			AutoFixable: true,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)InvalidParameterValue.*instance type.*not supported`),
			Category:    "terraform_aws",
			Severity:    "medium",
			Description: "Instance type not available in selected region/AZ",
			Solution:    "Choose a different instance type or availability zone",
			AutoFixable: true,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)DependencyViolation`),
			Category:    "terraform_aws",
			Severity:    "medium",
			Description: "Resource has dependencies that must be removed first",
			Solution:    "Remove dependent resources before destroying this resource",
			AutoFixable: false,
		},
	}...)

	// Kubernetes Errors
	ea.patterns = append(ea.patterns, []ErrorPattern{
		{
			Pattern:     regexp.MustCompile(`(?i)ImagePullBackOff|ErrImagePull`),
			Category:    "kubernetes",
			Severity:    "high",
			Description: "Cannot pull container image",
			Solution:    "Verify image name and tag are correct, and registry credentials are configured if needed",
			AutoFixable: false,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)CrashLoopBackOff`),
			Category:    "kubernetes",
			Severity:    "critical",
			Description: "Container is crashing repeatedly",
			Solution:    "Check container logs for application errors, verify environment variables and configuration",
			AutoFixable: false,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)OOMKilled`),
			Category:    "kubernetes",
			Severity:    "high",
			Description: "Container exceeded memory limit",
			Solution:    "Increase memory limits or optimize application memory usage",
			AutoFixable: true,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)Insufficient.*resources`),
			Category:    "kubernetes",
			Severity:    "high",
			Description: "Cluster lacks resources to schedule pod",
			Solution:    "Scale cluster nodes or reduce resource requests",
			AutoFixable: false,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)CreateContainerConfigError`),
			Category:    "kubernetes",
			Severity:    "high",
			Description: "Invalid container configuration",
			Solution:    "Check ConfigMap and Secret references are correct and exist",
			AutoFixable: false,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)InvalidImageName`),
			Category:    "kubernetes",
			Severity:    "medium",
			Description: "Container image name format is invalid",
			Solution:    "Use valid image format: registry/repository:tag",
			AutoFixable: true,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)FailedScheduling.*node.*didn't match`),
			Category:    "kubernetes",
			Severity:    "medium",
			Description: "Pod cannot be scheduled due to node selector constraints",
			Solution:    "Remove or adjust node selectors, or add matching labels to nodes",
			AutoFixable: true,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)Liveness probe failed`),
			Category:    "kubernetes",
			Severity:    "high",
			Description: "Health check is failing",
			Solution:    "Verify application is responding on health check endpoint or adjust probe settings",
			AutoFixable: true,
		},
	}...)

	// Terraform General Errors
	ea.patterns = append(ea.patterns, []ErrorPattern{
		{
			Pattern:     regexp.MustCompile(`(?i)Error: Invalid.*syntax`),
			Category:    "terraform_syntax",
			Severity:    "high",
			Description: "Terraform configuration syntax error",
			Solution:    "Review Terraform configuration for syntax errors",
			AutoFixable: false,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)Error: Reference to undeclared.*variable`),
			Category:    "terraform_syntax",
			Severity:    "high",
			Description: "Variable referenced but not declared",
			Solution:    "Declare the variable in variables.tf or remove the reference",
			AutoFixable: true,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)Error acquiring the state lock`),
			Category:    "terraform_state",
			Severity:    "medium",
			Description: "Another Terraform operation is in progress",
			Solution:    "Wait for other operation to complete or force-unlock if stuck",
			AutoFixable: true,
		},
		{
			Pattern:     regexp.MustCompile(`(?i)Backend initialization required`),
			Category:    "terraform_state",
			Severity:    "medium",
			Description: "Terraform backend not initialized",
			Solution:    "Run terraform init to initialize backend",
			AutoFixable: true,
		},
	}...)
}

// AnalyzeError analyzes an error message and returns detailed analysis
func (ea *ErrorAnalyzer) AnalyzeError(ctx context.Context, errorMsg string, deploymentType string) *ErrorAnalysis {
	analysis := &ErrorAnalysis{
		RawError:    errorMsg,
		Confidence:  0.0,
		Context:     make(map[string]interface{}),
		RelatedDocs: make([]string, 0),
		FixSteps:    make([]string, 0),
	}

	// Try to match against known patterns
	for _, pattern := range ea.patterns {
		if pattern.Pattern.MatchString(errorMsg) {
			analysis.ErrorType = pattern.Category
			analysis.Category = pattern.Category
			analysis.Severity = pattern.Severity
			analysis.Description = pattern.Description
			analysis.SuggestedFix = pattern.Solution
			analysis.AutoFixable = pattern.AutoFixable
			analysis.Confidence = 0.9 // High confidence for pattern match

			// Extract specific details from error
			analysis.RootCause = ea.extractRootCause(errorMsg, pattern)

			// Generate fix steps
			analysis.FixSteps = ea.generateFixSteps(pattern, errorMsg)

			// Add related documentation
			analysis.RelatedDocs = ea.getRelatedDocs(pattern.Category)

			// Estimate fix time
			analysis.EstimatedFixTime = ea.estimateFixTime(pattern)

			return analysis
		}
	}

	// No pattern matched - provide generic analysis
	analysis.ErrorType = "unknown"
	analysis.Category = deploymentType
	analysis.Severity = "medium"
	analysis.Description = "Unrecognized error pattern"
	analysis.RootCause = ea.extractGenericRootCause(errorMsg)
	analysis.SuggestedFix = "Review error message and deployment configuration"
	analysis.AutoFixable = false
	analysis.Confidence = 0.3
	analysis.FixSteps = []string{
		"Review the complete error message",
		"Check deployment configuration for issues",
		"Consult relevant documentation",
		"Contact support if issue persists",
	}

	return analysis
}

// extractRootCause extracts the root cause from error message
func (ea *ErrorAnalyzer) extractRootCause(errorMsg string, pattern ErrorPattern) string {
	// Extract specific error details based on pattern
	lines := strings.Split(errorMsg, "\n")
	for _, line := range lines {
		if pattern.Pattern.MatchString(line) {
			return strings.TrimSpace(line)
		}
	}
	return pattern.Description
}

// extractGenericRootCause attempts to extract root cause from unknown errors
func (ea *ErrorAnalyzer) extractGenericRootCause(errorMsg string) string {
	// Look for common error indicators
	errorIndicators := []string{"Error:", "error:", "ERROR:", "Failed:", "failed:", "Exception:"}
	
	lines := strings.Split(errorMsg, "\n")
	for _, line := range lines {
		for _, indicator := range errorIndicators {
			if strings.Contains(line, indicator) {
				return strings.TrimSpace(line)
			}
		}
	}

	// Return first non-empty line if no indicator found
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}

	return "Unable to determine root cause"
}

// generateFixSteps generates step-by-step fix instructions
func (ea *ErrorAnalyzer) generateFixSteps(pattern ErrorPattern, errorMsg string) []string {
	steps := make([]string, 0)

	switch pattern.Category {
	case "terraform_aws":
		if strings.Contains(errorMsg, "AccessDenied") {
			steps = append(steps, []string{
				"Navigate to AWS IAM Console",
				"Locate the IAM user or role used by AutoStack",
				"Review and update the IAM policy to include required permissions",
				"Test credentials by retrying the deployment",
			}...)
		} else if strings.Contains(errorMsg, "AlreadyExists") {
			steps = append(steps, []string{
				"Choose a different unique name for the resource",
				"Or import the existing resource: terraform import <resource_type>.<name> <id>",
				"Retry the deployment",
			}...)
		} else if strings.Contains(errorMsg, "BucketAlreadyExists") {
			steps = append(steps, []string{
				"S3 bucket names must be globally unique across all AWS accounts",
				"Choose a different bucket name (e.g., add random suffix)",
				"Update your deployment configuration",
				"Retry the deployment",
			}...)
		} else {
			steps = append(steps, []string{
				"Review the error message details",
				"Check AWS console for resource status",
				"Verify configuration parameters",
				"Retry deployment after corrections",
			}...)
		}

	case "kubernetes":
		if strings.Contains(errorMsg, "ImagePullBackOff") {
			steps = append(steps, []string{
				"Verify the image name and tag are correct",
				"Check if the image exists in the registry",
				"If using private registry, ensure image pull secrets are configured",
				"Update deployment with correct image reference",
			}...)
		} else if strings.Contains(errorMsg, "OOMKilled") {
			steps = append(steps, []string{
				"Review application memory usage patterns",
				"Increase memory limits in deployment configuration",
				"Consider optimizing application memory usage",
				"Redeploy with updated resource limits",
			}...)
		} else if strings.Contains(errorMsg, "CrashLoopBackOff") {
			steps = append(steps, []string{
				"Check pod logs: kubectl logs <pod-name>",
				"Verify environment variables are set correctly",
				"Check application configuration",
				"Fix application issues and redeploy",
			}...)
		} else {
			steps = append(steps, []string{
				"Check pod status and events",
				"Review pod logs for application errors",
				"Verify deployment configuration",
				"Update and redeploy",
			}...)
		}

	case "terraform_state":
		if strings.Contains(errorMsg, "state lock") {
			steps = append(steps, []string{
				"Wait 5-10 minutes for other operation to complete",
				"If stuck, check DynamoDB lock table",
				"Force unlock if necessary: terraform force-unlock <lock-id>",
				"Retry deployment",
			}...)
		}

	default:
		steps = append(steps, []string{
			"Review complete error message",
			"Check deployment configuration",
			"Consult documentation",
			"Retry after corrections",
		}...)
	}

	return steps
}

// getRelatedDocs returns relevant documentation links
func (ea *ErrorAnalyzer) getRelatedDocs(category string) []string {
	docs := make([]string, 0)

	switch category {
	case "terraform_aws":
		docs = append(docs, []string{
			"https://registry.terraform.io/providers/hashicorp/aws/latest/docs",
			"https://docs.aws.amazon.com/",
		}...)
	case "kubernetes":
		docs = append(docs, []string{
			"https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/",
			"https://kubernetes.io/docs/tasks/debug/debug-application/",
		}...)
	case "terraform_state":
		docs = append(docs, []string{
			"https://www.terraform.io/docs/language/state/",
			"https://www.terraform.io/docs/language/settings/backends/",
		}...)
	}

	return docs
}

// estimateFixTime estimates how long the fix will take
func (ea *ErrorAnalyzer) estimateFixTime(pattern ErrorPattern) string {
	if pattern.AutoFixable {
		return "< 5 minutes (auto-fixable)"
	}

	switch pattern.Severity {
	case "critical":
		return "30-60 minutes"
	case "high":
		return "15-30 minutes"
	case "medium":
		return "5-15 minutes"
	default:
		return "< 5 minutes"
	}
}

// AnalyzeTerraformError analyzes Terraform-specific errors
func (ea *ErrorAnalyzer) AnalyzeTerraformError(ctx context.Context, logs string) *ErrorAnalysis {
	return ea.AnalyzeError(ctx, logs, "terraform")
}

// AnalyzeKubernetesError analyzes Kubernetes-specific errors
func (ea *ErrorAnalyzer) AnalyzeKubernetesError(ctx context.Context, logs string) *ErrorAnalysis {
	return ea.AnalyzeError(ctx, logs, "kubernetes")
}

// SuggestAutoFix attempts to generate an automatic fix for the error
func (ea *ErrorAnalyzer) SuggestAutoFix(ctx context.Context, analysis *ErrorAnalysis, deployment *models.Record) (*AutoFix, error) {
	if !analysis.AutoFixable {
		return nil, fmt.Errorf("error is not auto-fixable")
	}

	fix := &AutoFix{
		AnalysisID:  fmt.Sprintf("analysis_%d", time.Now().Unix()),
		DeploymentID: deployment.Id,
		ErrorType:   analysis.ErrorType,
		FixType:     ea.determineFixType(analysis),
		Description: analysis.SuggestedFix,
		Confidence:  analysis.Confidence,
		CreatedAt:   time.Now(),
	}

	// Generate specific fix actions based on error type
	switch analysis.Category {
	case "terraform_aws":
		if strings.Contains(analysis.RawError, "BucketAlreadyExists") {
			fix.Actions = []FixAction{
				{
					Type:        "update_variable",
					Target:      "bucket_name",
					NewValue:    fmt.Sprintf("%s-%d", deployment.GetString("name"), time.Now().Unix()),
					Description: "Generate unique S3 bucket name",
				},
			}
		} else if strings.Contains(analysis.RawError, "timeout") {
			fix.Actions = []FixAction{
				{
					Type:        "retry",
					Description: "Retry deployment after timeout",
					Delay:       60,
				},
			}
		}

	case "kubernetes":
		if strings.Contains(analysis.RawError, "OOMKilled") {
			fix.Actions = []FixAction{
				{
					Type:        "update_resource",
					Target:      "memory_limit",
					NewValue:    "512Mi", // Double the default
					Description: "Increase memory limit",
				},
			}
		}

	case "terraform_state":
		if strings.Contains(analysis.RawError, "state lock") {
			fix.Actions = []FixAction{
				{
					Type:        "force_unlock",
					Description: "Force unlock Terraform state",
				},
			}
		}
	}

	return fix, nil
}

// determineFixType determines the type of fix needed
func (ea *ErrorAnalyzer) determineFixType(analysis *ErrorAnalysis) string {
	if strings.Contains(analysis.RawError, "timeout") {
		return "retry"
	}
	if strings.Contains(analysis.RawError, "AlreadyExists") {
		return "rename"
	}
	if strings.Contains(analysis.RawError, "OOMKilled") {
		return "scale_up"
	}
	if strings.Contains(analysis.RawError, "state lock") {
		return "unlock"
	}
	return "manual"
}

// AutoFix represents an automatic fix suggestion
type AutoFix struct {
	AnalysisID   string      `json:"analysis_id"`
	DeploymentID string      `json:"deployment_id"`
	ErrorType    string      `json:"error_type"`
	FixType      string      `json:"fix_type"`
	Description  string      `json:"description"`
	Actions      []FixAction `json:"actions"`
	Confidence   float64     `json:"confidence"`
	CreatedAt    time.Time   `json:"created_at"`
}

// FixAction represents a specific action to fix an error
type FixAction struct {
	Type        string      `json:"type"`
	Target      string      `json:"target,omitempty"`
	NewValue    interface{} `json:"new_value,omitempty"`
	Description string      `json:"description"`
	Delay       int         `json:"delay,omitempty"` // seconds
}
