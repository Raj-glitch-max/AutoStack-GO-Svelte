package terraform

import (
	"strings"
	"testing"
)

// TestGenerateTfvarsFileStringValues verifies string values are quoted correctly.
func TestGenerateTfvarsFileStringValues(t *testing.T) {
	executor := &ExecutorService{}
	vars := map[string]interface{}{
		"app_name": "my-web-app",
		"region":   "us-east-1",
	}

	result, err := executor.generateTfvarsFile(vars)
	if err != nil {
		t.Fatalf("generateTfvarsFile returned unexpected error: %v", err)
	}

	if !strings.Contains(result, `app_name = "my-web-app"`) {
		t.Errorf("Expected quoted string value, got:\n%s", result)
	}
	if !strings.Contains(result, `region = "us-east-1"`) {
		t.Errorf("Expected quoted string value, got:\n%s", result)
	}
}

// TestGenerateTfvarsFileBoolValues verifies booleans are written as literals (not quoted).
func TestGenerateTfvarsFileBoolValues(t *testing.T) {
	executor := &ExecutorService{}
	vars := map[string]interface{}{
		"enable_https": true,
		"multi_az":     false,
	}

	result, err := executor.generateTfvarsFile(vars)
	if err != nil {
		t.Fatalf("generateTfvarsFile returned unexpected error: %v", err)
	}

	if !strings.Contains(result, "enable_https = true") {
		t.Errorf("Expected unquoted bool true, got:\n%s", result)
	}
	if !strings.Contains(result, "multi_az = false") {
		t.Errorf("Expected unquoted bool false, got:\n%s", result)
	}
}

// TestGenerateTfvarsFileNumericValues verifies numbers are written as literals.
func TestGenerateTfvarsFileNumericValues(t *testing.T) {
	executor := &ExecutorService{}
	vars := map[string]interface{}{
		"instance_count": 3,
		"cpu_units":      256,
	}

	result, err := executor.generateTfvarsFile(vars)
	if err != nil {
		t.Fatalf("generateTfvarsFile returned unexpected error: %v", err)
	}

	if !strings.Contains(result, "instance_count = 3") {
		t.Errorf("Expected numeric literal, got:\n%s", result)
	}
	if !strings.Contains(result, "cpu_units = 256") {
		t.Errorf("Expected numeric literal, got:\n%s", result)
	}
}

// TestGenerateTfvarsFileEscapesQuotes verifies double-quotes in string values are escaped.
func TestGenerateTfvarsFileEscapesQuotes(t *testing.T) {
	executor := &ExecutorService{}
	vars := map[string]interface{}{
		"description": `say "hello"`,
	}

	result, err := executor.generateTfvarsFile(vars)
	if err != nil {
		t.Fatalf("generateTfvarsFile returned unexpected error: %v", err)
	}

	// Result must have escaped internal quotes to produce valid HCL.
	if !strings.Contains(result, `\"hello\"`) {
		t.Errorf("Expected escaped inner quotes in tfvars output, got:\n%s", result)
	}
}

// TestGenerateTfvarsFileEmpty verifies an empty variable map produces an empty file.
func TestGenerateTfvarsFileEmpty(t *testing.T) {
	executor := &ExecutorService{}
	vars := map[string]interface{}{}

	result, err := executor.generateTfvarsFile(vars)
	if err != nil {
		t.Fatalf("generateTfvarsFile returned unexpected error: %v", err)
	}

	if result != "" {
		t.Errorf("Expected empty output for empty vars, got: %q", result)
	}
}

// TestGenerateTfvarsFileNoHCLInterpolationCorruption verifies that the tfvars approach
// does not corrupt HCL interpolation syntax. The template is written verbatim; only
// terraform.tfvars carries values. This test documents the contract.
func TestGenerateTfvarsFileNoHCLInterpolationCorruption(t *testing.T) {
	// If the old string-replace approach was used, ${var.app_name} in the template
	// would be replaced with the literal value, corrupting any other HCL expressions.
	// The correct approach is: write template verbatim, values in tfvars.
	// This test verifies that generateTfvarsFile itself never touches the template.
	executor := &ExecutorService{}
	vars := map[string]interface{}{
		"app_name": "my-app",
	}

	result, err := executor.generateTfvarsFile(vars)
	if err != nil {
		t.Fatalf("generateTfvarsFile returned unexpected error: %v", err)
	}

	// tfvars must not contain any Terraform resource syntax.
	if strings.Contains(result, "resource ") || strings.Contains(result, "provider ") {
		t.Errorf("tfvars output must not contain HCL resource/provider blocks: %s", result)
	}

	// Values must be quoted, not raw substitutions.
	if strings.Contains(result, "${") {
		t.Errorf("tfvars output must not contain HCL interpolation syntax: %s", result)
	}
}
