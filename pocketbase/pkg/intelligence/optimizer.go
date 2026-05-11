package intelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
)

// CostRecommendation represents a single cost optimization suggestion
type CostRecommendation struct {
	ResourceID      string  `json:"resource"`
	CurrentCost     float64 `json:"current_cost"`
	PotentialSaving float64 `json:"potential_saving"`
	Recommendation  string  `json:"recommendation"`
	Action          string  `json:"action"`
	Category        string  `json:"category"` // e.g., "Right-sizing", "Cleanup", "Reserved"
}

// CostOptimizer analyzes cost data and suggests optimizations
type CostOptimizer struct {
	app    core.App
	claude *ClaudeClient
}

// NewCostOptimizer creates a new cost optimizer
func NewCostOptimizer(app core.App) *CostOptimizer {
	return &CostOptimizer{
		app:    app,
		claude: NewClaudeClient(),
	}
}

// RunOptimizationJob runs the weekly optimization analysis for all users
func (co *CostOptimizer) RunOptimizationJob() error {
	log.Println("[CostOptimizer] Starting weekly optimization job...")
	
	users, err := co.app.Dao().FindRecordsByFilter("_pb_users_auth_", "id != ''", "", 0, 0)
	if err != nil {
		return fmt.Errorf("failed to fetch users: %w", err)
	}

	for _, user := range users {
		if err := co.OptimizeForUser(context.Background(), user.Id); err != nil {
			log.Printf("[CostOptimizer] Error optimizing for user %s: %v", user.Id, err)
		}
	}

	log.Println("[CostOptimizer] Weekly optimization job completed")
	return nil
}

// OptimizeForUser generates cost recommendations for a specific user
func (co *CostOptimizer) OptimizeForUser(ctx context.Context, userID string) error {
	// 1. Fetch 30 days of cost data
	costData, err := co.getUserCostData(userID, 30)
	if err != nil {
		return err
	}

	if len(costData) == 0 {
		return nil // No data to analyze
	}

	// 2. Prepare data for Claude
	dataJSON, _ := json.Marshal(costData)
	
	systemPrompt := `You are the AutoStack AI Cost Optimizer. Analyze the provided AWS cost data for the last 30 days and identify optimization opportunities.
Return your recommendations as a JSON array of objects:
[{ "resource": "...", "current_cost": 12.34, "potential_saving": 5.67, "recommendation": "...", "action": "...", "category": "..." }]
Focus on: Right-sizing (e.g., downsizing t3.medium to t3.small), Unused resources, and Reserved Instance opportunities.`

	userPrompt := fmt.Sprintf("Analyze this cost data and suggest optimizations: %s", string(dataJSON))

	// 3. Get recommendations from Claude
	respText, err := co.claude.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		return fmt.Errorf("Claude optimization failed: %w", err)
	}

	// 4. Parse recommendations
	var recommendations []CostRecommendation
	jsonStr := co.extractJSONArray(respText)
	if err := json.Unmarshal([]byte(jsonStr), &recommendations); err != nil {
		return fmt.Errorf("failed to parse recommendations: %w", err)
	}

	// 5. Store recommendations in DB
	return co.saveRecommendations(userID, recommendations)
}

func (co *CostOptimizer) getUserCostData(userID string, days int) ([]map[string]interface{}, error) {
	startTime := time.Now().AddDate(0, 0, -days)
	
	records, err := co.app.Dao().FindRecordsByFilter(
		"actualCosts",
		"deployment.user = {:user} && created >= {:start}",
		"-created",
		1000,
		0,
		map[string]interface{}{
			"user":  userID,
			"start": startTime.Format("2006-01-02 15:04:05"),
		},
	)
	if err != nil {
		return nil, err
	}

	data := make([]map[string]interface{}, len(records))
	for i, r := range records {
		data[i] = map[string]interface{}{
			"deployment": r.GetString("deployment"),
			"cost":       r.GetFloat("costToDate"),
			"breakdown":  r.Get("breakdown"),
			"date":       r.GetDateTime("created").String(),
		}
	}
	return data, nil
}

func (co *CostOptimizer) saveRecommendations(userID string, recs []CostRecommendation) error {
	// Create or update costRecommendations record in DB
	// For now, we'll store it as a JSON field in a new collection or settings
	// Let's check if there's a suitable collection
	collection, err := co.app.Dao().FindCollectionByNameOrId("costAlerts") // Reusing for now or create new
	if err != nil {
		return err
	}

	// We'll create a special alert type "OPTIMIZATION"
	for _, rec := range recs {
		alert := models.NewRecord(collection)
		alert.Set("user", userID)
		alert.Set("type", "OPTIMIZATION")
		alert.Set("severity", "LOW")
		alert.Set("title", fmt.Sprintf("Optimization: %s", rec.Category))
		alert.Set("message", rec.Recommendation)
		
		metadata, _ := json.Marshal(rec)
		alert.Set("metadata", string(metadata))
		
		if err := co.app.Dao().SaveRecord(alert); err != nil {
			log.Printf("Failed to save optimization alert: %v", err)
		}
	}
	
	return nil
}

func (co *CostOptimizer) extractJSONArray(text string) string {
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start != -1 && end != -1 && end > start {
		return text[start : end+1]
	}
	return "[]"
}
