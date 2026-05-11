package intelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// AnomalyExplanation represents an AI-generated explanation for a cost spike
type AnomalyExplanation struct {
	Summary     string   `json:"summary"`
	LikelyCause string   `json:"likely_cause"`
	ActionSteps []string `json:"action_steps"`
}

// AnomalyExplainer provides AI explanations for cost anomalies
type AnomalyExplainer struct {
	claude *ClaudeClient
}

// NewAnomalyExplainer creates a new anomaly explainer
func NewAnomalyExplainer() *AnomalyExplainer {
	return &AnomalyExplainer{
		claude: NewClaudeClient(),
	}
}

// ExplainAnomaly analyzes cost data and returns an explanation
func (e *AnomalyExplainer) ExplainAnomaly(ctx context.Context, deploymentName string, breakdown map[string]float64, variance float64) (*AnomalyExplanation, error) {
	systemPrompt := `You are the AutoStack AI Anomaly Explainer. Your job is to provide plain-English explanations for cost spikes in AWS deployments.
Analyze the provided cost breakdown and variance percentage. Identify which services contributed most to the increase and suggest likely causes (e.g., traffic spike, retry loop, over-provisioning).

Return your explanation as a JSON object:
{
  "summary": "...",
  "likely_cause": "...",
  "action_steps": ["...", "..."]
}`

	breakdownJSON, _ := json.Marshal(breakdown)
	userPrompt := fmt.Sprintf("Deployment: %s\nVariance: %.2f%%\nCost Breakdown: %s\n\nExplain why this cost spike occurred and what can be done.", 
		deploymentName, variance, string(breakdownJSON))

	respText, err := e.claude.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("AI explainer failed: %w", err)
	}

	jsonStr := e.extractJSON(respText)
	
	var explanation AnomalyExplanation
	if err := json.Unmarshal([]byte(jsonStr), &explanation); err != nil {
		return nil, fmt.Errorf("failed to parse AI explanation: %w", err)
	}

	return &explanation, nil
}

func (e *AnomalyExplainer) extractJSON(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start != -1 && end != -1 && end > start {
		return text[start : end+1]
	}
	return text
}
