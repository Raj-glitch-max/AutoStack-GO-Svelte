package universal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Raj-glitch-max/AutoStack-GO-Svelte/pkg/intelligence"
)

const analyzerSystemPrompt = `You are an infrastructure analyst. Given information about an application, extract a structured profile.
Return ONLY a JSON object matching this exact schema, no explanation:
{
  "language": string,
  "framework": string,
  "app_type": "api" | "fullstack" | "static_site" | "worker" | "websocket" | "ml_inference" | "scheduled_job",
  "port": number,
  "resource_profile": "minimal" | "standard" | "compute_heavy" | "memory_heavy" | "gpu",
  "required_services": [
    {
      "service_type": string,
      "version": string,
      "is_critical": boolean,
      "env_var_name": string
    }
  ],
  "needs_file_storage": boolean,
  "needs_object_storage": boolean,
  "expects_web_traffic": boolean,
  "expects_websocket": boolean,
  "is_background_worker": boolean,
  "is_scheduled_job": boolean,
  "reasoning": string
}`

// AIAnalyzeApp uses the LLM to analyze an ambiguous app description.
// Called when DetectionConfidence < 0.6 from the deterministic analyzer.
func AIAnalyzeApp(ctx context.Context, raw RawAppDescription, partial AppProfile) (AppProfile, error) {
	client := intelligence.NewClaudeClient()

	userPrompt := fmt.Sprintf(`Analyze this application and return a structured profile.

Files found: %s
Package dependencies (first 2000 chars): %s
Environment variable keys: %s
Partial detection result: language=%s, framework=%s, confidence=%.2f
Natural language description: %s`,
		strings.Join(raw.Files[:min(len(raw.Files), 50)], ", "),
		truncate(collectDependencyText(raw.SourceDir, partial.Language), 2000),
		strings.Join(raw.EnvVarKeys, ", "),
		partial.Language,
		partial.Framework,
		partial.DetectionConfidence,
		raw.NaturalLanguage,
	)

	respText, err := client.Complete(ctx, analyzerSystemPrompt, userPrompt)
	if err != nil {
		return partial, fmt.Errorf("AI analyzer failed: %w", err)
	}

	// Extract JSON from response
	jsonStr := extractJSON(respText)

	var result struct {
		Language           string            `json:"language"`
		Framework          string            `json:"framework"`
		AppType            string            `json:"app_type"`
		Port               int               `json:"port"`
		ResourceProfile    string            `json:"resource_profile"`
		RequiredServices   []struct {
			ServiceType string `json:"service_type"`
			Version     string `json:"version"`
			IsCritical  bool   `json:"is_critical"`
			EnvVarName  string `json:"env_var_name"`
		} `json:"required_services"`
		NeedsFileStorage   bool `json:"needs_file_storage"`
		NeedsObjectStorage bool `json:"needs_object_storage"`
		ExpectsWebTraffic  bool `json:"expects_web_traffic"`
		ExpectsWebSocket   bool `json:"expects_websocket"`
		IsBackgroundWorker bool `json:"is_background_worker"`
		IsScheduledJob     bool `json:"is_scheduled_job"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return partial, fmt.Errorf("failed to parse AI response: %w", err)
	}

	// Merge AI result into profile
	profile := partial
	if result.Language != "" {
		profile.Language = result.Language
	}
	if result.Framework != "" {
		profile.Framework = result.Framework
	}
	if result.AppType != "" {
		profile.AppType = AppType(result.AppType)
	}
	if result.Port > 0 {
		profile.Port = result.Port
	}
	if result.ResourceProfile != "" {
		profile.ResourceProfile = ResourceProfile(result.ResourceProfile)
	}
	if len(result.RequiredServices) > 0 {
		profile.RequiredServices = nil
		for _, s := range result.RequiredServices {
			profile.RequiredServices = append(profile.RequiredServices, RequiredService{
				ServiceType: s.ServiceType,
				Version:     s.Version,
				IsCritical:  s.IsCritical,
				EnvVarName:  s.EnvVarName,
			})
		}
	}
	profile.NeedsFileStorage = result.NeedsFileStorage
	profile.NeedsObjectStorage = result.NeedsObjectStorage
	profile.ExpectsWebTraffic = result.ExpectsWebTraffic
	profile.ExpectsWebSocket = result.ExpectsWebSocket
	profile.IsBackgroundWorker = result.IsBackgroundWorker
	profile.IsScheduledJob = result.IsScheduledJob
	profile.DetectionConfidence = 0.85 // AI analysis is reasonably confident
	profile.IsStaticSite = profile.AppType == AppTypeStaticSite

	return profile, nil
}

// AnalyzeFromNaturalLanguage creates an AppProfile from a plain-English description
func AnalyzeFromNaturalLanguage(ctx context.Context, description string, projectName string) (AppProfile, error) {
	raw := RawAppDescription{
		InputType:       "natural_language",
		NaturalLanguage: description,
		ProjectName:     projectName,
	}
	partial := AppProfile{
		ProjectName:         projectName,
		Port:                8080,
		DetectionConfidence: 0.0,
	}
	return AIAnalyzeApp(ctx, raw, partial)
}

// extractJSON extracts the first JSON object from a string
func extractJSON(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start != -1 && end != -1 && end > start {
		return text[start : end+1]
	}
	return text
}

// truncate truncates a string to maxLen characters
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// min returns the smaller of two ints
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
