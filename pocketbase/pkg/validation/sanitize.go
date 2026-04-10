package validation

import (
	"fmt"
	"regexp"
	"strings"
)

// AllowedTerraformVarPattern — only alphanumeric, dash, underscore, dot, slash, colon
// This prevents shell injection and other malicious inputs
var AllowedTerraformVarPattern = regexp.MustCompile(`^[a-zA-Z0-9:/_.\-]+$`)

// AllowedDockerImagePattern validates Docker image names
// Format: [registry/][namespace/]repository[:tag|@digest]
var AllowedDockerImagePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*[a-zA-Z0-9](:[a-zA-Z0-9._-]+)?(@sha256:[a-fA-F0-9]{64})?$`)

// AllowedAWSRegionPattern validates AWS region codes
var AllowedAWSRegionPattern = regexp.MustCompile(`^[a-z]{2}-[a-z]+-\d{1}$`)

// AllowedDNSSafeNamePattern validates DNS-safe names (for deployments, projects, etc.)
var AllowedDNSSafeNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,61}[a-z0-9]$`)

// AllowedEnvVarKeyPattern validates environment variable keys
var AllowedEnvVarKeyPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// SanitizeTerraformVar validates and sanitizes a Terraform variable value.
// Returns error if the input contains invalid characters that could lead to injection attacks.
// Values are NEVER interpolated directly into .tf files as raw strings.
// They are ALWAYS passed via -var key=value arguments to the terraform process.
func SanitizeTerraformVar(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("terraform variable cannot be empty")
	}

	if !AllowedTerraformVarPattern.MatchString(input) {
		return "", fmt.Errorf("invalid character in terraform variable: only alphanumeric, dash, underscore, dot, slash, and colon are allowed")
	}

	// Additional check: reject if it looks like shell injection attempts
	dangerousPatterns := []string{
		";", "&&", "||", "|", "`", "$(",
		"$(", "${", ">>", "<<", ">", "<",
		"\n", "\r", "\t",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(input, pattern) {
			return "", fmt.Errorf("terraform variable contains potentially dangerous pattern: %s", pattern)
		}
	}

	return input, nil
}

// SanitizeDockerImage validates a Docker image name.
// Rejects images that don't match standard Docker naming conventions.
func SanitizeDockerImage(image string) (string, error) {
	if image == "" {
		return "", fmt.Errorf("docker image cannot be empty")
	}

	// Basic length check
	if len(image) > 255 {
		return "", fmt.Errorf("docker image name too long (max 255 characters)")
	}

	// Check for shell injection attempts
	if strings.ContainsAny(image, ";|&`$(){}[]<>\n\r\t") {
		return "", fmt.Errorf("docker image contains invalid characters")
	}

	// Validate against pattern
	if !AllowedDockerImagePattern.MatchString(image) {
		return "", fmt.Errorf("invalid docker image format")
	}

	return image, nil
}

// SanitizeAWSRegion validates an AWS region code.
func SanitizeAWSRegion(region string) (string, error) {
	if region == "" {
		return "", fmt.Errorf("aws region cannot be empty")
	}

	if !AllowedAWSRegionPattern.MatchString(region) {
		return "", fmt.Errorf("invalid aws region format (expected format: us-east-1)")
	}

	// Whitelist of known AWS regions (as of 2024)
	validRegions := map[string]bool{
		"us-east-1":      true,
		"us-east-2":      true,
		"us-west-1":      true,
		"us-west-2":      true,
		"af-south-1":     true,
		"ap-east-1":      true,
		"ap-south-1":     true,
		"ap-south-2":     true,
		"ap-northeast-1": true,
		"ap-northeast-2": true,
		"ap-northeast-3": true,
		"ap-southeast-1": true,
		"ap-southeast-2": true,
		"ap-southeast-3": true,
		"ap-southeast-4": true,
		"ca-central-1":   true,
		"eu-central-1":   true,
		"eu-central-2":   true,
		"eu-west-1":      true,
		"eu-west-2":      true,
		"eu-west-3":      true,
		"eu-south-1":     true,
		"eu-south-2":     true,
		"eu-north-1":     true,
		"me-south-1":     true,
		"me-central-1":   true,
		"sa-east-1":      true,
	}

	if !validRegions[region] {
		return "", fmt.Errorf("unknown aws region: %s", region)
	}

	return region, nil
}

