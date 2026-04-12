package cost

import (
	"testing"
)

func TestNewUsageAssumptionsManager(t *testing.T) {
	manager := NewUsageAssumptionsManager()

	if manager == nil {
		t.Fatal("Expected manager to be created, got nil")
	}

	if manager.assumptions == nil {
		t.Fatal("Expected assumptions map to be initialized")
	}

	// Verify all blueprint types have assumptions
	expectedBlueprints := []string{
		string(BlueprintStaticWebsite),
		string(BlueprintWebApp),
		string(BlueprintFullStack),
		string(BlueprintMicroservices),
	}

	for _, blueprintType := range expectedBlueprints {
		if _, exists := manager.assumptions[blueprintType]; !exists {
			t.Errorf("Expected assumptions for %s to be initialized", blueprintType)
		}
	}
}

func TestGetAssumptions(t *testing.T) {
	manager := NewUsageAssumptionsManager()

	tests := []struct {
		name          string
		blueprintType string
		expectError   bool
	}{
		{
			name:          "Static Website",
			blueprintType: string(BlueprintStaticWebsite),
			expectError:   false,
		},
		{
			name:          "Web Application",
			blueprintType: string(BlueprintWebApp),
			expectError:   false,
		},
		{
			name:          "Full Stack",
			blueprintType: string(BlueprintFullStack),
			expectError:   false,
		},
		{
			name:          "Microservices",
			blueprintType: string(BlueprintMicroservices),
			expectError:   false,
		},
		{
			name:          "Invalid Blueprint",
			blueprintType: "invalid-blueprint",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assumptions, err := manager.GetAssumptions(tt.blueprintType)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error for invalid blueprint type, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if assumptions == nil {
				t.Fatal("Expected assumptions to be returned, got nil")
			}

			if assumptions.BlueprintType != tt.blueprintType {
				t.Errorf("Expected blueprint type %s, got %s", tt.blueprintType, assumptions.BlueprintType)
			}

			if len(assumptions.Assumptions) == 0 {
				t.Error("Expected assumptions list to have items")
			}

			if len(assumptions.AsMap) == 0 {
				t.Error("Expected assumptions map to have items")
			}
		})
	}
}

func TestGetAssumptionsAsMap(t *testing.T) {
	manager := NewUsageAssumptionsManager()

	assumptionsMap, err := manager.GetAssumptionsAsMap(string(BlueprintStaticWebsite))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify expected keys exist
	expectedKeys := []string{"storage", "requests", "dataTransfer", "customDomain"}
	for _, key := range expectedKeys {
		if _, exists := assumptionsMap[key]; !exists {
			t.Errorf("Expected key %s to exist in assumptions map", key)
		}
	}

	// Verify values are not empty
	for key, value := range assumptionsMap {
		if value == "" {
			t.Errorf("Expected non-empty value for key %s", key)
		}
	}
}

func TestGetAssumptionsList(t *testing.T) {
	manager := NewUsageAssumptionsManager()

	assumptionsList, err := manager.GetAssumptionsList(string(BlueprintWebApp))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(assumptionsList) == 0 {
		t.Fatal("Expected assumptions list to have items")
	}

	// Verify each assumption has required fields
	for _, assumption := range assumptionsList {
		if assumption.Key == "" {
			t.Error("Expected assumption to have a key")
		}
		if assumption.Value == "" {
			t.Error("Expected assumption to have a value")
		}
		if assumption.Description == "" {
			t.Error("Expected assumption to have a description")
		}
	}
}

