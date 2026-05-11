package universal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/intelligence"
)

const maxFixAttempts = 3

const fixerSystemPrompt = `You are an expert AWS Terraform engineer. Fix the provided Terraform HCL so it passes terraform validate.
Return ONLY the corrected complete Terraform HCL. No explanation. No markdown fences.`

// ValidateAndFix runs terraform validate on the HCL and uses AI to fix errors.
// Returns the valid HCL or an error after maxFixAttempts.
func ValidateAndFix(ctx context.Context, terraformHCL string, workDir string, spec InfrastructureSpec) (string, error) {
	for attempt := 1; attempt <= maxFixAttempts; attempt++ {
		// Write HCL to work dir
		mainTF := filepath.Join(workDir, "main.tf")
		if err := os.WriteFile(mainTF, []byte(terraformHCL), 0644); err != nil {
			return "", fmt.Errorf("failed to write main.tf: %w", err)
		}

		// Run terraform init (no backend — just initializes providers for validation)
		initOut, initErr := runTerraformCmd(workDir, "init", "-backend=false", "-no-color", "-input=false")
		if initErr != nil {
			log.Printf("[TF Validator] Init attempt %d failed: %s", attempt, initOut)
			if attempt == maxFixAttempts {
				return "", fmt.Errorf("terraform init failed after %d attempts: %s", maxFixAttempts, initOut)
			}
			var fixErr error
			terraformHCL, fixErr = fixTerraformWithAI(ctx, terraformHCL, "terraform init failed: "+initOut, spec)
			if fixErr != nil {
				return "", fixErr
			}
			continue
		}

		// Run terraform validate
		validateOut, validateErr := runTerraformCmd(workDir, "validate", "-no-color")
		if validateErr == nil {
			log.Printf("[TF Validator] Valid after %d attempt(s)", attempt)
			return terraformHCL, nil
		}

		log.Printf("[TF Validator] Validate attempt %d failed: %s", attempt, validateOut)

		if attempt == maxFixAttempts {
			return "", fmt.Errorf("terraform validation failed after %d fix attempts. Last error: %s", maxFixAttempts, validateOut)
		}

		var fixErr error
		terraformHCL, fixErr = fixTerraformWithAI(ctx, terraformHCL, validateOut, spec)
		if fixErr != nil {
			return "", fixErr
		}
	}

	return "", fmt.Errorf("unreachable")
}

// fixTerraformWithAI asks the LLM to fix a validation error
func fixTerraformWithAI(ctx context.Context, hcl string, validationError string, spec InfrastructureSpec) (string, error) {
	client := intelligence.NewClaudeClient()

	specJSON, _ := json.MarshalIndent(spec, "", "  ")

	userPrompt := fmt.Sprintf(`The following Terraform HCL failed validation.

Error from terraform validate:
%s

Original InfrastructureSpec it should implement:
%s

Broken Terraform HCL:
%s

Fix ALL errors. Return ONLY the corrected complete Terraform HCL. No explanation. No markdown fences.`,
		validationError,
		string(specJSON),
		hcl,
	)

	fixed, err := client.Complete(ctx, fixerSystemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("AI fixer failed: %w", err)
	}

	return stripMarkdownFences(fixed), nil
}

// runTerraformCmd runs a terraform command and returns combined output
func runTerraformCmd(workDir string, args ...string) (string, error) {
	cmd := exec.Command("terraform", args...)
	cmd.Dir = workDir
	// Minimal safe environment — no AWS credentials needed for validate
	cmd.Env = []string{
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + os.Getenv("PATH"),
		"TF_IN_AUTOMATION=1",
		"TF_CLI_ARGS=-no-color",
	}
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return output, fmt.Errorf("terraform %s failed: %w", args[0], err)
	}
	return output, nil
}