// SanitizeDNSSafeName validates a DNS-safe name for deployments, projects, etc.
// Must start with a letter, end with alphanumeric, and contain only lowercase letters, numbers, and hyphens.
// Length must be between 2 and 63 characters.
func SanitizeDNSSafeName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name cannot be empty")
	}

	if len(name) < 2 || len(name) > 63 {
		return "", fmt.Errorf("name must be between 2 and 63 characters")
	}

	if !AllowedDNSSafeNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid name format: must start with a letter, end with alphanumeric, and contain only lowercase letters, numbers, and hyphens")
	}

	return name, nil
}

// SanitizeEnvVarKey validates an environment variable key.
// Must start with a letter or underscore, and contain only uppercase letters, numbers, and underscores.
func SanitizeEnvVarKey(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("environment variable key cannot be empty")
	}

	if len(key) > 128 {
		return "", fmt.Errorf("environment variable key too long (max 128 characters)")
	}

	if !AllowedEnvVarKeyPattern.MatchString(key) {
		return "", fmt.Errorf("invalid environment variable key: must start with letter or underscore, and contain only uppercase letters, numbers, and underscores")
	}

	return key, nil
}

// SanitizeEnvVarValue validates an environment variable value.
// Rejects values that contain null bytes or other control characters.
func SanitizeEnvVarValue(value string) (string, error) {
	// Check for null bytes and control characters
	for i, r := range value {
		if r == 0 {
			return "", fmt.Errorf("environment variable value contains null byte at position %d", i)
		}
		// Allow printable characters, tabs, and newlines only
		if r < 32 && r != 9 && r != 10 && r != 13 {
			return "", fmt.Errorf("environment variable value contains invalid control character at position %d", i)
		}
	}

	return value, nil
}

// SanitizeConfiguration validates an entire configuration map for Terraform variables.
// Returns error if any key or value is invalid.
func SanitizeConfiguration(config map[string]string) error {
	if config == nil {
		return fmt.Errorf("configuration cannot be nil")
	}

	for key, value := range config {
		// Validate key
		if _, err := SanitizeTerraformVar(key); err != nil {
			return fmt.Errorf("invalid configuration key %q: %w", key, err)
		}

		// Validate value
		if _, err := SanitizeTerraformVar(value); err != nil {
			return fmt.Errorf("invalid configuration value for key %q: %w", key, err)
		}
	}

	return nil
}

// SanitizeS3BucketName validates an S3 bucket name.
func SanitizeS3BucketName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("s3 bucket name cannot be empty")
	}

	if len(name) < 3 || len(name) > 63 {
		return "", fmt.Errorf("s3 bucket name must be between 3 and 63 characters")
	}

	// S3 bucket naming rules
	s3Pattern := regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*[a-z0-9]$`)
	if !s3Pattern.MatchString(name) {
		return "", fmt.Errorf("invalid s3 bucket name format")
	}

	// Additional S3 rules
	if strings.Contains(name, "..") {
		return "", fmt.Errorf("s3 bucket name cannot contain consecutive periods")
	}

	if strings.HasPrefix(name, "xn--") {
		return "", fmt.Errorf("s3 bucket name cannot start with 'xn--'")
	}

	if strings.HasSuffix(name, "-s3alias") {
		return "", fmt.Errorf("s3 bucket name cannot end with '-s3alias'")
	}

	return name, nil
}

// SanitizeDynamoDBTableName validates a DynamoDB table name.
func SanitizeDynamoDBTableName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("dynamodb table name cannot be empty")
	}

	if len(name) < 3 || len(name) > 255 {
		return "", fmt.Errorf("dynamodb table name must be between 3 and 255 characters")
	}

	// DynamoDB allows alphanumeric, underscore, hyphen, and period
	dynamoPattern := regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	if !dynamoPattern.MatchString(name) {
		return "", fmt.Errorf("invalid dynamodb table name: only alphanumeric, underscore, hyphen, and period allowed")
	}

	return name, nil
}
