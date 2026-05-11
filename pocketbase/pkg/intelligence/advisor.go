package intelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// DeploymentRecommendation represents a recommendation from the AI advisor
type DeploymentRecommendation struct {
	BlueprintID  string `json:"blueprint_id"`
	Region       string `json:"region"`
	InstanceSize string `json:"instance_size"`
	Reasoning    string `json:"reasoning"`
	EstimatedCost string `json:"estimated_cost"`
	Tradeoffs    []string `json:"tradeoffs"`
}

// AIAdvisor provides intelligent deployment recommendations
type AIAdvisor struct {
	claude *ClaudeClient
}

// NewAIAdvisor creates a new AI advisor
func NewAIAdvisor() *AIAdvisor {
	return &AIAdvisor{
		claude: NewClaudeClient(),
	}
}

// GetRecommendation analyzes a user's app description and returns a recommendation
func (a *AIAdvisor) GetRecommendation(ctx context.Context, description string) (*DeploymentRecommendation, error) {
	systemPrompt := `You are the AutoStack AI Deployment Advisor. Your job is to recommend the best AWS infrastructure setup for a user's application description.
Available Blueprints:
- ecs-web-app: Best for containerized web applications, microservices, and APIs.
- full-stack: Best for apps needing both compute (ECS) and a managed database (RDS).
- static-site: Best for frontend-only apps (React, Svelte, Vue) using S3 and CloudFront.
- serverless: Best for event-driven functions or low-traffic APIs using Lambda and API Gateway.

Regions: us-east-1, us-east-2, us-west-2, eu-west-1, ap-southeast-1.

Return your recommendation as a JSON object with the following fields:
{
  "blueprint_id": "...",
  "region": "...",
  "instance_size": "...",
  "reasoning": "...",
  "estimated_cost": "...",
  "tradeoffs": ["...", "..."]
}`

	userPrompt := fmt.Sprintf("Analyze this application description and recommend a deployment strategy: %s", description)

	respText, err := a.claude.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("AI advisor failed: %w", err)
	}

	// Extract JSON if Claude added conversational text
	jsonStr := a.extractJSON(respText)
	
	var recommendation DeploymentRecommendation
	if err := json.Unmarshal([]byte(jsonStr), &recommendation); err != nil {
		return nil, fmt.Errorf("failed to parse AI recommendation: %w", err)
	}

	return &recommendation, nil
}

func (a *AIAdvisor) extractJSON(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start != -1 && end != -1 && end > start {
		return text[start : end+1]
	}
	return text
}
