package universal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/intelligence"
)

const terraformGeneratorSystemPrompt = `You are an expert AWS Terraform engineer.
You write production-quality Terraform HCL that is:
- Valid and passes terraform validate
- Idiomatic (uses data sources, locals, and outputs correctly)
- Secure (no public RDS, encrypted storage, least-privilege IAM)
- Complete (every referenced resource is declared)

Rules you MUST follow:
1. Always include required_providers block with exact versions
2. Always use variables for values the user might want to change (instance sizes, etc.)
3. Always create outputs for: endpoint URL, database connection string placeholder, any resource ARN needed by the app
4. Never hardcode AWS account IDs or credentials
5. Always encrypt RDS instances (storage_encrypted = true)
6. Always encrypt S3 buckets (server_side_encryption_configuration)
7. Never make RDS publicly accessible (publicly_accessible = false)
8. Always tag every resource with: Project, Environment, ManagedBy = "autostack"
9. Use aws_secretsmanager_secret for database passwords — never put them in environment variables directly
10. Return ONLY the Terraform HCL. No explanation. No markdown fences. No comments except inline # comments within the HCL itself.`

const terraformGeneratorUserPrompt = `Generate complete Terraform HCL for this infrastructure specification:

%s

The Terraform must:
- Deploy to AWS region: %s
- Use this naming prefix for all resources: %s
- Create all networking (VPC, subnets, security groups, route tables, IGW%s)
- Create all compute resources listed in the spec
- Create all managed services listed in the spec
- Wire everything together (security group rules that allow ECS → RDS, ECS → Redis, etc.)
- Output the application endpoint URL as "app_endpoint"
- Output connection strings as environment-variable-ready values (e.g., DATABASE_URL in postgres:// format)`

// GenerateTerraform calls the LLM to generate Terraform HCL from an InfrastructureSpec
func GenerateTerraform(ctx context.Context, spec InfrastructureSpec) (string, error) {
	client := intelligence.NewClaudeClient()

	specJSON, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal spec: %w", err)
	}

	natNote := ""
	if spec.Network.EnableNATGateway {
		natNote = ", NAT Gateway"
	}

	userPrompt := fmt.Sprintf(terraformGeneratorUserPrompt,
		string(specJSON),
		spec.Region,
		spec.ProjectName,
		natNote,
	)

	hcl, err := client.Complete(ctx, terraformGeneratorSystemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("LLM terraform generation failed: %w", err)
	}

	// Strip any markdown fences the model might have added despite instructions
	hcl = stripMarkdownFences(hcl)

	return hcl, nil
}

// stripMarkdownFences removes ```hcl ... ``` or ``` ... ``` wrappers
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	// Remove opening fence
	for _, fence := range []string{"```hcl\n", "```terraform\n", "```\n"} {
		if strings.HasPrefix(s, fence) {
			s = s[len(fence):]
			break
		}
	}
	// Remove closing fence
	if strings.HasSuffix(s, "\n```") {
		s = s[:len(s)-4]
	} else if strings.HasSuffix(s, "```") {
		s = s[:len(s)-3]
	}
	return strings.TrimSpace(s)
}
