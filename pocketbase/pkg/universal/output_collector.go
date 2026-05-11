package universal

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase"
)

// CollectOutputs parses terraform output -json and stores results in the DB
func CollectOutputs(outputJSON string, deploymentID string, app *pocketbase.PocketBase) (*DeploymentOutputs, error) {
	var rawOutputs map[string]struct {
		Value interface{} `json:"value"`
		Type  string      `json:"type"`
	}
	if err := json.Unmarshal([]byte(outputJSON), &rawOutputs); err != nil {
		return nil, fmt.Errorf("failed to parse terraform outputs: %w", err)
	}

	outputs := &DeploymentOutputs{
		ResourceARNs: make(map[string]string),
		EnvVars:      make(map[string]string),
		RawOutputs:   make(map[string]interface{}),
	}

	for key, output := range rawOutputs {
		strVal := fmt.Sprintf("%v", output.Value)
		outputs.RawOutputs[key] = output.Value

		switch key {
		case "app_endpoint":
			outputs.AppEndpoint = strVal
		default:
			if strings.HasSuffix(key, "_arn") {
				outputs.ResourceARNs[key] = strVal
			}
			// Connection strings — store as env vars
			if strings.HasSuffix(key, "_url") ||
				strings.HasPrefix(key, "database") ||
				strings.HasPrefix(key, "redis") ||
				strings.HasPrefix(key, "rabbitmq") ||
				strings.HasPrefix(key, "kafka") {
				outputs.EnvVars[strings.ToUpper(key)] = strVal
			}
		}
	}

	// Update deployment record
	if app != nil {
		record, err := app.Dao().FindRecordById("awsDeployments", deploymentID)
		if err != nil {
			return outputs, fmt.Errorf("deployment record not found: %w", err)
		}

		outputsJSON, _ := json.Marshal(outputs.RawOutputs)
		record.Set("outputs", string(outputsJSON))
		record.Set("status", "active")

		if outputs.AppEndpoint != "" {
			record.Set("endpoint_url", outputs.AppEndpoint)
		}

		if err := app.Dao().SaveRecord(record); err != nil {
			return outputs, fmt.Errorf("failed to save deployment outputs: %w", err)
		}
	}

	return outputs, nil
}
