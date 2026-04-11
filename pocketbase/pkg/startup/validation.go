package startup

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Raj-glitch-max/autostack/pkg/crypto"
	"github.com/Raj-glitch-max/autostack/pkg/terraform"
)

// ValidationResult contains the results of startup validation
type ValidationResult struct {
	EncryptionKeyValid bool
	TerraformAvailable bool
	Errors             []error
}

// ValidateStartupRequirements validates all critical requirements before starting the application
// This function MUST be called during application startup
// If it returns errors, the application MUST exit immediately
func ValidateStartupRequirements(ctx context.Context) (*ValidationResult, error) {
	result := &ValidationResult{
		EncryptionKeyValid: false,
		TerraformAvailable: false,
		Errors:             []error{},
	}

	log.Println("[STARTUP] Validating critical requirements...")

	// 1. Validate encryption key (CRITICAL - IMMUTABLE SECURITY RULE)
	log.Println("[STARTUP] Checking AUTOSTACK_ENCRYPTION_KEY...")
	if err := crypto.ValidateEncryptionKeyAtStartup(); err != nil {
		result.Errors = append(result.Errors, err)
		log.Printf("[STARTUP] CRITICAL ERROR: %v", err)
	} else {
		result.EncryptionKeyValid = true
		log.Println("[STARTUP] ✓ Encryption key validated")
	}

	// 2. Validate Terraform binary (CRITICAL - REQUIRED FOR OPERATION)
	log.Println("[STARTUP] Checking Terraform binary...")
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	
	if err := terraform.ValidateTerraformBinary(ctx); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("terraform validation failed: %w", err))
		log.Printf("[STARTUP] CRITICAL ERROR: %v", err)
	} else {
		result.TerraformAvailable = true
		log.Println("[STARTUP] ✓ Terraform binary validated")
	}

	// 3. Check if all critical validations passed
	if len(result.Errors) > 0 {
		log.Println("[STARTUP] ✗ Startup validation FAILED")
		log.Println("[STARTUP] The application cannot start with the following errors:")
		for i, err := range result.Errors {
			log.Printf("[STARTUP]   %d. %v", i+1, err)
		}
		return result, fmt.Errorf("startup validation failed with %d error(s)", len(result.Errors))
	}

	log.Println("[STARTUP] ✓ All critical requirements validated successfully")
	return result, nil
}

// MustValidateStartup is a convenience function that validates startup requirements
// and exits the process if validation fails
// Use this in main.go to ensure the application never starts in an invalid state
func MustValidateStartup(ctx context.Context) {
	result, err := ValidateStartupRequirements(ctx)
	if err != nil {
		log.Println("═══════════════════════════════════════════════════════════")
		log.Println("CRITICAL STARTUP FAILURE")
		log.Println("═══════════════════════════════════════════════════════════")
		log.Println("")
		log.Println("The application cannot start due to missing or invalid configuration.")
		log.Println("")
		log.Println("Required fixes:")
		log.Println("")
		
		if !result.EncryptionKeyValid {
			log.Println("  1. Set AUTOSTACK_ENCRYPTION_KEY environment variable")
			log.Println("     - Must be a base64-encoded 32-byte key")
			log.Println("     - Generate with: openssl rand -base64 32")
			log.Println("     - Example: export AUTOSTACK_ENCRYPTION_KEY=\"your-base64-key-here\"")
			log.Println("")
		}
		
		if !result.TerraformAvailable {
			log.Println("  2. Install Terraform binary")
			log.Println("     - Download from: https://www.terraform.io/downloads")
			log.Println("     - Ensure 'terraform' is in your PATH")
			log.Println("     - Verify with: terraform version")
			log.Println("")
		}
		
		log.Println("═══════════════════════════════════════════════════════════")
		log.Println("")
		log.Fatal("Application startup aborted")
	}
}

// LogStartupBanner logs a startup banner with version and security information
func LogStartupBanner(version string) {
	log.Println("═══════════════════════════════════════════════════════════")
	log.Println("AutoStack Platform")
	log.Printf("Version: %s", version)
	log.Println("═══════════════════════════════════════════════════════════")
	log.Println("")
	log.Println("Security Features:")
	log.Println("  ✓ AES-256-GCM credential encryption")
	log.Println("  ✓ Isolated Terraform working directories")
	log.Println("  ✓ User-specific S3 state isolation")
	log.Println("  ✓ Input sanitization and validation")
	log.Println("  ✓ Ownership-based access control")
	log.Println("")
}