func TestUpdateAssumption(t *testing.T) {
	manager := NewUsageAssumptionsManager()

	tests := []struct {
		name          string
		blueprintType string
		key           string
		value         string
		expectError   bool
	}{
		{
			name:          "Update valid assumption",
			blueprintType: string(BlueprintStaticWebsite),
			key:           "storage",
			value:         "20 GB",
			expectError:   false,
		},
		{
			name:          "Update invalid blueprint",
			blueprintType: "invalid-blueprint",
			key:           "storage",
			value:         "20 GB",
			expectError:   true,
		},
		{
			name:          "Update invalid key",
			blueprintType: string(BlueprintStaticWebsite),
			key:           "invalid-key",
			value:         "20 GB",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.UpdateAssumption(tt.blueprintType, tt.key, tt.value)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Verify the update was applied
			assumptions, _ := manager.GetAssumptions(tt.blueprintType)
			if assumptions.AsMap[tt.key] != tt.value {
				t.Errorf("Expected value %s, got %s", tt.value, assumptions.AsMap[tt.key])
			}

			// Verify the list was also updated
			found := false
			for _, assumption := range assumptions.Assumptions {
				if assumption.Key == tt.key && assumption.Value == tt.value {
					found = true
					break
				}
			}
			if !found {
				t.Error("Expected assumption to be updated in list")
			}
		})
	}
}

func TestSetCustomAssumptions(t *testing.T) {
	manager := NewUsageAssumptionsManager()

	customAssumptions := map[string]string{
		"storage":      "50 GB",
		"requests":     "100,000 per month",
		"dataTransfer": "500 GB per month",
	}

	err := manager.SetCustomAssumptions(string(BlueprintStaticWebsite), customAssumptions)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify all custom assumptions were applied
	assumptions, _ := manager.GetAssumptions(string(BlueprintStaticWebsite))
	for key, expectedValue := range customAssumptions {
		if actualValue := assumptions.AsMap[key]; actualValue != expectedValue {
			t.Errorf("Expected %s to be %s, got %s", key, expectedValue, actualValue)
		}
	}
}

func TestGetSupportedBlueprints(t *testing.T) {
	manager := NewUsageAssumptionsManager()

	blueprints := manager.GetSupportedBlueprints()

	if len(blueprints) == 0 {
		t.Fatal("Expected supported blueprints list to have items")
	}

	// Verify expected blueprints are present
	expectedBlueprints := map[string]bool{
		string(BlueprintStaticWebsite): false,
		string(BlueprintWebApp):        false,
		string(BlueprintFullStack):     false,
		string(BlueprintMicroservices): false,
	}

	for _, blueprint := range blueprints {
		if _, exists := expectedBlueprints[blueprint]; exists {
			expectedBlueprints[blueprint] = true
		}
	}

	for blueprint, found := range expectedBlueprints {
		if !found {
			t.Errorf("Expected blueprint %s to be in supported list", blueprint)
		}
	}
}

func TestFormatAssumptionsForDisplay(t *testing.T) {
	manager := NewUsageAssumptionsManager()

	display, err := manager.FormatAssumptionsForDisplay(string(BlueprintStaticWebsite))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if display == "" {
		t.Error("Expected non-empty display string")
	}

	// Verify the display contains the blueprint type
	if len(display) < 10 {
		t.Error("Expected display string to have meaningful content")
	}
}

func TestValidateAssumptionValue(t *testing.T) {
	manager := NewUsageAssumptionsManager()

	tests := []struct {
		name          string
		blueprintType string
		key           string
		value         string
		expectError   bool
	}{
		{
			name:          "Valid assumption",
			blueprintType: string(BlueprintStaticWebsite),
			key:           "storage",
			value:         "20 GB",
			expectError:   false,
		},
		{
			name:          "Empty value",
			blueprintType: string(BlueprintStaticWebsite),
			key:           "storage",
			value:         "",
			expectError:   true,
		},
		{
			name:          "Invalid blueprint",
			blueprintType: "invalid-blueprint",
			key:           "storage",
			value:         "20 GB",
			expectError:   true,
		},
		{
			name:          "Invalid key",
			blueprintType: string(BlueprintStaticWebsite),
			key:           "invalid-key",
			value:         "20 GB",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateAssumptionValue(tt.blueprintType, tt.key, tt.value)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestStaticWebsiteAssumptions(t *testing.T) {
	manager := NewUsageAssumptionsManager()

	assumptions, err := manager.GetAssumptions(string(BlueprintStaticWebsite))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify specific assumptions for static website
	expectedAssumptions := map[string]bool{
		"storage":         false,
		"requests":        false,
		"dataTransfer":    false,
		"customDomain":    false,
		"cacheHitRatio":   false,
		"compressionRate": false,
	}

	for _, assumption := range assumptions.Assumptions {
		if _, exists := expectedAssumptions[assumption.Key]; exists {
			expectedAssumptions[assumption.Key] = true
		}
	}

	for key, found := range expectedAssumptions {
		if !found {
			t.Errorf("Expected assumption %s to be present for static website", key)
		}
	}
}

func TestWebAppAssumptions(t *testing.T) {
	manager := NewUsageAssumptionsManager()

	assumptions, err := manager.GetAssumptions(string(BlueprintWebApp))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify specific assumptions for web application
	expectedKeys := []string{
		"compute",
		"taskCount",
		"database",
		"loadBalancer",
		"natGateway",
		"cloudwatchLogs",
		"availability",
		"uptime",
	}

	for _, key := range expectedKeys {
		if _, exists := assumptions.AsMap[key]; !exists {
			t.Errorf("Expected assumption %s to be present for web application", key)
		}
	}
}

func TestFullStackAssumptions(t *testing.T) {
	manager := NewUsageAssumptionsManager()

	assumptions, err := manager.GetAssumptions(string(BlueprintFullStack))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify specific assumptions for full-stack
	expectedKeys := []string{
		"frontend",
		"backend",
		"database",
		"assets",
		"cdn",
		"loadBalancer",
		"natGateway",
		"cloudwatchLogs",
		"availability",
		"backupRetention",
	}

	for _, key := range expectedKeys {
		if _, exists := assumptions.AsMap[key]; !exists {
			t.Errorf("Expected assumption %s to be present for full-stack", key)
		}
	}
}

func TestMicroservicesAssumptions(t *testing.T) {
	manager := NewUsageAssumptionsManager()

	assumptions, err := manager.GetAssumptions(string(BlueprintMicroservices))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify specific assumptions for microservices
	expectedKeys := []string{
		"services",
		"tasksPerService",
		"database",
		"storage",
		"cdn",
		"loadBalancer",
		"apiGateway",
		"messaging",
		"cache",
		"natGateway",
		"cloudwatchLogs",
		"availability",
	}

	for _, key := range expectedKeys {
		if _, exists := assumptions.AsMap[key]; !exists {
			t.Errorf("Expected assumption %s to be present for microservices", key)
		}
	}
}

func TestAssumptionsConsistency(t *testing.T) {
	manager := NewUsageAssumptionsManager()

	blueprintTypes := []string{
		string(BlueprintStaticWebsite),
		string(BlueprintWebApp),
		string(BlueprintFullStack),
		string(BlueprintMicroservices),
	}

	for _, blueprintType := range blueprintTypes {
		t.Run(blueprintType, func(t *testing.T) {
			assumptions, err := manager.GetAssumptions(blueprintType)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Verify list and map have same number of items
			if len(assumptions.Assumptions) != len(assumptions.AsMap) {
				t.Errorf("Expected list and map to have same number of items, got %d and %d",
					len(assumptions.Assumptions), len(assumptions.AsMap))
			}

			// Verify all list items are in map
			for _, assumption := range assumptions.Assumptions {
				if _, exists := assumptions.AsMap[assumption.Key]; !exists {
					t.Errorf("Expected key %s from list to exist in map", assumption.Key)
				}
			}

			// Verify all map items are in list
			for key := range assumptions.AsMap {
				found := false
				for _, assumption := range assumptions.Assumptions {
					if assumption.Key == key {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected key %s from map to exist in list", key)
				}
			}
		})
	}
}
